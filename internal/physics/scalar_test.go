package physics

import "testing"

// Scalar is the 1-D form of Seek: a critically damped spring for animating a
// single number, such as a digit's drawn height.

func TestScalarStartsAtItsInitialValue(t *testing.T) {
	s := NewScalar(0.25, 3)

	closeTo(t, s.Value(), 0.25, "initial value")
}

func TestScalarConvergesOnItsTarget(t *testing.T) {
	s := NewScalar(0, 3)

	for i := 0; i < 300; i++ {
		s.Step(DT, 1)
	}

	closeToTol(t, s.Value(), 1, 0.01, "value after settling")
}

func TestScalarDoesNotOvershoot(t *testing.T) {
	s := NewScalar(0, 3)

	peak := 0.0
	for i := 0; i < 600; i++ {
		s.Step(DT, 1)
		if s.Value() > peak {
			peak = s.Value()
		}
	}

	if peak > 1.001 {
		t.Errorf("peaked at %v, want no overshoot past 1", peak)
	}
}

// Growth must be gradual enough to watch, not a snap.
func TestScalarTakesTimeToGrow(t *testing.T) {
	s := NewScalar(0, 3)

	s.Step(DT, 1)
	if s.Value() > 0.05 {
		t.Errorf("reached %v after a single frame, want a gradual climb", s.Value())
	}

	for i := 0; i < 20; i++ {
		s.Step(DT, 1)
	}
	if s.Value() >= 0.95 {
		t.Errorf("reached %v after a third of a second, want it still growing", s.Value())
	}
}

func TestScalarShrinksBackToZero(t *testing.T) {
	s := NewScalar(1, 3)

	for i := 0; i < 300; i++ {
		s.Step(DT, 0)
	}

	closeToTol(t, s.Value(), 0, 0.01, "value after shrinking")
}

func TestScalarHigherFrequencyConvergesFaster(t *testing.T) {
	slow, fast := NewScalar(0, 2), NewScalar(0, 8)

	for i := 0; i < 30; i++ {
		slow.Step(DT, 1)
		fast.Step(DT, 1)
	}

	if fast.Value() <= slow.Value() {
		t.Errorf("fast spring at %v, slow at %v; want fast ahead", fast.Value(), slow.Value())
	}
}

func TestScalarSetJumpsWithoutVelocity(t *testing.T) {
	s := NewScalar(0, 3)
	for i := 0; i < 30; i++ {
		s.Step(DT, 1)
	}

	s.Set(0)

	closeTo(t, s.Value(), 0, "value after Set")
	// A Set that left velocity behind would immediately drift.
	s.Step(DT, 0)
	closeToTol(t, s.Value(), 0, 1e-9, "value one step after Set")
}

func closeToTol(t *testing.T, got, want, tol float64, what string) {
	t.Helper()
	if d := got - want; d > tol || d < -tol {
		t.Errorf("%s = %v, want %v (tol %v)", what, got, want, tol)
	}
}

// Callers rely on a critically damped spring never overshooting, and on the
// zero-value Scalar being critically damped.
func TestCriticalDampingStillDoesNotOvershoot(t *testing.T) {
	for _, s := range []*Scalar{NewScalar(0, 6), {Frequency: 6}, NewScalarDamped(0, 6, 1)} {
		peak := 0.0
		for i := 0; i < 600; i++ {
			s.Step(DT, 1)
			if s.Value() > peak {
				peak = s.Value()
			}
		}
		if peak > 1.0001 {
			t.Errorf("a critically damped spring reached %v, past its target of 1", peak)
		}
		if s.Value() < 0.99 {
			t.Errorf("a critically damped spring settled at %v, want 1", s.Value())
		}
	}
}

// Underdamping gives a hinged thing its knock.
func TestAnUnderdampedSpringOvershootsAndSettles(t *testing.T) {
	s := NewScalarDamped(0, 6, 0.4)

	peak := 0.0
	for i := 0; i < 600; i++ {
		s.Step(DT, 1)
		if s.Value() > peak {
			peak = s.Value()
		}
	}
	if peak <= 1.02 {
		t.Errorf("an underdamped spring peaked at %v; it should overshoot its target", peak)
	}
	if got := s.Value(); got < 0.99 || got > 1.01 {
		t.Errorf("an underdamped spring settled at %v, want it coming to rest at 1", got)
	}
}
