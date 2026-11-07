# Architecture

lazymux is a single Go binary. It never keeps tmux state of its own: every second it asks tmux for the full session/window/pane tree, and every action is a plain `tmux` command.

```
cmd/lazymux/main.go        flags, starts the Bubble Tea program, execs `tmux attach` when outside tmux
internal/tmux/             everything that talks to tmux
  client.go                Client{Socket, ClientTTY}; Run(args…) execs `tmux [-L socket] args…`
  tree.go                  Session / Window / Pane types; ListTree (one list-panes call) + ParseTree
  actions.go               capture, switch, new, rename, kill, move, swap, break, zoom, layout, send-keys
  snapshot.go              undo: Snap{Session,Window,Pane} before a kill, Restore after
internal/ui/               the terminal UI (Bubble Tea model)
  app.go                   Model: selection, focus, refresh loop, key routing, layout and panel rendering
  actions.go               what each action key does: dialogs, marks, toasts, undo stack
  agents.go                the Agents page: page switch (`a`), state tabs, its panels and keys
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
          │   -F <19 fields>  │               │   -e -p -t %id    │
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
- **Selection by id, not index.** The model stores the selected session, window and pane ids. After every refresh, `cur()` resolves them back to items, falling back to the session's active window and the window's active pane when an id has disappeared. This keeps the cursor stable while things are created, killed or reordered.
- **All tmux calls run inside `tea.Cmd`s**, off the UI goroutine. Results come back as messages (`treeMsg`, `previewMsg`, `actionMsg`, `switchMsg`).

## UI structure

**Layout** (`View`, `panelHeights`):

- left column = 30% of the width, at least 32 columns, leaving at least 30 for the preview; below 64 columns the preview is dropped
- panel heights do not depend on focus: each panel wants `items + 2` rows, the body is shared by water-filling (a panel needing less than an equal share keeps only that, the rest is split among the others), spare rows are handed out so the column stays full, minimum 3 rows
- the Agents page (`page == pageAgents`) has its own layout: list + Activity (`clamp(bodyH/3, 3, 8)` rows) and a 36% left column
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

Before a kill, `SnapSession` / `SnapWindow` / `SnapPane` record names, the window index, the layout string, and per pane the directory and a re-run command (`rerunCommand`: shells and interpreters are skipped, `claude` → `claude --continue`, `codex` → `codex resume --last`, anything else re-runs by name). `Restore` recreates the items with `new-session` / `new-window` / `split-window -c <dir>`, applies the layout, and types the commands with `send-keys`, so a shell stays behind when the program exits. The stack holds 20 snapshots in memory.

## Planned: agent detection (P3)

Agents will be found through an interface so both approaches can plug in:

```go
type Detector interface {
	Detect(tree *tmux.Tree) []Agent // Agent{PaneID, Kind, State, Summary, Since, …}
}
```

- **Scraping**: walk each pane's process tree for known agent binaries, and infer the state from the screen and `#{pane_title}` (Claude Code puts a task summary in its terminal title). It needs no setup.
- **Hooks**: agents report events (prompt, tool calls, permission requests) that lazymux reads. That gives richer data (the Activity timeline, the waiting reason) but needs setup per agent.

Which to build first is an open decision; see [roadmap.md](roadmap.md#open-decisions).
