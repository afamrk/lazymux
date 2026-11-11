package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/afamrk/lazymux/internal/agent"
	"github.com/afamrk/lazymux/internal/tmux"
)

// The sidebar is one list: the agents, grouped by state (needs you,
// working, done, idle), then the sessions as a folding tree (session →
// windows → panes). Unfocused it is an overview; focused, a cursor moves
// through it and the keys act on the row under the cursor. There are no
// pages or panels: the cursor sets the same selection the actions and
// dialogs use (an agent row selects that agent, a session / window / pane
// row selects it as the focused panel would), so those work unchanged.

type rowKind int

const (
	rowHeading rowKind = iota // a state heading, the SESSIONS rule, a blank line
	rowAgent
	rowSession
	rowWindow
	rowPane
)

// listRow is one entry of the list. Agents take two lines, the rest one.
type listRow struct {
	kind  rowKind
	key   string // "a:<pane>", "s:<session>", "w:<session>:<window>", "p:<pane>"; "" for headings
	a     *agent.Agent
	sess  *tmux.Session
	win   *tmux.Window
	pane  *tmux.Pane
	title []seg // headings
	right []seg
}

func (r listRow) selectable() bool { return r.kind != rowHeading }

// listMode reports whether the focused sidebar list handles the keys.
func (m *Model) listMode() bool { return m.sidebar && m.focused }

// sidebarSession is the session the sidebar serves (and its current window).
func (m *Model) sidebarSession() (*tmux.Session, *tmux.Window) {
	if m.tree == nil {
		return nil, nil
	}
	for _, p := range m.tree.Sidebars {
		if p.ID == m.selfPane {
			return p.Window.Session, p.Window
		}
	}
	return nil, nil
}

// isOpen reports whether a session or window is unfolded: what you toggled,
// else the sidebar's own session is open and everything else closed. While
// filtering, everything is open.
func (m *Model) isOpen(key string, def bool) bool {
	if m.listFilter != "" {
		return true
	}
	if v, ok := m.fold[key]; ok {
		return v
	}
	return def
}

func (m *Model) setOpen(key string, v bool) {
	if m.fold == nil {
		m.fold = map[string]bool{}
	}
	m.fold[key] = v
}

func winKey(w *tmux.Window) string   { return "w:" + w.Session.ID + ":" + w.ID }
func sessKey(s *tmux.Session) string { return "s:" + s.ID }

// listRows builds the list.
func (m *Model) listRows() []listRow {
	var rows []listRow
	q := m.listFilter
	groups := []struct {
		rank        int
		glyph, name string
		st          lipgloss.Style
	}{{3, "◐", "NEEDS YOU", stRed}, {2, "●", "WORKING", stYellow}, {1, "✓", "DONE", stGreen}, {0, "○", "IDLE", stSub}}
	for _, g := range groups {
		var cards []listRow
		for _, s := range m.sessions() {
			as, ps := m.sessionAgents(s)
			for i, a := range as {
				if urgency(a) != g.rank || !matchAgent(q, a, ps[i]) {
					continue
				}
				cards = append(cards, listRow{kind: rowAgent, key: "a:" + ps[i].ID, a: a, pane: ps[i],
					win: ps[i].Window, sess: s})
			}
		}
		if len(cards) == 0 {
			continue
		}
		rows = append(rows, listRow{kind: rowHeading, title: []seg{{" " + g.glyph + " " + g.name, g.st.Bold(true)}},
			right: []seg{{fmt.Sprintf("%d ", len(cards)), stFaint}}})
		rows = append(rows, cards...)
	}
	if len(rows) == 0 && q == "" {
		rows = append(rows, listRow{kind: rowHeading, title: []seg{{" no agents running", stFaint}}})
	}

	rows = append(rows, listRow{kind: rowHeading}, listRow{kind: rowHeading, title: []seg{{" SESSIONS ", stBlue.Bold(true)}}})
	curSess, _ := m.sidebarSession()
	for _, s := range m.sessions() {
		sOK, _ := matchFields(q, s.Name)
		var wins []listRow
		for _, w := range s.Windows {
			wOK, _ := matchFields(q, w.Name, paneCommands(w)...)
			var panes []listRow
			for _, p := range w.Panes {
				if ok, _ := matchFields(q, p.ID, p.Command, p.Title); ok || q == "" {
					panes = append(panes, listRow{kind: rowPane, key: "p:" + p.ID, pane: p, win: w, sess: s})
				}
			}
			if q != "" && !sOK && !wOK && len(panes) == 0 {
				continue
			}
			wins = append(wins, listRow{kind: rowWindow, key: winKey(w), win: w, sess: s})
			if m.isOpen(winKey(w), false) {
				wins = append(wins, panes...)
			}
		}
		if q != "" && !sOK && len(wins) == 0 {
			continue
		}
		rows = append(rows, listRow{kind: rowSession, key: sessKey(s), sess: s})
		if m.isOpen(sessKey(s), s == curSess) {
			rows = append(rows, wins...)
		}
	}
	return rows
}

func paneCommands(w *tmux.Window) []string {
	var out []string
	for _, p := range w.Panes {
		out = append(out, p.Command)
	}
	return out
}

// matchAgent tests an agent against the list filter: program, place,
// summary, what it waits for.
func matchAgent(q string, a *agent.Agent, p *tmux.Pane) bool {
	if q == "" {
		return true
	}
	ok, _ := matchFields(q, string(a.Kind), p.Window.Session.Name, p.Window.Name, a.Summary, a.Waiting)
	return ok
}

// listCursor is the index of the cursor row, falling back to the row at the
// last index (the item was killed or filtered away), then to the first
// selectable row.
func (m *Model) listCursor(rows []listRow) int {
	for i, r := range rows {
		if r.key != "" && r.key == m.listSel {
			return i
		}
	}
	for i := min(m.listIdx, len(rows)-1); i >= 0; i-- {
		if rows[i].selectable() {
			return i
		}
	}
	for i, r := range rows {
		if r.selectable() {
			return i
		}
	}
	return -1
}

// listDefault puts the cursor where you most likely want it: the first
// agent that needs you, else your current window.
func (m *Model) listDefault() {
	rows := m.listRows()
	for _, r := range rows {
		if r.kind == rowAgent && r.a.State == agent.Blocked {
			m.listSel = r.key
			return
		}
	}
	if _, w := m.sidebarSession(); w != nil {
		m.listSel = winKey(w)
		return
	}
	m.listSel = ""
}

// listSync points the model's selection at the cursor row, so the actions,
// dialogs, help and preview act on it.
func (m *Model) listSync() tea.Cmd {
	rows := m.listRows()
	i := m.listCursor(rows)
	if i < 0 {
		return nil
	}
	r := rows[i]
	m.listSel, m.listIdx = r.key, i
	switch r.kind {
	case rowAgent:
		m.page, m.agentSel = pageAgents, r.pane.ID
		m.markSeen()
	case rowSession:
		m.page, m.focus = pageTmux, panelSessions
		m.selSess, m.selWin, m.selPane = r.sess.ID, "", ""
	case rowWindow:
		m.page, m.focus = pageTmux, panelWindows
		m.selSess, m.selWin, m.selPane = r.sess.ID, r.win.ID, ""
	case rowPane:
		m.page, m.focus = pageTmux, panelPanes
		m.selSess, m.selWin, m.selPane = r.sess.ID, r.win.ID, r.pane.ID
	}
	if r.kind != rowAgent {
		m.normalize()
	}
	return m.publishPreview()
}

// listRowKind is the kind of the cursor row.
func (m *Model) listRowKind() rowKind {
	rows := m.listRows()
	if i := m.listCursor(rows); i >= 0 {
		return rows[i].kind
	}
	return rowHeading
}

// listMove moves the cursor by delta selectable rows (clamped).
func (m *Model) listMove(delta int) tea.Cmd {
	rows := m.listRows()
	i := m.listCursor(rows)
	if i < 0 {
		return nil
	}
	step := 1
	if delta < 0 {
		step, delta = -1, -delta
	}
	for ; delta > 0; delta-- {
		j := i + step
		for j >= 0 && j < len(rows) && !rows[j].selectable() {
			j += step
		}
		if j < 0 || j >= len(rows) {
			break
		}
		i = j
	}
	m.listSel = rows[i].key
	return m.listSync()
}

// listJumpSection moves between the agents and the sessions (tab).
func (m *Model) listJumpSection() tea.Cmd {
	rows := m.listRows()
	i := m.listCursor(rows)
	if i < 0 {
		return nil
	}
	inAgents := rows[i].kind == rowAgent
	for _, r := range rows {
		if (inAgents && r.kind == rowSession) || (!inAgents && r.kind == rowAgent) {
			m.listSel = r.key
			return m.listSync()
		}
	}
	return nil
}

// listNextBlocked moves to the next agent that needs you (B), wrapping.
func (m *Model) listNextBlocked() tea.Cmd {
	rows := m.listRows()
	i := m.listCursor(rows)
	for n := 1; n <= len(rows); n++ {
		r := rows[(i+n+len(rows))%len(rows)]
		if r.kind == rowAgent && r.a.State == agent.Blocked {
			m.listSel = r.key
			return m.listSync()
		}
	}
	return m.setToast("no agent needs you", false)
}

// listFold opens (l) or closes (h) the cursor row; on a row that is already
// so, l steps into its first child and h to its parent.
func (m *Model) listFold(open bool) tea.Cmd {
	rows := m.listRows()
	i := m.listCursor(rows)
	if i < 0 {
		return nil
	}
	r := rows[i]
	curSess, _ := m.sidebarSession()
	var key string
	var isOpen bool
	switch r.kind {
	case rowSession:
		key, isOpen = sessKey(r.sess), m.isOpen(sessKey(r.sess), r.sess == curSess)
	case rowWindow:
		key, isOpen = winKey(r.win), m.isOpen(winKey(r.win), false)
	}
	switch {
	case open && key != "" && !isOpen:
		m.setOpen(key, true)
	case open && key != "" && i+1 < len(rows) && (rows[i+1].kind == r.kind+1):
		m.listSel = rows[i+1].key
	case !open && key != "" && isOpen:
		m.setOpen(key, false)
	case !open && (r.kind == rowWindow || r.kind == rowPane):
		parent := sessKey(r.sess)
		if r.kind == rowPane {
			parent = winKey(r.win)
		}
		m.listSel = parent
	}
	return m.listSync()
}

// handleListKey handles a key in the focused sidebar.
func (m *Model) handleListKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.filtering {
		return m, m.listFilterKey(k)
	}
	if k.Type == tea.KeyRunes && len(k.Runes) > 1 && !k.Paste {
		var cmds []tea.Cmd
		for _, r := range k.Runes {
			_, cmd := m.handleListKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: k.Alt})
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}
	m.listSync() // the tree or the agents may have changed under the cursor
	kind := m.listRowKind()
	switch s := k.String(); s {
	case "j", "down":
		return m, m.listMove(1)
	case "k", "up":
		return m, m.listMove(-1)
	case "g", "home":
		return m, m.listMove(-1 << 20)
	case "G", "end":
		return m, m.listMove(1 << 20)
	case "pgdown":
		return m, m.listMove(10)
	case "pgup":
		return m, m.listMove(-10)
	case "tab", "shift+tab":
		return m, m.listJumpSection()
	case "l", "right":
		return m, m.listFold(true)
	case "h", "left":
		return m, m.listFold(false)
	case "B":
		return m, m.listNextBlocked()
	case "/":
		m.filterIn = newFilterInput()
		m.filterIn.SetValue(m.listFilter)
		m.filterIn.CursorEnd()
		m.filterIn.Focus()
		m.filtering = true
		return m, nil
	case "esc":
		switch {
		case m.listFilter != "":
			m.listFilter = ""
			return m, m.listSync()
		case len(m.selected) > 0:
			m.selected = nil
			return m, nil
		}
		return m, m.leaveSidebar()
	case "q":
		return m, m.leaveSidebar()
	case "Q":
		return m, m.sidebarOff()
	case "f", "ctrl+p":
		m.openJump()
		return m, nil
	case "?":
		m.openHelp()
		return m, nil
	case "n":
		return m, m.loadProjects()
	case "enter":
		if kind == rowAgent {
			return m, m.jumpToAgent()
		}
		return m, m.switchCmd()
	}
	if kind == rowAgent {
		switch k.String() {
		case "1", "2", "3":
			return m, m.answerAgent(int(k.String()[0] - '0'))
		case "i":
			return m, m.replyAgent()
		case "x":
			return m, m.interruptAgent()
		case "N":
			return m, m.newAgentCmd()
		}
		return m, nil
	}
	switch k.String() {
	case " ":
		m.toggleSelect()
		return m, m.listMove(1)
	case "v":
		return m, m.selectRange()
	case "N":
		return m, m.newCmd()
	case "r":
		return m, m.renameCmd()
	case "d":
		return m, m.killCmd()
	case "u":
		return m, m.undoCmd()
	case "m":
		return m, m.moveCmd()
	case "s":
		return m, m.swapCmd()
	case "b":
		return m, m.breakCmd()
	case "z":
		return m, m.zoomCmd()
	case "L":
		return m, m.layoutCmd()
	}
	return m, nil
}

// listFilterKey handles a key while the filter input has the keys: typing
// narrows the list, ↑ ↓ move, enter keeps the filter, esc clears it.
func (m *Model) listFilterKey(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "enter":
		m.filtering = false
		return m.listSync()
	case "esc":
		m.filtering, m.listFilter = false, ""
		return m.listSync()
	case "down":
		return m.listMove(1)
	case "up":
		return m.listMove(-1)
	}
	var cmd tea.Cmd
	m.filterIn, cmd = m.filterIn.Update(k)
	m.listFilter = strings.TrimSpace(m.filterIn.Value())
	m.listSel = "" // the first match
	m.listIdx = 0
	return tea.Batch(cmd, m.listSync())
}

// --- view ---------------------------------------------------------------------

// viewList renders the sidebar: header, (focused) the filter line, the list,
// and a footer: key hints for the cursor row when focused, else the counts.
func (m *Model) viewList() string {
	w, h := m.w, m.h
	head := []string{m.compactHeader(w)}
	if m.focused {
		switch {
		case m.filtering:
			m.filterIn.Width = max(1, w-4)
			head = append(head, " "+m.filterIn.View())
		case m.listFilter != "":
			head = append(head, renderRow([]seg{{" / ", stBlue}, {m.listFilter, stText}},
				[]seg{{"esc clear ", stFaint}}, w, 0))
		default:
			head = append(head, renderRow([]seg{{" / ", stBlue}, {"filter…", stFaint}}, nil, w, 0))
		}
	}
	head = append(head, stFaint.Render(strings.Repeat("─", w)))

	var foot []string
	if m.focused {
		foot = append([]string{stFaint.Render(strings.Repeat("─", w))},
			indent(hintLines(listHints[m.listContext()], []kd{{"?", "keys"}}, w-2, 2))...)
	} else {
		nw := 0
		for _, s := range m.sessions() {
			nw += len(s.Windows)
		}
		foot = []string{stFaint.Render(strings.Repeat("─", w)),
			" " + stFaint.Render(fmt.Sprintf("%d sessions · %d windows", len(m.sessions()), nw))}
	}

	rows := m.listRows()
	cur := -1
	if m.focused {
		cur = m.listCursor(rows)
	}
	_, curWin := m.sidebarSession()
	var body []string
	curTop, curBot := -1, -1
	for i, r := range rows {
		if i == cur {
			curTop = len(body)
		}
		if !m.focused && r.kind == rowWindow && r.win == curWin {
			curTop = len(body)
		}
		body = append(body, m.renderListRow(r, w, i == cur)...)
		if i == cur || (!m.focused && r.kind == rowWindow && r.win == curWin) {
			curBot = len(body)
		}
	}

	avail := max(1, h-len(head)-len(foot))
	// keep the cursor (or your window) in view
	if curTop >= 0 {
		if curTop < m.listOff {
			m.listOff = curTop
		}
		if curBot > m.listOff+avail {
			m.listOff = curBot - avail
		}
	}
	m.listOff = clamp(m.listOff, 0, max(0, len(body)-avail))
	if m.listOff > 0 && m.listOff < len(body) {
		body = body[m.listOff:]
	}
	if len(body) > avail {
		body = body[:avail]
	}
	for len(body) < avail {
		body = append(body, "")
	}
	lines := append(append(head, body...), foot...)
	for i, l := range lines {
		if pad := w - ansi.StringWidth(l); pad > 0 {
			lines[i] = l + strings.Repeat(" ", pad)
		}
	}
	return strings.Join(lines[:min(len(lines), h)], "\n")
}

func indent(lines []string) []string {
	for i, l := range lines {
		lines[i] = " " + l
	}
	return lines
}

// renderListRow draws one row; cur is the cursor row.
func (m *Model) renderListRow(r listRow, w int, cur bool) []string {
	curSess, curWin := m.sidebarSession()
	switch r.kind {
	case rowHeading:
		if r.title == nil {
			return []string{""}
		}
		if r.title[0].text == " SESSIONS " {
			return []string{r.title[0].st.Render(r.title[0].text) +
				stFaint.Render(strings.Repeat("─", max(0, w-len(" SESSIONS "))))}
		}
		return []string{renderRow(r.title, r.right, w, 0)}
	case rowAgent:
		return m.agentCard(r.a, r.pane, w, cur)
	}

	var bg lipgloss.TerminalColor
	bold := false
	mark := seg{" ", stText}
	if m.isSelected(map[rowKind]int{rowSession: panelSessions, rowWindow: panelWindows, rowPane: panelPanes}[r.kind], rowID(r)) {
		mark = seg{"◆", stMauve.Bold(true)}
	}
	switch r.kind {
	case rowSession:
		s := r.sess
		as, _ := m.sessionAgents(s)
		open := m.isOpen(sessKey(s), s == curSess)
		name := stSub.Bold(true)
		switch {
		case s == curSess:
			name, bg = stText.Bold(true), cSurface0
		case len(as) == 0:
			name = stFaint
		}
		if cur {
			bg, bold = cCursor, true
		}
		right := append([]seg{{fmt.Sprintf("%dw ", len(s.Windows)), stFaint}}, m.sessionBadge(s)...)
		return []string{renderRowBg([]seg{mark, {arrow(open, true), stBlue}, {s.Name, name}}, right, w, bg, bold)}

	case rowWindow:
		win := r.win
		var right []seg
		if a := m.worstAgent(win.Panes); a != nil {
			right = []seg{kindIcon(a.Kind),
				{" " + agentGlyph(a) + " " + a.Label() + " " + agent.Age(a.Since, clock()) + " ", agentStyle(a)}}
		} else if p := win.ActivePane(); p != nil {
			right = []seg{{p.Command + " ", stFaint}}
		}
		name, idx := stSub, stFaint
		if win == curWin {
			name, idx, bg = stText, stBlue, cSurface1
		}
		if cur {
			bg, bold = cCursor, true
		}
		return []string{renderRowBg([]seg{mark, {"  " + arrow(m.isOpen(winKey(win), false), len(win.Panes) > 1), stBlue},
			{fmt.Sprintf("%d ", win.Index), idx}, {win.Name, name}}, right, w, bg, bold)}

	default: // pane
		p := r.pane
		right := []seg{{fmt.Sprintf("%dx%d ", p.Width, p.Height), stFaint}}
		if a := m.agents[p.ID]; a != nil {
			right = []seg{kindIcon(a.Kind), {" " + agentGlyph(a) + " " + a.Label() + " ", agentStyle(a)}}
		}
		name := stSub
		if p.Active {
			name = stText
		}
		if cur {
			bg, bold = cCursor, true
		}
		return []string{renderRowBg([]seg{mark, {"      " + p.ID + " ", stFaint}, {p.Command, name}}, right, w, bg, bold)}
	}
}

func rowID(r listRow) string {
	switch r.kind {
	case rowSession:
		return r.sess.ID
	case rowWindow:
		return r.win.ID
	case rowPane:
		return r.pane.ID
	}
	return ""
}

// arrow is the fold marker: ▾ open, ▸ closed, nothing when there is
// nothing to unfold.
func arrow(open, foldable bool) string {
	switch {
	case !foldable:
		return "  "
	case open:
		return "▾ "
	}
	return "▸ "
}

// agentCard is one agent: a bar in its state's color (thin when idle), its
// icon, place and age, and below what it waits for (?) or is doing (▷). An
// agent that needs you gets a red-tinted card; the cursor card is blue with
// a ▶.
//
//	┃✻ claude        api:3  2m
//	┃  ? Bash(go test ./...)
func (m *Model) agentCard(a *agent.Agent, p *tmux.Pane, w int, cur bool) []string {
	st := agentStyle(a)
	idle := urgency(a) == 0
	bar, bar2, name, text := seg{"┃", st}, seg{"┃", st}, stText.Bold(true), stSub
	if idle {
		bar, bar2, name, text = seg{"│", st}, seg{"│", st}, stSub, stFaint
	}
	var bg lipgloss.TerminalColor
	if a.State == agent.Blocked {
		bg = cRedBg
	}
	if cur {
		bg, bar, bar2, name = cCursor, seg{"▶", stBlue.Bold(true)}, seg{" ", stText}, stText.Bold(true)
	}
	loc := fmt.Sprintf("%s:%d", p.Window.Session.Name, p.Window.Index)
	out := []string{renderRowBg(
		[]seg{bar, kindIcon(a.Kind), {" " + string(a.Kind), name}},
		[]seg{{loc + "  ", stFaint}, {agent.Age(a.Since, clock()) + " ", st}}, w, bg, false)}
	var l2 []seg
	switch {
	case a.Waiting != "":
		l2 = []seg{{"  ? ", st}, {a.Waiting, st}}
	case a.Summary != "":
		l2 = []seg{{"  ▷ ", st}, {a.Summary, text}}
	}
	if l2 != nil {
		out = append(out, renderRowBg(append([]seg{bar2}, l2...), nil, w, bg, false))
	}
	return out
}

// listContext names the cursor row for the help and the key hints.
func (m *Model) listContext() string {
	switch m.listRowKind() {
	case rowAgent:
		return "Agent"
	case rowSession:
		return "Session"
	case rowWindow:
		return "Window"
	case rowPane:
		return "Pane"
	}
	return "Session"
}

// listHints are the keys for each kind of row, most used first.
var listHints = map[string][]kd{
	"Agent": {{"1/2/3", "answer"}, {"i", "reply"}, {"x", "stop"}, {"enter", "jump"}, {"B", "next blocked"},
		{"N", "new agent"}, {"tab", "sessions"}, {"/", "filter"}, {"f", "find"}, {"n", "project"}},
	"Session": {{"enter", "switch"}, {"l/h", "fold"}, {"N", "new"}, {"r", "rename"}, {"d", "kill"},
		{"u", "undo"}, {"space", "select"}, {"tab", "agents"}, {"/", "filter"}, {"n", "project"}, {"f", "find"}},
	"Window": {{"enter", "switch"}, {"l/h", "panes"}, {"N", "new window"}, {"r", "rename"}, {"d", "kill"},
		{"m", "move"}, {"s", "swap"}, {"L", "layout"}, {"u", "undo"}, {"space", "select"}, {"/", "filter"}},
	"Pane": {{"enter", "switch"}, {"N", "split"}, {"r", "title"}, {"d", "kill"}, {"z", "zoom"},
		{"b", "break out"}, {"m", "join"}, {"s", "swap"}, {"u", "undo"}, {"space", "select"}},
}
