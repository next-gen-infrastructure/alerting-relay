// Command alerting-relay receives Alertmanager webhooks, aggregates alerts by
// group, and keeps a single Slack message per group up to date: firing posts
// the root message, follow-ups reply in its thread, and resolution edits the
// root message and drops a final thread reply.
package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"alerting-relay/internal/slack"
	"alerting-relay/internal/store"
	"alerting-relay/internal/webhook"
)

// ClusterChannels is one cluster's Slack destinations, split by severity tier.
type ClusterChannels struct {
	Alerting      string `json:"alerting"`
	Notifications string `json:"notifications"`
}

type Config struct {
	DatabaseURL   string
	SlackToken    string
	SlackChannels map[string]ClusterChannels // cluster label -> channels
	WebhookToken  string
	Addr          string
}

func loadConfig() Config {
	var channels map[string]ClusterChannels
	if raw := os.Getenv("SLACK_CHANNELS"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &channels); err != nil {
			log.Fatalf("invalid SLACK_CHANNELS: %v", err)
		}
	}
	return Config{
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		SlackToken:    os.Getenv("SLACK_BOT_TOKEN"),
		SlackChannels: channels,
		WebhookToken:  os.Getenv("WEBHOOK_TOKEN"),
		Addr:          ":8080",
	}
}

// highSeverity is the set of severity values routed to the "alerting" channel;
// anything else (warning, info, unset) goes to "notifications".
var highSeverity = map[string]bool{"critical": true, "high": true}

// resolveChannel picks the destination Slack channel from the alert's own
// cluster/severity labels — the relay is the single place this mapping lives,
// so Alertmanager doesn't need per-cluster receiver config to route correctly.
func resolveChannel(channels map[string]ClusterChannels, labels map[string]string) (string, bool) {
	cluster, ok := channels[labels["cluster"]]
	if !ok {
		return "", false
	}
	if highSeverity[labels["severity"]] {
		return cluster.Alerting, cluster.Alerting != ""
	}
	return cluster.Notifications, cluster.Notifications != ""
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
		log.Printf("handle group %s/%s: %v", payload.Receiver, payload.GroupKey, err)
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

	attachment := slack.BuildAttachment(payload)

	if existing == nil {
		if payload.Status != "firing" {
			return nil // nothing to resolve, never saw it firing
		}
		channel, ok := resolveChannel(rl.cfg.SlackChannels, payload.CommonLabels)
		if !ok {
			return fmt.Errorf("no slack channel configured for cluster %q", payload.CommonLabels["cluster"])
		}
		ts, err := rl.slack.PostRoot(channel, attachment)
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

	if payload.Status == "resolved" {
		if err := rl.slack.UpdateRoot(existing.Channel, existing.MessageTS, attachment); err != nil {
			return err
		}
		if err := rl.slack.PostThreadReply(existing.Channel, existing.MessageTS, attachment); err != nil {
			return err
		}
		return rl.store.MarkResolved(payload.Receiver, payload.GroupKey)
	}

	// still firing, alert set changed since we last posted: reply in-thread.
	if err := rl.slack.PostThreadReply(existing.Channel, existing.MessageTS, attachment); err != nil {
		return err
	}
	return rl.store.Touch(payload.Receiver, payload.GroupKey)
}

func (rl *relay) startCleanupLoop() {
	ticker := time.NewTicker(time.Hour)
	go func() {
		for range ticker.C {
			n, err := rl.store.DeleteResolvedOlderThan(24 * time.Hour)
			if err != nil {
				log.Printf("cleanup: %v", err)
				continue
			}
			if n > 0 {
				log.Printf("cleanup: reaped %d resolved group(s)", n)
			}
		}
	}()
}

func main() {
	cfg := loadConfig()

	st, err := store.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect store: %v", err)
	}

	rl := &relay{cfg: cfg, store: st, slack: slack.New(cfg.SlackToken)}
	rl.startCleanupLoop()

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", rl.handleWebhook)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	log.Printf("alerting-relay listening on %s", cfg.Addr)
	log.Fatal(http.ListenAndServe(cfg.Addr, mux))
}
