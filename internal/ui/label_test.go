package ui

import (
	"strings"
	"testing"

	"github.com/paraizofelipe/ag-mux/internal/agent"
	"github.com/paraizofelipe/ag-mux/internal/tmux"
)

func agentAt(label, window string, win, pane int) agent.Agent {
	return agent.Agent{
		Label: label,
		Pane:  tmux.Pane{WindowName: window, WindowIndex: win, Index: pane},
	}
}

func labels(agents []agent.Agent) []string {
	out := make([]string, len(agents))
	for i, a := range agents {
		out[i] = a.Label
	}
	return out
}

func TestDisambiguate(t *testing.T) {
	cases := []struct {
		name   string
		agents []agent.Agent
		want   []string
	}{
		{
			name:   "distinct labels are left alone",
			agents: []agent.Agent{agentAt("atlas", "atlas", 1, 1), agentAt("farol", "farol", 2, 1)},
			want:   []string{"atlas", "farol"},
		},
		{
			// Two agents on one repo: the window names tell them apart.
			name:   "collision falls back to window names",
			agents: []agent.Agent{agentAt("ag-mux", "plan", 1, 1), agentAt("ag-mux", "review", 2, 1)},
			want:   []string{"plan", "review"},
		},
		{
			// Same repo, same window: only the position is left.
			name:   "collision with equal window names uses indexes",
			agents: []agent.Agent{agentAt("coder", "zsh", 3, 1), agentAt("coder", "zsh", 3, 2)},
			want:   []string{"coder 3.1", "coder 3.2"},
		},
		{
			// One window name is already another agent's label, so the group
			// cannot use window names — and it falls back as a whole, rather
			// than leaving one member named differently from the other.
			name: "falls back for the whole group when a name is taken",
			agents: []agent.Agent{
				agentAt("ag-mux", "atlas", 1, 1),
				agentAt("ag-mux", "other", 2, 1),
				agentAt("atlas", "win3", 3, 1),
			},
			want: []string{"ag-mux 1.1", "ag-mux 2.1", "atlas"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			disambiguate(c.agents)
			got := labels(c.agents)
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Errorf("labels = %v, want %v", got, c.want)
					return
				}
			}
			seen := map[string]bool{}
			for _, l := range got {
				if seen[l] {
					t.Errorf("label %q is still duplicated in %v", l, got)
				}
				seen[l] = true
			}
		})
	}
}

func TestWrapJoin(t *testing.T) {
	keys := []string{"⏎ ir", "z zoom", "p pin", "x kill", "n novo", "q sair"}
	for _, width := range []int{20, 30, 40, 80} {
		got := wrapJoin(keys, " · ", width)
		joined := ""
		for _, line := range got {
			if w := len([]rune(line)); w > width {
				t.Errorf("largura %d: linha %q tem %d colunas", width, line, w)
			}
			joined += line + " "
		}
		// Nothing may be dropped, whatever the width.
		for _, k := range keys {
			if !strings.Contains(joined, k) {
				t.Errorf("largura %d: perdeu %q em %v", width, k, got)
			}
		}
	}
}
