package tmux

import (
	"fmt"
	"strconv"
	"strings"
)

// Session is a tmux session with its windows in index order.
type Session struct {
	ID       string // $1
	Name     string
	Attached bool
	Windows  []*Window
}

// Window is a tmux window with its panes in index order.
type Window struct {
	ID      string // @3
	Index   int
	Name    string
	Active  bool   // active window of its session
	Last    bool   // previously active window of its session
	Layout  string // tmux layout string, e.g. "b25d,180x40,0,0{90x40,0,0,1,89x40,91,0,2}"
	Session *Session
	Panes   []*Pane
}

// Pane is a single tmux pane.
type Pane struct {
	ID      string // %12
	Index   int
	Active  bool // active pane of its window
	Last    bool // previously active pane of its window
	Command string
	Path    string
	PID     int
	Width   int
	Height  int
	Title   string
	Window  *Window
}

// Target returns "session:index.pane" for display, e.g. "work:2.%5".
func (p *Pane) Target() string {
	return fmt.Sprintf("%s:%d.%s", p.Window.Session.Name, p.Window.Index, p.ID)
}

// ActiveWindow returns the session's active window (or the first one).
func (s *Session) ActiveWindow() *Window {
	for _, w := range s.Windows {
		if w.Active {
			return w
		}
	}
	if len(s.Windows) > 0 {
		return s.Windows[0]
	}
	return nil
}

// ActivePane returns the window's active pane (or the first one).
func (w *Window) ActivePane() *Pane {
	for _, p := range w.Panes {
		if p.Active {
			return p
		}
	}
	if len(w.Panes) > 0 {
		return w.Panes[0]
	}
	return nil
}

// Tree is the full server state from one list-panes call.
type Tree struct {
	Sessions []*Session
}

// fields queried per pane; pane_title is last because it is free text and may
// itself contain the separator.
var paneFields = []string{
	"session_id", "session_name", "session_attached",
	"window_id", "window_index", "window_name", "window_active", "window_layout",
	"pane_id", "pane_index", "pane_active", "pane_current_command",
	"pane_current_path", "pane_pid", "pane_width", "pane_height",
	"window_last_flag", "pane_last",
	"pane_title",
}

const sep = "\t"

func paneFormat() string {
	parts := make([]string, len(paneFields))
	for i, f := range paneFields {
		parts[i] = "#{" + f + "}"
	}
	return strings.Join(parts, sep)
}

// ListTree queries every session, window and pane on the server.
func (c *Client) ListTree() (*Tree, error) {
	out, err := c.Run("list-panes", "-a", "-F", paneFormat())
	if err != nil {
		return nil, err
	}
	return ParseTree(out)
}

// ParseTree builds a Tree from list-panes output produced with paneFormat.
// Row order from tmux is session → window index → pane index, which we keep.
func ParseTree(out string) (*Tree, error) {
	t := &Tree{}
	sessions := map[string]*Session{}
	windows := map[string]*Window{} // key: session_id + window_id (linked windows repeat per session)
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		f := strings.SplitN(line, sep, len(paneFields))
		if len(f) != len(paneFields) {
			return nil, fmt.Errorf("unexpected list-panes line (%d fields): %q", len(f), line)
		}
		s, ok := sessions[f[0]]
		if !ok {
			s = &Session{ID: f[0], Name: f[1], Attached: f[2] != "0"}
			sessions[f[0]] = s
			t.Sessions = append(t.Sessions, s)
		}
		wkey := f[0] + f[3]
		w, ok := windows[wkey]
		if !ok {
			w = &Window{ID: f[3], Index: atoi(f[4]), Name: f[5], Active: f[6] == "1", Layout: f[7], Last: f[16] == "1", Session: s}
			windows[wkey] = w
			s.Windows = append(s.Windows, w)
		}
		w.Panes = append(w.Panes, &Pane{
			ID: f[8], Index: atoi(f[9]), Active: f[10] == "1", Command: f[11],
			Path: f[12], PID: atoi(f[13]), Width: atoi(f[14]), Height: atoi(f[15]),
			Last: f[17] == "1", Title: f[18], Window: w,
		})
	}
	return t, nil
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// Current returns the session, window and pane ids of the client lazymux runs
// in (or the pinned ClientTTY). Empty strings when not determinable.
func (c *Client) Current() (sessionID, windowID, paneID string) {
	args := []string{"display-message", "-p"}
	if c.ClientTTY != "" {
		args = append(args, "-c", c.ClientTTY)
	}
	args = append(args, "#{session_id}"+sep+"#{window_id}"+sep+"#{pane_id}")
	out, err := c.Run(args...)
	if err != nil {
		return "", "", ""
	}
	f := strings.Split(out, sep)
	if len(f) != 3 {
		return "", "", ""
	}
	return f[0], f[1], f[2]
}
