package ui

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// dirPickedMsg carries the directory chosen in fzf ("" when cancelled).
type dirPickedMsg struct {
	dir string
	err error
}

var (
	fzfOnce sync.Once
	fzfPath string
)

// hasFzf reports whether fzf is on PATH (checked once).
func hasFzf() bool {
	fzfOnce.Do(func() { fzfPath, _ = exec.LookPath("fzf") })
	return fzfPath != ""
}

// dirSource is the shell pipeline that lists candidate directories: zoxide's
// frecent dirs first, then fd (or find) under $HOME, without duplicates.
func dirSource() string {
	var parts []string
	if _, err := exec.LookPath("zoxide"); err == nil {
		parts = append(parts, "zoxide query -l 2>/dev/null")
	}
	if _, err := exec.LookPath("fd"); err == nil {
		parts = append(parts, `fd -t d --max-depth 4 --absolute-path . "$HOME" 2>/dev/null | sed 's:/$::'`)
	} else {
		parts = append(parts, `find "$HOME" -maxdepth 4 -type d -not -path '*/.*' 2>/dev/null`)
	}
	return "{ " + strings.Join(parts, "; ") + "; } | awk '!seen[$0]++'"
}

// shellQuote quotes s for sh.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// pickDirCmd suspends the UI, runs fzf over directories and reports the pick.
// fzf draws on /dev/tty, so only its result goes to the captured stdout.
func pickDirCmd(query string) tea.Cmd {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(query, "~") {
		query = home + query[1:]
	}
	script := dirSource() + " | fzf --scheme=path --height=100% --layout=reverse" +
		" --prompt='dir ❯ ' --header='enter pick · esc cancel'" +
		" --preview='ls -A --color=always {} 2>/dev/null | head -100'" +
		" --query=" + shellQuote(query)
	var out bytes.Buffer
	cmd := exec.Command("sh", "-c", script)
	cmd.Stdout = &out
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		var ee *exec.ExitError
		if errors.As(err, &ee) && (ee.ExitCode() == 130 || ee.ExitCode() == 1) {
			return dirPickedMsg{} // esc / no match: nothing picked
		}
		return dirPickedMsg{dir: strings.TrimSpace(out.String()), err: err}
	})
}

// setDir puts a picked directory into the dialog's directory field and, when
// the name is still empty, names it after the directory.
func (d *inputModal) setDir(dir string) {
	idx := -1
	if d.focus < len(d.fields) && d.fields[d.focus].dirComp {
		idx = d.focus
	} else {
		for i, f := range d.fields {
			if f.dirComp {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		return
	}
	d.fields[idx].in.SetValue(dir)
	d.fields[idx].in.CursorEnd()
	for i := range d.fields {
		if i != idx && d.fields[i].label == "Name" && strings.TrimSpace(d.fields[i].in.Value()) == "" {
			d.fields[i].in.SetValue(sessionName(filepath.Base(dir)))
			d.fields[i].in.CursorEnd()
		}
	}
}

// sessionName makes a directory name usable as a tmux name (tmux turns '.'
// and ':' into '_' anyway).
func sessionName(s string) string {
	return strings.NewReplacer(".", "_", ":", "_").Replace(s)
}
