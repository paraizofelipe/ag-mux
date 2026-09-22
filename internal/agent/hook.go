package agent

import (
	"strconv"
	"strings"
	"time"

	"github.com/paraizofelipe/ag-mux/internal/tmux"
)

// timeNow is a variable so tests can pin the clock.
var timeNow = time.Now

// Source says which signal decided an agent's state.
type Source int

const (
	// SourceScreen means the state was read off the pane's screen.
	SourceScreen Source = iota
	// SourceHook means the harness reported it through its own lifecycle
	// hooks. See hooks/ag-mux-hook.sh.
	SourceHook
)

func (s Source) String() string {
	if s == SourceHook {
		return "hook"
	}
	return "tela"
}

// hookReport is what a harness last said about itself.
type hookReport struct {
	State  State
	Detail string
	At     time.Time
}

// hookStates are the words the hook script writes. They are deliberately not
// the Go constant names: the script is the wire format, and it should stay
// readable in `tmux show-options -p`.
var hookStates = map[string]State{
	"busy":    StateBusy,
	"waiting": StateWaiting,
	"idle":    StateIdle,
}

// parseHook reads the pane option the hook writes: "<state> <epoch> [detail]".
// Anything it does not understand is treated as absent, so a half-written or
// hand-edited option degrades to plain screen detection instead of breaking it.
func parseHook(s string) (hookReport, bool) {
	name, rest, ok := strings.Cut(strings.TrimSpace(s), " ")
	if !ok {
		return hookReport{}, false
	}
	state, known := hookStates[name]
	if !known {
		return hookReport{}, false
	}
	epoch, detail, _ := strings.Cut(rest, " ")
	secs, err := strconv.ParseInt(epoch, 10, 64)
	if err != nil {
		return hookReport{}, false
	}
	return hookReport{
		State:  state,
		Detail: strings.TrimSpace(detail),
		At:     time.Unix(secs, 0),
	}, true
}

// arbitrate reconciles what the harness reported with what the screen shows.
//
// The two sources are not interchangeable, and neither is strictly better.
//
// The hook is the only one that can see a permission dialog with certainty.
// Reading that off the screen is guesswork — the rule ag-mux has for it was
// written without a real capture, and it is the known weak spot. But the hook
// speaks only at the events it is wired to, so an ESC interrupt or a crash
// leaves its last word standing with nothing to correct it.
//
// The screen has the opposite shape: it is redrawn every frame, so it cannot go
// stale, and the spinner line is unambiguous. What it cannot do is tell a
// permission dialog apart from an idle prompt box — both are a chevron and a
// box.
//
// So each source wins where it is strong. The hook owns "precisa de você". The
// screen owns everything else, and may overrule a stale "precisa de você" when
// the spinner comes back, which is what happens the moment you answer.
//
// That last correction only applies to a harness whose reports can go stale.
// One that reports the end of a wait as well as its start — opencode, through
// permission.replied — needs no correcting and must not get one: a turn paused
// on a permission prompt still reads as a turn in flight, so the other source
// would overrule a report that is simply right.
func arbitrate(s screenState, h hookReport, ok, authoritative bool, now time.Time) (screenState, Source) {
	if !ok {
		return s, SourceScreen
	}
	switch {
	case h.State == StateWaiting && s.State == StateBusy && !authoritative:
		// The spinner is back: you answered and the hook has not spoken since.
		return s, SourceScreen

	case h.State == StateWaiting:
		detail := h.Detail
		if detail == "" {
			detail = "pendente"
		}
		// How long it has been blocked is the number that matters here, and
		// the screen has no way to know it.
		waited := now.Sub(h.At).Truncate(time.Second)
		if waited < 0 {
			waited = 0
		}
		return screenState{StateWaiting, detail, waited}, SourceHook

	case s.State == StateUnknown:
		// No screen rule matched. The harness's own word beats showing "?".
		return screenState{h.State, h.Detail, s.Elapsed}, SourceHook

	default:
		return s, SourceScreen
	}
}

// screenState groups what Classify returns, so arbitrate reads as one decision
// instead of four parallel arguments.
type screenState struct {
	State   State
	Detail  string
	Elapsed time.Duration
}

// Explanation is every step of the decision for one pane. Detect uses it to
// build an Agent and doctor prints it, so what doctor shows is what the sidebar
// actually decided rather than a second implementation that can drift.
type Explanation struct {
	Screen  screenState // what the adapter read off the screen
	Hook    hookReport  // what the harness reported, if anything
	HasHook bool
	HookAge time.Duration
	Final   screenState
	Source  Source
}

// Explain runs classification and arbitration for one pane against a tail that
// has already been captured.
func Explain(ad Adapter, p tmux.Pane, tail []string, now time.Time) Explanation {
	state, detail, elapsed := ad.Classify(p, tail)
	screen := screenState{state, detail, elapsed}
	hook, hasHook := parseHook(p.Hook)
	final, source := arbitrate(screen, hook, hasHook, ad.HookAuthoritative(), now)

	e := Explanation{Screen: screen, Hook: hook, HasHook: hasHook, Final: final, Source: source}
	if hasHook {
		if age := now.Sub(hook.At).Truncate(time.Second); age > 0 {
			e.HookAge = age
		}
	}
	return e
}

// ScreenState is what the screen alone said, for doctor.
func (e Explanation) ScreenState() (State, string, time.Duration) {
	return e.Screen.State, e.Screen.Detail, e.Screen.Elapsed
}

// HookState is what the harness reported, and how long ago.
func (e Explanation) HookState() (State, string, time.Duration, bool) {
	return e.Hook.State, e.Hook.Detail, e.HookAge, e.HasHook
}

// FinalState is what the sidebar shows, and which source decided it.
func (e Explanation) FinalState() (State, string, time.Duration, Source) {
	return e.Final.State, e.Final.Detail, e.Final.Elapsed, e.Source
}
