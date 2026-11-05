package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"alerting-relay/internal/slack"
	"alerting-relay/internal/store"
	"alerting-relay/internal/webhook"
)

// handleArgoCD tracks one Slack message per app+revision: the first event
// posts the root, later events (syncing/synced/sync-failed/degraded/healthy)
// edit it and reply in its thread. sync-failed/degraded post to the cluster's
// alerting channel, the rest to notifications. "healthy" closes the group in
// whichever channel it lives; a later failure on the same revision opens a fresh thread.
func (rl *relay) handleArgoCD(w http.ResponseWriter, r *http.Request) {
	if !rl.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var ev webhook.ArgoEvent
	if err := json.NewDecoder(r.Body).Decode(&ev); err != nil || ev.App == "" || !slack.ValidArgoEvent(ev.Event) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := rl.handleArgoEvent(ev); err != nil {
		slog.Error("handle argocd event", "app", ev.App, "event", ev.Event, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (rl *relay) handleArgoEvent(ev webhook.ArgoEvent) error {
	const receiver = "argocd"
	key := ev.App + "@" + ev.Revision
	existing, err := rl.store.Get(receiver, key)
	if err != nil {
		return err
	}

	if ev.Event == "healthy" && (existing == nil || existing.Status == "resolved") {
		return nil // nothing to recover
	}

	// Failures/degradations go to the cluster's alerting channel, everything
	// else to notifications. A failure on a revision whose thread lives in
	// notifications (e.g. it already synced) starts a new thread in alerting.
	labels := map[string]string{"cluster": ev.Cluster}
	bad := ev.Event == "sync-failed" || ev.Event == "degraded"
	if bad {
		labels["severity"] = "critical"
	}
	if existing == nil || existing.Status == "resolved" || (bad && !rl.inChannel(existing, labels)) {
		channel, ok := resolveChannel(rl.cfg.SlackChannels, labels)
		if !ok {
			return fmt.Errorf("no slack channel configured for cluster %q", ev.Cluster)
		}
		ts, err := rl.slack.PostRoot(channel, slack.BuildArgoAttachment(ev, true))
		if err != nil {
			return err
		}
		return rl.store.Create(store.AlertGroup{Receiver: receiver, GroupKey: key, Channel: channel, MessageTS: ts, Status: "firing"})
	}

	if err := rl.slack.UpdateRoot(existing.Channel, existing.MessageTS, slack.BuildArgoAttachment(ev, false)); err != nil {
		return err
	}
	if err := rl.slack.PostThreadReply(existing.Channel, existing.MessageTS, slack.BuildArgoAttachment(ev, true)); err != nil {
		return err
	}
	if ev.Event == "healthy" {
		return rl.store.MarkResolved(receiver, key)
	}
	return rl.store.Touch(receiver, key)
}

func (rl *relay) inChannel(ag *store.AlertGroup, labels map[string]string) bool {
	ch, _ := resolveChannel(rl.cfg.SlackChannels, labels)
	return ag.Channel == ch
}
