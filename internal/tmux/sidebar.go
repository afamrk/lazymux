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

// SidebarCycle is the one key for the sidebar (bind it to prefix + b):
// when sidebars are off it opens one and moves into it, when the session's
// sidebar is open it moves into it, and when you are in it (pane is the
// sidebar) it closes all of them. session and pane are where the key was
// pressed ('#{session_id}', '#{pane_id}' in the binding).
func (c *Client) SidebarCycle(session, pane string) error {
	if session == "" || pane == "" {
		out, err := c.Run("display-message", "-p", "#{session_id}\t#{pane_id}")
		if err != nil {
			return err
		}
		session, pane, _ = strings.Cut(out, "\t")
	}
	if c.SidebarsOn() {
		if mine := c.sessionSidebar(session); mine == pane && mine != "" {
			return c.SidebarOff()
		}
	} else if err := c.SidebarToggle(); err != nil {
		return err
	}
	if err := c.SidebarFollow(session); err != nil { // creates or docks it
		return err
	}
	mine := c.sessionSidebar(session)
	if mine == "" {
		return fmt.Errorf("no sidebar in session %s", session)
	}
	_, err := c.Run("select-pane", "-t", mine)
	return err
}

// sessionSidebar is the pane of session's sidebar, or "".
func (c *Client) sessionSidebar(session string) string {
	refs, _ := c.sidebars()
	for _, r := range refs {
		if r.session == session {
			return r.pane
		}
	}
	return ""
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

// The preview popup: while the sidebar is focused, a display-popup on its
// right shows the selected pane and the sidebar's dialogs. The two programs
// talk over a unix socket (see internal/ui/link.go).

// PreviewState is what the sidebar tells its popup: the pane to show, the
// title, and the help context (the focused panel or "Agents").
type PreviewState struct{ Pane, Title, Context string }

// ActiveClient returns the tty of the most recently active client attached
// to session ("" when none is attached).
func (c *Client) ActiveClient(session string) string {
	out, err := c.Run("list-clients", "-t", session, "-F", "#{client_activity}\t#{client_tty}")
	if err != nil {
		return ""
	}
	best, bestAct := "", -1
	for _, l := range strings.Split(out, "\n") {
		f := strings.SplitN(l, "\t", 2)
		if len(f) == 2 && atoi(f[0]) > bestAct {
			best, bestAct = f[1], atoi(f[0])
		}
	}
	return best
}

// popupGeometry is where the preview popup goes: next to the sidebar, a
// share of the space on its right (@lazymux-preview-width, default 70%) and
// of its height (@lazymux-preview-height, default 70%), centered vertically.
type popupGeometry struct {
	x, y, w, h int // y is the popup's bottom edge (display-popup's -y)
}

const (
	previewWidthOpt  = "@lazymux-preview-width"  // percent of the space right of the sidebar
	previewHeightOpt = "@lazymux-preview-height" // percent of the sidebar's height
	defPreviewWidth  = 70
	defPreviewHeight = 70
)

// previewPopupGeometry queries the sidebar's place in client coordinates
// (a status line at the top shifts everything down).
func (c *Client) previewPopupGeometry(sidebar string) (popupGeometry, error) {
	out, err := c.Run("display-message", "-p", "-t", sidebar,
		"#{pane_left}\t#{pane_width}\t#{pane_top}\t#{pane_height}\t#{window_width}\t#{status}\t#{status-position}"+
			"\t#{"+previewWidthOpt+"}\t#{"+previewHeightOpt+"}")
	if err != nil {
		return popupGeometry{}, err
	}
	f := strings.Split(out, "\t")
	if len(f) != 9 {
		return popupGeometry{}, fmt.Errorf("unexpected display-message output %q", out)
	}
	left, pw, top, ph, ww := atoi(f[0]), atoi(f[1]), atoi(f[2]), atoi(f[3]), atoi(f[4])
	if f[6] == "top" {
		top += statusLines(f[5])
	}
	return popupPlace(left, pw, top, ph, ww, percent(f[7], defPreviewWidth), percent(f[8], defPreviewHeight)), nil
}

// popupPlace computes the geometry from the sidebar's position and size.
func popupPlace(left, pw, top, ph, ww, wPct, hPct int) popupGeometry {
	x := left + pw + 1 // skip the border
	w := max(20, (ww-x)*wPct/100)
	w = min(w, max(1, ww-x))
	h := min(ph, max(5, ph*hPct/100))
	t := top + (ph-h)/2
	return popupGeometry{x: x, y: t + h, w: w, h: h}
}

// percent parses an option like "60" or "60%" (1–100), or returns def.
func percent(v string, def int) int {
	n := atoi(strings.TrimSuffix(strings.TrimSpace(v), "%"))
	if n < 1 || n > 100 {
		return def
	}
	return n
}

// statusLines is the height of the status line for the status option.
func statusLines(v string) int {
	switch v {
	case "off", "":
		return 0
	case "on":
		return 1
	}
	return atoi(v)
}

// previewPopupArgs is the borderless display-popup command for the preview.
func (c *Client) previewPopupArgs(sidebar, client, socket string, g popupGeometry) []string {
	args := []string{"display-popup", "-E", "-B"}
	if client != "" {
		args = append(args, "-c", client)
	}
	return append(args, "-t", sidebar,
		"-x", strconv.Itoa(g.x), "-y", strconv.Itoa(g.y), "-w", strconv.Itoa(g.w), "-h", strconv.Itoa(g.h),
		c.sidebarCommand("preview", sidebar, shQuote(socket)))
}

// RunPreviewPopup shows the preview popup next to the sidebar on client,
// connecting to the sidebar's socket, and waits until the popup is closed.
func (c *Client) RunPreviewPopup(sidebar, client, socket string) error {
	g, err := c.previewPopupGeometry(sidebar)
	if err != nil {
		return err
	}
	_, err = c.Run(c.previewPopupArgs(sidebar, client, socket, g)...)
	return err
}

// SendClientKeys has client handle keys as if you typed them (its key
// table, so a prefix starts a binding).
func (c *Client) SendClientKeys(client string, keys []string) {
	args := []string{"send-keys", "-K"}
	if client != "" {
		args = append(args, "-c", client)
	}
	c.Run(append(args, keys...)...)
}

// ClosePopup closes the popup open on client (if any).
func (c *Client) ClosePopup(client string) {
	args := []string{"display-popup", "-C"}
	if client != "" {
		args = append(args, "-c", client)
	}
	c.Run(args...)
}
