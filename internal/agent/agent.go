// Package agent turns tmux panes into a list of coding agents with live state.
//
// Detection runs in three stages, cheapest first:
//
//  1. Candidate — the pane title carries a harness marker. Costs nothing
//     beyond the list-panes we already ran.
//  2. Liveness — the pane's foreground process is not a shell, and the
//     harness chrome sits at the bottom of the screen. Harnesses leave their
//     title behind when they exit, so a title alone is a false positive.
//  3. Classification — the harness adapter reads the tail and decides whether
//     the agent is working or waiting on you.
package agent

import (
	"path/filepath"
	"time"

	"github.com/paraizofelipe/ag-mux/internal/tmux"
)

// State is what an agent is doing right now.
type State int

const (
	// StateUnknown means the agent is alive but no rule matched. Shown as
	// such rather than guessed, so a stale rule is visible instead of silent.
	StateUnknown State = iota
	// StateIdle means the agent finished and it is your turn.
	StateIdle
	// StateWaiting means the agent is blocked on you specifically: a
	// permission dialog, or a prompt you started typing and never sent.
	StateWaiting
	// StateBusy means the agent is working.
	StateBusy
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "ocioso"
	case StateWaiting:
		return "precisa de você"
	case StateBusy:
		return "trabalhando"
	default:
		return "?"
	}
}

// Agent is one live coding agent running in a pane.
type Agent struct {
	Pane     tmux.Pane
	Harness  string // "claude" | "opencode"
	Label    string // directory basename, the name you think of it by
	Task     string // what it is doing, from the pane title
	State    State
	Detail   string // elapsed time, "rascunho pendente", ...
	Elapsed  time.Duration
	Source   Source // which signal decided State
	Branch   string // empty when the directory is not a repository
	Dirty    bool   // the branch has uncommitted changes
	Worktree bool   // a linked worktree rather than the main checkout
	Pinned   bool
}

// Current reports whether this agent's pane is the active one.
func (a Agent) Current() bool { return a.Pane.Active && a.Pane.WindowAct }

// Adapter knows how to recognise and read one harness.
type Adapter interface {
	// Name identifies the harness.
	Name() string
	// IsCandidate reports whether the pane might host this harness, using
	// only list-panes fields.
	IsCandidate(p tmux.Pane) bool
	// Confirm reports whether the harness is still running, by looking for
	// its chrome at the bottom of the screen.
	Confirm(tail []string) bool
	// Classify reads the tail into a state.
	Classify(p tmux.Pane, tail []string) (State, string, time.Duration)
	// Task extracts the task description from the pane title.
	Task(p tmux.Pane) string
	// Branch reads the branch out of the harness's own interface, when it
	// shows one. These lines are already in hand, so it saves asking git.
	Branch(tail []string) (name string, dirty bool, ok bool)
}

// Adapters is every harness ag-mux knows about.
func Adapters() []Adapter { return []Adapter{Claude{}} }

// label is the name a human uses for an agent: the working directory.
func label(p tmux.Pane) string {
	if p.Path == "" {
		return p.WindowName
	}
	return filepath.Base(p.Path)
}
