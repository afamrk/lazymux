package agent

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// KindOf maps a pane's current command (tmux pane_current_command) to an
// agent kind, or "" when the command is not an agent itself.
func KindOf(command string) Kind {
	for _, k := range Kinds {
		if command == string(k) {
			return k
		}
	}
	return ""
}

// interpreters are commands that may be running an agent script (gemini and
// codex installed with npm show up as "node"); only panes running one of
// these need a look at the process tree.
func isInterpreter(command string) bool {
	switch {
	case command == "node", command == "nodejs", command == "bun", command == "deno",
		command == "npx", command == "pnpm", command == "uv", command == "uvx",
		strings.HasPrefix(command, "python"):
		return true
	}
	// Claude Code's native build may report its version ("2.0.31") as name
	_, err := strconv.Atoi(strings.ReplaceAll(command, ".", ""))
	return err == nil && strings.Contains(command, ".")
}

// kindFromArgs finds an agent in a process command line, e.g.
// "node /usr/lib/node_modules/@google/gemini-cli/dist/index.js".
func kindFromArgs(args string) Kind {
	f := strings.Fields(args)
	for i, a := range f {
		if i > 2 {
			break
		}
		low := strings.ToLower(a)
		switch {
		case strings.Contains(low, "@anthropic-ai/claude-code"), strings.Contains(low, "/claude/versions/"):
			return Claude
		case strings.Contains(low, "@google/gemini-cli"):
			return Gemini
		case strings.Contains(low, "@openai/codex"):
			return Codex
		case strings.Contains(low, "opencode-ai"):
			return Opencode
		}
		base := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(a), ".js"), ".mjs")
		if k := KindOf(base); k != "" {
			return k
		}
	}
	return ""
}

type proc struct {
	ppid int
	args string
}

// processes lists every process (ps works the same on Linux and macOS).
func processes() map[int]proc {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,args=").Output()
	if err != nil {
		return nil
	}
	procs := map[int]proc{}
	for _, l := range strings.Split(string(out), "\n") {
		f := strings.Fields(l)
		if len(f) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil {
			continue
		}
		procs[pid] = proc{ppid, strings.Join(f[2:], " ")}
	}
	return procs
}

// kindUnder looks for an agent among the descendants of pid (the pane's
// shell) in procs.
func kindUnder(pid int, procs map[int]proc) Kind {
	children := map[int][]int{}
	for p, x := range procs {
		children[x.ppid] = append(children[x.ppid], p)
	}
	seen := map[int]bool{}
	queue := []int{pid}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		if seen[p] {
			continue
		}
		seen[p] = true
		if k := kindFromArgs(procs[p].args); k != "" {
			return k
		}
		queue = append(queue, children[p]...)
	}
	return ""
}
