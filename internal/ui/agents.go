package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/afamrk/lazymux/internal/agent"
	"github.com/afamrk/lazymux/internal/tmux"
)

// Pages: `a` switches between the tmux page (Sessions / Windows / Panes) and
// the Agents page (agent list / Activity).
const (
	pageTmux = iota
	pageAgents
)

// State tabs of the Agents page, switched with [ / ].
const (
	agentAll = iota
	agentBlocked
	agentWorking
	agentDone
	numAgentTabs
)

// Panels of the Agents page.
const (
	agentPanelList = iota
	agentPanelActivity
	numAgentPanels
)

var agentTabNames = [numAgentTabs]string{"All", "Blocked", "Working", "Done"}

// agentTabStyle colors a tab's count by the state it stands for.
var agentTabStyle = [numAgentTabs]lipgloss.Style{
	stText,
	stRed,
	stYellow,
	stGreen,
}

// agentGlyph / agentStyle: ⣿ blocked (red), ⣾ working (yellow), ● done
// (green: finished, not looked at yet), ○ idle (dim: seen).
func agentGlyph(a *agent.Agent) string {
	if a.State == agent.Idle && !a.Unseen {
		return "○"
	}
	return stateGlyph(a.State)
}

func agentStyle(a *agent.Agent) lipgloss.Style {
	if a.State == agent.Idle && !a.Unseen {
		return stFaint
	}
	return stateStyle(a.State)
}

// urgency orders agents for badges: blocked, working, done, idle.
func urgency(a *agent.Agent) int {
	switch {
	case a.State == agent.Blocked:
		return 3
	case a.State == agent.Working:
		return 2
	case a.Done():
		return 1
	}
	return 0
}

func stateGlyph(s agent.State) string {
	switch s {
	case agent.Blocked:
		return "⣿"
	case agent.Working:
		return "⣾"
	}
	return "●"
}

func stateStyle(s agent.State) lipgloss.Style {
	switch s {
	case agent.Blocked:
		return stRed
	case agent.Working:
		return stYellow
	}
	return stGreen
}

// agentsMsg carries a detection result.
type agentsMsg struct{ agents map[string]*agent.Agent }

// detectAgents runs agent detection on the current tree (one at a time).
func (m *Model) detectAgents() tea.Cmd {
	if m.det == nil || m.tree == nil || m.detecting {
		return nil
	}
	m.detecting = true
	det, t, c, leader, notifier := m.det, m.tree, m.c, m.leader, m.notifier
	return func() tea.Msg {
		agents := det.Detect(t)
		// one lazymux per server notifies and keeps the status file
		if leader != nil && notifier != nil && leader.Acquire() {
			cfg, now := agent.LoadConfig(c), time.Now()
			for _, n := range notifier.Observe(agents, t, cfg, now) {
				go agent.Deliver(c, cfg, n)
			}
			_ = agent.WriteStatus(c.ServerName(), agent.Count(agents, now))
		}
		return agentsMsg{agents}
	}
}

// markSeen turns the selected agent from "done" into "idle" once you look
// at it on the full Agents page.
func (m *Model) markSeen() {
	if m.page != pageAgents || m.compactView() {
		return
	}
	if a, _ := m.curAgent(); a != nil && a.Unseen {
		a.Unseen = false
		if m.det != nil {
			m.det.MarkSeen(a.Pane)
		}
	}
}

func (m *Model) setAgents(a map[string]*agent.Agent) tea.Cmd {
	m.detecting = false
	prev := ""
	if t := m.previewTarget(); t != nil {
		prev = t.ID
	}
	m.agents = a
	m.normalizeAgents()
	m.markSeen()
	if t := m.previewTarget(); t != nil && t.ID != prev {
		return m.fetchPreview()
	}
	return nil
}

// togglePage flips between the tmux and Agents pages. Each page keeps its own
// focus and selection.
func (m *Model) togglePage() tea.Cmd {
	m.page = 1 - m.page
	m.normalizeAgents()
	m.markSeen()
	return m.fetchPreview()
}

// --- agent list --------------------------------------------------------------

// agentRow is one entry of the agent list: a session header (a == nil) or an
// agent.
type agentRow struct {
	key  string // "s:<session id>" for headers, the pane id for agents
	sess *tmux.Session
	a    *agent.Agent
	pane *tmux.Pane
	// header: agents of the session per state (in the current tab)
	blocked, working, done, idle int
}

// tally counts a into a header's state counts.
func (r *agentRow) tally(a *agent.Agent) {
	switch {
	case a.State == agent.Blocked:
		r.blocked++
	case a.State == agent.Working:
		r.working++
	case a.Done():
		r.done++
	default:
		r.idle++
	}
}

func inTab(a *agent.Agent, tab int) bool {
	switch tab {
	case agentBlocked:
		return a.State == agent.Blocked
	case agentWorking:
		return a.State == agent.Working
	case agentDone:
		return a.State == agent.Idle
	}
	return true
}

// sessionAgents returns the agents of a session in window / pane order,
// with their panes.
func (m *Model) sessionAgents(s *tmux.Session) ([]*agent.Agent, []*tmux.Pane) {
	var as []*agent.Agent
	var ps []*tmux.Pane
	for _, w := range s.Windows {
		for _, p := range w.Panes {
			if a := m.agents[p.ID]; a != nil {
				as, ps = append(as, a), append(ps, p)
			}
		}
	}
	return as, ps
}

// agentRows builds the list for the current tab: sessions with agents
// (those with a blocked agent first), each followed by its agents unless
// collapsed.
func (m *Model) agentRows() []agentRow {
	if m.tree == nil {
		return nil
	}
	var urgent, rest [][]agentRow
	for _, s := range m.sessions() {
		as, ps := m.sessionAgents(s)
		head := agentRow{key: "s:" + s.ID, sess: s}
		var group []agentRow
		for i, a := range as {
			if !inTab(a, m.agentTab) {
				continue
			}
			head.tally(a)
			group = append(group, agentRow{key: a.Pane, sess: s, a: a, pane: ps[i]})
		}
		if len(group) == 0 {
			continue
		}
		if m.collapsed[s.ID] {
			group = nil
		}
		group = append([]agentRow{head}, group...)
		if head.blocked > 0 {
			urgent = append(urgent, group)
		} else {
			rest = append(rest, group)
		}
	}
	var rows []agentRow
	for _, g := range append(urgent, rest...) {
		rows = append(rows, g...)
	}
	return rows
}

// agentCursor is the index of the selected row (0 when the list is empty).
func (m *Model) agentCursor(rows []agentRow) int {
	for i, r := range rows {
		if r.key == m.agentSel {
			return i
		}
	}
	return 0
}

// normalizeAgents keeps the agent selection on an existing row, preferring
// the first agent over a header when the old selection is gone.
func (m *Model) normalizeAgents() {
	rows := m.agentRows()
	for _, r := range rows {
		if r.key == m.agentSel {
			return
		}
	}
	m.agentSel = ""
	for _, r := range rows {
		if r.a != nil {
			m.agentSel = r.key
			return
		}
	}
	if len(rows) > 0 {
		m.agentSel = rows[0].key
	}
}

// curAgent is the selected agent (nil when a header is selected).
func (m *Model) curAgent() (*agent.Agent, *tmux.Pane) {
	if m.agentSel == "" || strings.HasPrefix(m.agentSel, "s:") {
		return nil, nil
	}
	a := m.agents[m.agentSel]
	if a == nil {
		return nil, nil
	}
	return a, m.findPane(a.Pane)
}

func (m *Model) findPane(id string) *tmux.Pane {
	if m.tree == nil {
		return nil
	}
	for _, s := range m.tree.Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.ID == id {
					return p
				}
			}
		}
	}
	return nil
}

func (m *Model) moveAgent(delta int) {
	rows := m.agentRows()
	if len(rows) == 0 {
		return
	}
	i := clamp(m.agentCursor(rows)+delta, 0, len(rows)-1)
	m.agentSel = rows[i].key
	m.actOff = 0
}

// agentCounts returns the number of agents per tab.
func (m *Model) agentCounts() [numAgentTabs]int {
	var c [numAgentTabs]int
	if m.tree == nil {
		return c
	}
	for _, s := range m.sessions() {
		as, _ := m.sessionAgents(s)
		for _, a := range as {
			c[agentAll]++
			for t := agentBlocked; t < numAgentTabs; t++ {
				if inTab(a, t) {
					c[t]++
				}
			}
		}
	}
	return c
}

// agentSummary renders "⣿1 ⣾2 ●1" like `lazymux status`: blocked,
// working and done-but-unseen agents; states with none are left out.
func (m *Model) agentSummary() string {
	st := agent.Count(m.agents, clock())
	var parts []string
	for _, x := range []struct {
		n     int
		glyph string
		style lipgloss.Style
	}{{st.Blocked, "⣿", stRed}, {st.Working, "⣾", stYellow}, {st.Done, "●", stGreen}} {
		if x.n > 0 {
			parts = append(parts, x.style.Render(fmt.Sprintf("%s%d", x.glyph, x.n)))
		}
	}
	return strings.Join(parts, " ")
}

// --- keys ----------------------------------------------------------------------

// handleAgentKey handles keys on the Agents page. Keys that are not handled
// return false so global keys (quit, help, a) still work.
func (m *Model) handleAgentKey(k string) (bool, tea.Cmd) {
	prev := m.agentSel
	switch k {
	case "[":
		m.agentTab = (m.agentTab + numAgentTabs - 1) % numAgentTabs
		m.normalizeAgents()
	case "]":
		m.agentTab = (m.agentTab + 1) % numAgentTabs
		m.normalizeAgents()
	case "tab", "l", "right", "shift+tab", "h", "left":
		m.agentFocus = (m.agentFocus + 1) % numAgentPanels
	case "j", "down":
		if m.agentFocus == agentPanelActivity {
			m.actOff = max(0, m.actOff-1)
		} else {
			m.moveAgent(1)
		}
	case "k", "up":
		if m.agentFocus == agentPanelActivity {
			m.actOff++
		} else {
			m.moveAgent(-1)
		}
	case "g", "home":
		m.moveAgent(-1 << 20)
	case "G", "end":
		m.moveAgent(1 << 20)
	case "enter", " ":
		if strings.HasPrefix(m.agentSel, "s:") {
			id := strings.TrimPrefix(m.agentSel, "s:")
			if m.collapsed == nil {
				m.collapsed = map[string]bool{}
			}
			m.collapsed[id] = !m.collapsed[id]
			return true, nil
		}
		if k == "enter" {
			return true, m.jumpToAgent()
		}
	case "1", "2", "3":
		return true, m.answerAgent(int(k[0] - '0'))
	case "i":
		return true, m.replyAgent()
	case "x":
		return true, m.interruptAgent()
	case "B":
		return true, m.nextBlocked()
	default:
		return false, nil
	}
	m.markSeen()
	if m.agentSel != prev {
		return true, m.fetchPreview()
	}
	return true, nil
}

// jumpToAgent switches to the selected agent's pane.
func (m *Model) jumpToAgent() tea.Cmd {
	_, p := m.curAgent()
	if p == nil {
		return nil
	}
	w := p.Window
	s := w.Session
	c := m.c
	if !tmux.InsideTmux() && c.ClientTTY == "" {
		m.AttachTo = s
		return func() tea.Msg { return switchMsg{c.Select(s, w, p)} }
	}
	return func() tea.Msg { return switchMsg{c.SwitchTo(s, w, p)} }
}

func agentName(a *agent.Agent, p *tmux.Pane) string {
	return fmt.Sprintf("%s %d:%s", a.Kind, p.Window.Index, p.Window.Name)
}

var answerNames = [4]string{"", "yes", "always", "no"}

// answerAgent picks option n of the selected agent's permission prompt.
func (m *Model) answerAgent(n int) tea.Cmd {
	a, p := m.curAgent()
	if a == nil || p == nil {
		return m.setToast("select an agent first", true)
	}
	if a.State != agent.Blocked {
		return m.setToast(agentName(a, p)+" is not waiting for an answer", true)
	}
	c, keys, name := m.c, agent.AnswerKeys(a.Kind, n), agentName(a, p)
	return func() tea.Msg {
		return actionMsg{err: c.SendKeys(p.ID, keys...), toast: fmt.Sprintf("✓ %s → %s", answerNames[n], name)}
	}
}

// replyAgent opens a dialog and types the text into the agent's pane.
func (m *Model) replyAgent() tea.Cmd {
	a, p := m.curAgent()
	if a == nil || p == nil {
		return m.setToast("select an agent first", true)
	}
	c, name := m.c, agentName(a, p)
	d := &inputModal{title: "Reply to " + name, fields: []field{newField("", "", false)}, verb: "send",
		onSubmit: func(v []string, _ int) tea.Cmd {
			if v[0] == "" {
				return nil
			}
			return func() tea.Msg { return actionMsg{err: c.SendLine(p.ID, v[0]), toast: "✓ sent to " + name} }
		}}
	d.setFocus(0)
	m.modal = d
	return nil
}

// interruptAgent stops the selected agent's current turn.
func (m *Model) interruptAgent() tea.Cmd {
	a, p := m.curAgent()
	if a == nil || p == nil {
		return m.setToast("select an agent first", true)
	}
	c, key, name := m.c, agent.Interrupt(a.Kind), agentName(a, p)
	return func() tea.Msg { return actionMsg{err: c.SendKeys(p.ID, key), toast: "✓ interrupted " + name} }
}

// nextBlocked selects the next blocked agent after the cursor (wrapping),
// switching to the All tab and expanding its session when needed.
func (m *Model) nextBlocked() tea.Cmd {
	if m.agentCounts()[agentBlocked] == 0 {
		return m.setToast("no blocked agents", false)
	}
	if m.agentTab != agentAll && m.agentTab != agentBlocked {
		m.agentTab = agentAll
	}
	for id := range m.collapsed {
		delete(m.collapsed, id)
	}
	rows := m.agentRows()
	cur := m.agentCursor(rows)
	for i := 1; i <= len(rows); i++ {
		r := rows[(cur+i)%len(rows)]
		if r.a != nil && r.a.State == agent.Blocked {
			m.agentSel = r.key
			m.agentFocus = agentPanelList
			m.actOff = 0
			return m.fetchPreview()
		}
	}
	return nil
}

// --- view ------------------------------------------------------------------------

// agentTabsTitle renders "[1] All 4 │ Blocked 1 │ Working 2 │ Done 1",
// falling back to shorter forms when the panel is narrow.
func (m *Model) agentTabsTitle(maxW int) string {
	for _, f := range []struct {
		sep   string
		names [numAgentTabs]string
	}{
		{" │ ", agentTabNames},
		{"│", agentTabNames},
		{"│", [numAgentTabs]string{"All", "Blk", "Wrk", "Done"}},
	} {
		if t := m.agentTabsTitleWith(f.sep, f.names); lipgloss.Width(t) <= maxW {
			return t
		}
	}
	return m.agentTabsTitleWith("│", [numAgentTabs]string{"A", "B", "W", "D"})
}

func (m *Model) agentTabsTitleWith(sep string, names [numAgentTabs]string) string {
	focused := m.agentFocus == agentPanelList
	num := stFaint.Render("[1] ")
	if focused {
		num = stTitleOn.Render("[1] ")
	}
	counts := m.agentCounts()
	out := num
	for i, name := range names {
		if i > 0 {
			out += stFaint.Render(sep)
		}
		label := stFaint.Render(name)
		if i == m.agentTab {
			st := stSub.Bold(true)
			if focused {
				st = stTitleOn.Background(cSurface0)
			}
			label = st.Render(name)
		}
		out += label + " " + agentTabStyle[i].Render(fmt.Sprint(counts[i]))
	}
	return out + " "
}

// headerBadge is the worst state of a session: ⣿N, ⣾N or ●.
func headerBadge(r agentRow) seg {
	switch {
	case r.blocked > 0:
		return seg{fmt.Sprintf("⣿%d ", r.blocked), stRed}
	case r.working > 0:
		return seg{fmt.Sprintf("⣾%d ", r.working), stYellow}
	case r.done > 0:
		return seg{fmt.Sprintf("●%d ", r.done), stGreen}
	}
	return seg{"○ ", stFaint}
}

// agentLines renders one row of the agent list (1 line for a header, 2 for
// an agent).
func (m *Model) agentLines(r agentRow, w, sel int) []string {
	if r.a == nil {
		arrow := "▾ "
		if m.collapsed[r.sess.ID] {
			arrow = "▸ "
		}
		return []string{renderRow([]seg{{" " + arrow, stFaint}, {r.sess.Name, stText.Bold(true)}}, []seg{headerBadge(r)}, w, sel)}
	}
	a, p := r.a, r.pane
	st := agentStyle(a)
	kind := stText.Bold(true)
	if m.agentFocus != agentPanelList {
		kind = stSub.Bold(true)
	}
	l1 := renderRow(
		[]seg{{" ", stText}, {agentGlyph(a) + " ", st}, {string(a.Kind) + " ", kind},
			{fmt.Sprintf("%d:%s", p.Window.Index, p.Window.Name), stSub}},
		[]seg{{a.Label() + " " + agent.Age(a.Since, clock()) + " ", st}}, w, sel)

	var parts []string
	if a.Summary != "" {
		parts = append(parts, a.Summary)
	}
	if a.Waiting != "" {
		parts = append(parts, "waiting: "+a.Waiting)
	}
	sum := seg{strings.Join(parts, " · "), stSub}
	if len(parts) == 0 {
		sum = seg{tildePath(p.Path), stFaint}
	}
	if sel == 2 {
		sel = 3 // highlighted like line 1, but not bold
	}
	l2 := renderRow([]seg{{"   ", stText}, sum}, nil, w, sel)
	return []string{l1, l2}
}

func (m *Model) viewAgentsLeft(w, bodyH int) string {
	actH := clamp(bodyH/3, 3, 10)
	listH := bodyH - actH
	inner := w - 2
	avail := listH - 2

	rows := m.agentRows()
	cur := m.agentCursor(rows)
	var lines []string
	curTop, curBot := 0, 0
	nAgents, agentPos := 0, 0
	for i, r := range rows {
		if r.a != nil {
			nAgents++
			if i <= cur {
				agentPos = nAgents
			}
		}
		if r.a == nil && i > 0 {
			lines = append(lines, "") // a gap between sessions
		}
		sel := 0
		if i == cur {
			sel = 1
			if m.agentFocus == agentPanelList {
				sel = 2
			}
			curTop = len(lines)
		}
		lines = append(lines, m.agentLines(r, inner, sel)...)
		if i == cur {
			curBot = len(lines) - 1
		}
	}
	// scroll by lines so both lines of the selected agent stay visible
	if curBot >= m.agentOff+avail {
		m.agentOff = curBot - avail + 1
	}
	if curTop < m.agentOff {
		m.agentOff = curTop
	}
	m.agentOff = clamp(m.agentOff, 0, max(0, len(lines)-avail))
	lines = lines[m.agentOff:]

	if len(rows) == 0 {
		lines = m.agentsEmpty()
	}
	footer := fmt.Sprintf("%d of %d", agentPos, nAgents)
	list := renderBox(m.agentTabsTitle(w-6), footer, lines, w, listH, m.agentFocus == agentPanelList)
	return lipgloss.JoinVertical(lipgloss.Left, list, m.viewActivity(w, actH))
}

func (m *Model) agentsEmpty() []string {
	if m.agentCounts()[agentAll] > 0 {
		return []string{"", " " + stSub.Render(fmt.Sprintf("No %s agents.", strings.ToLower(agentTabNames[m.agentTab])))}
	}
	return []string{
		"",
		" " + stSub.Render("No agents running."),
		"",
		" " + stFaint.Render("Start claude, codex, gemini, opencode"),
		" " + stFaint.Render("or aider in any pane. Press ") + stKey.Render("a") + stFaint.Render(" for tmux."),
	}
}

// viewActivity is panel [2]: the selected agent's hook events, newest at
// the bottom.
func (m *Model) viewActivity(w, h int) string {
	focused := m.agentFocus == agentPanelActivity
	title := stSub.Render("[2] Activity")
	if focused {
		title = stTitleOn.Render("[2] Activity")
	}
	a, p := m.curAgent()
	if a == nil || p == nil {
		return renderBox(title+" ", "", []string{" " + stFaint.Render("select an agent")}, w, h, focused)
	}
	title += stFaint.Render(fmt.Sprintf(" (%s@%s:%d) ", a.Kind, p.Window.Session.Name, p.Window.Index))
	inner, avail := w-2, h-2

	if !a.Hooked {
		var lines []string
		if _, ok := agent.IntegrationFor(string(a.Kind)); ok {
			lines = []string{
				" " + stFaint.Render("no events yet. Run ") + stKey.Render("lazymux hooks install"),
				" " + stFaint.Render("to see prompts and tool calls here."),
			}
		} else {
			lines = []string{" " + stFaint.Render(string(a.Kind)+" has no hooks; its state comes from the screen")}
		}
		return renderBox(title, "", lines, w, h, focused)
	}

	var lines []string
	pend := -1
	if a.State == agent.Blocked {
		pend = agent.PendingTool(a.Events)
	}
	for i, e := range a.Events {
		pending := i == pend
		if l, ok := m.eventLine(e, inner, pending); ok {
			lines = append(lines, l)
		}
	}
	n := len(lines)
	m.actOff = clamp(m.actOff, 0, max(0, n-avail))
	end := n - m.actOff
	lines = lines[max(0, end-avail):end]
	color, cst := cSurface1, stFaint
	if focused {
		color, cst = cBlue, stBlue
	}
	return renderBox(titleRight(title, cst.Render(fmt.Sprintf(" %d events ", n)), w, color), "", lines, w, h, focused)
}

// eventLine renders one Activity row: "▸ prompt  text", "Read    path",
// the pending call tinted red, "● done". Session starts and notifications
// are not shown (the waiting reason already says what a notification says).
func (m *Model) eventLine(e agent.Event, w int, pending bool) (string, bool) {
	switch e.Kind {
	case "prompt":
		return renderRow([]seg{{" ▸ ", stBlue}, {"prompt  ", stBlue}, {e.Detail, stText}}, nil, w, 0), true
	case "tool":
		name := fmt.Sprintf("%-7s ", e.Name)
		if pending {
			bg := lipgloss.Color("#3b2633")
			// the whole row is tinted; "pending" follows the (shortened) detail
			tail := seg{"  pending ", stRed.Background(bg)}
			left := truncSegs([]seg{{" ⣿ ", stRed.Background(bg)}, {name, stRed.Bold(true).Background(bg)},
				{e.Detail, stText.Background(bg)}}, w-segsWidth([]seg{tail}))
			left = append(left, tail)
			var b strings.Builder
			for _, x := range left {
				b.WriteString(x.st.Render(x.text))
			}
			b.WriteString(lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", max(0, w-segsWidth(left)))))
			return b.String(), true
		}
		return renderRow([]seg{{"   ", stText}, {name, stText}, {e.Detail, stSub}}, nil, w, 0), true
	case "stop":
		return renderRow([]seg{{" ● ", stGreen}, {"done", stGreen}}, nil, w, 0), true
	}
	return "", false
}

func (m *Model) viewAgentsPreview(w, h int) string {
	a, p := m.curAgent()
	if a == nil || p == nil {
		return renderBox(stSub.Render("preview "), "", []string{stFaint.Render(" select an agent to see its pane")}, w, h, false)
	}
	title := titleRight(stText.Render(p.Target())+stFaint.Render(" · ")+stSub.Render(string(a.Kind))+" ",
		" "+agentStyle(a).Render(agentGlyph(a)+" "+a.Label()+" "+agent.Age(a.Since, clock()))+" ", w, cSurface1)
	var lines []string
	if m.previewFor == p.ID {
		lines = tmux.TrimCapture(m.preview, h-2)
	}
	for i, l := range lines {
		lines[i] = " " + l
	}
	return renderBox(title, "", lines, w, h, false)
}

// pageIndicator renders "⣿1 ⣾2 ●1  tmux │ agents" with the current page
// highlighted.
func (m *Model) pageIndicator() string {
	on := stTitleOn.Background(cSurface0)
	tm, ag := stFaint.Render("tmux"), stFaint.Render("agents")
	if m.page == pageTmux {
		tm = on.Render("tmux")
	} else {
		ag = on.Render("agents")
	}
	return tm + stFaint.Render(" │ ") + ag + " "
}

// agentBar is the key bar of the Agents page: the selected agent's actions
// (answers first when it is blocked), a divider, then navigation.
func (m *Model) agentBar(last kd) []kd {
	a, _ := m.curAgent()
	var keys []kd
	if a != nil {
		if a.State == agent.Blocked {
			keys = append(keys, kd{"1", "yes"}, kd{"2", "always"}, kd{"3", "no"})
		}
		keys = append(keys, kd{"i", "reply"}, kd{"x", "interrupt"}, kd{"│", ""}, kd{"enter", "jump"})
	} else if strings.HasPrefix(m.agentSel, "s:") {
		keys = append(keys, kd{"enter", "fold"})
	}
	return append(keys, kd{"[ ]", "tabs"}, kd{"tab", "panel"}, kd{"a", "tmux"}, kd{"?", "help"}, last)
}

// clock returns the time; tests replace it.
var clock = time.Now

// --- badges on the tmux page and in the compact sidebar -----------------------

// worstAgent is the most urgent agent among panes (blocked > working > idle).
func (m *Model) worstAgent(panes []*tmux.Pane) *agent.Agent {
	var worst *agent.Agent
	for _, p := range panes {
		if a := m.agents[p.ID]; a != nil && (worst == nil || urgency(a) > urgency(worst)) {
			worst = a
		}
	}
	return worst
}

// sessionBadge is "⣿1 " / "⣾2 " / "● " for a session with agents.
func (m *Model) sessionBadge(s *tmux.Session) []seg {
	as, _ := m.sessionAgents(s)
	if len(as) == 0 {
		return nil
	}
	r := agentRow{}
	for _, a := range as {
		r.tally(a)
	}
	return []seg{headerBadge(r)}
}

// windowBadge is the glyph of the window's most urgent agent.
func (m *Model) windowBadge(w *tmux.Window) []seg {
	if a := m.worstAgent(w.Panes); a != nil {
		return []seg{{agentGlyph(a) + " ", agentStyle(a)}}
	}
	return nil
}
