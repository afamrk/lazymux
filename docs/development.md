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
| `internal/tmux` | `ParseTree` on fixture lines (including a title with a tab), `TrimCapture`, `rerunCommand`, plus **integration tests** against private tmux servers: listing and capturing (`-L lazymux-test`), kill + restore of a window and a session (`-L lazymux-test-undo`), break-pane staying in its session (`-L lazymux-test-break`) |
| `internal/ui` | the model driven with key messages on a fake tree: navigation cascade and multi-rune keys, the kill dialog defaulting to No, the help screen running a filtered row, the hint shown by `m` without a mark |

The integration tests start servers with `-f /dev/null` (so your `~/.tmux.conf` is not loaded) and kill them at the end. They are skipped when `tmux` is not installed.

## Trying changes against a throwaway server

Never test destructive keys on your real sessions. Run lazymux inside a separate server and drive it with `send-keys`; `capture-pane` shows exactly what it drew:

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
