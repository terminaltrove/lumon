package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Hook handles one event from the agent. Its fields are the outside world, so
// tests can stand in for the clock, the sleep and the window.
type Hook struct {
	Dir Dir

	// Delay is how long a prompt must keep the agent working before the game
	// opens on its own; zero opens it only on request.
	Delay time.Duration

	Now     func() time.Time
	Sleep   func(time.Duration)
	Open    func() error // opens the game in a pane or window
	Running func() bool  // whether a game is already open

	// Mode is how a game opened starts: ModePlay, ModeWatch or
	// ModeScreensaver. It is passed on to the pane.
	Mode string

	// Later hands a prompt's wait to another process, which runs the wait
	// event for the session. Most agents wait for their prompt hook to return,
	// so it must not sleep. Nil waits here.
	Later func(session string) error
}

// input is the part of a hook's JSON that is read. The prompt's text is never
// looked at. Claude Code, Pi and OpenCode send session_id and cwd.
type input struct {
	Session string `json:"session_id"`
	Cwd     string `json:"cwd"`

	// The tool event's call. Only its name and a few fields of its input are
	// read; see Activity.
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

// inputMax bounds what is read of a hook's JSON. A tool call can carry a whole
// file being written, so the tool event reads more; a call past that is
// skipped.
const (
	inputMax     = 64 << 10
	toolInputMax = 8 << 20
)

// approvalHold is how long a wait for approval ignores tool calls. The agent
// sends the call's tool event and the approval's together, so the call's can
// land second.
const approvalHold = 3 * time.Second

// Pruned is how long a session's status lasts without news.
const Pruned = 24 * time.Hour

// Run handles event, reading the hook's JSON from in. The events are:
//
//	prompt   the player sent a prompt: working, and maybe open the game
//	waiting  the agent wants approval
//	stop     the agent finished
//	idle     the agent has sat finished a while (it also ends a wait cut short by Esc)
//	tool     the agent is starting a tool call: what it is doing now
//	start    a session began: sweep stale statuses
//	end      a session ended: forget it, unless it finished
//	open     open the game now (no JSON)
//	watch    open it now, to watch the computer refiners (no JSON)
//	screensaver
//	         open it now, on the screensaver (no JSON)
//	window   open it in a window even with the pane running: the pane's
//	         fallback where Claude Code would not place it (no JSON)
//	wait     sit out a prompt's delay, then open the game if the session is
//	         still on that prompt: what Later starts
func (h Hook) Run(event string, in io.Reader) error {
	switch event {
	case "open":
		return h.open()
	case "window":
		if h.Running() {
			return nil
		}
		return h.Open()
	case "watch":
		h.Mode = ModeWatch
		return h.open()
	case "screensaver":
		h.Mode = ModeScreensaver
		return h.open()
	case "prompt", "waiting", "stop", "idle", "start", "end", "wait", "tool":
		// Turned off, the game neither opens on its own nor follows the
		// agent. Asking for it, above, still opens it. See OffFile.
		if h.Dir.Off(h.Now()) {
			return nil
		}
	default:
		return fmt.Errorf("no hook event %q", event)
	}

	limit := int64(inputMax)
	if event == "tool" {
		limit = toolInputMax
	}
	var got input
	if err := json.NewDecoder(io.LimitReader(in, limit)).Decode(&got); err != nil {
		return fmt.Errorf("reading the hook's input: %w", err)
	}
	if !ValidSession(got.Session) {
		return errors.New("the hook's input has no usable session id")
	}

	switch event {
	case "prompt":
		return h.prompt(got)
	case "wait":
		return h.wait(got.Session)
	case "tool":
		return h.tool(got)
	case "waiting":
		return h.move(got.Session, Waiting, Working)
	case "stop":
		return h.move(got.Session, Done, Working, Waiting)
	case "idle":
		return h.move(got.Session, Done, Working, Waiting)
	case "start":
		h.Dir.Prune(h.Now(), Pruned)
	case "end":
		// A finished session's record stays for the game's next look, since
		// Claude may end the session a moment after Stop; Prune sweeps it.
		if s, ok := h.Dir.Get(got.Session); !ok || s.State != Done {
			h.Dir.Remove(got.Session)
		}
	}
	return nil
}

func (h Hook) prompt(got input) error {
	at := h.Now()
	if err := h.Dir.Write(Status{Session: got.Session, State: Working, Project: Label(got.Cwd), At: at, Since: at}); err != nil {
		return err
	}
	if h.Delay <= 0 {
		return nil
	}
	if h.Later != nil {
		return h.Later(got.Session)
	}
	return h.wait(got.Session)
}

// wait sleeps out the delay and opens the game if the session is still on the
// prompt it was on when the wait began.
func (h Hook) wait(session string) error {
	if h.Delay <= 0 {
		return nil
	}
	before, ok := h.Dir.Get(session)
	if !ok || before.State != Working {
		return nil
	}
	h.Sleep(h.Delay)

	s, ok := h.Dir.Get(session)
	if !ok || s.State != Working || !s.sameTurn(before) || h.Dir.Off(h.Now()) {
		return nil
	}
	return h.open()
}

// sameTurn reports whether two records are of one prompt. A tool call moves At
// on, so the prompt's own time decides; a record from an older release has
// only At.
func (s Status) sameTurn(o Status) bool {
	if !s.Since.IsZero() || !o.Since.IsZero() {
		return s.Since.Equal(o.Since)
	}
	return s.At.Equal(o.At)
}

// tool records the step a working session is on. A session waiting for
// approval that starts a tool call has been given it. A finished session's is
// a late arrival, since the shim runs this in the background.
func (h Hook) tool(got input) error {
	s, ok := h.Dir.Get(got.Session)
	if !ok {
		return nil
	}
	now := h.Now()
	switch s.State {
	case Working:
	case Waiting:
		if now.Sub(s.At) < approvalHold {
			return nil
		}
		s.State = Working
	default:
		return nil
	}
	doing, edited := Activity(got.ToolName, got.ToolInput)
	s.Doing, s.At = doing, now
	s.Steps++
	if edited != "" {
		s.Edits = addEdit(s.Edits, editHash(edited))
	}
	return h.Dir.Write(s)
}

func addEdit(edits []uint32, h uint32) []uint32 {
	for _, e := range edits {
		if e == h {
			return edits
		}
	}
	if len(edits) >= editsMax {
		return edits
	}
	return append(edits, h)
}

// move changes a session's state, but only from one of the states given: a
// session never prompted has nothing to finish.
func (h Hook) move(session string, to State, from ...State) error {
	s, ok := h.Dir.Get(session)
	if !ok {
		return nil
	}
	for _, f := range from {
		if s.State == f {
			s.State, s.At = to, h.Now()
			return h.Dir.Write(s)
		}
	}
	return nil
}

func (h Hook) open() error {
	if h.Running() || !h.Dir.claimLaunch(h.Now()) {
		return nil
	}
	if h.Dir.PaneAlive(h.Now()) {
		return h.Dir.RequestPane(h.Now(), h.Mode)
	}
	return h.Open()
}

// launchHold is how long one launch keeps others off while its window starts
// and the game inside it records that it is running.
const launchHold = 15 * time.Second

// claimLaunch reports whether this caller may open a window, so two waits that
// end together open one game.
func (d Dir) claimLaunch(now time.Time) bool {
	path := filepath.Join(string(d), "launch.lock")
	for range 2 {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			f.Close()
			// The lock's age is read from its own time, which tests control.
			os.Chtimes(path, now, now)
			return true
		}
		info, err := os.Stat(path)
		if err != nil || now.Sub(info.ModTime()) < launchHold {
			return false
		}
		os.Remove(path)
	}
	return false
}
