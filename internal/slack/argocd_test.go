package slack

import (
	"fmt"
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
	a := BuildArgoAttachment([]webhook.ArgoEvent{ev})
	if a.Color != severityColors["critical"] || len(a.Blocks.BlockSet) != 1 || !strings.Contains(a.Blocks.BlockSet[0].(*slackapi.SectionBlock).Text.Text, "<https://argo/x|web@abcdef1>") {
		t.Fatalf("unexpected attachment: %+v", a)
	}
	if got := BuildArgoAttachment([]webhook.ArgoEvent{ev}).Fallback; !strings.Contains(got, "web") {
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

func TestArgoSummaryMaxApps(t *testing.T) {
	apps := map[string]webhook.ArgoEvent{}
	for i := 0; i < 25; i++ {
		n := fmt.Sprintf("app-%02d", i)
		apps[n] = webhook.ArgoEvent{App: n, Event: "synced"}
	}
	txt := BuildArgoSummary(apps["app-00"], apps).Blocks.BlockSet[0].(*slackapi.SectionBlock).Text.Text
	if !strings.Contains(txt, "app-09") || strings.Contains(txt, "app-10") || !strings.Contains(txt, "and 15 more") || !strings.Contains(txt, "(25 apps)") {
		t.Fatal(txt)
	}
}

func TestArgoTextCapped(t *testing.T) {
	apps := map[string]webhook.ArgoEvent{}
	var evs []webhook.ArgoEvent
	for i := 0; i < 60; i++ {
		n := fmt.Sprintf("osint-data-some-long-application-name-%02d", i)
		ev := webhook.ArgoEvent{App: n, Event: "synced", Namespace: "osint-data-production", Revision: "0123456789abcdef",
			URL: "https://argocd.example.com/applications/argocd/" + n, Message: strings.Repeat("x", 2000)}
		apps[n] = ev
		evs = append(evs, ev)
	}
	for _, a := range []slackapi.Attachment{BuildArgoSummary(evs[0], apps), BuildArgoAttachment(evs)} {
		txt := a.Blocks.BlockSet[0].(*slackapi.SectionBlock).Text.Text
		if len(txt) > 3000 || !strings.Contains(txt, "more") {
			t.Fatalf("len %d, text %q", len(txt), txt[len(txt)-40:])
		}
	}
}

func TestArgoProgressingAndSuperseded(t *testing.T) {
	if !ValidArgoEvent("progressing") || WorstArgoEvent(map[string]webhook.ArgoEvent{"a": {Event: "synced"}, "b": {Event: "progressing"}}) != "progressing" {
		t.Fatal("progressing")
	}
	one := BuildArgoSummary(webhook.ArgoEvent{}, map[string]webhook.ArgoEvent{"a": {App: "a", Event: "synced"}}).Blocks.BlockSet[0].(*slackapi.SectionBlock).Text.Text
	if !strings.Contains(one, "(1 app)") {
		t.Fatal(one)
	}
	a := BuildArgoSuperseded(webhook.ArgoEvent{App: "web", Revision: "aaaaaaaa"}, "bbbbbbbb")
	if txt := a.Blocks.BlockSet[0].(*slackapi.SectionBlock).Text.Text; !strings.Contains(txt, "bbbbbbb") || !strings.Contains(txt, "web") {
		t.Fatal(txt)
	}
}
