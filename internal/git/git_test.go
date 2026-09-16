package git

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// repoWithWorktree builds a real repository and a linked worktree, because
// worktree detection is exactly the thing a fake would get wrong.
func repoWithWorktree(t *testing.T) (main, linked string) {
	t.Helper()
	root := t.TempDir()
	main = filepath.Join(root, "main")
	linked = filepath.Join(root, "linked")

	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(cmd.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run(root, "init", "-q", "-b", "principal", main)
	run(main, "commit", "-q", "--allow-empty", "-m", "inicial")
	run(main, "worktree", "add", "-q", "-b", "recurso", linked)
	return main, linked
}

func TestLookup(t *testing.T) {
	main, linked := repoWithWorktree(t)
	reset()

	t.Run("main checkout", func(t *testing.T) {
		got, ok := Lookup(main)
		if !ok {
			t.Fatal("esperava um repositório")
		}
		if got.Branch != "principal" {
			t.Errorf("Branch = %q, want %q", got.Branch, "principal")
		}
		if got.Worktree {
			t.Error("o checkout principal foi marcado como worktree")
		}
	})

	t.Run("linked worktree", func(t *testing.T) {
		got, ok := Lookup(linked)
		if !ok {
			t.Fatal("esperava um repositório")
		}
		if got.Branch != "recurso" {
			t.Errorf("Branch = %q, want %q", got.Branch, "recurso")
		}
		if !got.Worktree {
			t.Error("o worktree ligado não foi reconhecido")
		}
	})

	t.Run("not a repository", func(t *testing.T) {
		if _, ok := Lookup(t.TempDir()); ok {
			t.Error("um diretório fora de repositório passou por repositório")
		}
	})

	t.Run("empty path", func(t *testing.T) {
		if _, ok := Lookup(""); ok {
			t.Error("caminho vazio devia falhar")
		}
	})
}

// The cache is the whole reason this package exists: without it the sidebar
// would fork git for every agent on every scan.
func TestLookupCaches(t *testing.T) {
	main, _ := repoWithWorktree(t)
	reset()

	first, _ := Lookup(main)
	// Move the branch under git's feet; a cached answer must not notice yet.
	cmd := exec.Command("git", "checkout", "-q", "-b", "outra")
	cmd.Dir = main
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("checkout: %v\n%s", err, out)
	}

	cached, _ := Lookup(main)
	if cached.Branch != first.Branch {
		t.Errorf("cache não segurou: %q -> %q", first.Branch, cached.Branch)
	}

	// Past the TTL it must catch up.
	now = func() time.Time { return time.Now().Add(2 * ttl) }
	defer func() { now = time.Now }()
	if fresh, _ := Lookup(main); fresh.Branch != "outra" {
		t.Errorf("depois do TTL Branch = %q, want %q", fresh.Branch, "outra")
	}
}

func reset() {
	mu.Lock()
	cache = map[string]entry{}
	mu.Unlock()
	now = time.Now
}
