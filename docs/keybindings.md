# Keybindings

Inside lazymux, `?` shows this list, lets you filter it with `/`, and runs the selected key with `enter`.

Legend: ✅ available · 🔜 planned (phase in brackets)

## Sidebar (focused)

The focused sidebar is one list (agents, then sessions › windows › panes); keys act on the cursor row. No pages: `a` does nothing there.

| key | action | |
| --- | --- | --- |
| `j` / `k`, `↓` / `↑`, `ctrl+j` / `ctrl+k`, `g` / `G` | move through the whole list | ✅ |
| `tab` | jump between the agents and the sessions | ✅ |
| `l` / `h` | unfold / fold a session or window; on an open one, step into its first child / out to its parent | ✅ |
| `B` | next agent that needs you | ✅ |
| `/` | filter everything; `enter` keeps it, `esc` clears it | ✅ |
| `enter` | agent: jump to it · session / window / pane: switch | ✅ |
| `1` `2` `3`, `i`, `x` | on an agent: answer, reply, stop | ✅ |
| `N` | agent: new agent · session: new session / window · window: new window · pane: split | ✅ |
| `r` `d` `u` `m` `s` `b` `z` `L`, `space` `v` | on sessions, windows, panes: as in the panels below | ✅ |
| `n`, `f`, `?`, `esc` / `q`, `Q` | project, find, help, back to your pane, sidebar off | ✅ |

The rest of this page is plain `lazymux` full screen (panels and the Agents page).

## Navigation

| key | action | |
| --- | --- | --- |
| `j` / `↓` / `ctrl+j` | move down (`ctrl+j` / `ctrl+k` are ↓ / ↑ everywhere: panels, lists, dialogs, pickers) | ✅ |
| `k` / `↑` / `ctrl+k` | move up | ✅ |
| `g` / `home` | first item | ✅ |
| `G` / `end` | last item | ✅ |
| `tab`, `l`, `→` | next panel | ✅ |
| `shift+tab`, `h`, `←` | previous panel | ✅ |
| `1` `2` `3` | jump to panel | ✅ |
| `/` | filter the focused list (fuzzy; `enter` keeps it, `esc` clears it) | ✅ |
| `f` (or `ctrl+p`) | find: jump anywhere, one fuzzy search over all agents, sessions, windows and panes | ✅ |

## Sessions, windows and panes

What a key does depends on the focused panel.

| key | Sessions | Windows | Panes | |
| --- | --- | --- | --- | --- |
| `enter` | switch to session | switch to window | switch to pane | ✅ |
| `n` | open a project (picker) | open a project (picker) | open a project (picker) | ✅ |
| `N` | new session (dialog) | new window (dialog) | split side by side | ✅ |
| `r` | rename session | rename window | set pane title | ✅ |
| `d` | kill selected sessions / cursor (confirm) | kill selected windows / cursor (confirm) | kill selected panes / cursor (confirm) | ✅ |
| `u` | undo last kill (a bulk kill is one step) | ← | ← | ✅ |
| `m` | – | move selected windows / cursor to a session (picker) | join selected panes / cursor into a window (picker) | ✅ |
| `s` | – | swap the one selected window with the cursor | swap the one selected pane with the cursor | ✅ |
| `b` | – | – | break out to its own window | ✅ |
| `z` | – | – | zoom / un-zoom | ✅ |
| `L` | – | next layout | next layout (of its window) | ✅ |

## Selection

| key | action | |
| --- | --- | --- |
| `space` | select / unselect the cursor item (several allowed, one panel at a time) | ✅ |
| `v` | select every item from the last `space` to the cursor | ✅ |
| `esc` | clear the selection, then the filter (then quit / back to your pane) | ✅ |

## Agents page (full screen)

These work on the full-screen Agents page (`a` switches pages; in the sidebar the same answer / reply / stop keys work on agent rows). There, `tab` / `h` / `l` switch between the agent list and Activity; `1` / `2` / `3` answer instead of switching panels. tmux actions (`n`, `d`, `m`…) do nothing on this page.

| key | action | |
| --- | --- | --- |
| `a` | switch page: tmux ⇄ agents (works on both pages) | ✅ |
| `[` / `]` | state tab: All / Blocked / Working / Done | ✅ |
| `/` | filter agents: program, session, window, state, summary, waiting reason, directory | ✅ |
| `j` / `k`, `g` / `G` | move in the agent list (scroll Activity when it is focused) | ✅ |
| `enter` | go to the agent's pane; on a session header: fold / unfold | ✅ |
| `space` | fold / unfold the session header under the cursor | ✅ |
| `1` / `2` / `3` | answer a blocked agent's permission question (yes / yes, don't ask again / no) | ✅ |
| `i` | type a reply and send it to the agent | ✅ |
| `x` | interrupt the agent (`esc`; `ctrl-c` for aider) | ✅ |
| `B` | jump to the next blocked agent | ✅ |
| `n` (Agents page) | start a new agent in a new window (pick the agent and directory) | ✅ |

## Global

| key | action | |
| --- | --- | --- |
| `?` | help / keybindings | ✅ |
| `q`, `ctrl+c` | quit (in the sidebar `q` goes back to your pane instead) | ✅ |
| `esc` | sidebar: back to your pane (clears the selection, then the filter first) | ✅ |
| `Q` | sidebar: turn sidebars off everywhere | ✅ |
| `prefix` + `b` | tmux binding (`lazymux sidebar cycle`): open the sidebar and focus it, focus it, or close it when you are in it; works inside the preview popup too | ✅ |
| `n` / `N` (Agents page) | `n` on the tmux page, `N` on the Agents page (where `n` starts an agent): open a project from zoxide directories and git repos; switches to its session or creates it | ✅ |
| `?` | help; in the sidebar it opens inside the preview popup, where there is room | ✅ |

## Inside dialogs

| dialog | keys |
| --- | --- |
| input (new / rename) | `enter` confirm · `esc` cancel · `tab` complete dir or next field · `ctrl+f` pick the dir with fzf (when installed) · `shift+tab` / `↑` previous field · `←` `→` `space` change the choice row |
| confirm kill | `y` yes · `n` / `esc` / `q` no · `←` `→` `tab` move between choices · `enter` pick the highlighted choice (default: No) |
| picker (`m`, `f`) | type to filter · `↑` `↓` / `ctrl+j` `ctrl+k` move · `enter` pick · `tab` select only (`f`) · `esc` close |
| filter line (`/`) | type to filter · `↑` `↓` move · `enter` keep the filter · `esc` clear it |
| help | `j` / `k` move · `enter` run · `/` filter · `esc` clear filter / close · `q` / `?` close |
