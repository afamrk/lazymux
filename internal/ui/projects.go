package ui

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/afamrk/lazymux/internal/agent"
	"github.com/afamrk/lazymux/internal/project"
	"github.com/afamrk/lazymux/internal/tmux"
)

// --- project picker (N) -----------------------------------------------------

// projectsMsg carries the project directories found for the picker.
type projectsMsg struct{ dirs []string }

// projectOptions reads the picker settings from tmux options:
//
//	@lazymux-project-roots  comma-separated directories searched for git repositories
//	                        (default: those of ~/project ~/projects ~/src ~/code ~/work ~/dev that exist)
//	@lazymux-project-depth  how deep under a root (default 3)
//	@lazymux-project-zoxide on | off: include zoxide's directories (default on)
func projectOptions(c *tmux.Client) project.Options {
	o := project.Options{Roots: project.DefaultRoots, Depth: project.DefaultDepth, Zoxide: true}
	get := func(name string) string {
		out, _ := c.Run("show-options", "-gqv", name)
		return strings.Trim(strings.TrimSpace(out), `"'`)
	}
	if v := get("@lazymux-project-roots"); v != "" {
		o.Roots = nil
		for _, r := range strings.Split(v, ",") {
			if r = strings.TrimSpace(r); r != "" {
				o.Roots = append(o.Roots, r)
			}
		}
	}
	if n, err := strconv.Atoi(get("@lazymux-project-depth")); err == nil && n > 0 {
		o.Depth = n
	}
	if v := get("@lazymux-project-zoxide"); v == "off" || v == "0" || v == "false" {
		o.Zoxide = false
	}
	return o
}

// loadProjects lists the projects off the UI goroutine.
func (m *Model) loadProjects() tea.Cmd {
	c := m.c
	return tea.Batch(m.setToast("finding projects…", false), func() tea.Msg {
		return projectsMsg{project.List(projectOptions(c))}
	})
}

// sessionByName finds a session by name.
func (m *Model) sessionByName(name string) *tmux.Session {
	for _, s := range m.sessions() {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// openProjects shows the project picker: enter switches to the project's
// session, creating it (named after the directory, started in it) when
// there is none.
func (m *Model) openProjects(dirs []string) tea.Cmd {
	if len(dirs) == 0 {
		return m.setToast("no projects found: install zoxide, or set @lazymux-project-roots", true)
	}
	var items []pickItem
	for _, d := range dirs {
		name := project.SessionName(d)
		it := pickItem{text: tildePath(d), value: d}
		if s := m.sessionByName(name); s != nil {
			it.detail, it.sess = "● session "+name, s.ID
		}
		items = append(items, it)
	}
	c := m.c
	m.modal = newPicker("Open project", cGreen, "open", items, func(it pickItem) tea.Cmd {
		dir := it.value
		name := project.SessionName(dir)
		return func() tea.Msg {
			id := it.sess
			if id == "" {
				var err error
				if id, err = c.NewSession(name, dir); err != nil {
					return actionMsg{err: err}
				}
			}
			return projectOpenedMsg{id: id, created: it.sess == "", name: name}
		}
	})
	return nil
}

// projectOpenedMsg: a project's session exists now; switch to it.
type projectOpenedMsg struct {
	id, name string
	created  bool
}

func (m *Model) projectOpened(msg projectOpenedMsg) tea.Cmd {
	m.page = pageTmux
	m.filter = [numPanels]string{}
	m.selSess, m.selWin, m.selPane = msg.id, "", ""
	m.focus = panelSessions
	s := &tmux.Session{ID: msg.id, Name: msg.name}
	c := m.c
	if !tmux.InsideTmux() && c.ClientTTY == "" {
		m.AttachTo = s
		return func() tea.Msg { return switchMsg{} }
	}
	toast := "✓ switched to " + msg.name
	if msg.created {
		toast = "✓ created session " + msg.name
	}
	return tea.Batch(m.setToast(toast, false), func() tea.Msg { return switchMsg{c.SwitchTo(s, nil, nil)} })
}

// --- new agent (n on the Agents page) ------------------------------------------

// installedAgents are the agent programs found in $PATH, in lazymux's order.
var installedAgents = func() []string {
	var out []string
	for _, k := range agent.Kinds {
		if _, err := exec.LookPath(string(k)); err == nil {
			out = append(out, string(k))
		}
	}
	return out
}

// newAgentCmd asks for an agent and a directory and starts the agent in a
// new window of the selected session (the selected agent's, or the current
// one), next to the selected agent's directory.
func (m *Model) newAgentCmd() tea.Cmd {
	kinds := installedAgents()
	if len(kinds) == 0 {
		return m.setToast("no agent found in $PATH (claude, codex, agy, gemini, opencode, aider)", true)
	}
	var s *tmux.Session
	dir := ""
	rows := m.agentRows()
	if len(rows) > 0 {
		r := rows[m.agentCursor(rows)]
		s = r.sess
		if r.pane != nil {
			dir = r.pane.Path
		}
	}
	if s == nil {
		cs, _, _, _, p, _ := m.cur()
		s = cs
		if p != nil {
			dir = p.Path
		}
	}
	if s == nil {
		return nil
	}
	if dir == "" {
		if w := firstWindow(s); w != nil && w.ActivePane() != nil {
			dir = w.ActivePane().Path
		}
	}
	c := m.c
	d := &inputModal{
		title:      "New agent in " + s.Name,
		fields:     []field{newField("Dir", dir, true)},
		radio:      kinds,
		radioLabel: "agent:",
		verb:       "start",
		onSubmit: func(v []string, choice int) tea.Cmd {
			kind := kinds[choice]
			return func() tea.Msg {
				wid, pid, err := c.NewWindow(s.ID, kind, v[0])
				if err == nil {
					// typed into the window's shell, so the shell's PATH is used and
					// the window stays when the agent exits
					err = c.SendLine(pid, kind)
				}
				return actionMsg{err: err, toast: fmt.Sprintf("✓ started %s in %s", kind, s.Name),
					selSess: s.ID, selWin: wid, selPn: pid, selAgent: pid}
			}
		},
	}
	d.setFocus(len(d.fields)) // start on the agent choice
	m.modal = d
	return nil
}

func firstWindow(s *tmux.Session) *tmux.Window {
	for _, w := range s.Windows {
		if w.Active {
			return w
		}
	}
	if len(s.Windows) > 0 {
		return s.Windows[0]
	}
	return nil
}
