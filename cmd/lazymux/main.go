// Command lazymux is a lazygit-style TUI for tmux sessions, windows, panes
// and the AI coding agents running in them.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/afamrk/lazymux/internal/tmux"
	"github.com/afamrk/lazymux/internal/ui"
)

var version = "0.1.0-dev"

func main() {
	var (
		socket  = flag.String("L", "", "tmux server socket name (like tmux -L)")
		client  = flag.String("client", "", "client tty to act on, e.g. '#{client_tty}' from a key binding")
		showVer = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()
	if *showVer {
		fmt.Println("lazymux", version)
		return
	}

	c := &tmux.Client{Socket: *socket, ClientTTY: *client}
	m := ui.New(c)
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "lazymux:", err)
		os.Exit(1)
	}

	// Outside tmux, picking a session means attaching this terminal to it.
	if m.AttachTo != nil {
		argv := c.AttachArgs(m.AttachTo)
		bin, err := exec.LookPath(argv[0])
		if err == nil {
			err = syscall.Exec(bin, argv, os.Environ())
		}
		fmt.Fprintln(os.Stderr, "lazymux: attach:", err)
		os.Exit(1)
	}
}
