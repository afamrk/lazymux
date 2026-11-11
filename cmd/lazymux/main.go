// Command lazymux is a lazygit-style TUI for tmux sessions, windows, panes
// and the AI coding agents running in them.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
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
		case "preview": // the sidebar's preview popup
			if len(args) != 3 {
				fmt.Fprintln(os.Stderr, "usage: lazymux preview <sidebar-pane> <socket>")
				os.Exit(2)
			}
			if _, err := tea.NewProgram(ui.NewPreview(c, args[2]), tea.WithAltScreen()).Run(); err != nil {
				fmt.Fprintln(os.Stderr, "lazymux:", err)
				os.Exit(1)
			}
			return
		case "hook": // called by Claude Code hooks; must stay quiet and fast
			agent.RunHook(args[1:], os.Stdin, os.Stdout)
			return
		case "hooks":
			hooks(args[1:])
			return
		case "rules":
			rules(c, args[1:])
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
			fmt.Printf("delivery %v, notify on %v, delay %v, min work %v, sound %v\n",
				cfg.Delivery, cfg.On, cfg.Delay, cfg.MinWork, cfg.Sound)
			// two samples, on two panes so the second does not replace the first
			agent.DeliverSync(c, cfg, agent.Note{Kind: agent.NoteBlocked, Pane: "test-blocked",
				Title: "Claude needs you", Place: "api › editor", Verb: "run", Detail: "go test ./..."})
			agent.DeliverSync(c, cfg, agent.Note{Kind: agent.NoteDone, Pane: "test-done",
				Title: "Claude finished · 2m 13s", Place: "lazymux", Detail: "Sidebar preview popup"})
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

// sidebar handles `lazymux sidebar [toggle|cycle|follow <session>|off]`. Without a
// subcommand it runs the UI inside the sidebar pane.
func sidebar(c *tmux.Client, args []string) {
	var err error
	switch {
	case len(args) == 0:
		m := ui.NewSidebar(c)
		// the pane is killed with SIGHUP (sidebar off, kill-pane): remove the
		// popup socket on the way out
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGHUP, syscall.SIGTERM)
		go func() { <-sig; m.Close(); os.Exit(0) }()
		_, err = tea.NewProgram(m, tea.WithAltScreen(), tea.WithReportFocus()).Run()
		m.Close()
	case args[0] == "toggle":
		err = c.SidebarToggle()
	case args[0] == "cycle": // the one key: open + focus, focus, close
		session, pane := "", ""
		if len(args) == 3 {
			session, pane = args[1], args[2]
		}
		err = c.SidebarCycle(session, pane)
	case args[0] == "off":
		err = c.SidebarOff()
	case args[0] == "follow" && len(args) == 2:
		err = c.SidebarFollow(args[1])
	default:
		err = fmt.Errorf("usage: lazymux sidebar [toggle | cycle [<session-id> <pane-id>] | off | follow <session-id>]")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "lazymux:", err)
		os.Exit(1)
	}
}

// rules handles `lazymux rules [explain [pane] | show <agent>]`: the
// detection rules of each agent, where an override goes, and how the rules
// read a pane.
func rules(c *tmux.Client, args []string) {
	usage := "usage: lazymux rules [explain [pane-id] | show <agent>]"
	switch {
	case len(args) == 0:
		fmt.Println("overrides:", agent.RulesDir()+"/<agent>.toml (herdr manifests work too)")
		for _, k := range agent.Kinds {
			m, warn := agent.ManifestFor(k)
			fmt.Printf("%-9s %d rules, %s", k, len(m.Rules), m.Source)
			if m.Version != "" {
				fmt.Printf(" (version %s)", m.Version)
			}
			fmt.Println()
			if warn != "" {
				fmt.Println("          warning:", warn)
			}
		}
	case args[0] == "explain" && len(args) <= 2:
		pane := os.Getenv("TMUX_PANE")
		if len(args) == 2 {
			pane = args[1]
		}
		if pane == "" {
			fmt.Fprintln(os.Stderr, usage)
			os.Exit(2)
		}
		out, err := agent.Explain(c, pane)
		if err != nil {
			fmt.Fprintln(os.Stderr, "lazymux:", err)
			os.Exit(1)
		}
		fmt.Print(out)
	case args[0] == "show" && len(args) == 2:
		text := agent.BundledRules(agent.Kind(args[1]))
		if text == "" {
			fmt.Fprintf(os.Stderr, "lazymux: no rules for %q\n", args[1])
			os.Exit(2)
		}
		fmt.Print(text)
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
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
