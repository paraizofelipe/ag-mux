package ui

import (
	"charm.land/lipgloss/v2"

	"github.com/paraizofelipe/ag-mux/internal/agent"
)

// Gruvbox dark, to sit next to the palette already in the user's tmux.conf.
var (
	colFg     = lipgloss.Color("#ebdbb2")
	colDim    = lipgloss.Color("#928374")
	colOrange = lipgloss.Color("#fe8019")
	colRed    = lipgloss.Color("#fb4934")
	colGreen  = lipgloss.Color("#b8bb26")
	colYellow = lipgloss.Color("#fabd2f")
	colBlue   = lipgloss.Color("#83a598")
)

var (
	styHeader = lipgloss.NewStyle().Foreground(colOrange).Bold(true)
	styCount  = lipgloss.NewStyle().Foreground(colDim)
	styRule   = lipgloss.NewStyle().Foreground(lipgloss.Color("#504945"))
	// Dimmer than styRule: the rules around the list are structure, the ones
	// between agents are only separation, and they must not compete.
	stySep      = lipgloss.NewStyle().Foreground(lipgloss.Color("#3c3836"))
	styLabel    = lipgloss.NewStyle().Foreground(colFg)
	styLabelSel = lipgloss.NewStyle().Foreground(colOrange).Bold(true)
	styTask     = lipgloss.NewStyle().Foreground(colDim)
	styCursor   = lipgloss.NewStyle().Foreground(colOrange).Bold(true)
	styHelp     = lipgloss.NewStyle().Foreground(colDim)
	styErr      = lipgloss.NewStyle().Foreground(colRed)
	styWarn     = lipgloss.NewStyle().Foreground(colYellow)
	styPin      = lipgloss.NewStyle().Foreground(colBlue)
	styBranch   = lipgloss.NewStyle().Foreground(colBlue)
	styWorktree = lipgloss.NewStyle().Foreground(colGreen)
)

// spinnerFrames animates agents that are working.
var spinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// harnessGlyph is the mark each harness already uses for itself, so the
// sidebar reads the same way the pane titles do.
func harnessGlyph(harness string) string {
	switch harness {
	case "claude":
		return "✳"
	case "omp":
		return "π"
	default:
		return "•"
	}
}

// stateIcon renders the state marker. Waiting is the one that has to catch the
// eye: it is the only state that means an agent is blocked on you.
func stateIcon(s agent.State, frame int) string {
	switch s {
	case agent.StateBusy:
		return lipgloss.NewStyle().Foreground(colYellow).Render(spinnerFrames[frame%len(spinnerFrames)])
	case agent.StateWaiting:
		return lipgloss.NewStyle().Foreground(colRed).Bold(true).Render("●")
	case agent.StateIdle:
		return lipgloss.NewStyle().Foreground(colDim).Render("○")
	default:
		return lipgloss.NewStyle().Foreground(colDim).Render("?")
	}
}
