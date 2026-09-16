package agent

import (
	"regexp"
	"strings"
	"time"

	"github.com/paraizofelipe/ag-mux/internal/tmux"
)

// OMP reads Oh My Pi panes.
//
// OMP marks its pane title with π and, while working, a braille spinner:
// "π ⠸ Entender uso inicial do OMP" busy, "π - coder" idle.
type OMP struct{}

const ompMark = "π"

var (
	// The status line is the bottom row of OMP's UI. Both layouts seen in the
	// wild carry a context percentage in the same shape:
	//   ⠸ 26s · <modelo> · <projeto> · main · 15.1%/272K
	//   ↑141k ↓22k R705k CH99.1% $0.111 6.9%/1.0M (auto)
	ompStatusRe = regexp.MustCompile(`\d+(?:\.\d+)?%/\d+(?:\.\d+)?[KM]`)

	// Braille spinner, U+2800–U+28FF.
	ompSpinnerRe = regexp.MustCompile(`[\x{2800}-\x{28FF}]`)
)

func (OMP) Name() string { return "omp" }

func (OMP) IsCandidate(p tmux.Pane) bool { return strings.HasPrefix(p.Title, ompMark) }

// Confirm checks only the bottom line. OMP's status line is the last thing it
// draws, so when the session exits and the shell prompt comes back, the status
// line is still in the scrollback but no longer at the bottom — which is
// exactly the case a whole-buffer search gets wrong.
func (OMP) Confirm(tail []string) bool {
	return anyMatch(ompStatusRe, tailLines(tail, 1))
}

func (OMP) Classify(p tmux.Pane, tail []string) (State, string, time.Duration) {
	status := tailLines(tail, 1)
	if len(status) == 0 {
		return StateUnknown, "", 0
	}
	line := status[0]

	// The spinner shows up in the title and in the status line; the status
	// line is the one that also carries the elapsed time.
	if loc := ompSpinnerRe.FindStringIndex(line); loc != nil {
		d := parseElapsed(line[loc[1]:])
		return StateBusy, strings.TrimSpace(strings.SplitN(line[loc[1]:], "·", 2)[0]), d
	}
	if ompSpinnerRe.MatchString(p.Title) {
		return StateBusy, "", 0
	}
	return StateIdle, "", 0
}

func (OMP) Task(p tmux.Pane) string {
	t := strings.TrimPrefix(p.Title, ompMark)
	t = ompSpinnerRe.ReplaceAllString(t, "")
	// OMP separates the marker from the name with "-" or ">" depending on
	// version and state.
	return strings.TrimSpace(strings.TrimLeft(t, " -><·»:"))
}
