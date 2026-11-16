package physics

import (
	"math"
	"testing"
)

func TestASpinCoastsAndStops(t *testing.T) {
	s := NewSpin(0, 0.2)
	s.Push(10)

	moved := 0.0
	for i := 0; i < 60*5; i++ {
		before := s.Value()
		s.Step(1.0 / 60)
		moved += math.Abs(s.Value() - before)
	}

	if moved < 1 {
		t.Errorf("a push of ten carried it %v; it is not coasting at all", moved)
	}
	// Five seconds at a fifth left each second is 0.2^5 of the shove: three
	// thousandths of a card a second.
	if math.Abs(s.Velocity()) > 0.01 {
		t.Errorf("after five seconds it is still going at %v", s.Velocity())
	}
}

// World applies its damping once per substep, so the same constant means
// something different at another frame rate or substep count (0.99 leaves nine
// per cent after a second). A per-second drag does not depend on the loop.
func TestDragIsPerSecondNotPerStep(t *testing.T) {
	fine, coarse := NewSpin(0, 0.25), NewSpin(0, 0.25)
	fine.Push(10)
	coarse.Push(10)

	for i := 0; i < 600; i++ {
		fine.Step(1.0 / 600)
	}
	for i := 0; i < 6; i++ {
		coarse.Step(1.0 / 6)
	}

	// A hundred times the step size, and they agree.
	if got := math.Abs(fine.Value() - coarse.Value()); got > 1e-9 {
		t.Errorf("stepped finely it reached %v and coarsely %v, a gap of %v",
			fine.Value(), coarse.Value(), got)
	}
	if got := math.Abs(fine.Velocity() - coarse.Velocity()); got > 1e-9 {
		t.Errorf("the speeds disagree by %v; drag is being applied per step", got)
	}
}

// It stops at the ends without bouncing off or straining against them.
func TestStopsKillTheMotion(t *testing.T) {
	s := NewSpin(5, 0.5)
	s.Push(100)
	for i := 0; i < 60; i++ {
		s.Step(1.0 / 60)
		s.Bound(0, 10)
	}

	if s.Value() != 10 {
		t.Errorf("it came to rest at %v, want the stop at 10", s.Value())
	}
	if s.Velocity() != 0 {
		t.Errorf("it is still going at %v having hit the stop", s.Velocity())
	}
}

// Holding a key spins it up instead of repeating one nudge.
func TestPushesAccumulate(t *testing.T) {
	one, many := NewSpin(0, 0.3), NewSpin(0, 0.3)
	one.Push(5)
	for i := 0; i < 4; i++ {
		many.Push(5)
	}

	for i := 0; i < 60*3; i++ {
		one.Step(1.0 / 60)
		many.Step(1.0 / 60)
	}
	if many.Value() <= one.Value()*2 {
		t.Errorf("four pushes carried it %v and one carried it %v; they are not "+
			"adding up", many.Value(), one.Value())
	}
}

// Set stops it dead, so a jump does not keep the previous motion.
func TestSetCancelsTheMotion(t *testing.T) {
	s := NewSpin(0, 0.9)
	s.Push(50)
	s.Step(1.0 / 60)
	s.Set(3)

	if s.Value() != 3 || s.Velocity() != 0 {
		t.Errorf("after Set it is at %v going %v, want 3 and still", s.Value(), s.Velocity())
	}
}
