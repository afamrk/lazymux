package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/afamrk/lazymux/internal/tmux"
)

// captured from Claude Code 2.1.319 asking with AskUserQuestion
const claudeQuestion = `❯ can you show me input option with one, two , three
────────────────────────────────────────────────────────────────
 ☐ Pick one
Which option do you pick: One, Two, or Three?
❯ 1. One
     The first option.
  2. Two
     The second option.
  3. Three
     The third option.
  4. Type something.
────────────────────────────────────────────────────────────────
  5. Chat about this
Enter to select · ↑/↓ to navigate · Esc to cancel
`

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
		title   string
		want    State
		waiting string
		matched bool
	}{
		{"claude idle", Claude, claudeIdle, "", Idle, "", false},
		{"claude working", Claude, claudeWorking, "", Working, "", true},
		{"claude blocked", Claude, claudeBlocked, "", Blocked, "Bash command: go test ./internal/auth/...", true},
		{"claude nbsp prompt", Claude, "❯ 1. Yes\n", "", Blocked, "", true},
		{"claude spinning title", Claude, claudeIdle, "⠂ Fix JWT expiry", Working, "", true},
		{"claude half-circle title", Claude, claudeIdle, "◐ Fix JWT expiry", Working, "", true},
		{"claude idle title", Claude, claudeIdle, "✳ Fix JWT expiry", Idle, "", true},
		{"blocked beats a spinning title", Claude, claudeBlocked, "⠂ Fix JWT expiry", Blocked, "Bash command: go test ./internal/auth/...", true},
		{"claude question form", Claude, claudeQuestion, "✳ Input option", Blocked, "Which option do you pick: One, Two, or Three?", true},
		{"a menu's esc to cancel is not working", Claude, "Select model\n❯ Opus\n  Sonnet\nEnter to select · Esc to cancel\n", "", Idle, "", false},
		{"claude /usage dialog is idle", Claude, "   Settings  Status   Config   Usage   Stats\n   Current week (all models)\n   ███▌    39% used\n   Resets Nov 9, 5:30pm\n   d to day · w to week\n   Esc to cancel\n", "✳ Claude Code", Idle, "", true},
		{"codex working", Codex, "• Explaining\n\n• Working (12s • esc to interrupt)\n▌ \n", "", Working, "", true},
		{"codex blocked", Codex, "Would you like to run the following command?\n\n  $ rm -rf build\n\n› 1. Yes, proceed (y)\n  2. Yes, and don't ask again (a)\n  3. No (esc)\n", "", Blocked, "$ rm -rf build", true},
		{"gemini working", Gemini, "⠏ Thinking about it (esc to cancel, 4s)\n> \n", "", Working, "", true},
		{"gemini blocked", Gemini, "│ Allow execution of: 'ls'?\n│ ● 1. Yes, allow once\n│   2. Yes, allow always\n", "", Blocked, "", true},
		{"opencode permission panel", Opencode, "△ Permission required\n  $ ls\n  Allow once   Allow always   Reject\n", "", Blocked, "", true},
		{"aider asking", Aider, "Add file to the chat? (Y)es/(N)o/(D)on't ask again [Yes]: ", "", Blocked, "Add file to the chat? (Y)es/(N)o/(D)on't ask again [Yes]:", true},
		{"aider prompt", Aider, "Tokens: 2k sent\n\narchitect> ", "", Idle, "", true},
		{"aider output", Aider, "Applied edit to main.go\nCommit 1a2b3c", "", Idle, "", false},
		{"old prompt high above is ignored", Claude, "❯ 1. Yes\n" + strings.Repeat("line\n", 20), "", Idle, "", false},
	} {
		d := Classify(tc.kind, tc.screen, tc.title)
		if d.State != tc.want || d.Waiting != tc.waiting || d.Matched != tc.matched {
			t.Errorf("%s: got (%v, %q, %v) by %q, want (%v, %q, %v)", tc.name, d.State, d.Waiting, d.Matched, d.Rule, tc.want, tc.waiting, tc.matched)
		}
	}
}

// A herdr manifest (its format, nested any / not gates, its regions) works
// as an override, and an edit to the file is picked up.
func TestRuleOverrides(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LAZYMUX_RULES", dir)
	herdr := `id = "codex"
version = "2026.10.01.1"
min_engine_version = 2
updated_at = "2026-10-01T00:00:00Z"
aliases = ["openai-codex"]

[[rules]]
id = "working"
state = "working"
priority = 900
region = "bottom_non_empty_lines(3)"
visible_working = true
any = [{ contains = ["thinking"] }, { line_regex = ['^\s*•\s+Working'] }]
not = [{ contains = ["esc to cancel"] }]

[[rules]]
id = "approval"
state = "blocked"
priority = 950
region = "after_last_horizontal_rule"
contains = ["allow command?"]
`
	write := func(s string) {
		if err := os.WriteFile(filepath.Join(dir, "codex.toml"), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(herdr)
	if m, warn := ManifestFor(Codex); warn != "" || m.Source == "bundled" || len(m.Rules) != 2 {
		t.Fatalf("override not used: %v %q", m.Source, warn)
	}
	for _, tc := range []struct {
		screen string
		want   State
		rule   string
	}{
		{"• Working (3s)\n", Working, "working"},
		{"Thinking… esc to cancel\n", Idle, ""},
		{"Allow command?\n────\nnothing here\n", Idle, ""},
		{"old\n────\nAllow command? rm\n• Working\n", Blocked, "approval"},
	} {
		if d := Classify(Codex, tc.screen, ""); d.State != tc.want || d.Rule != tc.rule {
			t.Errorf("%q: got %v by %q, want %v by %q", tc.screen, d.State, d.Rule, tc.want, tc.rule)
		}
	}

	// a broken override is ignored with a warning, the bundled rules stay
	write(herdr + "\n[[rules]]\nid = \"bad\"\nstate = \"working\"\nregex = ['(']\n")
	time.Sleep(10 * time.Millisecond)
	if m, warn := ManifestFor(Codex); m.Source != "bundled" || !strings.Contains(warn, "bad") {
		t.Errorf("broken override: source %v, warn %q", m.Source, warn)
	}
	// unknown keys are an error, like in herdr
	if _, err := ParseManifest([]byte("id = \"x\"\ncolour = 1\n")); err == nil {
		t.Error("unknown key accepted")
	}
	// agy finds herdr's file name for it
	if err := os.WriteFile(filepath.Join(dir, "antigravity.toml"), []byte("id = \"agy\"\naliases = [\"antigravity\"]\n[[rules]]\nid = \"w\"\nstate = \"working\"\ncontains = [\"busy\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if d := Classify(Agy, "busy\n", ""); d.State != Working {
		t.Errorf("agy override (antigravity.toml) not used: %+v", d)
	}
}

func TestRegions(t *testing.T) {
	screen := "a\n\nb\n────────\n❯ typed\n────────\nfooter\n\n"
	in := Input{Screen: screen, Title: "⠂ x"}
	for spec, want := range map[string]string{
		"osc_title":                       "⠂ x",
		"bottom_non_empty_lines(1)":       "footer\n\n",
		"bottom_lines(2)":                 "\n",
		"top_non_empty_lines(2)":          "a\n\nb",
		"after_last_horizontal_rule":      "footer\n\n",
		"prompt_box_body":                 "❯ typed",
		"last_non_empty_above_prompt_box": "b",
	} {
		if got := region(in, spec); got != want {
			t.Errorf("%s = %q, want %q", spec, got, want)
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
		if ev, ok := eventFromHook("", []byte(raw), now); ok {
			if err := appendEvent(dir, "default", "%12", ev); err != nil {
				t.Fatal(err)
			}
		}
	}
	evs, err := readEvents(eventFile(dir, "default", "%12"))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 5 {
		t.Fatalf("got %d events, want 5: %+v", len(evs), evs)
	}
	if evs[1].Detail != "fix jwt expiry validation" || evs[2].Detail != "internal/auth/jwt.go" || evs[3].Name != "Bash" {
		t.Errorf("events: %+v", evs)
	}
	// a new session clears the log
	ev, _ := eventFromHook("", []byte(`{"hook_event_name":"SessionStart","source":"clear"}`), now)
	appendEvent(dir, "default", "%12", ev)
	if evs, _ := readEvents(eventFile(dir, "default", "%12")); len(evs) != 1 {
		t.Errorf("after start: %d events", len(evs))
	}
}

func TestHookPayloadsPerAgent(t *testing.T) {
	now := time.Unix(1000, 0)
	for _, tc := range []struct {
		name, event, raw string
		want             Event
	}{
		{"codex permission", "PermissionRequest", `{"hook_event_name":"PermissionRequest","tool_name":"Bash","tool_input":{"command":"rm -rf build"}}`,
			Event{Kind: "notify", Detail: "needs permission: Bash"}},
		{"agy prompt", "PreInvocation", `{"conversationId":"c1","lastUserInput":"add table tests","invocationNum":1}`,
			Event{Kind: "prompt", Detail: "add table tests"}},
		{"agy tool", "PostToolUse", `{"workspacePaths":["/src/ui"],"toolCall":{"name":"run_command","args":{"CommandLine":"go test ./..."}},"stepIdx":3}`,
			Event{Kind: "tool", Name: "run_command", Detail: "go test ./..."}},
		{"agy file tool", "PostToolUse", `{"workspacePaths":["/src/ui"],"toolCall":{"name":"view_file","args":{"AbsolutePath":"/src/ui/app.go"}}}`,
			Event{Kind: "tool", Name: "view_file", Detail: "app.go"}},
		{"agy stop", "Stop", `{"terminationReason":"NO_TOOL_CALL","fullyIdle":true}`, Event{Kind: "stop"}},
		{"opencode tool", "tool", `{"tool_name":"edit","tool_input":{"filePath":"/x/main.go"}}`, Event{Kind: "tool", Name: "edit", Detail: "/x/main.go"}},
		{"opencode permission", "permission", `{"title":"Run bash: ls"}`, Event{Kind: "notify", Detail: "needs permission: Run bash: ls"}},
	} {
		got, ok := eventFromHook(tc.event, []byte(tc.raw), now)
		got.T = time.Time{}
		if !ok || got != tc.want {
			t.Errorf("%s: got %+v (ok=%v), want %+v", tc.name, got, ok, tc.want)
		}
	}
	if _, ok := eventFromHook("PreInvocation", []byte(`{"lastUserInput":""}`), now); ok {
		t.Error("a PreInvocation without user input is not a prompt")
	}
	// agy repeats PreInvocation for every model call of a turn
	evs := []Event{{Kind: "prompt", Detail: "a"}, {Kind: "tool", Name: "x"}}
	if !skipDuplicate(evs, Event{Kind: "prompt", Detail: "a"}) || skipDuplicate(append(evs, Event{Kind: "stop"}), Event{Kind: "prompt", Detail: "a"}) {
		t.Error("duplicate prompt handling")
	}
}

func TestHookReplies(t *testing.T) {
	for _, tc := range []struct{ agent, event, want string }{
		{"agy", "PostToolUse", "{}\n"}, {"agy", "Stop", "{\"decision\":\"allow\"}\n"},
		{"codex", "Stop", "{}\n"}, {"codex", "PreToolUse", ""}, {"claude", "Stop", ""},
	} {
		var b strings.Builder
		t.Setenv("TMUX_PANE", "") // not in tmux: records nothing, still replies
		RunHook([]string{tc.agent, tc.event}, strings.NewReader("{}"), &b)
		if b.String() != tc.want {
			t.Errorf("%s %s replied %q, want %q", tc.agent, tc.event, b.String(), tc.want)
		}
	}
}

func TestIntegrations(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, v := range []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME", "ANTIGRAVITY_CLI_CONFIG_DIR", "OPENCODE_CONFIG_DIR", "XDG_CONFIG_HOME"} {
		t.Setenv(v, "")
	}
	mk := func(rel, content string) string {
		p := filepath.Join(home, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if content != "" {
			os.WriteFile(p, []byte(content), 0o640)
		}
		return p
	}
	claude := mk(".claude/settings.json", `{"model":"opus","hooks":{"Stop":[{"hooks":[{"type":"command","command":"notify-send done"}]}]}}`)
	codexHooks := mk(".codex/hooks.json", `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"bash '/h/.codex/herdr-agent-state.sh' session"}]}]}}`)
	codexConfig := mk(".codex/config.toml", "model = \"o3\"\n\n[tui]\nx = 1\n")
	agyHooks := mk(".gemini/config/hooks.json", `{"herdr":{"PreInvocation":[{"type":"command","command":"bash x session"}]}}`)
	mk(".config/opencode/opencode.jsonc", "{}")

	exe := "/usr/local/bin/lazymux"
	for _, i := range Integrations() {
		if !i.Detected() {
			t.Fatalf("%s not detected", i.Kind)
		}
		if changed, err := i.Install(exe); err != nil || !changed {
			t.Fatalf("%s install: %v %v", i.Kind, changed, err)
		}
		if !i.Installed() {
			t.Errorf("%s not installed after install", i.Kind)
		}
		if changed, _ := i.Install(exe); changed {
			t.Errorf("%s: second install changed files", i.Kind)
		}
	}
	read := func(p string) string { b, _ := os.ReadFile(p); return string(b) }
	if c := read(claude); !strings.Contains(c, "notify-send done") || !strings.Contains(c, "hook claude PreToolUse") || !strings.Contains(c, `"model": "opus"`) {
		t.Errorf("claude settings:\n%s", c)
	}
	if st, _ := os.Stat(claude); st.Mode().Perm() != 0o640 {
		t.Errorf("claude settings mode %v", st.Mode().Perm())
	}
	if c := read(codexHooks); !strings.Contains(c, "herdr-agent-state.sh") || !strings.Contains(c, "hook codex PermissionRequest") {
		t.Errorf("codex hooks:\n%s", c)
	}
	if c := read(codexConfig); !strings.Contains(c, "[features]\nhooks = true") || !strings.Contains(c, "model = \"o3\"") {
		t.Errorf("codex config:\n%s", c)
	}
	if c := read(agyHooks); !strings.Contains(c, `"herdr"`) || !strings.Contains(c, "hook agy PostToolUse") || strings.Contains(c, "hook agy PreToolUse") {
		t.Errorf("agy hooks:\n%s", c)
	}
	if c := read(filepath.Join(home, ".config/opencode/plugins/lazymux.js")); !strings.Contains(c, `"/usr/local/bin/lazymux"`) || !strings.Contains(c, opencodeMarker) {
		t.Errorf("opencode plugin:\n%s", c)
	}

	for _, i := range Integrations() {
		if changed, err := i.Uninstall(); err != nil || !changed {
			t.Errorf("%s uninstall: %v %v", i.Kind, changed, err)
		}
		if i.Installed() {
			t.Errorf("%s still installed", i.Kind)
		}
	}
	if c := read(claude); !strings.Contains(c, "notify-send done") || strings.Contains(c, "lazymux") {
		t.Errorf("claude after uninstall:\n%s", c)
	}
	if c := read(codexHooks); !strings.Contains(c, "herdr-agent-state.sh") || strings.Contains(c, "lazymux") {
		t.Errorf("codex after uninstall:\n%s", c)
	}
	if c := read(agyHooks); !strings.Contains(c, `"herdr"`) || strings.Contains(c, "lazymux") {
		t.Errorf("agy after uninstall:\n%s", c)
	}
}

func TestEnableCodexHooks(t *testing.T) {
	for in, want := range map[string]string{
		"":                                 "\n[features]\nhooks = true\n",
		"[features]\nother = 1\n":          "[features]\nhooks = true\nother = 1\n",
		"[features]\nhooks = false\n[x]\n": "[features]\nhooks = true\n[x]\n",
		"[features]\nhooks = true\n":       "[features]\nhooks = true\n",
		"a = 1":                            "a = 1\n\n[features]\nhooks = true\n",
	} {
		if got := enableCodexHooks(in); got != want {
			t.Errorf("enableCodexHooks(%q) = %q, want %q", in, got, want)
		}
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
	// looking at it (a client attaches) makes the done agent idle
	seenTree, _ := tmux.ParseTree(strings.ReplaceAll(row("%1", "claude", "✳ Fix login bug"), "\twork\t0\t", "\twork\t1\t"))
	if a := d.Detect(seenTree)["%1"]; a.Unseen || a.Label() != "idle" {
		t.Errorf("claude after being seen: %+v", a)
	}
	d.Detect(tree)

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

// row is a list-panes line in tmux.ParseTree's format, in a session no
// client is attached to (so nothing is visible).
func row(id, cmd, title string) string {
	f := []string{"$0", "work", "0", "@" + id[1:], id[1:], "w" + id[1:], "1", "lay",
		id, "0", "1", cmd, "/src", "100", "80", "24", "0", "0", "", title}
	return strings.Join(f, "\t")
}

func TestGoRegex(t *testing.T) {
	for in, want := range map[string]string{
		`^[\u2800-\u28FF]`:      `^[\x{2800}-\x{28FF}]`,
		`^⚠[\u{fe0e}\u{fe0f}]?`: `^⚠[\x{fe0e}\x{fe0f}]?`,
		`\p{Alphabetic}+ing`:    `\p{L}+ing`,
		`a\\u2800 \d\s`:         `a\\u2800 \d\s`, // an escaped backslash stays
	} {
		if got := goRegex(in); got != want {
			t.Errorf("goRegex(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBundledRules(t *testing.T) {
	for _, k := range Kinds {
		m := bundledManifest(k)
		if len(m.Rules) == 0 || !m.matches(k) {
			t.Errorf("%s: no bundled rules", k)
		}
		if BundledRules(k) == "" {
			t.Errorf("%s: no bundled file", k)
		}
	}
}
