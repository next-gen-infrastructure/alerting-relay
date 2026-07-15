package main

import (
	"log/slog"
	"testing"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"INFO":    slog.LevelInfo,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
		"":        slog.LevelInfo,
		"bogus":   slog.LevelInfo,
	}
	for in, want := range cases {
		if got := parseLevel(in); got != want {
			t.Errorf("parseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestResolveChannel(t *testing.T) {
	channels := map[string]ClusterChannels{
		"dev": {Alerting: "C-DEV-ALERTS", Notifications: "C-DEV-NOTIFS"},
	}

	if ch, ok := resolveChannel(channels, map[string]string{"cluster": "dev", "severity": "critical"}); !ok || ch != "C-DEV-ALERTS" {
		t.Fatalf("expected dev critical -> alerting channel, got %q, ok=%v", ch, ok)
	}
	if ch, ok := resolveChannel(channels, map[string]string{"cluster": "dev", "severity": "high"}); !ok || ch != "C-DEV-ALERTS" {
		t.Fatalf("expected dev high -> alerting channel, got %q, ok=%v", ch, ok)
	}
	if ch, ok := resolveChannel(channels, map[string]string{"cluster": "dev", "severity": "warning"}); !ok || ch != "C-DEV-NOTIFS" {
		t.Fatalf("expected dev warning -> notifications channel, got %q, ok=%v", ch, ok)
	}
	if ch, ok := resolveChannel(channels, map[string]string{"cluster": "dev"}); !ok || ch != "C-DEV-NOTIFS" {
		t.Fatalf("expected dev with no severity -> notifications channel, got %q, ok=%v", ch, ok)
	}
	if _, ok := resolveChannel(channels, map[string]string{"cluster": "staging", "severity": "critical"}); ok {
		t.Fatalf("expected unknown cluster to have no channel")
	}

	channels["default"] = ClusterChannels{Alerting: "C-DEFAULT-ALERTS", Notifications: "C-DEFAULT-NOTIFS"}
	if ch, ok := resolveChannel(channels, map[string]string{"severity": "critical"}); !ok || ch != "C-DEFAULT-ALERTS" {
		t.Fatalf("expected no cluster label -> default alerting channel, got %q, ok=%v", ch, ok)
	}
	if ch, ok := resolveChannel(channels, map[string]string{"cluster": "", "severity": "warning"}); !ok || ch != "C-DEFAULT-NOTIFS" {
		t.Fatalf("expected empty cluster label -> default notifications channel, got %q, ok=%v", ch, ok)
	}
}

func TestResolveGrafanaURL(t *testing.T) {
	channels := map[string]ClusterChannels{
		"dev":  {GrafanaURL: "https://grafana-dev.example.com"},
		"prod": {},
	}

	if got := resolveGrafanaURL(channels, "https://grafana.example.com", map[string]string{"cluster": "dev"}); got != "https://grafana-dev.example.com" {
		t.Fatalf("expected per-cluster override, got %q", got)
	}
	if got := resolveGrafanaURL(channels, "https://grafana.example.com", map[string]string{"cluster": "prod"}); got != "https://grafana.example.com" {
		t.Fatalf("expected default when cluster has no override, got %q", got)
	}
	if got := resolveGrafanaURL(channels, "https://grafana.example.com", map[string]string{"cluster": "unknown"}); got != "https://grafana.example.com" {
		t.Fatalf("expected default for unknown cluster, got %q", got)
	}
}
