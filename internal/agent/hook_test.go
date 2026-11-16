package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeHook is a hook whose clock, sleep and game are under the test's control.
type fakeHook struct {
	Hook
	now     time.Time
	opened  int
	running bool
	// during runs while the hook sleeps out its delay.
	during func()
}

func newFakeHook(t *testing.T, delay time.Duration) *fakeHook {
	f := &fakeHook{now: time.Unix(1_800_000_000, 0)}
	f.Hook = Hook{
		Dir:   testDir(t),
		Delay: delay,
		Now:   func() time.Time { return f.now },
		Sleep: func(d time.Duration) {
			f.now = f.now.Add(d)
			if f.during != nil {
				during := f.during
				f.during = nil
				during()
			}
		},
		Open:    func() error { f.opened++; f.running = true; return nil },
		Running: func() bool { return f.running },
	}
	return f
}

func hookJSON(session string) *strings.Reader {
	return strings.NewReader(`{"session_id":"` + session + `","cwd":"/home/mark/lumon","hook_event_name":"x"}`)
}

func (f *fakeHook) run(t *testing.T, event, session string) {
	t.Helper()
	if err := f.Run(event, hookJSON(session)); err != nil {
		t.Fatalf("%s: %v", event, err)
	}
}

func (f *fakeHook) state(session string) State {
	s, ok := f.Dir.Get(session)
	if !ok {
		return ""
	}
	return s.State
}

// A prompt the agent is still working on after the delay opens the game.
func TestALongPromptOpensTheGame(t *testing.T) {
	f := newFakeHook(t, 20*time.Second)
	f.run(t, "prompt", "s1")
	if f.opened != 1 {
		t.Fatalf("opened %d times", f.opened)
	}
	if f.state("s1") != Working {
		t.Fatalf("state %q", f.state("s1"))
	}
}

// A quick answer never opens anything.
func TestAQuickAnswerOpensNothing(t *testing.T) {
	f := newFakeHook(t, 20*time.Second)
	f.during = func() { f.run(t, "stop", "s1") }
	f.run(t, "prompt", "s1")
	if f.opened != 0 {
		t.Fatalf("opened %d times", f.opened)
	}
	if f.state("s1") != Done {
		t.Fatalf("state %q", f.state("s1"))
	}
}

// When the player sends a second prompt during the first one's wait, the
// second prompt's wait decides, not the first's.
func TestOnlyTheLatestPromptsWaitCounts(t *testing.T) {
	f := newFakeHook(t, 20*time.Second)
	f.during = func() {
		f.run(t, "stop", "s1")
		f.now = f.now.Add(time.Second)
		f.Delay = 0 // the second prompt's own wait is not under test
		f.run(t, "prompt", "s1")
	}
	f.run(t, "prompt", "s1")
	if f.opened != 0 {
		t.Fatalf("the first prompt's wait opened the game")
	}
}

// One game at a time: a game already open is not opened again, and two waits
// ending together open one.
func TestOneGameAtATime(t *testing.T) {
	f := newFakeHook(t, 20*time.Second)
	f.running = true
	f.run(t, "prompt", "s1")
	if f.opened != 0 {
		t.Fatal("opened a second game")
	}

	g := newFakeHook(t, 20*time.Second)
	g.Running = func() bool { return false } // the first window has not started yet
	g.during = func() { g.run(t, "prompt", "s2") }
	g.run(t, "prompt", "s1")
	if g.opened != 1 {
		t.Fatalf("two waits opened %d games", g.opened)
	}
}

// Most agents hold the player's prompt until its hook returns, so the hook
// hands the wait to another process and returns without sleeping. That
// process's wait opens the game.
func TestThePromptHookReturnsAtOnceAndTheWaitOpensTheGame(t *testing.T) {
	f := newFakeHook(t, 20*time.Second)
	var later []string
	f.Later = func(session string) error { later = append(later, session); return nil }
	start := f.now
	f.run(t, "prompt", "s1")
	if !f.now.Equal(start) {
		t.Fatalf("the prompt hook slept %v", f.now.Sub(start))
	}
	if f.opened != 0 || len(later) != 1 || later[0] != "s1" {
		t.Fatalf("opened %d, handed off %v", f.opened, later)
	}
	f.run(t, "wait", "s1")
	if f.opened != 1 {
		t.Fatalf("the wait opened %d games", f.opened)
	}
}

// A wait for a prompt the agent has already answered opens nothing.
func TestAWaitForAnAnsweredPromptOpensNothing(t *testing.T) {
	f := newFakeHook(t, 20*time.Second)
	f.Later = func(string) error { return nil }
	f.run(t, "prompt", "s1")
	f.run(t, "stop", "s1")
	f.run(t, "wait", "s1")
	if f.opened != 0 {
		t.Fatal("opened for a finished prompt")
	}

	g := newFakeHook(t, 20*time.Second)
	g.Later = func(string) error { return nil }
	g.run(t, "prompt", "s1")
	g.during = func() { g.run(t, "stop", "s1") }
	g.run(t, "wait", "s1")
	if g.opened != 0 {
		t.Fatal("opened for a prompt answered during the wait")
	}
}

// With no delay set, the game opens only when asked.
func TestNoDelayMeansOnlyOnRequest(t *testing.T) {
	f := newFakeHook(t, 0)
	f.run(t, "prompt", "s1")
	if f.opened != 0 {
		t.Fatal("opened without being asked")
	}
	if err := f.Run("open", strings.NewReader("")); err != nil {
		t.Fatal(err)
	}
	if f.opened != 1 {
		t.Fatal("open did not open")
	}
}

// The agent's states follow its events, and only for a session that has been
// given a prompt: the idle reminder after a quiet start is not news.
func TestTheAgentsStateFollowsItsEvents(t *testing.T) {
	f := newFakeHook(t, 0)
	f.run(t, "idle", "s1")
	f.run(t, "waiting", "s1")
	if got := f.state("s1"); got != "" {
		t.Fatalf("an unprompted session became %q", got)
	}

	f.run(t, "prompt", "s1")
	f.run(t, "waiting", "s1")
	if got := f.state("s1"); got != Waiting {
		t.Fatalf("after a permission prompt: %q", got)
	}
	// Esc skips Stop; the idle reminder still ends the wait.
	f.run(t, "idle", "s1")
	if got := f.state("s1"); got != Done {
		t.Fatalf("after idle: %q", got)
	}
	f.run(t, "end", "s1")
	if got := f.state("s1"); got != Done {
		t.Fatalf("a finished session's record went with it: %q", got)
	}
	// A session ended mid-work did not finish anything.
	f.run(t, "prompt", "s2")
	f.run(t, "end", "s2")
	if got := f.state("s2"); got != "" {
		t.Fatalf("a session ended mid-work left %q", got)
	}
}

// Claude can end a session a moment after it finishes (claude -p does), before
// the game's next look: the game must still hear that it finished.
func TestAFinishedSessionIsHeardEvenIfItEndsAtOnce(t *testing.T) {
	f := newFakeHook(t, 0)
	w := NewWatch()
	f.run(t, "prompt", "s1")
	w.Next(f.now, f.Dir.Read())
	f.run(t, "stop", "s1")
	f.run(t, "end", "s1")
	if news := w.Next(f.now, f.Dir.Read()); len(news) != 1 || news[0].State != Done {
		t.Fatalf("the game heard %+v", news)
	}
}

// Input that is not what Claude Code sends changes nothing.
func TestBadHookInputChangesNothing(t *testing.T) {
	f := newFakeHook(t, 0)
	for _, in := range []string{"", "nope", `{"session_id":"../x"}`, `{"session_id":""}`} {
		if err := f.Run("prompt", strings.NewReader(in)); err == nil {
			t.Errorf("accepted %q", in)
		}
	}
	if err := f.Run("launch-missiles", hookJSON("s1")); err == nil {
		t.Error("accepted an unknown event")
	}
	if n := len(f.Dir.Read()); n != 0 {
		t.Errorf("%d statuses written", n)
	}
}

// paneUp marks a pane module as running, as of the hook's clock.
func (f *fakeHook) paneUp(t *testing.T, at time.Time) {
	t.Helper()
	path := filepath.Join(string(f.Dir), "pane.alive")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

// With the plugin's pane running in Claude Code, the game is asked for there
// and no window opens; the request carries the mode.
func TestARunningPaneIsAskedInsteadOfAWindow(t *testing.T) {
	f := newFakeHook(t, 0)
	f.Mode = ModeWatch
	f.paneUp(t, f.now)
	if err := f.Run("open", strings.NewReader("")); err != nil {
		t.Fatal(err)
	}
	if f.opened != 0 {
		t.Fatal("a window opened with the pane running")
	}
	at, mode, ok := f.Dir.PaneRequest()
	if !ok || mode != ModeWatch || !at.Equal(f.now) {
		t.Fatalf("request %v %q %v", at, mode, ok)
	}
}

// The screensaver event opens the game on the screensaver, in the pane or a
// window, whatever the plugin's own mode.
func TestTheScreensaverEventOpensOnTheScreensaver(t *testing.T) {
	f := newFakeHook(t, 0)
	f.Mode = ModeWatch
	f.paneUp(t, f.now)
	if err := f.Run("screensaver", strings.NewReader("")); err != nil {
		t.Fatal(err)
	}
	if _, mode, ok := f.Dir.PaneRequest(); !ok || mode != ModeScreensaver {
		t.Fatalf("the pane was asked for %q", mode)
	}

	g := newFakeHook(t, 0)
	if err := g.Run("screensaver", strings.NewReader("")); err != nil {
		t.Fatal(err)
	}
	if g.opened != 1 {
		t.Fatalf("opened %d windows", g.opened)
	}
}

// A request the pane module cannot read as one of the modes is play.
func TestAnUnknownPaneModeIsPlay(t *testing.T) {
	f := newFakeHook(t, 0)
	if err := f.Dir.RequestPane(f.now, "dance"); err != nil {
		t.Fatal(err)
	}
	if _, mode, _ := f.Dir.PaneRequest(); mode != ModePlay {
		t.Fatalf("mode %q", mode)
	}
}

// A pane that stopped answering is gone: the window opens as before.
func TestAStalePaneGetsAWindow(t *testing.T) {
	f := newFakeHook(t, 0)
	f.paneUp(t, f.now.Add(-time.Minute))
	if err := f.Run("open", strings.NewReader("")); err != nil {
		t.Fatal(err)
	}
	if f.opened != 1 {
		t.Fatal("no window for a stale pane")
	}
	if _, _, ok := f.Dir.PaneRequest(); ok {
		t.Fatal("a request was left for a pane that is gone")
	}
}

// window is the pane's own fallback, where Claude Code would not place it: it
// always opens a window, unless a game is already open.
func TestWindowAlwaysOpensAWindow(t *testing.T) {
	f := newFakeHook(t, 0)
	f.paneUp(t, f.now)
	f.Run("open", strings.NewReader("")) // the request, which took the launch
	if err := f.Run("window", strings.NewReader("")); err != nil {
		t.Fatal(err)
	}
	if f.opened != 1 {
		t.Fatalf("window opened %d windows", f.opened)
	}
	f.Run("window", strings.NewReader(""))
	if f.opened != 1 {
		t.Fatal("window opened a second game")
	}
}
