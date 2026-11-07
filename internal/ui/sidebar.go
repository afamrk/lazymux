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

// Sidebar mode: lazymux runs in a narrow pane on the left of the current
// window. Unfocused it shows a compact tree; when you move into the pane it
// widens to sidebarFocusPct of the window and shows the full UI, and it
// shrinks back when you leave.
const sidebarFocusPct = 60

// minFullWidth is the narrowest pane that gets the full UI.
const minFullWidth = 50

// NewSidebar creates the model for `lazymux sidebar`.
func NewSidebar(c *tmux.Client) *Model {
	m := New(c)
	m.sidebar = true
	return m
}

// focusMsg reports whether the sidebar pane has focus (from a tick check,
// for terminals or tmux setups without focus events).
type focusMsg struct {
	focused     bool
	paneW, winW int
}

// checkFocus asks tmux whether the sidebar pane is the active one.
func (m *Model) checkFocus() tea.Cmd {
	if !m.sidebar || m.selfPane == "" {
		return nil
	}
	c, pane := m.c, m.selfPane
	return func() tea.Msg {
		st, err := c.SidebarState(pane)
		if err != nil {
			return nil
		}
		if !st.Docked && st.Session != "" { // a layout change moved it: put it back
			c.SidebarFollow(st.Session)
		}
		return focusMsg{focused: st.Focused, paneW: st.PaneW, winW: st.WinW}
	}
}

// sidebarWidth is the pane width for the focus state in a window winW wide.
func sidebarWidth(focused bool, winW int) int {
	if focused {
		return max(tmux.SidebarWidth, winW*sidebarFocusPct/100)
	}
	return tmux.SidebarWidth
}

// reconcile fixes the pane width when it does not match the focus state,
// e.g. after the sidebar was moved into another window while wide.
func (m *Model) reconcile(msg focusMsg) tea.Cmd {
	if cmd := m.setFocused(msg.focused, msg.winW); cmd != nil {
		return cmd
	}
	if want := sidebarWidth(msg.focused, msg.winW); msg.paneW != want && msg.winW > want {
		c, pane := m.c, m.selfPane
		return func() tea.Msg { c.ResizeWidth(pane, want); return nil }
	}
	return nil
}

// setFocused widens or narrows the sidebar pane when focus changes.
func (m *Model) setFocused(f bool, winW int) tea.Cmd {
	if !m.sidebar || f == m.focused {
		return nil
	}
	m.focused = f
	c, pane := m.c, m.selfPane
	resize := func() tea.Msg {
		w := winW
		if w == 0 {
			st, _ := c.SidebarState(pane)
			w = st.WinW
		}
		c.ResizeWidth(pane, sidebarWidth(f, w))
		return nil
	}
	if f {
		return tea.Batch(resize, m.fetchTree(), m.fetchPreview())
	}
	m.modal = nil // an open dialog or filter input does not survive leaving the sidebar
	m.filtering = false
	return resize
}

// leaveSidebar moves focus back to the pane you came from.
func (m *Model) leaveSidebar() tea.Cmd {
	c, pane := m.c, m.selfPane
	return func() tea.Msg {
		c.FocusLastPane(pane)
		return nil
	}
}

// sidebarOff closes every sidebar (including this one).
func (m *Model) sidebarOff() tea.Cmd {
	c := m.c
	return func() tea.Msg {
		c.SidebarOff()
		return nil
	}
}

// compactView is shown while the sidebar is not focused (or too narrow).
func (m *Model) compactView() bool {
	return m.sidebar && (!m.focused || m.w < minFullWidth)
}

// viewCompact renders the narrow sidebar: sessions (the current one expanded
// with its windows), an agents section and a hint.
func (m *Model) viewCompact() string {
	w, h := m.w, m.h
	var curSess *tmux.Session
	var curWin *tmux.Window
	if m.tree != nil {
		for _, p := range m.tree.Sidebars {
			if p.ID == m.selfPane {
				curWin = p.Window
				curSess = p.Window.Session
			}
		}
	}

	var lines []string
	m.compactCur = -1
	if m.page == pageAgents {
		lines = m.compactAgents(w)
	} else {
		lines = m.compactTmux(w, curSess, curWin)
	}
	return m.compactFrame(lines, w, h)
}

// compactHeader is "lazymux" with the agent counts on the right
// ("⣿1 ⣾2 ●1"), as in the sidebar mockup.
func (m *Model) compactHeader(w int) string {
	var right []seg
	if sum := m.agentSummary(); sum != "" {
		right = []seg{{sum + " ", lipgloss.NewStyle()}}
	}
	return renderRow([]seg{{" lazymux", stMauve.Bold(true)}}, right, w, 0)
}

// compactAgents is the narrow Agents page: the agents of the current tab
// grouped by session, each with its summary line.
func (m *Model) compactAgents(w int) []string {
	lines := []string{m.compactHeader(w)}
	if m.agentTab != agentAll {
		lines = append(lines, divider(" "+strings.ToLower(agentTabNames[m.agentTab])+" ", w))
	}
	n := 0
	for _, r := range m.agentRows() {
		if r.a == nil {
			lines = append(lines, "", renderRow([]seg{{" ▾ ", stFaint}, {r.sess.Name, stText.Bold(true)}}, []seg{headerBadge(r)}, w, 0))
			continue
		}
		n++
		lines = append(lines, m.compactAgentEntry(r.a, r.pane, w, false)...)
	}
	if n == 0 {
		empty := "no agents running"
		if m.agentTab != agentAll && m.agentCounts()[agentAll] > 0 {
			empty = "no " + strings.ToLower(agentTabNames[m.agentTab]) + " agents"
		}
		lines = append(lines, "", " "+stSub.Render(empty))
	}
	return lines
}

// compactTmux is the narrow tmux page: every session with its windows
// (the current session in bold, the current window marked; windows with an
// agent show its state and age), then the agents.
func (m *Model) compactTmux(w int, curSess *tmux.Session, curWin *tmux.Window) []string {
	lines := []string{m.compactHeader(w)}
	m.compactCur = -1
	for _, s := range m.sessions() {
		name := stSub
		if s == curSess {
			name = stText.Bold(true)
		}
		right := m.sessionBadge(s)
		if right == nil {
			right = []seg{{fmt.Sprintf("%dw ", len(s.Windows)), stFaint}}
		}
		lines = append(lines, renderRow([]seg{{" ▾ ", stFaint}, {s.Name, name}}, right, w, 0))
		for _, win := range s.Windows {
			mark, idx, st, sel := "   ", stFaint, stSub, 0
			if win == curWin {
				mark, idx, st, sel = " ▸ ", stBlue, stText, 2
				m.compactCur = len(lines)
			}
			var right []seg
			if a := m.worstAgent(win.Panes); a != nil {
				right = []seg{{stateGlyph(a.State) + " " + agent.Age(a.Since, clock()) + " ", stateStyle(a.State)}}
			} else if p := win.ActivePane(); p != nil {
				right = []seg{{p.Command + " ", stFaint}}
			}
			lines = append(lines, renderRow(
				[]seg{{" " + mark, stBlue}, {fmt.Sprintf("%d ", win.Index), idx}, {win.Name, st}}, right, w, sel))
		}
	}
	lines = append(lines, "", divider(" agents ", w))
	n := 0
	for _, st := range []agent.State{agent.Blocked, agent.Working, agent.Idle} {
		for _, s := range m.sessions() {
			as, ps := m.sessionAgents(s)
			for i, a := range as {
				if a.State == st {
					lines = append(lines, m.compactAgentEntry(a, ps[i], w, true)...)
					n++
				}
			}
		}
	}
	if n == 0 {
		lines = append(lines, " "+stFaint.Render("no agents running"))
	}
	return lines
}

// compactAgentEntry is one agent in the narrow sidebar:
//
//	⣿ claude      api:3   2m
//	  fix jwt expiry · Bash(go…
//
// The location includes the session when withSession is set (the agents
// section lists agents of every session together).
func (m *Model) compactAgentEntry(a *agent.Agent, p *tmux.Pane, w int, withSession bool) []string {
	st := stateStyle(a.State)
	loc := fmt.Sprintf("%d:%s", p.Window.Index, p.Window.Name)
	if withSession {
		loc = fmt.Sprintf("%s:%d", p.Window.Session.Name, p.Window.Index)
	}
	out := []string{renderRow(
		[]seg{{" " + stateGlyph(a.State) + " ", st}, {string(a.Kind), stText.Bold(true)}},
		[]seg{{loc + "  ", stFaint}, {agent.Age(a.Since, clock()) + " ", st}}, w, 0)}
	var parts []string
	if a.Summary != "" {
		parts = append(parts, a.Summary)
	}
	if a.Waiting != "" {
		parts = append(parts, a.Waiting)
	}
	if len(parts) > 0 {
		out = append(out, renderRow([]seg{{"   " + strings.Join(parts, " · "), stFaint}}, nil, w, 0))
	}
	return out
}

// compactFrame pads lines to the pane and adds the hint at the bottom.
func (m *Model) compactFrame(lines []string, w, h int) string {

	hint := " " + stFaint.Render("focus to expand · ") + stKey.Render("?") + stFaint.Render(" keys")
	body := h - 2 // hint line + its separator
	// too long: scroll (below the header) so the current window stays visible
	if len(lines) > body && m.compactCur > body-2 && body > 3 {
		drop := min(m.compactCur-(body-2), len(lines)-body)
		lines = append(lines[:1:1], lines[1+drop:]...)
	}
	if len(lines) > body {
		lines = lines[:max(0, body)]
	}
	for len(lines) < body {
		lines = append(lines, "")
	}
	lines = append(lines, stFaint.Render(strings.Repeat("─", w)), ansi.Truncate(hint, w, "…"))
	for i, l := range lines {
		if pad := w - ansi.StringWidth(l); pad > 0 {
			lines[i] = l + strings.Repeat(" ", pad)
		}
	}
	return strings.Join(lines, "\n")
}

// divider renders "─ title ─────" across w columns.
func divider(title string, w int) string {
	return stFaint.Render(ansi.Truncate("─"+title+strings.Repeat("─", max(0, w-ansi.StringWidth(title)-1)), w, ""))
}
