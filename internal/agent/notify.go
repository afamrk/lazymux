package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/afamrk/lazymux/internal/tmux"
)

// Notifications: when an agent goes from working to blocked (it needs you)
// or from working to idle (it finished), one lazymux process (the leader,
// see leader.go) tells you: a desktop notification, a notification through
// your terminal (OSC 9 / 99), a tmux message, and / or a sound. Like herdr, a
// note waits a moment (Config.Delay) and is dropped if the state changes
// again before it is due, so a flicker does not notify.
//
// Notes you do not need are not sent:
//   - the agent's pane is the one you are looking at (tmux.WatchedPanes);
//   - "finished" when you looked at the agent a moment ago (you probably
//     interrupted it, or just left it) or when its turn was shorter than
//     Config.MinWork;
//   - an agent has one desktop notification at a time: a new note replaces
//     the last one, and a "needs you" note is closed once it is answered;
//   - an agent's sound plays at most once every soundGap.

// NoteKind is what a notification is about.
type NoteKind string

const (
	NoteBlocked NoteKind = "blocked"
	NoteDone    NoteKind = "done"
	NoteClear   NoteKind = "clear" // the agent's "needs you" was answered: close it
)

// Note is one notification.
type Note struct {
	Kind   NoteKind
	Pane   string
	Title  string // "Claude needs you", "Claude finished · 2m 13s"
	Place  string // "api › editor"
	Verb   string // what it wants to do: "run", "edit", "asks" ("" when unknown)
	Detail string // its target ("go test ./..."), or what it worked on
}

// what is the verb and its target: "run  go test ./...".
func (n Note) what() string {
	if n.Verb == "" {
		return n.Detail
	}
	return n.Verb + "  " + n.Detail
}

// Body is the plain notification text: the place over what it is about.
func (n Note) Body() string {
	if n.Detail == "" {
		return n.Place
	}
	return n.Place + "\n" + n.what()
}

// Text is the body on one line (terminal and tmux messages).
func (n Note) Text() string {
	if n.Detail == "" {
		return n.Place
	}
	return n.Place + " · " + strings.Replace(n.what(), "  ", " ", 1)
}

// Config is read from tmux global options (set them in tmux.conf):
//
//	@lazymux-notify           system | terminal | tmux | off, or several: "system,tmux" (default system)
//	@lazymux-notify-on        blocked,done (default both)
//	@lazymux-notify-delay     seconds to wait before notifying (default 1)
//	@lazymux-notify-min-work  seconds a turn must last to notify "finished" (default 10)
//	@lazymux-sound            on | off (default on)
//	@lazymux-sound-blocked    sound file for blocked (default: freedesktop message-new-instant)
//	@lazymux-sound-done       sound file for done (default: freedesktop complete)
type Config struct {
	Delivery     []string
	On           map[NoteKind]bool
	Delay        time.Duration
	MinWork      time.Duration
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
		MinWork:      10 * time.Second,
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
		seconds := func(d *time.Duration) {
			if f, err := strconv.ParseFloat(val, 64); err == nil && f >= 0 {
				*d = time.Duration(f * float64(time.Second))
			}
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
			seconds(&cfg.Delay)
		case "@lazymux-notify-min-work":
			seconds(&cfg.MinWork)
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

// leftWindow: a "finished" within this long of looking at the agent is not
// sent (you interrupted it, or just left it).
const leftWindow = 10 * time.Second

// Notifier turns state changes into notes.
type Notifier struct {
	prev      map[string]State
	pending   map[string]pendingNote
	workStart map[string]time.Time // when the current turn started
	watchedAt map[string]time.Time // when you last looked at the agent
	asked     map[string]bool      // a "needs you" note was sent and not cleared
}

type pendingNote struct {
	note  Note
	state State // the state that caused it; a change cancels the note
	due   time.Time
}

// NewNotifier creates an empty notifier.
func NewNotifier() *Notifier {
	return &Notifier{prev: map[string]State{}, pending: map[string]pendingNote{},
		workStart: map[string]time.Time{}, watchedAt: map[string]time.Time{}, asked: map[string]bool{}}
}

// Observe compares agents with the previous call and returns the notes that
// are due now. watched holds the panes you are looking at. The first sight
// of an agent never notifies.
func (n *Notifier) Observe(agents map[string]*Agent, t *tmux.Tree, watched map[string]bool, cfg Config, now time.Time) []Note {
	panes := map[string]*tmux.Pane{}
	for _, s := range t.Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				panes[p.ID] = p
			}
		}
	}
	var due []Note
	for id, a := range agents {
		if watched[id] {
			n.watchedAt[id] = now
		}
		prev, known := n.prev[id]
		n.prev[id] = a.State
		if a.State == Working && (!known || prev == Idle) { // a new turn (blocked → working goes on)
			n.workStart[id] = now
		}
		if pn, ok := n.pending[id]; ok && pn.state != a.State {
			delete(n.pending, id) // changed again before it was due
		}
		if known && prev == Blocked && a.State != Blocked && n.asked[id] {
			delete(n.asked, id)
			due = append(due, Note{Kind: NoteClear, Pane: id})
		}
		if !known || prev == a.State || panes[id] == nil || watched[id] {
			continue
		}
		var kind NoteKind
		switch {
		case a.State == Blocked:
			kind = NoteBlocked
		case a.State == Idle && prev == Working:
			if now.Sub(n.workStart[id]) < cfg.MinWork {
				continue // a quick turn: you saw it coming
			}
			if at, ok := n.watchedAt[id]; ok && now.Sub(at) < leftWindow {
				continue // you were just there (an interrupt, most likely)
			}
			kind = NoteDone
		default:
			continue
		}
		if !cfg.On[kind] {
			continue
		}
		n.pending[id] = pendingNote{note: noteFor(kind, a, panes[id], now.Sub(n.workStart[id])), state: a.State, due: now.Add(cfg.Delay)}
	}
	for id := range n.prev {
		if agents[id] == nil {
			delete(n.prev, id)
			delete(n.pending, id)
			delete(n.workStart, id)
			delete(n.watchedAt, id)
			if n.asked[id] {
				delete(n.asked, id)
				due = append(due, Note{Kind: NoteClear, Pane: id})
			}
		}
	}
	for id, pn := range n.pending {
		if now.Before(pn.due) {
			continue
		}
		delete(n.pending, id)
		if watched[id] { // you went to it meanwhile
			continue
		}
		if pn.note.Kind == NoteBlocked {
			n.asked[id] = true
		}
		due = append(due, pn.note)
	}
	return due
}

// DisplayName is the agent's name in notifications.
func DisplayName(k Kind) string {
	switch k {
	case Claude:
		return "Claude"
	case Codex:
		return "Codex"
	case Gemini:
		return "Gemini"
	case Opencode:
		return "OpenCode"
	case Aider:
		return "Aider"
	case Agy:
		return "Antigravity"
	}
	return string(k)
}

func noteFor(kind NoteKind, a *Agent, p *tmux.Pane, worked time.Duration) Note {
	nt := Note{Kind: kind, Pane: a.Pane, Place: notePlace(a.Kind, p)}
	if kind == NoteBlocked {
		nt.Title = DisplayName(a.Kind) + " needs you"
		nt.Verb, nt.Detail = describeWaiting(a.Waiting, p.Path)
	} else {
		nt.Title, nt.Detail = DisplayName(a.Kind)+" finished · "+Duration(worked), humanize(a.Summary)
	}
	return nt
}

// Duration formats how long a turn took: "45s", "2m 13s", "1h 05m".
func Duration(d time.Duration) string {
	s := int(d.Round(time.Second).Seconds())
	switch {
	case s < 60:
		return fmt.Sprintf("%ds", s)
	case s < 3600:
		return fmt.Sprintf("%dm %02ds", s/60, s%60)
	}
	return fmt.Sprintf("%dh %02dm", s/3600, s%3600/60)
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// soundGap: an agent's sound plays at most once in this long.
const soundGap = 5 * time.Second

// delivered remembers, per agent pane, its desktop notification (to replace
// or close it) and when its sound last played. Only the leader delivers.
var delivered = struct {
	sync.Mutex
	id    map[string]string
	sound map[string]time.Time
}{id: map[string]string{}, sound: map[string]time.Time{}}

// Deliver sends a note the configured ways. It runs commands and should be
// called off the UI goroutine.
func Deliver(c *tmux.Client, cfg Config, n Note) {
	if n.Kind == NoteClear {
		closeSystem(n.Pane)
		return
	}
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
	if cfg.Sound && soundDue(n.Pane, time.Now()) {
		go playSound(soundFor(cfg, n))
	}
}

// soundDue reports whether pane's sound may play now, and notes that it did.
func soundDue(pane string, now time.Time) bool {
	if pane == "" {
		return true
	}
	delivered.Lock()
	defer delivered.Unlock()
	if last, ok := delivered.sound[pane]; ok && now.Sub(last) < soundGap {
		return false
	}
	delivered.sound[pane] = now
	return true
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
		script := fmt.Sprintf("display notification %q with title %q", n.Body(), n.Title)
		_ = exec.Command("osascript", "-e", script).Run()
		return
	}
	args := []string{"-p"}
	delivered.Lock()
	old := delivered.id[n.Pane]
	delivered.Unlock()
	if old != "" {
		args = append(args, "-r", old) // one notification per agent
	}
	out, err := exec.Command("notify-send", append(args, desktopArgs(n, serverStyle())...)...).Output()
	if id := strings.TrimSpace(string(out)); err == nil && id != "" && n.Pane != "" {
		delivered.Lock()
		delivered.id[n.Pane] = id
		delivered.Unlock()
	}
}

// closeSystem closes the agent's desktop notification, if it has one.
func closeSystem(pane string) {
	delivered.Lock()
	id := delivered.id[pane]
	delete(delivered.id, pane)
	delivered.Unlock()
	if id == "" || runtime.GOOS == "darwin" {
		return
	}
	_ = exec.Command("gdbus", "call", "--session", "--dest", "org.freedesktop.Notifications",
		"--object-path", "/org/freedesktop/Notifications",
		"--method", "org.freedesktop.Notifications.CloseNotification", id).Run()
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
		seq := TerminalSequence(term, n.Title, n.Text())
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
	head := "#[fg=green,bold]✓ "
	if n.Kind == NoteBlocked {
		head = "#[fg=red,bold]◐ "
	}
	msg := head + esc(n.Title) + "#[default]  " + esc(n.Text())
	for _, tty := range strings.Split(out, "\n") {
		if tty != "" {
			c.Run("display-message", "-c", tty, "-d", "5000", msg)
		}
	}
}

// esc keeps tmux from reading # in text as a format.
func esc(s string) string { return strings.ReplaceAll(s, "#", "##") }

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
