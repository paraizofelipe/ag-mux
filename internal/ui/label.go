package ui

import (
	"fmt"

	"github.com/paraizofelipe/ag-mux/internal/agent"
)

// disambiguate makes every label unique.
//
// Agents are named after their working directory, which is what you actually
// call them — but running several agents on one repo is normal, and a list
// with two identical rows is useless for picking one. Window names are the
// next most human thing to fall back on; a position suffix is the last resort.
//
// A colliding group is renamed as a whole, never agent by agent: half the
// group keeping the directory while the other half shows window names reads
// like two unrelated things.
func disambiguate(agents []agent.Agent) {
	if len(agents) < 2 {
		return
	}
	taken := map[string]int{}
	groups := map[string][]int{}
	for i, a := range agents {
		taken[a.Label]++
		groups[a.Label] = append(groups[a.Label], i)
	}

	for label, idx := range groups {
		if len(idx) < 2 {
			continue
		}
		if names, ok := freeWindowNames(agents, idx, taken); ok {
			for n, i := range idx {
				agents[i].Label = names[n]
			}
			continue
		}
		for _, i := range idx {
			p := agents[i].Pane
			agents[i].Label = fmt.Sprintf("%s %d.%d", label, p.WindowIndex, p.Index)
		}
	}
}

// freeWindowNames returns the window names of a colliding group, but only if
// they tell its members apart and none of them is already some other agent's
// label — renaming onto a name in use would just move the collision.
func freeWindowNames(agents []agent.Agent, idx []int, taken map[string]int) ([]string, bool) {
	names := make([]string, 0, len(idx))
	seen := map[string]bool{}
	for _, i := range idx {
		n := agents[i].Pane.WindowName
		if n == "" || seen[n] || taken[n] > 0 {
			return nil, false
		}
		seen[n] = true
		names = append(names, n)
	}
	return names, true
}
