# lazymux

A sidebar for tmux, in the spirit of [herdr](https://github.com/herdrdev/herdr) and [lazygit](https://github.com/jesseduffield/lazygit). It sits on the left of your current window, 30 columns wide, and follows you from window to window. It finds the AI coding agents running in your panes (Claude Code, Codex, Gemini, agy, opencode, aider) and lists them by state (needs you, working, done, idle) above your sessions and windows. Move into it and it becomes one list with a cursor: answer an agent's permission prompt, reply or stop it; switch to, create, rename, kill, move, swap, zoom and re-layout sessions, windows and panes, all with single keys. The live preview of the row under the cursor, and every dialog, open in a popup on its right, so your panes are never resized.

Unfocused (30 columns):

```
  ◐1   ●1             lazymux │ your panes
──────────────────────────────│
 ◐ NEEDS YOU                1 │
┃✻ claude           api:3  2m │
┃  ? Bash(go test ./...)      │
 ● WORKING                  1 │
┃⬡ codex          work:1  40s │
┃  ▷ add table tests for ui   │
                              │
 SESSIONS ────────────────────│
 ▌api                   3w ◐1 │
   ▸ 0 editor            nvim │
     1 server              go │
     3 claude  ✻ ◐ blocked 2m │
  work                  2w ●1 │
     0 code              nvim │
     1 ai     ⬡ ● working 40s │
                              │
──────────────────────────────│
 2 sessions · 5 windows       │
```

Colors: blocked agents get a red-tinted card, the counts at the top are colored chips, your session and window are highlighted, and each agent has its icon (`✻` Claude Code, `⬡` Codex, `✦` Gemini, `⌬` opencode, `◈` aider, `▲` agy).

Focused (still 30 columns; one list with a cursor, the preview in a popup over your panes):

```
  ◐1   ●1   ✓1        lazymux │╭─ api:3.%12 · claude ───────────────────╮
 / filter…                    ││ (live output of the row under the      │
──────────────────────────────││  cursor: agent, window or pane)        │
 ◐ NEEDS YOU                1 ││                                        │
▶✻ claude           api:3  2m ││                                        │
   ? Bash(go test ./...)      ││                                        │
 ● WORKING                  1 ││                                        │
┃⬡ codex          work:1  40s ││                                        │
┃  ▷ add table tests for ui   ││                                        │
                              ││                                        │
 SESSIONS ────────────────────││                                        │
 ▾ api                  3w ◐1 ││                                        │
     0 editor            nvim ││────────────────────────────────────────│
   ▸ 3 claude  ✻ ◐ blocked 2m ││ 1/2/3 answer · i reply · x stop · …    │
 ▸ work                 2w ●1 │╰────────────────────────────────────────╯
──────────────────────────────│
 1/2/3 answer · i reply       │
 x stop · enter jump · ? keys │
```

No pages and no panels: `j` / `k` run from the agents into the sessions, `tab` jumps between them, `l` / `h` fold, and the keys act on the row under the cursor.

## Status

| Phase | What | Status |
| --- | --- | --- |
| P0 | Panels, live preview, switching | ✅ done |
| P1 | New / rename / kill + undo / move / swap / break / zoom / layout, dialogs, help | ✅ done |
| Sidebar | Docked left pane that follows you; agents by state and a folding session tree; one list with a cursor when focused, keys per row; the preview and dialogs in a popup (never resizes) | ✅ done |
| P2 | `/` filter, multi-select + bulk kill / move, jump anywhere (`f`) | ✅ done |
| P3 | Agents: detection, answer / reply / interrupt; full screen also has the Agents page with state tabs and the Activity timeline from agent hooks | ✅ done |
| P4 | Notifications (desktop / terminal / tmux, sounds) on blocked and done, `lazymux status` for the tmux status line, done = unseen | ✅ done |
| P5 | Project picker (`n`), start a new agent (`N` on an agent row) | ✅ done |

See [docs/roadmap.md](docs/roadmap.md) for details and open decisions.

## Requirements

- tmux 3.2 or newer (tested with 3.4)
- Go 1.26+ to build (the version in `go.mod`)
- A terminal with true color and a Nerd Font is nice but not required

## Install

```sh
go install github.com/afamrk/lazymux/cmd/lazymux@latest
```

This puts `lazymux` in `$(go env GOPATH)/bin` (usually `~/go/bin`); make sure that is on your `PATH`.

Or build from source:

```sh
git clone https://github.com/afamrk/lazymux.git && cd lazymux
go build -o ~/.local/bin/lazymux ./cmd/lazymux
```

After upgrading, run `lazymux sidebar off` and press `prefix` + `b` so the sidebars restart with the new binary.

## Quick start

lazymux lives in a sidebar on the left of your tmux window. Add this to `~/.tmux.conf` and reload (`tmux source ~/.tmux.conf`):

```tmux
set -g focus-events on
bind b run-shell -b "lazymux sidebar cycle '#{session_id}' '#{pane_id}'"
```

Now:

1. `prefix` + `b` opens the sidebar and moves you into it. Pressed again from another pane, it moves you back into the sidebar; pressed while you are in the sidebar, it closes it. One key does all three.
2. You can also move into it like into any pane (`prefix` + `←`, or click it). A cursor appears (on the agent that needs you, else your window), and a popup on its right previews the row under it. Your panes keep their size.
3. Move with `j` / `k` (`tab` jumps between agents and sessions, `l` / `h` unfold / fold), press `enter` to go there, or `esc` to go back to your pane. Either way the popup closes.

`Q` inside the sidebar, or `lazymux sidebar off` from a shell, also closes it.

### Agents

Agents are found with no setup: lazymux looks at each pane's process and reads its screen to tell whether the agent is **blocked** (`◐`, a permission prompt), **working** (`●`), **done** (`✓`, finished and you haven't looked yet) or **idle** (`○`). In the sidebar, on an agent's row, `1` / `2` / `3` answer a prompt, `i` sends a reply, `x` interrupts, and `B` jumps to the next agent that needs you.

Hooks give lazymux the prompt, the exact tool call an agent waits on, and (in full-screen `lazymux`) an Activity timeline. One command adds them to every installed agent (Claude Code, Codex, agy, opencode), next to your existing hooks and with backups:

```sh
lazymux hooks install     # or: lazymux hooks install codex agy
lazymux hooks status
lazymux hooks uninstall
```

Detection rules are data in [herdr's manifest format](https://github.com/herdrdev/herdr): when an agent's UI changes, put a fixed `~/.config/lazymux/agent-detection/<agent>.toml` in place (`lazymux rules show claude` prints the bundled one, `lazymux rules explain` shows how a pane is read). herdr's manifests work as they are.

### Status line and notifications

While a sidebar runs, lazymux tells you when an agent needs you (**Claude needs you** · `api › editor` · `run  go test ./...`) or finishes (**Claude finished · 2m 13s**), with a desktop notification and a sound. It stays quiet about the agent you are looking at, about turns shorter than 10 seconds and right after you interrupt one, and keeps one notification per agent, closing a "needs you" once it is answered. Put the counts in your tmux status line too:

```tmux
set -g status-right '#(lazymux status) │ %H:%M'
set -g status-interval 2
```

Optional settings (defaults shown):

```tmux
set -g @lazymux-notify 'system'          # system, terminal (OSC 9 via ghostty/kitty/iTerm/WezTerm), tmux, off; or a list: 'system,tmux'
set -g @lazymux-notify-on 'blocked,done'
set -g @lazymux-notify-delay 1           # seconds; a state that flips back sooner does not notify
set -g @lazymux-notify-min-work 10       # seconds; shorter turns do not notify "finished"
set -g focus-events on                   # tmux option: lets lazymux notify when your terminal is not in front
set -g @lazymux-sound 'on'
set -g @lazymux-sound-blocked '/usr/share/sounds/freedesktop/stereo/message-new-instant.oga'
set -g @lazymux-sound-done '/usr/share/sounds/freedesktop/stereo/complete.oga'
```

`lazymux notify test` sends a sample notification with your settings.

You can still run plain `lazymux` in a terminal, full screen. It shows the lazygit-style panels (Sessions / Windows / Panes) and an Agents page (`a`) with a live preview on the right; inside tmux `enter` switches and exits, outside tmux it attaches your terminal to the chosen session.

## Most used keys

In the focused sidebar the keys act on the row under the cursor:

| key | action |
| --- | --- |
| `j` / `k` (`ctrl+j` / `ctrl+k`) | move down / up through agents and sessions |
| `tab` · `B` | jump agents ⇄ sessions · next agent that needs you |
| `l` / `h` | unfold / fold a session or window |
| `enter` | jump to the agent / switch to the session, window or pane |
| `1` `2` `3` · `i` · `x` | on an agent: answer yes / always / no · reply · stop |
| `N` | new: agent, session / window, window, or split, by row |
| `r` · `d` · `u` | rename · kill (asks first) · undo |
| `m` · `s` · `z` · `b` · `L` | move / join · swap · zoom · break pane out · next layout |
| `space` / `v` | select items / a range |
| `n` · `f` · `/` | open a project · find anything · filter the list |
| `ctrl+u` / `ctrl+d` | scroll the preview popup |
| `?` | all keys (filterable, runnable) |
| `esc` / `q` · `Q` | back to your pane · turn the sidebar off |

Full reference: [docs/keybindings.md](docs/keybindings.md).

## Documentation

- [User guide](docs/guide.md): the sidebar, every action explained, undo, agents, full-screen mode
- [Keybindings](docs/keybindings.md): complete reference, including planned keys
- [Architecture](docs/architecture.md): how the code is organized and why
- [Development](docs/development.md): building, testing, trying changes against a throwaway tmux server
- [Roadmap](docs/roadmap.md): phases, open decisions, backlog
- [Design](docs/design.md): the UI decisions and the Stitch mockups behind them

## Credits

Inspired by [lazygit](https://github.com/jesseduffield/lazygit), [tmux-fzf](https://github.com/sainnhe/tmux-fzf), [agenmux](https://snirt.github.io/agenmux/), [tmux-agent-sidebar](https://github.com/hiroppy/tmux-agent-sidebar) and [herdr](https://github.com/herdrdev/herdr). Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss); colors are [Catppuccin Mocha](https://catppuccin.com).

## License

[MIT](LICENSE)
