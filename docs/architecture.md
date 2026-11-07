# Architecture

lazymux is a single Go binary, normally running as a sidebar pane in each session (see [Sidebar](#sidebar)). It never keeps tmux state of its own: every second it asks tmux for the full session/window/pane tree, and every action is a plain `tmux` command.

```
cmd/lazymux/main.go        flags; `sidebar [toggle|off|follow]`, `hook`, `hooks [install|uninstall|status]`, `status`, `notify test` subcommands; starts the Bubble Tea program (sidebar or full screen); execs `tmux attach` when outside tmux
internal/tmux/             everything that talks to tmux
  client.go                Client{Socket, ClientTTY}; Run(args…) execs `tmux [-L socket] args…`
  tree.go                  Session / Window / Pane types; ListTree (one list-panes call) + ParseTree
  actions.go               capture, switch, new, rename, kill, move, swap, break, zoom, layout, send-keys
  snapshot.go              undo: Snap{Session,Window,Pane} before a kill, Restore after
  sidebar.go               sidebar panes: toggle, follow / re-dock, off, hooks, focus and size queries
internal/agent/            agent detection: agent.go (types, keys per agent), procs.go (process tree),
                           screen.go (Classify), rules.go + rules/*.toml (detection rules), hooks.go (hook events, install), detector.go,
                           notify.go (Notifier, Config from tmux options, delivery, sounds),
                           leader.go (lock-file leader, status summary for `lazymux status`)
internal/project/          project directories for N: zoxide, git repos under roots (fd or a walk)
internal/ui/               the terminal UI (Bubble Tea model)
  app.go                   Model: selection, focus, refresh loop, key routing, layout and panel rendering
  actions.go               what each action key does: dialogs, bulk kill, toasts, undo stack
  sidebar.go               sidebar mode: focus handling, width changes, compact view
  agents.go                the Agents page: agent list, Activity, keys (answer, reply, interrupt), badges for other views
  selection.go             per-panel filters (/) and visible lists, multi-selection (space / v), action targets
  projects.go              project picker (N), new agent dialog (n)
  picker.go                fuzzy picker dialog: move target (m), jump anywhere (f, agents included); swap
  fuzzy.go                 fuzzy matcher (subsequence + bonuses) and match highlighting
  fzf.go                   ctrl+f directory picker in dialogs (fzf over zoxide + fd)
  modal.go                 dialogs (input, confirm, help) + overlay / dim helpers
  panel.go                 renderRow (one list row), renderFrame / renderBox (bordered boxes), scrolling
  keys.go                  the binding table shown by the help screen
  theme.go                 Catppuccin Mocha colors and styles
```

## Data flow

```
             every 1s (tickMsg)                 on selection change
                    │                                   │
          ┌─────────▼─────────┐               ┌─────────▼─────────┐
          │ tmux list-panes -a│               │ tmux capture-pane │
          │   -F <20 fields>  │               │   -e -p -t %id    │
          └─────────┬─────────┘               └─────────┬─────────┘
               treeMsg                             previewMsg
                    │                                   │
                    └──────────────►  Model  ◄──────────┘
                                       │
            key ──► handleKey ──► action tea.Cmd (runs tmux) ──► actionMsg
                                       │                            │
                                       │            toast, new selection, undo push,
                                       │            immediate tree refresh
                                       ▼
                                     View()
```

- **One query for everything.** `list-panes -a` with a format string returns one line per pane carrying its session and window fields too. `ParseTree` folds that into sessions → windows → panes, keeping tmux's order.
- **Field separator is a tab.** tmux escapes control characters such as `\x1f` in format output (it prints `\037`), but passes tabs through. `pane_title` is the last field and is split with `SplitN`, so titles may contain tabs.
- **Linked windows.** A window linked into two sessions appears under both; windows are keyed by session id + window id.
- **Filtered lists.** `cur()` and `move()` work on `visSessions` / `visWindows` / `visPanes`, the lists after each panel's filter, so the cursor never lands on a hidden item and indices in the view are positions in the filtered list.
- **Selection by id, not index.** The model stores the selected session, window and pane ids. After every refresh, `cur()` resolves them back to items, falling back to the session's active window and the window's active pane when an id has disappeared. This keeps the cursor stable while things are created, killed or reordered.
- **All tmux calls run inside `tea.Cmd`s**, off the UI goroutine. Results come back as messages (`treeMsg`, `previewMsg`, `actionMsg`, `switchMsg`).

## UI structure

**Layout** (`View`, `panelHeights`):

- left column = 30% of the width (45% in the sidebar), at least 32 and at most 60 columns (64 on the Agents page), leaving at least 30 for the preview; on wide screens the extra width goes to the preview; below 64 columns the preview is dropped
- panel heights do not depend on focus: each panel wants `items + 2` rows, the body is shared by water-filling (a panel needing less than an equal share keeps only that, the rest is split among the others), spare rows are handed out so the column stays full, minimum 3 rows
- the Agents page (`page == pageAgents`) has its own layout: list + Activity (`clamp(bodyH/3, 3, 10)` rows) and a 36% left column
- the preview never captures lazymux's own pane when a session or window is selected: `previewTarget` substitutes the window's last pane (`#{pane_last}`) or the session's last window (`#{window_last_flag}`)

**Rows** are built from segments (`seg{text, style}`) by `renderRow`, which right-aligns the meta part, truncates the left part with `…`, and applies the selection background to every segment. That is needed because an inner ANSI reset would otherwise end the background.

**Boxes**: `renderFrame` draws a rounded border with a styled title in the top line and a footer (counter or key hints) in the bottom line. `renderBox` is the panel variant (blue when focused).

**Dialogs** implement a small interface:

```go
type modal interface {
	update(k tea.KeyMsg) (cmd tea.Cmd, done bool)
	view(maxW, maxH int) string
}
```

While a modal is open it receives all keys. `View` draws the main screen, strips its colors (`dim`), and composes the dialog in the middle with `overlay`, which cuts each background line around the dialog with `ansi.Truncate` / `ansi.TruncateLeft`. Toasts are overlaid the same way, bottom-right.

**Help** is generated from the `bindings` table in `keys.go`. Executing a row returns the row's key as a `tea.KeyMsg`, which goes through the normal key handler.

## tmux details worth knowing

| topic | what lazymux does | why |
| --- | --- | --- |
| which client to switch | `--client '#{client_tty}'`; every `switch-client` gets `-c <tty>` | with several attached clients, tmux would otherwise pick "the most recent" one (lesson from tmux-fzf) |
| kill window | `unlink-window -k` | removes a linked window only from this session; kills it if it is not linked elsewhere |
| break pane | `break-pane -d -s %id -t <session>:` | without `-t`, tmux creates the window in the *current client's* session, i.e. wherever lazymux runs |
| creating things | `-d -P -F '#{…_id}'` | create without switching, and get the new id back to select it |
| layout cycling | `next-layout` | tmux's own preset order |
| undo layout | `select-layout <window_layout string>` | restores exact splits; best effort when the window size changed |

## Keyboard input quirk

Bubble Tea delivers runes that arrive in the same read as **one** `KeyMsg` (for example `"kk"` when a key is held or typed fast, or when keys are sent with `tmux send-keys`). `handleKey` splits such messages and handles each rune on its own; without that, held-down `j` did nothing. Dialogs receive the message unsplit, which text inputs handle correctly.

## Undo

Before a kill, `SnapSession` / `SnapWindow` / `SnapPane` record names, the window index, the layout string, and per pane the directory and a re-run command (`rerunCommand`: shells and interpreters are skipped, `claude` → `claude --continue`, `codex` → `codex resume --last`, anything else re-runs by name). `Restore` recreates the items with `new-session` / `new-window` / `split-window -c <dir>`, applies the layout, and types the commands with `send-keys`, so a shell stays behind when the program exits. The stack holds 20 steps in memory; a bulk kill is one step holding several snapshots, restored in reverse order.

## Sidebar

`lazymux sidebar toggle` sets the global user option `@lazymux_sidebar_on` and installs two hooks in slot `[90]` (user hooks are left alone):

```
session-window-changed[90]  run-shell -b "lazymux sidebar follow '#{session_id}'"
client-session-changed[90]  run-shell -b "lazymux sidebar follow '#{session_id}'"
```

`SidebarFollow(session)` keeps exactly one sidebar per session docked in the session's current window. It creates one with `split-window -hbfd -l 30` (and marks it with the pane option `@lazymux_sidebar=<session id>`), moves it with `join-pane -hbfd -l 30`, or re-docks it (`break-pane` + `join-pane`) when it is no longer full height at the left edge. The pane keeps running while it moves, so cursor, undo stack and page survive.

`ParseTree` reads `@lazymux_sidebar` for every pane and puts sidebar panes into `Tree.Sidebars` instead of `Window.Panes`, so no other code sees them.

Inside the sidebar the program runs with `tea.WithReportFocus()`. `FocusMsg` / `BlurMsg` resize the pane to 60% of the window or back to 30 columns. Each tick also asks tmux (`SidebarState`) whether the pane is active in the active window of an attached session, what its width is, and whether it is docked. That covers missing focus events, fixes a stale width after a move, and re-docks after layout changes. `View` renders `viewCompact` while unfocused or narrower than 50 columns; otherwise it renders the normal UI with the panel column at 45% of the pane.

## Agent detection

`internal/agent` finds agents by scraping, and adds hook data where it exists. Both feed one `Agent{Pane, Kind, State, Since, Worked, Summary, Waiting, Hooked, Events}`.

```
tick ──► detectAgents (one at a time) ──► Detector.Detect(tree) ──► agentsMsg ──► Model.agents[paneID]
                                             │
              ┌──────────────────────────────┼───────────────────────────────┐
              ▼                              ▼                               ▼
   kind: pane_current_command      state: capture-pane -p           hooks: ~/.cache/lazymux/events/
   (claude, codex…), or `ps`       (plain text) → Classify          <server>-<pane>.jsonl, re-read
   descendants of pane_pid when    bottom 30 lines: prompt?         when its mtime / size changes
   the command is node/python/…    interrupt hint? spinner?
```

- **Kind.** `KindOf(pane_current_command)` covers native binaries. Only panes running an interpreter (`node`, `python*`, `bun`, `deno`, a version-number name) cost a `ps -axo pid,ppid,args` call, done at most once per detection; the pane's process tree is searched for an agent script (`@google/gemini-cli`, `@openai/codex`, a `claude` / `aider` basename…).
- **State.** `Classify(kind, screen, title)` runs the agent's detection rules (`rules.go`) on the screen (non-breaking spaces normalized, lines right-trimmed) and the pane title. The rules are herdr-format TOML manifests: bundled ones in `internal/agent/rules/*.toml` (embedded), replaced per agent by `~/.config/lazymux/agent-detection/<agent>.toml` (or herdr's file name, `antigravity.toml` for agy). `ManifestFor` caches a manifest and reloads an override when its mtime / size changes; a broken override falls back to the bundled rules with a warning. Every rule is a gate (`contains` / `regex` / `line_regex`, nested `all` / `any` / `not`) on a region (`bottom_non_empty_lines(N)`, `after_last_horizontal_rule`, `prompt_box_body`, `osc_title`… all of herdr's; `osc_progress` is always empty, tmux does not keep it); the matching rule with the highest priority wins. `goRegex` translates the Rust escapes herdr's patterns use (`\uXXXX`, `\u{…}`, `\p{Alphabetic}`). The bundled rules: blocked on a selected first option (`❯ 1. Yes`, `› 1. Yes, proceed`, `● 1. Allow once`) in the last 15 lines, opencode's permission panel, aider's `(Y)es/(N)o`; working on an interrupt hint in the last 10 lines unless it is a menu (`not` `enter to select`), Claude Code's spinner line, or its spinning title (braille or half-circle glyph, ahead of the screen but below a prompt); Claude's transcript view keeps the last state. No match is idle, except for aider, where a screen that changed within 3 seconds counts as working. Blocked agents get a waiting reason from the lines above the prompt's first option. `lazymux rules explain <pane>` prints every rule's outcome.
- **History.** The detector keeps a `track` per pane: the last screen, when it changed, the state and since when. `Since` changes only with the state; `Worked` turns an idle agent into "done". Tracks of panes that no longer run an agent are dropped.
- **Summary and waiting reason.** Summary: the last hook prompt, else Claude Code's terminal title (`✳ Fix JWT expiry`, minus the glyph; the hostname default is ignored). Waiting: the pending hook tool call (`Bash(go test ./...)`, the last `tool` event with only notifications after it), else the tool header and the line under it on the screen (`Bash command: go test ./...`).
- **Hooks.** `lazymux hook <agent> <event>` reads the agent's hook payload on stdin and appends a compact event (`{"t","k","n","d"}`, kinds start / prompt / tool / notify / stop / end) to the log named after `$TMUX`'s socket and `$TMUX_PANE`. `eventFromHook` maps every agent's event names (`UserPromptSubmit` / `PreInvocation` / `prompt`, `PreToolUse` / `PostToolUse` / `tool`, `PermissionRequest` / `Notification`…) and looks fields up by all their spellings: snake_case from Claude Code and Codex, agy's camelCase protojson (`lastUserInput`, `toolCall.name` / `.args`, `CommandLine`, `AbsolutePath`), the plugin's own. Repeats are dropped (agy sends `PreInvocation` before every model call of a turn). `start` truncates the log; it is cut back to 200 events when it grows; a log ending in `end` is ignored. `hookReply` prints what an agent requires (`{}` for agy and Codex's `Stop`).
- **Integrations** (`integrations.go`, after herdr's): one entry per agent with its config directory (env override, else the default), install / uninstall / status. Claude Code and Codex share `installGroups` (one group per event in a `"hooks"` object; lazymux's commands are recognized by `lazymux` + ` hook`); Codex also gets `hooks = true` in `[features]` of `config.toml` (a line edit, kept on uninstall); agy gets a whole named block `"lazymux"`; opencode gets a v1 plugin file (marked with `LAZYMUX_INTEGRATION=opencode`) that spawns `lazymux hook opencode <event>` detached. Every edit goes through `editJSON` / `editText`, which back up the old file and keep its mode.
- **Actions** are plain `send-keys`: `AnswerKeys(kind, n)` per agent, `Interrupt(kind)` (`Escape`, or `C-c` for aider), and `SendLine` for replies.

In the UI, `agentRows()` builds the list for the current tab (session headers + agents, blocked sessions first, folded sessions as headers only). The selection is a pane id or `s:<session id>`, so it survives refreshes like the tmux page selection. On the Agents page `previewTarget` returns the selected agent's pane, so the normal preview capture serves both pages.

## Notifications and status

Every lazymux process detects agents, but only the **leader** acts on changes. `Leader.Acquire` takes a non-blocking `flock` on `~/.cache/lazymux/leader-<server>.lock` and keeps the file open; the kernel drops the lock when the process exits, so the next detection of another sidebar takes over. In the detection command (off the UI goroutine) the leader:

1. reads the `@lazymux-*` options (`show-options -g`, every detection, so changes apply immediately),
2. feeds the agents to `Notifier.Observe`, which remembers each pane's previous state: `→ blocked` and `working → idle` create a pending note due after `Delay`; a pending note is dropped when the state changes before it is due; the first sight of a pane never notifies,
3. delivers due notes in goroutines (`notify-send` / `osascript`, the `OSC 9` / `OSC 99` sequence written straight to each attached client's tty from `list-clients`, `display-message -c`, a sound player with a 15 s timeout),
4. writes `status-<server>.json` (counts, time) atomically.

`lazymux status` prints that file when it is younger than 10 s; otherwise it runs one detection itself (no history, so no done count).

**Done = unseen** (from herdr): `Agent.Unseen` is set when the state changes to idle while the pane is not `Visible` (its window is not the current window of an attached session) and cleared whenever it is visible, or when the Agents page selects it (`Detector.MarkSeen`). Each lazymux keeps its own seen state; they agree because visibility comes from tmux.

