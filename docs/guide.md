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
- [Agents](#agents)
- [Notifications and status line](#notifications-and-status-line)
- [Help screen](#help-screen)
- [Command-line flags](#command-line-flags)

## The sidebar

lazymux lives in a **sidebar**: a pane on the left of your current window. Everything else in this guide describes what you see when the sidebar is focused (or when you run plain `lazymux` full screen).

```tmux
set -g focus-events on
bind b run-shell "lazymux sidebar toggle"
```

- **`prefix` + `b`** turns the sidebar on or off. "On" is global: every session gets its own sidebar, and it **follows you**. A tmux hook moves it into whichever window becomes current, so it is always docked on the left, full height, 30 columns wide.
- **Unfocused**, it is a compact overview: `lazymux` and the current page at the top, then every session with its windows (the current session in bold, `▸` marking the window you are in), and an agents section listing every agent, blocked ones first, each with its summary line. The header shows the agent totals (`⣿1 ⣾2 ●1`); sessions carry a badge (`⣿1`) and windows with an agent show its state and age (`⣿ 2m`) instead of the command (see [Agents](#agents)). On the Agents page it shows the agents of the current tab, grouped by session. If the list is longer than the pane, it scrolls so your current window stays visible.
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

**Agents page**: the coding agents in all sessions, their state, and what they wait for. See [Agents](#agents).

**Left column** of the tmux page: three panels, like lazygit.

- **[1] Sessions** lists every session. `●` (green) before the name means a client is attached to it. The number on the right is its window count (`3w`), preceded by an agent badge when the session runs agents: `⣿2` (two blocked), `⣾1` (one working) or `●` (all done).
- **[2] Windows** lists the windows of the selected session: index, name, the state glyph of its most urgent agent, pane count when there is more than one (`2p`), the command running in the active pane, and `*` on the session's active window.
- **[3] Panes** lists the panes of the selected window: pane id (`%12`), command, the agent state (`⣿ blocked`) for agent panes, size, and `*` on the active pane.

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

**`f` (find, also `ctrl+p`)** opens one fuzzy list of every agent (`AGENT`, with its state and what it waits for or works on), session (`SESS`), window (`WIN`) and pane (`PANE`, only for windows with several panes), with the command and directory on the right. Agents come first on the Agents page (blocked first) and last on the tmux page. `enter` goes there; `tab` only selects it (an agent on the Agents page). Type a few letters, e.g. `apicl` for `api:3 claude`.

- `enter` goes there (switches, like `enter` in the panels).
- `tab` only selects it in the panels, so you can act on it (kill, move…) without switching.

## Projects

**`N` (open project)** lists your project directories: zoxide's (most used first), then the git repositories under your project folders. Picking one switches to the session named after the directory (`~/project/lazy.mux` → `lazy_mux`), or creates that session in the directory first. Projects with a session are marked `● session <name>`.

```tmux
set -g @lazymux-project-roots '~/project,~/work'   # where git repos are searched (default: ~/project ~/projects ~/src ~/code ~/work ~/dev)
set -g @lazymux-project-depth 3                    # how deep under a root
set -g @lazymux-project-zoxide off                 # leave zoxide's directories out
```

Repositories are found with `fd` when it is installed, otherwise by walking the folders (skipping hidden folders and `node_modules`).

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

## Agents

lazymux finds AI coding agents in every pane: **Claude Code, Codex, agy (Antigravity CLI), opencode and aider** (and Gemini CLI, which agy replaces). Nothing needs to be set up.

**How it knows.** A pane runs an agent when its command is the agent (`claude`, `codex`…), or when a `node` / `python` / `bun` process in it runs one (npm installs of Gemini CLI and Codex). The state is read from the bottom of the agent's screen every second:

| state | glyph | seen as |
| --- | --- | --- |
| **blocked** | `⣿` red | a permission prompt with a selected first option (`❯ 1. Yes`, `› 1. Yes, proceed`, `● 1. Allow once`); aider's `(Y)es/(N)o` question |
| **working** | `⣾` yellow | `esc to interrupt` / `esc to cancel` in the status line (not in a menu that says `enter to select`), Claude Code's spinner line (`✽ Thinking… (12s · ↓ 1k tokens)`) or its spinning terminal title (`⠂ Fix JWT expiry`); for aider, a screen that changed in the last 3 seconds |
| **done** | `●` green | neither (waiting for a prompt), and it finished while you were not looking at its window |
| **idle** | `○` dim | waiting for a prompt, and you have seen it: its window was on screen, or you selected it on the Agents page |

**Done or idle** works like herdr: an agent that finishes in the window you are looking at becomes idle right away; one that finishes elsewhere stays done until you go to its window (or select it on the Agents page), so `●` always means "finished, not checked yet". The Done tab lists both.

The time next to the state (`blocked 2m`) is how long it has been in that state. Only the bottom of the screen counts, so the same words inside the conversation do not fool it.

**Detection rules.** What each screen shape means is data, not code: every agent has a rules file in [herdr's agent-detection manifest format](https://github.com/herdrdev/herdr/tree/main/src/detect/manifests). When an agent changes its UI and lazymux misreads it, fix the rules without waiting for a new lazymux:

```sh
lazymux rules                       # each agent's rules: bundled or your file
lazymux rules explain %12           # how the rules read pane %12 (default: this pane)
mkdir -p ~/.config/lazymux/agent-detection
lazymux rules show claude > ~/.config/lazymux/agent-detection/claude.toml   # then edit it
```

A file `~/.config/lazymux/agent-detection/<agent>.toml` (`$LAZYMUX_RULES` changes the directory) replaces the bundled rules of that agent and is reread when it changes. herdr's own manifests work as they are: copy `claude.toml`, `codex.toml`, `opencode.toml` or `antigravity.toml` (agy) from herdr. A file that does not parse is ignored, and `lazymux rules` says why.

```toml
[[rules]]
id = "interrupt_hint"
state = "working"                     # idle | working | blocked | unknown
priority = 500                        # the matching rule with the highest priority wins
region = "bottom_non_empty_lines(10)" # where to look; osc_title is the pane title
any = [{ contains = ["esc to interrupt"] }, { contains = ["esc to cancel"] }]
not = [{ contains = ["enter to select"] }]  # a menu waits for you
```

A rule matches when all of its conditions hold: `contains` (every text, case-insensitive), `regex` (every pattern on the region), `line_regex` (every pattern on some line), `all` / `any` / `not` (nested conditions). Regions: `whole_recent`, `bottom_non_empty_lines(N)`, `bottom_lines(N)`, `top_non_empty_lines(N)`, `after_last_horizontal_rule`, `prompt_box_body`, `above_prompt_box`, `last_non_empty_above_prompt_box`, `osc_title`, and herdr's Codex prompt regions. `state = "unknown"` or `skip_state_update = true` keeps the last state (for overlays such as Claude's transcript view). When no rule matches the agent is idle (aider: working while its screen changes).

**The page** (`a`):

```
╭─[1] All 4 │ Blocked 1 │ Working 2 │ Done 1 ─╮╭─ api:3.%12 · claude ──── ⣿ blocked 2m ─╮
│ ▾ api                                    ⣿1 ││                                       │
│ ⣿ claude 3:claude                blocked 2m ││   live screen of the selected agent   │
│   fix jwt expiry · waiting: Bash(go test…)  ││                                       │
│                                             ││                                       │
│ ▾ work                                   ⣾1 ││                                       │
│ ⣾ codex 1:ai                     working 40s││                                       │
│   add table tests for ui package            ││                                       │
╰────────────────────────────────── 1 of 2 ───╯│                                       │
╭─[2] Activity (claude@api:3) ─── 4 events ───╮│                                       │
│ ▸ prompt  fix jwt expiry                    ││                                       │
│   Read    internal/auth/jwt.go              ││                                       │
│   Edit    internal/auth/jwt.go              ││                                       │
│ ⣿ Bash    go test ./internal/auth/…  pending││                                       │
╰─────────────────────────────────────────────╯╰───────────────────────────────────────╯
 1 yes · 2 always · 3 no · i reply · x interrupt  │ enter jump · [ ] tabs · tab panel · a tmux · ? help
```

- **[1]** lists agents grouped by session, sessions with a blocked agent first. Each agent has two lines: glyph, program and window, state and time; then a dim summary (the last prompt, or the title Claude Code gives its terminal) and, when blocked, what it waits for. An agent without a summary shows its directory.
- **Filter** `/`: type to narrow the list; it matches the program, session, window, state, summary, waiting reason and directory (`/go test` finds the agent waiting on `go test`). Folded sessions open while filtering. `enter` keeps the filter, `esc` clears it.
- **Tabs** `[` / `]`: **All**, **Blocked**, **Working**, **Done**, each with a live count. The unfocused sidebar shows the totals in its header (`⣿1 ⣾2 ●1`).
- **Session headers** carry the worst state in the session. `enter` (or `space`) on a header folds or unfolds it.
- **[2] Activity** is the timeline of the selected agent: prompts, tool calls (`Read`, `Edit`, `Bash`…) and `done` when a turn ends. The tool call a blocked agent waits on is tinted red and marked `pending`. It needs hooks (below); `j` / `k` scroll it when it is focused.
- **Right side**: the agent's live pane.
- `tab` / `h` / `l` switch between [1] and [2]. `1` / `2` / `3` do not switch panels here: they answer.

**Acting on an agent:**

| key | action |
| --- | --- |
| `1` / `2` / `3` | answer the permission prompt: yes / yes and don't ask again / no. Only when the agent is blocked. |
| `i` | type a reply; it is sent to the agent's pane followed by `enter` |
| `x` | interrupt the current turn (`esc`; `ctrl-c` for aider) |
| `B` | select the next blocked agent (switches to the All tab and unfolds sessions when needed) |
| `n` | start a new agent: pick one of the agents installed in `$PATH` and a directory (the selected agent's); it starts in a new window of the selected session, typed into the window's shell so the window stays when the agent exits |
| `enter` | go to the agent's pane |

The keys sent per agent: Claude Code and agy get the digit, Gemini CLI too (its "no" is `esc`), Codex gets `y` / `a` / `esc`, opencode `enter` / `a` / `esc`, aider `y` / `a` / `n` and `enter`. Look at the preview before answering: the key goes straight to the agent.

Killing a pane, window or session with an agent in it adds `⣿ ends agents: claude 3:claude (blocked)` to the confirm dialog.

**Hooks (optional).** Without hooks, lazymux knows the state but not the story. With them, each agent reports its prompts, tool calls and permission requests to lazymux as they happen:

```sh
lazymux hooks install            # every installed agent
lazymux hooks install codex agy  # only these
lazymux hooks status
lazymux hooks uninstall          # removes only lazymux's entries
```

| agent | what install writes | events |
| --- | --- | --- |
| Claude Code | `~/.claude/settings.json` (`$CLAUDE_CONFIG_DIR`) | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `Notification`, `Stop`, `SessionEnd` |
| Codex | `~/.codex/hooks.json` and `hooks = true` under `[features]` in `config.toml` (`$CODEX_HOME`) | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `Stop` |
| agy | a `"lazymux"` block in `~/.gemini/config/hooks.json` (`$ANTIGRAVITY_CLI_CONFIG_DIR`) | `SessionStart`, `PreInvocation` (the prompt), `PostToolUse`, `Stop` |
| opencode | the plugin `~/.config/opencode/plugins/lazymux.js` (`$OPENCODE_CONFIG_DIR`) | prompt, tool call, permission asked, session idle |

Aider has no hooks; its state comes from the screen only.

lazymux's entries sit next to the hooks you (or other tools such as herdr) already have, and reinstalling or uninstalling touches only them; each edited file is backed up as `<file>.lazymux.bak` first (the JSON is rewritten with sorted keys). Restart running agents to pick the hooks up. Every hook runs `lazymux hook <agent> <event>`, which writes one line per event to `~/.cache/lazymux/events/<server>-<pane>.jsonl` (the last 200 are kept; a new session starts a new log), and never fails. It only answers where an agent requires it: `{}` for agy and for Codex's `Stop`, `{"decision":"allow"}` for agy's `Stop`. It never hooks agy's `PreToolUse`, whose answer decides whether a tool may run. For agy, whose screen has no reliable "working" line, a turn in the hook log (a prompt or tool call not yet followed by a stop) also counts as working.

## Notifications and status line

While at least one sidebar (or plain `lazymux`) runs, lazymux watches for two changes:

- an agent becomes **blocked**: `claude needs you` · `api:3 · Bash(go test ./...)`
- an agent goes from working to **done**: `claude finished` · `api:3 · fix jwt expiry`

The first time lazymux sees an agent it never notifies. A change waits `@lazymux-notify-delay` seconds (1) and is dropped if the state changes again before that, so a short flicker stays quiet. Only one lazymux per tmux server notifies: the sidebars agree through a lock file (`~/.cache/lazymux/leader-<server>.lock`), and when that one exits, another takes over within a second.

**Where notifications go** (`@lazymux-notify`, one or a comma list):

| value | what you get |
| --- | --- |
| `system` (default) | a desktop notification (`notify-send` on Linux, `osascript` on macOS) |
| `terminal` | a notification through your terminal: `OSC 9` for ghostty, iTerm2 and WezTerm, `OSC 99` for kitty, written to each attached client's terminal (no tmux passthrough needed; works over SSH) |
| `tmux` | a message in the status line of every attached client, for 5 seconds |
| `off` | nothing |

**Sounds** (`@lazymux-sound on|off`, default on): `@lazymux-sound-blocked` and `@lazymux-sound-done` are sound files (defaults: the freedesktop `message-new-instant` and `complete` sounds), played with the first of `paplay`, `pw-play`, `ffplay`, `mpv`, `afplay` that is installed. `LAZYMUX_NO_SOUND=1` silences them.

`@lazymux-notify-on` picks the events (`blocked,done`). All options are read from tmux on every check, so `tmux set -g @lazymux-notify tmux` takes effect within a second. `lazymux notify test` sends a sample with your settings.

**Status line.** `lazymux status` prints `⣿1 ⣾2 ●1` (blocked, working, done-but-unseen; zero counts left out, nothing at all when no agent needs a look) with tmux colors:

```tmux
set -g status-right '#(lazymux status) │ %H:%M'
set -g status-interval 2
```

It reads the summary the notifying lazymux writes every second (`~/.cache/lazymux/status-<server>.json`). When no sidebar runs, it detects once on its own; that shows blocked and working but cannot know done. `lazymux status --plain` prints without colors.

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
| `lazymux hooks install \| uninstall \| status [agent…]` | add / remove lazymux in the hooks of Claude Code, Codex, agy, opencode |
| `lazymux hook <agent> <event>` | record one hook event from stdin (run by the hooks) |
| `lazymux status [--plain]` | agent counts for tmux's `status-right` |
| `lazymux rules [explain [pane] \| show <agent>]` | detection rules per agent; how they read a pane; print the bundled rules |
| `lazymux notify test` | send a sample notification with the current settings |
