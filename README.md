# lazymux

A sidebar for tmux, in the spirit of [herdr](https://github.com/herdrdev/herdr) and [lazygit](https://github.com/jesseduffield/lazygit). It sits on the left of your current window and follows you from window to window. Unfocused, it is a compact list of every session and window. Move into it and it grows into a lazygit-style UI: browse sessions, windows and panes in three panels with a live preview, and create, rename, kill, move, swap, zoom and re-layout things with single keys. An **Agents** page (press `a`) for the AI coding agents running in your sessions (Claude Code, Codex, OpenCode…) is on the way.

Unfocused (30 columns):

```
 lazymux                 tmux │ your panes
                              │
 ▾ api                     4w │
  ▸ 0 editor             nvim │
    1 server               go │
    3 claude           claude │
 ▾ work                    3w │
    0 code               nvim │
    1 ai                codex │
                              │
─ agents ─────────────────────│
 detection arrives in P3      │
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
| P2 | `/` filter, multi-select, jump anywhere (`ctrl+p`) | planned |
| P3 | Agents page (`a`): state tabs All / Blocked / Working / Done, agents grouped by session, blocked / working / idle, answer from lazymux | planned |
| P4 | Desktop notifications, `lazymux status` for the tmux status line | planned |
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

You can still run plain `lazymux` in a terminal, full screen: inside tmux `enter` switches and exits, outside tmux it attaches your terminal to the chosen session.

## Most used keys

| key | action |
| --- | --- |
| `j` / `k` | move down / up |
| `tab`, `h` / `l`, `1` `2` `3` | change panel |
| `enter` | switch to the selection |
| `n` | new session / window / pane (depends on panel) |
| `r` | rename |
| `d` | kill (asks first) · `u` undo |
| `space` then `m` / `s` | mark a target, then move / swap onto it |
| `z` · `b` · `L` | zoom · break pane out · next layout |
| `a` | switch page: tmux ⇄ agents |
| `?` | all keys (filterable, runnable) |
| `esc` / `q` | sidebar: back to your pane (plain lazymux: quit) |
| `Q` | turn the sidebar off |

Full reference: [docs/keybindings.md](docs/keybindings.md).

## Documentation

- [User guide](docs/guide.md): the layout, every action explained, marks, undo, the sidebar
- [Keybindings](docs/keybindings.md): complete reference, including planned keys
- [Architecture](docs/architecture.md): how the code is organized and why
- [Development](docs/development.md): building, testing, trying changes against a throwaway tmux server
- [Roadmap](docs/roadmap.md): phases, open decisions, backlog
- [Design](docs/design.md): the UI decisions and the Stitch mockups behind them

## Credits

Inspired by [lazygit](https://github.com/jesseduffield/lazygit), [tmux-fzf](https://github.com/sainnhe/tmux-fzf), [agenmux](https://snirt.github.io/agenmux/), [tmux-agent-sidebar](https://github.com/hiroppy/tmux-agent-sidebar) and [herdr](https://github.com/herdrdev/herdr). Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss); colors are [Catppuccin Mocha](https://catppuccin.com).
