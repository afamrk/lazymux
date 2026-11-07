package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/afamrk/lazymux/internal/tmux"
)

// --- filter ----------------------------------------------------------------

// Each panel has its own filter (`/`). While typing, keys go to filterIn;
// enter keeps the filter, esc clears it.

func newFilterInput() textinput.Model {
	in := textinput.New()
	in.Prompt = "/ "
	in.PromptStyle = stBlue
	in.TextStyle = stText
	in.Cursor.SetMode(cursor.CursorStatic)
	return in
}

func (m *Model) startFilter() {
	m.filterIn = newFilterInput()
	m.filterIn.SetValue(m.filter[m.focus])
	m.filterIn.CursorEnd()
	m.filterIn.Focus()
	m.filtering = true
}

// handleFilterKey handles a key while the filter input is active.
func (m *Model) handleFilterKey(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "enter":
		m.filtering = false
		return m.afterSelectionChange()
	case "esc":
		m.filtering = false
		m.filter[m.focus] = ""
		return m.afterSelectionChange()
	case "down", "ctrl+j", "ctrl+n":
		m.move(1)
		return m.afterSelectionChange()
	case "up", "ctrl+k", "ctrl+p":
		m.move(-1)
		return m.afterSelectionChange()
	}
	var cmd tea.Cmd
	m.filterIn, cmd = m.filterIn.Update(k)
	m.filter[m.focus] = m.filterIn.Value()
	return tea.Batch(cmd, m.afterSelectionChange())
}

// afterSelectionChange re-resolves the selection against the filtered lists
// and refreshes the preview.
func (m *Model) afterSelectionChange() tea.Cmd {
	m.normalize()
	return m.fetchPreview()
}

// matchSession / matchWindow / matchPane test an item against its panel's
// filter. idx are the matched positions in the item's name (for highlight);
// a match only in other fields (command, path) has no idx.
func (m *Model) matchSession(s *tmux.Session) (bool, []int) {
	return matchFields(m.filter[panelSessions], s.Name)
}

func (m *Model) matchWindow(w *tmux.Window) (bool, []int) {
	cmd := ""
	if p := w.ActivePane(); p != nil {
		cmd = p.Command
	}
	return matchFields(m.filter[panelWindows], w.Name, fmt.Sprint(w.Index), cmd)
}

func (m *Model) matchPane(p *tmux.Pane) (bool, []int) {
	return matchFields(m.filter[panelPanes], p.Command, p.ID, p.Title, p.Path)
}

func matchFields(pattern, name string, more ...string) (bool, []int) {
	if pattern == "" {
		return true, nil
	}
	if _, idx, ok := fuzzyMatch(pattern, name); ok {
		return true, idx
	}
	for _, f := range more {
		if _, _, ok := fuzzyMatch(pattern, f); ok {
			return true, nil
		}
	}
	return false, nil
}

// visSessions / visWindows / visPanes are the lists after filtering, in
// tmux order.
func (m *Model) visSessions() []*tmux.Session {
	var out []*tmux.Session
	for _, s := range m.sessions() {
		if ok, _ := m.matchSession(s); ok {
			out = append(out, s)
		}
	}
	return out
}

func (m *Model) visWindows(s *tmux.Session) []*tmux.Window {
	if s == nil {
		return nil
	}
	var out []*tmux.Window
	for _, w := range s.Windows {
		if ok, _ := m.matchWindow(w); ok {
			out = append(out, w)
		}
	}
	return out
}

func (m *Model) visPanes(w *tmux.Window) []*tmux.Pane {
	if w == nil {
		return nil
	}
	var out []*tmux.Pane
	for _, p := range w.Panes {
		if ok, _ := m.matchPane(p); ok {
			out = append(out, p)
		}
	}
	return out
}

// --- multi-selection ---------------------------------------------------------

// selRef is one selected item. All selected items belong to one panel.
type selRef struct {
	panel int
	sess  string // session id the item belongs to
	id    string // session, window or pane id
	label string
}

// cursorRef describes the item under the cursor of the focused panel.
func (m *Model) cursorRef() (selRef, bool) {
	s, _, w, _, p, _ := m.cur()
	switch {
	case m.focus == panelSessions && s != nil:
		return selRef{panelSessions, s.ID, s.ID, "session " + s.Name}, true
	case m.focus == panelWindows && w != nil:
		return selRef{panelWindows, s.ID, w.ID, fmt.Sprintf("window %s:%d %s", s.Name, w.Index, w.Name)}, true
	case m.focus == panelPanes && p != nil:
		return selRef{panelPanes, s.ID, p.ID, "pane " + p.ID}, true
	}
	return selRef{}, false
}

// panelRefs returns refs for the visible items of the focused panel, in order.
func (m *Model) panelRefs() []selRef {
	s, _, w, _, _, _ := m.cur()
	var out []selRef
	switch m.focus {
	case panelSessions:
		for _, x := range m.visSessions() {
			out = append(out, selRef{panelSessions, x.ID, x.ID, "session " + x.Name})
		}
	case panelWindows:
		for _, x := range m.visWindows(s) {
			out = append(out, selRef{panelWindows, s.ID, x.ID, fmt.Sprintf("window %s:%d %s", s.Name, x.Index, x.Name)})
		}
	case panelPanes:
		for _, x := range m.visPanes(w) {
			out = append(out, selRef{panelPanes, s.ID, x.ID, "pane " + x.ID})
		}
	}
	return out
}

// toggleSelect adds or removes the cursor item (space). Selecting in another
// panel starts a new selection.
func (m *Model) toggleSelect() {
	ref, ok := m.cursorRef()
	if !ok {
		return
	}
	if len(m.selected) > 0 && m.selected[0].panel != ref.panel {
		m.selected = nil
	}
	for i, x := range m.selected {
		if x.id == ref.id {
			m.selected = append(m.selected[:i], m.selected[i+1:]...)
			return
		}
	}
	m.selected = append(m.selected, ref)
	m.anchor = ref.id
}

// selectRange selects every visible item between the anchor (the last item
// selected with space) and the cursor (v).
func (m *Model) selectRange() tea.Cmd {
	ref, ok := m.cursorRef()
	if !ok {
		return nil
	}
	refs := m.panelRefs()
	a, c := -1, -1
	for i, x := range refs {
		if x.id == m.anchor {
			a = i
		}
		if x.id == ref.id {
			c = i
		}
	}
	if a < 0 || len(m.selected) == 0 || m.selected[0].panel != ref.panel {
		return m.setToast("select the start of the range with space first", true)
	}
	if a > c {
		a, c = c, a
	}
	for _, x := range refs[a : c+1] {
		if !m.isSelected(x.panel, x.id) {
			m.selected = append(m.selected, x)
		}
	}
	return nil
}

func (m *Model) isSelected(panel int, id string) bool {
	for _, x := range m.selected {
		if x.panel == panel && x.id == id {
			return true
		}
	}
	return false
}

// targets are the items an action applies to: the selection when it is in
// the focused panel, otherwise the cursor item.
func (m *Model) targets() []selRef {
	if len(m.selected) > 0 && m.selected[0].panel == m.focus {
		return m.selected
	}
	if ref, ok := m.cursorRef(); ok {
		return []selRef{ref}
	}
	return nil
}

// pruneSelection drops selected items that no longer exist.
func (m *Model) pruneSelection() {
	if m.tree == nil || len(m.selected) == 0 {
		return
	}
	exists := map[string]bool{}
	for _, s := range m.tree.Sessions {
		exists[s.ID] = true
		for _, w := range s.Windows {
			exists[w.ID] = true
			for _, p := range w.Panes {
				exists[p.ID] = true
			}
		}
	}
	keep := m.selected[:0]
	for _, x := range m.selected {
		if exists[x.id] {
			keep = append(keep, x)
		}
	}
	m.selected = keep
}
