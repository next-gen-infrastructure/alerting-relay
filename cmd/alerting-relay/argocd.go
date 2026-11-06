package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

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
// revision opens a fresh thread. Events are queued and flushed to Slack every
// argoFlushInterval, one root update and one combined reply per group.
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
	slog.Info("argocd event queued", "app", ev.App, "event", ev.Event, "revision", ev.Revision, "cluster", ev.Cluster)
	w.WriteHeader(http.StatusOK)
}

const argoReceiver = "argocd"

// argoFlushInterval is how long Argo CD events are accumulated before Slack is
// updated, so a burst of events costs one root update and one reply per group.
const argoFlushInterval = 5 * time.Second

func argoKey(ev webhook.ArgoEvent) string {
	if ev.Cluster == "" {
		return ev.Revision
	}
	return ev.Cluster + "@" + ev.Revision
}

// handleArgoEvent queues the event for the next flush.
// ponytail: queue is in memory, so events from the last <=5s are lost on restart; persist if that matters.
func (rl *relay) handleArgoEvent(ev webhook.ArgoEvent) error {
	key := argoKey(ev)
	existing, err := rl.store.Get(argoReceiver, key)
	if err != nil {
		return err
	}
	rl.argoMu.Lock()
	defer rl.argoMu.Unlock()
	if ev.Event == "healthy" && (existing == nil || existing.Status == "resolved") && len(rl.argoPending[key]) == 0 {
		return nil // nothing to recover
	}
	if rl.argoPending == nil {
		rl.argoPending = map[string][]webhook.ArgoEvent{}
	}
	rl.argoPending[key] = append(rl.argoPending[key], ev)
	return nil
}

func (rl *relay) startArgoFlushLoop() {
	ticker := time.NewTicker(argoFlushInterval)
	go func() {
		for range ticker.C {
			rl.flushArgo()
		}
	}()
}

// flushArgo drains the queue, updating Slack once per group.
func (rl *relay) flushArgo() {
	rl.argoMu.Lock()
	batches := rl.argoPending
	rl.argoPending = nil
	rl.argoMu.Unlock()
	for key, evs := range batches {
		if err := rl.flushArgoGroup(key, evs); err != nil {
			slog.Error("flush argocd group", "key", key, "events", len(evs), "err", err)
		}
	}
}

// latestPerApp keeps each app's last event, in order of first appearance.
func latestPerApp(evs []webhook.ArgoEvent) []webhook.ArgoEvent {
	idx := map[string]int{}
	var out []webhook.ArgoEvent
	for _, ev := range evs {
		if i, ok := idx[ev.App]; ok {
			out[i] = ev
		} else {
			idx[ev.App] = len(out)
			out = append(out, ev)
		}
	}
	return out
}

func (rl *relay) flushArgoGroup(key string, evs []webhook.ArgoEvent) error {
	latest := evs[len(evs)-1]
	existing, err := rl.store.Get(argoReceiver, key)
	if err != nil {
		return err
	}
	if existing != nil && existing.Status == "resolved" {
		if err := rl.store.ClearAppStates(argoReceiver, key); err != nil {
			return err
		}
	}
	evs = latestPerApp(evs)
	for _, ev := range evs {
		b, err := json.Marshal(ev)
		if err != nil {
			return err
		}
		if err := rl.store.SetAppState(argoReceiver, key, ev.App, string(b)); err != nil {
			return err
		}
	}
	raw, err := rl.store.AppStates(argoReceiver, key)
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
	labels := map[string]string{"cluster": latest.Cluster}
	bad := worst == "sync-failed" || worst == "degraded"
	if bad {
		labels["severity"] = "critical"
	}
	if existing == nil || existing.Status == "resolved" || (bad && !rl.inChannel(existing, labels)) {
		channel, ok := resolveChannel(rl.cfg.SlackChannels, labels)
		if !ok {
			return fmt.Errorf("no slack channel configured for cluster %q", latest.Cluster)
		}
		ts, err := rl.slack.PostRoot(channel, slack.BuildArgoSummary(latest, apps))
		if err != nil {
			return err
		}
		return rl.store.Create(store.AlertGroup{Receiver: argoReceiver, GroupKey: key, Channel: channel, MessageTS: ts, Status: "firing"})
	}

	if err := rl.slack.UpdateRoot(existing.Channel, existing.MessageTS, slack.BuildArgoSummary(latest, apps)); err != nil {
		return err
	}
	if err := rl.slack.PostThreadReply(existing.Channel, existing.MessageTS, slack.BuildArgoAttachment(evs)); err != nil {
		return err
	}
	if worst == "healthy" { // every app on the revision is healthy
		return rl.store.MarkResolved(argoReceiver, key)
	}
	return rl.store.Touch(argoReceiver, key)
}

func (rl *relay) inChannel(ag *store.AlertGroup, labels map[string]string) bool {
	ch, _ := resolveChannel(rl.cfg.SlackChannels, labels)
	return ag.Channel == ch
}
