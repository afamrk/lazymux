package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/afamrk/lazymux/internal/tmux"
)

// Every sidebar detects agents, but only one of them, the leader, sends
// notifications and writes the status file that `lazymux status` prints.
// The leader is whoever holds an exclusive lock on a per-server lock file;
// when it exits, the kernel releases the lock and another lazymux takes
// over on its next detection.

// cacheDir is lazymux's cache directory (the hook events live under it too).
func cacheDir() string {
	return filepath.Dir(EventDir())
}

// Leader tries to hold the lock for one tmux server.
type Leader struct {
	path string
	f    *os.File
}

// NewLeader prepares the lock for server (nothing is locked yet).
func NewLeader(server string) *Leader {
	return &Leader{path: filepath.Join(cacheDir(), "leader-"+safeName(server)+".lock")}
}

// Acquire reports whether this process is the leader, taking the lock when
// it is free. Cheap to call on every detection.
func (l *Leader) Acquire() bool {
	if l.f != nil {
		return true
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return false
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return false
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return false
	}
	l.f = f
	return true
}

// Release gives up leadership.
func (l *Leader) Release() {
	if l.f != nil {
		l.f.Close()
		l.f = nil
	}
}

func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == '%' || r == '.' {
			return '_'
		}
		return r
	}, s)
}

// Status is the summary written by the leader for `lazymux status`.
type Status struct {
	T       time.Time `json:"t"`
	Blocked int       `json:"blocked"`
	Working int       `json:"working"`
	Done    int       `json:"done"` // finished and not looked at yet
	Idle    int       `json:"idle"`
}

// Count summarizes agents.
func Count(agents map[string]*Agent, now time.Time) Status {
	st := Status{T: now}
	for _, a := range agents {
		switch {
		case a.State == Blocked:
			st.Blocked++
		case a.State == Working:
			st.Working++
		case a.Done():
			st.Done++
		default:
			st.Idle++
		}
	}
	return st
}

func statusFile(server string) string {
	return filepath.Join(cacheDir(), "status-"+safeName(server)+".json")
}

// WriteStatus stores the summary for server (atomically).
func WriteStatus(server string, st Status) error {
	path := statusFile(server)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, _ := json.Marshal(st)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// statusMaxAge is how old the leader's summary may be before `lazymux
// status` detects on its own (no sidebar running).
const statusMaxAge = 10 * time.Second

// CurrentStatus returns the leader's summary when it is fresh, otherwise a
// one-shot detection. A one-shot detection has no history, so it cannot
// tell done from idle and reports neither.
func CurrentStatus(c *tmux.Client) Status {
	now := time.Now()
	if data, err := os.ReadFile(statusFile(c.ServerName())); err == nil {
		var st Status
		if json.Unmarshal(data, &st) == nil && now.Sub(st.T) < statusMaxAge {
			return st
		}
	}
	t, err := c.ListTree()
	if err != nil {
		return Status{T: now}
	}
	st := Count(NewDetector(c).Detect(t), now)
	st.Done, st.Idle = 0, 0
	return st
}

// FormatStatus renders "⣿1 ⣾2 ●1", leaving out states with no agents. With
// tmux set, colors are tmux style tags (#[fg=…]) for status-right.
func FormatStatus(st Status, tmuxStyle bool) string {
	var parts []string
	for _, x := range []struct {
		n     int
		glyph string
		color string
	}{
		{st.Blocked, "⣿", "#f38ba8"},
		{st.Working, "⣾", "#f9e2af"},
		{st.Done, "●", "#a6e3a1"},
	} {
		if x.n == 0 {
			continue
		}
		p := x.glyph + strconv.Itoa(x.n)
		if tmuxStyle {
			p = "#[fg=" + x.color + "]" + p + "#[fg=default]"
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, " ")
}
