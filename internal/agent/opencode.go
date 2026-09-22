package agent

import (
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/paraizofelipe/ag-mux/internal/opencode"
	"github.com/paraizofelipe/ag-mux/internal/tmux"
)

// opencodeLookup is a variable so tests can run without a database.
var (
	opencodeLookup  = opencode.Lookup
	opencodeSession = opencode.LookupSession
)

// OpenCode reads opencode panes without looking at one pixel of them.
//
// It can afford that because opencode gives two things Claude Code does not.
// It names itself in the foreground process, so a pane either is an opencode
// or is not — no title to go stale, no footer to match. And it keeps its own
// database, which answers what the session is about and whether the current
// turn is still running.
//
// What the database does not have is any notion of a terminal: a session
// records the directory it was started in and nothing about the process or
// the pane. So two opencode running in the same directory cannot be told
// apart from the outside, and this adapter says so rather than picking one.
type OpenCode struct {
	// shared holds the directories where more than one opencode pane lives.
	shared map[string]bool
}

func (*OpenCode) Name() string { return "opencode" }

// IsCandidate is also the liveness test: the foreground process of the pane is
// opencode itself, which is true exactly while it runs.
func (*OpenCode) IsCandidate(p tmux.Pane) bool { return p.Command == "opencode" }

// NeedsScreen is false, so detection never captures these panes.
func (*OpenCode) NeedsScreen() bool { return false }

// Confirm has nothing left to check: the process is the proof.
func (*OpenCode) Confirm([]string) bool { return true }

// Observe records which directories host more than one opencode, before any
// pane is classified.
func (o *OpenCode) Observe(panes []tmux.Pane) {
	count := map[string]int{}
	for _, p := range panes {
		if !p.IsSidebar && p.Command == "opencode" && p.Path != "" {
			count[p.Path]++
		}
	}
	o.shared = nil
	for dir, n := range count {
		if n < 2 {
			continue
		}
		if o.shared == nil {
			o.shared = map[string]bool{}
		}
		o.shared[dir] = true
	}
}

// status resolves which opencode conversation this pane is running.
//
// With the plugin installed the pane carries the session id, and there is
// nothing left to be ambiguous about: the answer is about this pane rather
// than about the newest session that happens to share its directory. Without
// it, the directory is all there is, and two opencode sharing one is the case
// that cannot be resolved from outside.
func (o *OpenCode) status(p tmux.Pane) (st opencode.Status, ok, ambiguous bool) {
	if p.AgentSession != "" {
		st, ok = opencodeSession(p.AgentSession)
		return st, ok, false
	}
	if o.shared[p.Path] {
		return opencode.Status{}, false, true
	}
	st, ok = opencodeLookup(p.Path, sessionFloor(p))
	return st, ok, false
}

func (o *OpenCode) Classify(p tmux.Pane, _ []string) (State, string, time.Duration) {
	st, ok, ambiguous := o.status(p)
	if ambiguous {
		// Guessing here would show one agent's work under another's name.
		return StateUnknown, "dois opencode neste diretório", 0
	}
	if !ok {
		// Running, but nothing has been asked of it yet, so there is no
		// session to read. That is idle, and it is the truth.
		return StateIdle, "sem sessão ainda", 0
	}
	if st.Working {
		return StateBusy, st.Session.Agent, timeNow().Sub(st.Since).Truncate(time.Second)
	}
	return StateIdle, "", 0
}

// HookAuthoritative is true: the opencode plugin reports permission.asked and
// permission.replied, so it closes every state it opens. Nothing read from the
// database may overrule it — and something would, because a turn paused on a
// permission prompt still looks like a turn in flight.
func (*OpenCode) HookAuthoritative() bool { return true }

// Task is the session title opencode writes for itself. The pane title is no
// help here: opencode sets it once, to "OpenCode", and never changes it.
func (o *OpenCode) Task(p tmux.Pane) string {
	st, ok, ambiguous := o.status(p)
	if ambiguous || !ok {
		return ""
	}
	return st.Session.Title
}

// sessionFloor is the earliest a session may have been touched and still
// belong to this pane's opencode. When the process start cannot be read the
// floor is zero, which accepts any session — degrading to the older, looser
// behaviour rather than losing the agent entirely.
func sessionFloor(p tmux.Pane) time.Time {
	start, ok := processStart(p.PID)
	if !ok {
		return time.Time{}
	}
	return start
}

// Branch defers to git: opencode prints a branch in its footer, but reading it
// there would trade a guaranteed source for a fragile one.
func (*OpenCode) Branch([]string) (string, bool, bool) { return "", false, false }

// processStart is when a pid's process began. It never changes for a given
// pid, so it is cached for the life of the sidebar.
//
// This exists because opencode opens on an empty prompt instead of resuming:
// without knowing when the process started, a directory you worked in last
// week would hand the sidebar that old session's title as today's task.
var (
	startMu    sync.Mutex
	startCache = map[int]time.Time{}
)

func processStart(pid int) (time.Time, bool) {
	if pid <= 0 {
		return time.Time{}, false
	}
	startMu.Lock()
	defer startMu.Unlock()
	if t, hit := startCache[pid]; hit {
		return t, true
	}
	// etime is [[DD-]HH:]MM:SS and, unlike lstart, has no locale in it.
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "etime=").Output()
	if err != nil {
		return time.Time{}, false
	}
	age, ok := parseETime(strings.TrimSpace(string(out)))
	if !ok {
		return time.Time{}, false
	}
	t := timeNow().Add(-age)
	startCache[pid] = t
	return t, true
}

func parseETime(s string) (time.Duration, bool) {
	if s == "" {
		return 0, false
	}
	days := 0
	if i := strings.IndexByte(s, '-'); i >= 0 {
		n, err := strconv.Atoi(s[:i])
		if err != nil {
			return 0, false
		}
		days, s = n, s[i+1:]
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	var units [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0, false
		}
		units[len(units)-len(parts)+i] = n
	}
	return time.Duration(days)*24*time.Hour +
		time.Duration(units[0])*time.Hour +
		time.Duration(units[1])*time.Minute +
		time.Duration(units[2])*time.Second, true
}
