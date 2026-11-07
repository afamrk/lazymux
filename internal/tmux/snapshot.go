package tmux

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Snapshots record enough about a session/window/pane to recreate it after a
// kill. tmux cannot revive processes, so a restore opens a shell in the same
// directory and re-runs the program that was in the foreground (if any).

type PaneSnap struct {
	Dir string
	Cmd string // command to re-run, "" for a plain shell
}

type WindowSnap struct {
	Name   string
	Index  int
	Layout string
	Panes  []PaneSnap
}

type Snapshot struct {
	Kind        string // "session", "window" or "pane"
	Label       string // for messages, e.g. "window 2:server"
	SessionID   string
	SessionName string
	WindowID    string // pane snapshots: the window the pane was in
	Windows     []WindowSnap
	Pane        PaneSnap
}

// shells and interpreters that are not worth re-running on restore.
var noRerun = map[string]bool{
	"bash": true, "zsh": true, "fish": true, "sh": true, "dash": true, "nu": true,
	"node": true, "python": true, "python3": true, "ruby": true, "login": true, "tmux": true,
}

// rerunCommand maps a pane's foreground command to what a restore should run.
func rerunCommand(cmd string) string {
	base := filepath.Base(cmd)
	switch {
	case base == "" || noRerun[base] || strings.HasPrefix(base, "-"):
		return ""
	case base == "claude":
		return "claude --continue" // resume the last conversation in that directory
	case base == "codex":
		return "codex resume --last"
	}
	return base
}

func snapPane(p *Pane) PaneSnap { return PaneSnap{Dir: p.Path, Cmd: rerunCommand(p.Command)} }

func snapWindow(w *Window) WindowSnap {
	ws := WindowSnap{Name: w.Name, Index: w.Index, Layout: w.Layout}
	for _, p := range w.Panes {
		ws.Panes = append(ws.Panes, snapPane(p))
	}
	return ws
}

func SnapSession(s *Session) *Snapshot {
	snap := &Snapshot{Kind: "session", Label: "session " + s.Name, SessionID: s.ID, SessionName: s.Name}
	for _, w := range s.Windows {
		snap.Windows = append(snap.Windows, snapWindow(w))
	}
	return snap
}

func SnapWindow(w *Window) *Snapshot {
	return &Snapshot{
		Kind: "window", Label: fmt.Sprintf("window %d:%s", w.Index, w.Name),
		SessionID: w.Session.ID, SessionName: w.Session.Name,
		Windows: []WindowSnap{snapWindow(w)},
	}
}

func SnapPane(p *Pane) *Snapshot {
	return &Snapshot{
		Kind: "pane", Label: "pane " + p.ID + " (" + p.Command + ")",
		SessionID: p.Window.Session.ID, SessionName: p.Window.Session.Name,
		WindowID: p.Window.ID, Pane: snapPane(p),
	}
}

// Restore recreates a killed item.
func (c *Client) Restore(s *Snapshot) error {
	switch s.Kind {
	case "session":
		return c.restoreSession(s)
	case "window":
		target, err := c.sessionTarget(s)
		if err != nil {
			return err
		}
		return c.restoreWindow(target, s.Windows[0], "")
	case "pane":
		return c.restorePane(s)
	}
	return fmt.Errorf("unknown snapshot kind %q", s.Kind)
}

// sessionTarget returns the session id if it still exists, else its name if a
// session with that name exists.
func (c *Client) sessionTarget(s *Snapshot) (string, error) {
	if _, err := c.Run("has-session", "-t", s.SessionID); err == nil {
		return s.SessionID, nil
	}
	if _, err := c.Run("has-session", "-t", "="+s.SessionName); err == nil {
		return "=" + s.SessionName, nil
	}
	return "", fmt.Errorf("session %s no longer exists", s.SessionName)
}

func (c *Client) restoreSession(s *Snapshot) error {
	if len(s.Windows) == 0 {
		return nil
	}
	first := s.Windows[0]
	dir := ""
	if len(first.Panes) > 0 {
		dir = first.Panes[0].Dir
	}
	out, err := c.Run("new-session", "-d", "-P", "-F", "#{session_id} #{window_id}", "-s", s.SessionName, "-n", first.Name, "-c", dir)
	if err != nil {
		return err
	}
	sid, wid, _ := strings.Cut(out, " ")
	if err := c.restoreWindow(sid, first, wid); err != nil {
		return err
	}
	for _, w := range s.Windows[1:] {
		if err := c.restoreWindow(sid, w, ""); err != nil {
			return err
		}
	}
	return nil
}

// restoreWindow creates window ws in session target. If existing is a window
// id, that (fresh, one-pane) window is reused for the first pane.
func (c *Client) restoreWindow(target string, ws WindowSnap, existing string) error {
	if len(ws.Panes) == 0 {
		return nil
	}
	wid := existing
	var paneIDs []string
	if wid == "" {
		args := []string{"new-window", "-d", "-P", "-F", "#{window_id} #{pane_id}", "-n", ws.Name, "-c", ws.Panes[0].Dir}
		out, err := c.Run(append(args, "-t", fmt.Sprintf("%s:%d", target, ws.Index))...)
		if err != nil { // index taken: append at the end instead
			out, err = c.Run(append(args, "-t", target+":")...)
		}
		if err != nil {
			return err
		}
		var pid string
		wid, pid, _ = strings.Cut(out, " ")
		paneIDs = append(paneIDs, pid)
	} else {
		pid, err := c.Run("display-message", "-p", "-t", wid, "#{pane_id}")
		if err != nil {
			return err
		}
		paneIDs = append(paneIDs, pid)
	}
	for _, p := range ws.Panes[1:] {
		pid, err := c.Run("split-window", "-d", "-P", "-F", "#{pane_id}", "-t", wid, "-c", p.Dir)
		if err != nil {
			return err
		}
		paneIDs = append(paneIDs, pid)
	}
	if ws.Layout != "" && len(ws.Panes) > 1 {
		_ = c.SelectLayout(wid, ws.Layout) // best effort: fails if the window size changed a lot
	}
	for i, p := range ws.Panes {
		if p.Cmd != "" {
			if err := c.SendLine(paneIDs[i], p.Cmd); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Client) restorePane(s *Snapshot) error {
	var pid string
	if _, err := c.Run("display-message", "-p", "-t", s.WindowID, "#{window_id}"); err == nil {
		out, err := c.Run("split-window", "-d", "-P", "-F", "#{pane_id}", "-t", s.WindowID, "-c", s.Pane.Dir)
		if err != nil {
			return err
		}
		pid = out
	} else { // its window is gone too: give it a new window
		target, err := c.sessionTarget(s)
		if err != nil {
			return err
		}
		_, pid, err = c.NewWindow(target, "", s.Pane.Dir)
		if err != nil {
			return err
		}
	}
	if s.Pane.Cmd != "" {
		return c.SendLine(pid, s.Pane.Cmd)
	}
	return nil
}
