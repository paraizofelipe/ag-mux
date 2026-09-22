package agent

import (
	"os"
	"testing"
	"time"

	"github.com/paraizofelipe/ag-mux/internal/git"
	"github.com/paraizofelipe/ag-mux/internal/opencode"
	"github.com/paraizofelipe/ag-mux/internal/tmux"
)

func stubOpencode(t *testing.T, byDir map[string]opencode.Status) {
	t.Helper()
	prev := opencodeLookup
	opencodeLookup = func(dir string, _ time.Time) (opencode.Status, bool) {
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
	got := Detect([]tmux.Pane{ocPane("%1", "/w/proj")}, []tmux.Pane{ocPane("%1", "/w/proj")}, capture)

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

func TestParseETime(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"00:07", 7 * time.Second, true},
		{"12:34", 12*time.Minute + 34*time.Second, true},
		{"01:02:03", time.Hour + 2*time.Minute + 3*time.Second, true},
		{"198-11:44:19", 198*24*time.Hour + 11*time.Hour + 44*time.Minute + 19*time.Second, true},
		{"", 0, false},
		{"amanhã", 0, false},
		{"1:2:3:4", 0, false},
	}
	for _, c := range cases {
		got, ok := parseETime(c.in)
		if ok != c.ok {
			t.Errorf("%q: ok = %v, queria %v", c.in, ok, c.ok)
			continue
		}
		if ok && got != c.want {
			t.Errorf("%q = %v, queria %v", c.in, got, c.want)
		}
	}
}

// Against a real process, because the point is reading a real ps.
func TestProcessStart(t *testing.T) {
	start, ok := processStart(os.Getpid())
	if !ok {
		t.Skip("ps não respondeu neste sistema")
	}
	if age := time.Since(start); age < 0 || age > time.Hour {
		t.Errorf("processo de teste começou há %v, o que não é plausível", age)
	}
	if _, ok := processStart(0); ok {
		t.Error("aceitou pid 0")
	}
}

// opencode's database is shared by the whole machine, so two of them in one
// directory are indistinguishable even sitting in different tmux sessions.
// Looking only at the panes being listed hides that, and hands one session's
// task to the other's agent with nothing looking wrong.
func TestOpenCodeAmbiguityIsServerWide(t *testing.T) {
	stubGit(t, git.Info{}, false)
	stubOpencode(t, map[string]opencode.Status{
		"/w/proj": {Session: opencode.Session{Title: "implementar filtros"}},
	})

	mine := ocPane("%1", "/w/proj")
	mine.Session = "um"
	other := ocPane("%2", "/w/proj")
	other.Session = "dois"

	capture := func(string, int) ([]string, error) { return nil, nil }

	// Listing only session "um", which holds a single opencode.
	got := Detect([]tmux.Pane{mine}, []tmux.Pane{mine, other}, capture)
	if len(got) != 1 {
		t.Fatalf("detectou %d agentes, queria 1", len(got))
	}
	if got[0].State != StateUnknown {
		t.Errorf("estado = %v, queria ? — há outro opencode no mesmo diretório em outra sessão",
			got[0].State)
	}
	if got[0].Task != "" {
		t.Errorf("mostrou a tarefa %q, que pode ser do processo da outra sessão", got[0].Task)
	}

	// Alone on the server, it is unambiguous again.
	got = Detect([]tmux.Pane{mine}, []tmux.Pane{mine}, capture)
	if got[0].Task != "implementar filtros" {
		t.Errorf("sozinho no servidor devia mostrar a tarefa, veio %q", got[0].Task)
	}
}

func stubOpencodeSession(t *testing.T, byID map[string]opencode.Status) {
	t.Helper()
	prev := opencodeSession
	opencodeSession = func(id string) (opencode.Status, bool) {
		st, ok := byID[id]
		return st, ok
	}
	t.Cleanup(func() { opencodeSession = prev })
}

// The plugin exists to answer the one question no amount of looking from
// outside can: which conversation belongs to this pane. With it, two opencode
// in one directory stop being ambiguous, because neither is being guessed at.
func TestOpenCodeSessionFromPluginBeatsDirectory(t *testing.T) {
	stubOpencode(t, map[string]opencode.Status{
		"/w/proj": {Session: opencode.Session{Title: "a sessão errada"}},
	})
	stubOpencodeSession(t, map[string]opencode.Status{
		"ses_um":   {Session: opencode.Session{Title: "tarefa do pane um"}},
		"ses_dois": {Session: opencode.Session{Title: "tarefa do pane dois"}},
	})

	um := ocPane("%1", "/w/proj")
	um.AgentSession = "ses_um"
	dois := ocPane("%2", "/w/proj")
	dois.AgentSession = "ses_dois"

	o := &OpenCode{}
	o.Observe([]tmux.Pane{um, dois})

	if got := o.Task(um); got != "tarefa do pane um" {
		t.Errorf("pane um -> %q", got)
	}
	if got := o.Task(dois); got != "tarefa do pane dois" {
		t.Errorf("pane dois -> %q", got)
	}
	for _, p := range []tmux.Pane{um, dois} {
		if state, _, _ := o.Classify(p, nil); state == StateUnknown {
			t.Errorf("%s ficou ambíguo mesmo com a sessão informada", p.ID)
		}
	}

	// Sem a sessão informada, o mesmo par volta a ser indistinguível.
	um.AgentSession, dois.AgentSession = "", ""
	o.Observe([]tmux.Pane{um, dois})
	if state, _, _ := o.Classify(um, nil); state != StateUnknown {
		t.Errorf("sem a sessão devia voltar a ser ?, veio %v", state)
	}
}
