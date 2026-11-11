package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/afamrk/lazymux/internal/tmux"
)

func TestParseConfig(t *testing.T) {
	cfg := ParseConfig("status on\n@lazymux-notify-min-work 30\n@lazymux-notify \"terminal,tmux\"\n@lazymux-notify-on blocked\n@lazymux-notify-delay 2.5\n@lazymux-sound off\n@lazymux-sound-done ~/done.wav\n")
	if strings.Join(cfg.Delivery, ",") != "terminal,tmux" || !cfg.On[NoteBlocked] || cfg.On[NoteDone] ||
		cfg.Delay != 2500*time.Millisecond || cfg.MinWork != 30*time.Second || cfg.Sound || !strings.HasSuffix(cfg.SoundDone, "/done.wav") || strings.HasPrefix(cfg.SoundDone, "~") {
		t.Errorf("parsed %+v", cfg)
	}
	if d := ParseConfig("@lazymux-notify off\n").Delivery; len(d) != 0 {
		t.Errorf("off should disable delivery: %v", d)
	}
	if d := DefaultConfig(); d.Delivery[0] != "system" || !d.On[NoteDone] || !d.Sound {
		t.Errorf("defaults %+v", d)
	}
}

// notifierRig drives a Notifier for one claude agent on pane %1.
type notifierRig struct {
	t       *testing.T
	n       *Notifier
	tree    *tmux.Tree
	cfg     Config
	now     time.Time
	watched map[string]bool
}

func newRig(t *testing.T) *notifierRig {
	tree, _ := tmux.ParseTree(row("%1", "claude", "x"))
	return &notifierRig{t: t, n: NewNotifier(), tree: tree, cfg: DefaultConfig(), now: time.Unix(1000, 0), watched: map[string]bool{}}
}

func (r *notifierRig) obs(st State, waiting string) []Note {
	return r.n.Observe(map[string]*Agent{"%1": {Pane: "%1", Kind: Claude, State: st, Waiting: waiting, Summary: "fix jwt"}},
		r.tree, r.watched, r.cfg, r.now)
}

func (r *notifierRig) wait(d time.Duration) { r.now = r.now.Add(d) }

func kinds(ns []Note) string {
	var k []string
	for _, n := range ns {
		k = append(k, string(n.Kind))
	}
	return strings.Join(k, ",")
}

func TestNotifierTransitionsAndDelay(t *testing.T) {
	r := newRig(t)
	if got := r.obs(Working, ""); len(got) != 0 {
		t.Fatalf("first sight must not notify: %v", got)
	}
	if got := r.obs(Blocked, "Bash(go test)"); len(got) != 0 {
		t.Fatalf("a note waits for the delay: %v", got)
	}
	r.wait(1500 * time.Millisecond)
	got := r.obs(Blocked, "Bash(go test)")
	if len(got) != 1 || got[0].Kind != NoteBlocked || got[0].Title != "Claude needs you" ||
		got[0].Detail != "Bash(go test)" || got[0].Place != "work · 1:w1" {
		t.Fatalf("blocked note: %+v", got)
	}
	// answered: the "needs you" note is closed; then a flicker to idle is cancelled
	if got := r.obs(Working, ""); kinds(got) != "clear" {
		t.Errorf("answering should clear the note: %v", got)
	}
	r.obs(Idle, "")
	if got := r.obs(Working, ""); len(got) != 0 {
		t.Errorf("flicker notified: %v", got)
	}
	r.wait(3 * time.Second)
	if got := r.obs(Working, ""); len(got) != 0 {
		t.Errorf("cancelled note delivered later: %v", got)
	}
	r.wait(20 * time.Second)
	r.obs(Idle, "")
	r.wait(2 * time.Second)
	if got := r.obs(Idle, ""); len(got) != 1 || got[0].Kind != NoteDone || got[0].Title != "Claude finished · 23s" || got[0].Detail != "fix jwt" {
		t.Errorf("done note: %+v", got)
	}
	r.cfg.On = map[NoteKind]bool{NoteBlocked: true}
	r.obs(Working, "")
	r.wait(20 * time.Second)
	r.obs(Idle, "")
	r.wait(2 * time.Second)
	if got := r.obs(Idle, ""); len(got) != 0 {
		t.Errorf("done is turned off but notified: %v", got)
	}
}

func TestNotifierSkipsWhatYouSee(t *testing.T) {
	// you are looking at the pane: nothing
	r := newRig(t)
	r.watched["%1"] = true
	r.obs(Working, "")
	r.wait(time.Minute)
	r.obs(Blocked, "Bash")
	r.wait(2 * time.Second)
	if got := r.obs(Blocked, "Bash"); len(got) != 0 {
		t.Errorf("blocked while watched: %v", got)
	}
	r.obs(Working, "")
	r.wait(time.Minute)
	r.obs(Idle, "")
	r.wait(2 * time.Second)
	if got := r.obs(Idle, ""); len(got) != 0 {
		t.Errorf("done while watched: %v", got)
	}

	// you went to the pane while the note waited for its delay: dropped
	r = newRig(t)
	r.obs(Working, "")
	r.obs(Blocked, "Bash")
	r.watched["%1"] = true
	r.wait(2 * time.Second)
	if got := r.obs(Blocked, "Bash"); len(got) != 0 {
		t.Errorf("note delivered after you went to it: %v", got)
	}

	// interrupted and left at once: no "finished"
	r = newRig(t)
	r.obs(Working, "")
	r.wait(time.Minute)
	r.watched["%1"] = true
	r.obs(Working, "")
	r.wait(2 * time.Second)
	delete(r.watched, "%1")
	r.obs(Idle, "")
	r.wait(2 * time.Second)
	if got := r.obs(Idle, ""); len(got) != 0 {
		t.Errorf("interrupt notified: %v", got)
	}

	// a quick turn: no "finished"
	r = newRig(t)
	r.obs(Idle, "")
	r.obs(Working, "")
	r.wait(3 * time.Second)
	r.obs(Idle, "")
	r.wait(2 * time.Second)
	if got := r.obs(Idle, ""); len(got) != 0 {
		t.Errorf("quick turn notified: %v", got)
	}
	// a blocked prompt in the middle counts toward the same turn
	r.obs(Working, "")
	r.wait(6 * time.Second)
	r.obs(Blocked, "Bash")
	r.wait(2 * time.Second)
	r.obs(Blocked, "Bash")
	r.obs(Working, "")
	r.wait(6 * time.Second)
	r.obs(Idle, "")
	r.wait(2 * time.Second)
	if got := r.obs(Idle, ""); kinds(got) != "done" || got[0].Title != "Claude finished · 14s" {
		t.Errorf("long turn with a prompt: %+v", got)
	}
}

func TestDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		45 * time.Second: "45s", 133 * time.Second: "2m 13s", 3900 * time.Second: "1h 05m",
	} {
		if got := Duration(d); got != want {
			t.Errorf("Duration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestTerminalSequence(t *testing.T) {
	if s := TerminalSequence("xterm-ghostty", "claude needs you", "api:3\x1b]bad"); s != "\x1b]9;claude needs you: api:3]bad\x1b\\" {
		t.Errorf("ghostty: %q", s)
	}
	if s := TerminalSequence("xterm-kitty", "t", "b"); !strings.Contains(s, "]99;i=1:d=0;t") || !strings.Contains(s, "]99;i=1:p=body;b") {
		t.Errorf("kitty: %q", s)
	}
	if s := TerminalSequence("xterm-256color", "t", "b"); s != "" {
		t.Errorf("unknown terminal should get nothing: %q", s)
	}
}

func TestStatusFormatAndLeader(t *testing.T) {
	if s := FormatStatus(Status{Blocked: 1, Working: 2, Done: 1, Idle: 4}, false); s != "◐1 ●2 ✓1" {
		t.Errorf("plain status %q", s)
	}
	if s := FormatStatus(Status{Working: 2}, true); s != "#[fg=#f9e2af]●2#[fg=default]" {
		t.Errorf("tmux status %q", s)
	}
	if s := FormatStatus(Status{Idle: 3}, false); s != "" {
		t.Errorf("only seen idle agents: want empty, got %q", s)
	}
	st := Count(map[string]*Agent{"a": {State: Idle, Unseen: true}, "b": {State: Idle}, "c": {State: Blocked}}, time.Now())
	if st.Done != 1 || st.Idle != 1 || st.Blocked != 1 {
		t.Errorf("count %+v", st)
	}

	t.Setenv("LAZYMUX_EVENTS", filepath.Join(t.TempDir(), "events"))
	a, b := NewLeader("srv"), NewLeader("srv")
	if !a.Acquire() || b.Acquire() {
		t.Fatal("exactly one leader expected")
	}
	a.Release()
	if !b.Acquire() {
		t.Error("the lock should pass on when the leader exits")
	}
	b.Release()
	if err := WriteStatus("srv", Status{T: time.Now(), Blocked: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(statusFile("srv")); err != nil {
		t.Error(err)
	}
}
