package agent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Claude Code hooks run `lazymux hook` with the event as JSON on stdin. The
// hook appends one line per event to a log file per tmux pane, which the
// detector reads. The pane comes from $TMUX_PANE (the agent runs inside it)
// and the server from $TMUX, so agents on different servers do not mix.

// Event is one entry of an agent's activity timeline.
type Event struct {
	T      time.Time `json:"t"`
	Kind   string    `json:"k"`           // start, prompt, tool, notify, stop, end
	Name   string    `json:"n,omitempty"` // tool name
	Detail string    `json:"d,omitempty"` // prompt text, command, file, message
}

// maxEvents is how many events are kept per pane.
const maxEvents = 200

// EventDir is where hook events are stored.
func EventDir() string {
	if d := os.Getenv("LAZYMUX_EVENTS"); d != "" {
		return d
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "lazymux", "events")
}

// eventFile is the log of one pane on one tmux server.
func eventFile(dir, server, pane string) string {
	clean := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r == '/' || r == '\\' || r == '%' || r == '.' {
				return '_'
			}
			return r
		}, s)
	}
	return filepath.Join(dir, clean(server)+"-"+clean(pane)+".jsonl")
}

// hookInput is the part of Claude Code's hook JSON lazymux uses.
type hookInput struct {
	Event     string          `json:"hook_event_name"`
	Cwd       string          `json:"cwd"`
	Prompt    string          `json:"prompt"`
	Tool      string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
	Message   string          `json:"message"`
	Source    string          `json:"source"`
}

// eventFromHook converts hook JSON to an Event; ok is false for events
// lazymux does not record.
func eventFromHook(raw []byte, now time.Time) (Event, bool) {
	var in hookInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return Event{}, false
	}
	ev := Event{T: now}
	switch in.Event {
	case "SessionStart":
		ev.Kind, ev.Detail = "start", in.Source
	case "UserPromptSubmit":
		ev.Kind, ev.Detail = "prompt", oneLine(in.Prompt, 300)
	case "PreToolUse":
		ev.Kind, ev.Name, ev.Detail = "tool", in.Tool, toolDetail(in.Tool, in.ToolInput, in.Cwd)
	case "Notification":
		ev.Kind, ev.Detail = "notify", oneLine(in.Message, 200)
	case "Stop":
		ev.Kind = "stop"
	case "SessionEnd":
		ev.Kind = "end"
	default:
		return Event{}, false
	}
	return ev, true
}

// toolDetail picks the interesting argument of a tool call.
func toolDetail(tool string, input json.RawMessage, cwd string) string {
	var m map[string]any
	_ = json.Unmarshal(input, &m)
	str := func(k string) string { s, _ := m[k].(string); return s }
	rel := func(p string) string {
		if cwd != "" {
			if r, err := filepath.Rel(cwd, p); err == nil && !strings.HasPrefix(r, "..") {
				return r
			}
		}
		return p
	}
	var d string
	switch {
	case str("command") != "":
		d = str("command")
	case str("file_path") != "":
		d = rel(str("file_path"))
	case str("notebook_path") != "":
		d = rel(str("notebook_path"))
	case str("pattern") != "":
		d = str("pattern")
	case str("url") != "":
		d = str("url")
	case str("query") != "":
		d = str("query")
	case str("description") != "":
		d = str("description")
	case str("prompt") != "":
		d = str("prompt")
	}
	return oneLine(d, 200)
}

// oneLine collapses whitespace and cuts s to n runes.
func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// RunHook handles `lazymux hook`: it records the event on stdin for the pane
// in $TMUX_PANE. It never fails loudly, so it cannot disturb the agent.
func RunHook(stdin io.Reader) {
	pane, tmuxEnv := os.Getenv("TMUX_PANE"), os.Getenv("TMUX")
	if pane == "" || tmuxEnv == "" {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(stdin, 1<<20))
	if err != nil {
		return
	}
	ev, ok := eventFromHook(raw, time.Now())
	if !ok {
		return
	}
	server, _, _ := strings.Cut(tmuxEnv, ",")
	_ = appendEvent(EventDir(), filepath.Base(server), pane, ev)
}

// appendEvent adds ev to the pane's log. A new session (start) clears it,
// and the log is cut back to maxEvents when it grows too long.
func appendEvent(dir, server, pane string, ev Event) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := eventFile(dir, server, pane)
	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if ev.Kind == "start" {
		return os.WriteFile(path, line, 0o600)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(line)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if st, serr := os.Stat(path); serr == nil && st.Size() > 4*maxEvents*200 {
		evs, _ := readEvents(path)
		var b bytes.Buffer
		for _, e := range evs {
			l, _ := json.Marshal(e)
			b.Write(append(l, '\n'))
		}
		_ = os.WriteFile(path, b.Bytes(), 0o600)
	}
	return err
}

// readEvents reads a pane's log, keeping the last maxEvents entries.
func readEvents(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var evs []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		var e Event
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.Kind != "" {
			evs = append(evs, e)
		}
	}
	if len(evs) > maxEvents {
		evs = evs[len(evs)-maxEvents:]
	}
	return evs, sc.Err()
}

// --- install into Claude Code settings ---------------------------------------

// hookEvents are the Claude Code events lazymux listens to.
var hookEvents = []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "Notification", "Stop", "SessionEnd"}

// ClaudeSettings is the default Claude Code user settings file.
func ClaudeSettings() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, "settings.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "settings.json")
}

// isOurs reports whether a hook command is lazymux's.
func isOurs(cmd string) bool {
	return strings.Contains(cmd, "lazymux") && strings.HasSuffix(strings.TrimSpace(cmd), " hook")
}

// InstallHooks adds `<command>` (e.g. "'/usr/bin/lazymux' hook") as a hook
// for each event in the settings file, replacing an older lazymux entry.
// The previous file is kept as <file>.lazymux.bak. Returns whether anything
// changed.
func InstallHooks(path, command string) (bool, error) {
	return editHooks(path, func(hooks map[string]any) {
		for _, ev := range hookEvents {
			groups := removeOurs(hooks[ev])
			group := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command, "timeout": 5}}}
			if ev == "PreToolUse" {
				group["matcher"] = "*"
			}
			hooks[ev] = append(groups, group)
		}
	})
}

// UninstallHooks removes lazymux's hooks from the settings file.
func UninstallHooks(path string) (bool, error) {
	return editHooks(path, func(hooks map[string]any) {
		for ev, v := range hooks {
			groups := removeOurs(v)
			if len(groups) == 0 {
				delete(hooks, ev)
			} else {
				hooks[ev] = groups
			}
		}
	})
}

// HooksInstalled reports whether the settings file has lazymux hooks.
func HooksInstalled(path string) bool {
	settings, err := readSettings(path)
	if err != nil {
		return false
	}
	hooks, _ := settings["hooks"].(map[string]any)
	for _, v := range hooks {
		if len(removeOurs(v)) != countGroups(v) {
			return true
		}
	}
	return false
}

func countGroups(v any) int { l, _ := v.([]any); return len(l) }

// removeOurs drops lazymux commands from an event's hook groups, and groups
// that end up empty.
func removeOurs(v any) []any {
	groups, _ := v.([]any)
	var out []any
	for _, g := range groups {
		gm, ok := g.(map[string]any)
		if !ok {
			out = append(out, g)
			continue
		}
		list, _ := gm["hooks"].([]any)
		var keep []any
		for _, h := range list {
			if hm, ok := h.(map[string]any); ok {
				if c, _ := hm["command"].(string); isOurs(c) {
					continue
				}
			}
			keep = append(keep, h)
		}
		if len(keep) == 0 && len(list) > 0 {
			continue
		}
		gm["hooks"] = keep
		out = append(out, gm)
	}
	return out
}

func readSettings(path string) (map[string]any, error) {
	settings := map[string]any{}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return settings, nil
	}
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return settings, nil
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return settings, nil
}

func editHooks(path string, edit func(hooks map[string]any)) (bool, error) {
	settings, err := readSettings(path)
	if err != nil {
		return false, err
	}
	before, _ := json.Marshal(settings)
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	edit(hooks)
	if len(hooks) == 0 {
		delete(settings, "hooks")
	} else {
		settings["hooks"] = hooks
	}
	after, _ := json.Marshal(settings)
	if bytes.Equal(before, after) {
		return false, nil
	}
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	if old, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".lazymux.bak", old, mode); err != nil {
			return false, err
		}
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, append(out, '\n'), mode)
}
