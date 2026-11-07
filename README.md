# lazymux

A lazygit-style terminal UI for tmux. Browse every session, window and pane in three panels, watch a live preview of the selected pane, and create, rename, kill, move, swap, zoom and re-layout things with single keys. An **Agents** page (press `a`) for the AI coding agents running in your sessions (Claude Code, Codex, OpenCode…) is on the way.

```
╭─[1] Sessions ────────────────╮╭─ api:1.%4 · claude ──────────────────────────────────────╮
│  ● work                  3w  ││ (live output of the selected pane, with colors)          │
│    dotfiles              1w  ││                                                          │
│◆   api                   2w  ││                                                          │
╰──────────────────── 3 of 3 ─╯│                                                          │
╭─[2] Windows (api) ──────────╮│                                                          │
│  0 editor          nvim *   ││                                                          │
│  1 claude       2p claude   ││                                                          │
╰──────────────────── 1 of 2 ─╯│                                                          │
╭─[3] Panes (0:editor) ───────╮│                                                          │
│  %2 nvim         120x40 *   ││                                                          │
╰──────────────────── 1 of 1 ─╯╰──────────────────────────────────────────────────────────╯
 ◆ session api · enter switch · n new · r rename · d kill · m move · ? help   tmux │ agents
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

Make sure `~/.local/bin` is on your `PATH`.

## Quick start

lazymux lives in a sidebar on the left of your tmux window. Add this to `~/.tmux.conf` and reload (`tmux source ~/.tmux.conf`):

```tmux
set -g focus-events on
bind b run-shell "lazymux sidebar toggle"
```

Now `prefix` + `b` turns the sidebar on or off. It follows you to whatever window you switch to. Unfocused it is a 30-column overview of your sessions and windows; move into it (`prefix` + `←` or a click) and it grows to 60% of the window with the full UI. `esc` takes you back to your pane.

You can also run plain `lazymux` in any terminal: inside tmux `enter` switches and exits, outside tmux it attaches your terminal to the chosen session.

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
