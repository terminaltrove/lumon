package physics

import (
	"math"
	"testing"
)

// This is why sparks use a closed form: under drag the speed falls during a
// step, so a stepped integrator's error depends on the step length. Spin makes
// the same claim without a constant pull; a stray v += g*dt beside the closed
// form would break this.
func TestASparkFallsTheSameWhateverTheStep(t *testing.T) {
	for _, drag := range []float64{1, 0.9, 0.4, 0.05} {
		at := func(steps int) Vec2 {
			s := NewSparks()
			s.Gravity = Vec2{Y: 30}
			s.Add(Vec2{}, Vec2{X: 12, Y: -40}, 10, drag)
			for i := 0; i < steps; i++ {
				s.Step(1.0 / float64(steps))
			}
			return s.Pos[0]
		}

		one, many := at(1), at(600)
		if math.Abs(one.X-many.X) > 1e-9 || math.Abs(one.Y-many.Y) > 1e-9 {
			t.Errorf("drag %.2f: one second in one step lands at %v and in six hundred "+
				"at %v; the step size is in the answer", drag, one, many)
		}
	}
}

// Terminal velocity is exactly -g/ln(drag), which checks that the integral is
// the correct one, not merely a smooth one.
func TestASparkUnderDragReachesATerminalSpeed(t *testing.T) {
	const drag = 0.3
	s := NewSparks()
	s.Gravity = Vec2{Y: 30}
	s.Add(Vec2{}, Vec2{}, 100, drag)

	// Twenty seconds: the gap closes by a factor of the drag every second, so
	// ten seconds still leaves a ten-thousandth of it.
	for i := 0; i < 60*20; i++ {
		s.Step(1.0 / 60)
	}

	want := -s.Gravity.Y / math.Log(drag)
	if got := s.Vel[0].Y; math.Abs(got-want) > 1e-6 {
		t.Errorf("it settled at %.6f, want the terminal speed %.6f", got, want)
	}
}

// The k == 0 branch. The general form divides by the log of the drag, and as
// the drag approaches one the two g/k terms grow without bound and cancel.
func TestASparkWithNoDragIsPlainBallistics(t *testing.T) {
	s := NewSparks()
	s.Gravity = Vec2{Y: 20}
	s.Add(Vec2{}, Vec2{X: 5, Y: -10}, 10, 1)

	const dt = 0.5
	s.Step(dt)

	wantX := 5 * dt
	wantY := -10*dt + 0.5*20*dt*dt
	if math.Abs(s.Pos[0].X-wantX) > 1e-12 || math.Abs(s.Pos[0].Y-wantY) > 1e-12 {
		t.Errorf("it is at %v, want exactly %v", s.Pos[0], Vec2{X: wantX, Y: wantY})
	}

	// A drag just under one gives nearly the same answer, so the two branches
	// meet without a step at the seam.
	n := NewSparks()
	n.Gravity = s.Gravity
	n.Add(Vec2{}, Vec2{X: 5, Y: -10}, 10, 1-1e-12)
	n.Step(dt)
	if math.Abs(n.Pos[0].X-s.Pos[0].X) > 1e-6 || math.Abs(n.Pos[0].Y-s.Pos[0].Y) > 1e-6 {
		t.Errorf("a drag of almost exactly one lands at %v against %v with none at all",
			n.Pos[0], s.Pos[0])
	}
}

// A drag of zero is a coefficient of minus infinity, and minus infinity times a
// zero step is NaN; dragFloor keeps every term finite.
//
// Stopping is asserted with the pull off: under gravity a spark reaches
// terminal speed, which at dragFloor against a pull of thirty is still a couple
// of units a second.
func TestADragOfNothingStopsASparkWithoutNaN(t *testing.T) {
	s := NewSparks()
	s.Gravity = Vec2{Y: 30}
	s.Add(Vec2{X: 3, Y: 4}, Vec2{X: 100, Y: -100}, 10, 0)

	for i := 0; i < 120; i++ {
		s.Step(1.0 / 60)
	}
	if math.IsNaN(s.Pos[0].X) || math.IsNaN(s.Pos[0].Y) ||
		math.IsNaN(s.Vel[0].X) || math.IsNaN(s.Vel[0].Y) {
		t.Fatalf("it went to NaN: pos %v vel %v", s.Pos[0], s.Vel[0])
	}
	// Sideways there is no pull, so that component stops.
	if math.Abs(s.Vel[0].X) > 1e-6 {
		t.Errorf("it is still going sideways at %.6f under total drag", s.Vel[0].X)
	}

	// A full second, because the drag is per second: a sixtieth of a second
	// applies only its sixtieth root, which leaves four fifths of the speed.
	free := NewSparks()
	free.Add(Vec2{}, Vec2{X: 100, Y: -100}, 10, 0)
	for i := 0; i < 60; i++ {
		free.Step(1.0 / 60)
	}
	if v := free.Vel[0].Len(); v > 1e-3 {
		t.Errorf("with no pull at all it is going %.6f a second after total drag", v)
	}
}

// Removing a spark moves the last into the hole. Callers mirror this in their
// own payload arrays, and a mismatch returns the wrong payload without
// panicking.
func TestRemovingASparkKeepsTheArraysInStep(t *testing.T) {
	s := NewSparks()
	for i := 0; i < 4; i++ {
		s.Add(Vec2{X: float64(i)}, Vec2{Y: float64(i)}, float64(i+1), 0.5)
	}

	s.Remove(1)

	if s.Len() != 3 {
		t.Fatalf("three left, got %d", s.Len())
	}
	for _, n := range []int{len(s.Pos), len(s.Vel), len(s.Life), len(s.Span), len(s.Drag)} {
		if n != 3 {
			t.Fatalf("the arrays came apart: %d %d %d %d %d",
				len(s.Pos), len(s.Vel), len(s.Life), len(s.Span), len(s.Drag))
		}
	}
	if s.Pos[1].X != 3 || s.Vel[1].Y != 3 || s.Life[1] != 4 {
		t.Errorf("slot 1 holds %v/%v/%v; the last should have been swapped in",
			s.Pos[1], s.Vel[1], s.Life[1])
	}

	// Out of range is a no-op, not a panic, for a caller walking backwards over
	// a shrinking set.
	s.Remove(-1)
	s.Remove(99)
	if s.Len() != 3 {
		t.Errorf("an out of range remove took one out anyway")
	}
}

// The caller owns removal because only the caller has the parallel arrays; a
// cull in Step would permute them unseen. A dead spark stays in the set with a
// life at or below zero until it is removed.
func TestStepAgesSparksButNeverCullsThem(t *testing.T) {
	s := NewSparks()
	s.Add(Vec2{}, Vec2{}, 0.1, 1)
	s.Add(Vec2{}, Vec2{}, 5.0, 1)

	for i := 0; i < 60; i++ {
		s.Step(1.0 / 60)
	}
	if s.Len() != 2 {
		t.Fatalf("the set culled itself down to %d", s.Len())
	}
	if s.Life[0] > 0 {
		t.Errorf("the short one has %.3f seconds left after one; it is not ageing", s.Life[0])
	}
	if s.Life[1] <= 0 {
		t.Errorf("the long one is already spent")
	}
}

// A spark given no life does not divide by zero.
func TestFadeReportsWhatIsLeftOfASpark(t *testing.T) {
	s := NewSparks()
	s.Add(Vec2{}, Vec2{}, 2, 1)
	s.Add(Vec2{}, Vec2{}, 0, 1)

	if got := s.Fade(0); got != 1 {
		t.Errorf("a fresh spark faded to %v, want 1", got)
	}
	s.Step(1)
	if got := s.Fade(0); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("halfway through it reads %v, want 0.5", got)
	}
	s.Step(5)
	if got := s.Fade(0); got != 0 {
		t.Errorf("a spent spark reads %v, want 0 rather than a negative", got)
	}
	if got := s.Fade(1); got != 0 {
		t.Errorf("a spark given no life at all reads %v", got)
	}
	if got := s.Fade(99); got != 0 {
		t.Errorf("a spark that does not exist reads %v", got)
	}
}

func TestAnEmptySparkSetSteps(t *testing.T) {
	s := NewSparks()
	s.Gravity = Vec2{Y: 10}
	s.Step(1.0 / 60)
	s.Step(0)
	s.Step(-1)
	if s.Len() != 0 {
		t.Errorf("an empty set grew to %d", s.Len())
	}
}
