// Package slack renders Alertmanager webhook payloads as Slack Block Kit
// attachments and posts/updates them via the Slack API.
package slack

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	slackapi "github.com/slack-go/slack"

	"alerting-relay/internal/webhook"
)

var severityColors = map[string]string{
	"critical": "#D32F2F",
	"high":     "#F57C00",
	"warning":  "#FBC02D",
	"info":     "#1976D2",
}

const (
	defaultColor  = "#1976D2"
	resolvedColor = "#2eb67d"
)

var prodPrefix = regexp.MustCompile(`(?i)^production`)

type Client struct {
	api *slackapi.Client
}

func New(token string) *Client {
	return &Client{api: slackapi.New(token)}
}

// PostRoot posts the first message for a newly-seen alert group.
func (c *Client) PostRoot(channel string, attachment slackapi.Attachment) (ts string, err error) {
	_, ts, err = c.api.PostMessage(channel, slackapi.MsgOptionAttachments(attachment))
	return ts, err
}

// PostThreadReply posts a follow-up (update or resolution note) into the group's thread.
func (c *Client) PostThreadReply(channel, threadTS string, attachment slackapi.Attachment) error {
	_, _, err := c.api.PostMessage(channel, slackapi.MsgOptionAttachments(attachment), slackapi.MsgOptionTS(threadTS))
	return err
}

// UpdateRoot edits the root message in place, e.g. to mark it resolved.
func (c *Client) UpdateRoot(channel, ts string, attachment slackapi.Attachment) error {
	_, _, _, err := c.api.UpdateMessage(channel, ts, slackapi.MsgOptionAttachments(attachment))
	return err
}

// BuildAttachment renders one Alertmanager webhook payload into a Slack
// attachment: header, a metadata grid, summary, deduped alert details, and
// (firing-only) a runbook button. Callers just send the alert's default
// labels/annotations — this is the one place formatting decisions live, so
// the root post, thread updates, and the resolved edit all render the same way.
func BuildAttachment(payload webhook.Payload, grafanaURL string) slackapi.Attachment {
	firing := payload.Status == "firing"
	labels := payload.CommonLabels
	ann := payload.CommonAnnotations

	color := resolvedColor
	if firing {
		color = defaultColor
		if c, ok := severityColors[labels["severity"]]; ok {
			color = c
		}
	}

	title := labels["alertname"]
	if title == "" {
		title = payload.Receiver
	}
	if len(title) > 150 {
		title = title[:150]
	}

	blocks := []slackapi.Block{
		slackapi.NewHeaderBlock(slackapi.NewTextBlockObject(slackapi.PlainTextType, title, true, false)),
	}

	if fields := metadataFields(labels); len(fields) > 0 {
		blocks = append(blocks, slackapi.NewSectionBlock(nil, fields, nil))
	}

	if summary := ann["summary"]; summary != "" {
		blocks = append(blocks, slackapi.NewSectionBlock(
			slackapi.NewTextBlockObject(slackapi.MarkdownType, "*Summary*\n"+summary, false, false), nil, nil,
		))
	}

	if details := alertDetails(payload.Alerts); len(details) > 0 {
		blocks = append(blocks, slackapi.NewSectionBlock(
			slackapi.NewTextBlockObject(slackapi.MarkdownType, strings.Join(details, "\n"), false, false), nil, nil,
		))
	}

	if firing {
		silence := silenceURL(grafanaURL, labels)
		if elements := actionElements(ann, silence); len(elements) > 0 {
			blocks = append(blocks, slackapi.NewDividerBlock())
			blocks = append(blocks, slackapi.NewActionBlock("", elements...))
		}
	}

	return slackapi.Attachment{Color: color, Blocks: slackapi.Blocks{BlockSet: blocks}}
}

// metadataFields renders the two-column grid (severity/cluster/namespace/instance/team),
// same shape as the amazon-prometheus Lambda's buildBlockKit.
func metadataFields(labels map[string]string) []*slackapi.TextBlockObject {
	var fields []*slackapi.TextBlockObject
	add := func(label, value string) {
		if value == "" {
			return
		}
		fields = append(fields, slackapi.NewTextBlockObject(slackapi.MarkdownType, fmt.Sprintf("*%s*\n%s", label, value), false, false))
	}

	add("Severity", capitalize(labels["severity"]))
	if cluster := labels["cluster"]; cluster != "" {
		if prodPrefix.MatchString(cluster) {
			add("Cluster", ":red_circle: "+cluster)
		} else {
			add("Cluster", cluster)
		}
	}
	add("Namespace", labels["namespace"])
	add("Instance", labels["instance"])
	if team := labels["team"]; team != "" {
		add("Team", "@"+team)
	}
	return fields
}

// alertDetails collects each alert's description/message, de-duplicated and
// sorted so re-renders (thread replies) are stable/diffable.
func alertDetails(alerts []webhook.Alert) []string {
	seen := map[string]bool{}
	var details []string
	for _, a := range alerts {
		for _, key := range []string{"description", "message"} {
			if v := a.Annotations[key]; v != "" && !seen[v] {
				seen[v] = true
				details = append(details, v)
			}
		}
	}
	sort.Strings(details)
	return details
}

// actionElements builds the runbook/silence/dashboard buttons. runbook_url and
// dashboard_url come from annotations (rule authors set them per alert, same
// as runbook_url always has); silence is computed by the caller via
// silenceURL since it's fully derivable from the alert's own labels.
func actionElements(ann map[string]string, silence string) []slackapi.BlockElement {
	var elements []slackapi.BlockElement
	if url := ann["runbook_url"]; url != "" {
		elements = append(elements, actionButton("runbook", ":green_book: Runbook", url))
	}
	if silence != "" {
		elements = append(elements, actionButton("silence", ":no_bell: Silence", silence))
	}
	if url := ann["dashboard_url"]; url != "" {
		elements = append(elements, actionButton("dashboard", ":bar_chart: Dashboard", url))
	}
	return elements
}

func actionButton(actionID, label, url string) slackapi.BlockElement {
	return slackapi.NewButtonBlockElement(actionID, actionID,
		slackapi.NewTextBlockObject(slackapi.PlainTextType, label, true, false)).WithURL(url)
}

// silenceURL builds a Grafana silence-creation deep link for an alert's
// cluster/alertname/namespace labels, matching the alert group's own
// group_by (cluster, severity, namespace) minus severity, so one silence
// covers the whole notified group across severity bumps.
func silenceURL(grafanaURL string, labels map[string]string) string {
	cluster := labels["cluster"]
	if grafanaURL == "" || cluster == "" {
		return ""
	}
	q := url.Values{"alertmanager": {"alert-" + cluster}}
	for _, key := range []string{"alertname", "cluster", "namespace"} {
		if v := labels[key]; v != "" {
			q.Add("matcher", key+"="+v)
		}
	}
	return strings.TrimRight(grafanaURL, "/") + "/alerting/silence/new?" + q.Encode()
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
