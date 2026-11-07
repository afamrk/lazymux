package agent

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"

	"github.com/afamrk/lazymux/internal/tmux"
)

// Detection rules are data: one TOML manifest per agent says which screen
// (or terminal title) shapes mean working, blocked or idle. The format is
// herdr's agent-detection manifest format (github.com/herdrdev/herdr,
// src/detect/manifests), so a herdr manifest works in lazymux too:
//
//	id = "claude"
//	aliases = ["claude-code"]
//
//	[[rules]]
//	id = "permission_prompt"
//	state = "blocked"                      # idle | working | blocked | unknown
//	priority = 900                         # the matching rule with the highest priority wins
//	region = "bottom_non_empty_lines(15)"  # which part of the screen is matched
//	line_regex = ['^\s*❯\s*1\.\s+Yes']    # every pattern matches some line
//	contains = ["esc to cancel"]           # every needle is in the region (case-insensitive)
//	regex = ['…']                          # every pattern matches the region
//	any = [{ contains = ["a"] }, { … }]    # at least one nested gate matches
//	all = [{ … }]                          # every nested gate matches
//	not = [{ contains = ["enter to select"] }]  # no nested gate matches
//	skip_state_update = true               # a match keeps the previous state (overlays, menus)
//
// lazymux ships a manifest per agent (rules/*.toml, embedded). A file
// ~/.config/lazymux/agent-detection/<agent>.toml replaces it, so a changed
// agent UI can be fixed without a new lazymux.

//go:embed rules/*.toml
var bundledRules embed.FS

// Manifest is one agent's rules.
type Manifest struct {
	ID               string   `toml:"id"`
	Version          string   `toml:"version"`
	MinEngineVersion int      `toml:"min_engine_version"`
	UpdatedAt        string   `toml:"updated_at"`
	Aliases          []string `toml:"aliases"`
	Rules            []Rule   `toml:"rules"`

	Source string `toml:"-"` // "bundled" or the override file
}

// Rule is one detection rule: a gate, the state it means and where it looks.
type Rule struct {
	ID              string `toml:"id"`
	State           string `toml:"state"`
	Priority        int    `toml:"priority"`
	Region          string `toml:"region"`
	VisibleIdle     bool   `toml:"visible_idle"`
	VisibleBlocker  bool   `toml:"visible_blocker"`
	VisibleWorking  bool   `toml:"visible_working"`
	SkipStateUpdate bool   `toml:"skip_state_update"`
	Gate
}

// Gate is a set of conditions that must all hold.
type Gate struct {
	Contains  []string `toml:"contains"`
	Regex     []string `toml:"regex"`
	LineRegex []string `toml:"line_regex"`
	All       []Gate   `toml:"all"`
	Any       []Gate   `toml:"any"`
	Not       []Gate   `toml:"not"`

	contains  []string
	regex     []*regexp.Regexp
	lineRegex []*regexp.Regexp
}

// ParseManifest reads and compiles a manifest.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	md, err := toml.Decode(string(data), &m)
	if err != nil {
		return nil, err
	}
	if extra := md.Undecoded(); len(extra) > 0 {
		return nil, fmt.Errorf("unknown key %s", extra[0])
	}
	if m.ID == "" {
		return nil, fmt.Errorf("missing id")
	}
	for i := range m.Rules {
		r := &m.Rules[i]
		if r.ID == "" {
			return nil, fmt.Errorf("rule %d: missing id", i+1)
		}
		if _, ok := ruleState(r.State); !ok {
			return nil, fmt.Errorf("rule %s: unknown state %q", r.ID, r.State)
		}
		if r.Region == "" {
			r.Region = "whole_recent"
		}
		if !validRegion(r.Region) {
			return nil, fmt.Errorf("rule %s: unknown region %q", r.ID, r.Region)
		}
		if !r.Gate.positive() {
			return nil, fmt.Errorf("rule %s: no condition", r.ID)
		}
		if err := r.Gate.compile(); err != nil {
			return nil, fmt.Errorf("rule %s: %v", r.ID, err)
		}
	}
	return &m, nil
}

// positive reports whether a gate tests anything besides `not`.
func (g *Gate) positive() bool {
	return len(g.Contains)+len(g.Regex)+len(g.LineRegex)+len(g.All)+len(g.Any) > 0
}

func (g *Gate) compile() error {
	g.contains = nil
	for _, c := range g.Contains {
		g.contains = append(g.contains, strings.ToLower(c))
	}
	compile := func(pats []string) ([]*regexp.Regexp, error) {
		var out []*regexp.Regexp
		for _, p := range pats {
			re, err := regexp.Compile(goRegex(p))
			if err != nil {
				return nil, fmt.Errorf("pattern %q: %v", p, err)
			}
			out = append(out, re)
		}
		return out, nil
	}
	var err error
	if g.regex, err = compile(g.Regex); err != nil {
		return err
	}
	if g.lineRegex, err = compile(g.LineRegex); err != nil {
		return err
	}
	for _, list := range [][]Gate{g.All, g.Any, g.Not} {
		for i := range list {
			if !list[i].positive() {
				return fmt.Errorf("empty nested gate")
			}
			if err := list[i].compile(); err != nil {
				return err
			}
		}
	}
	return nil
}

// match tests the gate on text (lower is text in lower case).
func (g *Gate) match(text, lower string) bool {
	for _, c := range g.contains {
		if !strings.Contains(lower, c) {
			return false
		}
	}
	for _, re := range g.regex {
		if !re.MatchString(text) {
			return false
		}
	}
	if len(g.lineRegex) > 0 {
		lines := strings.Split(text, "\n")
		for _, re := range g.lineRegex {
			found := false
			for _, l := range lines {
				if re.MatchString(l) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	for i := range g.All {
		if !g.All[i].match(text, lower) {
			return false
		}
	}
	if len(g.Any) > 0 {
		found := false
		for i := range g.Any {
			if g.Any[i].match(text, lower) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for i := range g.Not {
		if g.Not[i].match(text, lower) {
			return false
		}
	}
	return true
}

// goRegex translates the Rust regex syntax herdr's manifests use and Go's
// regexp lacks: \uXXXX and \u{X} become \x{…}, \p{Alphabetic} becomes
// \p{L} (letters; close enough for spinner words).
func goRegex(p string) string {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		if p[i] != '\\' || i+1 >= len(p) {
			b.WriteByte(p[i])
			continue
		}
		rest := p[i+1:]
		switch {
		case strings.HasPrefix(rest, "u{"):
			if end := strings.IndexByte(rest, '}'); end > 0 {
				b.WriteString(`\x{` + rest[2:end] + `}`)
				i += end + 1
				continue
			}
		case len(rest) >= 5 && rest[0] == 'u' && isHex(rest[1:5]):
			b.WriteString(`\x{` + rest[1:5] + `}`)
			i += 5
			continue
		case strings.HasPrefix(rest, "p{Alphabetic}"), strings.HasPrefix(rest, "P{Alphabetic}"):
			b.WriteString(`\` + rest[:1] + `{L}`)
			i += len("p{Alphabetic}")
			continue
		}
		b.WriteString(p[i : i+2]) // any other escape, kept as is (\\ too)
		i++
	}
	return b.String()
}

func isHex(s string) bool {
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

func ruleState(s string) (st State, ok bool) {
	switch s {
	case "idle":
		return Idle, true
	case "working":
		return Working, true
	case "blocked":
		return Blocked, true
	case "unknown", "":
		return Idle, true
	}
	return Idle, false
}

// --- regions ----------------------------------------------------------------------

// Input is what rules look at.
type Input struct {
	Screen string // the visible pane text
	Title  string // the pane title the agent set (OSC 0 / 2)
}

var fixedRegions = map[string]bool{
	"whole_recent": true, "osc_title": true, "osc_progress": true,
	"after_last_prompt_marker": true, "before_current_prompt_marker": true,
	"whole_recent_without_current_prompt_marker": true, "current_prompt_block_marker": true,
	"after_current_prompt_block_marker": true, "prompt_box_body": true, "above_prompt_box": true,
	"last_non_empty_above_prompt_box": true, "after_last_horizontal_rule": true,
}

func validRegion(spec string) bool {
	if fixedRegions[spec] {
		return true
	}
	for _, name := range []string{"bottom_lines", "bottom_non_empty_lines", "top_non_empty_lines"} {
		if _, ok := regionCount(spec, name); ok {
			return true
		}
	}
	return false
}

func regionCount(spec, name string) (int, bool) {
	rest, ok := strings.CutPrefix(spec, name+"(")
	if !ok {
		return 0, false
	}
	rest, ok = strings.CutSuffix(rest, ")")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	return n, err == nil && n > 0
}

// region returns the part of the input a rule looks at (herdr's regions).
// OSC progress is not available through tmux and is always empty.
func region(in Input, spec string) string {
	switch spec {
	case "osc_title":
		return in.Title
	case "osc_progress":
		return ""
	}
	lines := strings.Split(in.Screen, "\n")
	join := func(from, to int) string { return strings.Join(lines[from:to], "\n") }
	switch spec {
	case "whole_recent":
		return in.Screen
	case "after_last_prompt_marker":
		for i := len(lines) - 1; i >= 0; i-- {
			if codexPromptLine(lines[i]) {
				return join(i+1, len(lines))
			}
		}
		return in.Screen
	case "before_current_prompt_marker":
		if i := currentCodexPrompt(lines); i >= 0 {
			return join(0, i)
		}
		return in.Screen
	case "whole_recent_without_current_prompt_marker":
		if currentCodexPrompt(lines) >= 0 {
			return ""
		}
		return in.Screen
	case "current_prompt_block_marker", "after_current_prompt_block_marker":
		p := currentCodexPrompt(lines)
		for i := p - 1; p >= 0 && i >= 0; i-- {
			if codexBlockMarker(lines[i]) {
				if spec == "current_prompt_block_marker" {
					return lines[i]
				}
				return join(i, len(lines))
			}
		}
		return ""
	case "prompt_box_body":
		top := promptBoxTop(lines)
		if top < 0 {
			return ""
		}
		end := len(lines)
		for i := top + 1; i < len(lines); i++ {
			if horizontalRule(lines[i]) {
				end = i
				break
			}
		}
		return join(top+1, end)
	case "above_prompt_box", "last_non_empty_above_prompt_box":
		above := lines
		if top := promptBoxTop(lines); top >= 0 {
			above = lines[:top]
		}
		if spec == "above_prompt_box" {
			return strings.Join(above, "\n")
		}
		for i := len(above) - 1; i >= 0; i-- {
			if strings.TrimSpace(above[i]) != "" {
				return above[i]
			}
		}
		return ""
	case "after_last_horizontal_rule":
		for i := len(lines) - 1; i >= 0; i-- {
			if horizontalRule(lines[i]) {
				return join(i+1, len(lines))
			}
		}
		return in.Screen
	}
	if n, ok := regionCount(spec, "bottom_lines"); ok {
		return join(max(0, len(lines)-n), len(lines))
	}
	if n, ok := regionCount(spec, "bottom_non_empty_lines"); ok {
		start, seen := len(lines), 0
		for i := len(lines) - 1; i >= 0 && seen < n; i-- {
			if strings.TrimSpace(lines[i]) != "" {
				start, seen = i, seen+1
			}
		}
		return join(start, len(lines))
	}
	if n, ok := regionCount(spec, "top_non_empty_lines"); ok {
		end, seen := 0, 0
		for i := 0; i < len(lines) && seen < n; i++ {
			if strings.TrimSpace(lines[i]) != "" {
				end, seen = i+1, seen+1
			}
		}
		return join(0, end)
	}
	return ""
}

func codexPromptLine(l string) bool { return l == "›" || strings.HasPrefix(l, "› ") }

func codexBlockMarker(l string) bool {
	for _, p := range []string{"•", "■", "✗", "✓"} {
		if strings.HasPrefix(l, p) {
			return true
		}
	}
	return false
}

// currentCodexPrompt is the last prompt line with no output block after it, or -1.
func currentCodexPrompt(lines []string) int {
	p := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if codexPromptLine(lines[i]) {
			p = i
			break
		}
	}
	if p < 0 {
		return -1
	}
	for _, l := range lines[p+1:] {
		if codexBlockMarker(l) {
			return -1
		}
	}
	return p
}

// promptBoxTop is the second horizontal rule from the bottom, or -1.
func promptBoxTop(lines []string) int {
	n := 0
	for i := len(lines) - 1; i >= 0; i-- {
		if horizontalRule(lines[i]) {
			if n++; n == 2 {
				return i
			}
		}
	}
	return -1
}

// horizontalRule: a line that starts with ─ and is only ─, or has at least 3.
func horizontalRule(l string) bool {
	t := strings.TrimSpace(l)
	n := 0
	for _, r := range t {
		if r != '─' {
			break
		}
		n++
	}
	if n == 0 {
		return false
	}
	rest := strings.TrimLeft(t, "─")
	return strings.TrimSpace(rest) == "" || n >= 3
}

// --- evaluation --------------------------------------------------------------------

// Verdict is the outcome of a manifest on a screen.
type Verdict struct {
	State   State
	Rule    string // the matched rule's id; "" when no rule matched
	Matched bool
	Keep    bool // the rule says: keep the previous state (skip_state_update)
}

// RuleResult is one rule's outcome, for `lazymux explain`.
type RuleResult struct {
	Rule    *Rule
	Matched bool
	Region  string
}

// Evaluate runs a manifest's rules on the input. explain also returns every
// rule's outcome.
func (m *Manifest) Evaluate(in Input, explain bool) (Verdict, []RuleResult) {
	var best *Rule
	var results []RuleResult
	cache := map[string][2]string{}
	for i := range m.Rules {
		r := &m.Rules[i]
		reg, ok := cache[r.Region]
		if !ok {
			text := region(in, r.Region)
			reg = [2]string{text, strings.ToLower(text)}
			cache[r.Region] = reg
		}
		hit := r.Gate.match(reg[0], reg[1])
		if explain {
			results = append(results, RuleResult{Rule: r, Matched: hit, Region: reg[0]})
		}
		if hit && (best == nil || r.Priority > best.Priority) {
			best = r
		}
	}
	if best == nil {
		return Verdict{State: Idle}, results
	}
	st, _ := ruleState(best.State)
	return Verdict{State: st, Rule: best.ID, Matched: true,
		Keep: best.SkipStateUpdate || best.State == "unknown"}, results
}

// --- loading -----------------------------------------------------------------------

// herdrNames are herdr's ids for agents lazymux names differently.
var herdrNames = map[Kind]string{Agy: "antigravity"}

// RulesDir is where manifests that replace the bundled ones live:
// $LAZYMUX_RULES, or ~/.config/lazymux/agent-detection.
func RulesDir() string {
	if d := os.Getenv("LAZYMUX_RULES"); d != "" {
		return d
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "lazymux", "agent-detection")
}

func (m *Manifest) matches(k Kind) bool {
	names := append([]string{m.ID}, m.Aliases...)
	for _, n := range names {
		if n == string(k) || (herdrNames[k] != "" && n == herdrNames[k]) {
			return true
		}
	}
	return false
}

type loadedRules struct {
	m    *Manifest
	warn string // why an override was ignored
	mod  string // override file identity when loaded ("" = none), to notice edits
}

var rulesCache = struct {
	sync.Mutex
	by map[Kind]loadedRules
}{by: map[Kind]loadedRules{}}

// overrideFile finds an agent's override manifest: <kind>.toml, or herdr's
// file name for it.
func overrideFile(k Kind) (path, stamp string) {
	names := []string{string(k)}
	if h := herdrNames[k]; h != "" {
		names = append(names, h)
	}
	for _, n := range names {
		p := filepath.Join(RulesDir(), n+".toml")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, fmt.Sprint(st.ModTime().UnixNano(), st.Size())
		}
	}
	return "", ""
}

// ManifestFor returns the rules for an agent: the override file when there
// is a valid one, else the bundled manifest. warn says why an override was
// ignored. An edited override is picked up on the next call.
func ManifestFor(k Kind) (m *Manifest, warn string) {
	path, stamp := overrideFile(k)
	rulesCache.Lock()
	defer rulesCache.Unlock()
	if l, ok := rulesCache.by[k]; ok && l.mod == path+stamp {
		return l.m, l.warn
	}
	l := loadedRules{m: bundledManifest(k), mod: path + stamp}
	if path != "" {
		data, err := os.ReadFile(path)
		var om *Manifest
		if err == nil {
			om, err = ParseManifest(data)
		}
		switch {
		case err != nil:
			l.warn = fmt.Sprintf("ignored %s: %v", path, err)
		case !om.matches(k):
			l.warn = fmt.Sprintf("ignored %s: id %q is not %s", path, om.ID, k)
		default:
			om.Source = path
			l.m = om
		}
	}
	rulesCache.by[k] = l
	return l.m, l.warn
}

func bundledManifest(k Kind) *Manifest {
	data, err := bundledRules.ReadFile("rules/" + string(k) + ".toml")
	if err != nil {
		return &Manifest{ID: string(k), Source: "bundled"}
	}
	m, err := ParseManifest(data)
	if err != nil {
		panic("lazymux: bundled rules for " + string(k) + ": " + err.Error())
	}
	m.Source = "bundled"
	return m
}

// SortedResults orders explain results by priority, highest first.
func SortedResults(rs []RuleResult) []RuleResult {
	out := append([]RuleResult(nil), rs...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Rule.Priority > out[j].Rule.Priority })
	return out
}

// BundledRules is the text of an agent's bundled manifest, a starting
// point for an override.
func BundledRules(k Kind) string {
	data, _ := bundledRules.ReadFile("rules/" + string(k) + ".toml")
	return string(data)
}

// Explain shows how an agent's rules read a pane: the manifest used, the
// verdict, and every rule (highest priority first) with whether it matched.
func Explain(c *tmux.Client, paneID string) (string, error) {
	t, err := c.ListTree()
	if err != nil {
		return "", err
	}
	var pane *tmux.Pane
	for _, s := range t.Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.ID == paneID {
					pane = p
				}
			}
		}
	}
	if pane == nil {
		return "", fmt.Errorf("no pane %s", paneID)
	}
	k := KindOf(pane.Command)
	if k == "" && isInterpreter(pane.Command) {
		k = kindUnder(pane.PID, processes())
	}
	if k == "" {
		return "", fmt.Errorf("pane %s runs %s, not an agent lazymux knows", paneID, pane.Command)
	}
	screen, err := c.CaptureText(paneID)
	if err != nil {
		return "", err
	}
	m, warn := ManifestFor(k)
	in := Input{Screen: normalize(screen), Title: pane.Title}
	v, results := m.Evaluate(in, true)
	var b strings.Builder
	fmt.Fprintf(&b, "pane %s: %s, rules %s", paneID, k, m.Source)
	if m.Version != "" {
		fmt.Fprintf(&b, " (version %s)", m.Version)
	}
	b.WriteString("\n")
	if warn != "" {
		fmt.Fprintf(&b, "warning: %s\n", warn)
	}
	fmt.Fprintf(&b, "title: %q\n", pane.Title)
	switch {
	case !v.Matched && k == Aider:
		b.WriteString("state: no rule matched (working while the screen changes, else idle)\n")
	case !v.Matched:
		b.WriteString("state: idle (no rule matched)\n")
	case v.Keep:
		fmt.Fprintf(&b, "state: unchanged (rule %s keeps the previous state)\n", v.Rule)
	default:
		fmt.Fprintf(&b, "state: %s (rule %s)\n", v.State, v.Rule)
	}
	b.WriteString("\n")
	for _, r := range SortedResults(results) {
		mark := "  "
		if r.Matched {
			mark = "✓ "
		}
		if r.Matched && r.Rule.ID == v.Rule {
			mark = "▶ "
		}
		state := r.Rule.State
		if state == "" {
			state = "unknown"
		}
		fmt.Fprintf(&b, "%s%5d  %-8s %-28s %s\n", mark, r.Rule.Priority, state, r.Rule.ID, r.Rule.Region)
	}
	return b.String(), nil
}
