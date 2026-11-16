package physics

import (
	"testing"
	"time"
)

// stepCounter records every dt the clock steps with.
type stepCounter struct{ dts []float64 }

func (s *stepCounter) step(dt float64) { s.dts = append(s.dts, dt) }
func (s *stepCounter) count() int      { return len(s.dts) }

func secs(f float64) time.Duration { return time.Duration(f * float64(time.Second)) }

func TestClockRunsOneStepPerFixedInterval(t *testing.T) {
	c := NewClock()
	var s stepCounter

	c.Tick(secs(4*DT), s.step)

	if s.count() != 4 {
		t.Errorf("steps = %d, want 4", s.count())
	}
}

func TestClockHandsEveryStepTheFixedTimestep(t *testing.T) {
	c := NewClock()
	var s stepCounter

	c.Tick(secs(3*DT), s.step)

	for i, dt := range s.dts {
		closeTo(t, dt, DT, "dt of step")
		_ = i
	}
}

// A short frame runs no steps, not a tiny inaccurate one.
func TestClockRunsNoStepsForLessThanOneInterval(t *testing.T) {
	c := NewClock()
	var s stepCounter

	c.Tick(secs(DT*0.4), s.step)

	if s.count() != 0 {
		t.Errorf("steps = %d, want 0", s.count())
	}
}

// Otherwise the simulation runs slow.
func TestClockCarriesRemainderIntoTheNextTick(t *testing.T) {
	c := NewClock()
	var s stepCounter

	c.Tick(secs(DT*0.6), s.step)
	c.Tick(secs(DT*0.6), s.step)

	if s.count() != 1 {
		t.Errorf("steps across two 0.6-interval ticks = %d, want 1", s.count())
	}
}

func TestClockTimeScaleStretchesSimulatedTime(t *testing.T) {
	fast := NewClock()
	fast.TimeScale = 2
	var f stepCounter
	fast.Tick(secs(2*DT), f.step)

	slow := NewClock()
	slow.TimeScale = 0.5
	var s stepCounter
	slow.Tick(secs(2*DT), s.step)

	if f.count() != 4 {
		t.Errorf("2x steps for 2 intervals = %d, want 4", f.count())
	}
	if s.count() != 1 {
		t.Errorf("0.5x steps for 2 intervals = %d, want 1", s.count())
	}
}

func TestClockPausedRunsNoSteps(t *testing.T) {
	c := NewClock()
	c.Paused = true
	var s stepCounter

	c.Tick(secs(4*DT), s.step)

	if s.count() != 0 {
		t.Errorf("steps while paused = %d, want 0", s.count())
	}
}

// Otherwise unpausing fast-forwards through the paused time.
func TestClockPausedDiscardsAccumulatedTime(t *testing.T) {
	c := NewClock()
	c.Paused = true
	var s stepCounter

	c.Tick(secs(4*DT), s.step)
	c.Paused = false
	c.Tick(secs(DT), s.step)

	if s.count() != 1 {
		t.Errorf("steps after unpausing = %d, want 1", s.count())
	}
}

func TestClockStepOnceAdvancesASingleStepWhilePaused(t *testing.T) {
	c := NewClock()
	c.Paused = true
	var s stepCounter

	c.StepOnce()
	c.Tick(secs(0), s.step)

	if s.count() != 1 {
		t.Errorf("steps = %d, want 1", s.count())
	}

	c.Tick(secs(0), s.step)
	if s.count() != 1 {
		t.Errorf("StepOnce fired twice; steps = %d, want 1", s.count())
	}
}

// A long stall (a suspended laptop, a slow first frame) must not queue
// thousands of steps.
func TestClockCapsBacklogAtMaxCatchupSteps(t *testing.T) {
	c := NewClock()
	var s stepCounter

	c.Tick(10*time.Second, s.step)

	if s.count() != c.MaxCatchupSteps {
		t.Errorf("steps after a 10s stall = %d, want the cap %d", s.count(), c.MaxCatchupSteps)
	}
}

// When steps are too slow, the game goes into slow motion instead of stalling
// input and rendering: once the wall-clock budget is spent the backlog is
// dropped.
func TestClockAbandonsBacklogOnceWallClockBudgetIsSpent(t *testing.T) {
	c := NewClock()
	c.MaxCatchupSteps = 20 // let a backlog build, so the budget is what cuts it
	c.Budget = 10 * time.Millisecond

	// A fake clock that advances 4ms of wall time per step, so the 10ms budget
	// is spent after three steps.
	fake := time.Unix(0, 0)
	c.now = func() time.Time { return fake }
	var s stepCounter
	slowStep := func(dt float64) {
		s.step(dt)
		fake = fake.Add(4 * time.Millisecond)
	}

	c.Tick(secs(20*DT), slowStep)

	// The check runs after each step, so the third step trips it.
	if s.count() != 3 {
		t.Errorf("steps = %d, want 3 before the 10ms budget was spent", s.count())
	}

	// The next tick starts clean.
	var next stepCounter
	c.now = func() time.Time { return fake }
	c.Tick(secs(DT), next.step)
	if next.count() != 1 {
		t.Errorf("next tick ran %d steps, want 1 (backlog should have been dropped)", next.count())
	}
}

func TestClockWithoutBudgetRunsTheFullBacklog(t *testing.T) {
	c := NewClock()
	c.Budget = 0 // no wall-clock limit
	c.MaxCatchupSteps = 30
	var s stepCounter

	c.Tick(secs(30*DT), s.step)

	if s.count() != 30 {
		t.Errorf("steps = %d, want 30", s.count())
	}
}

func TestClockCapBoundsAnOrdinaryBacklog(t *testing.T) {
	c := NewClock()
	var s stepCounter

	c.Tick(secs(10*DT), s.step)

	if s.count() != DefaultMaxCatchupSteps {
		t.Errorf("steps for a 10-interval tick = %d, want the cap %d", s.count(), DefaultMaxCatchupSteps)
	}
}
