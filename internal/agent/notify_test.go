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
	cfg := ParseConfig("status on\n@lazymux-notify \"terminal,tmux\"\n@lazymux-notify-on blocked\n@lazymux-notify-delay 2.5\n@lazymux-sound off\n@lazymux-sound-done ~/done.wav\n")
	if strings.Join(cfg.Delivery, ",") != "terminal,tmux" || !cfg.On[NoteBlocked] || cfg.On[NoteDone] ||
		cfg.Delay != 2500*time.Millisecond || cfg.Sound || !strings.HasSuffix(cfg.SoundDone, "/done.wav") || strings.HasPrefix(cfg.SoundDone, "~") {
		t.Errorf("parsed %+v", cfg)
	}
	if d := ParseConfig("@lazymux-notify off\n").Delivery; len(d) != 0 {
		t.Errorf("off should disable delivery: %v", d)
	}
	if d := DefaultConfig(); d.Delivery[0] != "system" || !d.On[NoteDone] || !d.Sound {
		t.Errorf("defaults %+v", d)
	}
}

func TestNotifierTransitionsAndDelay(t *testing.T) {
	tree, _ := tmux.ParseTree(row("%1", "claude", "x"))
	n := NewNotifier()
	cfg := DefaultConfig()
	now := time.Unix(1000, 0)
	obs := func(st State, waiting string) []Note {
		return n.Observe(map[string]*Agent{"%1": {Pane: "%1", Kind: Claude, State: st, Waiting: waiting, Summary: "fix jwt"}}, tree, cfg, now)
	}
	if got := obs(Working, ""); len(got) != 0 {
		t.Fatalf("first sight must not notify: %v", got)
	}
	if got := obs(Blocked, "Bash(go test)"); len(got) != 0 {
		t.Fatalf("a note waits for the delay: %v", got)
	}
	now = now.Add(1500 * time.Millisecond)
	got := obs(Blocked, "Bash(go test)")
	if len(got) != 1 || got[0].Kind != NoteBlocked || got[0].Title != "claude needs you" || got[0].Body != "work:1 · Bash(go test)" {
		t.Fatalf("blocked note: %+v", got)
	}
	// blocked → working → idle quickly: done is cancelled if it flips back before the delay
	obs(Working, "")
	obs(Idle, "")
	if got := obs(Working, ""); len(got) != 0 {
		t.Errorf("flicker notified: %v", got)
	}
	now = now.Add(3 * time.Second)
	if got := obs(Working, ""); len(got) != 0 {
		t.Errorf("cancelled note delivered later: %v", got)
	}
	obs(Idle, "")
	now = now.Add(2 * time.Second)
	if got := obs(Idle, ""); len(got) != 1 || got[0].Kind != NoteDone || got[0].Title != "claude finished" || !strings.Contains(got[0].Body, "fix jwt") {
		t.Errorf("done note: %+v", got)
	}
	cfg.On = map[NoteKind]bool{NoteBlocked: true}
	obs(Working, "")
	obs(Idle, "")
	now = now.Add(2 * time.Second)
	if got := obs(Idle, ""); len(got) != 0 {
		t.Errorf("done is turned off but notified: %v", got)
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
	if s := FormatStatus(Status{Blocked: 1, Working: 2, Done: 1, Idle: 4}, false); s != "⣿1 ⣾2 ●1" {
		t.Errorf("plain status %q", s)
	}
	if s := FormatStatus(Status{Working: 2}, true); s != "#[fg=#f9e2af]⣾2#[fg=default]" {
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
