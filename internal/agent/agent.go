// Package agent finds AI coding agents (Claude Code, Codex, Gemini CLI,
// opencode, aider) running in tmux panes and tells whether each one is
// working, blocked on a question, or idle.
//
// Detection works without setup by scraping: the pane's process tells which
// agent runs there, and its screen tells the state. Claude Code can also
// report events through hooks (`lazymux hooks install`), which adds the
// prompt, the waiting reason and an activity timeline.
package agent

import (
	"fmt"
	"time"
)

// Kind is the agent program.
type Kind string

const (
	Claude   Kind = "claude"
	Codex    Kind = "codex"
	Gemini   Kind = "gemini"
	Opencode Kind = "opencode"
	Aider    Kind = "aider"
)

// Kinds lists the supported agents.
var Kinds = []Kind{Claude, Codex, Gemini, Opencode, Aider}

// State is what an agent is doing.
type State int

const (
	Idle    State = iota // waiting for a new prompt (done, or never started)
	Working              // running a turn
	Blocked              // waiting for an answer (permission prompt, question)
)

func (s State) String() string {
	switch s {
	case Working:
		return "working"
	case Blocked:
		return "blocked"
	}
	return "idle"
}

// Agent is one agent found in a pane.
type Agent struct {
	Pane    string // tmux pane id
	Kind    Kind
	State   State
	Since   time.Time // when State last changed (or when the agent was found)
	Worked  bool      // has been seen working: an idle agent is "done", not just "idle"
	Summary string    // what it works on: last prompt (hooks) or the pane title
	Waiting string    // blocked: what it waits for, e.g. "Bash(go test ./...)"
	Hooked  bool      // hook events exist for this pane
	Events  []Event   // activity timeline from hooks, oldest first
}

// Label is the state word shown in lists: "done" for an idle agent that
// worked before, "idle" for one that has not.
func (a *Agent) Label() string {
	if a.State == Idle && a.Worked {
		return "done"
	}
	return a.State.String()
}

// Age formats how long ago t was, compactly: "40s", "3m", "2h", "5d".
func Age(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", max(0, int(d.Seconds())))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// Interrupt is the tmux key that stops an agent's current turn.
func Interrupt(k Kind) string {
	if k == Aider {
		return "C-c"
	}
	return "Escape"
}

// AnswerKeys are the tmux keys that pick option n (1 yes, 2 always / yes for
// the session, 3 no) of a permission prompt.
func AnswerKeys(k Kind, n int) []string {
	switch k {
	case Codex:
		return [][]string{{"y"}, {"a"}, {"Escape"}}[n-1]
	case Gemini:
		return [][]string{{"1"}, {"2"}, {"Escape"}}[n-1]
	case Opencode:
		return [][]string{{"Enter"}, {"a"}, {"Escape"}}[n-1]
	case Aider:
		return [][]string{{"y", "Enter"}, {"a", "Enter"}, {"n", "Enter"}}[n-1]
	}
	return []string{fmt.Sprint(n)} // Claude Code: the digit picks the option
}
