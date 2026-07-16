// Command alerting-relay receives Alertmanager webhooks, aggregates alerts by
// group, and keeps a single Slack message per group up to date: firing posts
// the root message (bare, no counts yet), and every later update — including
// resolution — edits the root's header with the current firing/resolved
// tally and drops a lightweight instance-list reply in its thread.
package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"alerting-relay/internal/slack"
	"alerting-relay/internal/store"
	"alerting-relay/internal/webhook"
)

// ClusterChannels is one cluster's Slack destinations, split by severity
// tier, plus an optional Grafana base URL override for that cluster.
type ClusterChannels struct {
	Alerting      string `json:"alerting"`
	Notifications string `json:"notifications"`
	GrafanaURL    string `json:"grafana_url,omitempty"`
}

type Config struct {
	DatabaseURL   string
	SlackToken    string
	SlackChannels map[string]ClusterChannels // cluster label -> channels
	WebhookToken  string
	GrafanaURL    string // default Grafana base URL, overridable per cluster
	Addr          string
}

func loadConfig() Config {
	var channels map[string]ClusterChannels
	if raw := os.Getenv("SLACK_CHANNELS"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &channels); err != nil {
			slog.Error("invalid SLACK_CHANNELS", "err", err)
			os.Exit(1)
		}
	}
	return Config{
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		SlackToken:    os.Getenv("SLACK_BOT_TOKEN"),
		SlackChannels: channels,
		WebhookToken:  os.Getenv("WEBHOOK_TOKEN"),
		GrafanaURL:    os.Getenv("GRAFANA_URL"),
		Addr:          ":8080",
	}
}

// parseLevel maps LOG_LEVEL to a slog level, defaulting to info for unset or
// unrecognized values.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// newLogger builds the process-wide logger from LOG_FORMAT ("json" or
// "text", default "text") and LOG_LEVEL ("debug"/"info"/"warn"/"error",
// default "info").
func newLogger() *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(os.Getenv("LOG_LEVEL"))}
	if strings.EqualFold(os.Getenv("LOG_FORMAT"), "json") {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

// highSeverity is the set of severity values routed to the "alerting" channel;
// anything else (warning, info, unset) goes to "notifications".
var highSeverity = map[string]bool{"critical": true, "high": true}

// resolveChannel picks the destination Slack channel from the alert's own
// cluster/severity labels — the relay is the single place this mapping lives,
// so Alertmanager doesn't need per-cluster receiver config to route correctly.
// Alerts with no "cluster" label (e.g. cluster-agnostic rules) route through
// the "default" entry, if configured.
func resolveChannel(channels map[string]ClusterChannels, labels map[string]string) (string, bool) {
	clusterLabel := labels["cluster"]
	if clusterLabel == "" {
		clusterLabel = "default"
	}
	cluster, ok := channels[clusterLabel]
	if !ok {
		return "", false
	}
	if highSeverity[labels["severity"]] {
		return cluster.Alerting, cluster.Alerting != ""
	}
	return cluster.Notifications, cluster.Notifications != ""
}

// resolveGrafanaURL picks the Grafana base URL for an alert's cluster label,
// preferring a per-cluster override in SLACK_CHANNELS, falling back to the
// GRAFANA_URL default.
func resolveGrafanaURL(channels map[string]ClusterChannels, defaultURL string, labels map[string]string) string {
	clusterLabel := labels["cluster"]
	if clusterLabel == "" {
		clusterLabel = "default"
	}
	if cluster, ok := channels[clusterLabel]; ok && cluster.GrafanaURL != "" {
		return cluster.GrafanaURL
	}
	return defaultURL
}

type relay struct {
	cfg   Config
	store *store.Store
	slack *slack.Client
}

func (rl *relay) handleWebhook(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(token), []byte(rl.cfg.WebhookToken)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var payload webhook.Payload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if err := rl.handleGroup(payload); err != nil {
		slog.Error("handle group", "receiver", payload.Receiver, "group_key", payload.GroupKey, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (rl *relay) handleGroup(payload webhook.Payload) error {
	existing, err := rl.store.Get(payload.Receiver, payload.GroupKey)
	if err != nil {
		return err
	}

	grafanaURL := resolveGrafanaURL(rl.cfg.SlackChannels, rl.cfg.GrafanaURL, payload.CommonLabels)

	if existing == nil {
		if payload.Status != "firing" {
			return nil // nothing to resolve, never saw it firing
		}
		channel, ok := resolveChannel(rl.cfg.SlackChannels, payload.CommonLabels)
		if !ok {
			return fmt.Errorf("no slack channel configured for cluster %q", payload.CommonLabels["cluster"])
		}
		ts, err := rl.slack.PostRoot(channel, slack.BuildAttachment(payload, grafanaURL, false))
		if err != nil {
			return err
		}
		return rl.store.Create(store.AlertGroup{
			Receiver:  payload.Receiver,
			GroupKey:  payload.GroupKey,
			Channel:   channel,
			MessageTS: ts,
			Status:    "firing",
		})
	}

	// Alert set changed since we last posted: keep the root's header current
	// and drop a lightweight instance-list note in its thread.
	if err := rl.slack.UpdateRoot(existing.Channel, existing.MessageTS, slack.BuildAttachment(payload, grafanaURL, true)); err != nil {
		return err
	}
	if err := rl.slack.PostThreadReply(existing.Channel, existing.MessageTS, slack.BuildThreadUpdate(payload)); err != nil {
		return err
	}

	if payload.Status == "resolved" {
		return rl.store.MarkResolved(payload.Receiver, payload.GroupKey)
	}
	return rl.store.Touch(payload.Receiver, payload.GroupKey)
}

func (rl *relay) startCleanupLoop() {
	ticker := time.NewTicker(time.Hour)
	go func() {
		for range ticker.C {
			n, err := rl.store.DeleteResolvedOlderThan(24 * time.Hour)
			if err != nil {
				slog.Error("cleanup", "err", err)
				continue
			}
			if n > 0 {
				slog.Info("cleanup reaped resolved groups", "count", n)
			}
		}
	}()
}

func main() {
	slog.SetDefault(newLogger())

	cfg := loadConfig()

	st, err := store.New(cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect store", "err", err)
		os.Exit(1)
	}

	rl := &relay{cfg: cfg, store: st, slack: slack.New(cfg.SlackToken)}
	rl.startCleanupLoop()

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", rl.handleWebhook)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	slog.Info("alerting-relay listening", "addr", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, mux); err != nil {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}
