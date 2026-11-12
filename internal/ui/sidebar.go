package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/afamrk/lazymux/internal/agent"
	"github.com/afamrk/lazymux/internal/tmux"
)

// Sidebar mode: lazymux runs in a narrow pane on the left of the current
// window: one list of agents and sessions (list.go), an overview while
// unfocused and with a cursor while focused. The pane never resizes, so the
// other panes keep their layout; the preview of the cursor row and the
// dialogs open in a popup on the right of the sidebar, which receives the
// client's keys and forwards them here over a socket (link.go).

// NewSidebar creates the model for `lazymux sidebar`.
func NewSidebar(c *tmux.Client) *Model {
	m := New(c)
	m.sidebar = true
	return m
}

// focusMsg reports whether the sidebar pane has focus, as tmux sees it.
type focusMsg struct {
	focused     bool
	paneW, winW int
	session     string
}

// popupMsg reports the client the preview popup is about to open on.
type popupMsg struct{ client string }

// popupDoneMsg reports that the popup closed (display-popup returned) and
// how long it was open.
type popupDoneMsg struct {
	err error
	d   time.Duration
}

// replayedMsg reports that tmux got the prefix keys from the popup.
type replayedMsg struct{}

// popupClosedMsg reports that the sidebar closed the popup and focus left.
type popupClosedMsg struct{}

// checkFocus asks tmux whether the sidebar pane is the active one. Focus
// events only trigger this check: opening and closing the popup can send
// focus events of their own, tmux's pane_active is the truth.
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
		if st.Alone { // you closed the pane next to it: let tmux go to the next window
			c.SidebarReap()
			return focusMsg{focused: false, paneW: st.PaneW, winW: st.WinW, session: st.Session}
		}
		if !st.Docked && st.Session != "" { // a layout change moved it: put it back
			c.SidebarFollow(st.Session)
		}
		return focusMsg{focused: st.Focused, paneW: st.PaneW, winW: st.WinW, session: st.Session}
	}
}

// reconcile follows the focus state and keeps the pane at its fixed width
// (an older lazymux widened it on focus).
func (m *Model) reconcile(msg focusMsg) tea.Cmd {
	cmd := m.setFocused(msg.focused, msg.session)
	if m.reopen { // a prefix binding ran and left you in the sidebar: preview again
		m.reopen = false
		if msg.focused && m.focused {
			cmd = tea.Batch(cmd, m.openPopup(msg.session))
		}
	}
	if msg.paneW != tmux.SidebarWidth && msg.winW > tmux.SidebarWidth {
		c, pane := m.c, m.selfPane
		cmd = tea.Batch(cmd, func() tea.Msg { c.ResizeWidth(pane, tmux.SidebarWidth); return nil })
	}
	return cmd
}

// setFocused switches between the overview and the list with a cursor (put
// on the agent that needs you, else your window), and opens the preview
// popup when the sidebar gains focus.
func (m *Model) setFocused(f bool, session string) tea.Cmd {
	if !m.sidebar || f == m.focused {
		return nil
	}
	m.focused = f
	if !f {
		m.modal = nil // an open dialog or filter input does not survive leaving the sidebar
		m.filtering, m.listFilter = false, ""
		m.popupOpen, m.published = false, ""
		return nil
	}
	m.listDefault() // the agent that needs you, else your window
	return tea.Batch(m.fetchTree(), m.openPopup(session), m.listSync())
}

// openPopup shows the preview popup on the client looking at the sidebar.
func (m *Model) openPopup(session string) tea.Cmd {
	if m.popupOpen || m.selfPane == "" {
		return nil
	}
	var cmd tea.Cmd
	if m.link == nil {
		l, err := listenLink(linkSocket(m.c.ServerName(), m.selfPane))
		if err != nil {
			return m.setToast("✗ preview popup: "+err.Error(), true)
		}
		m.link, cmd = l, l.wait()
	}
	m.popupOpen = true
	c := m.c
	return tea.Batch(cmd, func() tea.Msg {
		client := c.ClientTTY
		if client == "" && session != "" {
			client = c.ActiveClient(session)
		}
		return popupMsg{client}
	})
}

// linked takes a popup that connected; an older connection is dropped.
func (m *Model) linked(conn *wireConn) tea.Cmd {
	if m.conn != nil {
		m.conn.close()
	}
	m.conn, m.published, m.overlaySent = conn, "", "\x00" // send everything again
	return waitWire(conn)
}

// fromPopup handles a message from the popup: its size, a key, an fzf pick.
func (m *Model) fromPopup(msg wireIn) (tea.Model, tea.Cmd) {
	if msg.conn != m.conn {
		return m, nil // an old connection
	}
	if msg.bye {
		m.conn = nil
		return m, nil
	}
	wait := waitWire(m.conn)
	switch w := msg.m; w.Kind {
	case "hello", "size":
		m.popupW, m.popupH, m.overlaySent = w.W, w.H, "\x00"
		return m, tea.Batch(wait, m.publishPreview())
	case "key":
		if w.Key != nil {
			_, cmd := m.update(tea.KeyMsg(*w.Key))
			return m, tea.Batch(wait, cmd)
		}
	case "prefix": // tmux gets these keys once the popup has closed
		m.replay = w.Keys
		return m, wait
	case "picked":
		var err error
		if w.Err != "" {
			err = fmt.Errorf("%s", w.Err)
		}
		_, cmd := m.update(dirPickedMsg{dir: w.Dir, err: err})
		return m, tea.Batch(wait, cmd)
	}
	return m, wait
}

// modalInPopup reports whether dialogs are drawn in the popup instead of
// the narrow sidebar.
func (m *Model) modalInPopup() bool {
	return m.sidebar && m.popupOpen && m.conn != nil && m.popupW > 0
}

// syncOverlay sends the open dialog, rendered for the popup, when it changed.
func (m *Model) syncOverlay() {
	if !m.modalInPopup() {
		return
	}
	ov := ""
	if m.modal != nil {
		ov = m.modal.view(m.popupW, m.popupH)
	}
	if ov != m.overlaySent {
		m.overlaySent = ov
		m.conn.post(wire{Kind: "overlay", Overlay: ov})
	}
}

// runPopup shows the popup; the command returns when the popup closes.
func (m *Model) runPopup(client string) tea.Cmd {
	c, pane, sock := m.c, m.selfPane, ""
	if m.link != nil {
		sock = m.link.ln.Addr().String()
	}
	return func() tea.Msg {
		start := time.Now()
		err := c.RunPreviewPopup(pane, client, sock)
		return popupDoneMsg{err, time.Since(start)}
	}
}

// popupMinOpen: a popup that "closes" sooner than this never opened.
const popupMinOpen = 300 * time.Millisecond

// popupDone handles the popup closing. When the sidebar did not close it
// itself (esc, a switch), it was closed some other way: leave the sidebar
// too, so focus is not stuck in it without a preview.
func (m *Model) popupDone(err error, d time.Duration) tea.Cmd {
	if keys := m.replay; keys != nil { // closed to hand a prefix key to tmux
		m.replay, m.popupOpen = nil, false
		c, client := m.c, m.popupClient
		return func() tea.Msg {
			c.SendClientKeys(client, keys)
			time.Sleep(300 * time.Millisecond) // let the binding run
			return replayedMsg{}
		}
	}
	if !m.popupOpen {
		return nil
	}
	m.popupOpen = false
	if err != nil {
		return m.setToast("✗ preview popup: "+err.Error(), true)
	}
	if d < popupMinOpen { // never really opened (another popup is open on the client)
		return nil
	}
	if m.focused {
		return m.leaveSidebar()
	}
	return nil
}

// previewInfo is what the popup should show: the pane, its title, and the
// help context (as openHelp names it).
func (m *Model) previewInfo() tmux.PreviewState {
	ctx := m.helpCtx()
	if m.page == pageAgents {
		a, p := m.curAgent()
		if a == nil || p == nil {
			return tmux.PreviewState{Title: "select an agent", Context: ctx}
		}
		return tmux.PreviewState{Pane: p.ID, Context: ctx,
			Title: p.Target() + " · " + string(a.Kind) + " · " + a.Label() + " " + agent.Age(a.Since, clock())}
	}
	p := m.previewTarget()
	if p == nil || p.ID == m.selfPane {
		return tmux.PreviewState{Title: "nothing to preview", Context: ctx}
	}
	return tmux.PreviewState{Pane: p.ID, Title: p.Target() + " · " + p.Command, Context: ctx}
}

// publishPreview tells the popup what to show when that changed. It stands
// in for fetchPreview in the sidebar: the popup does the capturing.
func (m *Model) publishPreview() tea.Cmd {
	if !m.focused || !m.popupOpen || m.selfPane == "" {
		return nil
	}
	st := m.previewInfo()
	key := st.Pane + "\t" + st.Title + "\t" + st.Context
	if key == m.published {
		return nil
	}
	m.published = key
	if m.conn != nil {
		m.conn.post(wire{Kind: "state", State: &st})
	}
	return nil
}

// closePopup closes the preview popup (after a switch).
func (m *Model) closePopup() tea.Cmd {
	if !m.popupOpen {
		return nil
	}
	m.popupOpen = false
	c, client := m.c, m.popupClient
	return func() tea.Msg { c.ClosePopup(client); return nil }
}

// unfocusSelf hands the focus in the sidebar's window back to the last pane
// after a switch to another session, so coming back does not land in the
// sidebar (and reopen the popup).
func (m *Model) unfocusSelf() tea.Cmd {
	if m.selfPane == "" {
		return nil
	}
	c, pane := m.c, m.selfPane
	return func() tea.Msg {
		if out, _ := c.Run("display-message", "-p", "-t", pane, "#{pane_active}"); out == "1" {
			c.FocusLastPane(pane)
		}
		return nil
	}
}

// leaveSidebar closes the popup and moves focus back to the pane you came
// from.
func (m *Model) leaveSidebar() tea.Cmd {
	c, pane, client, open := m.c, m.selfPane, m.popupClient, m.popupOpen
	m.popupOpen = false // its popupDoneMsg must not leave a second time
	return func() tea.Msg {
		if open {
			c.ClosePopup(client)
		}
		c.FocusLastPane(pane)
		return popupClosedMsg{}
	}
}

// sidebarOff closes every sidebar (including this one).
func (m *Model) sidebarOff() tea.Cmd {
	c, client, open := m.c, m.popupClient, m.popupOpen
	m.popupOpen = false
	return func() tea.Msg {
		if open {
			c.ClosePopup(client)
		}
		c.SidebarOff()
		return nil
	}
}

// compactHeader is the sidebar's first line: the agent counts as colored chips (" ◐1 " " ●2 "
// " ✓1 ") on the left and "lazymux" on the right.
func (m *Model) compactHeader(w int) string {
	st := agent.Count(m.agents, clock())
	left := []seg{{" ", stText}}
	for _, x := range []struct {
		n      int
		glyph  string
		fg, bg lipgloss.TerminalColor
	}{{st.Blocked, "◐", cRed, cRedBg}, {st.Working, "●", cYellow, cYellowBg}, {st.Done, "✓", cGreen, cGreenBg}} {
		if x.n > 0 {
			left = append(left, seg{fmt.Sprintf(" %s%d ", x.glyph, x.n),
				lipgloss.NewStyle().Foreground(x.fg).Background(x.bg).Bold(true)}, seg{" ", stText})
		}
	}
	return renderRow(left, []seg{{"lazymux ", stMauve.Bold(true)}}, w, 0)
}
