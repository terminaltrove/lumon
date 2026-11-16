package agent

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// OffFile is the switch, in the status directory, that turns the plugins off
// for every agent at once: /lumon:off, /lumon:snooze and /lumon:on write and
// remove it, and so can the player by hand. Empty, it is off until turned
// back on; holding a Unix time, it is off until then. Anything else is off,
// since it was put there to stop the game.
const OffFile = "off"

// Off reports whether the plugins are turned off at now. A switch whose time
// has passed is removed, so the plugins are back on by themselves.
func (d Dir) Off(now time.Time) bool {
	path := filepath.Join(string(d), OffFile)
	b, err := os.ReadFile(path)
	if err != nil {
		return !os.IsNotExist(err)
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return true
	}
	until, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return true
	}
	if now.Unix() < until {
		return true
	}
	os.Remove(path)
	return false
}
