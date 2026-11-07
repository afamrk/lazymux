package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

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

// compactHeader is "lazymux" plus the page you left the sidebar on.
func (m *Model) compactHeader(w int) string {
	page := "tmux"
	if m.page == pageAgents {
		page = "agents"
	}
	return renderRow([]seg{{" lazymux", stMauve.Bold(true)}}, []seg{{page + " ", stBlue}}, w, 0)
}

// compactAgents is the narrow Agents page: the state tabs on one line and
// the agents grouped by session (empty until detection arrives in P3).
func (m *Model) compactAgents(w int) []string {
	counts := m.agentCounts()
	short := [numAgentTabs]string{"All", "Blk", "Wrk", "Done"}
	tabs := " "
	for i, name := range short {
		if i > 0 {
			tabs += stFaint.Render(" ")
		}
		st := stFaint
		if i == m.agentTab {
			st = stTitleOn.Background(cSurface0)
		}
		tabs += st.Render(name) + agentTabStyle[i].Render(fmt.Sprint(counts[i]))
	}
	empty := "no agents yet"
	if m.agentTab != agentAll {
		empty = "no " + strings.ToLower(agentTabNames[m.agentTab]) + " agents"
	}
	return []string{
		m.compactHeader(w), "",
		ansi.Truncate(tabs, w, ""),
		divider("", w),
		" " + stSub.Render(empty),
		" " + stFaint.Render("detection arrives in P3"),
	}
}

// compactTmux is the narrow tmux page: every session with its windows
// (the current session in bold, the current window marked), and the agents
// section.
func (m *Model) compactTmux(w int, curSess *tmux.Session, curWin *tmux.Window) []string {
	lines := []string{m.compactHeader(w), ""}
	m.compactCur = -1
	for _, s := range m.sessions() {
		arrow, name := "▾ ", stSub
		if s == curSess {
			name = stText.Bold(true)
		}
		right := []seg{{fmt.Sprintf("%dw ", len(s.Windows)), stFaint}}
		if s.Attached {
			right = []seg{{"● ", stGreen}}
		}
		lines = append(lines, renderRow([]seg{{" " + arrow, stFaint}, {s.Name, name}}, right, w, 0))
		for _, win := range s.Windows {
			mark, st, sel := "   ", stSub, 0
			if win == curWin {
				mark, st, sel = " ▸ ", stText, 2
				m.compactCur = len(lines)
			}
			cmd := ""
			if p := win.ActivePane(); p != nil {
				cmd = p.Command
			}
			lines = append(lines, renderRow(
				[]seg{{" " + mark, stBlue}, {fmt.Sprintf("%d ", win.Index), stBlue}, {win.Name, st}},
				[]seg{{cmd + " ", stFaint}}, w, sel))
		}
	}
	return append(lines, "", divider(" agents ", w), " "+stFaint.Render("detection arrives in P3"))
}

// compactFrame pads lines to the pane and adds the hint at the bottom.
func (m *Model) compactFrame(lines []string, w, h int) string {

	hint := " " + stFaint.Render("focus to expand · ") + stKey.Render("?") + stFaint.Render(" keys")
	body := h - 2 // hint line + its separator
	// too long: scroll (below the header) so the current window stays visible
	if len(lines) > body && m.compactCur > body-2 && body > 3 {
		drop := min(m.compactCur-(body-2), len(lines)-body)
		lines = append(lines[:2:2], lines[2+drop:]...)
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
