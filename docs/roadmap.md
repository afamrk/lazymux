# Roadmap

## Phases

### ✅ P0: browse and switch
Three panels with cascading selection, fixed content-aware panel sizes, live colored preview refreshed every second, `enter` to switch (client pinning with `--client`, `tmux attach` outside tmux), popup binding (later replaced by the sidebar).

### ✅ Sidebar
`lazymux sidebar toggle` (bind to `prefix` + `b`): one sidebar per session, docked on the left of the current window and following you via tmux hooks; compact overview when unfocused, the full UI at 60% width when focused; re-docks itself after layout changes.

### ✅ P1: manage
New session / window / pane (with an fzf directory picker), rename, kill with a confirm dialog, undo, mark + move / join, swap, break, zoom, layout cycling, toasts, context-aware key bar, filterable and runnable help screen.

### ✅ P2: filter and multi-select
- `/` fuzzy filter in each panel with highlighted matches; `enter` keeps it, `esc` clears it
- `space` selects several items, `v` a range; bulk kill (one undo step), move windows / join panes through a fuzzy target picker, `s` swaps the one selected item with the cursor (replaces the P1 "mark a target" flow)
- `ctrl+p`: one fuzzy search over every session, window and pane; `enter` goes there, `tab` only selects

### ✅ P3: Agents page
- detection with no setup for Claude Code, Codex, Gemini CLI, opencode and aider: the pane's command or process tree gives the agent, the bottom of its screen the state (blocked / working / idle-done)
- optional Claude Code hooks (`lazymux hooks install`): last prompt as summary, the pending tool call as waiting reason, the Activity timeline
- agents grouped by session, blocked sessions first, foldable headers with the worst state (`⣿` / `⣾` / `●`); two lines per agent; `All │ Blocked │ Working │ Done` tabs with live counts
- `1` / `2` / `3` answer a permission prompt, `i` reply, `x` interrupt, `B` next blocked, `enter` go to the pane
- badges on session, window and pane rows and in the compact sidebar (totals `⣿1 ⣾2 ●1` in its header); the kill dialog warns about agents

### P4: notifications and status line
- `lazymux status` prints a compact summary such as `⣿1 ⣾2 ●1` for tmux's `status-right`
- desktop notification (`notify-send`) when an agent goes from working to blocked or done

### P5: projects and worktrees
- `N`: fuzzy project picker (zoxide → fd → configured roots), creates or attaches a session named after the directory
- `n` on the Agents page: start a new agent (moved here from P3: it belongs with the project / worktree flow)
- `w`: create a git worktree, open a window in it, and start the configured agent; `X` tears down window, worktree and branch (with a confirm)

## Open decisions

| question | options | notes |
| --- | --- | --- |
| Undo history | keep in memory (now) · save to `~/.cache/lazymux/undo.json` so it survives restarts · remove undo | The sidebar keeps running, so in-memory undo now covers a whole working session; it is lost when the sidebar is closed. |

## Backlog

Ideas collected from tmux-fzf, agenmux, tmux-agent-sidebar and herdr, roughly by value:

1. **Send keys / broadcast**: type a line and send it to the selected panes
2. **Session snapshots and templates**: save / restore a whole session (windows, layouts, dirs, commands) to YAML; project templates such as "nvim + shell + agent"
3. **Scrollback search** in the preview, copy to a tmux buffer
4. **Process actions**: send SIGINT / TERM / KILL / STOP / CONT to a pane's process, show its process tree
5. **User menu**: custom commands from a config file
6. **Row metadata**: git branch and dirty flag of a pane's directory, last activity, listening ports
7. **Config file** (`~/.config/lazymux/config.yml`): keys, theme, row formats; more themes (gruvbox…)
8. **Event-driven refresh** with tmux control mode (`tmux -C`) instead of polling
9. **Agent extras**: token / cost from hooks, quick approve, per-agent history; hooks for Codex (`notify`) and Gemini CLI; an Activity timeline from the screen for agents without hooks
10. **Mouse**: click to select, wheel to scroll
