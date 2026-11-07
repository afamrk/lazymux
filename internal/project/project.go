// Package project lists project directories for the project picker (N):
// zoxide's directories first (most used first), then git repositories under
// the configured roots.
package project

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DefaultRoots are searched for git repositories when no roots are set
// (those that exist).
var DefaultRoots = []string{"~/project", "~/projects", "~/src", "~/code", "~/work", "~/dev"}

// DefaultDepth is how deep under a root a repository may be.
const DefaultDepth = 3

// Options configure List.
type Options struct {
	Roots  []string // directories searched for git repositories (~ allowed)
	Depth  int      // how deep under a root (default DefaultDepth)
	Zoxide bool     // include zoxide's directories
}

// List returns project directories: zoxide's in its order, then the git
// repositories under the roots, sorted; each directory once, only ones that
// exist. It runs external commands with a timeout.
func List(o Options) []string {
	if o.Depth <= 0 {
		o.Depth = DefaultDepth
	}
	var out []string
	seen := map[string]bool{}
	add := func(d string) {
		d = filepath.Clean(d)
		if seen[d] {
			return
		}
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			return
		}
		seen[d] = true
		out = append(out, d)
	}
	home, _ := os.UserHomeDir()
	if o.Zoxide {
		for _, d := range zoxide() {
			if d != home && d != "/" {
				add(d)
			}
		}
	}
	var repos []string
	for _, r := range o.Roots {
		repos = append(repos, gitRepos(ExpandHome(r), o.Depth)...)
	}
	sort.Strings(repos)
	for _, d := range repos {
		add(d)
	}
	return out
}

// ExpandHome turns a leading ~ into the home directory.
func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return home + p[1:]
		}
	}
	return p
}

func run(name string, args ...string) string {
	bin, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, bin, args...).Output()
	return string(out)
}

func zoxide() []string {
	return lines(run("zoxide", "query", "--list"))
}

// gitRepos finds directories holding a .git under root, at most depth
// levels down: with fd when it is installed, else by walking.
func gitRepos(root string, depth int) []string {
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return nil
	}
	for _, fd := range []string{"fd", "fdfind"} {
		if _, err := exec.LookPath(fd); err != nil {
			continue
		}
		var out []string
		for _, l := range lines(run(fd, "--hidden", "--no-ignore", "--max-depth", strconv.Itoa(depth+1),
			"--glob", ".git", "--absolute-path", root)) {
			out = append(out, filepath.Dir(strings.TrimSuffix(l, "/")))
		}
		return out
	}
	var out []string
	base := strings.Count(filepath.Clean(root), string(os.PathSeparator))
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if strings.Count(p, string(os.PathSeparator))-base > depth {
			return filepath.SkipDir
		}
		if d.Name() == ".git" {
			out = append(out, filepath.Dir(p))
			return filepath.SkipDir
		}
		if d.Name() == "node_modules" || (strings.HasPrefix(d.Name(), ".") && p != root) {
			return filepath.SkipDir
		}
		return nil
	})
	return out
}

func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// SessionName is the tmux session name for a directory: its base name,
// with the characters tmux does not allow in names (. and :) replaced.
func SessionName(dir string) string {
	n := filepath.Base(filepath.Clean(dir))
	return strings.NewReplacer(".", "_", ":", "_").Replace(n)
}
