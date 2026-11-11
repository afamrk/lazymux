package ui

import (
	"fmt"
	"net"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/afamrk/lazymux/internal/tmux"
)

// The preview popup (`lazymux preview <sidebar-pane> <socket>`) runs in a
// tmux display-popup on the right of a focused sidebar and connects to the
// sidebar over a unix socket (link.go). A popup takes all of the client's
// keys, so every key but ctrl+u / ctrl+d (scrolling) goes to the sidebar,
// which keeps the selection and the dialogs. The sidebar sends back what to
// preview and the open dialog, rendered for the popup's size, which is
// drawn over the preview. The fzf directory picker runs here too.

const (
	previewEvery   = 200 * time.Millisecond
	previewHistory = 2000 // scrollback lines captured for ctrl+u
)

// Preview is the model of the preview popup.
type Preview struct {
	c      *tmux.Client
	socket string
	conn   *wireConn

	st      tmux.PreviewState // from the sidebar
	content string            // capture of st.Pane
	overlay string            // the sidebar's open dialog, "" when none
	w, h    int
	off     int // lines scrolled up from the bottom (ctrl+u / ctrl+d)

	prefixes []string // your tmux prefix keys ("C-b")
	prefixed string   // the prefix key just pressed, waiting for the next key
}

// NewPreview creates the popup model; socket is the sidebar's link.
func NewPreview(c *tmux.Client, socket string) *Preview {
	p := &Preview{c: c, socket: socket}
	for _, o := range []string{"prefix", "prefix2"} {
		if k, err := c.Run("show-options", "-gv", o); err == nil && k != "" && k != "None" {
			p.prefixes = append(p.prefixes, k)
		}
	}
	return p
}

// handOver closes the popup and has the sidebar send keys to tmux once it is
// closed, so your prefix bindings (prefix + b, prefix + arrows…) work while
// the sidebar is focused.
func (p *Preview) handOver(keys []string) tea.Cmd {
	conn := p.conn
	if conn == nil {
		return tea.Quit
	}
	conn.post(wire{Kind: "prefix", Keys: keys})
	return func() tea.Msg {
		conn.drain(500 * time.Millisecond)
		return tea.QuitMsg{}
	}
}

// tmuxKeys names keys the way tmux's send-keys does.
var tmuxKeys = map[string]string{
	"enter": "Enter", "esc": "Escape", "tab": "Tab", "shift+tab": "BTab", "backspace": "BSpace",
	"up": "Up", "down": "Down", "left": "Left", "right": "Right", "pgup": "PPage", "pgdown": "NPage",
	"home": "Home", "end": "End", "delete": "DC", "insert": "IC", " ": "Space",
}

// tmuxKey is the tmux name of a key ("ctrl+b" → "C-b", "alt+up" → "M-Up").
func tmuxKey(k tea.KeyMsg) string {
	s, mod := k.String(), ""
	for _, m := range []struct{ tea, tmux string }{{"ctrl+", "C-"}, {"alt+", "M-"}, {"shift+", "S-"}} {
		if s != "shift+tab" && strings.HasPrefix(s, m.tea) && len(s) > len(m.tea) {
			s, mod = s[len(m.tea):], mod+m.tmux
		}
	}
	if n, ok := tmuxKeys[s]; ok {
		s = n
	} else if len(s) > 1 && s[0] == 'f' && s[1] >= '0' && s[1] <= '9' {
		s = "F" + s[1:]
	}
	return mod + s
}

type (
	previewTickMsg struct{}
	captureMsg     struct{ pane, content string }
	dialedMsg      struct {
		conn *wireConn
		err  error
	}
)

func (p *Preview) Init() tea.Cmd {
	sock := p.socket
	return func() tea.Msg { // the sidebar listens before it opens the popup; retry briefly anyway
		var err error
		for i := 0; i < 20; i++ {
			var c net.Conn
			if c, err = net.Dial("unix", sock); err == nil {
				return dialedMsg{conn: newWireConn(c)}
			}
			time.Sleep(50 * time.Millisecond)
		}
		return dialedMsg{err: err}
	}
}

func (p *Preview) capture() tea.Cmd {
	c, pane := p.c, p.st.Pane
	return func() tea.Msg {
		var out string
		if pane != "" {
			out, _ = c.CaptureHistory(pane, previewHistory)
		}
		return captureMsg{pane, out}
	}
}

func (p *Preview) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.w, p.h = msg.Width, msg.Height
		if p.conn != nil {
			p.conn.post(wire{Kind: "size", W: p.w, H: p.h})
		}
	case dialedMsg:
		if msg.err != nil {
			return p, tea.Quit
		}
		p.conn = msg.conn
		p.conn.post(wire{Kind: "hello", W: p.w, H: p.h})
		return p, tea.Batch(waitWire(p.conn), p.capture())
	case wireIn:
		if msg.bye { // the sidebar closed the link (or is gone)
			return p, tea.Quit
		}
		return p, tea.Batch(waitWire(p.conn), p.handleWire(msg.m))
	case captureMsg:
		if msg.pane == p.st.Pane {
			p.content = msg.content
		}
		return p, tea.Tick(previewEvery, func(time.Time) tea.Msg { return previewTickMsg{} })
	case previewTickMsg:
		return p, p.capture()
	case dirPickedMsg: // fzf ran here for the sidebar's dialog
		m := wire{Kind: "picked", Dir: msg.dir}
		if msg.err != nil {
			m.Err = msg.err.Error()
		}
		p.conn.post(m)
	case tea.KeyMsg:
		if p.prefixed != "" { // prefix + key: tmux's, not ours
			return p, p.handOver([]string{p.prefixed, tmuxKey(msg)})
		}
		for _, k := range p.prefixes {
			if tmuxKey(msg) == k {
				p.prefixed = k
				return p, nil
			}
		}
		if p.overlay == "" { // the popup's own keys: scroll the preview half a page
			switch msg.String() {
			case "ctrl+u":
				p.off += max(1, (p.h-2)/2)
				return p, nil
			case "ctrl+d":
				p.off = max(0, p.off-max(1, (p.h-2)/2))
				return p, nil
			}
		}
		if p.conn != nil {
			k := tea.Key(msg)
			p.conn.post(wire{Kind: "key", Key: &k})
		}
	}
	return p, nil
}

func (p *Preview) handleWire(m wire) tea.Cmd {
	switch m.Kind {
	case "state":
		if m.State != nil {
			if m.State.Pane != p.st.Pane {
				p.off, p.content = 0, "" // another pane: start at the bottom again
			}
			p.st = *m.State
		}
	case "overlay":
		p.overlay = m.Overlay
	case "fzf":
		return pickDirCmd(m.Query)
	}
	return nil
}

func (p *Preview) View() string {
	if p.w == 0 || p.h == 0 {
		return ""
	}
	title := stSub.Render("preview ")
	if p.st.Title != "" {
		target, rest, _ := strings.Cut(p.st.Title, " · ")
		title = stText.Render(target)
		if rest != "" {
			title += stFaint.Render(" · ") + stSub.Render(rest)
		}
		title += " "
	}
	var lines []string
	const hintRows = 2
	showHints := p.h >= 12 // too short: keep the room for the pane
	avail := max(0, p.h-2)
	if showHints {
		avail -= hintRows + 1 // the hints and their divider
	}
	if p.st.Pane != "" {
		all := tmux.TrimCapture(p.content, 0)
		p.off = min(p.off, max(0, len(all)-avail)) // not past the top
		end := len(all) - p.off
		lines = all[max(0, end-avail):end]
	}
	var hints []string
	if showHints {
		keys := listHints[p.st.Context]
		if p.off > 0 {
			keys = append([]kd{{"↑", fmt.Sprintf("%d lines", p.off)}}, keys...)
		}
		hints = hintLines(keys, commonHints, p.w-4, hintRows)
	}
	for i, l := range lines {
		lines[i] = " " + l
	}
	if len(hints) > 0 { // keys of the focused panel, at the bottom
		for len(lines) < avail {
			lines = append(lines, "")
		}
		lines = append(lines, stFaint.Render(strings.Repeat("─", max(0, p.w-2))))
		for _, h := range hints {
			lines = append(lines, " "+h)
		}
	}
	view := renderFrame(title, "", true, lines, p.w, p.h, cRed)
	if p.overlay != "" {
		view = center(dim(view), p.overlay, p.w, p.h)
	}
	return view
}

// commonHints end the last hint line in every context.
var commonHints = []kd{{"ctrl+u/d", "scroll"}, {"?", "help"}, {"esc", "close"}}

// hintLines lays the keys out in at most n lines of w columns, wrapping
// between keys, and ends the last line with tail, which always stays; keys
// that do not fit are dropped from the end.
func hintLines(keys, tail []kd, w, n int) []string {
	sep := stFaint.Render(" · ")
	tailS := strings.Join(renderKeys(tail), sep)
	var lines []string
	cur := ""
	// room on the current line; the last line keeps space for the tail
	room := func() int {
		if len(lines) == n-1 {
			return w - lipgloss.Width(sep+tailS)
		}
		return w
	}
	for _, part := range renderKeys(keys) {
		switch {
		case cur == "" && lipgloss.Width(part) <= room():
			cur = part
		case cur != "" && lipgloss.Width(cur+sep+part) <= room():
			cur += sep + part
		case len(lines) < n-1:
			lines = append(lines, cur)
			cur = ""
			if lipgloss.Width(part) <= room() {
				cur = part
			}
		}
	}
	if cur == "" {
		cur = tailS
	} else {
		cur += sep + tailS
	}
	return append(lines, cur)
}

func renderKeys(keys []kd) []string {
	var out []string
	for _, k := range keys {
		ks := stKey
		if k.k == "↑" {
			ks = stRed
		}
		out = append(out, ks.Render(k.k)+" "+stSub.Render(k.d))
	}
	return out
}
