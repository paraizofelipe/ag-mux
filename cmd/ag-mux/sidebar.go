package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/paraizofelipe/ag-mux/internal/tmux"
	"github.com/paraizofelipe/ag-mux/internal/ui"
)

// Defaults for the tmux user options the plugin reads.
const (
	defaultWidth      = 40
	defaultIntervalMS = 1000
)

func runSidebar(args []string) error {
	fs := flag.NewFlagSet("sidebar", flag.ExitOnError)
	override := fs.String("session", "", "lista os agentes desta sessão em vez da atual")
	if err := fs.Parse(args); err != nil {
		return err
	}

	self, err := tmux.CurrentPane()
	if err != nil {
		return fmt.Errorf("a sidebar precisa rodar dentro do tmux: %w", err)
	}

	cfg := ui.Config{
		SessionOverride: *override,
		SelfPane:        self,
		Interval:        time.Duration(tmux.OptionInt("@ag-mux-interval", defaultIntervalMS)) * time.Millisecond,
	}
	_, err = tea.NewProgram(ui.New(cfg)).Run()
	return err
}

// runToggle shows or hides the sidebar.
//
// "Hide" closes the pane rather than parking it somewhere: the pins and the
// cursor live in a tmux option, so a fresh sidebar comes back exactly as you
// left it, and nothing is left running or cluttering the session list.
func runToggle(args []string) error {
	fs := flag.NewFlagSet("toggle", flag.ExitOnError)
	// The after-new-window hook has to name its window: the hook can fire
	// before the new window becomes the active one, and the toggle would then
	// open the sidebar in whichever window the client is still showing.
	target := fs.String("window", "", "janela alvo (default: a janela atual)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	window := *target
	var err error
	if window == "" {
		if window, err = tmux.CurrentWindow(); err != nil {
			return fmt.Errorf("o toggle precisa rodar dentro do tmux: %w", err)
		}
	}

	sb, found, err := tmux.FindSidebar()
	if err != nil {
		return err
	}
	if found {
		here, err := tmux.WindowID(window)
		if err != nil {
			return err
		}
		if sb.WindowID == here {
			return tmux.KillPane(sb.ID) // showing here: hide it
		}
		// Open in another window: close it there so it follows you here,
		// rather than leaving two of them behind.
		if err := tmux.KillPane(sb.ID); err != nil {
			return err
		}
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	width := tmux.OptionInt("@ag-mux-width", defaultWidth)
	// exec keeps the pane from outliving the sidebar as a stray shell.
	_, err = tmux.SplitSidebar(window, "exec "+strconv.Quote(exe)+" sidebar", width)
	return err
}
