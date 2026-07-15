package slack

import (
	"testing"

	slackapi "github.com/slack-go/slack"

	"alerting-relay/internal/webhook"
)

func testPayload(status string) webhook.Payload {
	return webhook.Payload{
		Receiver: "team-slack",
		Status:   status,
		CommonLabels: map[string]string{
			"alertname": "HighCPU",
			"severity":  "critical",
			"cluster":   "production-frankfurt",
			"namespace": "default",
			"instance":  "node-1",
			"team":      "platform",
		},
		CommonAnnotations: map[string]string{
			"summary":       "CPU usage above threshold",
			"runbook_url":   "https://runbooks.example.com/high-cpu",
			"dashboard_url": "https://grafana.infra.emil.de/d/app-error-logs",
		},
		Alerts: []webhook.Alert{
			{Annotations: map[string]string{"description": "b description"}},
			{Annotations: map[string]string{"description": "a description"}},
		},
	}
}

func hasBlockType(blocks []slackapi.Block, t slackapi.MessageBlockType) bool {
	for _, b := range blocks {
		if b.BlockType() == t {
			return true
		}
	}
	return false
}

func TestBuildAttachmentFiring(t *testing.T) {
	att := BuildAttachment(testPayload("firing"), "https://grafana.infra.emil.de")

	if att.Color != severityColors["critical"] {
		t.Fatalf("expected critical severity color, got %q", att.Color)
	}

	header, ok := att.Blocks.BlockSet[0].(*slackapi.HeaderBlock)
	if !ok || header.Text.Text != "HighCPU" {
		t.Fatalf("expected header block with alertname title, got %#v", att.Blocks.BlockSet[0])
	}

	if !hasBlockType(att.Blocks.BlockSet, slackapi.MBTAction) {
		t.Fatalf("expected a runbook action block when runbook_url is set and firing")
	}
}

func TestBuildAttachmentResolvedHasNoActionsAndGoodColor(t *testing.T) {
	att := BuildAttachment(testPayload("resolved"), "https://grafana.infra.emil.de")

	if att.Color != resolvedColor {
		t.Fatalf("expected resolved color %q, got %q", resolvedColor, att.Color)
	}
	if hasBlockType(att.Blocks.BlockSet, slackapi.MBTAction) {
		t.Fatalf("resolved messages must not show action buttons")
	}
}

func TestBuildAttachmentTitleFallsBackToReceiver(t *testing.T) {
	payload := webhook.Payload{Receiver: "team-slack", Status: "firing"}
	att := BuildAttachment(payload, "")

	header := att.Blocks.BlockSet[0].(*slackapi.HeaderBlock)
	if header.Text.Text != "team-slack" {
		t.Fatalf("expected receiver as title fallback, got %q", header.Text.Text)
	}
}

func TestSilenceURL(t *testing.T) {
	labels := map[string]string{"alertname": "HighCPU", "cluster": "prod-eu", "namespace": "default", "severity": "critical"}

	got := silenceURL("https://grafana.infra.emil.de/", labels)
	want := "https://grafana.infra.emil.de/alerting/silence/new?alertmanager=alert-prod-eu&matcher=alertname%3DHighCPU&matcher=cluster%3Dprod-eu&matcher=namespace%3Ddefault"
	if got != want {
		t.Fatalf("silenceURL mismatch:\ngot  %s\nwant %s", got, want)
	}

	if silenceURL("", labels) != "" {
		t.Fatalf("expected empty grafanaURL to yield no silence link")
	}
	if silenceURL("https://grafana.infra.emil.de", map[string]string{}) != "" {
		t.Fatalf("expected missing cluster label to yield no silence link")
	}
}

func TestActionElementsIncludesSilenceAndDashboardButtons(t *testing.T) {
	elements := actionElements(map[string]string{"dashboard_url": "https://grafana.example.com/d/abc"}, "https://grafana.example.com/alerting/silence/new")
	if len(elements) != 2 {
		t.Fatalf("expected silence + dashboard buttons, got %d elements", len(elements))
	}
}

func TestAlertDetailsDedupAndSort(t *testing.T) {
	alerts := []webhook.Alert{
		{Annotations: map[string]string{"description": "b description"}},
		{Annotations: map[string]string{"description": "a description"}},
		{Annotations: map[string]string{"description": "a description"}},
	}
	got := alertDetails(alerts)
	if len(got) != 2 || got[0] != "a description" || got[1] != "b description" {
		t.Fatalf("expected deduped, sorted details, got %v", got)
	}
}
