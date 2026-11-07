package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// seg is a piece of plain text with a style; rows are built from segments so
// the selection background can be applied to every piece.
type seg struct {
	text string
	st   lipgloss.Style
}

// renderRow lays out left segments flush left and right segments flush right
// in exactly width cells. The left side is truncated with "…" when needed.
// sel is 0 (none), 1 (selected in unfocused panel) or 2 (selected, focused).
func renderRow(left, right []seg, width int, sel int) string {
	rw := segsWidth(right)
	if rw > width {
		right, rw = nil, 0
	}
	avail := width - rw
	if rw > 0 {
		avail-- // keep one space before the right part
	}
	left = truncSegs(left, avail)
	lw := segsWidth(left)

	var bg lipgloss.TerminalColor
	switch sel {
	case 1:
		bg = cSelDim
	case 2:
		bg = cSurface0
	}
	style := func(st lipgloss.Style) lipgloss.Style {
		if bg != nil {
			st = st.Background(bg)
		}
		if sel == 2 {
			st = st.Bold(true)
		}
		return st
	}
	var b strings.Builder
	for _, s := range left {
		b.WriteString(style(s.st).Render(s.text))
	}
	b.WriteString(style(lipgloss.NewStyle()).Render(strings.Repeat(" ", max(0, width-lw-rw))))
	for _, s := range right {
		b.WriteString(style(s.st).Render(s.text))
	}
	return b.String()
}

func segsWidth(ss []seg) int {
	n := 0
	for _, s := range ss {
		n += ansi.StringWidth(s.text)
	}
	return n
}

func truncSegs(ss []seg, width int) []seg {
	if segsWidth(ss) <= width {
		return ss
	}
	out := []seg{}
	used := 0
	for _, s := range ss {
		w := ansi.StringWidth(s.text)
		if used+w <= width-1 {
			out = append(out, s)
			used += w
			continue
		}
		rest := width - used
		if rest > 0 {
			out = append(out, seg{ansi.Truncate(s.text, rest, "…"), s.st})
		}
		break
	}
	return out
}

// renderBox draws a panel: a rounded box of w×h cells with a styled title in
// the top border and an optional footer (e.g. "3 of 4") at the right of the
// bottom border. lines are truncated/padded to the inner width.
func renderBox(title, footer string, lines []string, w, h int, focused bool) string {
	color, fst := cSurface1, stFaint
	if focused {
		color, fst = cBlue, stBlue
	}
	if footer != "" {
		footer = fst.Render(footer)
	}
	return renderFrame(title, footer, false, lines, w, h, color)
}

// renderFrame is the general box: border color, title (already styled) in the
// top border, footer (already styled) in the bottom border, left- or
// right-aligned.
func renderFrame(title, footer string, footerLeft bool, lines []string, w, h int, color lipgloss.TerminalColor) string {
	if w < 4 || h < 2 {
		return ""
	}
	bc := lipgloss.NewStyle().Foreground(color)
	inner := w - 2

	title = ansi.Truncate(title, inner-2, "…")
	top := bc.Render("╭─") + title +
		bc.Render(strings.Repeat("─", max(0, inner-1-ansi.StringWidth(title)))+"╮")

	bottom := bc.Render("╰" + strings.Repeat("─", inner) + "╯")
	if fw := ansi.StringWidth(footer); footer != "" && fw+4 <= inner {
		if footerLeft { // ╰─ footer ───╯
			bottom = bc.Render("╰─") + footer + bc.Render(strings.Repeat("─", inner-1-fw)+"╯")
		} else { // ╰─── footer─╯
			bottom = bc.Render("╰"+strings.Repeat("─", inner-fw-1)) + footer + bc.Render("─╯")
		}
	}

	var b strings.Builder
	b.WriteString(top)
	side := bc.Render("│")
	for i := 0; i < h-2; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		if ansi.StringWidth(line) > inner {
			line = ansi.Truncate(line, inner, "")
		}
		b.WriteString("\n" + side + line + "\x1b[0m" +
			strings.Repeat(" ", max(0, inner-ansi.StringWidth(line))) + side)
	}
	b.WriteString("\n" + bottom)
	return b.String()
}

// scrollWindow returns the first visible index so that cursor stays within a
// viewport of n rows, given the previous offset.
func scrollWindow(off, cursor, n, total int) int {
	if n <= 0 {
		return 0
	}
	if cursor < off {
		off = cursor
	}
	if cursor >= off+n {
		off = cursor - n + 1
	}
	if off > total-n {
		off = max(0, total-n)
	}
	return max(0, off)
}
