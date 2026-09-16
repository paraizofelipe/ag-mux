package ui

import (
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/paraizofelipe/ag-mux/internal/agent"
	"github.com/paraizofelipe/ag-mux/internal/tmux"
)

// frameInterval drives the spinner. Scanning is far more expensive — it forks
// a capture-pane per agent — so it runs on its own, slower cadence.
const frameInterval = 150 * time.Millisecond

// Config is what the sidebar needs from tmux options and its own pane.
type Config struct {
	// SessionOverride pins the sidebar to one session. Empty means "whatever
	// session my pane is in right now" — the sidebar follows you between
	// windows, and can be carried into another session, so the answer has to
	// be recomputed rather than captured at startup.
	SessionOverride string
	SelfPane        string
	Interval        time.Duration // scan cadence while an agent is working
}

type mode int

const (
	modeList mode = iota
	modeConfirmKill
	modeNewAgent
)

// Model is the sidebar.
type Model struct {
	cfg    Config
	agents []agent.Agent
	cursor int
	pinned map[string]bool
	mode   mode

	frame    int
	visible  bool   // our window is the one being looked at
	restore  string // agent to put the cursor back on, from a previous run
	lastScan time.Time
	width    int
	height   int
	err      error
	status   string
}

// New builds the sidebar model, picking up the pins and cursor position from
// the last time the sidebar was open.
func New(cfg Config) Model {
	saved := loadState()
	return Model{
		cfg:     cfg,
		pinned:  saved.pinned,
		restore: saved.selected,
		visible: true, // assume so until the first scan says otherwise
		width:   40,
		height:  24,
	}
}

type frameMsg time.Time

type scanMsg struct {
	agents  []agent.Agent
	visible bool // our own window is the one on screen
	err     error
}

func tick() tea.Cmd {
	return tea.Tick(frameInterval, func(t time.Time) tea.Msg { return frameMsg(t) })
}

func (m Model) Init() tea.Cmd { return tea.Batch(tick(), m.scan()) }

// scan reads tmux off the UI goroutine.
func (m Model) scan() tea.Cmd {
	var (
		override = m.cfg.SessionOverride
		self     = m.cfg.SelfPane
	)
	return func() tea.Msg {
		panes, err := tmux.ListAllPanes()
		if err != nil {
			return scanMsg{err: err}
		}
		session := override
		if session == "" {
			for _, p := range panes {
				if p.ID == self {
					session = p.Session
					break
				}
			}
		}
		visible := false
		kept := panes[:0]
		for _, p := range panes {
			if p.ID == self {
				visible = p.WindowAct
				continue
			}
			if p.Session == session {
				kept = append(kept, p)
			}
		}
		return scanMsg{agents: agent.Detect(kept, tmux.CapturePane), visible: visible}
	}
}

func (m Model) selectedID() string {
	if a, ok := m.Selected(); ok {
		return a.Pane.ID
	}
	return ""
}

// Selected is the agent under the cursor.
func (m Model) Selected() (agent.Agent, bool) {
	if m.cursor < 0 || m.cursor >= len(m.agents) {
		return agent.Agent{}, false
	}
	return m.agents[m.cursor], true
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// The sidebar is closed to hide it, so the first size message is also
		// its first paint: scan now rather than waiting for a tick.
		return m, m.scan()

	case frameMsg:
		// Freeze the spinner when nobody is looking: an unchanged view means
		// the renderer writes nothing at all.
		if m.visible {
			m.frame++
		}
		var cmds []tea.Cmd
		cmds = append(cmds, tick())
		if time.Since(m.lastScan) >= m.scanInterval() {
			m.lastScan = time.Now()
			cmds = append(cmds, m.scan())
		}
		return m, tea.Batch(cmds...)

	case scanMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.visible = msg.visible
		m.setAgents(msg.agents)
		return m, nil

	case tea.KeyPressMsg:
		return m.onKey(msg.String())
	}
	return m, nil
}

// scanInterval backs off when nothing is working, so an idle sidebar is not
// forking processes every second for no reason.
//
// The sidebar normally follows you, so it is on screen whenever it exists —
// except when it could not follow, such as into a window with a zoomed pane.
// Polling hard for a screen nobody is looking at is pure waste, so it drops to
// a slow heartbeat, which is also what notices the window became visible again.
func (m Model) scanInterval() time.Duration {
	if !m.visible {
		return 10 * m.cfg.Interval
	}
	for _, a := range m.agents {
		if a.State == agent.StateBusy {
			return m.cfg.Interval
		}
	}
	return 2 * m.cfg.Interval
}

// setAgents replaces the list while keeping the cursor on the same agent and
// carrying pins over.
func (m *Model) setAgents(next []agent.Agent) {
	for i := range next {
		next[i].Pinned = m.pinned[next[i].Pane.ID]
	}
	sort.SliceStable(next, func(i, j int) bool {
		if next[i].Pinned != next[j].Pinned {
			return next[i].Pinned
		}
		if next[i].Pane.WindowIndex != next[j].Pane.WindowIndex {
			return next[i].Pane.WindowIndex < next[j].Pane.WindowIndex
		}
		return next[i].Pane.Index < next[j].Pane.Index
	})

	disambiguate(next)

	prev := m.selectedID()
	if prev == "" {
		prev = m.restore
	}
	m.agents = next
	for i, a := range next {
		if a.Pane.ID == prev {
			m.cursor = i
			return
		}
	}
	m.clampCursor()
}

func (m *Model) clampCursor() {
	if m.cursor >= len(m.agents) {
		m.cursor = len(m.agents) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m Model) onKey(key string) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeConfirmKill:
		m.mode = modeList
		if key != "y" {
			m.status = ""
			return m, nil
		}
		a, ok := m.Selected()
		if !ok {
			return m, nil
		}
		if err := tmux.KillPane(a.Pane.ID); err != nil {
			m.err = err
			return m, nil
		}
		m.status = "encerrado: " + a.Label
		return m, m.scan()

	case modeNewAgent:
		m.mode = modeList
		cmd, ok := map[string]string{"c": "claude", "o": "omp"}[key]
		if !ok {
			m.status = ""
			return m, nil
		}
		dir := "."
		if a, ok := m.Selected(); ok {
			dir = a.Pane.Path
		}
		if _, err := tmux.NewAgent(dir, cmd); err != nil {
			m.err = err
			return m, nil
		}
		m.status = "novo " + cmd
		return m, m.scan()
	}

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.cursor++
		m.clampCursor()
		m.save()
		return m, m.scan()
	case "k", "up":
		m.cursor--
		m.clampCursor()
		m.save()
		return m, m.scan()
	case "g", "home":
		m.cursor = 0
		m.save()
		return m, m.scan()
	case "G", "end":
		m.cursor = len(m.agents) - 1
		m.clampCursor()
		m.save()
		return m, m.scan()
	case "enter":
		if a, ok := m.Selected(); ok {
			if err := tmux.Focus(a.Pane); err != nil {
				m.err = err
			}
		}
		return m, nil
	case "z":
		if a, ok := m.Selected(); ok {
			if err := tmux.Zoom(a.Pane); err != nil {
				m.err = err
			}
		}
		return m, nil
	case "p":
		if a, ok := m.Selected(); ok {
			m.pinned[a.Pane.ID] = !m.pinned[a.Pane.ID]
			m.setAgents(m.agents)
			m.save()
		}
		return m, nil
	case "x":
		if _, ok := m.Selected(); ok {
			m.mode = modeConfirmKill
		}
		return m, nil
	case "n":
		m.mode = modeNewAgent
		return m, nil
	case "r":
		m.lastScan = time.Now()
		return m, m.scan()
	}
	return m, nil
}
