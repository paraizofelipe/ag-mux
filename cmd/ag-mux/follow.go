package main

import (
	"flag"

	"github.com/paraizofelipe/ag-mux/internal/tmux"
)

// runFollow moves the sidebar into the window that just became current.
//
// A tmux pane belongs to exactly one window — join-pane moves a pane, it never
// copies one, and there is no shared-pane concept. So rather than a sidebar
// per window, the single sidebar migrates to whichever window you are looking
// at: one process, and nothing polling in windows you cannot see.
//
// This runs from a hook on every window switch, so failures stay quiet. A
// wrong guess here would pop a tmux error message every time you press a key.
func runFollow(args []string) error {
	fs := flag.NewFlagSet("follow", flag.ExitOnError)
	target := fs.String("window", "", "janela que passou a ser a atual")
	if err := fs.Parse(args); err != nil {
		return nil
	}
	if tmux.Option("@ag-mux-follow", "on") != "on" {
		return nil
	}

	sb, found, err := tmux.FindSidebar()
	if err != nil || !found {
		return nil // hidden or never opened: nothing to follow with
	}

	window := *target
	if window == "" {
		if window, err = tmux.CurrentWindow(); err != nil {
			return nil
		}
	}
	here, err := tmux.WindowID(window)
	if err != nil || sb.WindowID == here {
		return nil // already showing here
	}
	// Joining a pane into a window undoes its zoom. Zoom means "give this one
	// pane the whole window", so following into it would silently take away
	// what you asked for. Skip; the toggle still brings the sidebar in when
	// you actually want it.
	if tmux.WindowZoomed(window) {
		return nil
	}
	_ = tmux.JoinPane(sb.ID, window, tmux.OptionInt("@ag-mux-width", defaultWidth))
	return nil
}
