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

// Hook payloads differ per agent: Claude Code and Codex send snake_case
// JSON with hook_event_name, agy sends camelCase protojson (toolCall,
// lastUserInput), and lazymux's own opencode plugin and aider command pass
// the event name as an argument. eventFromHook looks fields up by every name
// they use.

// eventFromHook converts a hook payload to an Event. name is the event name
// from the command line ("" means: read hook_event_name from the payload).
// ok is false for events lazymux does not record.
func eventFromHook(name string, raw []byte, now time.Time) (Event, bool) {
	var in map[string]any
	if len(strings.TrimSpace(string(raw))) > 0 {
		if err := json.Unmarshal(raw, &in); err != nil {
			return Event{}, false
		}
	}
	if name == "" {
		name = str(in, "hook_event_name", "hookEventName")
	}
	cwd := str(in, "cwd")
	if cwd == "" {
		if ws, ok := in["workspacePaths"].([]any); ok && len(ws) > 0 {
			cwd, _ = ws[0].(string)
		}
	}
	ev := Event{T: now}
	switch name {
	case "SessionStart", "start":
		ev.Kind, ev.Detail = "start", str(in, "source")
	case "UserPromptSubmit", "PreInvocation", "prompt":
		ev.Kind, ev.Detail = "prompt", oneLine(str(in, "prompt", "lastUserInput", "userPrompt", "text"), 300)
		if ev.Detail == "" {
			return Event{}, false
		}
	case "PreToolUse", "PostToolUse", "tool":
		call, _ := in["toolCall"].(map[string]any)
		ev.Kind = "tool"
		ev.Name = str(in, "tool_name", "toolName", "tool")
		if ev.Name == "" {
			ev.Name = str(call, "name")
		}
		input := in["tool_input"]
		for _, k := range []string{"toolInput", "args"} {
			if input == nil {
				input = in[k]
			}
		}
		if input == nil && call != nil {
			input = call["args"]
		}
		ev.Detail = toolDetail(input, cwd)
	case "Notification", "notify":
		ev.Kind, ev.Detail = "notify", oneLine(str(in, "message"), 200)
	case "PermissionRequest", "permission":
		ev.Kind = "notify"
		ev.Detail = "needs permission"
		if t := str(in, "tool_name", "toolName", "title"); t != "" {
			ev.Detail += ": " + oneLine(t, 100)
		}
	case "Stop", "stop":
		ev.Kind = "stop"
	case "SessionEnd", "end":
		ev.Kind = "end"
	default:
		return Event{}, false
	}
	return ev, true
}

// str returns the first non-empty string field of m among keys.
func str(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// toolDetail picks the interesting argument of a tool call. Argument names
// differ per agent (command / CommandLine, file_path / AbsolutePath…), so
// keys are compared case-insensitively.
func toolDetail(input any, cwd string) string {
	m, ok := input.(map[string]any)
	if !ok {
		if s, isStr := input.(string); isStr { // e.g. argumentsJson
			_ = json.Unmarshal([]byte(s), &m)
		}
	}
	lower := map[string]string{}
	for k, v := range m {
		if s, ok := v.(string); ok && s != "" {
			lower[strings.ToLower(k)] = s
		}
	}
	rel := func(p string) string {
		if cwd != "" {
			if r, err := filepath.Rel(cwd, p); err == nil && !strings.HasPrefix(r, "..") {
				return r
			}
		}
		return p
	}
	for _, k := range []string{"command", "commandline", "cmd"} {
		if v := lower[k]; v != "" {
			return oneLine(v, 200)
		}
	}
	for _, k := range []string{"file_path", "filepath", "absolutepath", "targetfile", "notebook_path", "path"} {
		if v := lower[k]; v != "" {
			return oneLine(rel(v), 200)
		}
	}
	for _, k := range []string{"pattern", "query", "url", "description", "prompt"} {
		if v := lower[k]; v != "" {
			return oneLine(v, 200)
		}
	}
	return ""
}

// oneLine collapses whitespace and cuts s to n runes.
func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// RunHook handles `lazymux hook [agent] [event]`: it records the event on
// stdin for the pane in $TMUX_PANE (the agent runs inside it). Without an
// agent it is Claude Code (hooks installed by older versions). It never
// fails loudly, so it cannot disturb the agent, and it prints the empty
// answer the agent expects where one is required.
func RunHook(args []string, stdin io.Reader, stdout io.Writer) {
	agent, event := "claude", ""
	if len(args) > 0 {
		agent = args[0]
	}
	if len(args) > 1 {
		event = args[1]
	}
	defer hookReply(agent, event, stdout)

	pane, tmuxEnv := os.Getenv("TMUX_PANE"), os.Getenv("TMUX")
	raw, err := io.ReadAll(io.LimitReader(stdin, 1<<20))
	if err != nil || pane == "" || tmuxEnv == "" {
		return
	}
	ev, ok := eventFromHook(event, raw, time.Now())
	if !ok {
		return
	}
	server, _, _ := strings.Cut(tmuxEnv, ",")
	dir, srv := EventDir(), filepath.Base(server)
	if evs, _ := readEvents(eventFile(dir, srv, pane)); skipDuplicate(evs, ev) {
		return
	}
	_ = appendEvent(dir, srv, pane, ev)
}

// skipDuplicate drops repeats: agy sends PreInvocation (with the same user
// input) before every model call of a turn, opencode reports idle twice.
func skipDuplicate(evs []Event, ev Event) bool {
	if len(evs) == 0 {
		return false
	}
	last := evs[len(evs)-1]
	switch ev.Kind {
	case "stop":
		return last.Kind == "stop"
	case "prompt":
		for i := len(evs) - 1; i >= 0; i-- {
			switch evs[i].Kind {
			case "prompt":
				return evs[i].Detail == ev.Detail
			case "stop", "start":
				return false
			}
		}
	}
	return false
}

// hookReply prints what an agent requires on stdout: agy wants a JSON object
// from every hook (Stop must not block), Codex wants JSON from Stop.
func hookReply(agent, event string, stdout io.Writer) {
	switch {
	case agent == "agy" && event == "Stop":
		fmt.Fprintln(stdout, `{"decision":"allow"}`)
	case agent == "agy", agent == "codex" && event == "Stop":
		fmt.Fprintln(stdout, "{}")
	}
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

// --- editing agents' config files ----------------------------------------------------

// isOurs reports whether a hook command is lazymux's.
func isOurs(cmd string) bool {
	return strings.Contains(cmd, "lazymux") && (strings.HasSuffix(strings.TrimSpace(cmd), " hook") || strings.Contains(cmd, " hook "))
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
	return editJSON(path, func(settings map[string]any) {
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
	})
}

// editJSON applies edit to a JSON object file (created when missing). When
// the content changes, the old file is kept as <file>.lazymux.bak and the
// file mode is preserved. Note that Go writes the keys sorted.
func editJSON(path string, edit func(root map[string]any)) (bool, error) {
	root, err := readSettings(path)
	if err != nil {
		return false, err
	}
	before, _ := json.Marshal(root)
	edit(root)
	after, _ := json.Marshal(root)
	if bytes.Equal(before, after) {
		return false, nil
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, err
	}
	return writeWithBackup(path, append(out, '\n'))
}

// editText applies edit to a text file (created when missing), like editJSON.
func editText(path string, edit func(string) string) (bool, error) {
	old, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	updated := edit(string(old))
	if updated == string(old) {
		return false, nil
	}
	return writeWithBackup(path, []byte(updated))
}

func writeWithBackup(path string, data []byte) (bool, error) {
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	if old, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".lazymux.bak", old, mode); err != nil {
			return false, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, data, mode)
}
