package agent

import (
	"regexp"
	"strings"
)

// Screen scraping: the visible pane text tells whether an agent is working
// (a spinner / "esc to interrupt" line near the bottom) or blocked (a
// permission prompt with numbered options). Only the bottom of the screen is
// looked at, so the same words inside the conversation do not count.

var (
	// a selected first option: "❯ 1. Yes", "› 1. Yes, proceed", "● 1. Allow once"
	reFirstOption = regexp.MustCompile(`^\s*[│┃|]?\s*[❯›>●▶▸]\s*1[.)]\s+(Yes|Allow|Approve|Proceed)`)
	// any numbered option line, for finding where a prompt box starts
	reOption = regexp.MustCompile(`^\s*[│┃|]?\s*[❯›>●▶▸]?\s*\d[.)]\s+\S`)
	// Claude Code's status line while it works: "✽ Philosophizing… (1m 29s · ↓ 5.6k tokens)"
	reClaudeSpinner = regexp.MustCompile(`^\s*[·✢✳✶✻✽*⏺]\s+\S+…\s*\(`)
	// aider asks with "(Y)es/(N)o"
	reAiderAsk = regexp.MustCompile(`\(Y\)es/\(N\)o`)
	// aider's input prompt: "> " or "architect> "
	reAiderPrompt = regexp.MustCompile(`^[\w-]*> ?$`)
	reToolHeader  = regexp.MustCompile(`^(Bash command|(Edit|Create|Read|Write|Delete) file|Fetch|Web ?Fetch|Web search|Tool use|MCP tool)\b.{0,20}$`)
	reBoxOnly     = regexp.MustCompile(`^[\s─━│┃╭╮╰╯┌┐└┘├┤═║╔╗╚╝-]*$`)
)

// bottom returns the last n non-blank lines of the screen, right-trimmed.
func bottom(screen string, n int) []string {
	all := strings.Split(screen, "\n")
	var out []string
	for i := len(all) - 1; i >= 0 && len(out) < n; i-- {
		l := strings.TrimRight(strings.ReplaceAll(all[i], "\u00a0", " "), " \t")
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// Classify reads an agent's state from its screen. ok is false when the
// screen shows neither a prompt nor a working indicator; the caller then
// decides (idle, or working if the screen keeps changing for agents without
// a working indicator).
func Classify(k Kind, screen string) (st State, waiting string, ok bool) {
	lines := bottom(screen, 30)
	if k == Aider {
		if len(lines) > 0 {
			last := lines[len(lines)-1]
			if reAiderAsk.MatchString(last) {
				return Blocked, strings.TrimSpace(last), true
			}
			if reAiderPrompt.MatchString(strings.TrimSpace(last)) {
				return Idle, "", true
			}
		}
		return Idle, "", false
	}

	// blocked: a selected "1. Yes / Allow" option near the bottom
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-15; i-- {
		if reFirstOption.MatchString(lines[i]) {
			return Blocked, promptReason(lines[:i]), true
		}
	}
	if k == Opencode {
		for _, l := range tail(lines, 12) {
			if strings.Contains(l, "Allow once") || strings.Contains(l, "Permission required") {
				return Blocked, "", true
			}
		}
	}

	// working: an interrupt hint or Claude's spinner line near the bottom
	for _, l := range tail(lines, 10) {
		low := strings.ToLower(l)
		if strings.Contains(low, "esc to interrupt") || strings.Contains(low, "esc to cancel") ||
			strings.Contains(low, "esc interrupt") || strings.Contains(low, "ctrl+c to interrupt") {
			return Working, "", true
		}
		if k == Claude && reClaudeSpinner.MatchString(l) {
			return Working, "", true
		}
	}
	return Idle, "", true
}

func tail(lines []string, n int) []string {
	if len(lines) > n {
		return lines[len(lines)-n:]
	}
	return lines
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
