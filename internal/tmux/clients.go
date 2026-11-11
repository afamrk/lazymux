package tmux

import (
	"strconv"
	"strings"
	"time"
)

// WatchIdle is how long after your last key a client still counts as
// watched (without focus-events tmux cannot tell whether the terminal is in
// front of you, so recent typing stands in for it).
const WatchIdle = 2 * time.Minute

// WatchedPanes returns the panes you are looking at: the current pane of
// each attached client that you typed into within WatchIdle and, when
// focus-events is on, whose terminal has focus.
func (c *Client) WatchedPanes(now time.Time) map[string]bool {
	out, err := c.Run("list-clients", "-F", "#{pane_id}\t#{client_flags}\t#{client_activity}")
	if err != nil {
		return nil
	}
	fe, _ := c.Run("show-options", "-gv", "focus-events")
	return parseWatched(out, fe == "on", now)
}

func parseWatched(out string, focusEvents bool, now time.Time) map[string]bool {
	watched := map[string]bool{}
	for _, l := range strings.Split(out, "\n") {
		f := strings.Split(l, "\t")
		if len(f) != 3 || f[0] == "" {
			continue
		}
		if focusEvents && !hasFlag(f[1], "focused") {
			continue
		}
		act, err := strconv.ParseInt(f[2], 10, 64)
		if err != nil || now.Sub(time.Unix(act, 0)) > WatchIdle {
			continue
		}
		watched[f[0]] = true
	}
	return watched
}

func hasFlag(flags, flag string) bool {
	for _, f := range strings.Split(flags, ",") {
		if f == flag {
			return true
		}
	}
	return false
}
