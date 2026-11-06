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

// Slack rejects a section whose text exceeds 3000 chars with invalid_attachments.
// Byte length over-counts chars, which only makes the cap safer.
const maxSectionText = 2900

const maxArgoApps = 10 // apps listed in the root summary; the rest are counted

// joinCapped joins lines, always keeping the first, and replaces whatever
// doesn't fit under maxSectionText with "… and N more".
func joinCapped(lines []string) string {
	text := lines[0]
	for i := 1; i < len(lines); i++ {
		if len(text)+1+len(lines[i]) > maxSectionText-20 {
			return text + fmt.Sprintf("\n… and %d more", len(lines)-i)
		}
		text += "\n" + lines[i]
	}
	return text
}

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
	lines := []string{fmt.Sprintf("%s *%s* — %s (%d apps)", st.emoji, st.title, revLink(ev.Repo, ev.Revision), len(apps))}
	for i, n := range names {
		if i == maxArgoApps {
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
	head := joinCapped(lines)
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

const maxArgoMessage = 500 // chars of an event message shown in a thread reply

// BuildArgoAttachment renders the events accumulated for a group since the last
// flush as one compact thread reply (colour = worst event); namespace and repo
// live in the root summary (BuildArgoSummary).
func BuildArgoAttachment(evs []webhook.ArgoEvent) slackapi.Attachment {
	worst := evs[0]
	lines := make([]string, 0, len(evs))
	for _, ev := range evs {
		if argoRank[ev.Event] > argoRank[worst.Event] {
			worst = ev
		}
		st := argoStates[ev.Event]
		ref := ev.App + "@" + shortRev(ev.Revision)
		if ev.URL != "" {
			ref = fmt.Sprintf("<%s|%s>", ev.URL, ref)
		}
		line := fmt.Sprintf("%s *%s* — %s", st.emoji, st.title, ref)
		if m := []rune(ev.Message); len(m) > maxArgoMessage {
			ev.Message = string(m[:maxArgoMessage]) + "…"
		}
		if ev.Message != "" {
			line += "\n" + ev.Message
		}
		lines = append(lines, line)
	}
	st := argoStates[worst.Event]
	blocks := []slackapi.Block{slackapi.NewSectionBlock(
		slackapi.NewTextBlockObject(slackapi.MarkdownType, joinCapped(lines), false, false), nil, nil)}
	return slackapi.Attachment{
		Color:    st.color,
		Fallback: fmt.Sprintf("%s: %s", st.title, worst.App),
		Blocks:   slackapi.Blocks{BlockSet: blocks},
	}
}
