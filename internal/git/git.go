// Package git answers the two questions the sidebar asks about an agent's
// directory: which branch it is on, and whether it is a linked worktree.
//
// Both are cached. Whether a directory is a worktree effectively never
// changes, and a branch changes rarely, while the sidebar rescans every
// second — so without a cache this would cost more than everything else the
// sidebar does put together.
package git

import (
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Info is what the sidebar shows about a repository.
type Info struct {
	Branch   string
	Worktree bool // a linked worktree, not the main checkout
}

// ttl is how long a lookup is trusted. Long enough that the git calls round to
// nothing, short enough that switching branches shows up while you watch.
const ttl = 10 * time.Second

type entry struct {
	info Info
	ok   bool
	at   time.Time
}

var (
	mu    sync.Mutex
	cache = map[string]entry{}
	now   = time.Now // swapped in tests
)

// Lookup returns repository info for a directory. The second result is false
// when the directory is not in a git repository at all.
func Lookup(dir string) (Info, bool) {
	if dir == "" {
		return Info{}, false
	}
	mu.Lock()
	if e, hit := cache[dir]; hit && now().Sub(e.at) < ttl {
		mu.Unlock()
		return e.info, e.ok
	}
	mu.Unlock()

	info, ok := probe(dir)

	mu.Lock()
	cache[dir] = entry{info: info, ok: ok, at: now()}
	mu.Unlock()
	return info, ok
}

// probe asks git everything in one invocation: three lines out, one fork.
func probe(dir string) (Info, bool) {
	out, err := exec.Command("git", "-C", dir, "rev-parse",
		"--abbrev-ref", "HEAD", "--git-dir", "--git-common-dir").Output()
	if err != nil {
		return Info{}, false
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 3 {
		return Info{}, false
	}
	branch := strings.TrimSpace(lines[0])
	if branch == "HEAD" {
		branch = "" // detached; nothing useful to show
	}
	return Info{
		Branch:   branch,
		Worktree: isLinkedWorktree(dir, lines[1], lines[2]),
	}, true
}

// isLinkedWorktree reports whether the checkout is one of `git worktree add`'s
// rather than the main one. In a linked worktree the private git dir sits
// under the shared one (…/.git/worktrees/<name>), so the two paths differ;
// in the main checkout they are the same directory.
func isLinkedWorktree(dir, gitDir, commonDir string) bool {
	resolve := func(p string) string {
		p = strings.TrimSpace(p)
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		return filepath.Clean(p)
	}
	return resolve(gitDir) != resolve(commonDir)
}
