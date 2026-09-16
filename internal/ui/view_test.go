package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/paraizofelipe/ag-mux/internal/agent"
	"github.com/paraizofelipe/ag-mux/internal/tmux"
)

func mkAgent(label string, st agent.State, current bool) agent.Agent {
	return agent.Agent{
		Pane:  tmux.Pane{ID: "%1", Active: current, WindowAct: current},
		Label: label,
		Task:  "uma tarefa qualquer",
		State: st,
	}
}

// The agent you are sitting in used to lose its state icon to a "you are here"
// arrow, which reads exactly like a frozen spinner — and it is the one agent
// whose marker you look at most. Nothing may share that column.
func TestCurrentAgentKeepsStateIcon(t *testing.T) {
	m := Model{}
	for _, st := range []agent.State{agent.StateBusy, agent.StateWaiting, agent.StateIdle} {
		head := m.agentRows(mkAgent("proj", st, true), false, 40)[0]
		icon := stripANSI(stateIcon(st, 0))
		if !strings.Contains(head, icon) {
			t.Errorf("%v: perdeu o ícone de estado %q em %q", st, icon, head)
		}
		if strings.ContainsAny(head, "◀") {
			t.Errorf("%v: a seta voltou pra coluna do estado: %q", st, head)
		}
	}
}

// The spinner must actually advance frame to frame, or "it animates" is a
// claim no test is making.
func TestSpinnerAdvancesForCurrentAgent(t *testing.T) {
	seen := map[string]bool{}
	for frame := range 4 {
		m := Model{frame: frame}
		seen[m.agentRows(mkAgent("proj", agent.StateBusy, true), false, 40)[0]] = true
	}
	if len(seen) < 4 {
		t.Errorf("4 frames renderizaram %d linhas distintas, queria 4", len(seen))
	}
}

// Every row has to fill the pane exactly: one cell too many wraps the line and
// shoves the whole sidebar down by one.
func TestAgentRowWidth(t *testing.T) {
	names := []string{"x", "proj", "um-nome-de-diretorio-bem-comprido-mesmo"}
	for _, w := range []int{20, 30, 40, 60} {
		for _, name := range names {
			for _, current := range []bool{false, true} {
				for _, pinned := range []bool{false, true} {
					a := mkAgent(name, agent.StateBusy, current)
					a.Pinned = pinned
					for i, row := range (Model{}).agentRows(a, true, w) {
						if got := lipgloss.Width(row); got > w {
							t.Errorf("w=%d nome=%q current=%v pinned=%v linha %d: largura %d > %d\n  %q",
								w, name, current, pinned, i, got, w, row)
						}
					}
				}
			}
		}
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
