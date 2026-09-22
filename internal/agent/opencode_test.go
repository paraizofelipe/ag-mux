package agent

import (
	"testing"
	"time"

	"github.com/paraizofelipe/ag-mux/internal/git"
	"github.com/paraizofelipe/ag-mux/internal/opencode"
	"github.com/paraizofelipe/ag-mux/internal/tmux"
)

func stubOpencode(t *testing.T, byDir map[string]opencode.Status) {
	t.Helper()
	prev := opencodeLookup
	opencodeLookup = func(dir string) (opencode.Status, bool) {
		st, ok := byDir[dir]
		return st, ok
	}
	t.Cleanup(func() { opencodeLookup = prev })
}

func ocPane(id, dir string) tmux.Pane {
	return tmux.Pane{ID: id, Command: "opencode", Title: "OpenCode", Path: dir}
}

// The whole point of the opencode adapter: no line of it comes from the
// screen, so detection must never capture one.
func TestOpenCodeNeverCaptures(t *testing.T) {
	stubGit(t, git.Info{}, false)
	stubOpencode(t, map[string]opencode.Status{
		"/w/proj": {Session: opencode.Session{Title: "ajustar exportacao"}},
	})

	captured := 0
	capture := func(string, int) ([]string, error) {
		captured++
		return nil, nil
	}
	got := Detect([]tmux.Pane{ocPane("%1", "/w/proj")}, capture)

	if len(got) != 1 {
		t.Fatalf("detectou %d agentes, queria 1", len(got))
	}
	if captured != 0 {
		t.Errorf("capture-pane foi chamado %d vez(es) para um pane de opencode", captured)
	}
	if got[0].Harness != "opencode" {
		t.Errorf("harness = %q", got[0].Harness)
	}
	if got[0].Task != "ajustar exportacao" {
		t.Errorf("tarefa = %q, queria o título da sessão", got[0].Task)
	}
}

func TestOpenCodeStates(t *testing.T) {
	clock := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	prev := timeNow
	timeNow = func() time.Time { return clock }
	t.Cleanup(func() { timeNow = prev })

	stubOpencode(t, map[string]opencode.Status{
		"/w/ocupado": {
			Session: opencode.Session{Title: "implementar filtros", Agent: "build"},
			Working: true, Since: clock.Add(-8*time.Minute - 3*time.Second),
		},
		"/w/parado": {Session: opencode.Session{Title: "revisar estrutura"}},
	})
	o := &OpenCode{}

	t.Run("trabalhando, com o tempo do próprio turno", func(t *testing.T) {
		state, _, elapsed := o.Classify(ocPane("%1", "/w/ocupado"), nil)
		if state != StateBusy {
			t.Errorf("estado = %v, queria trabalhando", state)
		}
		if want := 8*time.Minute + 3*time.Second; elapsed != want {
			t.Errorf("elapsed = %v, queria %v", elapsed, want)
		}
	})

	t.Run("turno fechado é ocioso", func(t *testing.T) {
		if state, _, _ := o.Classify(ocPane("%2", "/w/parado"), nil); state != StateIdle {
			t.Errorf("estado = %v, queria ocioso", state)
		}
	})

	t.Run("aberto mas nunca perguntado", func(t *testing.T) {
		state, detail, _ := o.Classify(ocPane("%3", "/w/novo"), nil)
		if state != StateIdle {
			t.Errorf("estado = %v, queria ocioso", state)
		}
		if detail == "" {
			t.Error("devia dizer que ainda não há sessão")
		}
	})
}

// The database ties a session to a directory, never to a pane. Two opencode in
// one directory are indistinguishable from outside, and showing one's work
// under the other's name is the failure this refuses to commit.
func TestOpenCodeAmbiguousDirectory(t *testing.T) {
	stubOpencode(t, map[string]opencode.Status{
		"/w/proj": {Session: opencode.Session{Title: "implementar filtros"}, Working: true},
	})
	o := &OpenCode{}
	panes := []tmux.Pane{ocPane("%1", "/w/proj"), ocPane("%2", "/w/proj"), ocPane("%3", "/w/outro")}
	o.Observe(panes)

	for _, id := range []string{"%1", "%2"} {
		state, detail, _ := o.Classify(ocPane(id, "/w/proj"), nil)
		if state != StateUnknown {
			t.Errorf("%s: estado = %v, queria ? em vez de um palpite", id, state)
		}
		if detail == "" {
			t.Errorf("%s: devia explicar por que não sabe", id)
		}
		if task := o.Task(ocPane(id, "/w/proj")); task != "" {
			t.Errorf("%s: mostrou a tarefa %q podendo ser do outro pane", id, task)
		}
	}
	// A directory with a single opencode is unaffected.
	o.Observe(panes[:1])
	if task := o.Task(ocPane("%1", "/w/proj")); task != "implementar filtros" {
		t.Errorf("com um só opencode a tarefa devia aparecer, veio %q", task)
	}
}

// The sidebar's own pane must not count towards the ambiguity.
func TestOpenCodeIgnoresSidebar(t *testing.T) {
	o := &OpenCode{}
	sidebar := ocPane("%9", "/w/proj")
	sidebar.IsSidebar = true
	o.Observe([]tmux.Pane{ocPane("%1", "/w/proj"), sidebar})
	if o.shared["/w/proj"] {
		t.Error("contou a sidebar como um segundo opencode")
	}
}
