package tmux

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// The sidebar is a narrow pane on the left of the current window that runs
// `lazymux sidebar`. There is at most one per session; a hook moves it into
// whichever window becomes current. A global user option remembers whether
// sidebars are turned on, so sessions you switch to get one too.
const (
	SidebarOpt   = "@lazymux_sidebar"    // pane option: id of the session the sidebar serves
	sidebarOnOpt = "@lazymux_sidebar_on" // global option: "1" while sidebars are on
	SidebarWidth = 30
	hookIndex    = "90" // fixed hook slot, so user hooks are left alone
)

var sidebarHooks = []string{"session-window-changed", "client-session-changed"}

// sidebarRef is one sidebar pane found on the server.
type sidebarRef struct {
	pane, window, session string // session = the one it serves (option value)
	left, height, winH    int
}

func (c *Client) sidebars() ([]sidebarRef, error) {
	out, err := c.Run("list-panes", "-a", "-F",
		"#{pane_id}\t#{window_id}\t#{"+SidebarOpt+"}\t#{pane_left}\t#{pane_height}\t#{window_height}")
	if err != nil {
		return nil, err
	}
	var refs []sidebarRef
	for _, l := range strings.Split(out, "\n") {
		f := strings.Split(l, "\t")
		if len(f) != 6 || f[2] == "" {
			continue
		}
		refs = append(refs, sidebarRef{pane: f[0], window: f[1], session: f[2],
			left: atoi(f[3]), height: atoi(f[4]), winH: atoi(f[5])})
	}
	return refs, nil
}

// SidebarsOn reports whether sidebars are turned on.
func (c *Client) SidebarsOn() bool {
	out, _ := c.Run("show-options", "-gqv", sidebarOnOpt)
	return out == "1"
}

// sidebarCmdOverride replaces the lazymux command in tests (the test binary
// must not be started inside a pane).
var sidebarCmdOverride string

// sidebarCommand is the shell command that runs lazymux with args against
// this server.
func (c *Client) sidebarCommand(args ...string) string {
	if sidebarCmdOverride != "" {
		return sidebarCmdOverride
	}
	exe, err := os.Executable()
	if err != nil {
		exe = "lazymux"
	}
	parts := []string{shQuote(exe)}
	if c.Socket != "" {
		parts = append(parts, "-L", shQuote(c.Socket))
	}
	return strings.Join(append(parts, args...), " ")
}

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// SidebarToggle turns sidebars on (opening one in the current session) or
// off (closing all of them).
func (c *Client) SidebarToggle() error {
	if c.SidebarsOn() {
		return c.SidebarOff()
	}
	if _, err := c.Run("set-option", "-g", sidebarOnOpt, "1"); err != nil {
		return err
	}
	follow := c.sidebarCommand("sidebar", "follow", "'#{session_id}'")
	for _, h := range sidebarHooks {
		if _, err := c.Run("set-hook", "-g", h+"["+hookIndex+"]", `run-shell -b "`+follow+`"`); err != nil {
			return err
		}
	}
	sess, err := c.Run("display-message", "-p", "#{session_id}")
	if err != nil {
		return err
	}
	return c.SidebarFollow(sess)
}

// SidebarOff closes every sidebar and removes the hooks.
func (c *Client) SidebarOff() error {
	c.Run("set-option", "-gu", sidebarOnOpt)
	for _, h := range sidebarHooks {
		c.Run("set-hook", "-gu", h+"["+hookIndex+"]")
	}
	refs, err := c.sidebars()
	if err != nil {
		return err
	}
	for _, r := range refs {
		c.Run("kill-pane", "-t", r.pane)
	}
	return nil
}

// SidebarFollow makes sure session has its sidebar docked on the left of its
// current window: it creates one, moves it there, or re-docks it after a
// layout change. Nothing happens while sidebars are off.
func (c *Client) SidebarFollow(session string) error {
	if !c.SidebarsOn() {
		return nil
	}
	win, err := c.Run("display-message", "-p", "-t", session, "#{window_id}")
	if err != nil {
		return err
	}
	refs, err := c.sidebars()
	if err != nil {
		return err
	}
	var mine *sidebarRef
	for i := range refs {
		if refs[i].session == session {
			if mine != nil { // duplicates (e.g. two hooks raced): keep one
				c.Run("kill-pane", "-t", refs[i].pane)
				continue
			}
			mine = &refs[i]
		}
	}
	if mine == nil {
		pane, err := c.Run("split-window", "-hbfd", "-l", strconv.Itoa(SidebarWidth), "-t", win,
			"-P", "-F", "#{pane_id}", c.sidebarCommand("sidebar"))
		if err != nil {
			return err
		}
		_, err = c.Run("set-option", "-p", "-t", pane, SidebarOpt, session)
		return err
	}
	if mine.window == win && mine.left == 0 && mine.height == mine.winH {
		return nil // already in place
	}
	if mine.window == win { // knocked out of place (e.g. next-layout): undock first
		if _, err := c.Run("break-pane", "-d", "-s", mine.pane); err != nil {
			return err
		}
	}
	_, err = c.Run("join-pane", "-hbfd", "-l", strconv.Itoa(SidebarWidth), "-s", mine.pane, "-t", win)
	return err
}

// ResizeWidth sets a pane's width in columns.
func (c *Client) ResizeWidth(pane string, cols int) error {
	_, err := c.Run("resize-pane", "-t", pane, "-x", strconv.Itoa(cols))
	return err
}

// SidebarPaneState describes the sidebar pane as tmux sees it.
type SidebarPaneState struct {
	PaneW, WinW int
	Focused     bool   // active pane of the active window of an attached session
	Docked      bool   // full height on the left edge
	Session     string // session it serves
}

// SidebarState queries the sidebar pane's size, focus and placement.
func (c *Client) SidebarState(pane string) (SidebarPaneState, error) {
	out, err := c.Run("display-message", "-p", "-t", pane,
		"#{pane_width}\t#{window_width}\t#{pane_active}#{window_active}#{?session_attached,1,0}"+
			"\t#{pane_left}\t#{pane_height}\t#{window_height}\t#{"+SidebarOpt+"}")
	if err != nil {
		return SidebarPaneState{}, err
	}
	f := strings.Split(out, "\t")
	if len(f) != 7 {
		return SidebarPaneState{}, fmt.Errorf("unexpected display-message output %q", out)
	}
	return SidebarPaneState{
		PaneW: atoi(f[0]), WinW: atoi(f[1]), Focused: f[2] == "111",
		Docked: f[3] == "0" && f[4] == f[5], Session: f[6],
	}, nil
}

// FocusLastPane moves focus from the sidebar back to the pane you came from.
func (c *Client) FocusLastPane(sidebar string) error {
	if _, err := c.Run("select-pane", "-l", "-t", sidebar); err == nil {
		return nil
	}
	_, err := c.Run("select-pane", "-R", "-t", sidebar) // no last pane: the one to the right
	return err
}
