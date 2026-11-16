package agent

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func turnOff(t *testing.T, d Dir, until string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(string(d), OffFile), []byte(until), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Turned off, a long prompt neither opens the game nor shows in the strip.
func TestOffMeansNothingOpens(t *testing.T) {
	f := newFakeHook(t, 20*time.Second)
	turnOff(t, f.Dir, "")

	f.during = func() { t.Error("a prompt waited out its delay while the plugins were off") }
	f.run(t, "prompt", "s1")
	f.run(t, "wait", "s1")
	if f.opened != 0 {
		t.Errorf("the game opened %d times while the plugins were off", f.opened)
	}
	if got := f.state("s1"); got != "" {
		t.Errorf("a session was recorded as %q while the plugins were off", got)
	}
}

// Asking for the game is not the game opening on its own, so it still works.
func TestAskingStillOpensTheGameWhileOff(t *testing.T) {
	f := newFakeHook(t, 20*time.Second)
	turnOff(t, f.Dir, "")
	if err := f.Run("open", nil); err != nil {
		t.Fatal(err)
	}
	if f.opened != 1 {
		t.Errorf("asking opened the game %d times, want 1", f.opened)
	}
}

// A snooze holds a time; once it has passed the plugins are back on and the
// switch is gone.
func TestASnoozeEndsWhenItsTimeHasPassed(t *testing.T) {
	f := newFakeHook(t, 20*time.Second)
	turnOff(t, f.Dir, strconv.FormatInt(f.now.Add(time.Hour).Unix(), 10))

	f.run(t, "prompt", "s1")
	if f.opened != 0 || f.state("s1") != "" {
		t.Fatal("a snoozed plugin acted on a prompt")
	}

	f.now = f.now.Add(2 * time.Hour)
	f.run(t, "prompt", "s2")
	if f.opened != 1 {
		t.Errorf("after the snooze a long prompt opened the game %d times, want 1", f.opened)
	}
	if _, err := os.Stat(filepath.Join(string(f.Dir), OffFile)); !os.IsNotExist(err) {
		t.Errorf("the spent snooze is still there: %v", err)
	}
}

// A switch nobody can read is off, not on: it was put there to stop the game.
func TestAnUnreadableSwitchIsOff(t *testing.T) {
	f := newFakeHook(t, 20*time.Second)
	turnOff(t, f.Dir, "tomorrow")
	f.run(t, "prompt", "s1")
	if f.opened != 0 {
		t.Error("a switch holding no time let the game open")
	}
}

// Turned off while a prompt waits out its delay, the game does not open when
// the delay ends.
func TestTurningOffMidWaitStopsTheOpen(t *testing.T) {
	f := newFakeHook(t, 20*time.Second)
	f.during = func() { turnOff(t, f.Dir, "") }
	f.run(t, "prompt", "s1")
	if f.opened != 0 {
		t.Errorf("the game opened %d times after the plugins were turned off", f.opened)
	}
}
