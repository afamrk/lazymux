# Keybindings

Inside lazymux, `?` shows this list, lets you filter it with `/`, and runs the selected key with `enter`.

Legend: ✅ available · 🔜 planned (phase in brackets)

## Navigation

| key | action | |
| --- | --- | --- |
| `j` / `↓` | move down | ✅ |
| `k` / `↑` | move up | ✅ |
| `g` / `home` | first item | ✅ |
| `G` / `end` | last item | ✅ |
| `tab`, `l`, `→` | next panel | ✅ |
| `shift+tab`, `h`, `←` | previous panel | ✅ |
| `1` `2` `3` | jump to panel | ✅ |
| `/` | filter the focused list | 🔜 P2 |
| `ctrl+p` | jump anywhere: fuzzy search over all sessions / windows / panes | 🔜 P2 |

## Sessions, windows and panes

What a key does depends on the focused panel.

| key | Sessions | Windows | Panes | |
| --- | --- | --- | --- | --- |
| `enter` | switch to session | switch to window | switch to pane | ✅ |
| `n` | new session (dialog) | new window (dialog) | split side by side | ✅ |
| `r` | rename session | rename window | set pane title | ✅ |
| `d` | kill session (confirm) | kill window (confirm) | kill pane (confirm) | ✅ |
| `u` | undo last kill | undo last kill | undo last kill | ✅ |
| `m` | – | move to marked session | join into marked window / pane | ✅ |
| `s` | – | swap with marked window | swap with marked pane | ✅ |
| `b` | – | – | break out to its own window | ✅ |
| `z` | – | – | zoom / un-zoom | ✅ |
| `L` | – | next layout | next layout (of its window) | ✅ |

## Selection

| key | action | |
| --- | --- | --- |
| `space` | mark / unmark the selected item (target for `m` / `s`) | ✅ |
| `esc` | clear the mark (quits when nothing is marked) | ✅ |
| `space` (several items) | mark many items for bulk kill / move | 🔜 P2 |
| `v` | mark a range | 🔜 P2 |

## Agents

These work on the Agents page (`a` switches pages). There, `tab` / `h` / `l` switch between the agent list and Activity.

| key | action | |
| --- | --- | --- |
| `a` | switch page: tmux ⇄ agents (works on both pages) | ✅ |
| `[` / `]` | state tab: All / Blocked / Working / Done | ✅ (agents 🔜 P3) |
| `1` / `2` / `3` | answer a blocked agent's permission question (yes / yes, don't ask again / no) | 🔜 P3 |
| `i` | type a reply and send it to the agent | 🔜 P3 |
| `x` | interrupt the agent (sends ctrl-c) | 🔜 P3 |
| `B` | jump to the next blocked agent | 🔜 P3 |
| `n` (Agents page) | start a new agent | 🔜 P3 |
| `w` | new git worktree + window + agent | 🔜 P5 |

## Global

| key | action | |
| --- | --- | --- |
| `?` | help / keybindings | ✅ |
| `q`, `ctrl+c` | quit (in the sidebar `q` goes back to your pane instead) | ✅ |
| `esc` | sidebar: back to your pane (clears a mark first) | ✅ |
| `Q` | sidebar: turn sidebars off everywhere | ✅ |
| `prefix` + `b` | tmux binding: turn the sidebar on / off (`lazymux sidebar toggle`) | ✅ |
| `N` | open a project (sessionizer) | 🔜 P5 |

## Inside dialogs

| dialog | keys |
| --- | --- |
| input (new / rename) | `enter` confirm · `esc` cancel · `tab` complete dir or next field · `ctrl+f` pick the dir with fzf (when installed) · `shift+tab` / `↑` previous field · `←` `→` `space` change the choice row |
| confirm kill | `y` yes · `n` / `esc` / `q` no · `←` `→` `tab` move between choices · `enter` pick the highlighted choice (default: No) |
| help | `j` / `k` move · `enter` run · `/` filter · `esc` clear filter / close · `q` / `?` close |
