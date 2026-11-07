package tmux

import (
	"os/exec"
	"strings"
	"testing"
)

func TestParseTree(t *testing.T) {
	row := func(f ...string) string { return strings.Join(f, sep) }
	out := strings.Join([]string{
		row("$0", "work", "1", "@0", "1", "editor", "1", "L1", "%0", "0", "1", "nvim", "/home/u", "100", "120", "40", "0", "0", "", "title\twith tab"),
		row("$0", "work", "1", "@0", "1", "editor", "1", "L1", "%1", "1", "0", "zsh", "/home/u", "101", "60", "40", "0", "1", "", ""),
		row("$0", "work", "1", "@1", "2", "shell", "0", "L2", "%2", "0", "1", "zsh", "/tmp", "102", "180", "40", "1", "0", "", ""),
		row("$1", "notes", "0", "@2", "1", "main", "1", "L3", "%3", "0", "1", "claude", "/n", "103", "180", "40", "0", "0", "", "✳ fix bug"),
	}, "\n")
	tr, err := ParseTree(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(tr.Sessions))
	}
	work := tr.Sessions[0]
	if work.Name != "work" || !work.Attached || len(work.Windows) != 2 {
		t.Fatalf("bad work session: %+v", work)
	}
	if got := work.ActiveWindow().Name; got != "editor" {
		t.Errorf("active window = %q", got)
	}
	ed := work.Windows[0]
	if len(ed.Panes) != 2 || ed.ActivePane().ID != "%0" {
		t.Errorf("editor panes wrong: %d active=%s", len(ed.Panes), ed.ActivePane().ID)
	}
	if !ed.Panes[1].Last || ed.Panes[0].Last || !work.Windows[1].Last || ed.Last {
		t.Errorf("last flags wrong")
	}
	if ed.Panes[0].Title != "title\twith tab" {
		t.Errorf("title with separator not preserved: %q", ed.Panes[0].Title)
	}
	if got := tr.Sessions[1].Windows[0].Panes[0].Target(); got != "notes:1.%3" {
		t.Errorf("target = %q", got)
	}
}

func TestTrimCapture(t *testing.T) {
	got := TrimCapture("a\nb\nc\n\x1b[0m  \n\n", 2)
	if strings.Join(got, ",") != "b,c" {
		t.Errorf("got %q", got)
	}
}

// TestIntegration runs against a private tmux server.
func TestIntegration(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	c := &Client{Socket: "lazymux-test"}
	c.Run("kill-server")
	defer c.Run("kill-server")
	// -f is a global flag, so it goes before the command (after -L).
	if _, err := c.Run("-f", "/dev/null", "new-session", "-d", "-s", "alpha", "-x", "100", "-y", "30"); err != nil {
		t.Fatal(err)
	}
	must := func(args ...string) {
		if _, err := c.Run(args...); err != nil {
			t.Fatal(err)
		}
	}
	must("new-window", "-t", "alpha", "-n", "second")
	must("split-window", "-t", "alpha:second")
	must("new-session", "-d", "-s", "beta")
	must("send-keys", "-t", "beta", "echo hello-lazymux", "Enter")

	tr, err := c.ListTree()
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Sessions) != 2 {
		t.Fatalf("sessions = %d", len(tr.Sessions))
	}
	alpha := tr.Sessions[0]
	if alpha.Name != "alpha" || len(alpha.Windows) != 2 || len(alpha.Windows[1].Panes) != 2 {
		t.Fatalf("unexpected alpha: windows=%d", len(alpha.Windows))
	}
	beta := tr.Sessions[1]
	p := beta.ActiveWindow().ActivePane()
	var out string
	for i := 0; i < 20; i++ { // wait for the shell to print
		out, _ = c.CapturePane(p.ID)
		if strings.Contains(out, "hello-lazymux\n") || strings.Count(out, "hello-lazymux") >= 2 {
			break
		}
		exec.Command("sleep", "0.1").Run()
	}
	if !strings.Contains(out, "hello-lazymux") {
		t.Errorf("capture missing output: %q", out)
	}
}

// TestKillAndRestore kills a 2-pane window and a session and restores both.
func TestKillAndRestore(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	c := &Client{Socket: "lazymux-test-undo"}
	c.Run("kill-server")
	defer c.Run("kill-server")
	if _, err := c.Run("-f", "/dev/null", "new-session", "-d", "-s", "keep", "-x", "120", "-y", "40", "-c", "/tmp"); err != nil {
		t.Fatal(err)
	}
	wid, _, err := c.NewWindow("keep", "two", "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Run("split-window", "-t", wid, "-c", "/"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.NewSession("gone", "/tmp"); err != nil {
		t.Fatal(err)
	}

	tr, _ := c.ListTree()
	find := func(tr *Tree, name string) *Session {
		for _, s := range tr.Sessions {
			if s.Name == name {
				return s
			}
		}
		return nil
	}
	keep := find(tr, "keep")
	win := keep.Windows[1]
	wsnap := SnapWindow(win)
	ssnap := SnapSession(find(tr, "gone"))
	if err := c.KillWindow(keep.ID, win.ID); err != nil {
		t.Fatal(err)
	}
	if err := c.KillSession(ssnap.SessionID); err != nil {
		t.Fatal(err)
	}
	if err := c.Restore(wsnap); err != nil {
		t.Fatal("restore window:", err)
	}
	if err := c.Restore(ssnap); err != nil {
		t.Fatal("restore session:", err)
	}

	tr, _ = c.ListTree()
	keep = find(tr, "keep")
	if keep == nil || len(keep.Windows) != 2 {
		t.Fatalf("window not restored: %+v", keep)
	}
	rw := keep.Windows[1]
	if rw.Name != "two" || len(rw.Panes) != 2 || rw.Panes[1].Path != "/" {
		t.Errorf("restored window wrong: name=%s panes=%d", rw.Name, len(rw.Panes))
	}
	if find(tr, "gone") == nil {
		t.Error("session not restored")
	}
}

func TestRerunCommand(t *testing.T) {
	for in, want := range map[string]string{"zsh": "", "nvim": "nvim", "claude": "claude --continue", "node": "", "-zsh": ""} {
		if got := rerunCommand(in); got != want {
			t.Errorf("rerunCommand(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestBreakPaneStaysInSession: the new window must land in the pane's own
// session, not the "current" one.
func TestBreakPaneStaysInSession(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	c := &Client{Socket: "lazymux-test-break"}
	c.Run("kill-server")
	defer c.Run("kill-server")
	if _, err := c.Run("-f", "/dev/null", "new-session", "-d", "-s", "other", "-x", "120", "-y", "40"); err != nil {
		t.Fatal(err)
	}
	sid, err := c.NewSession("target", "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	pid, err := c.SplitPane(sid+":", "/tmp", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.BreakPane(pid, sid); err != nil {
		t.Fatal(err)
	}
	out, _ := c.Run("list-windows", "-t", sid, "-F", "#{window_panes}")
	if out != "1\n1" {
		t.Errorf("target windows = %q, want two 1-pane windows", out)
	}
	if out, _ := c.Run("list-windows", "-t", "other"); strings.Count(out, "\n") != 0 {
		t.Errorf("window leaked into other session: %q", out)
	}
}

// TestSidebarToggleFollowOff: the toggle docks a sidebar on the left, it
// follows the session's current window, and off removes it and the hooks.
func TestSidebarToggleFollowOff(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	sidebarCmdOverride = "sleep 600"
	defer func() { sidebarCmdOverride = "" }()
	c := &Client{Socket: "lazymux-test-sidebar"}
	c.Run("kill-server")
	defer c.Run("kill-server")
	sid, err := c.Run("-f", "/dev/null", "new-session", "-d", "-s", "s", "-x", "120", "-y", "40", "-P", "-F", "#{session_id}")
	if err != nil {
		t.Fatal(err)
	}
	w2, _, err := c.NewWindow(sid, "two", "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	c.Run("select-window", "-t", sid+":0")
	if err := c.SidebarToggle(); err != nil {
		t.Fatal(err)
	}
	where := func() (win string, left, width int) {
		out, _ := c.Run("list-panes", "-a", "-F", "#{window_id}\t#{pane_left}\t#{pane_width}\t#{"+SidebarOpt+"}")
		for _, l := range strings.Split(out, "\n") {
			f := strings.Split(l, "\t")
			if len(f) == 4 && f[3] == sid {
				return f[0], atoi(f[1]), atoi(f[2])
			}
		}
		return "", -1, -1
	}
	win0, _ := c.Run("display-message", "-p", "-t", sid+":0", "#{window_id}")
	if w, left, width := where(); w != win0 || left != 0 || width != SidebarWidth {
		t.Fatalf("after toggle: sidebar in %q left=%d width=%d, want %s left=0 width=%d", w, left, width, win0, SidebarWidth)
	}
	tr, _ := c.ListTree()
	if tr.Sidebars[sid] == nil || len(tr.Sessions[0].Windows[0].Panes) != 1 {
		t.Errorf("tree: sidebar must be in Sidebars and out of Panes")
	}
	c.Run("select-window", "-t", w2)
	if err := c.SidebarFollow(sid); err != nil { // what the hook runs
		t.Fatal(err)
	}
	if w, left, _ := where(); w != w2 || left != 0 {
		t.Errorf("after follow: sidebar in %q left=%d, want %s left=0", w, left, w2)
	}
	if err := c.SidebarOff(); err != nil {
		t.Fatal(err)
	}
	if w, _, _ := where(); w != "" {
		t.Errorf("sidebar still present after off")
	}
	if hooks, _ := c.Run("show-hooks", "-g"); strings.Contains(hooks, "["+hookIndex+"]") {
		t.Errorf("hooks left behind: %s", hooks)
	}
}
