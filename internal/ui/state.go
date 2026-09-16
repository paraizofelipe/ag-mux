package ui

import (
	"sort"
	"strings"

	"github.com/paraizofelipe/ag-mux/internal/tmux"
)

// StateOption is the tmux option the sidebar keeps its state in.
//
// Hiding the sidebar closes it, so anything the user set by hand — which
// agents are pinned, where the cursor is — has to survive outside the
// process. It is a handful of pane ids, so a tmux option is enough and
// leaves nothing behind on disk.
const StateOption = "@ag-mux-state"

type persisted struct {
	selected string
	pinned   map[string]bool
}

// encodeState renders state as "selected|pin,pin". Pane ids are "%12", so
// neither separator can appear inside a value.
func encodeState(p persisted) string {
	pins := make([]string, 0, len(p.pinned))
	for id, on := range p.pinned {
		if on {
			pins = append(pins, id)
		}
	}
	sort.Strings(pins) // stable, so the option stops churning
	return p.selected + "|" + strings.Join(pins, ",")
}

func decodeState(s string) persisted {
	out := persisted{pinned: map[string]bool{}}
	sel, pins, _ := strings.Cut(s, "|")
	out.selected = strings.TrimSpace(sel)
	for _, id := range strings.Split(pins, ",") {
		if id = strings.TrimSpace(id); id != "" {
			out.pinned[id] = true
		}
	}
	return out
}

func loadState() persisted { return decodeState(tmux.Option(StateOption, "")) }

// save writes the state the user set by hand. Called only from key handling,
// so it costs one tmux call per keypress that changes something, never per
// frame.
func (m Model) save() {
	_ = tmux.SetOption(StateOption, encodeState(persisted{
		selected: m.selectedID(),
		pinned:   m.pinned,
	}))
}
