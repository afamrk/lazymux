package ui

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/afamrk/lazymux/internal/agent"
	"github.com/afamrk/lazymux/internal/tmux"
)

func fakeTree(t *testing.T) *tmux.Tree {
	t.Helper()
	row := func(f ...string) string { return strings.Join(f, "\t") }
	tr, err := tmux.ParseTree(strings.Join([]string{
		row("$0", "api", "0", "@0", "1", "editor", "0", "L", "%0", "0", "1", "nvim", "/", "1", "80", "24", "0", "0", "", ""),
		row("$0", "api", "0", "@1", "2", "claude", "1", "L", "%1", "0", "1", "claude", "/", "2", "80", "24", "0", "0", "", ""),
		row("$1", "notes", "0", "@2", "1", "main", "1", "L", "%2", "0", "1", "zsh", "/", "3", "80", "24", "0", "0", "", ""),
		row("$2", "work", "1", "@3", "1", "w", "1", "L", "%3", "0", "0", "zsh", "/", "4", "40", "24", "0", "1", "", ""),
		row("$2", "work", "1", "@3", "1", "w", "1", "L", "%4", "1", "1", "go", "/", "5", "40", "24", "0", "0", "", ""),
	}, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestNavigationCascades(t *testing.T) {
	m := &Model{c: &tmux.Client{}, selSess: "$2"}
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m.Update(treeMsg{tree: fakeTree(t)})
	if m.selSess != "$2" || m.selWin != "@3" || m.selPane != "%4" {
		t.Fatalf("initial sel = %s %s %s", m.selSess, m.selWin, m.selPane)
	}
	m.Update(key("kk")) // two runes in one KeyMsg, as when holding a key
	if m.selSess != "$0" || m.selWin != "@1" || m.selPane != "%1" {
		t.Fatalf("after kk sel = %s %s %s (want api's active window/pane)", m.selSess, m.selWin, m.selPane)
	}
	m.Update(key("2"))
	m.Update(key("k"))
	if m.selWin != "@0" || m.selPane != "%0" {
		t.Fatalf("window move sel = %s %s", m.selWin, m.selPane)
	}
	out := m.View()
	if !strings.Contains(out, "api:1.%0") {
		t.Errorf("preview title missing target:\n%s", out)
	}
}

func newTestModel(t *testing.T) *Model {
	m := &Model{c: &tmux.Client{Socket: "lazymux-unused"}, selSess: "$2"}
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m.Update(treeMsg{tree: fakeTree(t)})
	return m
}

func TestKillDialogDefaultsToNo(t *testing.T) {
	m := newTestModel(t)
	m.Update(key("2")) // windows panel
	m.Update(key("d"))
	if _, ok := m.modal.(*confirmModal); !ok {
		t.Fatalf("d did not open the confirm dialog: %T", m.modal)
	}
	if !strings.Contains(m.View(), "Kill window?") {
		t.Error("dialog not rendered")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // enter on default = No
	if m.modal != nil || cmd != nil {
		t.Fatalf("enter on default choice should close without killing (modal=%v cmd=%v)", m.modal, cmd != nil)
	}
}

func TestHelpRunsSelectedBinding(t *testing.T) {
	m := newTestModel(t)
	m.Update(key("?"))
	h, ok := m.modal.(*helpModal)
	if !ok {
		t.Fatal("? did not open help")
	}
	h.filter.SetValue("rename") // narrow to the r row
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.modal != nil || cmd == nil {
		t.Fatal("enter should close help and dispatch the key")
	}
	m.Update(cmd()) // the dispatched "r"
	if d, ok := m.modal.(*inputModal); !ok || !strings.HasPrefix(d.title, "Rename session") {
		t.Fatalf("expected rename dialog, got %T", m.modal)
	}
}

func TestSwapNeedsOneSelected(t *testing.T) {
	m := newTestModel(t)
	m.Update(key("2"))
	m.Update(key("s"))
	if !strings.Contains(m.toast.text, "select one item") {
		t.Errorf("toast = %q", m.toast.text)
	}
}

func TestFuzzyMatch(t *testing.T) {
	if _, idx, ok := fuzzyMatch("apicl", "api:3 claude"); !ok || len(idx) != 5 || idx[3] != 6 {
		t.Errorf("apicl vs api:3 claude: ok=%v idx=%v", ok, idx)
	}
	if _, _, ok := fuzzyMatch("xyz", "claude"); ok {
		t.Error("xyz must not match claude")
	}
	a, _, _ := fuzzyMatch("cla", "claude")
	b, _, _ := fuzzyMatch("cla", "c-l-a")
	if a <= b {
		t.Errorf("consecutive match should score higher: %d <= %d", a, b)
	}
}

func TestFilterNarrowsAndEscClears(t *testing.T) {
	m := newTestModel(t)
	m.Update(key("1"))
	m.Update(key("/"))
	m.Update(key("no")) // typed into the filter, not run as keys
	if vs := m.visSessions(); len(vs) != 1 || vs[0].Name != "notes" || m.selSess != "$1" {
		t.Fatalf("filter 'no': visible=%d sel=%s", len(vs), m.selSess)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.filtering || !strings.Contains(m.View(), "/ no") {
		t.Fatalf("enter should keep the filter line:\n%s", m.View())
	}
	m.Update(key("j")) // navigation works again, stays inside the filtered list
	if m.selSess != "$1" {
		t.Errorf("j moved outside the filter: %s", m.selSess)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.filter[panelSessions] != "" || len(m.visSessions()) != 3 || m.selSess != "$1" {
		t.Errorf("esc should clear the filter and keep notes selected")
	}
}

func TestSelectRangeAndBulkKillDialog(t *testing.T) {
	m := newTestModel(t)
	m.Update(key("1"))
	m.Update(key("g"))
	m.Update(key(" "))
	m.Update(key("G"))
	m.Update(key("v"))
	if len(m.selected) != 3 {
		t.Fatalf("selected = %d, want 3", len(m.selected))
	}
	if !strings.Contains(m.View(), "3 selected") {
		t.Errorf("footer/bar should show 3 selected")
	}
	m.Update(key("d"))
	d, ok := m.modal.(*confirmModal)
	if !ok || d.title != "Kill 3 sessions?" {
		t.Fatalf("modal = %T %+v", m.modal, m.modal)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // close the dialog
	m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // clear the selection
	if len(m.selected) != 0 {
		t.Errorf("esc should clear the selection")
	}
}

func TestMovePickerListsOtherSessions(t *testing.T) {
	m := newTestModel(t) // work is selected
	m.Update(key("2"))
	m.Update(key("m"))
	p, ok := m.modal.(*pickerModal)
	if !ok {
		t.Fatalf("modal = %T", m.modal)
	}
	if len(p.items) != 2 || p.items[0].text != "api" || p.items[1].text != "notes" {
		t.Fatalf("targets = %+v", p.items)
	}
	p.update(key("no"))
	if len(p.hits) != 1 || p.hits[0].item.text != "notes" {
		t.Errorf("filter 'no' hits = %+v", p.hits)
	}
}

func TestJumpTabSelectsWindow(t *testing.T) {
	m := newTestModel(t)
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	p, ok := m.modal.(*pickerModal)
	if !ok {
		t.Fatalf("modal = %T", m.modal)
	}
	m.Update(key("apicl"))
	if len(p.hits) == 0 || p.hits[0].item.win != "@1" {
		t.Fatalf("first hit = %+v", p.hits)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.modal != nil || m.selSess != "$0" || m.selWin != "@1" || m.focus != panelWindows {
		t.Errorf("tab should select api:2 claude: modal=%v sel=%s %s focus=%d", m.modal, m.selSess, m.selWin, m.focus)
	}
}

func TestPreviewSkipsOwnPane(t *testing.T) {
	m := newTestModel(t)
	m.selfPane = "%4" // lazymux runs in work's active pane
	if p := m.previewTarget(); p == nil || p.ID != "%3" {
		t.Fatalf("sessions focus: preview = %v, want the last pane %%3", p)
	}
	m.Update(key("3"))
	if p := m.previewTarget(); p == nil || p.ID != "%4" {
		t.Fatalf("panes focus: preview = %v, want lazymux's own pane", p)
	}
	if !strings.Contains(m.View(), "lazymux itself") {
		t.Errorf("placeholder missing when the own pane is selected")
	}
}

func TestPanelHeightsIgnoreFocus(t *testing.T) {
	m := &Model{}
	for _, c := range []struct {
		body   int
		counts [numPanels]int
	}{{35, [numPanels]int{3, 4, 1}}, {35, [numPanels]int{30, 2, 1}}, {35, [numPanels]int{30, 30, 30}}, {12, [numPanels]int{9, 9, 9}}} {
		var first [numPanels]int
		for f := 0; f < numPanels; f++ {
			m.focus = f
			hs := m.panelHeights(c.body, c.counts)
			if f == 0 {
				first = hs
			} else if hs != first {
				t.Fatalf("%v: focus %d gives %v, focus 0 gave %v", c, f, hs, first)
			}
			if sum := hs[0] + hs[1] + hs[2]; sum != c.body {
				t.Errorf("%v: heights %v sum to %d", c, hs, sum)
			}
			for i, h := range hs {
				if h < 3 {
					t.Errorf("%v: panel %d height %d", c, i, h)
				}
			}
		}
	}
	// a short sessions list keeps only what it needs when the others are long
	if hs := m.panelHeights(35, [numPanels]int{2, 30, 30}); hs[0] != 4 {
		t.Errorf("tight sessions panel: %v", hs)
	}
}

func TestPickedDirFillsDialog(t *testing.T) {
	m := newTestModel(t)
	m.Update(key("N")) // new session dialog, Name focused
	m.Update(dirPickedMsg{dir: "/home/u/project/my.app"})
	d := m.modal.(*inputModal)
	if got := d.fields[1].in.Value(); got != "/home/u/project/my.app" {
		t.Errorf("dir = %q", got)
	}
	if got := d.fields[0].in.Value(); got != "my_app" {
		t.Errorf("name = %q, want my_app", got)
	}
}

func TestAgentsPageToggle(t *testing.T) {
	m := newTestModel(t)
	m.Update(key("2"))
	selW := m.selWin
	if v := m.View(); !strings.Contains(v, "[1] Sessions") || strings.Contains(v, "Blocked") {
		t.Fatalf("tmux page should show plain Sessions title:\n%s", v)
	}
	m.Update(key("a"))
	if m.page != pageAgents {
		t.Fatal("a did not open the agents page")
	}
	v := m.View()
	for _, want := range []string{"All 0", "Blocked 0", "Working 0", "Done 0", "[2] Activity", "agents"} {
		if !strings.Contains(v, want) {
			t.Errorf("agents page missing %q", want)
		}
	}
	m.Update(key("d"))
	m.Update(key("r"))
	if m.modal != nil {
		t.Fatal("tmux actions must not run on the agents page")
	}
	m.Update(key("]"))
	m.Update(key("]"))
	if m.agentTab != agentWorking {
		t.Errorf("agentTab = %d, want Working", m.agentTab)
	}
	m.Update(key("["))
	if m.agentTab != agentBlocked {
		t.Errorf("agentTab = %d, want Blocked", m.agentTab)
	}
	m.Update(key("a"))
	if m.page != pageTmux || m.focus != panelWindows || m.selWin != selW {
		t.Errorf("back on tmux: page=%d focus=%d win=%s (want windows focus, %s)", m.page, m.focus, m.selWin, selW)
	}
	m.Update(key("]")) // no-op on the tmux page
	if m.agentTab != agentBlocked {
		t.Errorf("[ ] changed the agent tab from the tmux page")
	}
}

func sidebarModel(t *testing.T) *Model {
	t.Helper()
	row := func(f ...string) string { return strings.Join(f, "\t") }
	tr, err := tmux.ParseTree(strings.Join([]string{
		row("$0", "api", "1", "@0", "1", "editor", "1", "L", "%9", "0", "0", "lazymux", "/", "1", "30", "40", "0", "0", "$0", ""),
		row("$0", "api", "1", "@0", "1", "editor", "1", "L", "%0", "1", "1", "nvim", "/", "1", "109", "40", "0", "0", "", ""),
		row("$0", "api", "1", "@1", "2", "server", "0", "L", "%1", "0", "1", "go", "/", "2", "140", "40", "1", "0", "", ""),
		row("$1", "notes", "0", "@2", "1", "main", "1", "L", "%2", "0", "1", "zsh", "/", "3", "140", "40", "0", "0", "", ""),
	}, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	linkDir = t.TempDir() // sockets of popups the tests "open"
	m := &Model{c: &tmux.Client{Socket: "lazymux-unused"}, sidebar: true, selfPane: "%9"}
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 30})
	m.Update(treeMsg{tree: tr})
	return m
}

func TestSidebarCompactView(t *testing.T) {
	m := sidebarModel(t)
	if len(m.tree.Sessions[0].Windows[0].Panes) != 1 {
		t.Fatalf("sidebar pane must not be listed in its window")
	}
	v := m.View()
	for _, want := range []string{"lazymux", "▾ api", "1 editor", "2 server", "▸ notes", " SESSIONS ─", "no agents running", "2 sessions · 3 windows"} {
		if !strings.Contains(v, want) {
			t.Errorf("compact view missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "[1] Sessions") || strings.Contains(v, "1 main") {
		t.Errorf("unfocused sidebar: no panels, other sessions folded:\n%s", v)
	}
}

func TestSidebarStaysNarrow(t *testing.T) {
	m := sidebarModel(t)
	if _, cmd := m.Update(focusMsg{focused: false, paneW: 30, winW: 200}); cmd != nil {
		t.Errorf("a 30-column sidebar must not be resized")
	}
	if _, cmd := m.Update(focusMsg{focused: false, paneW: 120, winW: 200}); cmd == nil {
		t.Errorf("a widened sidebar should be put back to 30 columns")
	}
}

func TestSidebarCompactScrollsToCurrentWindow(t *testing.T) {
	m := sidebarModel(t)
	for h := 6; h <= 14; h++ {
		m.Update(tea.WindowSizeMsg{Width: 30, Height: h})
		v := m.View()
		if !strings.Contains(v, "1 editor") {
			t.Errorf("height %d: current window not visible:\n%s", h, v)
		}
		if !strings.Contains(v, "lazymux") {
			t.Errorf("height %d: header scrolled away", h)
		}
		if n := strings.Count(v, "\n") + 1; n != h {
			t.Errorf("height %d: rendered %d lines", h, n)
		}
	}
}

// agentModel is newTestModel with two agents: claude in api:2 (blocked) and
// a working codex in work:1 (%4).
func agentModel(t *testing.T) *Model {
	t.Helper()
	m := newTestModel(t)
	now := time.Unix(10_000, 0)
	clock = func() time.Time { return now }
	t.Cleanup(func() { clock = time.Now })
	m.Update(agentsMsg{map[string]*agent.Agent{
		"%1": {Pane: "%1", Kind: agent.Claude, State: agent.Blocked, Since: now.Add(-2 * time.Minute),
			Summary: "fix jwt expiry", Waiting: "Bash(go test ./...)", Hooked: true,
			Events: []agent.Event{{T: now, Kind: "prompt", Detail: "fix jwt expiry"}, {T: now, Kind: "tool", Name: "Bash", Detail: "go test ./..."}}},
		"%4": {Pane: "%4", Kind: agent.Codex, State: agent.Working, Since: now.Add(-40 * time.Second)},
	}})
	return m
}

func TestAgentsPageListsGroupedAgents(t *testing.T) {
	m := agentModel(t)
	m.Update(key("a"))
	v := ansi.Strip(m.View())
	for _, want := range []string{
		"All 2", "Blocked 1", "Working 1", "Done 0",
		"▾ api", "◐1", "◐ ✻ claude 2:claude", "blocked 2m", "fix jwt expiry · waiting: Bash(go test ./...)",
		"▾ work", "● ⬡ codex 1:w", "working 40s",
		"[2] Activity (claude@api:2)", "▸ prompt  fix jwt expiry", "◐ Bash    go test ./...  pending",
		"api:2.%1 · claude ", "◐ blocked 2m ─╮",
		"1 yes · 2 always · 3 no · i reply · x interrupt  │ enter jump · [ ] tabs · / filter · f find · tab panel · a tmux",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("agents page missing %q:\n%s", want, v)
		}
	}
	if strings.Index(v, "▾ api") > strings.Index(v, "▾ work") {
		t.Error("the session with a blocked agent should come first")
	}
	if p := m.previewTarget(); p == nil || p.ID != "%1" {
		t.Errorf("preview should show the selected agent's pane, got %v", p)
	}

	m.Update(key("]")) // Blocked tab
	m.Update(key("]")) // Working tab
	v = ansi.Strip(m.View())
	if strings.Contains(v, "◐ ✻ claude") || !strings.Contains(v, "● ⬡ codex") {
		t.Errorf("Working tab should show only codex:\n%s", v)
	}
	if a, _ := m.curAgent(); a == nil || a.Pane != "%4" {
		t.Errorf("selection should move to a visible agent, got %v", a)
	}
}

func TestAgentsPageKeys(t *testing.T) {
	m := agentModel(t)
	m.Update(key("a"))
	m.Update(key("j")) // codex is not blocked
	if a, _ := m.curAgent(); a == nil || a.Pane != "%4" {
		// rows: api header, claude, work header, codex; cursor starts on claude
		m.Update(key("j"))
	}
	if a, _ := m.curAgent(); a == nil || a.Pane != "%4" {
		t.Fatalf("cursor should be on codex, sel=%q", m.agentSel)
	}
	m.Update(key("1"))
	if !m.toast.isErr || !strings.Contains(m.toast.text, "not waiting") {
		t.Errorf("answering a working agent should explain, toast=%q", m.toast.text)
	}
	m.Update(key("B"))
	if a, _ := m.curAgent(); a == nil || a.Pane != "%1" {
		t.Errorf("B should select the blocked agent, sel=%q", m.agentSel)
	}
	if _, cmd := m.Update(key("1")); cmd == nil {
		t.Error("1 on a blocked agent should send the answer")
	}
	m.Update(key("i"))
	if d, ok := m.modal.(*inputModal); !ok || !strings.Contains(d.title, "Reply to claude") {
		t.Fatalf("i should open the reply dialog, got %T", m.modal)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	m.Update(key("k")) // api header
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.collapsed["$0"] || strings.Contains(ansi.Strip(m.View()), "◐ ✻ claude 2:claude") {
		t.Errorf("enter on a header should fold the session")
	}
}

func TestAgentBadgesOnTmuxPage(t *testing.T) {
	m := agentModel(t)
	v := ansi.Strip(m.View())
	for _, want := range []string{"◐1 2w", "●1 1w"} {
		if !strings.Contains(v, want) {
			t.Errorf("tmux page missing badge %q:\n%s", want, v)
		}
	}
	m.Update(key("3"))
	if v := ansi.Strip(m.View()); !strings.Contains(v, "● working") {
		t.Errorf("pane row should show the agent state:\n%s", v)
	}
}

func TestSidebarCompactShowsAgents(t *testing.T) {
	m := agentModel(t)
	m.sidebar, m.selfPane = true, "%9"
	m.setOpen("s:$0", true) // unfold api
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 40})
	v := ansi.Strip(m.View())
	for _, want := range []string{
		"lazymux", " ◐1 ", " ●1 ", // header count chips
		"NEEDS YOU", "WORKING", // agents grouped by state
		"┃✻ claude", "api:2  2m", "┃  ? Bash(go test ./...)", // two-line card: waiting for
		"┃⬡ codex", "work:1  40s",
		" SESSIONS ─", "2 claude", "✻ ◐ blocked 2m", // agent window: state spelled out + age
	} {
		if !strings.Contains(v, want) {
			t.Errorf("compact sidebar missing %q:\n%s", want, v)
		}
	}
	if strings.Index(v, "✻ claude") > strings.Index(v, "⬡ codex") || strings.Index(v, "NEEDS YOU") > strings.Index(v, "WORKING") {
		t.Error("blocked agents should be listed first")
	}
	if strings.Index(v, "WORKING") > strings.Index(v, " SESSIONS ─") {
		t.Error("agents come before the sessions")
	}
}

func TestLeftColumnIsCappedOnWideScreens(t *testing.T) {
	m := agentModel(t)
	m.Update(tea.WindowSizeMsg{Width: 205, Height: 50})
	for _, page := range []string{"tmux", "agents"} {
		first := ansi.Strip(strings.SplitN(m.View(), "\n", 2)[0])
		// the first box ends at its "╮"; the preview box starts right after
		if w := strings.Index(first, "╮"); w < 0 || len([]rune(first[:w])) > maxAgentsLeftW {
			t.Errorf("%s page: left column too wide on a 205-column screen:\n%s", page, first)
		}
		m.Update(key("a"))
	}
}

func TestDoneAgentBecomesIdleWhenSelected(t *testing.T) {
	m := agentModel(t)
	m.agents["%4"].State, m.agents["%4"].Unseen = agent.Idle, true // codex finished while you were away
	m.Update(key("3"))
	if v := ansi.Strip(m.View()); !strings.Contains(v, "✓ done") || !strings.Contains(ansi.Strip(m.compactHeader(30)), " ✓1 ") {
		t.Fatalf("finished agent should show as done (✓ and in the totals):\n%s", v)
	}
	m.Update(key("a")) // Agents page: claude (blocked) is selected first
	if !m.agents["%4"].Unseen {
		t.Fatal("codex is not selected yet, it must stay done")
	}
	m.Update(key("G")) // select codex
	if a, _ := m.curAgent(); a == nil || a.Pane != "%4" || a.Unseen {
		t.Fatalf("selecting the done agent should mark it seen: %+v", a)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "○ ⬡ codex") || !strings.Contains(v, "idle") {
		t.Errorf("seen agent should show as ○ idle:\n%s", v)
	}
}

func TestAgentFilter(t *testing.T) {
	m := agentModel(t)
	m.Update(key("a"))
	m.Update(key("/"))
	m.Update(key("codex"))
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "codex 1:w") || strings.Contains(v, "claude 2:claude") {
		t.Errorf("filter codex should show only codex:\n%s", v)
	}
	if a, _ := m.curAgent(); a == nil || a.Pane != "%4" {
		t.Errorf("selection should move to the match, got %v", a)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // keep it
	if v := ansi.Strip(m.View()); !strings.Contains(v, "/ codex") {
		t.Errorf("kept filter not shown:\n%s", v)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // clear it, stay on the page
	if m.agentFilter != "" || m.page != pageAgents {
		t.Errorf("esc should clear the filter: %q page %v", m.agentFilter, m.page)
	}
	m.Update(key("/"))
	m.Update(key("go test")) // the waiting reason matches too
	if a, _ := m.curAgent(); a == nil || a.Pane != "%1" {
		t.Errorf("waiting reason should match claude, got %v", a)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m.Update(key("/"))
	m.Update(key("zzz"))
	if v := ansi.Strip(m.View()); !strings.Contains(v, "No agents match /zzz.") {
		t.Errorf("no-match message missing:\n%s", v)
	}
}

func TestFindListsAgents(t *testing.T) {
	m := agentModel(t)
	m.Update(key("f"))
	p, ok := m.modal.(*pickerModal)
	if !ok {
		t.Fatalf("f should open jump anywhere, modal = %T", m.modal)
	}
	if !p.items[len(p.items)-1].agent {
		t.Error("on the tmux page agents come after sessions / windows / panes")
	}
	m.Update(key("codex"))
	if len(p.hits) == 0 || !p.hits[0].item.agent || p.hits[0].item.pane != "%4" {
		t.Fatalf("first hit = %+v", p.hits)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.modal != nil || m.page != pageAgents || m.agentSel != "%4" {
		t.Errorf("tab on an agent should select it on the Agents page: page %v sel %q", m.page, m.agentSel)
	}
	m.Update(key("f")) // on the Agents page agents come first, blocked first
	p = m.modal.(*pickerModal)
	if !p.items[0].agent || p.items[0].pane != "%1" {
		t.Errorf("first item = %+v", p.items[0])
	}
}

func TestNewAgentDialog(t *testing.T) {
	m := agentModel(t)
	old := installedAgents
	installedAgents = func() []string { return []string{"claude", "codex"} }
	t.Cleanup(func() { installedAgents = old })
	m.Update(key("a"))
	m.Update(key("n"))
	d, ok := m.modal.(*inputModal)
	if !ok {
		t.Fatalf("n should open the new agent dialog, modal = %T", m.modal)
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"New agent in api", "agent:", "● claude", "○ codex"} {
		if !strings.Contains(v, want) {
			t.Errorf("dialog missing %q:\n%s", want, v)
		}
	}
	if d.focus != len(d.fields) {
		t.Error("the dialog should start on the agent choice")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if d.radioSel != 1 {
		t.Errorf("right should pick codex, radioSel = %d", d.radioSel)
	}
	// once the started agent is detected, it is selected
	m.Update(actionMsg{selAgent: "%9"})
	m.Update(agentsMsg{map[string]*agent.Agent{"%9": {Pane: "%9", Kind: agent.Codex}, "%1": m.agents["%1"]}})
	if m.agentSel != "%9" && m.pendingAgent != "%9" {
		t.Errorf("new agent not selected: sel %q pending %q", m.agentSel, m.pendingAgent)
	}
}

func TestProjectPicker(t *testing.T) {
	m := newTestModel(t)
	m.Update(projectsMsg{[]string{"/home/u/project/api", "/home/u/project/lazy.mux"}})
	p, ok := m.modal.(*pickerModal)
	if !ok {
		t.Fatalf("modal = %T", m.modal)
	}
	if p.items[0].sess == "" || !strings.Contains(p.items[0].detail, "session api") {
		t.Errorf("api has a session: %+v", p.items[0])
	}
	if p.items[1].sess != "" {
		t.Errorf("lazy.mux has none: %+v", p.items[1])
	}
	m.modal = nil
	m.Update(projectsMsg{nil})
	if m.modal != nil || !strings.Contains(m.toast.text, "no projects") {
		t.Errorf("empty list should toast, got %q", m.toast.text)
	}
}

func TestPreviewScroll(t *testing.T) {
	p := NewPreview(&tmux.Client{Socket: "lazymux-unused"}, "")
	p.Update(tea.WindowSizeMsg{Width: 40, Height: 12}) // 10 lines inside the frame
	var lines []string
	for i := 1; i <= 50; i++ {
		lines = append(lines, fmt.Sprintf("line %02d", i))
	}
	setPreview(p, "%1", "api:0.%1 · zsh", "", strings.Join(lines, "\n"))
	if v := ansi.Strip(p.View()); !strings.Contains(v, "line 50") || strings.Contains(v, "line 40") {
		t.Fatalf("should start at the bottom:\n%s", v)
	}
	p.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	if v := ansi.Strip(p.View()); !strings.Contains(v, "line 45") || strings.Contains(v, "line 46") || !strings.Contains(v, "↑ 5 lines") {
		t.Fatalf("ctrl+u should scroll up half a page:\n%s", v)
	}
	for range 20 {
		p.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	}
	if v := ansi.Strip(p.View()); !strings.Contains(v, "line 01") {
		t.Fatalf("ctrl+u stops at the top:\n%s", v)
	}
	for range 20 {
		p.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	}
	if v := ansi.Strip(p.View()); !strings.Contains(v, "line 50") || strings.Contains(v, "↑") {
		t.Fatalf("ctrl+d back to the bottom:\n%s", v)
	}
	p.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	setPreview(p, "%2", "", "", "other")
	if p.off != 0 {
		t.Errorf("another pane should start at the bottom")
	}
}

func TestNOpensNewDialogAndNOpensProjects(t *testing.T) {
	m := sidebarModel(t)
	m.Update(focusMsg{focused: true, paneW: 30})
	if _, cmd := m.Update(key("n")); cmd == nil || m.modal != nil {
		t.Errorf("n on the tmux page should load projects (a command, no dialog yet), modal = %T", m.modal)
	}
	m.Update(key("N"))
	if _, ok := m.modal.(*inputModal); !ok {
		t.Errorf("N should open the new session dialog, modal = %T", m.modal)
	}
}

func TestPreviewHintsFollowPanel(t *testing.T) {
	for ctx, want := range map[string][]string{
		"Session": {"N new", "r rename", "l/h fold"},
		"Window":  {"N new window", "m move", "L layout"},
		"Pane":    {"N split", "z zoom", "b break out"},
		"Agent":   {"1/2/3 answer", "x stop", "B next blocked"},
	} {
		lines := hintLines(listHints[ctx], commonHints, 100, 2)
		h := ansi.Strip(strings.Join(lines, "\n"))
		if len(lines) != 2 || !strings.HasSuffix(ansi.Strip(lines[1]), "ctrl+u/d scroll · ? help · esc close") {
			t.Errorf("%s: want 2 lines ending with the common keys: %s", ctx, h)
		}
		for _, l := range lines {
			if ansi.StringWidth(l) > 100 {
				t.Errorf("%s: hint line too wide: %q", ctx, ansi.Strip(l))
			}
		}
		for _, w := range want {
			if !strings.Contains(h, w) {
				t.Errorf("%s hints missing %q: %s", ctx, w, h)
			}
		}
	}
	if l := hintLines(listHints["Pane"], commonHints, 40, 2); len(l) != 2 || !strings.Contains(ansi.Strip(l[1]), "esc close") {
		t.Errorf("narrow: the common keys must stay: %q", l)
	}

	p := NewPreview(&tmux.Client{Socket: "lazymux-unused"}, "")
	p.Update(tea.WindowSizeMsg{Width: 110, Height: 30})
	var content []string
	for i := 0; i < 80; i++ {
		content = append(content, fmt.Sprintf("line %d", i))
	}
	setPreview(p, "%1", "api:0.%1 · zsh", "Pane", strings.Join(content, "\n"))
	p.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	v := strings.Split(ansi.Strip(p.View()), "\n")
	if len(v) != 30 || !strings.Contains(v[26], "───") || !strings.Contains(v[27], "↑ 14 lines") ||
		!strings.Contains(v[28], "esc close") || strings.Contains(v[29], "esc") {
		t.Errorf("hints and scroll state in the box, plain bottom border:\n%s", strings.Join(v, "\n"))
	}
}

// setPreview gives the popup what the sidebar and a capture would.
func setPreview(p *Preview, pane, title, ctx, content string) {
	p.handleWire(wire{Kind: "state", State: &tmux.PreviewState{Pane: pane, Title: title, Context: ctx}})
	p.Update(captureMsg{pane: pane, content: content})
}

// recv waits for the next message of a kind on conn.
func recv(t *testing.T, conn *wireConn, kind string) wire {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case m, ok := <-conn.in:
			if !ok {
				t.Fatalf("connection closed waiting for %q", kind)
			}
			if m.Kind == kind {
				return m
			}
		case <-timeout:
			t.Fatalf("no %q message", kind)
		}
	}
}

func TestDialogsOpenInPopup(t *testing.T) {
	m := sidebarModel(t)
	m.Update(focusMsg{focused: true, paneW: 30})
	m.popupOpen = true
	a, b := net.Pipe()
	side, pop := newWireConn(a), newWireConn(b)
	defer side.close()
	defer pop.close()
	m.Update(linkUp{conn: side})
	m.Update(wireIn{conn: side, m: wire{Kind: "hello", W: 110, H: 34}})
	if st := recv(t, pop, "state"); st.State == nil || st.State.Pane != "%0" || st.State.Context != "Window" {
		t.Fatalf("the popup should get the preview state, got %+v", st.State)
	}

	k := tea.Key(key("N"))
	m.Update(wireIn{conn: side, m: wire{Kind: "key", Key: &k}}) // a key typed in the popup
	if _, ok := m.modal.(*inputModal); !ok {
		t.Fatalf("N from the popup (cursor on a window) should open the new window dialog, modal = %T", m.modal)
	}
	ov := recv(t, pop, "overlay").Overlay
	for ov == "" { // the empty overlay sent on connect
		ov = recv(t, pop, "overlay").Overlay
	}
	if !strings.Contains(ansi.Strip(ov), "New window") || lipgloss.Width(ov) <= 30 {
		t.Fatalf("the dialog should be drawn for the popup (%d wide):\n%s", lipgloss.Width(ov), ov)
	}
	if strings.Contains(ansi.Strip(m.View()), "New window") {
		t.Errorf("the dialog must not be drawn in the 30-column sidebar")
	}

	p := NewPreview(&tmux.Client{Socket: "lazymux-unused"}, "")
	p.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	p.handleWire(wire{Kind: "overlay", Overlay: ov})
	if !strings.Contains(ansi.Strip(p.View()), "New window") {
		t.Errorf("the popup should draw the dialog")
	}

	m.Update(pickDirReq{query: "proj"}) // ctrl+f in the dialog
	if f := recv(t, pop, "fzf"); f.Query != "proj" {
		t.Errorf("fzf should run in the popup, got query %q", f.Query)
	}

	esc := tea.Key(tea.KeyMsg{Type: tea.KeyEsc})
	m.Update(wireIn{conn: side, m: wire{Kind: "key", Key: &esc}})
	if m.modal != nil || recv(t, pop, "overlay").Overlay != "" {
		t.Errorf("esc should close the dialog in the popup too")
	}
}

func TestCtrlJKAreArrows(t *testing.T) {
	m := sidebarModel(t)
	m.Update(focusMsg{focused: true, paneW: 30})
	m.Update(key("2")) // Windows
	first := m.selWin
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if m.selWin == first {
		t.Fatalf("ctrl+j should move down")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if m.selWin != first {
		t.Errorf("ctrl+k should move back up")
	}

	m.Update(key("?")) // help: a dialog with its own cursor
	h := m.modal.(*helpModal)
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if h.cur != 2 {
		t.Errorf("ctrl+j in the help should move its cursor, cur = %d", h.cur)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if h.cur != 1 {
		t.Errorf("ctrl+k in the help should move up, cur = %d", h.cur)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	m.Update(key("N")) // input dialog: ctrl+j / ctrl+k change fields
	d := m.modal.(*inputModal)
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if d.focus != 1 {
		t.Errorf("ctrl+j in a dialog should go to the next field, focus = %d", d.focus)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if d.focus != 0 {
		t.Errorf("ctrl+k in a dialog should go to the previous field, focus = %d", d.focus)
	}
}

// focusedSidebar is the sidebar model, focused (cursor on the list).
func focusedSidebar(t *testing.T) *Model {
	t.Helper()
	m := sidebarModel(t)
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 40})
	m.Update(focusMsg{focused: true, paneW: 30, winW: 200, session: "$0"})
	return m
}

func TestSidebarFocusShowsListWithoutResizing(t *testing.T) {
	m := sidebarModel(t)
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 40})
	if _, cmd := m.Update(tea.FocusMsg{}); cmd == nil || m.focused {
		t.Fatalf("a focus event should only ask tmux for the focus state")
	}
	m.Update(focusMsg{focused: true, paneW: 30, winW: 200, session: "$0"})
	v := ansi.Strip(m.View())
	if !m.focused || !m.popupOpen || strings.Contains(v, "[1] Sessions") {
		t.Fatalf("focused sidebar: the list (no panels) and the popup:\n%s", v)
	}
	for _, want := range []string{"/ filter…", " SESSIONS ─", "▾ api", "1 editor", "▸ notes", "enter switch"} {
		if !strings.Contains(v, want) {
			t.Errorf("focused list missing %q:\n%s", want, v)
		}
	}
	for _, l := range strings.Split(v, "\n") {
		if ansi.StringWidth(l) > 30 {
			t.Fatalf("the list must fit the 30-column pane, got %d: %q", ansi.StringWidth(l), l)
		}
	}
	if m.listSel != "w:$0:@0" || m.listContext() != "Window" {
		t.Errorf("the cursor should start on your window, got %q (%s)", m.listSel, m.listContext())
	}

	// the popup is told what to show, and follows the cursor
	m.Update(popupMsg{client: "/dev/pts/9"})
	if !strings.HasPrefix(m.published, "%0\t") {
		t.Errorf("published %q, want the editor pane %%0 (never the sidebar itself)", m.published)
	}
	m.Update(key("j"))
	if m.listSel != "w:$0:@1" || !strings.HasPrefix(m.published, "%1\t") {
		t.Errorf("after j: cursor %q, published %q (want the server window, %%1)", m.listSel, m.published)
	}
	if _, cmd := m.Update(switchMsg{}); cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Errorf("enter must not quit the sidebar")
		}
	}
	if m.popupOpen {
		t.Errorf("a switch should close the popup")
	}
	m.Update(focusMsg{focused: false, paneW: 30, winW: 200})
	if m.focused || strings.Contains(ansi.Strip(m.View()), "/ filter…") {
		t.Errorf("blurred sidebar should be the overview")
	}
}

func TestListNavigationAndFolding(t *testing.T) {
	m := focusedSidebar(t)
	m.Update(key("j")) // server
	m.Update(key("j")) // notes (folded)
	if m.listSel != "s:$1" || m.listContext() != "Session" {
		t.Fatalf("j should reach the notes session, got %q", m.listSel)
	}
	m.Update(key("l")) // unfold
	m.Update(key("l")) // into its first window
	if m.listSel != "w:$1:@2" {
		t.Fatalf("l, l should unfold notes and step into 1 main, got %q", m.listSel)
	}
	m.Update(key("h")) // to the parent session
	m.Update(key("h")) // fold it
	if m.listSel != "s:$1" || strings.Contains(ansi.Strip(m.View()), "1 main") {
		t.Errorf("h, h should go up and fold notes, cursor %q", m.listSel)
	}
	m.Update(key("g"))
	if m.listSel != "s:$0" {
		t.Errorf("g should go to the first row (no agents: the api session), got %q", m.listSel)
	}
	if m.page != pageTmux || m.focus != panelSessions || m.selSess != "$0" {
		t.Errorf("the cursor row should drive the selection: page %d focus %d sess %s", m.page, m.focus, m.selSess)
	}
}

func TestListKeysActOnTheRow(t *testing.T) {
	m := focusedSidebar(t)
	m.Update(key("r")) // cursor on the editor window
	d, ok := m.modal.(*inputModal)
	if !ok || !strings.Contains(d.title, "window") {
		t.Fatalf("r on a window row should rename the window, modal %T %+v", m.modal, m.modal)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m.Update(key("a")) // no pages any more
	if m.page != pageTmux || m.listSel != "w:$0:@0" {
		t.Errorf("a must do nothing in the sidebar list")
	}
	m.Update(key("/"))
	for _, r := range "main" {
		m.Update(key(string(r)))
	}
	if !strings.Contains(ansi.Strip(m.View()), "1 main") || m.listSel != "s:$1" {
		t.Errorf("filtering 'main' should show notes › 1 main with the cursor on the first match, got %q:\n%s", m.listSel, ansi.Strip(m.View()))
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.listFilter != "" || m.filtering {
		t.Errorf("esc should clear the filter")
	}
}

func TestListAgents(t *testing.T) {
	m := agentModel(t)
	m.sidebar, m.selfPane = true, "%9"
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 40})
	m.Update(focusMsg{focused: true, paneW: 30, winW: 200, session: "$0"})
	if m.listSel != "a:%1" || m.listContext() != "Agent" {
		t.Fatalf("focus should put the cursor on the agent that needs you, got %q", m.listSel)
	}
	if a, _ := m.curAgent(); a == nil || a.Pane != "%1" {
		t.Fatalf("the cursor should select the agent for the actions")
	}
	if _, cmd := m.Update(key("1")); cmd == nil {
		t.Errorf("1 on a blocked agent should answer it")
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd == nil {
		t.Errorf("enter on an agent should jump to it")
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "▶✻ claude") || !strings.Contains(v, "1/2/3 answer") {
		t.Errorf("cursor card and agent keys missing:\n%s", v)
	}
	m.Update(key("j"))
	if m.listSel != "a:%4" {
		t.Errorf("j should move to the next agent card (codex), got %q", m.listSel)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.listContext() != "Session" {
		t.Errorf("tab should jump to the sessions, got %s", m.listContext())
	}
	m.Update(key("B"))
	if m.listSel != "a:%1" {
		t.Errorf("B should go to the agent that needs you, got %q", m.listSel)
	}
}

func TestTmuxKey(t *testing.T) {
	for _, c := range []struct {
		k    tea.KeyMsg
		want string
	}{
		{tea.KeyMsg{Type: tea.KeyCtrlB}, "C-b"},
		{key("b"), "b"},
		{tea.KeyMsg{Type: tea.KeyUp, Alt: true}, "M-Up"},
		{tea.KeyMsg{Type: tea.KeyLeft}, "Left"},
		{tea.KeyMsg{Type: tea.KeyEnter}, "Enter"},
		{tea.KeyMsg{Type: tea.KeyShiftTab}, "BTab"},
		{tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}, "Space"},
		{tea.KeyMsg{Type: tea.KeyPgDown}, "NPage"},
		{tea.KeyMsg{Type: tea.KeyF5}, "F5"},
	} {
		if got := tmuxKey(c.k); got != c.want {
			t.Errorf("tmuxKey(%q) = %q, want %q", c.k.String(), got, c.want)
		}
	}
}

func TestPrefixKeysGoBackToTmux(t *testing.T) {
	m := sidebarModel(t)
	m.Update(focusMsg{focused: true, paneW: 30})
	m.popupOpen = true
	a, b := net.Pipe()
	side, pop := newWireConn(a), newWireConn(b)
	defer side.close()
	m.Update(linkUp{conn: side})

	p := NewPreview(&tmux.Client{Socket: "lazymux-unused"}, "")
	p.prefixes = []string{"C-b"}
	p.conn = pop
	if _, cmd := p.Update(tea.KeyMsg{Type: tea.KeyCtrlB}); cmd != nil {
		t.Fatalf("the prefix alone should only wait for the next key")
	}
	_, cmd := p.Update(key("b"))
	if cmd == nil {
		t.Fatalf("prefix + b should close the popup")
	}
	got := recv(t, side, "prefix")
	if strings.Join(got.Keys, " ") != "C-b b" {
		t.Fatalf("keys handed over: %v", got.Keys)
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("the popup should quit after handing the keys over")
	}

	m.Update(wireIn{conn: side, m: got})
	if c := m.popupDone(nil, time.Second); c == nil || m.popupOpen || m.replay != nil {
		t.Errorf("the sidebar should send the keys to tmux once the popup closed (open=%v)", m.popupOpen)
	}
	if !m.focused {
		t.Errorf("handing a key to tmux must not leave the sidebar")
	}
}
