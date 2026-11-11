package agent

import (
	_ "embed"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Desktop notifications on Linux. lazymux asks the notification server once
// what it is and what it supports, and styles its notes to match:
//
//	dunst            dark card with a colored border (bgcolor / fgcolor /
//	                 frcolor hints, which only dunst reads) and colored text
//	body-markup      the place in bold
//	anything else    plain text
//
// The icons are lazymux's own (a red "?" and a green "✓"), written to the
// cache directory; any server can show an image file.

type noteStyle int

const (
	stylePlain noteStyle = iota
	styleBold
	styleDunst
)

var (
	//go:embed icons/blocked.svg
	iconBlocked []byte
	//go:embed icons/done.svg
	iconDone []byte
)

// Catppuccin Mocha, as in the sidebar.
const (
	colBase    = "#1e1e2e"
	colText    = "#cdd6f4"
	colSubtle  = "#a6adc8"
	colOverlay = "#6c7086"
	colRed     = "#f38ba8"
	colGreen   = "#a6e3a1"
	colYellow  = "#f9e2af"
)

var serverStyle = sync.OnceValue(func() noteStyle {
	call := func(method string) string {
		out, err := exec.Command("gdbus", "call", "--session", "--dest", "org.freedesktop.Notifications",
			"--object-path", "/org/freedesktop/Notifications", "--method", "org.freedesktop.Notifications."+method).Output()
		if err != nil {
			return ""
		}
		return string(out)
	}
	return styleFor(call("GetServerInformation"), call("GetCapabilities"))
})

// styleFor picks the style from gdbus's answers, e.g. "('dunst', 'knopwob',
// '1.9.2', '1.2')" and "(['actions', 'body', 'body-markup'],)".
func styleFor(info, caps string) noteStyle {
	if !strings.Contains(caps, "'body-markup'") {
		return stylePlain
	}
	if strings.HasPrefix(strings.TrimLeft(info, "( "), "'dunst'") {
		return styleDunst
	}
	return styleBold
}

// noteIcon is the icon file for a note, or a theme icon name when the file
// cannot be written.
func noteIcon(k NoteKind) string {
	name, data, fallback := "done.svg", iconDone, "emblem-ok-symbolic"
	if k == NoteBlocked {
		name, data, fallback = "blocked.svg", iconBlocked, "dialog-question"
	}
	path := filepath.Join(cacheDir(), "icons", name)
	if cur, err := os.ReadFile(path); err == nil && string(cur) == string(data) {
		return path
	}
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil || os.WriteFile(path, data, 0o644) != nil {
		return fallback
	}
	return path
}

// desktopArgs is the notify-send arguments for a note (without -r / -p).
func desktopArgs(n Note, st noteStyle) []string {
	args := []string{"-a", "lazymux", "-i", noteIcon(n.Kind)}
	if st == styleDunst {
		border := colGreen
		if n.Kind == NoteBlocked {
			border = colRed
		}
		args = append(args, "-h", "string:bgcolor:"+colBase, "-h", "string:fgcolor:"+colText,
			"-h", "string:frcolor:"+border)
	}
	return append(args, n.Title, n.markupBody(st))
}

// markupBody is the body for a style: the place over what it is about.
func (n Note) markupBody(st noteStyle) string {
	if st == stylePlain {
		return n.Body()
	}
	e := html.EscapeString
	place := e(n.Place)
	if st == styleDunst {
		place = strings.ReplaceAll(place, " › ", ` <span color="`+colOverlay+`">›</span> `)
	}
	lines := []string{"<b>" + place + "</b>"}
	switch {
	case n.Detail == "":
	case st == styleBold && n.Verb != "":
		lines = append(lines, e(n.Verb)+"  "+e(n.Detail))
	case st == styleBold:
		lines = append(lines, e(n.Detail))
	case n.Verb != "":
		lines = append(lines, `<span color="`+colYellow+`">`+e(n.Verb)+"</span>  "+e(n.Detail))
	default:
		lines = append(lines, `<span color="`+colSubtle+`">`+e(n.Detail)+"</span>")
	}
	return strings.Join(lines, "\n")
}
