package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Integrations install lazymux's hook into each agent's own configuration,
// so the agent reports prompts, tool calls, permission requests and the end
// of a turn to `lazymux hook <agent> <event>`. Modeled on herdr's
// integrations; lazymux's entries sit next to the user's (and other tools')
// hooks and only they are replaced or removed.
//
//	claude    ~/.claude/settings.json            "hooks" groups
//	codex     ~/.codex/hooks.json                "hooks" groups, plus [features] hooks = true in config.toml
//	agy       ~/.gemini/config/hooks.json        a named "lazymux" block
//	opencode  ~/.config/opencode/plugins/lazymux.js   a plugin that runs `lazymux hook opencode …`
type Integration struct {
	Kind      Kind
	Dir       string   // the agent's config directory; missing = agent not installed
	Files     []string // what install writes
	install   func(exe string) (bool, error)
	uninstall func() (bool, error)
	installed func() bool
}

// Detected reports whether the agent's config directory exists.
func (i Integration) Detected() bool {
	st, err := os.Stat(i.Dir)
	return err == nil && st.IsDir()
}

// Install writes lazymux's hooks; exe is the lazymux binary. It reports
// whether anything changed.
func (i Integration) Install(exe string) (bool, error) {
	if !i.Detected() {
		return false, fmt.Errorf("%s config directory %s not found: install %s first", i.Kind, i.Dir, i.Kind)
	}
	return i.install(exe)
}

// Uninstall removes lazymux's hooks.
func (i Integration) Uninstall() (bool, error) { return i.uninstall() }

// Installed reports whether lazymux's hooks are present.
func (i Integration) Installed() bool { return i.installed() }

// configDir is $env, or ~/rel.
func configDir(env string, rel ...string) string {
	if d := os.Getenv(env); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(append([]string{home}, rel...)...)
}

// hookCommand is the shell command an agent runs for an event.
func hookCommand(exe string, kind Kind, event string) string {
	return shellQuote(exe) + " hook " + string(kind) + " " + event
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// Integrations returns every agent lazymux can hook into.
func Integrations() []Integration {
	return []Integration{claudeIntegration(), codexIntegration(), agyIntegration(), opencodeIntegration()}
}

// IntegrationFor finds the integration of an agent ("claude", "codex"…).
func IntegrationFor(name string) (Integration, bool) {
	for _, i := range Integrations() {
		if string(i.Kind) == name {
			return i, true
		}
	}
	return Integration{}, false
}

// --- Claude Code and Codex: "hooks" groups --------------------------------------

// claudeEvents / codexEvents are the events lazymux listens to.
var (
	claudeEvents = []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "Notification", "Stop", "SessionEnd"}
	codexEvents  = []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PermissionRequest", "Stop"}
)

func claudeIntegration() Integration {
	dir := configDir("CLAUDE_CONFIG_DIR", ".claude")
	settings := filepath.Join(dir, "settings.json")
	return Integration{
		Kind: Claude, Dir: dir, Files: []string{settings},
		install:   func(exe string) (bool, error) { return installGroups(settings, claudeEvents, exe, Claude) },
		uninstall: func() (bool, error) { return UninstallHooks(settings) },
		installed: func() bool { return HooksInstalled(settings) },
	}
}

func codexIntegration() Integration {
	dir := configDir("CODEX_HOME", ".codex")
	hooks, config := filepath.Join(dir, "hooks.json"), filepath.Join(dir, "config.toml")
	return Integration{
		Kind: Codex, Dir: dir, Files: []string{hooks, config},
		install: func(exe string) (bool, error) {
			changed, err := installGroups(hooks, codexEvents, exe, Codex)
			if err != nil {
				return changed, err
			}
			c2, err := editText(config, enableCodexHooks)
			return changed || c2, err
		},
		// [features] hooks = true stays: other tools' hooks may need it
		uninstall: func() (bool, error) { return UninstallHooks(hooks) },
		installed: func() bool { return HooksInstalled(hooks) },
	}
}

// installGroups adds one lazymux group per event to the "hooks" object of a
// Claude-style JSON file, replacing older lazymux entries.
func installGroups(path string, events []string, exe string, kind Kind) (bool, error) {
	return editHooks(path, func(hooks map[string]any) {
		for _, ev := range events {
			groups := removeOurs(hooks[ev])
			group := map[string]any{"hooks": []any{map[string]any{
				"type": "command", "command": hookCommand(exe, kind, ev), "timeout": 5}}}
			if ev == "PreToolUse" {
				group["matcher"] = "*"
			}
			hooks[ev] = append(groups, group)
		}
	})
}

var (
	reTOMLHeader   = regexp.MustCompile(`^\s*\[([^\[\]]+)\]\s*(#.*)?$`)
	reTOMLHooksKey = regexp.MustCompile(`^\s*hooks\s*=`)
)

// enableCodexHooks sets `hooks = true` in the [features] table of Codex's
// config.toml (Codex runs hooks only with this feature on).
func enableCodexHooks(content string) string {
	lines := strings.Split(content, "\n")
	inFeatures, header := false, -1
	for i, l := range lines {
		if m := reTOMLHeader.FindStringSubmatch(l); m != nil {
			inFeatures = strings.TrimSpace(m[1]) == "features"
			if inFeatures && header < 0 {
				header = i
			}
			continue
		}
		if inFeatures && reTOMLHooksKey.MatchString(l) {
			lines[i] = "hooks = true"
			return strings.Join(lines, "\n")
		}
	}
	if header >= 0 {
		lines = append(lines[:header+1], append([]string{"hooks = true"}, lines[header+1:]...)...)
		return strings.Join(lines, "\n")
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content + "\n[features]\nhooks = true\n"
}

// --- agy (Antigravity CLI): a named block -------------------------------------------

const agyBlock = "lazymux"

func agyIntegration() Integration {
	dir := configDir("ANTIGRAVITY_CLI_CONFIG_DIR", ".gemini", "config")
	hooks := filepath.Join(dir, "hooks.json")
	return Integration{
		Kind: Agy, Dir: dir, Files: []string{hooks},
		install: func(exe string) (bool, error) {
			return editJSON(hooks, func(root map[string]any) {
				handler := func(ev string) map[string]any {
					return map[string]any{"type": "command", "command": hookCommand(exe, Agy, ev), "timeout": 5}
				}
				// SessionStart, PreInvocation and Stop take a flat handler list;
				// PostToolUse needs a matcher group. PreToolUse is left alone:
				// its output decides whether a tool may run.
				root[agyBlock] = map[string]any{
					"SessionStart":  []any{handler("SessionStart")},
					"PreInvocation": []any{handler("PreInvocation")},
					"PostToolUse":   []any{map[string]any{"matcher": "*", "hooks": []any{handler("PostToolUse")}}},
					"Stop":          []any{handler("Stop")},
				}
			})
		},
		uninstall: func() (bool, error) {
			return editJSON(hooks, func(root map[string]any) { delete(root, agyBlock) })
		},
		installed: func() bool {
			root, err := readSettings(hooks)
			return err == nil && root[agyBlock] != nil
		},
	}
}

// --- opencode: a plugin file -------------------------------------------------------------

const opencodeMarker = "LAZYMUX_INTEGRATION=opencode"

func opencodeIntegration() Integration {
	dir := configDir("OPENCODE_CONFIG_DIR", ".config", "opencode")
	if os.Getenv("OPENCODE_CONFIG_DIR") == "" && os.Getenv("XDG_CONFIG_HOME") != "" {
		dir = filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "opencode")
	}
	plugin := filepath.Join(dir, "plugins", "lazymux.js")
	return Integration{
		Kind: Opencode, Dir: dir, Files: []string{plugin},
		install: func(exe string) (bool, error) {
			want := opencodePlugin(exe)
			if old, err := os.ReadFile(plugin); err == nil && string(old) == want {
				return false, nil
			}
			if err := os.MkdirAll(filepath.Dir(plugin), 0o755); err != nil {
				return false, err
			}
			return true, os.WriteFile(plugin, []byte(want), 0o644)
		},
		uninstall: func() (bool, error) {
			data, err := os.ReadFile(plugin)
			if os.IsNotExist(err) {
				return false, nil
			}
			if err != nil || !strings.Contains(string(data), opencodeMarker) {
				return false, err // not ours: leave it
			}
			return true, os.Remove(plugin)
		},
		installed: func() bool {
			data, err := os.ReadFile(plugin)
			return err == nil && strings.Contains(string(data), opencodeMarker)
		},
	}
}

// opencodePlugin is the opencode (v1 plugin API) plugin: it forwards the
// prompt, tool calls, permission requests and the end of a turn to
// `lazymux hook opencode <event>`, without waiting for it.
func opencodePlugin(exe string) string {
	return `// installed by lazymux (lazymux hooks install); reinstalling overwrites this file.
// ` + opencodeMarker + `
// Reports opencode activity to lazymux: prompts, tool calls, permission
// requests and the end of a turn. Does nothing outside tmux.
import { spawn } from "node:child_process";

const LAZYMUX = ` + jsString(exe) + `;

function report(event, data) {
  try {
    const child = spawn(LAZYMUX, ["hook", "opencode", event], {
      stdio: ["pipe", "ignore", "ignore"],
      detached: true,
    });
    child.on("error", () => {});
    child.stdin.on("error", () => {});
    child.stdin.end(JSON.stringify(data ?? {}));
    child.unref();
  } catch {}
}

function promptText(output) {
  const parts = output?.parts ?? [];
  return parts
    .filter((p) => p?.type === "text" && typeof p.text === "string")
    .map((p) => p.text)
    .join(" ");
}

export const LazymuxPlugin = async () => {
  // a shared server (opencode serve / web) does not run in the pane it serves
  const cmd = process.argv[2];
  if (!process.env.TMUX_PANE || cmd === "serve" || cmd === "web") {
    return {};
  }
  return {
    "chat.message": async (_input, output) => {
      report("prompt", { prompt: promptText(output) });
    },
    "tool.execute.before": async (input, output) => {
      report("tool", { tool_name: input?.tool, tool_input: output?.args });
    },
    event: async ({ event }) => {
      const type = event?.type;
      const props = event?.properties ?? {};
      if (type === "permission.asked" || type === "permission.updated") {
        report("permission", { title: props.title ?? props.type ?? props.permission ?? "" });
      } else if (type === "session.idle") {
        report("stop", {});
      } else if (type === "session.status") {
        const status = typeof props.status === "string" ? props.status : props.status?.type;
        if (status === "idle") report("stop", {});
      }
    },
  };
};
`
}

// jsString quotes s as a JavaScript string literal.
func jsString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return `"` + r.Replace(s) + `"`
}
