// Package tmux is a thin wrapper over the tmux binary. Every call shells out;
// nothing here caches, so callers decide how often to poll.
package tmux

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// sep separates fields in list-panes output. Titles and paths may contain tabs
// or spaces, so we need a delimiter that cannot show up in real data.
const sep = "\x1fAGMUX\x1f"

// SidebarOption marks the pane that hosts the sidebar, so we can find it again
// from any window and keep it out of the agent list.
const SidebarOption = "@ag-mux-sidebar"

var paneFormat = strings.Join([]string{
	"#{pane_id}",
	"#{window_id}",
	"#{session_name}",
	"#{window_index}",
	"#{window_name}",
	"#{pane_index}",
	"#{pane_current_command}",
	"#{pane_title}",
	"#{pane_current_path}",
	"#{pane_active}",
	"#{window_active}",
	"#{pane_width}",
	"#{pane_height}",
	"#{window_zoomed_flag}",
	"#{" + SidebarOption + "}",
}, sep)

// Pane is one row of list-panes.
type Pane struct {
	ID          string
	WindowID    string
	Session     string
	WindowIndex int
	WindowName  string
	Index       int
	Command     string
	Title       string
	Path        string
	Active      bool
	WindowAct   bool
	Width       int
	Height      int
	WindowZoom  bool
	IsSidebar   bool
}

// Target addresses this pane in later tmux commands.
func (p Pane) Target() string { return p.ID }

func run(args ...string) (string, error) {
	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("tmux %s: %s", strings.Join(args, " "), strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("tmux %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// ListPanes returns every pane in the session. An empty session means the
// session the caller is attached to.
func ListPanes(session string) ([]Pane, error) {
	args := []string{"list-panes", "-s", "-F", paneFormat}
	if session != "" {
		args = append(args, "-t", session)
	}
	return listPanes(args)
}

// ListAllPanes returns every pane on the server, across sessions.
func ListAllPanes() ([]Pane, error) {
	return listPanes([]string{"list-panes", "-a", "-F", paneFormat})
}

func listPanes(args []string) ([]Pane, error) {
	out, err := run(args...)
	if err != nil {
		return nil, err
	}
	var panes []Pane
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		p, err := parsePane(line)
		if err != nil {
			continue // a malformed row should not blank the whole sidebar
		}
		panes = append(panes, p)
	}
	return panes, nil
}

func parsePane(line string) (Pane, error) {
	f := strings.Split(line, sep)
	if len(f) != 15 {
		return Pane{}, fmt.Errorf("expected 15 fields, got %d", len(f))
	}
	atoi := func(s string) int { n, _ := strconv.Atoi(s); return n }
	return Pane{
		ID:          f[0],
		WindowID:    f[1],
		Session:     f[2],
		WindowIndex: atoi(f[3]),
		WindowName:  f[4],
		Index:       atoi(f[5]),
		Command:     f[6],
		Title:       f[7],
		Path:        f[8],
		Active:      f[9] == "1",
		WindowAct:   f[10] == "1",
		Width:       atoi(f[11]),
		Height:      atoi(f[12]),
		WindowZoom:  f[13] == "1",
		IsSidebar:   f[14] == "1",
	}, nil
}

// CapturePane returns the last n lines currently on the pane's screen.
func CapturePane(target string, n int) ([]string, error) {
	out, err := run("capture-pane", "-p", "-t", target, "-S", "-"+strconv.Itoa(n))
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimRight(out, "\n"), "\n"), nil
}

// here resolves a format against the pane this process runs in, not the
// client's active pane. tmux sets TMUX_PANE in every pane it spawns; without
// it, display-message answers for whichever pane happens to be focused — and
// the sidebar is usually not that pane, because you jump away from it.
func here(format string) (string, error) {
	args := []string{"display-message", "-p"}
	if pane := os.Getenv("TMUX_PANE"); pane != "" {
		args = append(args, "-t", pane)
	}
	out, err := run(append(args, format)...)
	return strings.TrimSpace(out), err
}

// CurrentSession is the session this process runs in.
func CurrentSession() (string, error) { return here("#{session_name}") }

// CurrentPane is the pane this process runs in.
func CurrentPane() (string, error) { return here("#{pane_id}") }

// CurrentWindow is the window this process runs in.
func CurrentWindow() (string, error) { return here("#{session_name}:#{window_index}") }

// Focus brings the pane into view: its window first, then the pane itself.
func Focus(p Pane) error {
	// Target the pane id itself: tmux resolves it to the right window
	// regardless of which session or window the caller is sitting in.
	if _, err := run("select-window", "-t", p.ID); err != nil {
		return err
	}
	_, err := run("select-pane", "-t", p.ID)
	return err
}

// Zoom focuses the pane and expands it to fill its window.
func Zoom(p Pane) error {
	if err := Focus(p); err != nil {
		return err
	}
	_, err := run("resize-pane", "-Z", "-t", p.ID)
	return err
}

// KillPane closes a pane and whatever is running in it.
func KillPane(target string) error {
	_, err := run("kill-pane", "-t", target)
	return err
}

// SplitSidebar opens a pane on the left of the window running cmd, marks it as
// the sidebar and returns its id.
func SplitSidebar(window, cmd string, width int) (string, error) {
	out, err := run("split-window", "-h", "-b", "-l", strconv.Itoa(width),
		"-t", window, "-P", "-F", "#{pane_id}", cmd)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(out)
	// Best effort: the sidebar still works unmarked, it just cannot be found
	// again by toggle, so surface nothing if these fail.
	_, _ = run("set-option", "-p", "-t", id, SidebarOption, "1")
	_, _ = run("select-pane", "-t", id, "-T", "ag-mux")
	return id, nil
}

// NewAgent opens a window running cmd in dir and returns its pane id.
func NewAgent(dir, cmd string) (string, error) {
	out, err := run("new-window", "-c", dir, "-P", "-F", "#{pane_id}", cmd)
	return strings.TrimSpace(out), err
}

// Option reads a tmux user option, falling back to def when unset.
func Option(name, def string) string {
	out, err := run("show-option", "-gqv", name)
	if err != nil {
		return def
	}
	if v := strings.TrimSpace(out); v != "" {
		return v
	}
	return def
}

// OptionInt is Option parsed as an int, falling back to def when unparseable.
func OptionInt(name string, def int) int {
	n, err := strconv.Atoi(Option(name, ""))
	if err != nil {
		return def
	}
	return n
}

// Message shows text in the tmux status line.
func Message(text string) error {
	_, err := run("display-message", text)
	return err
}

// ListWindowPanes returns the panes of a single window.
func ListWindowPanes(window string) ([]Pane, error) {
	return listPanes([]string{"list-panes", "-t", window, "-F", paneFormat})
}

// FindSidebar returns the sidebar pane wherever it lives on the server.
func FindSidebar() (Pane, bool, error) {
	panes, err := ListAllPanes()
	if err != nil {
		return Pane{}, false, err
	}
	for _, p := range panes {
		if p.IsSidebar {
			return p, true, nil
		}
	}
	return Pane{}, false, nil
}

// WindowID resolves a window target to its stable @id.
func WindowID(window string) (string, error) {
	out, err := run("display-message", "-p", "-t", window, "#{window_id}")
	return strings.TrimSpace(out), err
}

// SetOption stores a global tmux option. The sidebar uses it to keep the few
// pieces of state that must outlive the process, since hiding the sidebar
// means closing it.
func SetOption(name, value string) error {
	_, err := run("set-option", "-g", name, value)
	return err
}

// JoinPane moves an existing pane into a window, to the left, without giving
// it focus.
func JoinPane(paneID, window string, width int) error {
	_, err := run("join-pane", "-h", "-b", "-d", "-l", strconv.Itoa(width),
		"-s", paneID, "-t", window)
	return err
}

// WindowZoomed reports whether a pane in the window is zoomed to fill it.
func WindowZoomed(window string) bool {
	out, err := run("display-message", "-p", "-t", window, "#{window_zoomed_flag}")
	return err == nil && strings.TrimSpace(out) == "1"
}
