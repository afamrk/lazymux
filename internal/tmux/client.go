// Package tmux wraps the tmux CLI: querying the session/window/pane tree,
// capturing pane contents and running management commands.
package tmux

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Client runs tmux commands, optionally against a non-default server socket
// (-L name), and optionally pinned to a specific client tty for commands that
// act on "the current client" (switch-client).
type Client struct {
	Socket    string // tmux -L <socket>; empty = default server
	ClientTTY string // e.g. /dev/pts/3; empty = tmux picks the most recent client
}

// Run executes `tmux <args...>` and returns trimmed stdout.
func (c *Client) Run(args ...string) (string, error) {
	full := c.baseArgs(args...)
	cmd := exec.Command("tmux", full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("tmux %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}

// Command returns the argv (including "tmux") for running args against this
// client's server; used when lazymux execs into `tmux attach`.
func (c *Client) Command(args ...string) []string {
	return append([]string{"tmux"}, c.baseArgs(args...)...)
}

func (c *Client) baseArgs(args ...string) []string {
	if c.Socket == "" {
		return args
	}
	return append([]string{"-L", c.Socket}, args...)
}

// InsideTmux reports whether lazymux itself runs inside a tmux client
// (directly or in a display-popup).
func InsideTmux() bool { return os.Getenv("TMUX") != "" }
