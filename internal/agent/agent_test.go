package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/afamrk/lazymux/internal/tmux"
)

const claudeIdle = `  Summary

────────────────────────────────────────────────
❯ yes, sketch how it fits lazymux
────────────────────────────────────────────────
  📁 lazymux 🌿 master | Opus 5.5·medium | 🧠 7% | 📅 34%
  -- INSERT -- ⏵⏵ auto mode on (shift+tab to cycle)`

const claudeWorking = `⏺ Reading the files.

✽ Philosophizing… (1m 29s · ↓ 5.6k tokens)
──────────────────────────────────────────── lazymux ─
❯
────────────────────────────────────────────────
  📁 lazymux 🌿 master | Opus 5.5·medium | 🧠 9%`

const claudeBlocked = `⏺ Bash(go test ./internal/auth/...)

────────────────────────────────────────────────
 Bash command

   go test ./internal/auth/...
   Run the auth tests

 Do you want to proceed?
 ❯ 1. Yes
   2. Yes, and don't ask again for go test commands in /home/u/api
   3. No, and tell Claude what to do differently (esc)
`

func TestClassify(t *testing.T) {
	for _, tc := range []struct {
		name    string
		kind    Kind
		screen  string
		want    State
		waiting string
		ok      bool
	}{
		{"claude idle", Claude, claudeIdle, Idle, "", true},
		{"claude working", Claude, claudeWorking, Working, "", true},
		{"claude blocked", Claude, claudeBlocked, Blocked, "Bash command: go test ./internal/auth/...", true},
		{"claude nbsp prompt", Claude, "❯ 1. Yes\n", Blocked, "", true},
		{"codex working", Codex, "• Explaining\n\n• Working (12s • esc to interrupt)\n▌ \n", Working, "", true},
		{"codex blocked", Codex, "Would you like to run the following command?\n\n  $ rm -rf build\n\n› 1. Yes, proceed (y)\n  2. Yes, and don't ask again (a)\n  3. No (esc)\n", Blocked, "$ rm -rf build", true},
		{"gemini working", Gemini, "⠏ Thinking about it (esc to cancel, 4s)\n> \n", Working, "", true},
		{"gemini blocked", Gemini, "│ Allow execution of: 'ls'?\n│ ● 1. Yes, allow once\n│   2. Yes, allow always\n", Blocked, "", true},
		{"aider asking", Aider, "Add file to the chat? (Y)es/(N)o/(D)on't ask again [Yes]: ", Blocked, "Add file to the chat? (Y)es/(N)o/(D)on't ask again [Yes]:", true},
		{"aider prompt", Aider, "Tokens: 2k sent\n\narchitect> ", Idle, "", true},
		{"aider output", Aider, "Applied edit to main.go\nCommit 1a2b3c", Idle, "", false},
		{"old prompt high above is ignored", Claude, "❯ 1. Yes\n" + strings.Repeat("line\n", 20), Idle, "", true},
	} {
		st, waiting, ok := Classify(tc.kind, tc.screen)
		if st != tc.want || waiting != tc.waiting || ok != tc.ok {
			t.Errorf("%s: got (%v, %q, %v), want (%v, %q, %v)", tc.name, st, waiting, ok, tc.want, tc.waiting, tc.ok)
		}
	}
}

func TestKindDetection(t *testing.T) {
	for cmd, want := range map[string]Kind{"claude": Claude, "codex": Codex, "zsh": "", "nvim": ""} {
		if got := KindOf(cmd); got != want {
			t.Errorf("KindOf(%q) = %q", cmd, got)
		}
	}
	procs := map[int]proc{
		100: {1, "-zsh"},
		200: {100, "node /home/u/.local/share/pnpm/global/5/node_modules/@google/gemini-cli/dist/index.js"},
		300: {1, "-zsh"},
		400: {300, "python3 /home/u/.local/bin/aider --model x"},
		500: {1, "-zsh"},
		600: {500, "node server.js"},
	}
	for pid, want := range map[int]Kind{100: Gemini, 300: Aider, 500: ""} {
		if got := kindUnder(pid, procs); got != want {
			t.Errorf("kindUnder(%d) = %q, want %q", pid, got, want)
		}
	}
	if !isInterpreter("node") || !isInterpreter("python3.12") || !isInterpreter("2.0.31") || isInterpreter("zsh") {
		t.Error("isInterpreter")
	}
}

func TestHookEvents(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1000, 0)
	in := []string{
		`{"hook_event_name":"SessionStart","source":"startup"}`,
		`{"hook_event_name":"UserPromptSubmit","prompt":"fix jwt\n expiry validation"}`,
		`{"hook_event_name":"PreToolUse","cwd":"/src/api","tool_name":"Read","tool_input":{"file_path":"/src/api/internal/auth/jwt.go"}}`,
		`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"go test ./internal/auth/..."}}`,
		`{"hook_event_name":"PostToolUse","tool_name":"Bash"}`,
	}
	for _, raw := range in {
		if ev, ok := eventFromHook([]byte(raw), now); ok {
			if err := appendEvent(dir, "default", "%12", ev); err != nil {
				t.Fatal(err)
			}
		}
	}
	evs, err := readEvents(eventFile(dir, "default", "%12"))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 4 {
		t.Fatalf("got %d events, want 4 (PostToolUse is not recorded): %+v", len(evs), evs)
	}
	if evs[1].Detail != "fix jwt expiry validation" || evs[2].Detail != "internal/auth/jwt.go" || evs[3].Name != "Bash" {
		t.Errorf("events: %+v", evs)
	}
	// a new session clears the log
	ev, _ := eventFromHook([]byte(`{"hook_event_name":"SessionStart","source":"clear"}`), now)
	appendEvent(dir, "default", "%12", ev)
	if evs, _ := readEvents(eventFile(dir, "default", "%12")); len(evs) != 1 {
		t.Errorf("after start: %d events", len(evs))
	}
}

func TestInstallHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	orig := `{"model":"opus","hooks":{"Stop":[{"hooks":[{"type":"command","command":"notify-send done"}]}]}}`
	os.WriteFile(path, []byte(orig), 0o640)

	changed, err := InstallHooks(path, "'/usr/bin/lazymux' hook")
	if err != nil || !changed {
		t.Fatalf("install: %v %v", changed, err)
	}
	if !HooksInstalled(path) {
		t.Fatal("not installed")
	}
	if changed, _ := InstallHooks(path, "'/usr/bin/lazymux' hook"); changed {
		t.Error("second install changed the file")
	}
	var s map[string]any
	data, _ := os.ReadFile(path)
	json.Unmarshal(data, &s)
	stop := s["hooks"].(map[string]any)["Stop"].([]any)
	if len(stop) != 2 || s["model"] != "opus" {
		t.Errorf("user hook or setting lost: %s", data)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o640 {
		t.Errorf("mode %v", st.Mode().Perm())
	}
	if bak, _ := os.ReadFile(path + ".lazymux.bak"); string(bak) != orig {
		t.Error("no backup")
	}

	if changed, err := UninstallHooks(path); err != nil || !changed {
		t.Fatalf("uninstall: %v %v", changed, err)
	}
	data, _ = os.ReadFile(path)
	if HooksInstalled(path) || !strings.Contains(string(data), "notify-send done") || strings.Contains(string(data), "PreToolUse") {
		t.Errorf("after uninstall: %s", data)
	}
}

func TestDetector(t *testing.T) {
	tree, err := tmux.ParseTree(strings.Join([]string{
		row("%1", "claude", "✳ Fix login bug"),
		row("%2", "zsh", "host"),
		row("%3", "aider", "host"),
	}, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	screens := map[string]string{"%1": claudeWorking, "%3": "Applied edit"}
	now := time.Unix(1000, 0)
	d := &Detector{
		server: "default", dir: t.TempDir(), now: func() time.Time { return now },
		capture: func(p string) (string, error) { return screens[p], nil },
		procs:   func() map[int]proc { return nil },
		known:   map[string]*track{},
	}
	got := d.Detect(tree)
	if len(got) != 2 || got["%1"].State != Working || got["%1"].Summary != "Fix login bug" || got["%3"].State != Idle {
		t.Fatalf("first: %+v %+v", got["%1"], got["%3"])
	}
	// claude finishes; aider's screen keeps changing
	now = now.Add(5 * time.Second)
	screens["%1"], screens["%3"] = claudeIdle, "Applied edit\nmore output"
	got = d.Detect(tree)
	if a := got["%1"]; a.State != Idle || a.Label() != "done" || !a.Since.Equal(now) {
		t.Errorf("claude after work: %+v", a)
	}
	if got["%3"].State != Working {
		t.Errorf("aider with changing screen: %+v", got["%3"])
	}
	now = now.Add(5 * time.Second)
	if got = d.Detect(tree); got["%3"].State != Idle {
		t.Errorf("aider with a still screen: %+v", got["%3"])
	}

	// hooks: the prompt becomes the summary, the pending tool the waiting reason
	for _, e := range []Event{{T: now, Kind: "prompt", Detail: "fix jwt expiry"}, {T: now, Kind: "tool", Name: "Bash", Detail: "go test ./..."}} {
		appendEvent(d.dir, "default", "%1", e)
	}
	screens["%1"] = claudeBlocked
	got = d.Detect(tree)
	if a := got["%1"]; a.State != Blocked || a.Summary != "fix jwt expiry" || a.Waiting != "Bash(go test ./...)" || !a.Hooked || len(a.Events) != 2 {
		t.Errorf("hooked claude: %+v", a)
	}
}

// row is a list-panes line in tmux.ParseTree's format.
func row(id, cmd, title string) string {
	f := []string{"$0", "work", "1", "@" + id[1:], id[1:], "w" + id[1:], "1", "lay",
		id, "0", "1", cmd, "/src", "100", "80", "24", "0", "0", "", title}
	return strings.Join(f, "\t")
}
