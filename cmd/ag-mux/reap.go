package main

import (
	"github.com/paraizofelipe/ag-mux/internal/tmux"
)

// window is what reap needs to know about one tmux window.
type window struct {
	id       string
	session  string
	real     int // panes that are not the sidebar
	sidebars []string
	active   bool
	zoomed   bool
}

// runReap empties any window left holding nothing but the sidebar.
//
// A window containing only the sidebar is a window tmux keeps alive for no
// reason: you closed your last real pane and it should have gone with it.
//
// The fix is to move the sidebar out, not to kill it. There is only one
// sidebar in the whole server, so killing it there would close it everywhere —
// including the windows you are still working in. Once it leaves, the empty
// window is destroyed by tmux on its own.
//
// Killing is the last resort, for when no window is left to move it to: then
// the session really is over and tmux should exit as it normally would.
func runReap(args []string) error {
	panes, err := tmux.ListAllPanes()
	if err != nil {
		return nil // called from a hook; stay quiet
	}

	windows := map[string]*window{}
	var order []string
	for _, p := range panes {
		w, ok := windows[p.WindowID]
		if !ok {
			w = &window{id: p.WindowID, session: p.Session, active: p.WindowAct, zoomed: p.WindowZoom}
			windows[p.WindowID] = w
			order = append(order, p.WindowID)
		}
		if p.IsSidebar {
			w.sidebars = append(w.sidebars, p.ID)
		} else {
			w.real++
		}
	}

	width := tmux.OptionInt("@ag-mux-width", defaultWidth)
	for _, id := range order {
		w := windows[id]
		if w.real > 0 || len(w.sidebars) == 0 {
			continue
		}
		target := relocationTarget(windows, order, w)
		for _, pane := range w.sidebars {
			if target == "" {
				_ = tmux.KillPane(pane)
				continue
			}
			if err := tmux.JoinPane(pane, target, width); err != nil {
				_ = tmux.KillPane(pane)
			}
		}
	}
	return nil
}

// relocationTarget picks the window to move a stranded sidebar into: one in the
// same session that still has real panes. The window you are looking at wins,
// then any window that is not zoomed, since joining a pane cancels zoom.
// Empty means there is nowhere left to go.
func relocationTarget(windows map[string]*window, order []string, from *window) string {
	var fallback string
	for _, id := range order {
		w := windows[id]
		if w.id == from.id || w.session != from.session || w.real == 0 {
			continue
		}
		if w.active && !w.zoomed {
			return w.id
		}
		if fallback == "" || !w.zoomed {
			fallback = w.id
		}
	}
	return fallback
}
