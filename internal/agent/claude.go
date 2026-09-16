package agent

import (
	"regexp"
	"strings"
	"time"

	"github.com/paraizofelipe/ag-mux/internal/tmux"
)

// Claude reads Claude Code panes.
//
// Claude Code puts its version in the pane's foreground command ("2.1.269")
// and the current task in the pane title, prefixed with ✳.
type Claude struct{}

const claudeMark = "✳"

var (
	// The foreground process is the claude binary, which reports itself as a
	// bare version number.
	claudeVersionRe = regexp.MustCompile(`^\d+\.\d+\.\d+`)

	// The footer block is the chrome: it is redrawn on every frame and only
	// exists while Claude Code is running.
	claudeChromeRe = regexp.MustCompile(`Context\s+\S*\s*\d+%|shift\+tab to cycle`)

	// The spinner line, present only while working:
	//   ✢ Wandering… (8m 3s · ↓ 28.4k tokens)
	// The ellipsis and the counters in parentheses are what separate it from
	// the past-tense line Claude leaves behind when it finishes:
	//   ✻ Cooked for 1m 27s · done 3:18 PM
	claudeBusyRe = regexp.MustCompile(`^\s*\S\s+\S+…\s*\(([^)]*)\)`)

	// The input box. Text after the chevron is a prompt you typed and never
	// sent, which means the agent is parked waiting on you.
	claudePromptRe = regexp.MustCompile(`^\s*❯\s?(.*)$`)

	// Best effort, not yet seen in a real capture: the permission dialog.
	claudeAskRe = regexp.MustCompile(`Do you want|Would you like|Proceed\?`)

	claudeDoneAtRe = regexp.MustCompile(`done\s+(\d{1,2}:\d{2}\s*[AP]M)`)
)

// chromeDepth is how many trailing lines the footer block occupies. Confirm
// looks no deeper, so an exited session's scrollback cannot pass for a live one.
const claudeChromeDepth = 5

func (Claude) Name() string { return "claude" }

func (Claude) IsCandidate(p tmux.Pane) bool {
	return strings.HasPrefix(p.Title, claudeMark) || claudeVersionRe.MatchString(p.Command)
}

func (Claude) Confirm(tail []string) bool {
	return anyMatch(claudeChromeRe, tailLines(tail, claudeChromeDepth))
}

func (Claude) Classify(p tmux.Pane, tail []string) (State, string, time.Duration) {
	recent := tailLines(tail, 15)

	for _, l := range recent {
		if m := claudeBusyRe.FindStringSubmatch(l); m != nil {
			d := parseElapsed(m[1])
			return StateBusy, strings.TrimSpace(m[1]), d
		}
	}
	if anyMatch(claudeAskRe, recent) {
		return StateWaiting, "confirmação pendente", 0
	}
	// Last chevron wins: earlier ones are tool output scrolled past.
	for i := len(recent) - 1; i >= 0; i-- {
		m := claudePromptRe.FindStringSubmatch(recent[i])
		if m == nil {
			continue
		}
		if draft := strings.TrimSpace(m[1]); draft != "" {
			return StateWaiting, "rascunho pendente: " + draft, 0
		}
		break
	}
	if anyMatch(claudeChromeRe, tailLines(tail, claudeChromeDepth)) {
		detail := ""
		for _, l := range recent {
			if m := claudeDoneAtRe.FindStringSubmatch(l); m != nil {
				detail = "desde " + strings.TrimSpace(m[1])
				break
			}
		}
		return StateIdle, detail, 0
	}
	return StateUnknown, "", 0
}

func (Claude) Task(p tmux.Pane) string {
	return strings.TrimSpace(strings.TrimPrefix(p.Title, claudeMark))
}
