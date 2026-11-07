package tmux

import (
	"os"
	"path/filepath"
	"strings"
)

// CapturePane returns the visible contents of a pane with ANSI escapes kept.
func (c *Client) CapturePane(paneID string) (string, error) {
	return c.Run("capture-pane", "-e", "-p", "-t", paneID)
}

// Select makes w the active window of s and p the active pane of w, without
// switching any client. w and p may be nil to leave them unchanged.
func (c *Client) Select(s *Session, w *Window, p *Pane) error {
	if w != nil {
		if _, err := c.Run("select-window", "-t", s.ID+":"+w.ID); err != nil {
			return err
		}
	}
	if p != nil {
		if _, err := c.Run("select-pane", "-t", p.ID); err != nil {
			return err
		}
	}
	return nil
}

// SwitchTo selects w/p (see Select) and switches the client to session s.
func (c *Client) SwitchTo(s *Session, w *Window, p *Pane) error {
	if err := c.Select(s, w, p); err != nil {
		return err
	}
	args := []string{"switch-client"}
	if c.ClientTTY != "" {
		args = append(args, "-c", c.ClientTTY)
	}
	args = append(args, "-t", s.ID)
	_, err := c.Run(args...)
	return err
}

// AttachArgs returns the argv to attach a fresh terminal to the session, used
// when lazymux runs outside tmux.
func (c *Client) AttachArgs(s *Session) []string {
	return c.Command("attach-session", "-t", s.ID)
}

// TrimCapture drops trailing blank lines from capture-pane output and returns
// at most the last n lines (n <= 0 means all).
func TrimCapture(out string, n int) []string {
	lines := strings.Split(out, "\n")
	for len(lines) > 0 && strings.TrimSpace(stripANSI(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// stripANSI removes CSI escape sequences; good enough for blank-line checks.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// --- creation --------------------------------------------------------------

// NewSession creates a detached session and returns its id.
func (c *Client) NewSession(name, dir string) (string, error) {
	args := []string{"new-session", "-d", "-P", "-F", "#{session_id}"}
	if name != "" {
		args = append(args, "-s", name)
	}
	if dir != "" {
		args = append(args, "-c", dir)
	}
	return c.Run(args...)
}

// NewWindow creates a window at the end of session s (without switching to
// it) and returns "window_id pane_id".
func (c *Client) NewWindow(sessionID, name, dir string) (windowID, paneID string, err error) {
	args := []string{"new-window", "-d", "-P", "-F", "#{window_id} #{pane_id}", "-t", sessionID + ":"}
	if name != "" {
		args = append(args, "-n", name)
	}
	if dir != "" {
		args = append(args, "-c", dir)
	}
	out, err := c.Run(args...)
	if err != nil {
		return "", "", err
	}
	windowID, paneID, _ = strings.Cut(out, " ")
	return windowID, paneID, nil
}

// SplitPane splits the pane side by side (vertical=false) or top/bottom and
// returns the new pane id. The new pane starts in dir.
func (c *Client) SplitPane(paneID, dir string, vertical bool) (string, error) {
	dirFlag := "-h"
	if vertical {
		dirFlag = "-v"
	}
	args := []string{"split-window", dirFlag, "-d", "-P", "-F", "#{pane_id}", "-t", paneID}
	if dir != "" {
		args = append(args, "-c", dir)
	}
	return c.Run(args...)
}

// --- rename ----------------------------------------------------------------

func (c *Client) RenameSession(sessionID, name string) error {
	_, err := c.Run("rename-session", "-t", sessionID, name)
	return err
}

func (c *Client) RenameWindow(sessionID, windowID, name string) error {
	_, err := c.Run("rename-window", "-t", sessionID+":"+windowID, name)
	return err
}

// SetPaneTitle sets the pane title (panes have no names in tmux).
func (c *Client) SetPaneTitle(paneID, title string) error {
	_, err := c.Run("select-pane", "-t", paneID, "-T", title)
	return err
}

// --- kill ------------------------------------------------------------------

func (c *Client) KillSession(sessionID string) error {
	_, err := c.Run("kill-session", "-t", sessionID)
	return err
}

// KillWindow uses unlink-window -k: a window linked into several sessions is
// only removed from this one; otherwise it is killed.
func (c *Client) KillWindow(sessionID, windowID string) error {
	_, err := c.Run("unlink-window", "-k", "-t", sessionID+":"+windowID)
	return err
}

func (c *Client) KillPane(paneID string) error {
	_, err := c.Run("kill-pane", "-t", paneID)
	return err
}

// --- rearrange -------------------------------------------------------------

// MoveWindow moves a window to the end of another session.
func (c *Client) MoveWindow(srcSessionID, windowID, dstSessionID string) error {
	_, err := c.Run("move-window", "-d", "-s", srcSessionID+":"+windowID, "-t", dstSessionID+":")
	return err
}

// JoinPane moves pane src into dst (a pane or window target) as a split.
func (c *Client) JoinPane(srcPaneID, dstTarget string) error {
	_, err := c.Run("join-pane", "-d", "-h", "-s", srcPaneID, "-t", dstTarget)
	return err
}

func (c *Client) SwapWindows(aSess, aWin, bSess, bWin string) error {
	_, err := c.Run("swap-window", "-d", "-s", aSess+":"+aWin, "-t", bSess+":"+bWin)
	return err
}

func (c *Client) SwapPanes(a, b string) error {
	_, err := c.Run("swap-pane", "-d", "-s", a, "-t", b)
	return err
}

// BreakPane moves a pane out of its split into a new window of its own, at
// the end of the pane's own session. (Without -t, tmux would put the window
// in the current client's session, i.e. wherever lazymux runs.)
func (c *Client) BreakPane(paneID, sessionID string) error {
	_, err := c.Run("break-pane", "-d", "-s", paneID, "-t", sessionID+":")
	return err
}

// ToggleZoom zooms the pane to fill its window, or un-zooms it.
func (c *Client) ToggleZoom(paneID string) error {
	_, err := c.Run("resize-pane", "-Z", "-t", paneID)
	return err
}

// NextLayout cycles the window through tmux's preset layouts.
func (c *Client) NextLayout(sessionID, windowID string) error {
	_, err := c.Run("next-layout", "-t", sessionID+":"+windowID)
	return err
}

// SelectLayout applies a layout string (from window_layout) to a window.
func (c *Client) SelectLayout(target, layout string) error {
	_, err := c.Run("select-layout", "-t", target, layout)
	return err
}

// SendLine types text into a pane followed by Enter.
func (c *Client) SendLine(paneID, text string) error {
	if _, err := c.Run("send-keys", "-t", paneID, "-l", text); err != nil {
		return err
	}
	_, err := c.Run("send-keys", "-t", paneID, "Enter")
	return err
}

// CaptureText returns the visible contents of a pane as plain text.
func (c *Client) CaptureText(paneID string) (string, error) {
	return c.Run("capture-pane", "-p", "-t", paneID)
}

// SendKeys sends tmux key names (e.g. "Escape", "C-c", "y") to a pane.
func (c *Client) SendKeys(paneID string, keys ...string) error {
	_, err := c.Run(append([]string{"send-keys", "-t", paneID}, keys...)...)
	return err
}

// ServerName is the name of the tmux server lazymux talks to: the -L socket,
// or the socket of the tmux lazymux runs in, or "default".
func (c *Client) ServerName() string {
	if c.Socket != "" {
		return c.Socket
	}
	return ServerFromEnv(os.Getenv("TMUX"))
}

// ServerFromEnv extracts the socket name from a $TMUX value such as
// "/tmp/tmux-1000/default,3751,0".
func ServerFromEnv(env string) string {
	path, _, _ := strings.Cut(env, ",")
	if path == "" {
		return "default"
	}
	return filepath.Base(path)
}
