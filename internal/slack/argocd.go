package slack

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

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

// argoRank orders events by severity for per-revision aggregation.
var argoRank = map[string]int{"healthy": 0, "synced": 1, "syncing": 2, "degraded": 3, "sync-failed": 4}

// WorstArgoEvent returns the most severe event across apps ("" if empty).
func WorstArgoEvent(apps map[string]webhook.ArgoEvent) string {
	worst := ""
	for _, ev := range apps {
		if worst == "" || argoRank[ev.Event] > argoRank[worst] {
			worst = ev.Event
		}
	}
	return worst
}

const maxArgoAppLines = 30 // keeps the section under Slack's 3000-char text limit

// BuildArgoSummary renders the per-revision root message: the worst status as
// header colour/title, one line per app with its latest status. ev is the
// latest event (source of the revision and repo).
func BuildArgoSummary(ev webhook.ArgoEvent, apps map[string]webhook.ArgoEvent) slackapi.Attachment {
	st := argoStates[WorstArgoEvent(apps)]
	names := make([]string, 0, len(apps))
	for n := range apps {
		names = append(names, n)
	}
	sort.Strings(names)
	var lines []string
	for i, n := range names {
		if i == maxArgoAppLines {
			lines = append(lines, fmt.Sprintf("… and %d more", len(names)-i))
			break
		}
		a := apps[n]
		name := "`" + n + "`"
		if a.URL != "" {
			name = fmt.Sprintf("<%s|%s>", a.URL, n)
		}
		line := fmt.Sprintf("%s %s — %s", argoStates[a.Event].emoji, name, argoStates[a.Event].title)
		if a.Namespace != "" {
			line += " (" + a.Namespace + ")"
		}
		lines = append(lines, line)
	}
	head := fmt.Sprintf("%s *%s* — %s (%d apps)\n%s", st.emoji, st.title, revLink(ev.Repo, ev.Revision), len(apps), strings.Join(lines, "\n"))
	blocks := []slackapi.Block{slackapi.NewSectionBlock(
		slackapi.NewTextBlockObject(slackapi.MarkdownType, head, false, false), nil, nil)}
	if ev.Repo != "" {
		blocks = append(blocks, slackapi.NewContextBlock("", slackapi.NewTextBlockObject(slackapi.MarkdownType, "*Repository:* "+ev.Repo, false, false)))
	}
	return slackapi.Attachment{
		Color:    st.color,
		Fallback: strings.TrimSpace(fmt.Sprintf("%s: %s @ %s", st.title, ev.Cluster, shortRev(ev.Revision))),
		Blocks:   slackapi.Blocks{BlockSet: blocks},
	}
}

var githubRepo = regexp.MustCompile(`^(?:https://github\.com/|git@github\.com:|ssh://git@github\.com/)([^/]+/[^/]+?)(?:\.git)?/?$`)

// revLink renders the short revision, linked to its GitHub commit when repo is a GitHub URL.
func revLink(repo, rev string) string {
	if m := githubRepo.FindStringSubmatch(repo); m != nil && rev != "" {
		return fmt.Sprintf("<https://github.com/%s/commit/%s|`%s`>", m[1], rev, shortRev(rev))
	}
	return "`" + shortRev(rev) + "`"
}

func shortRev(r string) string {
	if len(r) > 7 {
		return r[:7]
	}
	return r
}

// BuildArgoAttachment renders a single Argo CD event as a compact thread reply;
// namespace and repo live in the root summary (BuildArgoSummary).
func BuildArgoAttachment(ev webhook.ArgoEvent, withMessage bool) slackapi.Attachment {
	st := argoStates[ev.Event]
	ref := ev.App + "@" + shortRev(ev.Revision)
	if ev.URL != "" {
		ref = fmt.Sprintf("<%s|%s>", ev.URL, ref)
	}
	head := fmt.Sprintf("%s *%s* — %s", st.emoji, st.title, ref)
	if withMessage && ev.Message != "" {
		head += "\n" + ev.Message
	}
	blocks := []slackapi.Block{slackapi.NewSectionBlock(
		slackapi.NewTextBlockObject(slackapi.MarkdownType, head, false, false), nil, nil)}
	return slackapi.Attachment{
		Color:    st.color,
		Fallback: fmt.Sprintf("%s: %s", st.title, ev.App),
		Blocks:   slackapi.Blocks{BlockSet: blocks},
	}
}
