package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/paraizofelipe/ag-mux/internal/agent"
	"github.com/paraizofelipe/ag-mux/internal/tmux"
)

// clickModel is two agents of three lines each: lines 2-4 and 6-8, with the
// separator at 5.
func clickModel(t *testing.T) (Model, *[]string) {
	t.Helper()
	var jumped []string
	prevFocus, prevNow := focusPane, uiNow
	focusPane = func(p tmux.Pane) error { jumped = append(jumped, p.ID); return nil }
	t.Cleanup(func() { focusPane, uiNow = prevFocus, prevNow })

	a := mkAgent("alfa", agent.StateIdle, false)
	a.Pane = tmux.Pane{ID: "%1"}
	a.Branch = "main"
	b := mkAgent("beta", agent.StateBusy, false)
	b.Pane = tmux.Pane{ID: "%2"}
	b.Branch = "main"
	return Model{agents: []agent.Agent{a, b}, width: 40, lastClicked: -1}, &jumped
}

func clickAt(m Model, y int) Model {
	next, _ := m.Update(tea.MouseClickMsg{Y: y, Button: tea.MouseLeft})
	return next.(Model)
}

func at(base time.Time, d time.Duration) func() time.Time {
	return func() time.Time { return base.Add(d) }
}

func TestClickSelects(t *testing.T) {
	m, jumped := clickModel(t)
	base := time.Now()
	uiNow = at(base, 0)

	m = clickAt(m, 6) // primeira linha do segundo agente
	if m.cursor != 1 {
		t.Errorf("cursor = %d, queria 1", m.cursor)
	}
	if len(*jumped) != 0 {
		t.Errorf("um clique só não devia pular, pulou para %v", *jumped)
	}
}

func TestDoubleClickJumps(t *testing.T) {
	m, jumped := clickModel(t)
	base := time.Now()

	uiNow = at(base, 0)
	m = clickAt(m, 6)
	uiNow = at(base, 150*time.Millisecond)
	m = clickAt(m, 7) // outra linha DO MESMO agente ainda conta

	if len(*jumped) != 1 || (*jumped)[0] != "%2" {
		t.Fatalf("pulos = %v, queria um para %%2", *jumped)
	}

	// Um terceiro clique rápido não pode pular de novo: o par foi consumido.
	uiNow = at(base, 250*time.Millisecond)
	clickAt(m, 6)
	if len(*jumped) != 1 {
		t.Errorf("o terceiro clique pulou de novo: %v", *jumped)
	}
}

func TestSlowClicksDoNotJump(t *testing.T) {
	m, jumped := clickModel(t)
	base := time.Now()

	uiNow = at(base, 0)
	m = clickAt(m, 6)
	uiNow = at(base, doubleClickWindow+time.Millisecond)
	clickAt(m, 6)

	if len(*jumped) != 0 {
		t.Errorf("cliques separados por mais que a janela pularam: %v", *jumped)
	}
}

func TestClicksOnDifferentAgentsDoNotJump(t *testing.T) {
	m, jumped := clickModel(t)
	base := time.Now()

	uiNow = at(base, 0)
	m = clickAt(m, 2) // agente 0
	uiNow = at(base, 100*time.Millisecond)
	m = clickAt(m, 6) // agente 1

	if len(*jumped) != 0 {
		t.Errorf("dois agentes diferentes contaram como duplo clique: %v", *jumped)
	}
	if m.cursor != 1 {
		t.Errorf("cursor = %d, queria 1", m.cursor)
	}
}

// A click on a separator selects nothing, and must not pair with the click
// before it — otherwise clicking an agent, then the rule, then the agent again
// would jump by accident.
func TestClickOnSeparatorBreaksThePair(t *testing.T) {
	m, jumped := clickModel(t)
	base := time.Now()

	uiNow = at(base, 0)
	m = clickAt(m, 6)
	uiNow = at(base, 80*time.Millisecond)
	m = clickAt(m, 5) // a régua
	uiNow = at(base, 160*time.Millisecond)
	clickAt(m, 6)

	if len(*jumped) != 0 {
		t.Errorf("a régua no meio não quebrou o par: %v", *jumped)
	}
}
