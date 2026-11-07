package ui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

// fuzzyMatch reports whether all runes of pattern appear in text in order
// (case-insensitive), with a score (higher is better) and the matched rune
// positions for highlighting. Consecutive matches, matches at the start of a
// word and matches near the start score higher. Spaces in the pattern are
// ignored, so "api cl" matches "api:3 claude".
func fuzzyMatch(pattern, text string) (score int, idx []int, ok bool) {
	pat := []rune(strings.ToLower(strings.ReplaceAll(pattern, " ", "")))
	if len(pat) == 0 {
		return 0, nil, true
	}
	runes := []rune(text)
	lower := []rune(strings.ToLower(text))

	// Prefer the leftmost match that starts each run as late as possible would
	// need DP; a greedy forward pass followed by a backward tightening pass is
	// enough for short names.
	pi := 0
	for i := 0; i < len(lower) && pi < len(pat); i++ {
		if lower[i] == pat[pi] {
			idx = append(idx, i)
			pi++
		}
	}
	if pi < len(pat) {
		return 0, nil, false
	}
	// tighten: walk back from the last match so the matched span is minimal
	end := idx[len(idx)-1]
	pi = len(pat) - 1
	tight := make([]int, len(pat))
	for i := end; i >= 0 && pi >= 0; i-- {
		if lower[i] == pat[pi] {
			tight[pi] = i
			pi--
		}
	}
	idx = tight

	for k, i := range idx {
		score += 10
		if k > 0 && idx[k-1] == i-1 {
			score += 15 // consecutive
		}
		if i == 0 || !unicode.IsLetter(runes[i-1]) && !unicode.IsDigit(runes[i-1]) {
			score += 10 // start of a word
		}
	}
	score -= idx[0]                         // closer to the start is better
	score -= (idx[len(idx)-1] - idx[0]) / 2 // compact matches are better
	return score, idx, true
}

// hlSegs splits text into segments, styling the runes at idx with hl and the
// rest with base. Positions >= len(text) are ignored.
func hlSegs(text string, idx []int, base, hl lipgloss.Style) []seg {
	if len(idx) == 0 {
		return []seg{{text, base}}
	}
	set := make(map[int]bool, len(idx))
	for _, i := range idx {
		set[i] = true
	}
	var out []seg
	var cur []rune
	curHL := false
	flush := func() {
		if len(cur) > 0 {
			st := base
			if curHL {
				st = hl
			}
			out = append(out, seg{string(cur), st})
			cur = cur[:0]
		}
	}
	for i, r := range []rune(text) {
		if set[i] != curHL {
			flush()
			curHL = set[i]
		}
		cur = append(cur, r)
	}
	flush()
	return out
}

// stMatch highlights matched characters (peach, bold).
var stMatch = lipgloss.NewStyle().Foreground(cPeach).Bold(true)
