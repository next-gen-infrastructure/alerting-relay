package main

import (
	"fmt"
	"time"

	"alerting-relay/internal/slack"
	"alerting-relay/internal/webhook"
)

// runArgoCD posts a degraded -> healthy sequence ("test" app) into one channel.
func runArgoCD(client *slack.Client, channel string) {
	ev := webhook.ArgoEvent{
		App: "test", Repo: "https://github.com/example/test-repo", Cluster: "DevelopmentBackendServices", Namespace: "default",
		Revision: "0123456789abcdef", URL: "https://argocd.example.com/applications/test",
	}
	steps := []struct{ event, msg string }{
		{"syncing", "Sync operation started"},
		{"synced", "successfully synced (all tasks run)"},
		{"degraded", "Deployment test: 0/2 replicas available"},
		{"healthy", "Application is healthy again"},
	}
	var ts string
	for i, s := range steps {
		ev.Event, ev.Message = s.event, s.msg
		var err error
		if i == 0 {
			ts, err = client.PostRoot(channel, slack.BuildArgoAttachment(ev, true))
		} else if err = client.UpdateRoot(channel, ts, slack.BuildArgoAttachment(ev, false)); err == nil {
			err = client.PostThreadReply(channel, ts, slack.BuildArgoAttachment(ev, true))
		}
		if err != nil {
			fmt.Println("argocd step", s.event, "error:", err)
			return
		}
		fmt.Println("posted", s.event)
		time.Sleep(5 * time.Second)
	}
}
