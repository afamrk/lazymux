package agent

import (
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/afamrk/lazymux/internal/tmux"
)

// Detector finds agents in a tmux tree and keeps per-pane history between
// calls (when the state changed, whether the screen keeps changing). It is
// safe to call from several goroutines, one call at a time is enough.
type Detector struct {
	server  string // tmux server name, for hook event files
	dir     string // hook event directory
	now     func() time.Time
	capture func(pane string) (string, error)
	procs   func() map[int]proc

	mu    sync.Mutex
	known map[string]*track
}

type track struct {
	a       Agent
	screen  string
	changed time.Time // last time the screen changed
	evMod   time.Time // event file mtime when last read
	evSize  int64
}

// NewDetector creates a detector for the server c talks to.
func NewDetector(c *tmux.Client) *Detector {
	return &Detector{
		server:  c.ServerName(),
		dir:     EventDir(),
		now:     time.Now,
		capture: c.CaptureText,
		procs:   processes,
		known:   map[string]*track{},
	}
}

// changeWindow is how long after a screen change an agent without a working
// indicator (aider) still counts as working.
const changeWindow = 3 * time.Second

// Detect returns the agents in t by pane id.
func (d *Detector) Detect(t *tmux.Tree) map[string]*Agent {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := d.now()

	type found struct {
		kind Kind
		pane *tmux.Pane
	}
	var list []found
	var procs map[int]proc
	for _, s := range t.Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				k := KindOf(p.Command)
				if k == "" && isInterpreter(p.Command) {
					if procs == nil {
						procs = d.procs()
					}
					k = kindUnder(p.PID, procs)
				}
				if k != "" {
					list = append(list, found{k, p})
				}
			}
		}
	}

	out := map[string]*Agent{}
	seen := map[string]bool{}
	for _, f := range list {
		id := f.pane.ID
		if seen[id] { // linked windows list a pane twice
			continue
		}
		seen[id] = true
		tr := d.known[id]
		fresh := tr == nil || tr.a.Kind != f.kind
		if fresh {
			tr = &track{a: Agent{Pane: id, Kind: f.kind, Since: now}, changed: now}
			d.known[id] = tr
		}
		screen, err := d.capture(id)
		if err != nil {
			screen = tr.screen
		}
		if screen != tr.screen {
			tr.screen, tr.changed = screen, now
		}

		st, waiting, ok := Classify(f.kind, screen)
		if !ok {
			st = Idle
			if !fresh && now.Sub(tr.changed) < changeWindow {
				st = Working
			}
		}
		if f.kind == Claude {
			d.loadEvents(tr)
		}
		a := &tr.a
		if st != a.State {
			a.State, a.Since = st, now
		}
		if st == Working {
			a.Worked = true
		}
		a.Waiting = ""
		if st == Blocked {
			a.Waiting = waiting
			if i := PendingTool(a.Events); i >= 0 {
				e := a.Events[i]
				a.Waiting = e.Name
				if e.Detail != "" {
					a.Waiting += "(" + e.Detail + ")"
				}
			}
		}
		a.Summary = ""
		if e := lastOf(a.Events, "prompt"); e != nil {
			a.Summary = e.Detail
			a.Worked = true
		} else if f.kind == Claude {
			a.Summary = titleSummary(f.pane.Title)
		}
		cp := *a
		cp.Events = append([]Event(nil), a.Events...)
		out[id] = &cp
	}
	for id := range d.known {
		if !seen[id] {
			delete(d.known, id)
		}
	}
	return out
}

// loadEvents re-reads the pane's hook log when it changed.
func (d *Detector) loadEvents(tr *track) {
	path := eventFile(d.dir, d.server, tr.a.Pane)
	st, err := os.Stat(path)
	if err != nil {
		tr.a.Events, tr.a.Hooked = nil, false
		return
	}
	if st.ModTime().Equal(tr.evMod) && st.Size() == tr.evSize {
		return
	}
	evs, err := readEvents(path)
	if err != nil {
		return
	}
	tr.evMod, tr.evSize = st.ModTime(), st.Size()
	if e := lastEvent(evs); e != nil && e.Kind == "end" {
		evs = nil // that session has ended; this agent has not reported yet
	}
	tr.a.Events, tr.a.Hooked = evs, len(evs) > 0
}

func lastEvent(evs []Event) *Event {
	if len(evs) == 0 {
		return nil
	}
	return &evs[len(evs)-1]
}

// PendingTool is the index of the last tool call when nothing but
// notifications came after it (the call a blocked agent waits on), or -1.
func PendingTool(evs []Event) int {
	for i := len(evs) - 1; i >= 0; i-- {
		switch evs[i].Kind {
		case "tool":
			return i
		case "notify":
			continue
		}
		return -1
	}
	return -1
}

func lastOf(evs []Event, kind string) *Event {
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Kind == kind {
			return &evs[i]
		}
	}
	return nil
}

// titleSummary turns Claude Code's terminal title ("✳ Fix JWT expiry") into
// a summary; the default title says nothing.
func titleSummary(title string) string {
	t := strings.TrimLeftFunc(title, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	t = strings.TrimSpace(t)
	if t == "" || strings.EqualFold(t, "claude code") || strings.EqualFold(t, "claude") {
		return ""
	}
	if h, _ := os.Hostname(); t == h || strings.HasPrefix(h, t+".") {
		return ""
	}
	return t
}
