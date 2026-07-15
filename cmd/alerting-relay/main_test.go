package main

import "testing"

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
}
