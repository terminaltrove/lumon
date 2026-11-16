package lumon

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// The Clock turns the measured duration into a bounded number of fixed steps, so
// a busy server drops frames. Given a constant, it would advance one step per
// delivered frame and a slow server would slow the game for everybody on it.
func TestAFrameCarriesTheTimeItActuallyTook(t *testing.T) {
	// A frame that took a hundred milliseconds to draw.
	slow := time.Now().Add(-100 * time.Millisecond)

	msg := frameCmd(slow)()
	got, ok := msg.(FrameMsg)
	if !ok {
		t.Fatalf("the frame command produced %T, want a FrameMsg", msg)
	}

	if time.Duration(got) < 100*time.Millisecond {
		t.Errorf("a frame that took 100ms reported %v; the elapsed time is being "+
			"discarded and the board will run slow under load", time.Duration(got))
	}

	// The first frame of a session has nothing to measure against, so it gets the
	// nominal interval, not the time since the epoch.
	first, ok := frameCmd(time.Time{})().(FrameMsg)
	if !ok {
		t.Fatal("the first frame produced no FrameMsg")
	}
	if time.Duration(first) != frameRate {
		t.Errorf("the first frame reported %v, want the nominal %v",
			time.Duration(first), frameRate)
	}
}

func TestAFrameAsksForTheNextOne(t *testing.T) {
	m := ready()
	_, cmd := m.Update(FrameMsg(frameInterval))
	if cmd == nil {
		t.Fatal("handling a frame did not ask for another")
	}
	if _, ok := cmd().(FrameMsg); !ok {
		t.Error("the follow-up command does not produce a frame")
	}
	var _ tea.Cmd = cmd
}

// Halving the frame rate halves the rows Bubble Tea writes, so a small host can
// serve twice the people. Physics runs at a fixed DT, so the same seed and keys
// give the same board at any rate.
func TestTheFrameRateIsADialOnCostAndNotOnTheGame(t *testing.T) {
	defer restoreFrameRate(t)

	if err := SetFrameRate(30); err != nil {
		t.Fatalf("30 fps refused: %v", err)
	}
	if got, want := FrameRate(), time.Second/30; got != want {
		t.Errorf("30 fps asks for a frame every %v, want %v", got, want)
	}

	// Twice the interval is half the frames, and so half the wire.
	SetFrameRate(60)
	sixty := FrameRate()
	SetFrameRate(30)
	// Within a nanosecond, because second/60 and second/30 are each truncated
	// and twice the first is one nanosecond short of the second.
	if d := FrameRate() - 2*sixty; d < -time.Nanosecond || d > time.Nanosecond {
		t.Errorf("30 fps is %v against 60's %v; halving the rate must double the "+
			"interval or it buys nothing", FrameRate(), sixty)
	}

	// The physics step is not the frame, and never follows it.
	if physicsDT := time.Second / 60; FrameRate() == physicsDT {
		t.Error("the frame interval and the physics step have become the same " +
			"number; the simulation is about to depend on the renderer")
	}
}

// Above Bubble Tea's own maximum the extra frames are built and discarded
// unsent; below the floor the ambient motion reads as a fault.
func TestAnUnservableFrameRateIsRefused(t *testing.T) {
	defer restoreFrameRate(t)

	for _, fps := range []int{0, -1, MinFPS - 1, MaxFPS + 1, 1000} {
		if err := SetFrameRate(fps); err == nil {
			t.Errorf("%d fps was accepted; it is outside %d-%d", fps, MinFPS, MaxFPS)
		}
	}

	// A refusal must leave the rate unchanged.
	before := FrameRate()
	SetFrameRate(MaxFPS + 1)
	if FrameRate() != before {
		t.Errorf("a refused rate still changed the interval to %v", FrameRate())
	}

	for _, fps := range []int{MinFPS, 30, 60, MaxFPS} {
		if err := SetFrameRate(fps); err != nil {
			t.Errorf("%d fps refused: %v", fps, err)
		}
	}
}

// restoreFrameRate puts the package back as it found it. frameRate is process
// wide (it is the host's budget), so a test that changes it must restore the
// default for the tests after it.
func restoreFrameRate(t *testing.T) {
	t.Helper()
	if err := SetFrameRate(60); err != nil {
		t.Fatalf("could not restore the default frame rate: %v", err)
	}
}
