# lazymux

A sidebar for tmux, in the spirit of [herdr](https://github.com/herdrdev/herdr) and [lazygit](https://github.com/jesseduffield/lazygit). It sits on the left of your current window and follows you from window to window. Unfocused, it is a compact list of every session and window. Move into it and it grows into a lazygit-style UI: browse sessions, windows and panes in three panels with a live preview, and create, rename, kill, move, swap, zoom and re-layout things with single keys. The **Agents** page (press `a`) finds the AI coding agents running in your panes (Claude Code, Codex, agy, opencode, aider), shows which are blocked, working or done, and lets you answer a permission prompt, reply or interrupt without leaving the sidebar.

Unfocused (30 columns):

```
 lazymux             ⣿1 ⣾1 │ your panes
 ▾ api                     ⣿1 │
  ▸ 0 editor             nvim │
    1 server               go │
    3 claude             ⣿ 2m │
 ▾ work                    ⣾1 │
    0 code               nvim │
    1 ai                ⣾ 40s │
                              │
─ agents ─────────────────────│
 ⣿ claude           api:3  2m │
   fix jwt expiry · Bash(go t…│
 ⣾ codex           work:1 40s │
   add table tests for ui     │
```

Focused (60% of the window):

```
╭─[1] Sessions ──────────────╮╭─ api:3.%12 · claude ──────────────╮│ your
│ ● api                   4w ││ (live output of the selected pane)││ panes,
│   work                  3w ││                                   ││ squeezed
╰─────────────────── 1 of 2 ─╯│                                   ││
╭─[2] Windows (api) ─────────╮│                                   ││
│ 0 editor           nvim *  ││                                   ││
│ 3 claude      2p claude    ││                                   ││
╰─────────────────── 1 of 3 ─╯│                                   ││
╭─[3] Panes (0:editor) ──────╮│                                   ││
│ %2 nvim        120x40 *    ││                                   ││
╰─────────────────── 1 of 1 ─╯╰───────────────────────────────────╯│
 enter switch · esc back · n new · r rename · d kill   tmux │ agents
```

## Status

| Phase | What | Status |
| --- | --- | --- |
| P0 | Panels, live preview, switching | ✅ done |
| P1 | New / rename / kill + undo / move / swap / break / zoom / layout, dialogs, help | ✅ done |
| Sidebar | Docked left pane that follows you; compact when unfocused, full UI at 60% when focused | ✅ done |
| P2 | `/` filter, multi-select + bulk kill / move, jump anywhere (`ctrl+p`) | ✅ done |
| P3 | Agents page (`a`): agents grouped by session under All / Blocked / Working / Done tabs, answer / reply / interrupt, Activity timeline from agent hooks hooks | ✅ done |
| P4 | Notifications (desktop / terminal / tmux, sounds) on blocked and done, `lazymux status` for the tmux status line, done = unseen | ✅ done |
| P5 | Project picker (sessionizer), git worktree + agent spawn | planned |

See [docs/roadmap.md](docs/roadmap.md) for details and open decisions.

## Requirements

- tmux 3.2 or newer (tested with 3.4)
- Go 1.26+ to build (the version in `go.mod`)
- A terminal with true color and a Nerd Font is nice but not required

## Install

```sh
git clone <this repo> lazymux && cd lazymux
go build -o ~/.local/bin/lazymux ./cmd/lazymux
```

Make sure `~/.local/bin` is on your `PATH`. After rebuilding, press `prefix` + `b` twice so the running sidebars restart with the new binary.

## Quick start

lazymux lives in a sidebar on the left of your tmux window. Add this to `~/.tmux.conf` and reload (`tmux source ~/.tmux.conf`):

```tmux
set -g focus-events on
bind b run-shell "lazymux sidebar toggle"
```

Now:

1. `prefix` + `b` opens the sidebar (and closes it again later).
2. Move into it like into any pane (`prefix` + `←`, or click it). It grows to 60% of the window.
3. Pick something with `j` / `k` / `tab` and press `enter` to go there, or `esc` to go back to your pane. Either way the sidebar shrinks back.

`Q` inside the sidebar, or `lazymux sidebar off` from a shell, also closes it.

### Agents

Agents are found with no setup: lazymux looks at each pane's process and reads its screen to tell whether the agent is **blocked** (`⣿`, a permission prompt), **working** (`⣾`), **done** (`●`, finished and you haven't looked yet) or **idle** (`○`). Press `a` for the Agents page; there `1` / `2` / `3` answer a prompt, `i` sends a reply, `x` interrupts and `B` jumps to the next blocked agent.

Hooks give the Agents page the prompt, the exact tool call an agent waits on, and an Activity timeline. One command adds them to every installed agent (Claude Code, Codex, agy, opencode), next to your existing hooks and with backups:

```sh
lazymux hooks install     # or: lazymux hooks install codex agy
lazymux hooks status
lazymux hooks uninstall
```

### Status line and notifications

While a sidebar runs, lazymux tells you when an agent needs you (`⣿ claude needs you: api:3 · Bash(go test…)`) or finishes, with a desktop notification and a sound. Put the counts in your tmux status line too:

```tmux
set -g status-right '#(lazymux status) │ %H:%M'
set -g status-interval 2
```

Optional settings (defaults shown):

```tmux
set -g @lazymux-notify 'system'          # system, terminal (OSC 9 via ghostty/kitty/iTerm/WezTerm), tmux, off; or a list: 'system,tmux'
set -g @lazymux-notify-on 'blocked,done'
set -g @lazymux-notify-delay 1           # seconds; a state that flips back sooner does not notify
set -g @lazymux-sound 'on'
set -g @lazymux-sound-blocked '/usr/share/sounds/freedesktop/stereo/message-new-instant.oga'
set -g @lazymux-sound-done '/usr/share/sounds/freedesktop/stereo/complete.oga'
```

`lazymux notify test` sends a sample notification with your settings.

You can still run plain `lazymux` in a terminal, full screen: inside tmux `enter` switches and exits, outside tmux it attaches your terminal to the chosen session.

## Most used keys

| key | action |
| --- | --- |
| `j` / `k` | move down / up |
| `tab`, `h` / `l`, `1` `2` `3` | change panel |
| `enter` | switch to the selection |
| `n` | new session / window / pane (depends on panel) |
| `r` | rename |
| `d` | kill selection or cursor item (asks first) · `u` undo |
| `/` · `ctrl+p` | filter the panel · jump anywhere |
| `space` / `v` | select items / a range |
| `m` · `s` | move selection to… (picker) · swap selected with cursor |
| `z` · `b` · `L` | zoom · break pane out · next layout |
| `a` | switch page: tmux ⇄ agents |
| `1` `2` `3` · `i` · `x` · `B` | Agents page: answer yes / always / no · reply · interrupt · next blocked |
| `?` | all keys (filterable, runnable) |
| `esc` / `q` | sidebar: back to your pane (plain lazymux: quit) |
| `Q` | turn the sidebar off |

Full reference: [docs/keybindings.md](docs/keybindings.md).

## Documentation

- [User guide](docs/guide.md): the layout, every action explained, undo, the sidebar, agents
- [Keybindings](docs/keybindings.md): complete reference, including planned keys
- [Architecture](docs/architecture.md): how the code is organized and why
- [Development](docs/development.md): building, testing, trying changes against a throwaway tmux server
- [Roadmap](docs/roadmap.md): phases, open decisions, backlog
- [Design](docs/design.md): the UI decisions and the Stitch mockups behind them

## Credits

Inspired by [lazygit](https://github.com/jesseduffield/lazygit), [tmux-fzf](https://github.com/sainnhe/tmux-fzf), [agenmux](https://snirt.github.io/agenmux/), [tmux-agent-sidebar](https://github.com/hiroppy/tmux-agent-sidebar) and [herdr](https://github.com/herdrdev/herdr). Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss); colors are [Catppuccin Mocha](https://catppuccin.com).
