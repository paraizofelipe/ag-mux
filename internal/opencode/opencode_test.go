package opencode

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newDB builds a real opencode database with the columns this package reads,
// so the SQL is exercised rather than mocked. A fake would have agreed with
// whatever query I wrote.
func newDB(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 não está instalado")
	}
	path := filepath.Join(t.TempDir(), "opencode.db")
	run(t, path, `
CREATE TABLE session (id text PRIMARY KEY, directory text NOT NULL,
  title text NOT NULL, agent text, time_updated integer NOT NULL);
CREATE TABLE message (id text PRIMARY KEY, session_id text NOT NULL,
  time_created integer NOT NULL, time_updated integer NOT NULL, data text NOT NULL);`)
	return path
}

func run(t *testing.T, path, sql string) {
	t.Helper()
	cmd := exec.Command("sqlite3", path)
	cmd.Stdin = strings.NewReader(sql)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sqlite3: %v\n%s", err, out)
	}
}

func ms(tm time.Time) int64 { return tm.UnixMilli() }

func addSession(t *testing.T, path, id, dir, title, agent string, updated time.Time) {
	t.Helper()
	run(t, path, fmt.Sprintf(
		`INSERT INTO session VALUES ('%s','%s','%s','%s',%d);`,
		id, dir, title, agent, ms(updated)))
}

// addMessage writes an assistant message. A zero completed time means the turn
// is still in flight, which is exactly how opencode stores it.
func addMessage(t *testing.T, path, id, session string, created, touched time.Time, completed time.Time) {
	t.Helper()
	data := fmt.Sprintf(`{"role":"assistant","time":{"created":%d}}`, ms(created))
	if !completed.IsZero() {
		data = fmt.Sprintf(`{"role":"assistant","time":{"created":%d,"completed":%d}}`,
			ms(created), ms(completed))
	}
	run(t, path, fmt.Sprintf(`INSERT INTO message VALUES ('%s','%s',%d,%d,'%s');`,
		id, session, ms(created), ms(touched), data))
}

func useDB(t *testing.T, path string, clock time.Time) {
	t.Helper()
	prevPath, prevNow := dbPath, now
	dbPath = func() string { return path }
	now = func() time.Time { return clock }
	cache = map[string]entry{}
	t.Cleanup(func() {
		dbPath, now = prevPath, prevNow
		cache = map[string]entry{}
	})
}

func TestLookup(t *testing.T) {
	clock := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	db := newDB(t)

	// Finished its turn.
	addSession(t, db, "ses_done", "/w/calmo", "ajustar exportacao", "build", clock.Add(-time.Hour))
	addMessage(t, db, "m1", "ses_done", clock.Add(-70*time.Minute), clock.Add(-65*time.Minute), clock.Add(-65*time.Minute))

	// Turn in flight: no completed, and still being written.
	addSession(t, db, "ses_live", "/w/ocupado", "implementar filtros", "build", clock.Add(-time.Minute))
	addMessage(t, db, "m2", "ses_live", clock.Add(-8*time.Minute), clock.Add(-10*time.Second), time.Time{})

	// Interrupted weeks ago: no completed either, but nothing has touched it
	// since. This is the row that would otherwise read as "working for weeks".
	addSession(t, db, "ses_orfa", "/w/abandonado", "revisar estrutura", "", clock.Add(-500*time.Hour))
	addMessage(t, db, "m3", "ses_orfa", clock.Add(-500*time.Hour), clock.Add(-500*time.Hour), time.Time{})

	useDB(t, db, clock)

	t.Run("turno terminado", func(t *testing.T) {
		st, ok := Lookup("/w/calmo", time.Time{})
		if !ok {
			t.Fatal("não achou a sessão")
		}
		if st.Working {
			t.Error("disse que está trabalhando com o turno fechado")
		}
		if st.Session.Title != "ajustar exportacao" || st.Session.Agent != "build" {
			t.Errorf("sessão = %+v", st.Session)
		}
	})

	t.Run("turno em voo", func(t *testing.T) {
		st, ok := Lookup("/w/ocupado", time.Time{})
		if !ok {
			t.Fatal("não achou a sessão")
		}
		if !st.Working {
			t.Fatal("turno sem completed deveria contar como trabalhando")
		}
		if got := clock.Sub(st.Since); got != 8*time.Minute {
			t.Errorf("trabalhando há %v, queria 8m", got)
		}
	})

	t.Run("turno interrompido não conta", func(t *testing.T) {
		st, ok := Lookup("/w/abandonado", time.Time{})
		if !ok {
			t.Fatal("não achou a sessão")
		}
		if st.Working {
			t.Errorf("turno largado há 500h virou trabalho (desde %v)", st.Since)
		}
		if st.Session.Title != "revisar estrutura" {
			t.Errorf("perdeu o título: %q", st.Session.Title)
		}
	})

	t.Run("diretório sem sessão", func(t *testing.T) {
		if _, ok := Lookup("/w/nunca-usado", time.Time{}); ok {
			t.Error("inventou uma sessão para um diretório sem nenhuma")
		}
	})

	t.Run("diretório vazio", func(t *testing.T) {
		if _, ok := Lookup("", time.Time{}); ok {
			t.Error("aceitou diretório vazio")
		}
	})
}

// With several sessions in one directory the newest is the live one; the older
// ones are previous conversations in the same project.
func TestLookupPicksNewest(t *testing.T) {
	clock := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	db := newDB(t)
	addSession(t, db, "ses_velha", "/w/proj", "conversa antiga", "build", clock.Add(-48*time.Hour))
	addSession(t, db, "ses_nova", "/w/proj", "conversa de agora", "build", clock.Add(-time.Minute))
	useDB(t, db, clock)

	st, ok := Lookup("/w/proj", time.Time{})
	if !ok {
		t.Fatal("não achou sessão")
	}
	if st.Session.ID != "ses_nova" {
		t.Errorf("escolheu %q, queria a mais recente", st.Session.ID)
	}
}

// A directory with a quote in it must be read, not break the statement.
func TestLookupQuotedDirectory(t *testing.T) {
	clock := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	db := newDB(t)
	dir := "/w/de o'brien"
	run(t, db, fmt.Sprintf(`INSERT INTO session VALUES ('ses_q','%s','tarefa','build',%d);`,
		strings.ReplaceAll(dir, "'", "''"), ms(clock)))
	useDB(t, db, clock)

	st, ok := Lookup(dir, time.Time{})
	if !ok {
		t.Fatal("aspas no caminho quebraram a consulta")
	}
	if st.Session.ID != "ses_q" {
		t.Errorf("sessão = %q", st.Session.ID)
	}
}

// The cache is what keeps the sidebar from forking sqlite3 once per agent per
// scan; it also has to let go.
func TestLookupCache(t *testing.T) {
	clock := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	db := newDB(t)
	addSession(t, db, "ses_a", "/w/p", "primeiro titulo", "build", clock)
	useDB(t, db, clock)

	if st, _ := Lookup("/w/p", time.Time{}); st.Session.Title != "primeiro titulo" {
		t.Fatalf("título = %q", st.Session.Title)
	}
	run(t, db, `UPDATE session SET title='segundo titulo' WHERE id='ses_a';`)

	if st, _ := Lookup("/w/p", time.Time{}); st.Session.Title != "primeiro titulo" {
		t.Errorf("dentro do TTL devia servir do cache, veio %q", st.Session.Title)
	}
	now = func() time.Time { return clock.Add(ttl + time.Second) }
	if st, _ := Lookup("/w/p", time.Time{}); st.Session.Title != "segundo titulo" {
		t.Errorf("depois do TTL devia reconsultar, veio %q", st.Session.Title)
	}
}

// A project reached through a symlink is recorded by opencode under whichever
// form it saw. Missing it would not look like a failure — the agent would just
// never have a task.
func TestLookupFollowsSymlink(t *testing.T) {
	clock := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	db := newDB(t)

	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "atalho")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("sem symlink neste sistema: %v", err)
	}
	// opencode grava o caminho já resolvido — foi o que ele registrou no
	// próprio log. A sidebar vai perguntar pelo link.
	stored, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatalf("resolver %s: %v", real, err)
	}
	addSession(t, db, "ses_l", stored, "implementar filtros", "build", clock)
	useDB(t, db, clock)

	st, ok := Lookup(link, time.Time{})
	if !ok {
		t.Fatal("não achou a sessão pelo caminho com symlink")
	}
	if st.Session.Title != "implementar filtros" {
		t.Errorf("título = %q", st.Session.Title)
	}
}

// opencode opens on an empty prompt instead of resuming. So in a directory you
// worked in before, the newest stored session belongs to a run that already
// ended, and handing it over would label a fresh agent with last week's task —
// wrong, and never looking wrong.
func TestLookupIgnoresSessionsOlderThanTheProcess(t *testing.T) {
	clock := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	started := clock.Add(-5 * time.Minute)
	db := newDB(t)
	addSession(t, db, "ses_ontem", "/w/proj", "conversa de ontem", "build", clock.Add(-24*time.Hour))
	useDB(t, db, clock)

	if st, ok := Lookup("/w/proj", started); ok {
		t.Errorf("entregou uma sessão de antes do processo: %q", st.Session.Title)
	}
	// A session touched after the process started is this run's.
	addSession(t, db, "ses_agora", "/w/proj", "conversa de agora", "build", clock.Add(-time.Minute))
	cache = map[string]entry{}
	st, ok := Lookup("/w/proj", started)
	if !ok {
		t.Fatal("descartou a sessão desta execução")
	}
	if st.Session.Title != "conversa de agora" {
		t.Errorf("título = %q", st.Session.Title)
	}
}
