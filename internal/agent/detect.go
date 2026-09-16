package agent

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/paraizofelipe/ag-mux/internal/git"
	"github.com/paraizofelipe/ag-mux/internal/tmux"
)

// gitLookup is a variable so tests can run without touching the filesystem.
var gitLookup = git.Lookup

// CaptureLines is how much scrollback an adapter gets. Harness chrome lives at
// the bottom of the screen, so this only has to reach past one long block of
// tool output.
const CaptureLines = 45

// Capturer returns the last n lines of a pane's screen. tmux.CapturePane
// satisfies it; tests pass fixtures instead.
type Capturer func(target string, n int) ([]string, error)

// shells are foreground processes that mean no agent is running in the pane,
// whatever its title still claims.
var shells = map[string]bool{
	"zsh": true, "bash": true, "sh": true, "fish": true, "tcsh": true,
	"ksh": true, "dash": true, "-zsh": true, "-bash": true, "login": true,
}

// Detect turns panes into the agents actually running in them.
func Detect(panes []tmux.Pane, capture Capturer) []Agent {
	var agents []Agent
	for _, p := range panes {
		if a, ok := detectOne(p, capture); ok {
			agents = append(agents, a)
		}
	}
	return agents
}

func detectOne(p tmux.Pane, capture Capturer) (Agent, bool) {
	if p.IsSidebar {
		return Agent{}, false
	}
	for _, ad := range Adapters() {
		if !ad.IsCandidate(p) {
			continue
		}
		// A harness that exits leaves its title behind, so the title alone is
		// a false positive. Two independent checks catch it: the foreground
		// process is back to the shell, and the chrome is gone from the
		// bottom of the screen.
		if shells[p.Command] {
			return Agent{}, false
		}
		tail, err := capture(p.ID, CaptureLines)
		if err != nil || !ad.Confirm(tail) {
			return Agent{}, false
		}
		state, detail, elapsed := ad.Classify(p, tail)
		a := Agent{
			Pane:    p,
			Harness: ad.Name(),
			Label:   label(p),
			Task:    ad.Task(p),
			State:   state,
			Detail:  detail,
			Elapsed: elapsed,
		}
		a.Branch, a.Dirty, _ = ad.Branch(tail)
		// git fills in what the harness does not show, and is the only source
		// for whether this is a worktree. Cached, so it is nearly free.
		if info, ok := gitLookup(p.Path); ok {
			if a.Branch == "" {
				a.Branch = info.Branch
			}
			a.Worktree = info.Worktree
		}
		return a, true
	}
	return Agent{}, false
}

// tailLines returns up to n trailing non-blank lines, in screen order. Chrome
// detection anchors here: a running TUI owns the bottom of the screen, so
// scrollback left over from an exited one never counts.
func tailLines(lines []string, n int) []string {
	var out []string
	for i := len(lines) - 1; i >= 0 && len(out) < n; i-- {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		out = append([]string{lines[i]}, out...)
	}
	return out
}

// anyMatch reports whether re matches any of the lines.
func anyMatch(re *regexp.Regexp, lines []string) bool {
	for _, l := range lines {
		if re.MatchString(l) {
			return true
		}
	}
	return false
}

var durPartRe = regexp.MustCompile(`(\d+)\s*([hms])`)

// parseElapsed reads durations the way harnesses print them: "8m 3s", "26s",
// "1h 2m 3s". Anything after a separator is ignored, so token counters and
// clock times in the same field do not leak in.
func parseElapsed(s string) time.Duration {
	if i := strings.IndexAny(s, "·|"); i >= 0 {
		s = s[:i]
	}
	var d time.Duration
	for _, m := range durPartRe.FindAllStringSubmatch(s, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		switch m[2] {
		case "h":
			d += time.Duration(n) * time.Hour
		case "m":
			d += time.Duration(n) * time.Minute
		case "s":
			d += time.Duration(n) * time.Second
		}
	}
	return d
}
