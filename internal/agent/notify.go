package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/afamrk/lazymux/internal/tmux"
)

// Notifications: when an agent goes from working to blocked (it needs you)
// or from working to idle (it finished), one lazymux process (the leader,
// see leader.go) tells you: a desktop notification, a notification through
// your terminal (OSC 9 / 99), a tmux message, and / or a sound. Like herdr, a
// note waits a moment (Config.Delay) and is dropped if the state changes
// again before it is due, so a flicker does not notify.

// NoteKind is what a notification is about.
type NoteKind string

const (
	NoteBlocked NoteKind = "blocked"
	NoteDone    NoteKind = "done"
)

// Note is one notification.
type Note struct {
	Kind  NoteKind
	Pane  string
	Title string // "claude needs you"
	Body  string // "api:3 · Bash(go test ./...)"
}

// Config is read from tmux global options (set them in tmux.conf):
//
//	@lazymux-notify        system | terminal | tmux | off, or several: "system,tmux" (default system)
//	@lazymux-notify-on     blocked,done (default both)
//	@lazymux-notify-delay  seconds to wait before notifying (default 1)
//	@lazymux-sound         on | off (default on)
//	@lazymux-sound-blocked sound file for blocked (default: freedesktop message-new-instant)
//	@lazymux-sound-done    sound file for done (default: freedesktop complete)
type Config struct {
	Delivery     []string
	On           map[NoteKind]bool
	Delay        time.Duration
	Sound        bool
	SoundBlocked string
	SoundDone    string
}

// DefaultConfig is the configuration without any options set.
func DefaultConfig() Config {
	return Config{
		Delivery:     []string{"system"},
		On:           map[NoteKind]bool{NoteBlocked: true, NoteDone: true},
		Delay:        time.Second,
		Sound:        true,
		SoundBlocked: "/usr/share/sounds/freedesktop/stereo/message-new-instant.oga",
		SoundDone:    "/usr/share/sounds/freedesktop/stereo/complete.oga",
	}
}

// ParseConfig reads the @lazymux-* options from `show-options -g` output.
func ParseConfig(out string) Config {
	cfg := DefaultConfig()
	for _, l := range strings.Split(out, "\n") {
		name, val, ok := strings.Cut(strings.TrimSpace(l), " ")
		if !ok || !strings.HasPrefix(name, "@lazymux-") {
			continue
		}
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		list := func() []string {
			var out []string
			for _, x := range strings.Split(val, ",") {
				if x = strings.TrimSpace(x); x != "" {
					out = append(out, x)
				}
			}
			return out
		}
		switch name {
		case "@lazymux-notify":
			cfg.Delivery = nil
			for _, x := range list() {
				if x != "off" {
					cfg.Delivery = append(cfg.Delivery, x)
				}
			}
		case "@lazymux-notify-on":
			cfg.On = map[NoteKind]bool{}
			for _, x := range list() {
				cfg.On[NoteKind(x)] = true
			}
		case "@lazymux-notify-delay":
			if f, err := strconv.ParseFloat(val, 64); err == nil && f >= 0 {
				cfg.Delay = time.Duration(f * float64(time.Second))
			}
		case "@lazymux-sound":
			cfg.Sound = val == "on" || val == "1" || val == "true"
		case "@lazymux-sound-blocked":
			cfg.SoundBlocked = expandHome(val)
		case "@lazymux-sound-done":
			cfg.SoundDone = expandHome(val)
		}
	}
	return cfg
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return home + p[1:]
		}
	}
	return p
}

// LoadConfig reads the options from the server.
func LoadConfig(c *tmux.Client) Config {
	out, err := c.Run("show-options", "-g")
	if err != nil {
		return DefaultConfig()
	}
	return ParseConfig(out)
}

// Notifier turns state changes into notes.
type Notifier struct {
	prev    map[string]State
	pending map[string]pendingNote
}

type pendingNote struct {
	note  Note
	state State // the state that caused it; a change cancels the note
	due   time.Time
}

// NewNotifier creates an empty notifier.
func NewNotifier() *Notifier {
	return &Notifier{prev: map[string]State{}, pending: map[string]pendingNote{}}
}

// Observe compares agents with the previous call and returns the notes that
// are due now. The first sight of an agent never notifies.
func (n *Notifier) Observe(agents map[string]*Agent, t *tmux.Tree, cfg Config, now time.Time) []Note {
	panes := map[string]*tmux.Pane{}
	for _, s := range t.Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				panes[p.ID] = p
			}
		}
	}
	for id, a := range agents {
		prev, known := n.prev[id]
		n.prev[id] = a.State
		if pn, ok := n.pending[id]; ok && pn.state != a.State {
			delete(n.pending, id) // changed again before it was due
		}
		if !known || prev == a.State || panes[id] == nil {
			continue
		}
		var kind NoteKind
		switch {
		case a.State == Blocked:
			kind = NoteBlocked
		case a.State == Idle && prev == Working:
			kind = NoteDone
		default:
			continue
		}
		if !cfg.On[kind] {
			continue
		}
		n.pending[id] = pendingNote{note: noteFor(kind, a, panes[id]), state: a.State, due: now.Add(cfg.Delay)}
	}
	for id := range n.prev {
		if agents[id] == nil {
			delete(n.prev, id)
			delete(n.pending, id)
		}
	}
	var due []Note
	for id, pn := range n.pending {
		if !now.Before(pn.due) {
			due = append(due, pn.note)
			delete(n.pending, id)
		}
	}
	return due
}

func noteFor(kind NoteKind, a *Agent, p *tmux.Pane) Note {
	where := fmt.Sprintf("%s:%d", p.Window.Session.Name, p.Window.Index)
	if kind == NoteBlocked {
		body := where
		if a.Waiting != "" {
			body += " · " + a.Waiting
		}
		return Note{Kind: kind, Pane: a.Pane, Title: string(a.Kind) + " needs you", Body: body}
	}
	body := where
	if a.Summary != "" {
		body += " · " + a.Summary
	}
	return Note{Kind: kind, Pane: a.Pane, Title: string(a.Kind) + " finished", Body: body}
}

// Deliver sends a note the configured ways. It runs commands and should be
// called off the UI goroutine.
func Deliver(c *tmux.Client, cfg Config, n Note) {
	for _, d := range cfg.Delivery {
		switch d {
		case "system":
			systemNotify(n)
		case "terminal":
			terminalNotify(c, n)
		case "tmux":
			tmuxNotify(c, n)
		}
	}
	if cfg.Sound {
		go playSound(soundFor(cfg, n))
	}
}

// DeliverSync is Deliver that waits for the sound to finish (for `lazymux
// notify test`, which exits right after).
func DeliverSync(c *tmux.Client, cfg Config, n Note) {
	sound := cfg.Sound
	cfg.Sound = false
	Deliver(c, cfg, n)
	if sound {
		playSound(soundFor(cfg, n))
	}
}

func soundFor(cfg Config, n Note) string {
	if n.Kind == NoteBlocked {
		return cfg.SoundBlocked
	}
	return cfg.SoundDone
}

func systemNotify(n Note) {
	if runtime.GOOS == "darwin" {
		script := fmt.Sprintf("display notification %q with title %q", n.Body, n.Title)
		_ = exec.Command("osascript", "-e", script).Run()
		return
	}
	icon := "emblem-default"
	if n.Kind == NoteBlocked {
		icon = "dialog-warning"
	}
	_ = exec.Command("notify-send", "-a", "lazymux", "-i", icon, n.Title, n.Body).Run()
}

// terminalNotify writes a notification escape sequence straight to the
// terminal of every attached client that understands one: OSC 9 for
// ghostty, iTerm2 and WezTerm, OSC 99 for kitty. Writing to the client's tty
// avoids tmux's passthrough setting.
func terminalNotify(c *tmux.Client, n Note) {
	out, err := c.Run("list-clients", "-F", "#{client_tty}\t#{client_termname}")
	if err != nil {
		return
	}
	for _, l := range strings.Split(out, "\n") {
		tty, term, _ := strings.Cut(l, "\t")
		seq := TerminalSequence(term, n.Title, n.Body)
		if tty == "" || seq == "" {
			continue
		}
		if f, err := os.OpenFile(tty, os.O_WRONLY, 0); err == nil {
			_, _ = f.WriteString(seq)
			f.Close()
		}
	}
}

// TerminalSequence is the notification escape sequence for a terminal
// type, or "" when it has none.
func TerminalSequence(term, title, body string) string {
	clean := func(s string) string {
		return strings.Map(func(r rune) rune {
			switch r {
			case '\x1b', '\a', '\u009c':
				return -1
			case '\n', '\r', '\t':
				return ' '
			}
			return r
		}, s)
	}
	t := strings.ToLower(term)
	switch {
	case strings.Contains(t, "kitty"):
		return "\x1b]99;i=1:d=0;" + clean(title) + "\x1b\\\x1b]99;i=1:p=body;" + clean(body) + "\x1b\\"
	case strings.Contains(t, "ghostty"), strings.Contains(t, "wezterm"), strings.Contains(t, "iterm"):
		return "\x1b]9;" + clean(title+": "+body) + "\x1b\\"
	}
	return ""
}

// tmuxNotify shows the note in the status line of every attached client.
func tmuxNotify(c *tmux.Client, n Note) {
	out, err := c.Run("list-clients", "-F", "#{client_tty}")
	if err != nil {
		return
	}
	glyph := "●"
	if n.Kind == NoteBlocked {
		glyph = "⣿"
	}
	msg := strings.ReplaceAll(glyph+" "+n.Title+": "+n.Body, "#", "##")
	for _, tty := range strings.Split(out, "\n") {
		if tty != "" {
			c.Run("display-message", "-c", tty, "-d", "5000", msg)
		}
	}
}

// soundPlayers can play .oga / .wav / .mp3 files; the first one found is used.
var soundPlayers = [][]string{
	{"paplay"}, {"pw-play"}, {"ffplay", "-nodisp", "-autoexit", "-loglevel", "quiet"},
	{"mpv", "--no-video", "--really-quiet"}, {"afplay"},
}

func playSound(file string) {
	if file == "" || os.Getenv("LAZYMUX_NO_SOUND") != "" {
		return
	}
	if _, err := os.Stat(file); err != nil {
		return
	}
	for _, p := range soundPlayers {
		bin, err := exec.LookPath(p[0])
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err = exec.CommandContext(ctx, bin, append(p[1:], file)...).Run()
		cancel()
		if err == nil {
			return
		}
	}
}
