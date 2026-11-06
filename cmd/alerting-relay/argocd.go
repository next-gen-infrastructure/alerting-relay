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

// handleArgoCD tracks one Slack message per cluster+revision: the first event
// posts the root, later events (syncing/synced/sync-failed/degraded/healthy)
// from any app on that revision update the root's per-app status list (worst
// status wins) and reply in its thread. If any app is sync-failed/degraded the
// message lives in the cluster's alerting channel, otherwise notifications.
// The group closes once every app is "healthy"; a later event on the same
// revision opens a fresh thread.
func (rl *relay) handleArgoCD(w http.ResponseWriter, r *http.Request) {
	if !rl.authorized(r) {
		slog.Warn("argocd unauthorized", "remote", r.RemoteAddr)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var ev webhook.ArgoEvent
	err := json.NewDecoder(r.Body).Decode(&ev)
	if err != nil || ev.App == "" || !slack.ValidArgoEvent(ev.Event) {
		slog.Warn("argocd bad request", "method", r.Method, "app", ev.App, "event", ev.Event, "decode_err", err, "remote", r.RemoteAddr)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := rl.handleArgoEvent(ev); err != nil {
		slog.Error("handle argocd event", "app", ev.App, "event", ev.Event, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	slog.Info("argocd event handled", "app", ev.App, "event", ev.Event, "revision", ev.Revision, "cluster", ev.Cluster)
	w.WriteHeader(http.StatusOK)
}

func (rl *relay) handleArgoEvent(ev webhook.ArgoEvent) error {
	const receiver = "argocd"
	key := ev.Revision
	if ev.Cluster != "" {
		key = ev.Cluster + "@" + key
	}
	existing, err := rl.store.Get(receiver, key)
	if err != nil {
		return err
	}

	if ev.Event == "healthy" && (existing == nil || existing.Status == "resolved") {
		return nil // nothing to recover
	}
	if existing != nil && existing.Status == "resolved" {
		if err := rl.store.ClearAppStates(receiver, key); err != nil {
			return err
		}
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if err := rl.store.SetAppState(receiver, key, ev.App, string(b)); err != nil {
		return err
	}
	raw, err := rl.store.AppStates(receiver, key)
	if err != nil {
		return err
	}
	apps := make(map[string]webhook.ArgoEvent, len(raw))
	for app, s := range raw {
		var a webhook.ArgoEvent
		if json.Unmarshal([]byte(s), &a) == nil {
			apps[app] = a
		}
	}
	worst := slack.WorstArgoEvent(apps)

	// If any app on the revision failed/degraded, the message belongs in the
	// cluster's alerting channel, otherwise notifications. A failure on a
	// revision whose thread lives in notifications (e.g. it already synced)
	// starts a new thread in alerting.
	labels := map[string]string{"cluster": ev.Cluster}
	bad := worst == "sync-failed" || worst == "degraded"
	if bad {
		labels["severity"] = "critical"
	}
	if existing == nil || existing.Status == "resolved" || (bad && !rl.inChannel(existing, labels)) {
		channel, ok := resolveChannel(rl.cfg.SlackChannels, labels)
		if !ok {
			return fmt.Errorf("no slack channel configured for cluster %q", ev.Cluster)
		}
		ts, err := rl.slack.PostRoot(channel, slack.BuildArgoSummary(ev, apps))
		if err != nil {
			return err
		}
		return rl.store.Create(store.AlertGroup{Receiver: receiver, GroupKey: key, Channel: channel, MessageTS: ts, Status: "firing"})
	}

	if err := rl.slack.UpdateRoot(existing.Channel, existing.MessageTS, slack.BuildArgoSummary(ev, apps)); err != nil {
		return err
	}
	if err := rl.slack.PostThreadReply(existing.Channel, existing.MessageTS, slack.BuildArgoAttachment(ev, true)); err != nil {
		return err
	}
	if worst == "healthy" { // every app on the revision is healthy
		return rl.store.MarkResolved(receiver, key)
	}
	return rl.store.Touch(receiver, key)
}

func (rl *relay) inChannel(ag *store.AlertGroup, labels map[string]string) bool {
	ch, _ := resolveChannel(rl.cfg.SlackChannels, labels)
	return ag.Channel == ch
}
