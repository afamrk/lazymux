# User guide

- [The screen](#the-screen)
- [Moving around](#moving-around)
- [Switching](#switching)
- [Creating and renaming](#creating-and-renaming)
- [Killing and undo](#killing-and-undo)
- [Marks: move and swap](#marks-move-and-swap)
- [Pane and window tools](#pane-and-window-tools)
- [Help screen](#help-screen)
- [Popup vs. standalone](#popup-vs-standalone)
- [Command-line flags](#command-line-flags)

## The screen

```
╭─[1] Sessions ───────────╮╭─ session:window.%pane · command ─────────────╮
│ sessions                ││                                               │
╰─────────────── 2 of 4 ─╯│  live preview of the selected pane            │
╭─[2] Windows (session) ─╮│                                               │
│ windows of the session ││                                               │
╰─────────────── 1 of 3 ─╯│                                               │
╭─[3] Panes (n:window) ──╮│                                               │
│ panes of the window    ││                                               │
╰─────────────── 1 of 2 ─╯╰───────────────────────────────────────────────╯
 context-aware key hints                                    tmux │ agents
```

**Two pages.** lazymux has a **tmux page** (described here) and an **Agents page**. Press `a` to switch between them; the right end of the bottom bar shows which one you are on (`tmux │ agents`), and each page keeps its own selection and focus.

**Agents page** (content arrives in P3): panel [1] lists the coding agents in all sessions, grouped by session, under four state tabs switched with `[` / `]`: **All** (blocked first), **Blocked** (waiting for you: a permission prompt or a question), **Working**, and **Done** (finished its turn, waiting for a new prompt). Panel [2] **Activity** is a timeline of the selected agent, and the right side is its live pane. Panels are switched with `tab` / `h` / `l` there, because `1` / `2` / `3` will answer a blocked agent. tmux actions (`n`, `d`, `m`…) do nothing on this page. Until agent detection is built, the page is empty.

**Left column** of the tmux page: three panels, like lazygit.

- **[1] Sessions** lists every session. `●` (green) means a client is attached to it. The number on the right is its window count (`3w`). 
- **[2] Windows** lists the windows of the selected session: index, name, pane count when there is more than one (`2p`), the command running in the active pane, and `*` on the session's active window.
- **[3] Panes** lists the panes of the selected window: pane id (`%12`), command, size, and `*` on the active pane.

Selection **cascades**: choosing a session shows its windows (starting at its active window), and choosing a window shows its panes (starting at its active pane).

The **focused** panel has a blue border and a bold, highlighted row. Panel sizes do not change with focus: the column is shared equally, and a panel with few items keeps only the rows it needs and gives the rest to the others.

Each panel shows `n of m` in its bottom border: the position of the selection.

**Right side**: the live preview of the selected pane. It is the pane's visible screen with its colors, refreshed every second. The border shows `session:window.%pane · command`. If lazymux runs in a normal pane and the session or window you select resolves to that pane, the preview shows the pane you came from instead (the window's last pane, or the session's last window); the border names the pane actually shown. Only when you select lazymux's own pane in the Panes panel does it say "(this is lazymux itself)", instead of drawing a picture inside a picture.

**Bottom bar**: key hints for the focused panel, plus the marked item (`◆ …`) when there is one. Hints that don't fit are dropped; `?` always lists everything.

On terminals narrower than 64 columns, the preview is hidden and the panels use the full width.

## Moving around

| key | action |
| --- | --- |
| `j` / `↓`, `k` / `↑` | move down / up in the focused panel |
| `g` / `home`, `G` / `end` | first / last item |
| `tab` / `l` / `→` | next panel |
| `shift+tab` / `h` / `←` | previous panel |
| `1`, `2`, `3` | jump to a panel |

lazymux starts with the session, window and pane you are currently in already selected.

## Switching

`enter` switches to the selection, then lazymux exits:

- in **Sessions**: switch to that session, keeping its current window
- in **Windows**: select that window and switch to its session
- in **Panes**: select that window and pane, and switch

Inside tmux, the client is switched (see [Popup vs. standalone](#popup-vs-standalone) for which client). Outside tmux, lazymux exits and attaches your terminal to the session.

## Creating and renaming

**`n` (new)** depends on the focused panel:

- **Sessions**: opens the *New session* dialog.
  - **Name**: the session name (empty lets tmux pick a number).
  - **Dir**: the starting directory, pre-filled with the selected pane's directory. `tab` completes directory names (`~` works); when there is nothing to complete, `tab` moves to the next field. With [fzf](https://github.com/junegunn/fzf) installed, `ctrl+f` opens a fuzzy directory picker: zoxide's frequent directories first (when zoxide is installed), then everything under your home up to 4 levels deep (via `fd`, or `find`). The pick fills **Dir** and, if **Name** is still empty, names the session after the directory. The same key works in the new window and new pane dialogs.
  - **start in**: `● new session` or `○ new window in <session>`. Move to this row with `tab` / `↓` and change it with `←` / `→` / `space`.
  - `enter` creates it, `esc` cancels. The new item is selected afterwards.
- **Windows**: opens *New window in &lt;session&gt;* with Name and Dir.
- **Panes**: splits the selected pane side by side right away. The new pane starts in the same directory and gets selected.

**`r` (rename)**:

- **Sessions**: rename the session.
- **Windows**: rename the window.
- **Panes**: tmux panes have no names, so `r` sets the pane **title** instead (shown by tmux in pane borders if you enable `pane-border-status`).

The current name is pre-filled; edit it and press `enter`.

## Killing and undo

**`d` (kill)** closes the selection and everything inside it, after a red confirm dialog:

```
╭─ Kill window? ─────────────────────────────────╮
│                                                │
│ Kill window 1:server of work?                  │
│                                                │
│   2 panes                                      │
│   running: go, nvim                            │
│                                                │
│   [y] Yes, kill it     ❯ [n] No, keep it       │
│                                                │
╰─ y yes · n / esc no · u undo later ────────────╯
```

- The dialog lists how many windows and panes go away, and warns in red about **programs still running** in them (anything that is not a plain shell).
- The default is **No**: pressing `enter` right away does nothing. Press `y` to kill, or move to "Yes" with `←` / `→` / `tab` and press `enter`.
- lazymux refuses to kill its own pane.
- Windows are killed with `unlink-window -k`: a window linked into several sessions is only removed from this one.

After a kill, a green toast confirms it: `✓ killed window 1:server · u to undo`.

**`u` (undo)** recreates the last killed item. Before every kill, lazymux records:

- the session / window name and window number
- the window layout (splits and sizes)
- each pane's directory and foreground command

Undo rebuilds that: same names, same splits, each pane in its old directory, and the program re-run. Shells and interpreters (`zsh`, `bash`, `node`, `python`…) are not re-run. A few programs get a resume command: Claude Code is restarted with `claude --continue` (its last conversation in that directory), Codex with `codex resume --last`.

What undo **cannot** bring back, because tmux cannot revive a process:

- the screen contents and scrollback
- unsaved work inside programs
- the state of running servers, tests or REPLs (they start over)
- shell history and environment changes of that pane

Undo keeps the last 20 kills, most recent first, but only while lazymux is running. In popup mode it closes after every switch, so undo mostly helps with mistakes made in the same popup session. (Whether to keep the history on disk is an [open decision](roadmap.md#open-decisions).)

## Marks: move and swap

Some actions need two things: the item to change (the selection) and a **target**. You set the target by marking it.

1. Select the target and press **`space`**. It gets a mauve `◆` and the bottom bar shows `◆ session api` (or the window / pane).
2. Select the item you want to change and press **`m`** or **`s`**.
3. After a successful move or swap the mark is cleared. `space` on the marked item again, or `esc`, clears it by hand.

**`m` (move / join)**:

- **Window → marked session**: moves the selected window to the end of the marked session. Example: mark session `api`, select window `editor` in `work`, press `m`; `editor` now lives in `api`.
- **Pane → marked window or pane**: joins the selected pane into the marked window (or next to the marked pane) as a side-by-side split. Example: mark your editor window, select the `logs` pane in another window, press `m`; the logs appear beside the editor.

**`s` (swap)**:

- **Window ↔ marked window**: the two windows trade places (index and position). They can be in different sessions.
- **Pane ↔ marked pane**: the two panes trade places, e.g. editor-left / shell-right becomes shell-left / editor-right.

If the mark is missing or of the wrong kind, a toast says what to mark (for example "move window: mark a target first with space on a session").

## Pane and window tools

| key | panel | action |
| --- | --- | --- |
| `z` | Panes | **Zoom**: the pane fills its whole window, temporarily. `z` again un-zooms. Nothing is closed. |
| `b` | Panes | **Break**: take the pane out of its split into a new window of its own, at the end of the same session. The opposite of join. |
| `L` | Windows, Panes | **Layout**: cycle the window through tmux's preset layouts: even-horizontal (side by side), even-vertical (stacked), main-vertical (one big pane left), main-horizontal (one big pane on top), tiled (grid). |

These need a window with at least two panes; otherwise a toast explains why nothing happened.

## Help screen

`?` opens the keybindings screen. It shows which panel you are in and lists every key in groups. Keys of later phases are dimmed and marked "(soon)".

- `j` / `k` or `↑` / `↓`: move through the list
- `enter`: run the selected key, as if you had pressed it
- `/`: filter (type to narrow, e.g. `zoom`); `esc` clears the filter
- `esc`, `q` or `?`: close

## Popup vs. standalone

**Popup (recommended).** With the binding from the README:

```tmux
bind L display-popup -E -w 90% -h 90% "lazymux --client '#{client_tty}'"
```

lazymux opens on top of your current window and closes after `enter`. The `--client '#{client_tty}'` part pins switching to *the terminal that opened the popup*. Without it, tmux switches "the most recently active client", which goes wrong when several terminals are attached to the same server.

**In a pane or window.** Just run `lazymux`. It behaves the same; `enter` switches your client and lazymux exits.

**Outside tmux.** Run `lazymux` in a plain terminal. It talks to the default tmux server; `enter` prepares the selection and then replaces lazymux with `tmux attach` to that session.

## Command-line flags

| flag | meaning |
| --- | --- |
| `--client <tty>` | client to switch, e.g. `'#{client_tty}'` in a key binding |
| `-L <name>` | use the tmux server with this socket name (same as `tmux -L`) |
| `--version` | print the version and exit |
