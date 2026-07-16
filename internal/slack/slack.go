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

var severityEmoji = map[string]string{
	"critical": "\U0001F534", // red circle
	"high":     "\U0001F7E0", // orange circle
	"warning":  "\U0001F7E1", // yellow circle
	"info":     "\U0001F535", // blue circle
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

// BuildAttachment renders one Alertmanager webhook payload into the full
// root-message Slack attachment: header, a metadata grid, summary, deduped
// alert details, and the Runbook/Silence/Dashboard action buttons — kept on
// the root even after resolution, since Runbook/Dashboard stay relevant and
// the root is a single message that's continuously edited, not replaced.
// This is the one place formatting decisions for the root message live.
// includeCounts adds the current firing/resolved tally to the header —
// false for the initial post (nothing has changed yet), true for every
// later edit, so the header visibly tracks state as the group evolves.
func BuildAttachment(payload webhook.Payload, grafanaURL string, includeCounts bool) slackapi.Attachment {
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
	if scope := scopeSuffix(labels); scope != "" {
		title = fmt.Sprintf("%s (%s)", title, scope)
	}
	if includeCounts {
		numFiring, numResolved := alertCounts(payload.Alerts)
		var parts []string
		if numFiring > 0 {
			parts = append(parts, fmt.Sprintf("FIRING: %d", numFiring))
		}
		if numResolved > 0 {
			parts = append(parts, fmt.Sprintf("RESOLVED: %d", numResolved))
		}
		if len(parts) > 0 {
			title = fmt.Sprintf("[%s] %s", strings.Join(parts, ", "), title)
		}
	}

	emoji := "✅" // resolved: white heavy check mark
	if firing {
		emoji = severityEmoji[labels["severity"]]
		if emoji == "" {
			emoji = severityEmoji["info"]
		}
	}
	title = emoji + " " + title

	if len(title) > 150 {
		title = string([]rune(title)[:150])
	}

	blocks := []slackapi.Block{
		slackapi.NewHeaderBlock(slackapi.NewTextBlockObject(slackapi.PlainTextType, title, true, false)),
	}

	if fields := metadataFields(labels, firing); len(fields) > 0 {
		blocks = append(blocks, slackapi.NewSectionBlock(nil, fields, nil))
	}

	if summary := ann["summary"]; summary != "" {
		blocks = append(blocks, slackapi.NewSectionBlock(
			slackapi.NewTextBlockObject(slackapi.MarkdownType, "*Summary*\n"+summary, false, false), nil, nil,
		))
	}

	silence := silenceURL(grafanaURL, labels)
	if elements := actionElements(ann, silence); len(elements) > 0 {
		blocks = append(blocks, slackapi.NewDividerBlock())
		blocks = append(blocks, slackapi.NewActionBlock("", elements...))
	}

	// Instance details go last: Slack auto-collapses long attachments behind
	// a "Show more" link, so pushing the (often long) per-instance list to
	// the bottom keeps the header/summary/actions visible without expanding.
	if details := alertDetails(payload.Alerts); details != "" {
		blocks = append(blocks, slackapi.NewSectionBlock(
			slackapi.NewTextBlockObject(slackapi.MarkdownType, details, false, false), nil, nil,
		))
	}

	return slackapi.Attachment{Color: color, Fallback: title, Blocks: slackapi.Blocks{BlockSet: blocks}}
}

// BuildThreadUpdate renders a lightweight thread reply for a change to an
// already-posted group: just the current firing/resolved instance details.
// The header/metadata/summary/actions already live on the root message
// (kept current via BuildAttachment), so this doesn't repeat them.
func BuildThreadUpdate(payload webhook.Payload) slackapi.Attachment {
	firing := payload.Status == "firing"
	color := resolvedColor
	if firing {
		color = defaultColor
		if c, ok := severityColors[payload.CommonLabels["severity"]]; ok {
			color = c
		}
	}

	numFiring, numResolved := alertCounts(payload.Alerts)
	var parts []string
	if numFiring > 0 {
		parts = append(parts, fmt.Sprintf("FIRING: %d", numFiring))
	}
	if numResolved > 0 {
		parts = append(parts, fmt.Sprintf("RESOLVED: %d", numResolved))
	}

	var blocks []slackapi.Block
	if details := alertDetails(payload.Alerts); details != "" {
		blocks = append(blocks, slackapi.NewSectionBlock(
			slackapi.NewTextBlockObject(slackapi.MarkdownType, details, false, false), nil, nil,
		))
	}

	return slackapi.Attachment{Color: color, Fallback: strings.Join(parts, ", "), Blocks: slackapi.Blocks{BlockSet: blocks}}
}

// metadataFields renders the two-column grid (severity/cluster/namespace/instance/team),
// same shape as the amazon-prometheus Lambda's buildBlockKit.
func metadataFields(labels map[string]string, firing bool) []*slackapi.TextBlockObject {
	var fields []*slackapi.TextBlockObject
	add := func(label, value string) {
		if value == "" {
			return
		}
		fields = append(fields, slackapi.NewTextBlockObject(slackapi.MarkdownType, fmt.Sprintf("*%s*\n%s", label, value), false, false))
	}

	if firing {
		add("Severity", capitalize(labels["severity"]))
	}
	if cluster := labels["cluster"]; cluster != "" {
		if prodPrefix.MatchString(cluster) {
			add("Cluster", ":red_circle: "+cluster)
		} else {
			add("Cluster", cluster)
		}
	}
	add("Namespace", labels["namespace"])
	add("Instance", labels["instance"])
	team := labels["team"]
	if team == "" {
		team = "team-devops-oncall"
	}
	add("Team", "@"+team)
	return fields
}

// scopeSuffix renders the namespace/pod portion of the title, e.g. "default/web-1",
// falling back to whichever of the two is present.
func scopeSuffix(labels map[string]string) string {
	ns, pod := labels["namespace"], labels["pod"]
	switch {
	case ns != "" && pod != "":
		return ns + "/" + pod
	case ns != "":
		return ns
	case pod != "":
		return pod
	default:
		return ""
	}
}

// alertCounts tallies firing/resolved alerts within the group for the header.
func alertCounts(alerts []webhook.Alert) (firing, resolved int) {
	for _, a := range alerts {
		switch a.Status {
		case "firing":
			firing++
		case "resolved":
			resolved++
		}
	}
	return firing, resolved
}

// alertDetails collects each alert's description/message, de-duplicated per
// status and sorted so re-renders (thread replies) are stable/diffable.
// Firing and resolved instances are grouped under their own bold subheading
// (Slack Block Kit text has no per-line color) instead of an inline marker,
// firing first since those need attention; entries with no status (e.g.
// incomplete test data) are listed under neither, ungrouped.
func alertDetails(alerts []webhook.Alert) string {
	firing := dedupAnnotations(alerts, "firing")
	resolved := dedupAnnotations(alerts, "resolved")
	other := dedupAnnotations(alerts, "")

	var sections []string
	if len(firing) > 0 {
		sections = append(sections, fmt.Sprintf("*Firing (%d)*\n%s", len(firing), strings.Join(firing, "\n")))
	}
	if len(resolved) > 0 {
		sections = append(sections, fmt.Sprintf("*Resolved (%d)*\n%s", len(resolved), strings.Join(resolved, "\n")))
	}
	if len(other) > 0 {
		sections = append(sections, strings.Join(other, "\n"))
	}
	return strings.Join(sections, "\n\n")
}

// dedupAnnotations collects description/message annotations from alerts
// matching status, de-duplicated and sorted.
func dedupAnnotations(alerts []webhook.Alert, status string) []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range alerts {
		if a.Status != status {
			continue
		}
		for _, key := range []string{"description", "message"} {
			if v := a.Annotations[key]; v != "" && !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
	}
	sort.Strings(out)
	return out
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
