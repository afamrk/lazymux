# Architecture

lazymux is a single Go binary, normally running as a sidebar pane in each session (see [Sidebar](#sidebar)). It never keeps tmux state of its own: every second it asks tmux for the full session/window/pane tree, and every action is a plain `tmux` command.

```
cmd/lazymux/main.go        flags; `sidebar [toggle|off|follow]`, `hook`, `hooks [install|uninstall|status]` subcommands; starts the Bubble Tea program (sidebar or full screen); execs `tmux attach` when outside tmux
internal/tmux/             everything that talks to tmux
  client.go                Client{Socket, ClientTTY}; Run(args…) execs `tmux [-L socket] args…`
  tree.go                  Session / Window / Pane types; ListTree (one list-panes call) + ParseTree
  actions.go               capture, switch, new, rename, kill, move, swap, break, zoom, layout, send-keys
  snapshot.go              undo: Snap{Session,Window,Pane} before a kill, Restore after
  sidebar.go               sidebar panes: toggle, follow / re-dock, off, hooks, focus and size queries
internal/agent/            agent detection: agent.go (types, keys per agent), procs.go (process tree),
                           screen.go (Classify), hooks.go (hook events, install), detector.go
internal/ui/               the terminal UI (Bubble Tea model)
  app.go                   Model: selection, focus, refresh loop, key routing, layout and panel rendering
  actions.go               what each action key does: dialogs, bulk kill, toasts, undo stack
  sidebar.go               sidebar mode: focus handling, width changes, compact view
  agents.go                the Agents page: agent list, Activity, keys (answer, reply, interrupt), badges for other views
  selection.go             per-panel filters (/) and visible lists, multi-selection (space / v), action targets
  picker.go                fuzzy picker dialog: move target (m), jump anywhere (ctrl+p); swap
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
- **State.** `Classify(kind, screen)` looks only at the last 30 non-blank lines. Blocked: a selected first option (`❯ 1. Yes`, `› 1. Yes, proceed`, `● 1. Allow once`) within the last 15 lines; opencode's "Allow once"; aider's `(Y)es/(N)o` on the last line. Working: an interrupt hint (`esc to interrupt`, `esc to cancel`) in the last 10 lines, or Claude Code's spinner line `✽ Word… (`. Otherwise idle. For aider, which has neither, a screen that changed within 3 seconds counts as working. Non-breaking spaces (Claude's prompt uses them) are normalized first.
- **History.** The detector keeps a `track` per pane: the last screen, when it changed, the state and since when. `Since` changes only with the state; `Worked` turns an idle agent into "done". Tracks of panes that no longer run an agent are dropped.
- **Summary and waiting reason.** Summary: the last hook prompt, else Claude Code's terminal title (`✳ Fix JWT expiry`, minus the glyph; the hostname default is ignored). Waiting: the pending hook tool call (`Bash(go test ./...)`, the last `tool` event with only notifications after it), else the tool header and the line under it on the screen (`Bash command: go test ./...`).
- **Hooks.** `lazymux hook` reads Claude Code's hook JSON on stdin and appends a compact event (`{"t","k","n","d"}`, kinds start / prompt / tool / notify / stop / end) to the log named after `$TMUX`'s socket and `$TMUX_PANE`. `start` truncates the log; it is cut back to 200 events when it grows. A log ending in `end` is ignored. `lazymux hooks install` edits the settings JSON generically (`map[string]any`), adding one group per event and recognizing its own entries by a command that contains `lazymux` and ends in ` hook`, so reinstalling replaces them and uninstall removes only them. Note that Go re-sorts the keys of the file.
- **Actions** are plain `send-keys`: `AnswerKeys(kind, n)` per agent, `Interrupt(kind)` (`Escape`, or `C-c` for aider), and `SendLine` for replies.

In the UI, `agentRows()` builds the list for the current tab (session headers + agents, blocked sessions first, folded sessions as headers only). The selection is a pane id or `s:<session id>`, so it survives refreshes like the tmux page selection. On the Agents page `previewTarget` returns the selected agent's pane, so the normal preview capture serves both pages.
