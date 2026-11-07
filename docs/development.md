# Development

## Build and run

```sh
go build -o ~/.local/bin/lazymux ./cmd/lazymux   # install
go run ./cmd/lazymux                             # run from source against your real tmux server
go vet ./...
gofmt -l .                                       # should print nothing
```

## Tests

```sh
go test ./...
```

| package | tests |
| --- | --- |
| `internal/agent` | `Classify` on idle / working / blocked screens of each agent (Claude Code screens captured from real sessions, a prompt high above the bottom ignored), finding agents in a process tree, hook JSON → events and log reset on a new session, `hooks install` / `uninstall` keeping user hooks, settings and the file mode, the detector's state history (working → done, aider's changing screen) and hook data (summary, waiting reason) |
| `internal/tmux` | `ParseTree` on fixture lines (including a title with a tab), `TrimCapture`, `rerunCommand`, plus **integration tests** against private tmux servers: listing and capturing (`-L lazymux-test`), kill + restore of a window and a session (`-L lazymux-test-undo`), break-pane staying in its session (`-L lazymux-test-break`), sidebar toggle / follow / off with sidebar panes kept out of the tree (`-L lazymux-test-sidebar`, with a `sleep` standing in for the lazymux binary) |
| `internal/ui` | the model driven with key messages on a fake tree: navigation cascade and multi-rune keys, the kill dialog defaulting to No, the help screen running a filtered row, `s` asking for one selected item, fuzzy matching, the `/` filter (typing, enter, esc), `space` + `v` range selection and the bulk kill dialog, the move picker targets, `ctrl+p` + `tab` selecting a window, fixed panel heights, preview skipping the own pane, the fzf pick filling the dialog, the Agents page (grouping, blocked first, tabs, summary and waiting lines, Activity with the pending call, answer / reply / `B` / folding keys, badges on the tmux page); sidebar mode: compact view (all sessions, scrolling to the current window, following the page), focus / blur switching to the full UI, `enter` not quitting |

The integration tests start servers with `-f /dev/null` (so your `~/.tmux.conf` is not loaded) and kill them at the end. They are skipped when `tmux` is not installed.

## Trying the sidebar by hand

Use a separate tmux server in a new terminal, so your real sessions are never touched:

```sh
go build -o /tmp/lazymux ./cmd/lazymux
cat > /tmp/lzdemo.conf <<'EOF2'
set -g focus-events on
set -g mouse on
bind b run-shell "/tmp/lazymux -L lzdemo sidebar toggle"
EOF2
tmux -L lzdemo -f /tmp/lzdemo.conf new-session -s work
```

Inside it, `prefix` + `b` opens the sidebar. Click it or use `prefix` + `←` to focus it, and `esc` to leave. Create windows (`prefix` + `c`) and switch between them to watch it follow. `tmux -L lzdemo kill-server` cleans up.

Focus only counts while a client is attached, so scripted checks with `send-keys` need a real client. Attach one from a second server:

```sh
command tmux -L lzouter -f /dev/null new-session -d -x 140 -y 41 "TMUX= tmux -L lzdemo attach -t work"
command tmux -L lzdemo run-shell "/tmp/lazymux -L lzdemo sidebar toggle"
SB=$(command tmux -L lzdemo list-panes -a -F '#{pane_id} #{@lazymux_sidebar}' | awk '$2!=""{print $1}')
command tmux -L lzdemo select-pane -t $SB           # focus: it widens to 60%
command tmux -L lzdemo list-panes -F '#{pane_id} #{pane_width}'
command tmux -L lzdemo capture-pane -p -t $SB       # what it drew
```

## Trying changes against a throwaway server

For the full-screen UI and its keys, plain `lazymux` is easier to script. Never test destructive keys on your real sessions. Run lazymux inside a separate server and drive it with `send-keys`; `capture-pane` shows exactly what it drew:

```sh
go build -o /tmp/lazymux ./cmd/lazymux
T() { command tmux -L lzdemo "$@"; }

T -f /dev/null new-session -d -s work -n editor -x 140 -y 40 -c /tmp
T new-window -t work -n server -c /tmp
T split-window -t work:server -c /
T new-session -d -s api -x 140 -y 40

# lazymux itself, in a 140×40 "laptop" session, pointed at the same server
T new-session -d -s ui -x 140 -y 40 "/tmp/lazymux -L lzdemo"
sleep 0.8

T send-keys -t ui j 2 j d       # select work, windows panel, server, kill…
T capture-pane -p -t ui         # …shows the confirm dialog
T send-keys -t ui y
T list-windows -t work          # check the effect on tmux itself

T kill-server                   # clean up
```

Use `command tmux` (or the function above) if your shell aliases `tmux`. Add `-e` to `capture-pane` to see the colors as escape codes.

To test client pinning, attach a real client from a second server and pass its tty:

```sh
command tmux -L lzouter -f /dev/null new-session -d "TMUX= tmux -L lzdemo attach -t work"
TTY=$(T list-clients -F '#{client_tty}')
T new-session -d -s ui "/tmp/lazymux -L lzdemo --client $TTY"
T send-keys -t ui g Enter
T list-clients -F '#{client_tty} #{session_name}'   # the client is now on the picked session
```

### Agents without spending tokens

Real agents need a login and a prompt to reach a permission dialog. A fake is enough to test detection and the answer keys: a small compiled program named `claude` (tmux reports the process name, so a shell script would show up as `bash`) that prints a permission prompt, waits for a key, then prints a spinner line and finally an idle prompt. Hook events can be fed by hand; point `LAZYMUX_EVENTS` at a scratch directory so `~/.cache` is not touched:

```sh
export LAZYMUX_EVENTS=/tmp/lzevents
T set-environment -g LAZYMUX_EVENTS /tmp/lzevents      # for the sidebar started by tmux
echo '{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"go test ./..."}}' |
  TMUX="$(T display -p '#{socket_path}'),0,0" TMUX_PANE=%1 /tmp/lazymux hook
```

`lazymux hooks install /tmp/settings.json` tries the installer on a copy instead of your real Claude Code settings.

## Conventions

- tmux interaction lives only in `internal/tmux`; the UI calls typed methods and never builds tmux argv itself.
- Every tmux call from the UI runs in a `tea.Cmd` and reports back with a message.
- Store selections as tmux ids (`$1`, `@3`, `%12`), never as list indices.
- New keys go into `bindings` in `internal/ui/keys.go` so that the help screen stays complete. Mark unfinished ones `soon`.
- New screens are mocked in Stitch and agreed on before they are built (see [design.md](design.md)).

## Dependencies

| module | use |
| --- | --- |
| `charmbracelet/bubbletea` v1 | event loop, key and resize messages |
| `charmbracelet/lipgloss` v1 | styles, joining blocks |
| `charmbracelet/bubbles` v1 | `textinput` in dialogs |
| `charmbracelet/x/ansi` | ANSI-aware width, truncation, stripping |
