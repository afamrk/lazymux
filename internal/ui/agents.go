package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// togglePage flips between the tmux and Agents pages. Each page keeps its own
// focus and selection.
func (m *Model) togglePage() tea.Cmd {
	m.page = 1 - m.page
	if m.page == pageTmux {
		return m.fetchPreview()
	}
	return nil
}

// handleAgentKey handles keys on the Agents page. tmux actions are not
// available here; keys that are not handled return false so global keys
// (quit, help, a) still work.
func (m *Model) handleAgentKey(k string) bool {
	switch k {
	case "[":
		m.agentTab = (m.agentTab + numAgentTabs - 1) % numAgentTabs
	case "]":
		m.agentTab = (m.agentTab + 1) % numAgentTabs
	case "tab", "l", "right", "shift+tab", "h", "left":
		m.agentFocus = (m.agentFocus + 1) % numAgentPanels
	case "j", "k", "down", "up", "g", "G", "home", "end", "enter":
		// nothing to move over until agents are detected (P3)
	default:
		return false
	}
	return true
}

// agentCounts returns the number of agents per tab. Detection arrives in P3.
func (m *Model) agentCounts() [numAgentTabs]int {
	return [numAgentTabs]int{}
}

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

func (m *Model) viewAgentsLeft(w, bodyH int) string {
	actH := clamp(bodyH/3, 3, 8)
	listH := bodyH - actH

	empty := "No agents yet: detection arrives in P3."
	if m.agentTab != agentAll {
		empty = fmt.Sprintf("No %s agents: detection arrives in P3.", agentTabNames[m.agentTab])
	}
	rows := []string{
		"",
		" " + stSub.Render(empty),
		"",
		" " + stFaint.Render("Agents will be grouped by session,"),
		" " + stFaint.Render("blocked ones first. Press ") + stKey.Render("a") + stFaint.Render(" for tmux."),
	}
	list := renderBox(m.agentTabsTitle(w-6), "", rows, w, listH, m.agentFocus == agentPanelList)

	title := stSub.Render("[2] Activity")
	if m.agentFocus == agentPanelActivity {
		title = stTitleOn.Render("[2] Activity")
	}
	act := renderBox(title+" ", "", []string{" " + stFaint.Render("timeline of the selected agent")}, w, actH, m.agentFocus == agentPanelActivity)
	return lipgloss.JoinVertical(lipgloss.Left, list, act)
}

func (m *Model) viewAgentsPreview(w, h int) string {
	lines := []string{stFaint.Render(" the selected agent's pane appears here")}
	return renderBox(stSub.Render("preview "), "", lines, w, h, false)
}

// pageIndicator renders "tmux │ agents" with the current page highlighted.
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
