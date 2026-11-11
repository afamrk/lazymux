# Design

lazymux's screens are mocked in **Google Stitch** before they are built, and built only after they are agreed on. The mockups live in the Stitch project **"lazymux — tmux TUI"**.

## Goal

Control tmux and the coding agents running in it easily, on a laptop screen (about 140×40 characters), and still work at 100×30.

## Decisions

**Sidebar first.** lazymux lives in a herdr-style sidebar pane on the left of the current window (toggled with `prefix` + `b`, one per session, following you between windows). It never resizes: an earlier version widened it to 60% of the window on focus, which reflowed every other pane and was jarring with several splits. The sidebar is **one list**: the agents grouped by state (needs you / working / done / idle), then the sessions as a folding tree. Unfocused it is an overview; focused, a cursor moves through it and the keys act on the row under it, so there are no pages or panels to switch between (the `a` key and the panels felt heavy in 30 columns). The preview of the cursor row and every dialog open in a tmux popup on the right of the sidebar, where there is room. This replaced the popup-only lazymux as the main way to use it; full-screen `lazymux` keeps the lazygit layout below.

**Sidebar look.** Ideas taken from other agent sidebars (tmux-agent-sidebar, herdr, agent-deck, wsx): simple geometric state glyphs with one fixed color each (`◐` blocked red, `●` working yellow, `✓` done green, `○` idle grey) instead of braille; the counts as colored chips in the first line; agents as two-line cards with a state-colored bar (thin when idle), a red-tinted card for an agent that needs you, `?` before what it waits for and `▷` before what it is doing; a small icon per agent program in its own color (`✻` Claude Code orange, `⬡` Codex, `✦` Gemini, `⌬` opencode, `◈` aider, `▲` agy); the current session and window as filled chips; sessions without agents dimmed; the cursor row in a blue-grey background with `▶` on agent cards.

**Full-screen layout: lazygit style.** Four alternatives were mocked and rejected: a single collapsible tree, an agent dashboard grid, a command-palette popup, and ranger-style Miller columns. The lazygit layout won, with changes for small screens:

- **Two pages instead of 4 panels.** The tmux page has 3 panels (Sessions / Windows / Panes). Agents live on their own page with [1] the agent list and [2] Activity; `a` switches pages and a `tmux │ agents` indicator sits at the right of the key bar. (Agents were first a tab of panel [1]; the user preferred a separate page.)
- **State tabs on the Agents page**: `All │ Blocked │ Working │ Done` with counts, switched with `[` / `]` like lazygit. Idle is folded into Done (the row says "idle" until the agent has worked once, "done" after); there is no error tab because agents do not report errors reliably.
- **Fixed, content-aware sizing.** Panels share the column equally; a short list keeps only the rows it needs and gives the rest away. Sizes do not change with focus (an accordion was tried in P0 and dropped after use). The Agents page uses a wider left column (36%) because its rows carry a summary line. The left column is capped (60 columns, 64 on the Agents page) so on wide screens the preview grows instead, keeping the mockups' proportions. No italics: terminals set up as `screen-256color` draw them as reverse video.
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
| done (finished, unseen) agents | green `#a6e3a1` `✓`; seen idle agents are dim `○` |
| tinted backgrounds: blocked card / chips | red `#3b2a35`, yellow `#3a3631`, green `#2c3a32` |
| sidebar cursor row | `#3e4566` |
| agent icons | Claude peach `#fab387`, Codex teal `#94e2d5`, Gemini blue, opencode sky `#89dceb`, aider green, agy mauve |
| selection `◆`, active `*`, help border, jump palette | mauve `#cba6f7` |
| filter matches | peach `#fab387` bold |
| pane ids, help group headers | peach `#fab387` |
| text / dim text / faint | `#cdd6f4` / `#a6adc8` / `#6c7086` |
| selection background | `#313244` (focused), `#2a2b3d` (unfocused) |

## Mockups

| screen | Stitch title | status |
| --- | --- | --- |
| main view | lazymux - Terminal TUI (Laptop 1280x800) | built (P0) |
| Agents tab, grouped, preview only | lazymux - Grouped Agents & Full Terminal Preview | superseded by the Agents page |
| Agents page with state tabs | lazymux - Agents Page with State Tabs (Laptop 1280x800) | built (P3) |
| new session / rename dialogs | lazymux - Input Dialogs (Laptop 1280x800) | built (P1) |
| confirm kill + undo toast | lazymux - Confirm Kill Dialog & Undo Toast | built (P1) |
| help | lazymux - Keybindings Help Modal | built (P1) |
| sidebar, unfocused | lazymux - Sidebar (Laptop 1280x800) | built |
| sidebar, focused at 60% | lazymux - Sidebar Focused (60%) (Laptop 1280x800) | replaced: one list with a cursor in 30 columns + preview popup |
| filter, multi-select, move picker | lazymux - Filter, Multi-select & Move Picker (Laptop 1280x800) | built (P2) |
| jump anywhere | lazymux - Jump Anywhere (f, was ctrl+p) (Laptop 1280x800) | built (P2) |
| project picker, new agent | – | to design (P5) |

Stitch sometimes changed the background behind dialogs (adding a top bar, dropping panel borders). Only the dialogs themselves are the design; in the app they float over the normal main view.

## Differences between mockups and the app

- The main-view mockup still shows the old `Sessions │ Agents` tab title; the app shows `[1] Sessions` and the `tmux │ agents` indicator.
- The Agents page mockup adds a cwd / PID line and a styled permission box to the preview; the app shows the raw pane only.
- The sidebar lists the agents first, grouped by state, as two-line cards with a state-colored bar, then the sessions as a folding tree (the mockup lists agents last, ungrouped). Only the sidebar's own session starts unfolded. The footer counts sessions and windows when unfocused and shows the cursor row's keys when focused.
- The P2 filter mockup has an invented top bar and tab strip; the app has none. Selected rows use the existing one-column gutter for `◆`.
- On the Agents page, session badges are plain colored text, not pills; the selected agent has no left accent bar (the rest of the app highlights rows with a background only); the list footer has no `[j/k] move` hint; an agent without a summary shows its directory on the second line.
- The confirm dialog lists agents (`◐ ends agents: claude 3:claude (blocked)`) and running programs (`running: go, nvim`).
- The new-window dialog has no "start in" choice: it always creates a window in the selected session.
