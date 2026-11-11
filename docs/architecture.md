# Architecture

lazymux is a single Go binary, normally running as a sidebar pane in each session (see [Sidebar](#sidebar)). It never keeps tmux state of its own: every second it asks tmux for the full session/window/pane tree, and every action is a plain `tmux` command.

```
cmd/lazymux/main.go        flags; `sidebar [toggle|off|follow]`, `preview <pane> <socket>`, `hook`, `hooks [install|uninstall|status]`, `status`, `notify test` subcommands; starts the Bubble Tea program (sidebar or full screen); execs `tmux attach` when outside tmux
internal/tmux/             everything that talks to tmux
  client.go                Client{Socket, ClientTTY}; Run(args…) execs `tmux [-L socket] args…`
  tree.go                  Session / Window / Pane types; ListTree (one list-panes call) + ParseTree
  actions.go               capture, switch, new, rename, kill, move, swap, break, zoom, layout, send-keys
  snapshot.go              undo: Snap{Session,Window,Pane} before a kill, Restore after
  sidebar.go               sidebar panes: toggle, follow / re-dock, off, hooks, focus and size queries, preview popup
internal/agent/            agent detection: agent.go (types, keys per agent), procs.go (process tree),
                           screen.go (Classify), rules.go + rules/*.toml (detection rules), hooks.go (hook events, install), detector.go,
                           notify.go (Notifier, Config from tmux options, delivery, sounds),
                           leader.go (lock-file leader, status summary for `lazymux status`)
internal/project/          project directories for N: zoxide, git repos under roots (fd or a walk)
internal/ui/               the terminal UI (Bubble Tea model)
  app.go                   Model: selection, focus, refresh loop, key routing, full-screen layout and panel rendering; sidebar View → viewList
  actions.go               what each action key does: dialogs, bulk kill, toasts, undo stack
  sidebar.go               sidebar mode: focus handling, preview popup open / close, header chips
  list.go                  the sidebar list: rows (agents by state, folding session tree), cursor, filter, keys per row kind, view
  preview.go               the preview popup program: captures the pane, draws the sidebar's dialogs, forwards keys, runs fzf
  link.go                  the sidebar ⇄ popup socket: JSON-lines messages (wire), listener, connection goroutines
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

**Layout** of full-screen lazymux (`View`, `panelHeights`; the sidebar is described under [Sidebar](#sidebar)):

- left column = 30% of the width (full-screen lazymux; the sidebar draws the list instead), at least 32 and at most 60 columns (64 on the Agents page), leaving at least 30 for the preview; on wide screens the extra width goes to the preview; below 64 columns the preview is dropped
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

`lazymux sidebar cycle <session> <pane>` (the `prefix` + `b` binding) turns sidebars on if needed, then selects the session's sidebar pane, or turns them off when `<pane>` is that sidebar. While the sidebar is focused the popup has all keys, so the popup reads `prefix` / `prefix2`; on the prefix it waits for the next key, sends both to the sidebar (`prefix` wire message) and exits; when `display-popup` returns, the sidebar runs `send-keys -K -c <client>` with them (tmux looks them up in the client's key table, so the binding runs) and checks focus 300 ms later to reopen the popup if you are still in the sidebar.

`lazymux sidebar toggle` sets the global user option `@lazymux_sidebar_on` and installs two hooks in slot `[90]` (user hooks are left alone):

```
session-window-changed[90]  run-shell -b "lazymux sidebar follow '#{session_id}'"
client-session-changed[90]  run-shell -b "lazymux sidebar follow '#{session_id}'"
```

`SidebarFollow(session)` keeps exactly one sidebar per session docked in the session's current window. It creates one with `split-window -hbfd -l 30` (and marks it with the pane option `@lazymux_sidebar=<session id>`), moves it with `join-pane -hbfd -l 30`, or re-docks it (`break-pane` + `join-pane`) when it is no longer full height at the left edge. The pane keeps running while it moves, so cursor, undo stack and page survive.

`ParseTree` reads `@lazymux_sidebar` for every pane and puts sidebar panes into `Tree.Sidebars` instead of `Window.Panes`, so no other code sees them.

Inside the sidebar the program runs with `tea.WithReportFocus()`. The pane is never resized (a 60% widening on focus reflowed every other pane of the window). `FocusMsg` / `BlurMsg` only trigger `checkFocus`, which asks tmux (`SidebarState`) whether the pane is active in the active window of an attached session; each tick asks too. tmux is the truth because opening and closing a popup send focus events of their own. The same check re-docks after layout changes and puts a pane widened by an older lazymux back to 30 columns. `View` renders `viewList` in the sidebar, focused or not. The list (`listRows`) is the agents grouped by state, then the sessions as a folding tree (`fold`, the sidebar's own session open by default; everything open while filtering). Focused, a cursor (`listSel`, a row key like `a:%3`, `w:$1:@2`; `listIdx` to fall back to when the row disappears) moves through it, and `listSync` points the existing selection at the cursor row: an agent row sets `page = pageAgents` and `agentSel`, a session / window / pane row sets `page = pageTmux`, `focus` to that panel and `selSess` / `selWin` / `selPane`. So `handleListKey` only adds navigation, folding and the filter, and calls the existing actions (rename, kill, answer, reply…) and dialogs, which read that selection. Full-screen `lazymux` keeps the panels and pages.

**Preview popup.** On focus the sidebar resolves the client (`ActiveClient`: most recent `client_activity` of the session) and runs `display-popup -E -B -c <client> -x <sidebar right + 2> -y <bottom> -w <w> -h <h> 'lazymux preview <sidebar pane> <socket>'`, where w / h are `@lazymux-preview-width` / `-height` percent (default 70) of the space right of the sidebar and of its height, centered vertically (`popupPlace`). tmux 3.4 does not expand formats in `-w` / `-h` (and `popup_pane_*` were empty), so `RunPreviewPopup` computes the numbers from `display-message`; `-y` is the popup's bottom edge, shifted by the status line when it is at the top. The command blocks until the popup closes and returns `popupDoneMsg`:

```
 sidebar (Bubble Tea)            unix socket                 popup: lazymux preview %sb <socket>
 listenLink before the popup     $XDG_RUNTIME_DIR/lazymux-<uid>/<server>-<pane>.sock   (dials, retries 1s)
   ◄── hello / size {w,h}  ─────────────────────────────────  on connect and resize
   ◄── key {tea.Key}       ─────────────────────────────────  every key but ctrl+u / ctrl+d (scroll)
   ──► state {pane,title,context} ──────────────────────────►  capture-pane -e -S -2000 every 200ms
   ──► overlay "<dialog>"  ─────────────────────────────────►  drawn centered over the dimmed preview
   ──► fzf {query}         ─────────────────────────────────►  runs fzf here (ExecProcess)
   ◄── picked {dir, err}   ─────────────────────────────────  → dirPickedMsg in the sidebar
```

Every message is a JSON line; each side has a writer goroutine (keeps the order) and a reader goroutine whose channel a `tea.Cmd` waits on (`waitWire`, re-armed after each message). The sidebar keeps every dialog (input, confirm, pickers, help) as before: `Update` wraps `update` and, while a popup is connected, `syncOverlay` renders `modal.view(popupW, popupH)` after every message and sends it when it changed; `View` then leaves the dialog out of the 30 columns. The dialog's `ctrl+f` returns `pickDirReq`, which goes to the popup when one is connected, else runs fzf in place. The popup quits when the connection closes.

A popup takes all of the client's keys, which is why it forwards them. `esc` / `q` close the popup (`display-popup -C`) and select the last pane; a switch closes it and, when the target is in another session, moves the sidebar's own window off the sidebar (`unfocusSelf`), so coming back does not reopen it. `popupOpen` is cleared before the close runs, so the popup's own `popupDoneMsg` is ignored; a popup that closes some other way while the sidebar is focused makes the sidebar leave too. A `display-popup` that returns within 300 ms never opened (another popup was already open on the client), and the sidebar just stays without a preview.

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
2. feeds the agents and the watched panes (`tmux.WatchedPanes`: each client's current `#{pane_id}`, with `client_activity` within 2 minutes and, when `focus-events` is on, the `focused` client flag) to `Notifier.Observe`. It remembers each pane's previous state, when its turn started (`→ working` from idle; blocked → working continues the turn) and when it was last watched. `→ blocked` and `working → idle` create a pending note due after `Delay`, except for a watched pane; "finished" also needs a turn of at least `MinWork` and no watching in the last 10 s (an interrupt). A pending note is dropped when the state changes before it is due or when the pane is watched by then; the first sight of a pane never notifies. Leaving blocked after a "needs you" note was sent (or the agent going away) yields a `clear` note,
3. delivers due notes in goroutines (`notify-send -p`, replacing the agent's last notification with `-r <id>`, a `clear` closes it with `gdbus … CloseNotification`; `osascript`; the `OSC 9` / `OSC 99` sequence written straight to each attached client's tty from `list-clients`; a styled `display-message -c`; a sound player with a 15 s timeout, at most once per agent every 5 s),
4. writes `status-<server>.json` (counts, time) atomically.

`lazymux status` prints that file when it is younger than 10 s; otherwise it runs one detection itself (no history, so no done count).

**Done = unseen** (from herdr): `Agent.Unseen` is set when the state changes to idle while the pane is not `Visible` (its window is not the current window of an attached session) and cleared whenever it is visible, or when the Agents page selects it (`Detector.MarkSeen`). Each lazymux keeps its own seen state; they agree because visibility comes from tmux.

