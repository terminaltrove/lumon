package agent

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func (d Dir) gameFile() string { return filepath.Join(string(d), "game.pid") }

// MarkGame records that the game in process pid is open, and returns the
// function that clears the record when it closes.
func (d Dir) MarkGame(pid int) (done func()) {
	os.WriteFile(d.gameFile(), []byte(strconv.Itoa(pid)), 0o600)
	return func() { os.Remove(d.gameFile()) }
}

// GameRunning reports whether a game recorded by MarkGame is still open.
func (d Dir) GameRunning() bool {
	data, err := os.ReadFile(d.gameFile())
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	return err == nil && pid > 0 && alive(pid)
}
