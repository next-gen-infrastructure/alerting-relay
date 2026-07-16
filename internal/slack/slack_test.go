package slack

import (
	"strings"
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
			{Status: status, Annotations: map[string]string{"description": "b description"}},
			{Status: status, Annotations: map[string]string{"description": "a description"}},
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
	att := BuildAttachment(testPayload("firing"), "https://grafana.infra.emil.de", nil, true)

	if att.Color != severityColors["critical"] {
		t.Fatalf("expected critical severity color, got %q", att.Color)
	}

	header, ok := att.Blocks.BlockSet[0].(*slackapi.HeaderBlock)
	if !ok || header.Text.Text != "🔴 [FIRING: 2] HighCPU (default)" {
		t.Fatalf("expected header block with severity emoji, firing count, alertname and namespace title, got %#v", att.Blocks.BlockSet[0])
	}

	if att.Fallback != header.Text.Text {
		t.Fatalf("expected Fallback to match header title, got %q", att.Fallback)
	}

	if !hasBlockType(att.Blocks.BlockSet, slackapi.MBTAction) {
		t.Fatalf("expected a runbook action block when runbook_url is set and firing")
	}
}

func TestBuildAttachmentInitialPostOmitsCounts(t *testing.T) {
	att := BuildAttachment(testPayload("firing"), "https://grafana.infra.emil.de", nil, false)

	header := att.Blocks.BlockSet[0].(*slackapi.HeaderBlock)
	if header.Text.Text != "🔴 HighCPU (default)" {
		t.Fatalf("expected initial post header with no firing/resolved count, got %q", header.Text.Text)
	}
}

func TestBuildThreadUpdateOmitsHeaderAndMetadata(t *testing.T) {
	att := BuildThreadUpdate(testPayload("firing"))

	if hasBlockType(att.Blocks.BlockSet, slackapi.MBTHeader) {
		t.Fatalf("thread update must not repeat the root's header")
	}
	if hasBlockType(att.Blocks.BlockSet, slackapi.MBTAction) {
		t.Fatalf("thread update must not repeat the root's action buttons")
	}
	if att.Fallback != "FIRING: 2" {
		t.Fatalf("expected fallback to summarize the count change, got %q", att.Fallback)
	}
}

func TestBuildAttachmentHeaderCountsMixedStatuses(t *testing.T) {
	payload := testPayload("firing")
	payload.Alerts[0].Status = "resolved"

	att := BuildAttachment(payload, "https://grafana.infra.emil.de", nil, true)

	header := att.Blocks.BlockSet[0].(*slackapi.HeaderBlock)
	if header.Text.Text != "🔴 [FIRING: 1, RESOLVED: 1] HighCPU (default)" {
		t.Fatalf("expected mixed firing/resolved counts in header, got %q", header.Text.Text)
	}
}

func TestBuildAttachmentTitleIncludesPod(t *testing.T) {
	payload := testPayload("firing")
	payload.CommonLabels["pod"] = "web-1"

	att := BuildAttachment(payload, "https://grafana.infra.emil.de", nil, true)

	header := att.Blocks.BlockSet[0].(*slackapi.HeaderBlock)
	if header.Text.Text != "🔴 [FIRING: 2] HighCPU (default/web-1)" {
		t.Fatalf("expected namespace/pod in header, got %q", header.Text.Text)
	}
}

func TestBuildAttachmentResolvedKeepsActionsAndGoodColor(t *testing.T) {
	att := BuildAttachment(testPayload("resolved"), "https://grafana.infra.emil.de", nil, true)

	if att.Color != resolvedColor {
		t.Fatalf("expected resolved color %q, got %q", resolvedColor, att.Color)
	}
	if !hasBlockType(att.Blocks.BlockSet, slackapi.MBTAction) {
		t.Fatalf("root message must keep its action buttons after resolution")
	}

	header := att.Blocks.BlockSet[0].(*slackapi.HeaderBlock)
	if !strings.HasPrefix(header.Text.Text, "✅ ") {
		t.Fatalf("expected resolved header to use the resolved checkmark, got %q", header.Text.Text)
	}

	fields := metadataFields(testPayload("resolved").CommonLabels, false, nil)
	for _, f := range fields {
		if strings.HasPrefix(f.Text, "*Severity*") {
			t.Fatalf("resolved messages must not show a severity field, got %#v", fields)
		}
	}
}

func TestMetadataFieldsDefaultsTeamToOncall(t *testing.T) {
	fields := metadataFields(map[string]string{}, true, nil)
	if len(fields) != 1 || fields[0].Text != "*Team*\n@team-devops-oncall" {
		t.Fatalf("expected default oncall team field, got %#v", fields)
	}

	fields = metadataFields(map[string]string{"team": "platform"}, true, nil)
	if len(fields) != 1 || fields[0].Text != "*Team*\n@platform" {
		t.Fatalf("expected team label from labels, got %#v", fields)
	}
}

func TestMetadataFieldsUsesRealMentionWhenTeamMapped(t *testing.T) {
	teamMentions := map[string]string{"platform": "S0123ABC"}

	fields := metadataFields(map[string]string{"team": "platform"}, true, teamMentions)
	if len(fields) != 1 || fields[0].Text != "*Team*\n<!subteam^S0123ABC>" {
		t.Fatalf("expected real subteam mention for mapped team, got %#v", fields)
	}

	fields = metadataFields(map[string]string{"team": "insurance"}, true, teamMentions)
	if len(fields) != 1 || fields[0].Text != "*Team*\n@insurance" {
		t.Fatalf("expected plain text fallback for unmapped team, got %#v", fields)
	}
}

func TestBuildAttachmentTitleFallsBackToReceiver(t *testing.T) {
	payload := webhook.Payload{Receiver: "team-slack", Status: "firing"}
	att := BuildAttachment(payload, "", nil, false)

	header := att.Blocks.BlockSet[0].(*slackapi.HeaderBlock)
	if header.Text.Text != "🔵 team-slack" {
		t.Fatalf("expected receiver as title fallback with no count prefix, got %q", header.Text.Text)
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
	want := "a description\nb description"
	if got != want {
		t.Fatalf("expected deduped, sorted details %q, got %q", want, got)
	}
}

func TestAlertDetailsGroupsFiringAndResolvedUnderSubheadings(t *testing.T) {
	alerts := []webhook.Alert{
		{Status: "resolved", Annotations: map[string]string{"description": "b resolved"}},
		{Status: "firing", Annotations: map[string]string{"description": "z firing"}},
		{Status: "firing", Annotations: map[string]string{"description": "a firing"}},
		{Status: "resolved", Annotations: map[string]string{"description": "b resolved"}},
	}
	got := alertDetails(alerts)
	want := "*Firing (2)*\na firing\nz firing\n\n*Resolved (1)*\nb resolved"
	if got != want {
		t.Fatalf("expected:\n%s\ngot:\n%s", want, got)
	}
}
