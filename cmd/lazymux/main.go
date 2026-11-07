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
			agent.RunHook(args[1:], os.Stdin, os.Stdout)
			return
		case "hooks":
			hooks(args[1:])
			return
		case "status": // for tmux's status-right: #(lazymux status)
			plain := len(args) > 1 && args[1] == "--plain"
			fmt.Println(agent.FormatStatus(agent.CurrentStatus(c), !plain))
			return
		case "notify":
			if len(args) != 2 || args[1] != "test" {
				fmt.Fprintln(os.Stderr, "usage: lazymux notify test")
				os.Exit(2)
			}
			cfg := agent.LoadConfig(c)
			fmt.Printf("delivery %v, notify on %v, delay %v, sound %v\n", cfg.Delivery, cfg.On, cfg.Delay, cfg.Sound)
			agent.DeliverSync(c, cfg, agent.Note{Kind: agent.NoteBlocked, Title: "claude needs you",
				Body: "api:3 · Bash(go test ./...)  (lazymux notify test)"})
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

// hooks handles `lazymux hooks [install | uninstall | status] [agent…]`: it
// adds lazymux to the hooks of Claude Code, Codex, agy and opencode, so they
// report prompts, tool calls and permission requests (the Agents page
// Activity panel). Without agents it acts on every agent that is installed.
func hooks(args []string) {
	usage := "usage: lazymux hooks [install | uninstall | status] [claude | codex | agy | opencode …]"
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	var targets []agent.Integration
	for _, name := range args[1:] {
		i, ok := agent.IntegrationFor(name)
		if !ok {
			fmt.Fprintf(os.Stderr, "lazymux: no hooks for %q\n%s\n", name, usage)
			os.Exit(2)
		}
		targets = append(targets, i)
	}
	all := len(targets) == 0
	if all {
		targets = agent.Integrations()
	}
	exe, err := os.Executable()
	if err != nil {
		exe = "lazymux"
	}
	failed := false
	for _, i := range targets {
		name := fmt.Sprintf("%-9s", i.Kind)
		switch args[0] {
		case "install":
			if all && !i.Detected() {
				fmt.Printf("%s skipped: not installed (%s missing)\n", name, i.Dir)
				continue
			}
			changed, err := i.Install(exe)
			switch {
			case err != nil:
				fmt.Printf("%s error: %v\n", name, err)
				failed = true
			case changed:
				fmt.Printf("%s installed: %s (backups: *.lazymux.bak)\n", name, strings.Join(i.Files, ", "))
			default:
				fmt.Printf("%s already installed\n", name)
			}
		case "uninstall":
			changed, err := i.Uninstall()
			switch {
			case err != nil:
				fmt.Printf("%s error: %v\n", name, err)
				failed = true
			case changed:
				fmt.Printf("%s removed from %s\n", name, strings.Join(i.Files, ", "))
			default:
				fmt.Printf("%s not installed\n", name)
			}
		case "status":
			state := "not installed"
			switch {
			case !i.Detected():
				state = "agent not found (" + i.Dir + ")"
			case i.Installed():
				state = "installed (" + strings.Join(i.Files, ", ") + ")"
			}
			fmt.Printf("%s %s\n", name, state)
		default:
			fmt.Fprintln(os.Stderr, usage)
			os.Exit(2)
		}
	}
	switch args[0] {
	case "install":
		fmt.Println("restart running agents to pick up the hooks; the command is:", "'"+exe+"' hook <agent> <event>")
	case "status":
		fmt.Println("events are stored in", agent.EventDir())
	}
	if failed {
		os.Exit(1)
	}
}
