package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/paraizofelipe/ag-mux/internal/agent"
)

func (m Model) View() tea.View {
	w := m.width
	if w < 12 {
		w = 12
	}
	rule := styRule.Render(strings.Repeat("─", w))

	top := []string{m.header(w), rule}
	switch {
	case m.err != nil:
		top = append(top, styErr.Render(truncate("erro: "+m.err.Error(), w)))
	case len(m.agents) == 0:
		top = append(top, styTask.Render(truncate("nenhum agente nesta sessão", w)))
	default:
		top = append(top, m.agentList(w)...)
	}

	// The help sits at the bottom of the pane so it does not shift every time
	// an agent appears or finishes.
	bottom := append([]string{rule}, m.footer(w)...)

	lines := top
	if fill := m.height - len(top) - len(bottom); fill > 0 {
		lines = append(lines, make([]string, fill)...)
	}
	lines = append(lines, bottom...)

	v := tea.NewView(strings.Join(lines, "\n"))
	// Own the screen: the sidebar redraws continuously and must not scroll
	// the pane's scrollback while doing it.
	v.AltScreen = true
	v.WindowTitle = "ag-mux"
	return v
}

func (m Model) header(w int) string {
	busy, waiting := 0, 0
	for _, a := range m.agents {
		switch a.State {
		case agent.StateBusy:
			busy++
		case agent.StateWaiting:
			waiting++
		}
	}
	right := fmt.Sprintf("%d", len(m.agents))
	if waiting > 0 {
		right = fmt.Sprintf("%d! %d", waiting, len(m.agents))
	}
	left := "AGENTES"
	pad := w - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 1 {
		return styHeader.Render(truncate(left, w))
	}
	countStyle := styCount
	if waiting > 0 {
		countStyle = lipgloss.NewStyle().Foreground(colRed).Bold(true)
	}
	return styHeader.Render(left) + strings.Repeat(" ", pad) + countStyle.Render(right)
}

// agentList renders every agent, separated by a rule.
//
// The rule goes only *between* neighbours: above the first and below the last
// the header and footer rules already sit, and doubling them would read as an
// empty entry.
func (m Model) agentList(w int) []string {
	divider := stySep.Render(strings.Repeat("─", w))
	var out []string
	for i, a := range m.agents {
		if i > 0 {
			out = append(out, divider)
		}
		out = append(out, m.agentRows(a, i == m.cursor, w)...)
	}
	return out
}

// agentRows renders one agent: a name line and a second line with what it is
// doing.
func (m Model) agentRows(a agent.Agent, selected bool, w int) []string {
	cursor := " "
	if selected {
		cursor = "▸"
	}
	// The marker column holds the state and nothing else. It used to be shared
	// with a "you are here" arrow, which was wrong twice over: the arrow
	// replaced the state on the one agent you look at most, and ◀ is an East
	// Asian Ambiguous glyph, so terminals draw it two cells wide while every
	// width calculation counts it as one — it printed over its neighbour.
	marker := stateIcon(a.State, m.frame)
	mw := lipgloss.Width(marker)

	name := a.Label
	if a.Pinned {
		name = "◆ " + name
	}
	name = truncate(name, w-3-mw)
	pad := w - 3 - lipgloss.Width(name) - mw
	if pad < 0 {
		pad = 0
	}
	style := styLabel
	if selected {
		style = styLabelSel
	}
	if a.Current() {
		// "You are here" marks the name instead of taking a column. An
		// underline is a modifier, so it stacks with the selected colour
		// rather than replacing it, and it cannot overflow the way the arrow
		// this replaced did.
		style = style.Underline(true)
	}
	glyph := styTask.Render(harnessGlyph(a.Harness))
	if a.Pinned {
		glyph = styPin.Render(harnessGlyph(a.Harness))
	}

	line := styCursor.Render(cursor) + glyph + " " + style.Render(name) +
		strings.Repeat(" ", pad) + marker

	detail := a.Task
	if detail == "" {
		detail = a.State.String()
	}
	right := ""
	if a.Elapsed > 0 {
		right = " " + compactDuration(a.Elapsed)
	}
	detail = truncate(detail, w-3-lipgloss.Width(right))
	dpad := w - 3 - lipgloss.Width(detail) - lipgloss.Width(right)
	if dpad < 0 {
		dpad = 0
	}
	rows := []string{line, "   " + styTask.Render(detail) +
		strings.Repeat(" ", dpad) + styWarn.Render(right)}
	if repo := repoLine(a, w); repo != "" {
		rows = append(rows, repo)
	}
	return rows
}

// repoLine says which copy of a repository the agent is working in: the branch
// and, when it matters, that this is a linked worktree rather than the main
// checkout. The worktree mark is pinned right so a long branch name truncates
// before it does.
func repoLine(a agent.Agent, w int) string {
	if a.Branch == "" && !a.Worktree {
		return ""
	}
	mark := ""
	if a.Worktree {
		mark = "⧉ worktree"
	}
	left := ""
	if a.Branch != "" {
		left = "⎇ " + a.Branch
		if a.Dirty {
			left += "*"
		}
	}
	avail := w - 3 - lipgloss.Width(mark)
	if mark != "" {
		avail -= 2
	}
	left = truncate(left, avail)
	pad := w - 3 - lipgloss.Width(left) - lipgloss.Width(mark)
	if pad < 1 {
		pad = 1
	}
	return "   " + styBranch.Render(left) + strings.Repeat(" ", pad) + styWorktree.Render(mark)
}

// footer renders the prompt or the key help, wrapped to the pane width. The
// keys wrap onto extra lines rather than being cut off: a help line that ends
// mid-word is worse than one that takes two rows.
func (m Model) footer(w int) []string {
	switch m.mode {
	case modeConfirmKill:
		a, _ := m.Selected()
		return []string{styErr.Render(truncate("matar "+a.Label+"? y/n", w))}
	case modeNewAgent:
		return []string{styWarn.Render(truncate("novo agente: c=claude o=opencode", w))}
	}
	if m.status != "" {
		return []string{styWarn.Render(truncate(m.status, w))}
	}
	keys := []string{"⏎ ir", "z zoom", "p pin", "x kill", "n novo", "q sair"}
	lines := make([]string, 0, 2)
	for _, line := range wrapJoin(keys, " · ", w) {
		lines = append(lines, styHelp.Render(line))
	}
	return lines
}

// wrapJoin packs items into lines no wider than w, joined by sep.
func wrapJoin(items []string, sep string, w int) []string {
	var lines []string
	current := ""
	for _, item := range items {
		candidate := item
		if current != "" {
			candidate = current + sep + item
		}
		if current != "" && lipgloss.Width(candidate) > w {
			lines = append(lines, current)
			current = item
			continue
		}
		current = candidate
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

// truncate cuts s to w display columns, marking the cut with an ellipsis.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	var (
		b     strings.Builder
		width int
	)
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if width+rw > w-1 {
			break
		}
		b.WriteRune(r)
		width += rw
	}
	return b.String() + "…"
}

// compactDuration is "14m", "2m14s", "45s" — short enough for a 40-column pane.
func compactDuration(d time.Duration) string {
	s := int(d.Seconds())
	switch {
	case s >= 3600:
		return fmt.Sprintf("%dh%dm", s/3600, (s%3600)/60)
	case s >= 60:
		return fmt.Sprintf("%dm%02ds", s/60, s%60)
	default:
		return fmt.Sprintf("%ds", s)
	}
}
