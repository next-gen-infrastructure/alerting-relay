package slack

import (
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
	if a.Color != severityColors["critical"] || len(a.Blocks.BlockSet) != 2 {
		t.Fatalf("unexpected attachment: %+v", a)
	}
	if got := BuildArgoAttachment(ev, false).Fallback; !strings.Contains(got, "web") {
		t.Fatalf("fallback %q", got)
	}
}
