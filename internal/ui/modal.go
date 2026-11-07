package ui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// modal is a dialog drawn over the dimmed main view. update returns done=true
// when the dialog should close.
type modal interface {
	update(k tea.KeyMsg) (cmd tea.Cmd, done bool)
	view(maxW, maxH int) string
}

// keyHints renders "enter create · esc cancel" with keys in blue.
func keyHints(pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, stKey.Render(pairs[i])+" "+stSub.Render(pairs[i+1]))
	}
	return " " + strings.Join(parts, stFaint.Render(" · ")) + " "
}

// --- input dialog ----------------------------------------------------------

type field struct {
	label   string
	in      textinput.Model
	dirComp bool   // tab completes directories
	def     string // initial value
}

type inputModal struct {
	title      string
	fields     []field
	radio      []string // optional choice row, e.g. new session / new window
	radioLabel string   // label of the choice row (default "start in:")
	radioSel   int
	focus      int // index into fields, len(fields) = radio row
	verb       string
	onSubmit   func(vals []string, radio int) tea.Cmd
}

func newField(label, value string, dir bool) field {
	in := textinput.New()
	in.Prompt = "❯ "
	in.PromptStyle = stBlue
	in.TextStyle = stText
	in.Cursor.Style = lipgloss.NewStyle().Foreground(cText)
	in.Cursor.SetMode(cursor.CursorStatic)
	in.SetValue(value)
	in.CursorEnd()
	return field{label: label, in: in, dirComp: dir, def: value}
}

func (d *inputModal) setFocus(i int) {
	n := len(d.fields)
	if d.radio != nil {
		n++
	}
	d.focus = (i + n) % n
	for j := range d.fields {
		if j == d.focus {
			d.fields[j].in.Focus()
		} else {
			d.fields[j].in.Blur()
		}
	}
}

func (d *inputModal) update(k tea.KeyMsg) (tea.Cmd, bool) {
	switch k.Type {
	case tea.KeyEsc:
		return nil, true
	case tea.KeyEnter:
		vals := make([]string, len(d.fields))
		for i, f := range d.fields {
			vals[i] = strings.TrimSpace(f.in.Value())
		}
		return d.onSubmit(vals, d.radioSel), true
	case tea.KeyTab:
		if d.focus < len(d.fields) && d.fields[d.focus].dirComp {
			f := &d.fields[d.focus]
			if c := completeDir(f.in.Value()); c != f.in.Value() {
				f.in.SetValue(c)
				f.in.CursorEnd()
				return nil, false
			}
		}
		d.setFocus(d.focus + 1)
		return nil, false
	case tea.KeyCtrlF:
		if d.focus < len(d.fields) && d.fields[d.focus].dirComp && hasFzf() {
			q := strings.TrimSpace(d.fields[d.focus].in.Value())
			if q == d.fields[d.focus].def { // untouched default: search everything
				q = ""
			}
			return pickDirCmd(q), false
		}
		return nil, false
	case tea.KeyShiftTab, tea.KeyUp:
		d.setFocus(d.focus - 1)
		return nil, false
	case tea.KeyDown:
		d.setFocus(d.focus + 1)
		return nil, false
	}
	if d.focus == len(d.fields) { // radio row
		switch k.String() {
		case "left", "h":
			d.radioSel = (d.radioSel + len(d.radio) - 1) % len(d.radio)
		case "right", "l", " ":
			d.radioSel = (d.radioSel + 1) % len(d.radio)
		}
		return nil, false
	}
	var cmd tea.Cmd
	d.fields[d.focus].in, cmd = d.fields[d.focus].in.Update(k)
	return cmd, false
}

func (d *inputModal) view(maxW, maxH int) string {
	w := min(64, maxW-4)
	inner := w - 4
	labelW := 0
	for _, f := range d.fields {
		labelW = max(labelW, ansi.StringWidth(f.label))
	}
	var lines []string
	for i := range d.fields {
		f := &d.fields[i]
		label := ""
		if len(d.fields) > 1 || f.label != "" {
			ls := stSub
			if i == d.focus {
				ls = stText
			}
			label = ls.Render(padRight(f.label, labelW)) + "  "
		}
		hint := ""
		if f.dirComp && i == d.focus {
			hint = stFaint.Render("  tab: complete")
			if hasFzf() {
				hint = stFaint.Render("  tab · ctrl+f fzf")
			}
		}
		f.in.Width = max(4, inner-ansi.StringWidth(ansi.Strip(label))-3-ansi.StringWidth(hint))
		lines = append(lines, " "+label+f.in.View()+hint)
	}
	if d.radio != nil {
		label := d.radioLabel
		if label == "" {
			label = "start in:"
		}
		row := " " + stSub.Render(label) + "  "
		for i, o := range d.radio {
			dot, st := "○ ", stFaint
			if i == d.radioSel {
				dot, st = "● ", stBlue
				if d.focus == len(d.fields) {
					st = st.Bold(true)
				}
			}
			row += st.Render(dot+o) + "   "
		}
		lines = append(lines, "", row)
	}
	hints := keyHints("enter", d.verb, "esc", "cancel")
	if len(d.fields) > 1 || d.radio != nil {
		hints = keyHints("enter", d.verb, "tab", "next field", "esc", "cancel")
	}
	return renderFrame(stTitleOn.Render(" "+d.title+" "), hints, true, padLines(lines, 1), w, len(lines)+4, cBlue)
}

// completeDir extends a path to the longest common prefix of matching
// directories (appending "/" when unique).
func completeDir(p string) string {
	if p == "" {
		return p
	}
	expanded := p
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			expanded = home + p[1:]
		}
	}
	dir, prefix := filepath.Split(expanded)
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return p
	}
	var matches []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), prefix) && (prefix != "" || !strings.HasPrefix(e.Name(), ".")) {
			matches = append(matches, e.Name())
		}
	}
	if len(matches) == 0 {
		return p
	}
	common := matches[0]
	for _, m := range matches[1:] {
		for !strings.HasPrefix(m, common) {
			common = common[:len(common)-1]
		}
	}
	if len(matches) == 1 {
		common += "/"
	}
	return p + strings.TrimPrefix(common, prefix)
}

// --- confirm dialog --------------------------------------------------------

type confirmModal struct {
	title string
	body  []string
	yes   bool // highlighted choice; defaults to No
	onYes func() tea.Cmd
}

func (d *confirmModal) update(k tea.KeyMsg) (tea.Cmd, bool) {
	switch k.String() {
	case "y", "Y":
		return d.onYes(), true
	case "n", "N", "esc", "q":
		return nil, true
	case "left", "right", "h", "l", "tab", "shift+tab":
		d.yes = !d.yes
	case "enter":
		if d.yes {
			return d.onYes(), true
		}
		return nil, true
	}
	return nil, false
}

func (d *confirmModal) view(maxW, maxH int) string {
	w := min(60, maxW-4)
	yes := stRed.Render("  [y] Yes, kill it  ")
	no := stSub.Render("  [n] No, keep it  ")
	sel := lipgloss.NewStyle().Background(cSurface0).Bold(true)
	if d.yes {
		yes = sel.Foreground(cRed).Render("❯ [y] Yes, kill it  ")
	} else {
		no = sel.Foreground(cText).Render("❯ [n] No, keep it  ")
	}
	lines := append([]string{}, d.body...)
	lines = append(lines, "", " "+yes+"   "+no)
	hints := keyHints("y", "yes", "n / esc", "no", "u", "undo later")
	return renderFrame(stRed.Bold(true).Render(" "+d.title+" "), hints, true, padLines(lines, 1), w, len(lines)+4, cRed)
}

// --- help ------------------------------------------------------------------

type helpModal struct {
	panel     string
	filter    textinput.Model
	filtering bool
	cur       int
	run       func(key string) tea.Cmd
}

func newHelp(panel string, run func(string) tea.Cmd) *helpModal {
	f := textinput.New()
	f.Prompt = "/ "
	f.PromptStyle = stBlue
	f.Placeholder = "filter keys…"
	f.PlaceholderStyle = stFaint
	f.Cursor.SetMode(cursor.CursorStatic)
	return &helpModal{panel: panel, filter: f, run: run}
}

// visible returns the bindings in display order (column 0 then column 1),
// narrowed by the filter.
func (h *helpModal) visible() []binding {
	q := strings.ToLower(strings.TrimSpace(h.filter.Value()))
	var out []binding
	for col := 0; col < 2; col++ {
		for _, b := range bindings {
			if b.col != col {
				continue
			}
			if q != "" && !strings.Contains(strings.ToLower(b.keys+" "+b.desc+" "+b.group), q) {
				continue
			}
			out = append(out, b)
		}
	}
	return out
}

func (h *helpModal) update(k tea.KeyMsg) (tea.Cmd, bool) {
	vis := h.visible()
	switch k.String() {
	case "esc":
		if h.filtering || h.filter.Value() != "" {
			h.filtering = false
			h.filter.SetValue("")
			h.filter.Blur()
			h.cur = 0
			return nil, false
		}
		return nil, true
	case "enter":
		if h.cur < len(vis) && vis[h.cur].run != "" && !vis[h.cur].soon {
			return h.run(vis[h.cur].run), true
		}
		return nil, false
	case "down", "ctrl+n":
		h.cur = clamp(h.cur+1, 0, len(vis)-1)
		return nil, false
	case "up", "ctrl+p":
		h.cur = clamp(h.cur-1, 0, len(vis)-1)
		return nil, false
	}
	if h.filtering {
		var cmd tea.Cmd
		h.filter, cmd = h.filter.Update(k)
		h.cur = 0
		return cmd, false
	}
	switch k.String() {
	case "j":
		h.cur = clamp(h.cur+1, 0, len(vis)-1)
	case "k":
		h.cur = clamp(h.cur-1, 0, len(vis)-1)
	case "/":
		h.filtering = true
		h.filter.Focus()
	case "q", "?":
		return nil, true
	}
	return nil, false
}

func (h *helpModal) view(maxW, maxH int) string {
	w := min(96, maxW-4)
	vis := h.visible()
	sel := ""
	if h.cur < len(vis) {
		sel = vis[h.cur].keys + vis[h.cur].desc
	}
	twoCols := h.filter.Value() == "" && w >= 80
	colW := w - 4
	if twoCols {
		colW = (w - 6) / 2
	}
	keyW := 12

	render := func(col int, all bool) []string {
		var lines []string
		group := ""
		for _, b := range vis {
			if !all && b.col != col {
				continue
			}
			if b.group != group {
				if group != "" {
					lines = append(lines, "")
				}
				group = b.group
				lines = append(lines, stPeach.Bold(true).Render(strings.ToUpper(group)))
			}
			desc := b.desc
			ds := stText
			if b.soon {
				ds = stFaint
				desc += " (soon)"
			}
			row := renderRow(
				[]seg{{" " + padRight(b.keys, keyW), stBlue}, {desc, ds}}, nil, colW,
				boolSel(b.keys+b.desc == sel))
			lines = append(lines, row)
		}
		return lines
	}

	filterLine := " " + h.filter.View()
	lines := []string{filterLine, stFaint.Render(" " + strings.Repeat("─", w-6)), ""}
	if twoCols {
		left, right := render(0, false), render(1, false)
		for i := 0; i < max(len(left), len(right)); i++ {
			l, r := "", ""
			if i < len(left) {
				l = left[i]
			}
			if i < len(right) {
				r = right[i]
			}
			lines = append(lines, " "+padRight(l, colW)+"  "+r)
		}
	} else {
		for _, l := range render(0, true) {
			lines = append(lines, " "+l)
		}
	}
	h2 := min(len(lines)+2, maxH-2)
	if len(lines) > h2-2 {
		lines = lines[:h2-2]
	}
	title := stMauve.Bold(true).Render(" Keybindings ") + stFaint.Render("─ panel: "+h.panel+" ")
	return renderFrame(title, keyHints("enter", "run", "/", "filter", "esc", "close"), true, lines, w, h2, cMauve)
}

func boolSel(b bool) int {
	if b {
		return 2
	}
	return 0
}

// --- helpers ---------------------------------------------------------------

func padRight(s string, w int) string {
	return s + strings.Repeat(" ", max(0, w-ansi.StringWidth(s)))
}

// padLines adds a blank line above and below the content.
func padLines(lines []string, n int) []string {
	out := make([]string, 0, len(lines)+2*n)
	for i := 0; i < n; i++ {
		out = append(out, "")
	}
	out = append(out, lines...)
	for i := 0; i < n; i++ {
		out = append(out, "")
	}
	return out
}

// dim renders the view without colors in a faint color, as a backdrop.
func dim(view string) string {
	lines := strings.Split(view, "\n")
	for i, l := range lines {
		lines[i] = stFaint.Render(ansi.Strip(l))
	}
	return strings.Join(lines, "\n")
}

// overlay draws fg on top of bg with its top-left corner at column x, row y.
func overlay(bg, fg string, x, y int) string {
	bl := strings.Split(bg, "\n")
	for i, l := range strings.Split(fg, "\n") {
		r := y + i
		if r < 0 || r >= len(bl) {
			continue
		}
		b := bl[r]
		left := ansi.Truncate(b, x, "")
		left += strings.Repeat(" ", max(0, x-ansi.StringWidth(left)))
		right := ansi.TruncateLeft(b, x+ansi.StringWidth(l), "")
		bl[r] = left + "\x1b[0m" + l + "\x1b[0m" + right
	}
	return strings.Join(bl, "\n")
}

// center overlays fg in the middle of bg (w×h cells).
func center(bg, fg string, w, h int) string {
	fw, fh := lipgloss.Width(fg), lipgloss.Height(fg)
	return overlay(bg, fg, max(0, (w-fw)/2), max(0, (h-fh)/2))
}
