package ui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/afamrk/lazymux/internal/tmux"
)

// pickItem is one row of a picker.
type pickItem struct {
	kind   string // optional tag column, e.g. "win"
	kindSt lipgloss.Style
	text   string // matched and highlighted
	detail string // dim, right-aligned; also searched
	sess   string // target ids
	win    string
	pane   string
	agent  bool // an agent row of jump anywhere (pane is its pane)
}

type pickHit struct {
	item  pickItem
	idx   []int
	score int
}

// pickerModal is a fuzzy list in a dialog: the move target picker (m) and
// jump anywhere (f).
type pickerModal struct {
	title  string
	color  lipgloss.Color
	items  []pickItem
	in     textinput.Model
	hits   []pickHit
	cur    int
	off    int
	verb   string                 // "move", "go"
	onPick func(pickItem) tea.Cmd // enter
	onTab  func(pickItem) tea.Cmd // tab (optional)
}

func newPicker(title string, color lipgloss.Color, verb string, items []pickItem, onPick func(pickItem) tea.Cmd) *pickerModal {
	in := textinput.New()
	in.Prompt = "❯ "
	in.PromptStyle = lipgloss.NewStyle().Foreground(color)
	in.TextStyle = stText
	in.Placeholder = "type to filter…"
	in.PlaceholderStyle = stFaint
	in.Cursor.SetMode(cursor.CursorStatic)
	in.Focus()
	p := &pickerModal{title: title, color: color, items: items, in: in, verb: verb, onPick: onPick}
	p.refilter()
	return p
}

func (p *pickerModal) refilter() {
	q := p.in.Value()
	p.hits = p.hits[:0]
	for _, it := range p.items {
		score, idx, ok := fuzzyMatch(q, it.text)
		if !ok {
			// also search the details (no highlight), ranked below name hits
			s2, _, ok2 := fuzzyMatch(q, it.detail)
			if !ok2 {
				continue
			}
			score, idx = s2-1000, nil
		}
		p.hits = append(p.hits, pickHit{it, idx, score})
	}
	if q != "" {
		sort.SliceStable(p.hits, func(i, j int) bool { return p.hits[i].score > p.hits[j].score })
	}
	p.cur, p.off = 0, 0
}

func (p *pickerModal) update(k tea.KeyMsg) (tea.Cmd, bool) {
	switch k.String() {
	case "esc", "ctrl+c":
		return nil, true
	case "enter":
		if len(p.hits) == 0 {
			return nil, false
		}
		return p.onPick(p.hits[p.cur].item), true
	case "tab":
		if p.onTab != nil && len(p.hits) > 0 {
			return p.onTab(p.hits[p.cur].item), true
		}
		return nil, false
	case "down", "ctrl+j", "ctrl+n":
		p.cur = min(p.cur+1, max(0, len(p.hits)-1))
		return nil, false
	case "up", "ctrl+k", "ctrl+p":
		p.cur = max(p.cur-1, 0)
		return nil, false
	}
	var cmd tea.Cmd
	before := p.in.Value()
	p.in, cmd = p.in.Update(k)
	if p.in.Value() != before {
		p.refilter()
	}
	return cmd, false
}

func (p *pickerModal) view(maxW, maxH int) string {
	w := min(76, maxW-4)
	inner := w - 2
	rows := clamp(maxH-10, 3, 12)
	p.off = scrollWindow(p.off, p.cur, rows, len(p.hits))

	kindW := 0
	for _, it := range p.items {
		kindW = max(kindW, len(it.kind))
	}
	p.in.Width = inner - 4
	lines := []string{" " + p.in.View(), stFaint.Render(strings.Repeat("─", inner))}
	for i := p.off; i < len(p.hits) && i < p.off+rows; i++ {
		h := p.hits[i]
		sel := 0
		if i == p.cur {
			sel = 2
		}
		left := []seg{{" ", stText}}
		if kindW > 0 {
			left = append(left, seg{padRight(h.item.kind, kindW) + "  ", h.item.kindSt})
		}
		base := stSub
		if sel == 2 {
			base = stText.Bold(true)
		}
		left = append(left, hlSegs(h.item.text, h.idx, base, stMatch)...)
		lines = append(lines, renderRow(left, []seg{{h.item.detail + " ", stFaint}}, inner, sel))
	}
	if len(p.hits) == 0 {
		lines = append(lines, " "+stFaint.Render("no match"))
	}
	for len(lines) < rows+2 {
		lines = append(lines, "")
	}
	hints := []string{"enter", p.verb}
	if p.onTab != nil {
		hints = append(hints, "tab", "select")
	}
	hints = append(hints, "esc", "close")
	footer := stFaint.Render(fmt.Sprintf(" %d of %d ", len(p.hits), len(p.items))) + keyHints(hints...)
	title := lipgloss.NewStyle().Foreground(p.color).Bold(true).Render(" " + p.title + " ")
	return renderFrame(title, ansi.Truncate(footer, w-4, "…"), true, lines, w, len(lines)+2, p.color)
}

// --- jump anywhere (f) ------------------------------------------------

var (
	stKindSess = stGreen.Bold(true)
	stKindWin  = stBlue.Bold(true)
	stKindPane = stPeach.Bold(true)
)

// tildePath shortens $HOME to ~.
func tildePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

// agentJumpItems lists every agent, the most urgent first.
func (m *Model) agentJumpItems() []pickItem {
	var items []pickItem
	for _, s := range m.sessions() {
		as, ps := m.sessionAgents(s)
		for i, a := range as {
			p := ps[i]
			detail := agentGlyph(a) + " " + a.Label()
			switch {
			case a.Waiting != "":
				detail += " · " + a.Waiting
			case a.Summary != "":
				detail += " · " + a.Summary
			}
			items = append(items, pickItem{kind: "AGENT", kindSt: agentStyle(a).Bold(true),
				text:   fmt.Sprintf("%s %s:%d %s", a.Kind, s.Name, p.Window.Index, p.Window.Name),
				detail: ansi.Truncate(detail, 40, "…"), sess: s.ID, win: p.Window.ID, pane: p.ID, agent: true})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return urgency(m.agents[items[i].pane]) > urgency(m.agents[items[j].pane])
	})
	return items
}

// jumpItems lists every session, window and pane, and every agent (first
// on the Agents page, last on the tmux page).
func (m *Model) jumpItems() []pickItem {
	agents := m.agentJumpItems()
	if m.page == pageAgents {
		return append(agents, m.placeItems()...)
	}
	return append(m.placeItems(), agents...)
}

func (m *Model) placeItems() []pickItem {
	var items []pickItem
	for _, s := range m.sessions() {
		detail := fmt.Sprintf("%d windows", len(s.Windows))
		if s.Attached {
			detail += " · attached ●"
		}
		items = append(items, pickItem{kind: "SESS", kindSt: stKindSess, text: s.Name, detail: detail, sess: s.ID})
		for _, w := range s.Windows {
			d := ""
			if p := w.ActivePane(); p != nil {
				d = p.Command + " · " + tildePath(p.Path)
			}
			items = append(items, pickItem{kind: "WIN", kindSt: stKindWin,
				text: fmt.Sprintf("%s:%d %s", s.Name, w.Index, w.Name), detail: d, sess: s.ID, win: w.ID})
			if len(w.Panes) < 2 {
				continue // a single pane is the same place as its window
			}
			for _, p := range w.Panes {
				items = append(items, pickItem{kind: "PANE", kindSt: stKindPane,
					text:   fmt.Sprintf("%s:%d.%s %s", s.Name, w.Index, p.ID, p.Command),
					detail: fmt.Sprintf("%dx%d · %s", p.Width, p.Height, tildePath(p.Path)),
					sess:   s.ID, win: w.ID, pane: p.ID})
			}
		}
	}
	return items
}

// openJump opens the jump anywhere palette (f). enter selects the item and
// switches to it; tab only selects it: agents on the Agents page, the rest
// in the tmux panels.
func (m *Model) openJump() {
	selectAgent := func(it pickItem) {
		m.page = pageAgents
		m.agentTab, m.agentFilter, m.agentSel = agentAll, "", it.pane
		delete(m.collapsed, it.sess)
		m.normalizeAgents()
	}
	selectIt := func(it pickItem) {
		if it.agent {
			selectAgent(it)
			return
		}
		m.page = pageTmux
		m.filter = [numPanels]string{}
		m.selSess, m.selWin, m.selPane = it.sess, it.win, it.pane
		switch {
		case it.pane != "":
			m.focus = panelPanes
		case it.win != "":
			m.focus = panelWindows
		default:
			m.focus = panelSessions
		}
		m.normalize()
	}
	p := newPicker("Jump to", cMauve, "go", m.jumpItems(), func(it pickItem) tea.Cmd {
		selectIt(it)
		if it.agent {
			return tea.Batch(m.jumpToAgent(), m.fetchPreview())
		}
		return tea.Batch(m.switchCmd(), m.fetchPreview())
	})
	p.onTab = func(it pickItem) tea.Cmd {
		selectIt(it)
		return m.fetchPreview()
	}
	m.modal = p
}

// --- move target picker (m) ------------------------------------------------

// moveCmd moves the selected windows to a session, or joins the selected
// panes into a window, chosen in a picker.
func (m *Model) moveCmd() tea.Cmd {
	refs := m.targets()
	if len(refs) == 0 {
		return nil
	}
	c := m.c
	noun := func(one, many string) string {
		if len(refs) == 1 {
			return one
		}
		return fmt.Sprintf("%d %s", len(refs), many)
	}
	switch m.focus {
	case panelWindows:
		var items []pickItem
		for _, s := range m.sessions() {
			if allIn(refs, func(r selRef) bool { return r.sess == s.ID }) {
				continue // everything is already there
			}
			items = append(items, pickItem{text: s.Name, detail: fmt.Sprintf("%dw", len(s.Windows)), sess: s.ID})
		}
		if len(items) == 0 {
			return m.setToast("no other session to move to", true)
		}
		m.modal = newPicker("Move "+noun("window", "windows")+" to", cBlue, "move", items, func(it pickItem) tea.Cmd {
			return func() tea.Msg {
				moved := 0
				for _, r := range refs {
					if r.sess == it.sess {
						continue
					}
					if err := c.MoveWindow(r.sess, r.id, it.sess); err != nil {
						return actionMsg{err: err, clearSel: true}
					}
					moved++
				}
				return actionMsg{toast: fmt.Sprintf("✓ moved %d window(s) to %s", moved, it.text),
					clearSel: true, selSess: it.sess, selWin: refs[0].id}
			}
		})
	case panelPanes:
		var items []pickItem
		for _, s := range m.sessions() {
			for _, w := range s.Windows {
				inThis := func(r selRef) bool { return paneWindow(m.tree, r.id) == w }
				if allIn(refs, inThis) {
					continue
				}
				cmd := ""
				if p := w.ActivePane(); p != nil {
					cmd = p.Command
				}
				items = append(items, pickItem{text: fmt.Sprintf("%s:%d %s", s.Name, w.Index, w.Name),
					detail: fmt.Sprintf("%dp · %s", len(w.Panes), cmd), sess: s.ID, win: w.ID})
			}
		}
		if len(items) == 0 {
			return m.setToast("no other window to join into", true)
		}
		m.modal = newPicker("Join "+noun("pane", "panes")+" into", cBlue, "join", items, func(it pickItem) tea.Cmd {
			return func() tea.Msg {
				for _, r := range refs {
					if paneWindowID(c, r.id) == it.win {
						continue
					}
					if err := c.JoinPane(r.id, it.sess+":"+it.win); err != nil {
						return actionMsg{err: err, clearSel: true}
					}
				}
				return actionMsg{toast: fmt.Sprintf("✓ joined %s into %s", noun("pane", "panes"), it.text),
					clearSel: true, selSess: it.sess, selWin: it.win, selPn: refs[0].id}
			}
		})
	default:
		return m.setToast("m moves windows (to a session) and panes (into a window)", true)
	}
	return nil
}

func allIn(refs []selRef, f func(selRef) bool) bool {
	for _, r := range refs {
		if !f(r) {
			return false
		}
	}
	return true
}

// paneWindow finds the window that holds pane id in the tree.
func paneWindow(t *tmux.Tree, id string) *tmux.Window {
	if t == nil {
		return nil
	}
	for _, s := range t.Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.ID == id {
					return w
				}
			}
		}
	}
	return nil
}

// paneWindowID asks tmux which window a pane is in right now (earlier joins
// in the same batch may have moved things).
func paneWindowID(c *tmux.Client, pane string) string {
	out, _ := c.Run("display-message", "-p", "-t", pane, "#{window_id}")
	return out
}

// swapCmd swaps the one selected item with the cursor item.
func (m *Model) swapCmd() tea.Cmd {
	if m.focus != panelWindows && m.focus != panelPanes {
		return m.setToast("s swaps windows or panes", true)
	}
	cur, ok := m.cursorRef()
	sel := m.selected
	if !ok || len(sel) != 1 || sel[0].panel != m.focus || sel[0].id == cur.id {
		return m.setToast("swap: select one item with space, move to the other, press s", true)
	}
	c, a := m.c, sel[0]
	if m.focus == panelWindows {
		return func() tea.Msg {
			err := c.SwapWindows(cur.sess, cur.id, a.sess, a.id)
			return actionMsg{err: err, toast: "✓ swapped " + cur.label + " with " + a.label, clearSel: true}
		}
	}
	return func() tea.Msg {
		err := c.SwapPanes(cur.id, a.id)
		return actionMsg{err: err, toast: "✓ swapped " + cur.id + " with " + a.id, clearSel: true}
	}
}
