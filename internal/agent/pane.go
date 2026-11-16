package agent

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The plugin's pane module (plugins/claude-code/hooks/pane.tsx) shows the game
// beside the chat in Claude Code. While it runs it touches pane.alive every
// second; a hook that would open the game then leaves pane.request for it
// instead of opening a window. The module cannot delete a file, so the request
// carries its time and the module takes each one once.

// paneFresh is how recently pane.alive must have been touched for the module
// to count as running.
const paneFresh = 5 * time.Second

func (d Dir) paneFile(name string) string { return filepath.Join(string(d), name) }

// PaneAlive reports whether the pane module touched pane.alive lately.
func (d Dir) PaneAlive(now time.Time) bool {
	info, err := os.Stat(d.paneFile("pane.alive"))
	if err != nil {
		return false
	}
	age := now.Sub(info.ModTime())
	return age > -paneFresh && age < paneFresh
}

// How a game opened for an agent starts: at a desk, watching the computer
// refiners, or on the screensaver.
const (
	ModePlay        = "play"
	ModeWatch       = "watch"
	ModeScreensaver = "screensaver"
)

// knownMode is mode if it is one of the three, else ModePlay.
func knownMode(mode string) string {
	switch mode {
	case ModeWatch, ModeScreensaver:
		return mode
	}
	return ModePlay
}

// RequestPane asks the pane module to open the game in mode.
func (d Dir) RequestPane(now time.Time, mode string) error {
	mode = knownMode(mode)
	tmp, err := os.CreateTemp(string(d), ".tmp-*")
	if err != nil {
		return err
	}
	_, werr := tmp.WriteString(strconv.FormatInt(now.UnixNano(), 10) + " " + mode + "\n")
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		os.Remove(tmp.Name())
		return werr
	}
	return os.Rename(tmp.Name(), d.paneFile("pane.request"))
}

// PaneRequest reads the last request: when it was made and its mode.
func (d Dir) PaneRequest() (at time.Time, mode string, ok bool) {
	data, err := os.ReadFile(d.paneFile("pane.request"))
	if err != nil {
		return time.Time{}, "", false
	}
	f := strings.Fields(string(data))
	if len(f) != 2 {
		return time.Time{}, "", false
	}
	n, err := strconv.ParseInt(f[0], 10, 64)
	if err != nil {
		return time.Time{}, "", false
	}
	return time.Unix(0, n), knownMode(f[1]), true
}
