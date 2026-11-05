package slack

import (
	slackapi "github.com/slack-go/slack"
	"strings"
	"testing"

	"alerting-relay/internal/webhook"
)

func TestBuildArgoAttachment(t *testing.T) {
	ev := webhook.ArgoEvent{App: "web", Revision: "abcdef1234", Event: "sync-failed", Message: "boom", URL: "https://argo/x"}
	if !ValidArgoEvent(ev.Event) || ValidArgoEvent("nope") {
		t.Fatal("ValidArgoEvent")
	}
	a := BuildArgoAttachment(ev, true)
	if a.Color != severityColors["critical"] || len(a.Blocks.BlockSet) != 1 || !strings.Contains(a.Blocks.BlockSet[0].(*slackapi.SectionBlock).Text.Text, "<https://argo/x|web@abcdef1>") {
		t.Fatalf("unexpected attachment: %+v", a)
	}
	if got := BuildArgoAttachment(ev, false).Fallback; !strings.Contains(got, "web") {
		t.Fatalf("fallback %q", got)
	}
}

func TestBuildArgoSummary(t *testing.T) {
	apps := map[string]webhook.ArgoEvent{
		"a": {App: "a", Event: "synced"},
		"b": {App: "b", Event: "sync-failed"},
	}
	if WorstArgoEvent(apps) != "sync-failed" {
		t.Fatal("worst")
	}
	a := BuildArgoSummary(webhook.ArgoEvent{Revision: "abcdef1234"}, apps)
	if a.Color != severityColors["critical"] || !strings.Contains(a.Fallback, "abcdef1") {
		t.Fatalf("unexpected attachment: %+v", a)
	}
}

func TestRevLink(t *testing.T) {
	for repo, want := range map[string]string{
		"https://github.com/o/r.git": "<https://github.com/o/r/commit/abcdef1234|`abcdef1`>",
		"git@github.com:o/r.git":     "<https://github.com/o/r/commit/abcdef1234|`abcdef1`>",
		"https://gitlab.com/o/r":     "`abcdef1`",
		"":                           "`abcdef1`",
	} {
		if got := revLink(repo, "abcdef1234"); got != want {
			t.Errorf("%q: got %s want %s", repo, got, want)
		}
	}
}
