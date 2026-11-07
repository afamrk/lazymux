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
| P0 | Panels, live preview, switching, popup mode | ✅ done |
| P1 | New / rename / kill + undo / move / swap / break / zoom / layout, dialogs, help | ✅ done |
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

Run it in any terminal or tmux pane:

```sh
lazymux
```

The recommended way is a tmux popup. Add this to `~/.tmux.conf` and reload (`tmux source ~/.tmux.conf`):

```tmux
bind L display-popup -E -w 90% -h 90% "lazymux --client '#{client_tty}'"
```

Now `prefix` + `L` opens lazymux on top of whatever you are doing. Pick something, press `enter`, and the popup closes with your terminal switched to it.

Outside tmux, `enter` attaches your terminal to the chosen session.

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
| `q` | quit |

Full reference: [docs/keybindings.md](docs/keybindings.md).

## Documentation

- [User guide](docs/guide.md): the layout, every action explained, marks, undo, popup vs. standalone
- [Keybindings](docs/keybindings.md): complete reference, including planned keys
- [Architecture](docs/architecture.md): how the code is organized and why
- [Development](docs/development.md): building, testing, trying changes against a throwaway tmux server
- [Roadmap](docs/roadmap.md): phases, open decisions, backlog
- [Design](docs/design.md): the UI decisions and the Stitch mockups behind them

## Credits

Inspired by [lazygit](https://github.com/jesseduffield/lazygit), [tmux-fzf](https://github.com/sainnhe/tmux-fzf), [agenmux](https://snirt.github.io/agenmux/), [tmux-agent-sidebar](https://github.com/hiroppy/tmux-agent-sidebar) and [herdr](https://github.com/herdrdev/herdr). Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss); colors are [Catppuccin Mocha](https://catppuccin.com).
