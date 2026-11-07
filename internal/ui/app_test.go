package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
	m.Update(key("n")) // new session dialog, Name focused
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
	m.Update(key("n"))
	m.Update(key("d"))
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
	for _, want := range []string{"lazymux", "▾ api", "▸ 1 editor", "2 server", "▾ notes", "1 main", "agents"} {
		if !strings.Contains(v, want) {
			t.Errorf("compact view missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "[1] Sessions") {
		t.Errorf("unfocused sidebar must not show the full UI")
	}
}

func TestSidebarFocusShowsFullUIAndEnterStays(t *testing.T) {
	m := sidebarModel(t)
	m.Update(tea.FocusMsg{})
	m.Update(tea.WindowSizeMsg{Width: 84, Height: 40})
	if !m.focused || !strings.Contains(m.View(), "[1] Sessions") {
		t.Fatalf("focused sidebar should show the full UI:\n%s", m.View())
	}
	if _, cmd := m.Update(switchMsg{}); cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Errorf("enter must not quit the sidebar")
		}
	}
	m.Update(tea.BlurMsg{})
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 40})
	if m.focused || strings.Contains(m.View(), "[1] Sessions") {
		t.Errorf("blurred sidebar should be compact")
	}
}

func TestSidebarCompactFollowsPage(t *testing.T) {
	m := sidebarModel(t)
	m.Update(tea.FocusMsg{})
	m.Update(key("a")) // Agents page
	m.Update(key("]")) // Blocked tab
	m.Update(tea.BlurMsg{})
	v := m.View()
	for _, want := range []string{"lazymux", "─ blocked ─", "no agents running"} {
		if !strings.Contains(ansi.Strip(v), want) {
			t.Errorf("compact agents view missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "▾ api") {
		t.Errorf("compact view on the Agents page must not show the tmux tree:\n%s", v)
	}
	m.Update(tea.FocusMsg{})
	m.Update(key("a")) // back to tmux
	m.Update(tea.BlurMsg{})
	if !strings.Contains(m.View(), "▾ api") {
		t.Errorf("compact view on the tmux page should show the tree")
	}
}

func TestSidebarCompactScrollsToCurrentWindow(t *testing.T) {
	m := sidebarModel(t)
	for h := 6; h <= 14; h++ {
		m.Update(tea.WindowSizeMsg{Width: 30, Height: h})
		v := m.View()
		if !strings.Contains(v, "▸ 1 editor") {
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
		"▾ api", "⣿1", "⣿ claude 2:claude", "blocked 2m", "fix jwt expiry · waiting: Bash(go test ./...)",
		"▾ work", "⣾ codex 1:w", "working 40s",
		"[2] Activity (claude@api:2)", "▸ prompt  fix jwt expiry", "⣿ Bash    go test ./...  pending",
		"api:2.%1 · claude ", "⣿ blocked 2m ─╮",
		"1 yes · 2 always · 3 no · i reply · x interrupt  │ enter jump · [ ] tabs · tab panel · a tmux",
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
	if strings.Contains(v, "⣿ claude") || !strings.Contains(v, "⣾ codex") {
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
	if !m.collapsed["$0"] || strings.Contains(ansi.Strip(m.View()), "⣿ claude 2:claude") {
		t.Errorf("enter on a header should fold the session")
	}
}

func TestAgentBadgesOnTmuxPage(t *testing.T) {
	m := agentModel(t)
	v := ansi.Strip(m.View())
	for _, want := range []string{"⣿1 2w", "⣾1 1w"} {
		if !strings.Contains(v, want) {
			t.Errorf("tmux page missing badge %q:\n%s", want, v)
		}
	}
	m.Update(key("3"))
	if v := ansi.Strip(m.View()); !strings.Contains(v, "⣾ working") {
		t.Errorf("pane row should show the agent state:\n%s", v)
	}
}

func TestSidebarCompactShowsAgents(t *testing.T) {
	m := agentModel(t)
	m.sidebar, m.selfPane = true, "%9"
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 40})
	v := ansi.Strip(m.View())
	for _, want := range []string{
		"lazymux", "⣿1 ⣾1", // header counts
		"▾ api", "2 claude", "⣿ 2m", // agent window: state + age
		"─ agents", "⣿ claude", "api:2  2m", "fix jwt expiry · Bash(go t", // two-line entry
		"⣾ codex", "work:1  40s",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("compact sidebar missing %q:\n%s", want, v)
		}
	}
	if strings.Index(v, "⣿ claude") > strings.Index(v, "⣾ codex") {
		t.Error("blocked agents should be listed first")
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
	if v := ansi.Strip(m.View()); !strings.Contains(v, "● done") || ansi.Strip(m.agentSummary()) != "⣿1 ●1" {
		t.Fatalf("finished agent should show as done (● and in the totals):\n%s", v)
	}
	m.Update(key("a")) // Agents page: claude (blocked) is selected first
	if !m.agents["%4"].Unseen {
		t.Fatal("codex is not selected yet, it must stay done")
	}
	m.Update(key("G")) // select codex
	if a, _ := m.curAgent(); a == nil || a.Pane != "%4" || a.Unseen {
		t.Fatalf("selecting the done agent should mark it seen: %+v", a)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "○ codex") || !strings.Contains(v, "idle") {
		t.Errorf("seen agent should show as ○ idle:\n%s", v)
	}
}
