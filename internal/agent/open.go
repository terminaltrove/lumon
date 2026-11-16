package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

var (
	// ErrNoTerminal means no way to open a pane or window was found here.
	ErrNoTerminal = errors.New("no terminal to open the game in")

	// ErrUnsafeArgument means an argument holds a character that a shell or
	// AppleScript string could read as more than text.
	ErrUnsafeArgument = errors.New("an argument could escape its quotes")
)

// Opener opens the game beside the agent: a pane of the multiplexer the agent
// runs in, otherwise a window of the same terminal. Its fields are the outside
// world, so tests can record instead of run.
type Opener struct {
	GOOS   string
	Getenv func(string) string

	// Run runs a program to completion and returns its standard output.
	Run func(name string, args ...string) ([]byte, error)

	// Start starts a program that outlives this process.
	Start func(name string, args ...string) error
}

// Open opens game, a program and its arguments.
func (o Opener) Open(game []string) error {
	for _, a := range game {
		if !safe(a) {
			return fmt.Errorf("%w: %q", ErrUnsafeArgument, a)
		}
	}
	env := o.Getenv

	if pane := env("HERDR_PANE_ID"); pane != "" {
		return o.herdr(pane, game)
	}
	if env("TMUX") != "" {
		args := []string{"split-window", "-h"}
		if pane := env("TMUX_PANE"); pane != "" {
			args = append(args, "-t", pane)
		}
		_, err := o.Run("tmux", append(args, game...)...)
		return err
	}

	switch o.GOOS {
	case "darwin":
		return o.mac(game)
	case "linux", "freebsd", "openbsd", "netbsd":
		term := env("TERMINAL")
		if term == "" || !safe(term) {
			term = "x-terminal-emulator"
		}
		return o.Start(term, append([]string{"-e"}, game...)...)
	}
	return ErrNoTerminal
}

func (o Opener) mac(game []string) error {
	env := o.Getenv
	switch {
	case env("TERM_PROGRAM") == "iTerm.app":
		return o.osascript("iTerm", "create window with default profile command "+applescript(shellLine(game)))
	case env("TERM_PROGRAM") == "ghostty" || env("__CFBundleIdentifier") == "com.mitchellh.ghostty":
		_, err := o.Run("open", append([]string{"-na", "Ghostty", "--args", "-e"}, game...)...)
		return err
	case env("TERM_PROGRAM") == "WezTerm":
		_, err := o.Run("open", append([]string{"-na", "WezTerm", "--args", "start", "--"}, game...)...)
		return err
	case env("KITTY_WINDOW_ID") != "":
		_, err := o.Run("open", append([]string{"-na", "kitty", "--args"}, game...)...)
		return err
	}
	// Terminal comes with every Mac, so it is the fallback.
	return o.osascript("Terminal", "do script "+applescript("exec "+shellLine(game)))
}

func (o Opener) osascript(app, command string) error {
	_, err := o.Run("osascript",
		"-e", `tell application "`+app+`"`,
		"-e", "activate",
		"-e", command,
		"-e", "end tell")
	return err
}

// herdr splits the agent's pane and types the game's command into the new
// pane's shell. exec closes the pane when the game quits.
func (o Opener) herdr(pane string, game []string) error {
	bin := o.Getenv("HERDR_BIN_PATH")
	if !filepath.IsAbs(bin) || !safe(bin) {
		bin = "herdr"
	}
	out, err := o.Run(bin, "pane", "split", "--pane", pane, "--direction", "right", "--focus")
	if err != nil {
		return err
	}
	var reply struct {
		Result struct {
			Pane struct {
				ID string `json:"pane_id"`
			} `json:"pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &reply); err != nil {
		return fmt.Errorf("herdr's reply: %w", err)
	}
	if !paneID(reply.Result.Pane.ID) {
		return fmt.Errorf("herdr named no usable pane: %q", reply.Result.Pane.ID)
	}
	_, err = o.Run(bin, "pane", "run", reply.Result.Pane.ID, "exec "+shellLine(game))
	return err
}

// safe reports whether s can be single-quoted for a shell and then put in an
// AppleScript string unchanged: letters, digits, spaces and a few path marks.
func safe(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(" /._-:+,@=%~", r) {
			continue
		}
		return false
	}
	return true
}

func paneID(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == ':' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// shellLine single-quotes each argument; safe ones hold no quote to escape.
func shellLine(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = "'" + a + "'"
	}
	return strings.Join(q, " ")
}

// applescript quotes s as an AppleScript string; safe text holds no quote or
// backslash to escape.
func applescript(s string) string { return `"` + s + `"` }
