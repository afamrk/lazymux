# Design

lazymux's screens are mocked in **Google Stitch** before they are built, and built only after they are agreed on. The mockups live in the Stitch project **"lazymux — tmux TUI"**.

## Goal

Control tmux and the coding agents running in it easily, on a laptop screen (about 140×40 characters), and still work at 100×30.

## Decisions

**Layout: lazygit style.** Four alternatives were mocked and rejected: a single collapsible tree, an agent dashboard grid, a command-palette popup, and ranger-style Miller columns. The lazygit layout won, with changes for small screens:

- **Two pages instead of 4 panels.** The tmux page has 3 panels (Sessions / Windows / Panes). Agents live on their own page with [1] the agent list and [2] Activity; `a` switches pages and a `tmux │ agents` indicator sits at the right of the key bar. (Agents were first a tab of panel [1]; the user preferred a separate page.)
- **State tabs on the Agents page**: `All │ Blocked │ Working │ Done` with counts, switched with `[` / `]` like lazygit. Idle is folded into Done; there is no error tab because agents do not report errors reliably.
- **Fixed, content-aware sizing.** Panels share the column equally; a short list keeps only the rows it needs and gives the rest away. Sizes do not change with focus (an accordion was tried in P0 and dropped after use). The Agents page uses a wider left column (36%) because its rows carry a summary line.
- **Preview only on the right.** No info sheet (process, tokens, state…) and no `Output │ Info` tabs, just the live pane output with a one-line title in the border.
- **Context-aware bottom bar.** The hints change per panel and state; a blocked agent puts its answer keys first.
- **Agents grouped by session**, each with a one-line summary of what it is doing.

**Look.**

- Catppuccin Mocha palette; the terminal's own background is kept
- thin rounded box-drawing borders with the title in the top line and a counter (`3 of 4`) or key hints in the bottom line
- focused panel: blue border and bold highlighted row; unfocused panels dimmed
- dialogs float over a dimmed screen: **blue** for input, **red** for destructive confirms, **mauve** for help, **green** / **red** toasts for results / errors

**Colors in use.**

| role | color |
| --- | --- |
| focus, keys, window numbers | blue `#89b4fa` |
| attached session, success | green `#a6e3a1` |
| danger, errors, blocked agents | red `#f38ba8` |
| working agents | yellow `#f9e2af` |
| marks, active `*`, help border | mauve `#cba6f7` |
| pane ids, help group headers | peach `#fab387` |
| text / dim text / faint | `#cdd6f4` / `#a6adc8` / `#6c7086` |
| selection background | `#313244` (focused), `#2a2b3d` (unfocused) |

## Mockups

| screen | Stitch title | status |
| --- | --- | --- |
| main view | lazymux - Terminal TUI (Laptop 1280x800) | built (P0) |
| Agents tab, grouped, preview only | lazymux - Grouped Agents & Full Terminal Preview | superseded by the Agents page |
| Agents page with state tabs | lazymux - Agents Page with State Tabs (Laptop 1280x800) | approved; page switch built, content P3 |
| new session / rename dialogs | lazymux - Input Dialogs (Laptop 1280x800) | built (P1) |
| confirm kill + undo toast | lazymux - Confirm Kill Dialog & Undo Toast | built (P1) |
| help | lazymux - Keybindings Help Modal | built (P1) |
| filter / multi-select | – | to design (P2) |
| project picker / worktree | – | to design (P5) |

Stitch sometimes changed the background behind dialogs (adding a top bar, dropping panel borders). Only the dialogs themselves are the design; in the app they float over the normal main view.

## Differences between mockups and the app

- The main-view mockup still shows the old `Sessions │ Agents` tab title; the app shows `[1] Sessions` and the `tmux │ agents` indicator.
- The Agents page mockup adds a cwd / PID line and a styled permission box to the preview; the app shows the raw pane only.
- The mockups show agent badges (`⣿1`, `blocked 2m`) on session and window rows; they arrive with agent detection in P3.
- The confirm dialog lists running programs (`running: go, nvim`) where the mockup showed an agent warning; the agent-specific warning comes in P3.
- The new-window dialog has no "start in" choice: it always creates a window in the selected session.
