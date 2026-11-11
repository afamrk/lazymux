# Roadmap

## Phases

### ✅ P0: browse and switch
Three panels with cascading selection, fixed content-aware panel sizes, live colored preview refreshed every second, `enter` to switch (client pinning with `--client`, `tmux attach` outside tmux), popup binding (later replaced by the sidebar).

### ✅ Sidebar
`lazymux sidebar cycle` (bind to `prefix` + `b`: open + focus → focus → close; prefix keys handed from the popup to tmux): one sidebar per session, docked on the left of the current window and following you via tmux hooks; re-docks itself after layout changes; never resizes.
- one list: agents grouped by state (needs you / working / done / idle) as two-line cards, then the sessions as a folding tree (session › windows › panes); colored count chips, agent icons, current session / window as chips
- focused: a cursor through the whole list, keys per row kind (agent: answer / reply / stop / jump; session, window, pane: switch / new / rename / kill / move / swap / zoom / break / layout), `tab` agents ⇄ sessions, `B` next blocked, `l` / `h` fold, `/` filter; no pages or panels (no `a`)
- the preview of the cursor row and every dialog in a `display-popup` on the right (`lazymux preview`, talking to the sidebar over a unix socket); `ctrl+u` / `ctrl+d` scroll it; fzf runs there too

### ✅ P1: manage
New session / window / pane (with an fzf directory picker), rename, kill with a confirm dialog, undo, mark + move / join, swap, break, zoom, layout cycling, toasts, context-aware key bar, filterable and runnable help screen.

### ✅ P2: filter and multi-select
- `/` fuzzy filter in each panel with highlighted matches; `enter` keeps it, `esc` clears it
- `space` selects several items, `v` a range; bulk kill (one undo step), move windows / join panes through a fuzzy target picker, `s` swaps the one selected item with the cursor (replaces the P1 "mark a target" flow)
- `f` (or `ctrl+p`): one fuzzy search over every agent, session, window and pane; `enter` goes there, `tab` only selects
- `/` filter on the Agents page

### ✅ P3: agents
- detection with no setup for Claude Code, Codex, agy, opencode and aider (and Gemini CLI): the pane's command or process tree gives the agent, the bottom of its screen the state (blocked / working / idle-done)
- optional hooks for Claude Code, Codex, agy and opencode (`lazymux hooks install`, modeled on herdr's integrations): last prompt as summary, the pending tool call as waiting reason, the Activity timeline
- agents grouped by session, blocked sessions first, foldable headers with the worst state (`◐` / `●` / `✓`); two lines per agent; `All │ Blocked │ Working │ Done` tabs with live counts
- `1` / `2` / `3` answer a permission prompt, `i` reply, `x` interrupt, `B` next blocked, `enter` go to the pane
- badges on session, window and pane rows and in the sidebar (totals `◐1 ●2 ✓1` as chips); the kill dialog warns about agents
- the Agents page (agents by session, state tabs, Activity) is full-screen lazymux; the sidebar groups agents by state and has the same keys on agent rows

### ✅ P4: notifications and status line (ideas from herdr's source)
- `lazymux status` prints `◐1 ●2 ✓1` with tmux colors for `status-right`
- notifications when an agent becomes blocked or finishes: desktop (`notify-send` / `osascript`), terminal (`OSC 9` / `OSC 99` straight to the client tty), tmux message, sounds; a 1 s delay drops flickers
- one notifier per server, picked with a lock file; it also writes the status summary
- done means unseen: `✓ done` until you look at the window or select the agent, then `○ idle`
- settings as tmux options (`@lazymux-notify`, `-notify-on`, `-notify-delay`, `-sound`, `-sound-blocked`, `-sound-done`)

### ✅ Detection rules as data (from herdr)
- each agent's detection rules are a TOML manifest in herdr's format, bundled and replaceable in `~/.config/lazymux/agent-detection/`; herdr's own manifests work unchanged
- `not` conditions (an `esc to cancel` menu is not working), regions, priorities, a rule that keeps the last state
- Claude Code's spinning terminal title counts as working
- `lazymux rules`, `lazymux rules explain <pane>`, `lazymux rules show <agent>`

### ✅ P5: projects and new agents
- `n` (full-screen Agents page: `N`): fuzzy project picker (zoxide's directories, then git repositories under `@lazymux-project-roots` found with `fd` or a walk); switches to the session named after the directory, or creates it there
- `N` on an agent row in the sidebar (`n` on the full-screen Agents page): start an installed agent in a new window of the selected session, in the selected agent's directory; selected once detected

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
7. **Config file** (`~/.config/lazymux/config.yml`): keys, theme, row formats; more themes (gruvbox…). Notification settings already live in tmux options (`@lazymux-*`)
11. **Notification extras** (herdr has them): click a notification to jump to the pane, per-agent sound on/off, skip notifications for the window you are looking at
8. **Event-driven refresh** with tmux control mode (`tmux -C`) instead of polling
9. **Agent extras**: token / cost from hooks, quick approve, per-agent history; an Activity timeline from the screen for agents without hooks
10. **Mouse**: click to select, wheel to scroll
