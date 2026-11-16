package agent

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fakeOpener records what would have run.
type fakeOpener struct {
	Opener
	ran     [][]string
	started [][]string
}

func newFakeOpener(goos string, env map[string]string) *fakeOpener {
	f := &fakeOpener{}
	f.Opener = Opener{
		GOOS:   goos,
		Getenv: func(k string) string { return env[k] },
		Run: func(name string, args ...string) ([]byte, error) {
			f.ran = append(f.ran, append([]string{name}, args...))
			if len(args) > 1 && args[0] == "pane" && args[1] == "split" {
				return []byte(`{"id":"1","result":{"pane":{"pane_id":"wG:p9"}}}`), nil
			}
			return nil, nil
		},
		Start: func(name string, args ...string) error {
			f.started = append(f.started, append([]string{name}, args...))
			return nil
		},
	}
	return f
}

var game = []string{"/Users/mark/Library/plugin data/lumon", "--no-intro", "--agent-status", "/Users/mark/status"}

const quoted = "'/Users/mark/Library/plugin data/lumon' '--no-intro' '--agent-status' '/Users/mark/status'"

// Each place an agent runs gets the game beside it: a pane in a multiplexer,
// otherwise a window of the same terminal.
func TestTheGameOpensBesideTheAgent(t *testing.T) {
	for _, tc := range []struct {
		name    string
		goos    string
		env     map[string]string
		ran     [][]string
		started [][]string
	}{
		{
			name: "herdr", goos: "darwin",
			env: map[string]string{"HERDR_PANE_ID": "wG:pQ", "HERDR_BIN_PATH": "/opt/herdr", "TERM_PROGRAM": "herdr"},
			ran: [][]string{
				{"/opt/herdr", "pane", "split", "--pane", "wG:pQ", "--direction", "right", "--focus"},
				{"/opt/herdr", "pane", "run", "wG:p9", "exec " + quoted},
			},
		},
		{
			name: "tmux", goos: "linux",
			env: map[string]string{"TMUX": "/tmp/tmux-1/default,1,0", "TMUX_PANE": "%3"},
			ran: [][]string{append([]string{"tmux", "split-window", "-h", "-t", "%3"}, game...)},
		},
		{
			name: "Terminal", goos: "darwin",
			env: map[string]string{"TERM_PROGRAM": "Apple_Terminal"},
			ran: [][]string{{"osascript",
				"-e", `tell application "Terminal"`,
				"-e", "activate",
				"-e", `do script "exec ` + quoted + `"`,
				"-e", "end tell"}},
		},
		{
			name: "iTerm2", goos: "darwin",
			env: map[string]string{"TERM_PROGRAM": "iTerm.app"},
			ran: [][]string{{"osascript",
				"-e", `tell application "iTerm"`,
				"-e", "activate",
				"-e", `create window with default profile command "` + quoted + `"`,
				"-e", "end tell"}},
		},
		{
			name: "Ghostty", goos: "darwin",
			env: map[string]string{"TERM_PROGRAM": "ghostty"},
			ran: [][]string{append([]string{"open", "-na", "Ghostty", "--args", "-e"}, game...)},
		},
		{
			name: "WezTerm", goos: "darwin",
			env: map[string]string{"TERM_PROGRAM": "WezTerm"},
			ran: [][]string{append([]string{"open", "-na", "WezTerm", "--args", "start", "--"}, game...)},
		},
		{
			name: "some other Mac terminal", goos: "darwin",
			env: map[string]string{"TERM_PROGRAM": "vscode"},
			ran: [][]string{{"osascript",
				"-e", `tell application "Terminal"`,
				"-e", "activate",
				"-e", `do script "exec ` + quoted + `"`,
				"-e", "end tell"}},
		},
		{
			name: "Linux with $TERMINAL", goos: "linux",
			env:     map[string]string{"TERMINAL": "foot"},
			started: [][]string{append([]string{"foot", "-e"}, game...)},
		},
		{
			name: "Linux", goos: "linux",
			env:     map[string]string{},
			started: [][]string{append([]string{"x-terminal-emulator", "-e"}, game...)},
		},
	} {
		f := newFakeOpener(tc.goos, tc.env)
		if err := f.Open(game); err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if !reflect.DeepEqual(f.ran, tc.ran) || !reflect.DeepEqual(f.started, tc.started) {
			t.Errorf("%s:\n ran     %q\n want    %q\n started %q\n want    %q", tc.name, f.ran, tc.ran, f.started, tc.started)
		}
	}
}

// Some openers hand the command to a shell or AppleScript, so an argument
// that could break out of its quotes is refused and nothing runs.
func TestAnArgumentThatCouldEscapeItsQuotesOpensNothing(t *testing.T) {
	for _, bad := range []string{`/a'b`, `/a"b`, `/a\b`, "/a\nb", "/a$(b)", "/a`b`"} {
		f := newFakeOpener("darwin", map[string]string{"TERM_PROGRAM": "Apple_Terminal"})
		err := f.Open([]string{bad, "--no-intro"})
		if !errors.Is(err, ErrUnsafeArgument) {
			t.Errorf("%q: err %v", bad, err)
		}
		if len(f.ran)+len(f.started) != 0 {
			t.Errorf("%q: ran %q", bad, f.ran)
		}
	}
}

// Without a terminal to open, it says so and runs nothing.
func TestWithNoTerminalNothingRuns(t *testing.T) {
	f := newFakeOpener("windows", map[string]string{})
	if err := f.Open(game); !errors.Is(err, ErrNoTerminal) {
		t.Fatalf("err %v", err)
	}
	if len(f.ran)+len(f.started) != 0 {
		t.Fatalf("ran %q", f.ran)
	}
}

// herdr's reply names the new pane; a reply without one runs nothing in it.
func TestABadHerdrReplyRunsNothing(t *testing.T) {
	f := newFakeOpener("darwin", map[string]string{"HERDR_PANE_ID": "wG:pQ", "HERDR_BIN_PATH": "/opt/herdr"})
	f.Run = func(name string, args ...string) ([]byte, error) {
		f.ran = append(f.ran, append([]string{name}, args...))
		return []byte(`{"result":{"pane":{"pane_id":"x; rm -rf ~"}}}`), nil
	}
	if err := f.Open(game); err == nil {
		t.Fatal("ran in a pane with a strange id")
	}
	for _, r := range f.ran {
		if strings.Contains(strings.Join(r, " "), "pane run") {
			t.Fatalf("ran %q", r)
		}
	}
}
