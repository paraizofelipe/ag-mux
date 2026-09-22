// Package opencode reads what an opencode agent is doing from opencode's own
// database, instead of from the pane's screen.
//
// opencode keeps every session in a SQLite file it holds open while running.
// That file answers what the screen can only be asked to imply: the session
// title, whether the current turn is still in flight, and since when. None of
// it moves when opencode redraws its interface.
//
// Reading is done through the sqlite3 binary rather than a driver, to match
// internal/tmux and internal/git — the whole project talks to tools it does
// not link against — and to keep a cgo-free build with no heavyweight
// dependency for one read-only query.
package opencode

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Session is one opencode conversation.
type Session struct {
	ID    string
	Title string // what the session is about; opencode writes it itself
	Agent string // "build", "plan", ...; empty on sessions from older versions
}

// Status is what an opencode process in a directory is doing right now.
type Status struct {
	Session Session
	Working bool
	Since   time.Time // when the in-flight turn started
}

// staleAfter is how long an unfinished turn may go without being touched
// before we stop calling it work.
//
// A turn that is really running rewrites its message row continuously — in the
// sessions on hand, every single assistant message has time_updated after
// time_created. A turn that was interrupted stops being written and keeps its
// opening timestamp forever, and without this guard such a row would read as
// an agent that has been working for weeks.
const staleAfter = 2 * time.Minute

// ttl is how long a lookup is trusted, mirroring internal/git: long enough
// that the sidebar's polling does not turn into a fork per agent per second,
// short enough that a finished turn is noticed as it happens.
const ttl = 2 * time.Second

var (
	mu    sync.Mutex
	cache = map[string]entry{}
	now   = time.Now // swapped in tests

	// dbPath is a variable so tests can point at a database they built.
	dbPath = defaultDBPath
)

type entry struct {
	status Status
	ok     bool
	at     time.Time
}

func defaultDBPath() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "opencode", "opencode.db")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "opencode", "opencode.db")
}

// Lookup returns the session opencode is working in inside a directory.
//
// notBefore discards sessions older than the process asking about them.
// opencode opens on an empty prompt — it does not resume the last
// conversation — so in a directory you have used before, the newest stored
// session belongs to a run that already ended. Without this the sidebar would
// label a fresh agent with last week's task and never look wrong doing it.
// Pass the zero time to accept any session.
//
// The second result is false when there is nothing to report: no database, no
// sqlite3, or no session in that directory since the process started — which
// is the normal state of an opencode opened and not yet asked anything.
func Lookup(dir string, notBefore time.Time) (Status, bool) {
	if dir == "" {
		return Status{}, false
	}
	key := dir + "\x00" + notBefore.UTC().Format(time.RFC3339)
	mu.Lock()
	defer mu.Unlock()
	if e, hit := cache[key]; hit && now().Sub(e.at) < ttl {
		return e.status, e.ok
	}
	st, ok := probe(dir, notBefore)
	cache[key] = entry{status: st, ok: ok, at: now()}
	return st, ok
}

// row is one line of the query's JSON output.
type row struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Agent   string `json:"agent"`
	Last    string `json:"last"`    // the newest message's JSON, "" when none
	Touched int64  `json:"touched"` // that message's time_updated, epoch ms
	Updated int64  `json:"updated"` // the session's time_updated, epoch ms
}

// message is the shape we need out of a message's stored JSON. An assistant
// message carries time.completed once its turn ends; while the turn runs the
// field is simply absent, which is the signal this package exists to read.
type message struct {
	Role string `json:"role"`
	Time struct {
		Created   int64 `json:"created"`
		Completed int64 `json:"completed"`
	} `json:"time"`
}

const query = `SELECT s.id AS id,
       s.title AS title,
       coalesce(s.agent, '') AS agent,
       coalesce((SELECT m.data FROM message m
                  WHERE m.session_id = s.id
                  ORDER BY m.time_created DESC LIMIT 1), '') AS last,
       coalesce((SELECT m.time_updated FROM message m
                  WHERE m.session_id = s.id
                  ORDER BY m.time_created DESC LIMIT 1), 0) AS touched,
       s.time_updated AS updated
  FROM session s
 WHERE s.directory IN (%s)
 ORDER BY s.time_updated DESC
 LIMIT 1;`

func probe(dir string, notBefore time.Time) (Status, bool) {
	path := dbPath()
	if path == "" {
		return Status{}, false
	}
	if _, err := os.Stat(path); err != nil {
		return Status{}, false
	}
	// Match the directory as tmux reports it and as the filesystem resolves
	// it. A project reached through a symlink is recorded by opencode under
	// whichever form it saw, and a mismatch would not look like an error —
	// the agent would simply show up with no task, forever.
	var lits []string
	for _, d := range candidates(dir) {
		lits = append(lits, "'"+strings.ReplaceAll(d, "'", "''")+"'")
	}
	// Open read-only so a running opencode is never blocked or altered.
	sql := strings.Replace(query, "%s", strings.Join(lits, ", "), 1)
	out, err := exec.Command("sqlite3", "-json", "-readonly", path, sql).Output()
	if err != nil {
		return Status{}, false
	}
	var rows []row
	if err := json.Unmarshal(out, &rows); err != nil || len(rows) == 0 {
		return Status{}, false
	}
	r := rows[0]
	if !notBefore.IsZero() && millis(r.Updated).Before(notBefore) {
		return Status{}, false // a conversation from a run that already ended
	}

	st := Status{Session: Session{ID: r.ID, Title: r.Title, Agent: r.Agent}}
	if r.Last == "" {
		return st, true
	}
	var m message
	if err := json.Unmarshal([]byte(r.Last), &m); err != nil {
		return st, true
	}
	if m.Role != "assistant" || m.Time.Completed != 0 || m.Time.Created == 0 {
		return st, true
	}
	if now().Sub(millis(r.Touched)) > staleAfter {
		return st, true // the turn stopped being written; it was interrupted
	}
	st.Working = true
	st.Since = millis(m.Time.Created)
	return st, true
}

// candidates is the directory as given plus its symlink-resolved form, when
// they differ.
func candidates(dir string) []string {
	out := []string{dir}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil && resolved != dir {
		out = append(out, resolved)
	}
	return out
}

func millis(ms int64) time.Time { return time.Unix(ms/1000, (ms%1000)*1e6) }
