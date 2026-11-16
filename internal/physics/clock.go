package physics

import "time"

// DT is the fixed simulation timestep in seconds. Physics runs at 60Hz
// whatever the render rate, so behaviour does not depend on how fast the
// terminal draws.
const DT = 1.0 / 60

// stepInterval is DT as a Duration, truncated by 6ns.
const stepInterval = time.Second / 60

// DefaultMaxCatchupSteps caps the steps one Tick runs, as inertia's
// MAX_CATCHUP_STEPS. Without it a suspended process or a slow first frame
// queues thousands of steps that then run at once.
const DefaultMaxCatchupSteps = 5

// DefaultBudget is the wall-clock time one Tick may spend stepping, as
// inertia's PHYSICS_BUDGET. When steps are too expensive to keep up, the rest
// of the backlog is dropped, so the game slows down instead of freezing input
// and rendering.
const DefaultBudget = 20 * time.Millisecond

// Clock is the fixed-timestep accumulator from inertia's App::tick (app.rs).
// Elapsed time goes in, whole steps of DT come out, and the remainder is kept
// for the next frame.
type Clock struct {
	// MaxCatchupSteps bounds the backlog worked through per Tick.
	MaxCatchupSteps int
	// Budget bounds the wall-clock time spent stepping per Tick. Zero disables
	// the limit and always runs the full backlog.
	Budget time.Duration
	// TimeScale stretches or compresses simulated time. 1 is real time.
	TimeScale float64
	// Paused stops the simulation and banks no time while set.
	Paused bool

	accumulator time.Duration
	stepOnce    bool

	// now is replaced in tests to exercise the budget without sleeping.
	now func() time.Time
}

// NewClock returns a Clock with inertia's defaults.
func NewClock() *Clock {
	return &Clock{
		MaxCatchupSteps: DefaultMaxCatchupSteps,
		Budget:          DefaultBudget,
		TimeScale:       1,
		now:             time.Now,
	}
}

// StepOnce queues a single step to run on the next Tick even while paused,
// giving frame-by-frame inspection.
func (c *Clock) StepOnce() { c.stepOnce = true }

// Tick advances simulated time by elapsed wall-clock time, calling step once
// per DT, and returns how many steps ran.
func (c *Clock) Tick(elapsed time.Duration, step func(dt float64)) int {
	ran := 0

	if c.Paused {
		// Keep nothing, so unpausing does not fast-forward.
		c.accumulator = 0
		if c.stepOnce {
			step(DT)
			ran++
		}
		c.stepOnce = false
		return ran
	}

	c.accumulator += time.Duration(float64(elapsed) * c.TimeScale)
	if cap := stepInterval * time.Duration(c.MaxCatchupSteps); c.accumulator > cap {
		c.accumulator = cap
	}

	var start time.Time
	if c.Budget > 0 {
		start = c.now()
	}

	for c.accumulator >= stepInterval {
		step(DT)
		ran++
		c.accumulator -= stepInterval

		if c.Budget > 0 && c.now().Sub(start) >= c.Budget {
			// Out of wall-clock time. Drop the backlog: carried into the next
			// frame it would only grow.
			c.accumulator = 0
			break
		}
	}

	c.stepOnce = false
	return ran
}
