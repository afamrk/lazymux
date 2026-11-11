package agent

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/afamrk/lazymux/internal/tmux"
)

// The words of a notification: where the agent is ("api › editor") and, in
// plain words, what it waits for ("run  go test ./...") or what it did.

// notePlace is "session › window", without the window when its name only
// repeats the agent or the program ("claude", "node").
func notePlace(k Kind, p *tmux.Pane) string {
	s, w := p.Window.Session.Name, p.Window.Name
	if w == "" || strings.EqualFold(w, string(k)) || strings.EqualFold(w, DisplayName(k)) || w == p.Command {
		return s
	}
	return s + " › " + w
}

var (
	reCall       = regexp.MustCompile(`^([\w.:-]+)\((.*)\)$`)                 // hooks: Bash(go test ./...)
	reScreenTool = regexp.MustCompile(`^([\w ]+?)(?: command| file)?: (.+)$`) // screen: Bash command: go test ./...
)

// toolVerbs maps tool names of every agent to what you would call it.
var toolVerbs = map[string]string{
	"bash": "run", "shell": "run", "run_command": "run", "run_shell_command": "run", "exec_command": "run",
	"local_shell": "run", "command": "run",
	"edit": "edit", "multiedit": "edit", "write": "edit", "write_file": "edit", "replace": "edit",
	"apply_patch": "edit", "str_replace": "edit", "str_replace_editor": "edit", "edit_file": "edit",
	"create": "edit", "patch": "edit", "delete": "delete",
	"read": "read", "read_file": "read", "view": "read", "view_file": "read",
	"webfetch": "fetch", "web fetch": "fetch", "fetch": "fetch", "web_fetch": "fetch", "read_url_content": "fetch",
	"websearch": "search", "web search": "search", "web_search": "search", "google_web_search": "search",
	"askuserquestion": "asks", "question": "asks",
}

// describeWaiting turns a waiting reason into a verb and its target:
// "Bash(go test ./...)" → "run", "go test ./..."; "Edit file: /src/app.go"
// → "edit", "app.go"; "Which database?" → "asks", "Which database?".
func describeWaiting(w, cwd string) (verb, what string) {
	w = strings.TrimSpace(w)
	name := ""
	if m := reCall.FindStringSubmatch(w); m != nil {
		name, what = m[1], m[2]
	} else if m := reScreenTool.FindStringSubmatch(w); m != nil && toolVerbs[strings.ToLower(m[1])] != "" {
		name, what = m[1], m[2] // a screen text with a known tool header
	} else if strings.HasSuffix(w, "?") {
		return "asks", clip(w, 90)
	} else {
		return "", clip(w, 90)
	}
	verb = toolVerbs[strings.ToLower(name)]
	switch verb {
	case "edit", "read", "delete":
		what = shortPath(what, cwd)
	case "fetch":
		if u, err := url.Parse(what); err == nil && u.Host != "" {
			what = u.Host + strings.TrimSuffix(u.Path, "/")
		}
	case "":
		verb, what = "use", strings.TrimSpace(name+" "+what)
		if strings.HasPrefix(name, "mcp__") { // mcp__server__tool
			verb, what = "use", strings.ReplaceAll(strings.TrimPrefix(name, "mcp__"), "__", " ")
		}
	}
	return verb, clip(what, 90)
}

// shortPath shows a path relative to the agent's directory, else from ~.
func shortPath(p, cwd string) string {
	p = strings.TrimSpace(p)
	if cwd != "" && strings.HasPrefix(p, cwd+"/") {
		return strings.TrimPrefix(p, cwd+"/")
	}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home+"/") {
		p = "~/" + strings.TrimPrefix(p, home+"/")
	}
	if len(p) > 60 && strings.Contains(p, "/") {
		return "…/" + filepath.Base(p)
	}
	return p
}

var reSlug = regexp.MustCompile(`^[a-z0-9]+([-_][a-z0-9]+)+$`)

// humanize turns a slug title ("sidebar-preview-popup") into words
// ("Sidebar preview popup"); other text keeps its case.
func humanize(s string) string {
	s = strings.TrimSpace(s)
	if reSlug.MatchString(s) {
		s = strings.NewReplacer("-", " ", "_", " ").Replace(s)
	}
	r := []rune(s)
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
	}
	return clip(string(r), 90)
}
