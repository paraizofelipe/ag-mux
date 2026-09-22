package ui

import (
	"fmt"
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

// The rule belongs between agents only: one above the first or below the last
// would sit against the header/footer rules and read as an empty entry.
func TestAgentListSeparators(t *testing.T) {
	isRule := func(s string) bool { return strings.Contains(stripANSI(s), "──") }

	for _, n := range []int{0, 1, 2, 5} {
		m := Model{}
		for i := range n {
			m.agents = append(m.agents, mkAgent(fmt.Sprintf("a%d", i), agent.StateIdle, false))
		}
		rows := m.agentList(40)

		got := 0
		for _, r := range rows {
			if isRule(r) {
				got++
			}
		}
		want := n - 1
		if n == 0 {
			want = 0
		}
		if got != want {
			t.Errorf("%d agentes: %d réguas, queria %d", n, got, want)
		}
		if n > 0 {
			if isRule(rows[0]) {
				t.Errorf("%d agentes: régua antes do primeiro", n)
			}
			if isRule(rows[len(rows)-1]) {
				t.Errorf("%d agentes: régua depois do último", n)
			}
		}
	}
}

// A rule that is not exactly the pane width either wraps or leaves a notch.
func TestSeparatorWidth(t *testing.T) {
	m := Model{agents: []agent.Agent{
		mkAgent("a", agent.StateIdle, false),
		mkAgent("b", agent.StateIdle, false),
	}}
	for _, w := range []int{20, 30, 40, 60} {
		for _, r := range m.agentList(w) {
			if !strings.Contains(stripANSI(r), "──") {
				continue
			}
			if got := lipgloss.Width(r); got != w {
				t.Errorf("w=%d: régua com %d colunas", w, got)
			}
		}
	}
}

// The pane you are sitting in has to be findable in the list. It used to carry
// an arrow in the state column, which both hid the state and overflowed; the
// mark now rides on the name, where it costs no column.
func TestCurrentAgentIsMarked(t *testing.T) {
	m := Model{}
	here := m.agentRows(mkAgent("proj", agent.StateBusy, true), false, 40)[0]
	away := m.agentRows(mkAgent("proj", agent.StateBusy, false), false, 40)[0]

	if here == away {
		t.Error("o agente em que você está não se distingue dos outros")
	}
	if stripANSI(here) != stripANSI(away) {
		t.Errorf("a marca mudou o texto, não só o estilo:\n  aqui: %q\n  fora: %q",
			stripANSI(here), stripANSI(away))
	}
	if lipgloss.Width(here) != lipgloss.Width(away) {
		t.Errorf("larguras diferentes: %d vs %d", lipgloss.Width(here), lipgloss.Width(away))
	}

	// It must survive being the selected row too, or it vanishes exactly when
	// you navigate to it.
	sel := m.agentRows(mkAgent("proj", agent.StateBusy, true), true, 40)
	selAway := m.agentRows(mkAgent("proj", agent.StateBusy, false), true, 40)
	if sel[0] == selAway[0] {
		t.Error("selecionado E atual perdeu a marca de atual")
	}
}

// The rule runs down every line of an agent, so the three lines read as one
// block instead of three loose rows.
func TestAgentRowsHaveLeftRule(t *testing.T) {
	a := mkAgent("proj", agent.StateBusy, false)
	a.Branch = "main"
	rows := (Model{}).agentRows(a, false, 40)
	if len(rows) != 3 {
		t.Fatalf("%d linhas, queria 3", len(rows))
	}
	for i, r := range rows {
		if !strings.HasPrefix(stripANSI(r), barGlyph) {
			t.Errorf("linha %d não começa com a régua: %q", i, stripANSI(r))
		}
	}
}

// Selection is the rule's colour now that the arrow is gone, so it has to
// actually change — and only in style, never in width.
func TestSelectedAgentRuleDiffers(t *testing.T) {
	a := mkAgent("proj", agent.StateIdle, false)
	a.Branch = "main"
	m := Model{}
	sel := m.agentRows(a, true, 40)
	plain := m.agentRows(a, false, 40)

	same := 0
	for i := range sel {
		if sel[i] == plain[i] {
			same++
		}
		if stripANSI(sel[i]) != stripANSI(plain[i]) {
			t.Errorf("linha %d: a seleção mudou o texto, não só a cor", i)
		}
		if lipgloss.Width(sel[i]) != lipgloss.Width(plain[i]) {
			t.Errorf("linha %d: larguras diferentes", i)
		}
	}
	if same == len(sel) {
		t.Error("selecionar não mudou nada visualmente")
	}
}
