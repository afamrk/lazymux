package ui

import tea "github.com/charmbracelet/bubbletea"

// binding is one row of the help screen. run is the key that is dispatched
// when the row is executed with enter; soon marks keys planned for a later
// phase (shown dimmed, not runnable).
type binding struct {
	keys  string
	desc  string
	group string
	col   int
	run   string
	soon  bool
}

var bindings = []binding{
	{"j / k", "move down / up", "Navigation", 0, "", false},
	{"g / G", "first / last", "Navigation", 0, "", false},
	{"h / l, tab", "previous / next panel", "Navigation", 0, "", false},
	{"1 2 3", "jump to panel", "Navigation", 0, "", false},
	{"/", "filter the list (enter keep · esc clear)", "Navigation", 0, "/", false},
	{"f", "find: jump anywhere, agents too (also ctrl+p)", "Navigation", 0, "f", false},

	{"enter", "switch to selection", "Sessions & windows", 0, "enter", false},
	{"n", "new (session / window / pane)", "Sessions & windows", 0, "n", false},
	{"r", "rename", "Sessions & windows", 0, "r", false},
	{"d", "kill selection or cursor item (asks)", "Sessions & windows", 0, "d", false},
	{"u", "undo last kill", "Sessions & windows", 0, "u", false},
	{"m", "move windows / join panes to… (picker)", "Sessions & windows", 0, "m", false},
	{"s", "swap selected item with cursor", "Sessions & windows", 0, "s", false},
	{"b", "break pane to window", "Sessions & windows", 0, "b", false},
	{"z", "zoom pane", "Sessions & windows", 0, "z", false},
	{"L", "cycle layout", "Sessions & windows", 0, "L", false},

	{"[ / ]", "state tab (All / Blocked / Working / Done)", "Agents", 1, "", false},
	{"/", "filter agents (program, session, prompt…)", "Agents", 1, "", false},
	{"enter", "jump to agent · fold session header", "Agents", 1, "", false},
	{"1 / 2 / 3", "answer permission (yes / always / no)", "Agents", 1, "", false},
	{"i", "reply to agent (types a line)", "Agents", 1, "", false},
	{"x", "interrupt (esc; ctrl-c for aider)", "Agents", 1, "", false},
	{"B", "jump to next blocked", "Agents", 1, "", false},
	{"n", "start a new agent (in a new window)", "Agents", 1, "", false},

	{"space", "select / unselect item", "Selection", 1, " ", false},
	{"v", "select range (from last space)", "Selection", 1, "v", false},
	{"esc", "clear selection, then filter", "Selection", 1, "", false},

	{"a", "switch page (tmux ⇄ agents)", "Global", 1, "a", false},
	{"N", "open project (zoxide, git repos)", "Global", 1, "N", false},
	{"?", "this help", "Global", 1, "", false},
	{"q", "quit (sidebar: back to your pane)", "Global", 1, "q", false},
	{"Q", "turn the sidebar off", "Global", 1, "Q", false},
}

// keyMsg builds the KeyMsg for a binding's run key.
func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	case "ctrl+p":
		return tea.KeyMsg{Type: tea.KeyCtrlP}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}
