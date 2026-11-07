// Command lazymux is a lazygit-style TUI for tmux sessions, windows, panes
// and the AI coding agents running in them.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/afamrk/lazymux/internal/agent"
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
	args := flag.Args()
	if len(args) > 0 {
		switch args[0] {
		case "sidebar":
			sidebar(c, args[1:])
			return
		case "hook": // called by Claude Code hooks; must stay quiet and fast
			agent.RunHook(os.Stdin)
			return
		case "hooks":
			hooks(args[1:])
			return
		}
	}
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

// sidebar handles `lazymux sidebar [toggle|follow <session>|off]`. Without a
// subcommand it runs the UI inside the sidebar pane.
func sidebar(c *tmux.Client, args []string) {
	var err error
	switch {
	case len(args) == 0:
		m := ui.NewSidebar(c)
		_, err = tea.NewProgram(m, tea.WithAltScreen(), tea.WithReportFocus()).Run()
	case args[0] == "toggle":
		err = c.SidebarToggle()
	case args[0] == "off":
		err = c.SidebarOff()
	case args[0] == "follow" && len(args) == 2:
		err = c.SidebarFollow(args[1])
	default:
		err = fmt.Errorf("usage: lazymux sidebar [toggle | off | follow <session-id>]")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "lazymux:", err)
		os.Exit(1)
	}
}

// hooks handles `lazymux hooks [install | uninstall | status] [settings.json]`:
// it adds lazymux to Claude Code's hooks so agents report their prompt, tool
// calls and permission requests (the Agents page Activity panel).
func hooks(args []string) {
	usage := "usage: lazymux hooks [install | uninstall | status] [path/to/claude/settings.json]"
	if len(args) == 0 || len(args) > 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	path := agent.ClaudeSettings()
	if len(args) == 2 {
		path = args[1]
	}
	exe, err := os.Executable()
	if err != nil {
		exe = "lazymux"
	}
	command := "'" + strings.ReplaceAll(exe, "'", `'\''`) + "' hook"
	var changed bool
	switch args[0] {
	case "install":
		changed, err = agent.InstallHooks(path, command)
		if err == nil {
			if changed {
				fmt.Printf("added lazymux hooks to %s (backup: %s.lazymux.bak)\n", path, path)
				fmt.Println("hook command:", command)
				fmt.Println("restart running Claude Code sessions to pick them up")
			} else {
				fmt.Println("lazymux hooks are already installed in", path)
			}
		}
	case "uninstall":
		changed, err = agent.UninstallHooks(path)
		if err == nil {
			if changed {
				fmt.Println("removed lazymux hooks from", path)
			} else {
				fmt.Println("no lazymux hooks in", path)
			}
		}
	case "status":
		if agent.HooksInstalled(path) {
			fmt.Println("installed in", path)
		} else {
			fmt.Println("not installed in", path, "(run: lazymux hooks install)")
		}
		fmt.Println("events are stored in", agent.EventDir())
	default:
		err = fmt.Errorf("%s", usage)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "lazymux:", err)
		os.Exit(1)
	}
}
