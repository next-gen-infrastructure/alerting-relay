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
	SlackTeams    map[string]string          // team label -> Slack user-group ID, for real pings
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
	var teams map[string]string
	if raw := os.Getenv("SLACK_TEAMS"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &teams); err != nil {
			slog.Error("invalid SLACK_TEAMS", "err", err)
			os.Exit(1)
		}
	}
	return Config{
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		SlackToken:    os.Getenv("SLACK_BOT_TOKEN"),
		SlackChannels: channels,
		SlackTeams:    teams,
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

// channelIndex builds a name-or-ID -> ID lookup from the workspace's channel
// list, so SLACK_CHANNELS can reference a channel by either.
func channelIndex(channels []slack.Channel) map[string]string {
	index := make(map[string]string, len(channels)*2)
	for _, c := range channels {
		index[c.Name] = c.ID
		index[c.ID] = c.ID
	}
	return index
}

// resolveChannels replaces every non-empty Alerting/Notifications reference
// in channels with its resolved Slack ID (in place). A reference that
// matches neither a channel name nor an ID is a hard error: without a real
// channel there's nowhere to post, so this fails startup instead of
// silently dropping alerts later.
func resolveChannels(channels map[string]ClusterChannels, index map[string]string) error {
	for cluster, cc := range channels {
		for _, field := range [...]*string{&cc.Alerting, &cc.Notifications} {
			if *field == "" {
				continue
			}
			name := strings.TrimPrefix(*field, "#")
			id, ok := index[name]
			if !ok {
				return fmt.Errorf("cluster %q: slack channel %q not found (by name or ID)", cluster, *field)
			}
			*field = id
		}
		channels[cluster] = cc
	}
	return nil
}

// teamIndex builds a handle-or-ID -> ID lookup from the workspace's user
// groups, so SLACK_TEAMS can reference a team by its Slack handle or ID.
func teamIndex(groups []slack.UserGroup) map[string]string {
	index := make(map[string]string, len(groups)*2)
	for _, g := range groups {
		index[strings.TrimPrefix(g.Handle, "@")] = g.ID
		index[g.ID] = g.ID
	}
	return index
}

// resolveTeams resolves each SLACK_TEAMS entry to its Slack user-group ID.
// An entry that isn't found is dropped (logged as a warning) rather than
// failing startup — BuildAttachment already falls back to a plain "@team"
// mention for any label missing from the returned map.
func resolveTeams(teams map[string]string, index map[string]string) map[string]string {
	resolved := make(map[string]string, len(teams))
	for label, name := range teams {
		id, ok := index[strings.TrimPrefix(name, "@")]
		if !ok {
			slog.Warn("slack team not found, falling back to plain mention", "team_label", label, "configured_name", name)
			continue
		}
		resolved[label] = id
	}
	return resolved
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
		ts, err := rl.slack.PostRoot(channel, slack.BuildAttachment(payload, grafanaURL, rl.cfg.SlackTeams, false))
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
	if err := rl.slack.UpdateRoot(existing.Channel, existing.MessageTS, slack.BuildAttachment(payload, grafanaURL, rl.cfg.SlackTeams, true)); err != nil {
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

	slackClient := slack.New(cfg.SlackToken)

	if len(cfg.SlackChannels) > 0 {
		channels, err := slackClient.ListChannels()
		if err != nil {
			slog.Error("list slack channels", "err", err)
			os.Exit(1)
		}
		if err := resolveChannels(cfg.SlackChannels, channelIndex(channels)); err != nil {
			slog.Error("resolve SLACK_CHANNELS", "err", err)
			os.Exit(1)
		}
	}

	if len(cfg.SlackTeams) > 0 {
		groups, err := slackClient.ListUserGroups()
		if err != nil {
			slog.Warn("list slack user groups, falling back to plain team mentions", "err", err)
			cfg.SlackTeams = map[string]string{}
		} else {
			cfg.SlackTeams = resolveTeams(cfg.SlackTeams, teamIndex(groups))
		}
	}

	rl := &relay{cfg: cfg, store: st, slack: slackClient}
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
