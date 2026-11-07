# User guide

- [The sidebar](#the-sidebar)
- [The screen](#the-screen)
- [Moving around](#moving-around)
- [Switching](#switching)
- [Creating and renaming](#creating-and-renaming)
- [Killing and undo](#killing-and-undo)
- [Filter and jump](#filter-and-jump)
- [Selecting, moving and swapping](#selecting-moving-and-swapping)
- [Pane and window tools](#pane-and-window-tools)
- [Help screen](#help-screen)
- [Command-line flags](#command-line-flags)

## The sidebar

lazymux lives in a **sidebar**: a pane on the left of your current window. Everything else in this guide describes what you see when the sidebar is focused (or when you run plain `lazymux` full screen).

```tmux
set -g focus-events on
bind b run-shell "lazymux sidebar toggle"
```

- **`prefix` + `b`** turns the sidebar on or off. "On" is global: every session gets its own sidebar, and it **follows you**. A tmux hook moves it into whichever window becomes current, so it is always docked on the left, full height, 30 columns wide.
- **Unfocused**, it is a compact overview: `lazymux` and the current page at the top, then every session with its windows (the current session in bold, `▸` marking the window you are in), and an agents section, which is empty until P3. On the Agents page it shows the state tabs and agents instead. If the list is longer than the pane, it scrolls so your current window stays visible.
- **Focus it** like any other pane (`prefix` + `←`, a mouse click, `select-pane -L`). It grows to **60% of the window** and shows the full lazymux described in this guide: the three panels, a preview (smaller than when lazymux runs full screen), the Agents page, dialogs and help.
- **Leave it** with `esc` or `q` (focus goes back to the pane you came from), or move to another pane yourself. It shrinks back to 30 columns.
- **`enter`** switches to the selection and the sidebar stays. Focus moves to the target, so the sidebar shrinks, and if you went to another window the sidebar moves there too.
- **`Q`** turns the sidebar off everywhere, like `prefix` + `b`.

Other details:
- The sidebar pane is invisible to lazymux itself. It is not listed in the Panes panel, not counted, never previewed, and moves, kills and undo never touch it.
- If a layout change (`L`, `select-layout`, a new split) pushes the sidebar out of place, it docks itself back within a second. Zooming another pane (`z` / `prefix` + `z`) is left alone.
- Without `focus-events on`, lazymux still notices focus changes, but only within a second.
- Killing the window the sidebar is in kills the sidebar too; a new one appears in the next current window.

**Without the sidebar.** Run `lazymux` in any pane or terminal. `enter` switches and lazymux exits. Outside tmux, `enter` replaces lazymux with `tmux attach` to the chosen session. `--client '#{client_tty}'` pins switching to one terminal when several are attached (useful in a `display-popup` binding).

## The screen

This is the focused sidebar (or plain `lazymux`):

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

**Right side**: the live preview of the selected pane. It is the pane's visible screen with its colors, refreshed every second. The border shows `session:window.%pane · command`. The sidebar pane itself is never shown or listed. If you run plain `lazymux` in a normal pane and the session or window you select resolves to that pane, the preview shows the pane you came from instead (the window's last pane, or the session's last window); the border names the pane actually shown. Only when you select lazymux's own pane in the Panes panel does it say "(this is lazymux itself)", instead of drawing a picture inside a picture.

**Bottom bar**: key hints for the focused panel; while items are selected it shows `◆ N selected` and what you can do with them. Hints that don't fit are dropped; `?` always lists everything.

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

`enter` switches to the selection. The sidebar stays open (it shrinks, because focus moves to the target); plain `lazymux` exits.

- in **Sessions**: switch to that session, keeping its current window
- in **Windows**: select that window and switch to its session
- in **Panes**: select that window and pane, and switch

Inside tmux, the client showing the sidebar is switched (plain `lazymux` can be pinned to a terminal with `--client`, see [the sidebar](#the-sidebar)). Outside tmux, plain `lazymux` exits and attaches your terminal to the session.

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

- With several items selected, one dialog covers them all (see [Selecting, moving and swapping](#selecting-moving-and-swapping)).
- The dialog lists how many windows and panes go away, and warns in red about **programs still running** in them (anything that is not a plain shell).
- The default is **No**: pressing `enter` right away does nothing. Press `y` to kill, or move to "Yes" with `←` / `→` / `tab` and press `enter`.
- lazymux refuses to kill its own pane.
- Windows are killed with `unlink-window -k`: a window linked into several sessions is only removed from this one.

After a kill, a green toast confirms it: `✓ killed window 1:server · u to undo`.

**`u` (undo)** recreates the last killed item (or the whole batch of a bulk kill). Before every kill, lazymux records:

- the session / window name and window number
- the window layout (splits and sizes)
- each pane's directory and foreground command

Undo rebuilds that: same names, same splits, each pane in its old directory, and the program re-run. Shells and interpreters (`zsh`, `bash`, `node`, `python`…) are not re-run. A few programs get a resume command: Claude Code is restarted with `claude --continue` (its last conversation in that directory), Codex with `codex resume --last`.

What undo **cannot** bring back, because tmux cannot revive a process:

- the screen contents and scrollback
- unsaved work inside programs
- the state of running servers, tests or REPLs (they start over)
- shell history and environment changes of that pane

Undo keeps the last 20 kills, most recent first, but only while lazymux is running. The sidebar keeps running, so undo covers everything since the sidebar started; a plain `lazymux` run exits after every switch, so there it only covers that run. (Whether to keep the history on disk is an [open decision](roadmap.md#open-decisions).)

## Filter and jump

**`/` (filter)** narrows the focused panel. A filter line `/ cla` appears at the top of the panel and the list shows only matching items, with the matched letters highlighted in peach. Matching is fuzzy: the letters must appear in order, not next to each other (`cla` matches `claude-api` and `clang-tools`).

- Sessions match on their name; windows on name, number and command; panes on command, id, title and directory.
- While typing, `↑` / `↓` move through the matches. `enter` keeps the filter and gives the keys back (`j`, `space`, `d`… work on the filtered list); `esc` clears it.
- Each panel keeps its own filter until you clear it with `esc`.

**`ctrl+p` (jump anywhere)** opens one fuzzy list of every session (`SESS`), window (`WIN`) and pane (`PANE`, only for windows with several panes), with the command and directory on the right. Type a few letters, e.g. `apicl` for `api:3 claude`.

- `enter` goes there (switches, like `enter` in the panels).
- `tab` only selects it in the panels, so you can act on it (kill, move…) without switching.

## Selecting, moving and swapping

**`space`** selects the item under the cursor: it gets a mauve `◆` and the panel's border shows `3 of 6 · 2 selected`. Press `space` again to unselect. All selected items are in one panel; selecting in another panel starts over.

**`v`** selects a range: every item from the last one you selected with `space` to the cursor. With a filter active, only the visible items are included, so `/ cla` → `g` `space` `G` `v` selects every match.

While something is selected, the bottom bar shows what you can do with it: `◆ 2 selected · m move · d kill all · space select · v range · esc clear`. **`esc`** clears the selection.

Actions use the selection when there is one in the focused panel, otherwise the item under the cursor:

**`d` (kill)** asks once for all of them, listing up to five (`◆ window 1:claude-api of api (1 panes)` …). **`u`** brings the whole batch back in one step.

**`m` (move)** opens a picker for the destination:

- **Windows → a session**: `Move 2 windows to` lists the other sessions. Type to filter, `enter` moves them (to the end of that session).
- **Panes → a window**: `Join pane into` lists every other window. The panes are joined into it as side-by-side splits.

**`s` (swap)** exchanges two items: select one with `space`, move the cursor to the other, press `s`. Windows trade places (they may be in different sessions); panes trade places in their layouts.

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

## Command-line flags

| flag | meaning |
| --- | --- |
| `--client <tty>` | client to switch, e.g. `'#{client_tty}'` in a key binding |
| `-L <name>` | use the tmux server with this socket name (same as `tmux -L`) |
| `--version` | print the version and exit |

| subcommand | meaning |
| --- | --- |
| `lazymux sidebar toggle` | turn sidebars on / off (for the key binding) |
| `lazymux sidebar off` | close all sidebars and remove the hooks |
| `lazymux sidebar` | run the sidebar UI (started by the toggle inside the sidebar pane) |
| `lazymux sidebar follow <session-id>` | dock the session's sidebar in its current window (run by the hook) |
