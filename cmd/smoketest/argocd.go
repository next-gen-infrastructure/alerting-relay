package main

import (
	"fmt"
	"time"

	"alerting-relay/internal/slack"
	"alerting-relay/internal/webhook"
)

// runArgoCD posts a two-app sequence on one revision into one channel,
// rendering the per-revision aggregate root like the relay does.
func runArgoCD(client *slack.Client, channel string) {
	base := webhook.ArgoEvent{
		Repo: "https://github.com/example/test-repo", Cluster: "DevelopmentBackendServices", Namespace: "default",
		Revision: "0123456789abcdef",
	}
	steps := []struct{ app, event, msg string }{
		{"web", "syncing", "Sync operation started"},
		{"api", "syncing", "Sync operation started"},
		{"web", "synced", "successfully synced (all tasks run)"},
		{"api", "degraded", "Deployment api: 0/2 replicas available"},
		{"api", "healthy", "Application is healthy again"},
		{"web", "healthy", "Application is healthy"},
	}
	apps := map[string]webhook.ArgoEvent{}
	var ts string
	for i, s := range steps {
		ev := base
		ev.App, ev.Event, ev.Message = s.app, s.event, s.msg
		ev.URL = "https://argocd.example.com/applications/" + s.app
		apps[s.app] = ev
		var err error
		if i == 0 {
			ts, err = client.PostRoot(channel, slack.BuildArgoSummary(ev, apps))
		} else if err = client.UpdateRoot(channel, ts, slack.BuildArgoSummary(ev, apps)); err == nil {
			err = client.PostThreadReply(channel, ts, slack.BuildArgoAttachment(ev, true))
		}
		if err != nil {
			fmt.Println("argocd step", s.app, s.event, "error:", err)
			return
		}
		fmt.Println("posted", s.app, s.event)
		time.Sleep(4 * time.Second)
	}
}
