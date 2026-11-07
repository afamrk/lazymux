package agent

import (
	"regexp"
	"strings"
)

// Screen scraping: the visible pane text (and the terminal title) tells
// whether an agent is working, blocked or idle. What each shape means lives
// in the agent's detection rules (rules.go, rules/*.toml); this file
// prepares the screen and reads what a blocked agent waits for.

var (
	// a selected first option: "❯ 1. Yes", "› 1. Yes, proceed", "● 1. Allow once"
	reFirstOption = regexp.MustCompile(`^\s*[│┃|]?\s*[❯›>●▶▸]\s*1[.)]\s+(Yes|Allow|Approve|Proceed)`)
	// a selected first option of any kind: "❯ 1. One"
	reSelectedFirst = regexp.MustCompile(`^\s*[│┃|]?\s*[❯›>●▶▸]\s*1[.)]\s+\S`)
	// any numbered option line, for finding where a prompt box starts
	reOption = regexp.MustCompile(`^\s*[│┃|]?\s*[❯›>●▶▸]?\s*\d[.)]\s+\S`)
	// aider asks with "(Y)es/(N)o"
	reAiderAsk   = regexp.MustCompile(`\(Y\)es/\(N\)o`)
	reToolHeader = regexp.MustCompile(`^(Bash command|(Edit|Create|Read|Write|Delete) file|Fetch|Web ?Fetch|Web search|Tool use|MCP tool)\b.{0,20}$`)
	reBoxOnly    = regexp.MustCompile(`^[\s─━│┃╭╮╰╯┌┐└┘├┤═║╔╗╚╝-]*$`)
)

// normalize turns non-breaking spaces (Claude's prompt uses them) into
// spaces and trims each line's right end.
func normalize(screen string) string {
	lines := strings.Split(strings.ReplaceAll(screen, "\u00a0", " "), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Join(lines, "\n")
}

// bottom returns the last n non-blank lines of the screen, right-trimmed.
func bottom(screen string, n int) []string {
	all := strings.Split(normalize(screen), "\n")
	var out []string
	for i := len(all) - 1; i >= 0 && len(out) < n; i-- {
		if strings.TrimSpace(all[i]) != "" {
			out = append(out, all[i])
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// Detection is what the rules read from a pane.
type Detection struct {
	State   State
	Waiting string // blocked: what the prompt is about
	Rule    string // the matched rule; "" when none matched
	Matched bool   // false: no rule matched (the caller decides: idle, or working for a changing screen)
	Keep    bool   // the rule says: keep the previous state
}

// Classify reads an agent's state from its screen and terminal title with
// the agent's detection rules.
func Classify(k Kind, screen, title string) Detection {
	m, _ := ManifestFor(k)
	v, _ := m.Evaluate(Input{Screen: normalize(screen), Title: title}, false)
	d := Detection{State: v.State, Rule: v.Rule, Matched: v.Matched, Keep: v.Keep}
	if d.State == Blocked && !d.Keep {
		d.Waiting = waitingReason(k, screen)
	}
	return d
}

// waitingReason reads what a permission prompt asks about: the lines above
// its selected first option, or aider's question.
func waitingReason(k Kind, screen string) string {
	lines := bottom(screen, 30)
	if k == Aider {
		for i := len(lines) - 1; i >= 0; i-- {
			if reAiderAsk.MatchString(lines[i]) {
				return strings.TrimSpace(lines[i])
			}
		}
		return ""
	}
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-15; i-- {
		if reFirstOption.MatchString(lines[i]) {
			return promptReason(lines[:i])
		}
	}
	// a question form: the question above its options
	for i := len(lines) - 1; i >= 0; i-- {
		if !reSelectedFirst.MatchString(lines[i]) {
			continue
		}
		for j := i - 1; j >= 0 && j >= i-4; j-- {
			if q := strings.TrimSpace(strings.Trim(strings.TrimSpace(lines[j]), "│┃|")); strings.HasSuffix(q, "?") {
				return q
			}
		}
		return ""
	}
	return ""
}

// promptReason guesses what a permission prompt is about from the lines
// above its options: the question is skipped, and the nearest content lines
// (e.g. "Bash command" / "go test ./...") are joined as "Bash command: go test ./...".
func promptReason(above []string) string {
	clean := func(l string) string {
		return strings.TrimSpace(strings.Trim(strings.TrimSpace(l), "│┃|"))
	}
	// Claude Code: a short tool header ("Bash command", "Edit file") followed
	// by the command or file
	for i := len(above) - 1; i >= 0 && i >= len(above)-12; i-- {
		if h := clean(above[i]); reToolHeader.MatchString(h) {
			for _, l := range above[i+1:] {
				if c := clean(l); c != "" && !reBoxOnly.MatchString(c) {
					return h + ": " + c
				}
			}
			return h
		}
	}
	var got []string
	for i := len(above) - 1; i >= 0 && i >= len(above)-10 && len(got) < 2; i-- {
		l := strings.Trim(strings.TrimSpace(above[i]), "│┃|")
		l = strings.TrimSpace(l)
		if l == "" || reBoxOnly.MatchString(l) || reOption.MatchString(above[i]) {
			continue
		}
		low := strings.ToLower(l)
		if strings.HasPrefix(low, "do you want") || strings.HasPrefix(low, "would you like") ||
			strings.HasPrefix(low, "allow ") || strings.HasSuffix(l, "?") {
			continue // the question itself
		}
		got = append(got, l)
	}
	switch len(got) {
	case 0:
		return ""
	case 1:
		return got[0]
	}
	return got[1] + ": " + got[0]
}
