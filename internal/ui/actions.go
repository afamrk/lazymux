package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/afamrk/lazymux/internal/tmux"
)

// markRef is the item marked with space; it is the target of move / swap.
type markRef struct {
	panel int // panelSessions / panelWindows / panelPanes
	sess  string
	id    string // session, window or pane id
	label string
}

// actionMsg reports the result of a tmux action. Non-empty sel* ids move the
// selection (e.g. to a newly created window).
type actionMsg struct {
	toast                  string
	err                    error
	selSess, selWin, selPn string
	clearMark              bool
	undo                   *tmux.Snapshot // pushed on success (kills)
	popUndo                bool           // undo applied: drop the snapshot
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
	if msg.clearMark {
		m.mark = nil
	}
	if msg.undo != nil {
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

func (m *Model) killCmd() tea.Cmd {
	s, _, w, _, p, _ := m.cur()
	if s == nil {
		return nil
	}
	c := m.c
	var (
		title string
		body  []string
		panes []*tmux.Pane
		snap  *tmux.Snapshot
		do    func() error
	)
	switch m.focus {
	case panelSessions:
		for _, x := range s.Windows {
			panes = append(panes, x.Panes...)
		}
		title = "Kill session?"
		body = []string{
			" Kill session " + stText.Bold(true).Render(s.Name) + " and everything in it?", "",
			stSub.Render(fmt.Sprintf("   %d windows · %d panes", len(s.Windows), len(panes))),
		}
		snap = tmux.SnapSession(s)
		do = func() error { return c.KillSession(s.ID) }
	case panelWindows:
		panes = w.Panes
		title = "Kill window?"
		body = []string{
			" Kill window " + stText.Bold(true).Render(fmt.Sprintf("%d:%s", w.Index, w.Name)) + " of " + s.Name + "?", "",
			stSub.Render(fmt.Sprintf("   %d panes", len(w.Panes))),
		}
		snap = tmux.SnapWindow(w)
		do = func() error { return c.KillWindow(s.ID, w.ID) }
	case panelPanes:
		panes = []*tmux.Pane{p}
		title = "Kill pane?"
		body = []string{" Kill pane " + stText.Bold(true).Render(p.ID) + " (" + p.Command + ")?"}
		snap = tmux.SnapPane(p)
		do = func() error { return c.KillPane(p.ID) }
	}
	for _, x := range panes {
		if x.ID == m.selfPane {
			return m.setToast("✗ that would kill lazymux itself", true)
		}
	}
	if r := running(panes); len(r) > 0 {
		body = append(body, stRed.Render("   running: "+strings.Join(r, ", ")))
	}
	m.modal = &confirmModal{
		title: title, body: body,
		onYes: func() tea.Cmd {
			return func() tea.Msg {
				if err := do(); err != nil {
					return actionMsg{err: err}
				}
				return actionMsg{toast: "✓ killed " + snap.Label + " · u to undo", undo: snap, clearMark: true}
			}
		},
	}
	return nil
}

func (m *Model) undoCmd() tea.Cmd {
	if len(m.undo) == 0 {
		return m.setToast("nothing to undo", false)
	}
	snap, c := m.undo[len(m.undo)-1], m.c
	return func() tea.Msg {
		if err := c.Restore(snap); err != nil {
			return actionMsg{err: fmt.Errorf("undo %s: %w", snap.Label, err)}
		}
		return actionMsg{toast: "✓ restored " + snap.Label, popUndo: true}
	}
}

// --- mark / move / swap ----------------------------------------------------

func (m *Model) toggleMark() tea.Cmd {
	s, _, w, _, p, _ := m.cur()
	if s == nil {
		return nil
	}
	ref := markRef{panel: m.focus, sess: s.ID}
	switch m.focus {
	case panelSessions:
		ref.id, ref.label = s.ID, "session "+s.Name
	case panelWindows:
		ref.id, ref.label = w.ID, fmt.Sprintf("window %s:%d", s.Name, w.Index)
	case panelPanes:
		ref.id, ref.label = p.ID, "pane "+p.ID
	}
	if m.mark != nil && *m.mark == ref {
		m.mark = nil
		return nil
	}
	m.mark = &ref
	return nil
}

func (m *Model) isMarked(panel int, id string) bool {
	return m.mark != nil && m.mark.panel == panel && m.mark.id == id
}

const markHint = "mark a target first with space"

func (m *Model) moveCmd() tea.Cmd {
	s, _, w, _, p, _ := m.cur()
	if s == nil {
		return nil
	}
	c, mk := m.c, m.mark
	switch {
	case m.focus == panelWindows && mk != nil && mk.panel == panelSessions:
		if mk.id == s.ID {
			return m.setToast("window is already in that session", true)
		}
		return func() tea.Msg {
			err := c.MoveWindow(s.ID, w.ID, mk.id)
			return actionMsg{err: err, toast: fmt.Sprintf("✓ moved window %s to %s", w.Name, mk.label), clearMark: true, selSess: mk.id, selWin: w.ID}
		}
	case m.focus == panelPanes && mk != nil && (mk.panel == panelWindows || mk.panel == panelPanes):
		target := mk.id
		if mk.panel == panelWindows {
			target = mk.sess + ":" + mk.id
		}
		if mk.id == p.ID || (mk.panel == panelWindows && mk.id == w.ID) {
			return m.setToast("pane is already there", true)
		}
		return func() tea.Msg {
			err := c.JoinPane(p.ID, target)
			return actionMsg{err: err, toast: "✓ joined " + p.ID + " into " + mk.label, clearMark: true}
		}
	case m.focus == panelWindows:
		return m.setToast("move window: "+markHint+" on a session", true)
	case m.focus == panelPanes:
		return m.setToast("join pane: "+markHint+" on a window or pane", true)
	}
	return m.setToast("m works on windows and panes", true)
}

func (m *Model) swapCmd() tea.Cmd {
	s, _, w, _, p, _ := m.cur()
	if s == nil {
		return nil
	}
	c, mk := m.c, m.mark
	switch {
	case m.focus == panelWindows && mk != nil && mk.panel == panelWindows && mk.id != w.ID:
		return func() tea.Msg {
			err := c.SwapWindows(s.ID, w.ID, mk.sess, mk.id)
			return actionMsg{err: err, toast: "✓ swapped window " + w.Name + " with " + mk.label, clearMark: true}
		}
	case m.focus == panelPanes && mk != nil && mk.panel == panelPanes && mk.id != p.ID:
		return func() tea.Msg {
			err := c.SwapPanes(p.ID, mk.id)
			return actionMsg{err: err, toast: "✓ swapped " + p.ID + " with " + mk.id, clearMark: true}
		}
	case m.focus == panelWindows || m.focus == panelPanes:
		return m.setToast("swap: "+markHint+" on another item in this panel", true)
	}
	return m.setToast("s works on windows and panes", true)
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
