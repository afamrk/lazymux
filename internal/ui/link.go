package ui

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/afamrk/lazymux/internal/tmux"
)

// The sidebar and its preview popup talk over a unix socket: the sidebar
// listens, the popup connects. Messages are JSON lines (wire).
//
//	popup → sidebar   hello / size (popup size), key (every key but the
//	                  popup's own), picked (fzf result), prefix (your tmux
//	                  prefix and the key after it, for tmux to run once the
//	                  popup is closed)
//	sidebar → popup   state (pane to preview, title, help context),
//	                  overlay (the open dialog, rendered for the popup's size),
//	                  fzf (run the directory picker here, where there is room)
type wire struct {
	Kind    string             `json:"k"`
	W       int                `json:"w,omitempty"`
	H       int                `json:"h,omitempty"`
	Key     *tea.Key           `json:"key,omitempty"`
	State   *tmux.PreviewState `json:"state,omitempty"`
	Overlay string             `json:"overlay,omitempty"`
	Query   string             `json:"q,omitempty"`
	Dir     string             `json:"dir,omitempty"`
	Err     string             `json:"err,omitempty"`
	Keys    []string           `json:"keys,omitempty"`
}

// linkDir overrides the socket directory (tests).
var linkDir string

// linkSocket is the socket path for a sidebar pane on a tmux server.
func linkSocket(server, pane string) string {
	dir := linkDir
	if dir == "" {
		dir = os.Getenv("XDG_RUNTIME_DIR")
		if dir == "" {
			dir = os.TempDir()
		}
		dir = filepath.Join(dir, fmt.Sprintf("lazymux-%d", os.Getuid()))
	}
	os.MkdirAll(dir, 0o700)
	return filepath.Join(dir, server+"-"+strings.TrimPrefix(pane, "%")+".sock")
}

// wireConn is one connection: a writer goroutine keeps the order of posted
// messages, a reader goroutine delivers incoming ones on in (closed at EOF).
type wireConn struct {
	conn net.Conn
	out  chan wire
	in   chan wire
	done chan struct{} // the writer stopped
	once sync.Once
}

func newWireConn(c net.Conn) *wireConn {
	w := &wireConn{conn: c, out: make(chan wire, 256), in: make(chan wire, 64), done: make(chan struct{})}
	go func() {
		defer close(w.done)
		enc := json.NewEncoder(c)
		for m := range w.out {
			if enc.Encode(m) != nil {
				w.close()
				return
			}
		}
	}()
	go func() {
		defer close(w.in)
		dec := json.NewDecoder(c)
		for {
			var m wire
			if dec.Decode(&m) != nil {
				w.close()
				return
			}
			w.in <- m
		}
	}()
	return w
}

// post queues a message; it never blocks (a full queue drops it).
func (w *wireConn) post(m wire) {
	defer func() { recover() }() // posted after close
	select {
	case w.out <- m:
	default:
	}
}

// drain sends what is queued, then closes the connection (waiting at most d).
func (w *wireConn) drain(d time.Duration) {
	w.once.Do(func() { close(w.out) })
	select {
	case <-w.done:
	case <-time.After(d):
	}
	w.conn.Close()
}

func (w *wireConn) close() {
	w.once.Do(func() {
		w.conn.Close()
		close(w.out)
	})
}

// wireIn is a message received on conn; bye reports the connection ended.
type wireIn struct {
	conn *wireConn
	m    wire
	bye  bool
}

// waitWire waits for the next message on conn.
func waitWire(conn *wireConn) tea.Cmd {
	return func() tea.Msg {
		m, ok := <-conn.in
		return wireIn{conn: conn, m: m, bye: !ok}
	}
}

// --- sidebar side -------------------------------------------------------------

// linkListener accepts popup connections for a sidebar.
type linkListener struct {
	ln       net.Listener
	accepted chan *wireConn
}

func listenLink(path string) (*linkListener, error) {
	os.Remove(path) // a stale socket from a crashed sidebar
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	l := &linkListener{ln: ln, accepted: make(chan *wireConn, 4)}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			l.accepted <- newWireConn(c)
		}
	}()
	return l, nil
}

// linkUp reports a popup that connected.
type linkUp struct{ conn *wireConn }

func (l *linkListener) wait() tea.Cmd {
	return func() tea.Msg { return linkUp{<-l.accepted} }
}

// Close removes the sidebar's socket (when the program ends).
func (m *Model) Close() {
	if m.link != nil {
		m.link.close()
	}
}

func (l *linkListener) close() {
	path := l.ln.Addr().String()
	l.ln.Close()
	os.Remove(path)
}

// pickDirReq asks for the fzf directory picker; in the sidebar it runs in the
// popup when one is connected.
type pickDirReq struct{ query string }
