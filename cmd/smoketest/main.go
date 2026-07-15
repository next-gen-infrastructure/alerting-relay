package main

import (
	"fmt"
	"os"
	"time"

	"alerting-relay/internal/slack"
	"alerting-relay/internal/webhook"
)

func main() {
	token := os.Getenv("SLACK_BOT_TOKEN")
	channel := os.Getenv("SLACK_CHANNEL")
	if token == "" || channel == "" {
		fmt.Println("set SLACK_BOT_TOKEN and SLACK_CHANNEL")
		os.Exit(1)
	}
	client := slack.New(token)

	base := webhook.Payload{
		Receiver: "development-backend-services",
		CommonLabels: map[string]string{
			"alertname": "AlertColorCodeSmokeTest",
			"severity":  "warning",
			"cluster":   "DevelopmentBackendServices",
			"namespace": "default",
		},
		CommonAnnotations: map[string]string{
			"summary": "Testing per-instance firing/resolved color coding",
		},
	}

	// 1 instance down
	step1 := base
	step1.Status = "firing"
	step1.Alerts = []webhook.Alert{
		{Status: "firing", Annotations: map[string]string{"description": "instance-1 down"}},
	}
	ts, err := client.PostRoot(channel, slack.BuildAttachment(step1, ""))
	if err != nil {
		fmt.Println("post root error:", err)
		return
	}
	fmt.Println("posted root ts:", ts)
	time.Sleep(5 * time.Second)

	// 2 instances down
	step2 := base
	step2.Status = "firing"
	step2.Alerts = []webhook.Alert{
		{Status: "firing", Annotations: map[string]string{"description": "instance-1 down"}},
		{Status: "firing", Annotations: map[string]string{"description": "instance-2 down"}},
	}
	if err := client.PostThreadReply(channel, ts, slack.BuildAttachment(step2, "")); err != nil {
		fmt.Println("thread reply error:", err)
		return
	}
	fmt.Println("posted thread reply: 2 firing")
	time.Sleep(5 * time.Second)

	// first recovered, second still down
	step3 := base
	step3.Status = "firing"
	step3.Alerts = []webhook.Alert{
		{Status: "resolved", Annotations: map[string]string{"description": "instance-1 down"}},
		{Status: "firing", Annotations: map[string]string{"description": "instance-2 down"}},
	}
	if err := client.PostThreadReply(channel, ts, slack.BuildAttachment(step3, "")); err != nil {
		fmt.Println("thread reply error:", err)
		return
	}
	fmt.Println("posted thread reply: 1 firing, 1 resolved")
	time.Sleep(5 * time.Second)

	// all instances recovered
	step4 := base
	step4.Status = "resolved"
	step4.Alerts = []webhook.Alert{
		{Status: "resolved", Annotations: map[string]string{"description": "instance-1 down"}},
		{Status: "resolved", Annotations: map[string]string{"description": "instance-2 down"}},
	}
	attachment := slack.BuildAttachment(step4, "")
	if err := client.UpdateRoot(channel, ts, attachment); err != nil {
		fmt.Println("update root error:", err)
		return
	}
	if err := client.PostThreadReply(channel, ts, attachment); err != nil {
		fmt.Println("resolved thread reply error:", err)
		return
	}
	fmt.Println("root updated + resolved thread reply posted: all resolved")
}
