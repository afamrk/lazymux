package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/afamrk/lazymux/internal/tmux"
)

// actionMsg reports the result of a tmux action. Non-empty sel* ids move the
// selection (e.g. to a newly created window).
type actionMsg struct {
	toast                  string
	err                    error
	selSess, selWin, selPn string
	clearSel               bool
	undo                   []*tmux.Snapshot // pushed as one undo step on success (kills)
	popUndo                bool             // undo applied: drop the step
}

type toastState struct {
	text  string
	isErr bool
	id    int
}

type toastExpireMsg struct{ id int }

const toastFor = 4 * time.Second

func (m *Model) setToast(text string, isErr bool) tea.Cmd {
	m.toastSeq++
	m.toast = toastState{text: text, isErr: isErr, id: m.toastSeq}
	id := m.toastSeq
	return tea.Tick(toastFor, func(time.Time) tea.Msg { return toastExpireMsg{id} })
}

func (m *Model) handleAction(msg actionMsg) tea.Cmd {
	cmds := []tea.Cmd{m.fetchTree()}
	if msg.err != nil {
		cmds = append(cmds, m.setToast("✗ "+msg.err.Error(), true))
		return tea.Batch(cmds...)
	}
	if msg.selSess != "" {
		m.selSess, m.selWin, m.selPane = msg.selSess, msg.selWin, msg.selPn
	} else if msg.selWin != "" {
		m.selWin, m.selPane = msg.selWin, msg.selPn
	} else if msg.selPn != "" {
		m.selPane = msg.selPn
	}
	if msg.clearSel {
		m.selected = nil
	}
	if len(msg.undo) > 0 {
		m.undo = append(m.undo, msg.undo)
		if len(m.undo) > 20 {
			m.undo = m.undo[1:]
		}
	}
	if msg.popUndo && len(m.undo) > 0 {
		m.undo = m.undo[:len(m.undo)-1]
	}
	if msg.toast != "" {
		cmds = append(cmds, m.setToast(msg.toast, false))
	}
	return tea.Batch(cmds...)
}

// --- new / rename ----------------------------------------------------------

func (m *Model) newCmd() tea.Cmd {
	s, _, w, _, p, _ := m.cur()
	if s == nil {
		return nil
	}
	c := m.c
	dir := ""
	if p != nil {
		dir = p.Path
	}
	switch m.focus {
	case panelSessions:
		m.modal = &inputModal{
			title:  "New session",
			fields: []field{newField("Name", "", false), newField("Dir", dir, true)},
			radio:  []string{"new session", "new window in " + s.Name},
			verb:   "create",
			onSubmit: func(v []string, radio int) tea.Cmd {
				return func() tea.Msg {
					if radio == 1 {
						wid, pid, err := c.NewWindow(s.ID, v[0], v[1])
						return actionMsg{err: err, toast: "✓ created window in " + s.Name, selSess: s.ID, selWin: wid, selPn: pid}
					}
					id, err := c.NewSession(v[0], v[1])
					return actionMsg{err: err, toast: "✓ created session " + v[0], selSess: id}
				}
			},
		}
		m.modal.(*inputModal).setFocus(0)
	case panelWindows:
		m.modal = &inputModal{
			title:  "New window in " + s.Name,
			fields: []field{newField("Name", "", false), newField("Dir", dir, true)},
			verb:   "create",
			onSubmit: func(v []string, _ int) tea.Cmd {
				return func() tea.Msg {
					wid, pid, err := c.NewWindow(s.ID, v[0], v[1])
					return actionMsg{err: err, toast: "✓ created window", selSess: s.ID, selWin: wid, selPn: pid}
				}
			},
		}
		m.modal.(*inputModal).setFocus(0)
	case panelPanes:
		if p == nil || w == nil {
			return nil
		}
		return func() tea.Msg {
			id, err := c.SplitPane(p.ID, p.Path, false)
			return actionMsg{err: err, toast: "✓ split " + p.ID, selPn: id}
		}
	}
	return nil
}

func (m *Model) renameCmd() tea.Cmd {
	s, _, w, _, p, _ := m.cur()
	if s == nil {
		return nil
	}
	c := m.c
	var d *inputModal
	switch m.focus {
	case panelSessions:
		d = &inputModal{title: "Rename session " + s.Name, fields: []field{newField("", s.Name, false)}, verb: "rename",
			onSubmit: func(v []string, _ int) tea.Cmd {
				return func() tea.Msg { return actionMsg{err: c.RenameSession(s.ID, v[0]), toast: "✓ renamed to " + v[0]} }
			}}
	case panelWindows:
		d = &inputModal{title: fmt.Sprintf("Rename window %d:%s", w.Index, w.Name), fields: []field{newField("", w.Name, false)}, verb: "rename",
			onSubmit: func(v []string, _ int) tea.Cmd {
				return func() tea.Msg {
					return actionMsg{err: c.RenameWindow(s.ID, w.ID, v[0]), toast: "✓ renamed to " + v[0]}
				}
			}}
	case panelPanes:
		d = &inputModal{title: "Set title of pane " + p.ID, fields: []field{newField("", p.Title, false)}, verb: "set",
			onSubmit: func(v []string, _ int) tea.Cmd {
				return func() tea.Msg { return actionMsg{err: c.SetPaneTitle(p.ID, v[0]), toast: "✓ pane title set"} }
			}}
	}
	d.setFocus(0)
	m.modal = d
	return nil
}

// --- kill / undo -----------------------------------------------------------

// running lists the distinct non-shell programs in the given panes, so the
// kill dialog can warn about them.
func running(panes []*tmux.Pane) []string {
	seen := map[string]bool{}
	for _, p := range panes {
		switch p.Command {
		case "bash", "zsh", "fish", "sh", "dash", "":
			continue
		}
		seen[p.Command] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// killCmd kills the selected items (or the cursor item) after a confirm.
// All of them form one undo step.
func (m *Model) killCmd() tea.Cmd {
	refs := m.targets()
	if len(refs) == 0 || m.tree == nil {
		return nil
	}
	c := m.c
	type victim struct {
		label string
		panes []*tmux.Pane
		snap  *tmux.Snapshot
		do    func() error
	}
	var vs []victim
	for _, r := range refs {
		var v victim
		switch r.panel {
		case panelSessions:
			s := findSession(m.tree, r.id)
			if s == nil {
				continue
			}
			for _, x := range s.Windows {
				v.panes = append(v.panes, x.Panes...)
			}
			v.label = fmt.Sprintf("session %s (%d windows, %d panes)", s.Name, len(s.Windows), len(v.panes))
			v.snap = tmux.SnapSession(s)
			id := s.ID
			v.do = func() error { return c.KillSession(id) }
		case panelWindows:
			s, w := findWindow(m.tree, r.sess, r.id)
			if w == nil {
				continue
			}
			v.panes = w.Panes
			v.label = fmt.Sprintf("window %d:%s of %s (%d panes)", w.Index, w.Name, s.Name, len(w.Panes))
			v.snap = tmux.SnapWindow(w)
			sid, wid := s.ID, w.ID
			v.do = func() error { return c.KillWindow(sid, wid) }
		case panelPanes:
			w := paneWindow(m.tree, r.id)
			if w == nil {
				continue
			}
			var p *tmux.Pane
			for _, x := range w.Panes {
				if x.ID == r.id {
					p = x
				}
			}
			v.panes = []*tmux.Pane{p}
			v.label = fmt.Sprintf("pane %s (%s)", p.ID, p.Command)
			v.snap = tmux.SnapPane(p)
			id := p.ID
			v.do = func() error { return c.KillPane(id) }
		}
		vs = append(vs, v)
	}
	if len(vs) == 0 {
		return nil
	}
	var all []*tmux.Pane
	for _, v := range vs {
		all = append(all, v.panes...)
	}
	for _, x := range all {
		if x.ID == m.selfPane {
			return m.setToast("✗ that would kill lazymux itself", true)
		}
	}

	what := []string{"session", "window", "pane"}[refs[0].panel]
	title := "Kill " + what + "?"
	var body []string
	if len(vs) == 1 {
		body = []string{" Kill " + stText.Bold(true).Render(vs[0].label) + "?"}
	} else {
		title = fmt.Sprintf("Kill %d %ss?", len(vs), what)
		body = []string{fmt.Sprintf(" Kill these %d %ss and everything in them?", len(vs), what), ""}
		for i, v := range vs {
			if i == 5 {
				body = append(body, stSub.Render(fmt.Sprintf("   … and %d more", len(vs)-5)))
				break
			}
			body = append(body, stSub.Render("   ◆ "+v.label))
		}
	}
	var agents []string
	for _, p := range all {
		if a := m.agents[p.ID]; a != nil {
			agents = append(agents, fmt.Sprintf("%s %d:%s (%s)", a.Kind, p.Window.Index, p.Window.Name, a.Label()))
		}
	}
	if len(agents) > 0 {
		body = append(body, "", stRed.Render("   ⣿ ends agents: "+strings.Join(agents, ", ")))
	}
	if r := running(all); len(r) > 0 {
		body = append(body, "", stRed.Render("   running: "+strings.Join(r, ", ")))
	}
	m.modal = &confirmModal{
		title: title, body: body,
		onYes: func() tea.Cmd {
			return func() tea.Msg {
				var snaps []*tmux.Snapshot
				for _, v := range vs {
					if err := v.do(); err != nil {
						return actionMsg{err: err, undo: snaps, clearSel: true}
					}
					snaps = append(snaps, v.snap)
				}
				label := snaps[0].Label
				if len(snaps) > 1 {
					label = fmt.Sprintf("%d %ss", len(snaps), what)
				}
				return actionMsg{toast: "✓ killed " + label + " · u to undo", undo: snaps, clearSel: true}
			}
		},
	}
	return nil
}

func findSession(t *tmux.Tree, id string) *tmux.Session {
	for _, s := range t.Sessions {
		if s.ID == id {
			return s
		}
	}
	return nil
}

func findWindow(t *tmux.Tree, sess, id string) (*tmux.Session, *tmux.Window) {
	s := findSession(t, sess)
	if s == nil {
		return nil, nil
	}
	for _, w := range s.Windows {
		if w.ID == id {
			return s, w
		}
	}
	return s, nil
}

func (m *Model) undoCmd() tea.Cmd {
	if len(m.undo) == 0 {
		return m.setToast("nothing to undo", false)
	}
	step, c := m.undo[len(m.undo)-1], m.c
	return func() tea.Msg {
		for i := len(step) - 1; i >= 0; i-- {
			if err := c.Restore(step[i]); err != nil {
				return actionMsg{err: fmt.Errorf("undo %s: %w", step[i].Label, err)}
			}
		}
		label := step[0].Label
		if len(step) > 1 {
			label = fmt.Sprintf("%d items", len(step))
		}
		return actionMsg{toast: "✓ restored " + label, popUndo: true}
	}
}

// --- pane / window commands ------------------------------------------------

func (m *Model) breakCmd() tea.Cmd {
	s, _, w, _, p, _ := m.cur()
	if p == nil {
		return nil
	}
	if len(w.Panes) < 2 {
		return m.setToast("pane is already alone in its window", true)
	}
	c := m.c
	return func() tea.Msg {
		return actionMsg{err: c.BreakPane(p.ID, s.ID), toast: "✓ " + p.ID + " moved to its own window"}
	}
}

func (m *Model) zoomCmd() tea.Cmd {
	_, _, w, _, p, _ := m.cur()
	if p == nil {
		return nil
	}
	if len(w.Panes) < 2 {
		return m.setToast("nothing to zoom: window has one pane", true)
	}
	c := m.c
	return func() tea.Msg { return actionMsg{err: c.ToggleZoom(p.ID), toast: "✓ toggled zoom on " + p.ID} }
}

func (m *Model) layoutCmd() tea.Cmd {
	s, _, w, _, _, _ := m.cur()
	if w == nil {
		return nil
	}
	if len(w.Panes) < 2 {
		return m.setToast("layout: window has one pane", true)
	}
	c := m.c
	return func() tea.Msg {
		err := c.NextLayout(s.ID, w.ID)
		return actionMsg{err: err, toast: "✓ next layout for " + w.Name}
	}
}

func (m *Model) openHelp() {
	names := []string{"Sessions", "Windows", "Panes"}
	ctx := names[m.focus]
	if m.page == pageAgents {
		ctx = "Agents"
	}
	m.modal = newHelp(ctx, func(key string) tea.Cmd {
		return func() tea.Msg { return keyMsg(key) }
	})
}
