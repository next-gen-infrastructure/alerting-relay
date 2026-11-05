package slack

import (
	"fmt"

	slackapi "github.com/slack-go/slack"

	"alerting-relay/internal/webhook"
)

var argoStates = map[string]struct{ emoji, title, color string }{
	"syncing":     {":arrows_counterclockwise:", "Syncing", defaultColor},
	"synced":      {":white_check_mark:", "Synced", resolvedColor},
	"sync-failed": {":x:", "Sync failed", severityColors["critical"]},
	"degraded":    {":rotating_light:", "Degraded", severityColors["critical"]},
	"healthy":     {":white_check_mark:", "Healthy", resolvedColor},
}

// ValidArgoEvent reports whether e is a known Argo CD event name.
func ValidArgoEvent(e string) bool { _, ok := argoStates[e]; return ok }

func shortRev(r string) string {
	if len(r) > 7 {
		return r[:7]
	}
	return r
}

// BuildArgoAttachment renders an Argo CD event: the root message shows the
// latest state; the same shape (with the event message) serves as the thread reply.
func BuildArgoAttachment(ev webhook.ArgoEvent, withMessage bool) slackapi.Attachment {
	st := argoStates[ev.Event]
	head := fmt.Sprintf("%s *%s* — `%s` @ `%s`", st.emoji, st.title, ev.App, shortRev(ev.Revision))
	if withMessage && ev.Message != "" {
		head += "\n" + ev.Message
	}
	blocks := []slackapi.Block{slackapi.NewSectionBlock(
		slackapi.NewTextBlockObject(slackapi.MarkdownType, head, false, false), nil, nil)}
	var fields []*slackapi.TextBlockObject
	for _, f := range [][2]string{{"Namespace", ev.Namespace}, {"Repository", ev.Repo}} {
		if f[1] != "" {
			fields = append(fields, slackapi.NewTextBlockObject(slackapi.MarkdownType, "*"+f[0]+":* "+f[1], false, false))
		}
	}
	if len(fields) > 0 {
		blocks = append(blocks, slackapi.NewSectionBlock(nil, fields, nil))
	}
	if ev.URL != "" {
		blocks = append(blocks, slackapi.NewActionBlock("", actionButton("argocd", "Open in Argo CD", ev.URL)))
	}
	return slackapi.Attachment{
		Color:    st.color,
		Fallback: fmt.Sprintf("%s: %s", st.title, ev.App),
		Blocks:   slackapi.Blocks{BlockSet: blocks},
	}
}
