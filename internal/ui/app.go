// Package ui implements the lazymux terminal UI (Bubble Tea).
package ui

import (
	"fmt"
	"github.com/charmbracelet/bubbles/textinput"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/afamrk/lazymux/internal/agent"
	"github.com/afamrk/lazymux/internal/tmux"
)

const (
	panelSessions = iota
	panelWindows
	panelPanes
	numPanels
)

const refreshEvery = time.Second

// Widest left column: the tmux page's panels and the Agents page's list.
const (
	maxLeftW       = 60
	maxAgentsLeftW = 64
)

// Model is the root Bubble Tea model.
type Model struct {
	c    *tmux.Client
	tree *tmux.Tree

	// Selection is kept as ids so it survives refreshes that reorder or
	// remove items.
	selSess, selWin, selPane string

	focus int
	page  int // pageTmux or pageAgents

	agentTab   int // All / Blocked / Working / Done
	agentFocus int // agent list or Activity

	det       *agent.Detector
	leader    *agent.Leader // held by the one lazymux that notifies and writes the status file
	notifier  *agent.Notifier
	agents    map[string]*agent.Agent // detected agents by pane id
	detecting bool                    // a detection is running
	agentSel  string                  // selected agent row: pane id, or "s:<session id>" for a header
	collapsed map[string]bool         // session ids folded in the agent list
	agentOff  int                     // agent list scroll (lines)
	actOff    int                     // Activity scroll from the bottom (events)
	off       [numPanels]int

	preview    string
	previewFor string

	w, h int
	err  error

	selfPane string // lazymux's own pane ($TMUX_PANE), never previewed

	sidebar bool // running as `lazymux sidebar`
	focused bool // sidebar pane has focus (shows the full UI)

	compactCur int // line of the current window in the compact view (for scrolling)

	modal    modal // open dialog, if any
	toast    toastState
	toastSeq int
	undo     [][]*tmux.Snapshot // undo steps (one per kill, possibly several items), most recent last

	filter       [numPanels]string // per-panel filter (/)
	filtering    bool              // the filter input of the focused panel has the keys
	filterIn     textinput.Model
	agentFilter  string // Agents page filter (/)
	pendingAgent string // a just-started agent to select once detected

	selected []selRef // multi-selection (space / v), all in one panel
	anchor   string   // id of the last item toggled with space (start of a v range)

	// AttachTo is set when lazymux runs outside tmux and the user picked a
	// session; the caller execs `tmux attach` after the program exits.
	AttachTo *tmux.Session
}

// New creates the model. The initial selection is the client's current
// session/window/pane, when known.
func New(c *tmux.Client) *Model {
	s, w, p := c.Current()
	return &Model{c: c, selSess: s, selWin: w, selPane: p, selfPane: os.Getenv("TMUX_PANE"),
		det: agent.NewDetector(c), collapsed: map[string]bool{},
		leader: agent.NewLeader(c.ServerName()), notifier: agent.NewNotifier()}
}

type (
	treeMsg struct {
		tree *tmux.Tree
		err  error
	}
	previewMsg struct {
		pane    string
		content string
		err     error
	}
	tickMsg   struct{}
	switchMsg struct{ err error }
)

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.fetchTree(), tick())
}

func tick() tea.Cmd {
	return tea.Tick(refreshEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m *Model) fetchTree() tea.Cmd {
	c := m.c
	return func() tea.Msg {
		t, err := c.ListTree()
		return treeMsg{t, err}
	}
}

func (m *Model) fetchPreview() tea.Cmd {
	p := m.previewTarget()
	if p == nil || p.ID == m.selfPane {
		return nil
	}
	pane, c := p.ID, m.c
	return func() tea.Msg {
		out, err := c.CapturePane(pane)
		return previewMsg{pane, out, err}
	}
}

// previewTarget is the pane shown on the right. Capturing lazymux's own pane
// would only show lazymux, so when the session or window selection resolves to
// it, the pane the user came from is shown instead: the window's last pane,
// another pane of the window, or the active pane of the session's last (or
// another) window. Nil means there is nothing else to show.
func (m *Model) previewTarget() *tmux.Pane {
	if m.page == pageAgents {
		_, p := m.curAgent()
		return p
	}
	_, _, _, _, p, _ := m.cur()
	if p == nil || p.ID != m.selfPane || m.focus == panelPanes {
		return p
	}
	if alt := otherPane(p.Window, p.ID); alt != nil {
		return alt
	}
	s := p.Window.Session
	var other *tmux.Window
	for _, w := range s.Windows {
		if w == p.Window {
			continue
		}
		if other == nil || w.Last {
			other = w
		}
	}
	if other != nil {
		if ap := other.ActivePane(); ap != nil && ap.ID != m.selfPane {
			return ap
		}
		if alt := otherPane(other, m.selfPane); alt != nil {
			return alt
		}
	}
	return p
}

// otherPane returns the window's last pane, or any pane other than skip.
func otherPane(w *tmux.Window, skip string) *tmux.Pane {
	var other *tmux.Pane
	for _, x := range w.Panes {
		if x.ID == skip {
			continue
		}
		if other == nil || x.Last {
			other = x
		}
	}
	return other
}

// --- selection helpers -----------------------------------------------------

func (m *Model) sessions() []*tmux.Session {
	if m.tree == nil {
		return nil
	}
	return m.tree.Sessions
}

// cur resolves the selected ids to items and indices, falling back to the
// first / active item when an id no longer exists.
func (m *Model) cur() (s *tmux.Session, si int, w *tmux.Window, wi int, p *tmux.Pane, pi int) {
	ss := m.visSessions()
	if len(ss) == 0 {
		return
	}
	s, si = ss[0], 0
	for i, x := range ss {
		if x.ID == m.selSess {
			s, si = x, i
		}
	}
	ws := m.visWindows(s)
	if len(ws) == 0 {
		return
	}
	w = ws[0]
	if a := s.ActiveWindow(); indexOf(ws, a) >= 0 && ws[indexOf(ws, a)] == a {
		w = a
	}
	for _, x := range ws {
		if x.ID == m.selWin {
			w = x
		}
	}
	wi = indexOf(ws, w)
	ps := m.visPanes(w)
	if len(ps) == 0 {
		return
	}
	p = ps[0]
	if a := w.ActivePane(); indexOf(ps, a) >= 0 && ps[indexOf(ps, a)] == a {
		p = a
	}
	for _, x := range ps {
		if x.ID == m.selPane {
			p = x
		}
	}
	pi = indexOf(ps, p)
	return
}

func indexOf[T comparable](xs []T, x T) int {
	for i, y := range xs {
		if y == x {
			return i
		}
	}
	return 0
}

// normalize rewrites the selected ids to the resolved items.
func (m *Model) normalize() {
	s, _, w, _, p, _ := m.cur()
	m.selSess, m.selWin, m.selPane = "", "", ""
	if s != nil {
		m.selSess = s.ID
	}
	if w != nil {
		m.selWin = w.ID
	}
	if p != nil {
		m.selPane = p.ID
	}
}

// move moves the cursor of the focused panel by delta (clamped) and cascades
// the selection to child panels.
func (m *Model) move(delta int) {
	s, si, w, wi, _, pi := m.cur()
	if s == nil {
		return
	}
	switch m.focus {
	case panelSessions:
		ss := m.visSessions()
		m.selSess = ss[clamp(si+delta, 0, len(ss)-1)].ID
		m.selWin, m.selPane = "", ""
	case panelWindows:
		ws := m.visWindows(s)
		if len(ws) == 0 {
			return
		}
		m.selWin = ws[clamp(wi+delta, 0, len(ws)-1)].ID
		m.selPane = ""
	case panelPanes:
		ps := m.visPanes(w)
		if len(ps) == 0 {
			return
		}
		m.selPane = ps[clamp(pi+delta, 0, len(ps)-1)].ID
	}
	m.normalize()
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return max(lo, min(hi, v))
}

// --- update ----------------------------------------------------------------

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil

	case treeMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.tree = msg.tree
		m.normalize()
		m.pruneSelection()
		if t := m.previewTarget(); t != nil && m.previewFor != t.ID {
			return m, m.fetchPreview()
		}
		return m, nil

	case previewMsg:
		if t := m.previewTarget(); t != nil && msg.pane == t.ID && msg.err == nil {
			m.preview, m.previewFor = msg.content, msg.pane
		}
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.fetchTree(), m.fetchPreview(), m.detectAgents(), m.checkFocus(), tick())

	case agentsMsg:
		return m, m.setAgents(msg.agents)

	case tea.FocusMsg:
		return m, m.setFocused(true, 0)
	case tea.BlurMsg:
		return m, m.setFocused(false, 0)
	case focusMsg:
		return m, m.reconcile(msg)

	case switchMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		if m.sidebar { // the sidebar stays; focus moved to the target pane
			return m, m.fetchTree()
		}
		return m, tea.Quit

	case actionMsg:
		return m, m.handleAction(msg)

	case projectsMsg:
		m.toast = toastState{}
		return m, m.openProjects(msg.dirs)

	case projectOpenedMsg:
		return m, tea.Batch(m.fetchTree(), m.projectOpened(msg))

	case dirPickedMsg:
		if msg.err != nil {
			return m, m.setToast("✗ fzf: "+msg.err.Error(), true)
		}
		if d, ok := m.modal.(*inputModal); ok && msg.dir != "" {
			d.setDir(msg.dir)
		}
		return m, nil

	case toastExpireMsg:
		if msg.id == m.toast.id {
			m.toast = toastState{}
		}
		return m, nil

	case tea.KeyMsg:
		if m.modal != nil {
			cmd, done := m.modal.update(msg)
			if done {
				m.modal = nil
			}
			return m, cmd
		}
		return m.handleKey(msg)
	}
	// keep text input cursors etc. alive inside dialogs
	return m, nil
}

func (m *Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.filtering { // the filter input gets everything, typed text included
		return m, m.handleFilterKey(k)
	}
	// Bubble Tea delivers runes that arrive together (fast typing, key
	// repeat when holding j) as one KeyMsg; handle them one by one.
	if k.Type == tea.KeyRunes && len(k.Runes) > 1 && !k.Paste {
		var cmds []tea.Cmd
		for _, r := range k.Runes {
			_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: k.Alt})
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}
	if k.String() == "esc" && m.page == pageAgents && m.agentFilter != "" {
		m.agentFilter = ""
		m.normalizeAgents()
		return m, m.fetchPreview()
	}
	if k.String() == "esc" && m.page == pageTmux { // esc clears selection, then filter
		if len(m.selected) > 0 {
			m.selected = nil
			return m, nil
		}
		if m.filter[m.focus] != "" {
			m.filter[m.focus] = ""
			return m, m.afterSelectionChange()
		}
	}
	if m.sidebar {
		switch k.String() {
		case "q", "esc":
			return m, m.leaveSidebar()
		case "Q":
			return m, m.sidebarOff()
		}
	}
	switch k.String() {
	case "a":
		return m, m.togglePage()
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	case "f", "ctrl+p":
		m.openJump()
		return m, nil
	case "N":
		return m, m.loadProjects()
	case "?":
		m.openHelp()
		return m, nil
	}
	if m.page == pageAgents {
		_, cmd := m.handleAgentKey(k.String())
		return m, cmd
	}
	prevPane, prevTarget := m.selPane, ""
	if t := m.previewTarget(); t != nil {
		prevTarget = t.ID
	}
	switch k.String() {
	case " ":
		m.toggleSelect()
		return m, nil
	case "v":
		return m, m.selectRange()
	case "/":
		m.startFilter()
		return m, nil
	case "n":
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
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "g", "home":
		m.move(-1 << 20)
	case "G", "end":
		m.move(1 << 20)
	case "tab", "l", "right":
		m.focus = (m.focus + 1) % numPanels
	case "shift+tab", "h", "left":
		m.focus = (m.focus + numPanels - 1) % numPanels
	case "1":
		m.focus = panelSessions
	case "2":
		m.focus = panelWindows
	case "3":
		m.focus = panelPanes
	case "enter":
		return m, m.switchCmd()
	}
	if t := m.previewTarget(); t != nil && (m.selPane != prevPane || t.ID != prevTarget) {
		return m, m.fetchPreview()
	}
	return m, nil
}

// switchCmd jumps to the selection: the session (panel 1), window (2) or
// pane (3). Inside tmux it switches the client; outside it prepares an attach.
func (m *Model) switchCmd() tea.Cmd {
	s, _, w, _, p, _ := m.cur()
	if s == nil {
		return nil
	}
	switch m.focus {
	case panelSessions:
		w, p = nil, nil
	case panelWindows:
		p = nil
	}
	c := m.c
	if !tmux.InsideTmux() && c.ClientTTY == "" {
		m.AttachTo = s
		return func() tea.Msg { return switchMsg{c.Select(s, w, p)} }
	}
	return func() tea.Msg { return switchMsg{c.SwitchTo(s, w, p)} }
}

// --- view ------------------------------------------------------------------

func (m *Model) View() string {
	if m.w == 0 || m.h == 0 {
		return ""
	}
	if m.tree == nil {
		if m.err != nil {
			return stRed.Render("lazymux: "+m.err.Error()) + "\n" + stSub.Render("is a tmux server running? (q to quit)")
		}
		return stSub.Render("loading…")
	}

	if m.compactView() {
		return m.viewCompact()
	}
	bodyH := m.h - 1
	leftPct := 30
	if m.sidebar { // the pane is already narrow: give the panels more of it
		leftPct = 45
	}
	// the panels need ~60 columns at most; on wide screens the rest goes to
	// the preview (as in the mockups)
	leftW := clamp(min(m.w*leftPct/100, maxLeftW), 32, m.w-30)
	if m.w < 64 { // too narrow for a preview
		leftW = m.w
	}

	var left string
	if m.page == pageAgents {
		// agent rows carry a summary line, so this page's list is wider
		if leftW < m.w {
			leftW = max(leftW, clamp(min(m.w*36/100, maxAgentsLeftW), min(46, m.w-30), m.w-30))
		}
		left = m.viewAgentsLeft(leftW, bodyH)
	} else {
		left = m.viewLeft(leftW, bodyH)
	}
	body := left
	if leftW < m.w {
		preview := m.viewPreview
		if m.page == pageAgents {
			preview = m.viewAgentsPreview
		}
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, preview(m.w-leftW, bodyH))
	}
	view := body + "\n" + m.viewBar()
	if m.modal != nil {
		view = center(dim(view), m.modal.view(m.w, m.h), m.w, m.h)
	}
	if m.toast.text != "" {
		t := m.viewToast()
		// bottom-right, inside the preview's border and above the key bar
		view = overlay(view, t, max(0, m.w-lipgloss.Width(t)-2), max(0, m.h-2-lipgloss.Height(t)))
	}
	return view
}

func (m *Model) viewToast() string {
	color, st := cGreen, stText
	if m.toast.isErr {
		color, st = cRed, stRed
	}
	text := ansi.Truncate(m.toast.text, max(10, m.w/2), "…")
	w := ansi.StringWidth(text) + 4
	return renderFrame("", "", false, []string{" " + st.Render(text) + " "}, w, 3, color)
}

// panelHeights shares the left column between the panels. The sizes do not
// depend on focus: each panel wants its items plus the border, a panel that
// needs less than an equal share keeps only that, and the spare rows are
// split among the others.
func (m *Model) panelHeights(bodyH int, counts [numPanels]int) [numPanels]int {
	var hs [numPanels]int
	const minH = 3
	want := [numPanels]int{}
	for i := range want {
		want[i] = max(minH, counts[i]+2)
	}
	left, open := bodyH, []int{0, 1, 2}
	for len(open) > 0 {
		share := left / len(open)
		var rest []int
		for _, i := range open {
			if want[i] <= share {
				hs[i] = want[i]
				left -= want[i]
			} else {
				rest = append(rest, i)
			}
		}
		if len(rest) == len(open) { // everyone wants more: split what is left
			for j, i := range rest {
				hs[i] = left / len(rest)
				if j >= len(rest)-left%len(rest) {
					hs[i]++
				}
			}
			left = 0
			break
		}
		open = rest
	}
	// everything fits: give the spare rows away so the column stays filled
	for j := 0; left > 0; j++ {
		hs[numPanels-1-j%numPanels]++
		left--
	}
	for i := range hs {
		hs[i] = max(minH, hs[i])
	}
	return hs
}

func (m *Model) viewLeft(w, bodyH int) string {
	s, si, win, wi, _, pi := m.cur()
	ss, ws, ps := m.visSessions(), m.visWindows(s), m.visPanes(win)
	counts := [numPanels]int{len(ss), len(ws), len(ps)}
	for i := range counts {
		if m.showFilter(i) {
			counts[i]++
		}
	}
	hs := m.panelHeights(bodyH, counts)
	inner := w - 2

	// [1] Sessions
	rows := m.panelRows(panelSessions, hs[0], inner, len(ss), si, func(i, sel int) string {
		return m.sessionRow(ss[i], inner, sel)
	})
	p1 := renderBox(m.numTitle(1, "Sessions", panelSessions)+" ", m.footer(panelSessions, si, len(ss)), rows, w, hs[0], m.focus == panelSessions)

	// [2] Windows
	rows = m.panelRows(panelWindows, hs[1], inner, len(ws), wi, func(i, sel int) string {
		return m.windowRow(ws[i], inner, sel)
	})
	title := m.numTitle(2, "Windows", panelWindows)
	if s != nil {
		title += stFaint.Render(" (" + s.Name + ") ")
	}
	p2 := renderBox(title, m.footer(panelWindows, wi, len(ws)), rows, w, hs[1], m.focus == panelWindows)

	// [3] Panes
	rows = m.panelRows(panelPanes, hs[2], inner, len(ps), pi, func(i, sel int) string {
		return m.paneRow(ps[i], inner, sel)
	})
	title = m.numTitle(3, "Panes", panelPanes)
	if win != nil {
		title += stFaint.Render(fmt.Sprintf(" (%d:%s) ", win.Index, win.Name))
	}
	p3 := renderBox(title, m.footer(panelPanes, pi, len(ps)), rows, w, hs[2], m.focus == panelPanes)

	return lipgloss.JoinVertical(lipgloss.Left, p1, p2, p3)
}

// showFilter reports whether a panel shows its filter line.
func (m *Model) showFilter(panel int) bool {
	return m.filter[panel] != "" || (m.filtering && m.focus == panel)
}

// panelRows renders a panel's body: the filter line (if any) and the visible
// part of its list, scrolled to keep the cursor in view.
func (m *Model) panelRows(panel, h, inner, n, cur int, row func(i, sel int) string) []string {
	var rows []string
	avail := h - 2
	if m.showFilter(panel) {
		if m.filtering && m.focus == panel {
			m.filterIn.Width = max(1, inner-4)
			rows = append(rows, " "+m.filterIn.View())
		} else {
			rows = append(rows, " "+stBlue.Render("/ ")+stText.Render(m.filter[panel]))
		}
		avail--
	}
	m.off[panel] = scrollWindow(m.off[panel], cur, avail, n)
	for i := m.off[panel]; i < n && i < m.off[panel]+avail; i++ {
		rows = append(rows, row(i, m.selState(panel, i == cur)))
	}
	if n == 0 && m.filter[panel] != "" {
		rows = append(rows, " "+stFaint.Render("no match"))
	}
	return rows
}

// footer is the bottom-border text of a panel: "3 of 6 · 2 selected".
func (m *Model) footer(panel, cur, n int) string {
	pos := 0
	if n > 0 {
		pos = cur + 1
	}
	f := fmt.Sprintf("%d of %d", pos, n)
	if k := m.selectedIn(panel); k > 0 {
		f += fmt.Sprintf(" · %d selected", k)
	}
	return f
}

// selectedIn counts selected items of a panel.
func (m *Model) selectedIn(panel int) int {
	if len(m.selected) == 0 || m.selected[0].panel != panel {
		return 0
	}
	return len(m.selected)
}

func (m *Model) selState(panel int, selected bool) int {
	switch {
	case !selected:
		return 0
	case m.focus == panel:
		return 2
	default:
		return 1
	}
}

func (m *Model) numTitle(n int, name string, panel int) string {
	if m.focus == panel {
		return stTitleOn.Render(fmt.Sprintf("[%d] %s", n, name))
	}
	return stSub.Render(fmt.Sprintf("[%d] %s", n, name))
}

func (m *Model) textStyle(panel int) lipgloss.Style {
	if m.focus == panel {
		return stText
	}
	return stSub
}

func (m *Model) sessionRow(s *tmux.Session, w, sel int) string {
	mark := seg{"  ", stText}
	if s.Attached {
		mark = seg{"● ", stGreen}
	}
	_, idx := m.matchSession(s)
	left := append([]seg{m.markSeg(panelSessions, s.ID), mark}, hlSegs(s.Name, idx, m.textStyle(panelSessions), stMatch)...)
	right := append(m.sessionBadge(s), seg{fmt.Sprintf("%dw ", len(s.Windows)), stFaint})
	return renderRow(left, right, w, sel)
}

func (m *Model) windowRow(win *tmux.Window, w, sel int) string {
	_, idx := m.matchWindow(win)
	left := append([]seg{m.markSeg(panelWindows, win.ID), {fmt.Sprintf("%d ", win.Index), stBlue}},
		hlSegs(win.Name, idx, m.textStyle(panelWindows), stMatch)...)
	right := m.windowBadge(win)
	if n := len(win.Panes); n > 1 {
		right = append(right, seg{fmt.Sprintf("%dp ", n), stFaint})
	}
	if p := win.ActivePane(); p != nil {
		right = append(right, seg{p.Command, stSub})
	}
	if win.Active {
		right = append(right, seg{" *", stMauve})
	} else {
		right = append(right, seg{"  ", stSub})
	}
	right = append(right, seg{" ", stSub})
	return renderRow(left, right, w, sel)
}

func (m *Model) paneRow(p *tmux.Pane, w, sel int) string {
	_, idx := m.matchPane(p)
	left := append([]seg{m.markSeg(panelPanes, p.ID), {p.ID + " ", stPeach}},
		hlSegs(p.Command, idx, m.textStyle(panelPanes), stMatch)...)
	var right []seg
	if a := m.agents[p.ID]; a != nil {
		right = append(right, seg{agentGlyph(a) + " " + a.Label() + " ", agentStyle(a)})
	}
	right = append(right, seg{fmt.Sprintf("%dx%d", p.Width, p.Height), stFaint})
	if p.Active {
		right = append(right, seg{" *", stMauve})
	} else {
		right = append(right, seg{"  ", stSub})
	}
	right = append(right, seg{" ", stSub})
	return renderRow(left, right, w, sel)
}

// markSeg is the one-cell gutter at the start of a row: ◆ when selected.
func (m *Model) markSeg(panel int, id string) seg {
	if m.isSelected(panel, id) {
		return seg{"◆", stMauve}
	}
	return seg{" ", stText}
}

func (m *Model) viewPreview(w, h int) string {
	p := m.previewTarget()
	title := stSub.Render("preview ")
	var lines []string
	if p != nil {
		title = stText.Render(p.Target()) + stFaint.Render(" · ") + stSub.Render(p.Command) + " "
		switch {
		case p.ID == m.selfPane:
			lines = []string{stFaint.Render("(this is lazymux itself)")}
		case m.previewFor == p.ID:
			lines = tmux.TrimCapture(m.preview, h-2)
		}
	}
	for i, l := range lines {
		lines[i] = " " + l
	}
	return renderBox(title, "", lines, w, h, false)
}

func (m *Model) viewBar() string {
	if m.err != nil {
		return ansi.Truncate(stRed.Render(" "+m.err.Error()), m.w, "…")
	}
	var keys []kd
	if n := m.selectedIn(m.focus); n > 0 && m.page == pageTmux {
		keys = append(keys, kd{"◆", fmt.Sprintf("%d selected", n)})
		switch m.focus {
		case panelWindows:
			keys = append(keys, kd{"m", "move"})
			if n == 1 {
				keys = append(keys, kd{"s", "swap with cursor"})
			}
		case panelPanes:
			keys = append(keys, kd{"m", "join"})
			if n == 1 {
				keys = append(keys, kd{"s", "swap with cursor"})
			}
		}
		keys = append(keys, kd{"d", "kill all"}, kd{"space", "select"}, kd{"v", "range"}, kd{"esc", "clear"}, kd{"?", "help"})
		return m.renderBar(keys)
	}
	keys = append(keys, kd{"enter", "switch"})
	if m.sidebar {
		keys = append(keys, kd{"esc", "back"})
	}
	keys = append(keys, kd{"n", "new"}, kd{"r", "rename"}, kd{"d", "kill"})
	if len(m.undo) > 0 {
		keys = append(keys, kd{"u", "undo"})
	}
	switch m.focus {
	case panelSessions:
		keys = append(keys, kd{"/", "filter"}, kd{"f", "find"}, kd{"space", "select"})
	case panelWindows:
		keys = append(keys, kd{"m", "move"}, kd{"/", "filter"}, kd{"space", "select"}, kd{"L", "layout"})
	case panelPanes:
		keys = append(keys, kd{"z", "zoom"}, kd{"b", "break"}, kd{"m", "join"}, kd{"/", "filter"}, kd{"space", "select"})
	}
	last := kd{"q", "quit"}
	if m.sidebar {
		last = kd{"Q", "close sidebar"}
	}
	keys = append(keys, kd{"a", "agents"}, kd{"?", "help"}, last)
	if m.page == pageAgents {
		keys = m.agentBar(last)
	}
	return m.renderBar(keys)
}

// kd is one key hint of the bottom bar.
type kd struct{ k, d string }

// renderBar lays out key hints on the left (dropping what does not fit) and
// the page indicator on the right.
func (m *Model) renderBar(keys []kd) string {
	right := m.pageIndicator()
	avail := m.w - ansi.StringWidth(right) - 1
	left := ""
	for i, x := range keys {
		ks := stKey
		if x.k == "◆" {
			ks = stMauve
		}
		part := ks.Render(x.k) + " " + stSub.Render(x.d)
		if i > 0 {
			part = stFaint.Render(" · ") + part
		}
		if x.k == "│" { // a group divider
			part = stFaint.Render("  │ ")
		} else if i > 0 && keys[i-1].k == "│" {
			part = ks.Render(x.k) + " " + stSub.Render(x.d)
		}
		if ansi.StringWidth(left+part)+1 > avail {
			break // drop what does not fit; ? help lists everything
		}
		left += part
	}
	left = " " + left
	return left + strings.Repeat(" ", max(1, m.w-ansi.StringWidth(left)-ansi.StringWidth(right))) + right
}
