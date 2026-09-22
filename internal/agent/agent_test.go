package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/paraizofelipe/ag-mux/internal/git"
	"github.com/paraizofelipe/ag-mux/internal/tmux"
)

// fixture is a real capture taken from a live pane, paired with the
// list-panes fields that pane had at the time.
type fixture struct {
	file    string
	pane    tmux.Pane
	harness string        // adapter expected to claim it
	live    bool          // survives the liveness stage
	state   State         // only meaningful when live
	elapsed time.Duration // 0 when not asserted
	detail  string        // substring match, "" to skip
}

var fixtures = []fixture{
	{
		file:    "claude-busy.txt",
		pane:    tmux.Pane{ID: "%46", Command: "2.1.269", Title: "✳ ajustar exportacao", Path: "/Users/dev/projects/atlas"},
		harness: "claude", live: true, state: StateBusy,
		elapsed: 8*time.Minute + 3*time.Second,
	},
	{
		file:    "claude-idle.txt",
		pane:    tmux.Pane{ID: "%30", Command: "2.1.159", Title: "✳ mapear configuracoes", Path: "/Users/dev/projects/orbita"},
		harness: "claude", live: true, state: StateIdle,
	},
	{
		file:    "claude-idle-done-clock.txt",
		pane:    tmux.Pane{ID: "%49", Command: "2.1.269", Title: "✳ revisar estrutura", Path: "/Users/dev/projects/atlas"},
		harness: "claude", live: true, state: StateIdle,
	},
	{
		// Finished its turn, but a prompt was typed and never sent.
		file:    "claude-draft.txt",
		pane:    tmux.Pane{ID: "%31", Command: "2.1.178", Title: "✳ implementar filtros", Path: "/Users/dev/projects/farol"},
		harness: "claude", live: true, state: StateWaiting, detail: "rascunho",
	},
	{
		// Claude Code exited; its chrome is still in the scrollback but the
		// shell prompt is back at the bottom and the title was never cleared.
		// A rule that searches the whole buffer instead of the bottom passes
		// this one wrongly and invents an agent that is not running.
		file:    "claude-dead-scrollback.txt",
		pane:    tmux.Pane{ID: "%39", Command: "zsh", Title: "✳ mapear configuracoes", Path: "/Users/dev/projects/orbita"},
		harness: "claude", live: false,
	},
}

// stubGit keeps the detection tests off the filesystem: the fixture paths are
// invented, and what git would say about them is tested in internal/git.
func stubGit(t *testing.T, info git.Info, ok bool) {
	t.Helper()
	prev := gitLookup
	gitLookup = func(string) (git.Info, bool) { return info, ok }
	t.Cleanup(func() { gitLookup = prev })
}

func load(t *testing.T, name string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

func adapterNamed(t *testing.T, name string) Adapter {
	t.Helper()
	for _, a := range Adapters() {
		if a.Name() == name {
			return a
		}
	}
	t.Fatalf("no adapter named %q", name)
	return nil
}

// Stage 1: the title marks a candidate, and nothing else does.
func TestIsCandidate(t *testing.T) {
	for _, f := range fixtures {
		t.Run(f.file, func(t *testing.T) {
			a := adapterNamed(t, f.harness)
			if !a.IsCandidate(f.pane) {
				t.Errorf("%s should be a %s candidate (title %q)", f.pane.ID, f.harness, f.pane.Title)
			}
		})
	}
	plain := tmux.Pane{ID: "%34", Command: "zsh", Title: "", Path: "/tmp"}
	for _, a := range Adapters() {
		if a.IsCandidate(plain) {
			t.Errorf("%s claimed a plain shell with an empty title", a.Name())
		}
	}
}

// Stage 2: this is what separates a running agent from a leftover title. The
// dead fixtures contain the harness chrome in their scrollback, so a rule that
// searches the whole buffer instead of the bottom passes here wrongly.
func TestConfirmRejectsStaleTitles(t *testing.T) {
	for _, f := range fixtures {
		t.Run(f.file, func(t *testing.T) {
			a := adapterNamed(t, f.harness)
			if got := a.Confirm(load(t, f.file)); got != f.live {
				t.Errorf("Confirm() = %v, want %v", got, f.live)
			}
		})
	}
}

// Stage 3: busy vs. your turn.
func TestClassify(t *testing.T) {
	for _, f := range fixtures {
		if !f.live {
			continue
		}
		t.Run(f.file, func(t *testing.T) {
			a := adapterNamed(t, f.harness)
			state, detail, elapsed := a.Classify(f.pane, load(t, f.file))
			if state != f.state {
				t.Errorf("state = %v, want %v (detail %q)", state, f.state, detail)
			}
			if f.elapsed != 0 && elapsed != f.elapsed {
				t.Errorf("elapsed = %v, want %v", elapsed, f.elapsed)
			}
			if f.detail != "" && !strings.Contains(detail, f.detail) {
				t.Errorf("detail = %q, want it to contain %q", detail, f.detail)
			}
		})
	}
}

func TestTask(t *testing.T) {
	cases := []struct {
		harness, title, want string
	}{
		{"claude", "✳ mapear configuracoes", "mapear configuracoes"},
	}
	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			got := adapterNamed(t, c.harness).Task(tmux.Pane{Title: c.title})
			if got != c.want {
				t.Errorf("Task() = %q, want %q", got, c.want)
			}
		})
	}
}

// End to end over the whole fixture set: the five live agents come back, the
// three leftover titles do not, and neither does the sidebar itself.
func TestDetect(t *testing.T) {
	stubGit(t, git.Info{}, false)

	var panes []tmux.Pane
	captures := map[string][]string{}
	for _, f := range fixtures {
		panes = append(panes, f.pane)
		captures[f.pane.ID] = load(t, f.file)
	}
	panes = append(panes,
		tmux.Pane{ID: "%99", Command: "ag-mux", Title: "ag-mux", IsSidebar: true},
		tmux.Pane{ID: "%34", Command: "zsh", Title: "", Path: "/tmp"},
	)
	capture := func(target string, n int) ([]string, error) { return captures[target], nil }

	got := Detect(panes, capture)

	want := []string{"%46", "%30", "%49", "%31"}
	if len(got) != len(want) {
		var ids []string
		for _, a := range got {
			ids = append(ids, a.Pane.ID)
		}
		t.Fatalf("detected %d agents %v, want %d %v", len(got), ids, len(want), want)
	}
	seen := map[string]Agent{}
	for _, a := range got {
		seen[a.Pane.ID] = a
	}
	for _, id := range want {
		if _, ok := seen[id]; !ok {
			t.Errorf("agent %s missing from results", id)
		}
	}
	if a := seen["%30"]; a.Label != "orbita" {
		t.Errorf("label = %q, want %q", a.Label, "orbita")
	}
}

// Claude Code prints the branch in its own footer, so the sidebar can show it
// without asking git at all.
func TestClaudeBranchFromChrome(t *testing.T) {
	cases := []struct {
		file   string
		branch string
		dirty  bool
		ok     bool
	}{
		{"claude-idle.txt", "main", false, true},
		{"claude-draft.txt", "main", true, true},
		// No git suffix in the footer: the directory is not a repository, and
		// the adapter must say so rather than invent a branch.
		{"claude-busy.txt", "", false, false},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			branch, dirty, ok := Claude{}.Branch(load(t, c.file))
			if ok != c.ok || branch != c.branch || dirty != c.dirty {
				t.Errorf("Branch() = (%q, %v, %v), want (%q, %v, %v)",
					branch, dirty, ok, c.branch, c.dirty, c.ok)
			}
		})
	}
}

// Claude redraws its spinner in place, but a frame can survive above the live
// one in the captured tail. Reading the first match reports a time that jumps
// backwards as the screen moves.
func TestClaudeUsesBottomSpinner(t *testing.T) {
	tail := []string{
		"✻ Levitating… (12s · ↓ 1.1k tokens)",
		"  ⎿  algum output de ferramenta",
		"✻ Levitating… (3m 9s · ↓ 5.1k tokens)",
		"  Context ████░░ 40% │ Usage ░░ 1%",
		"  ⏵⏵ bypass permissions on (shift+tab to cycle)",
	}
	state, _, elapsed := Claude{}.Classify(tmux.Pane{}, tail)
	if state != StateBusy {
		t.Fatalf("estado = %v", state)
	}
	if want := 3*time.Minute + 9*time.Second; elapsed != want {
		t.Errorf("elapsed = %v, queria %v (o spinner de baixo é o vivo)", elapsed, want)
	}
}
