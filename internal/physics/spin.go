package physics

import "math"

// Spin is a value with momentum, for things that are flicked: it is pushed,
// coasts, and drag slows it. Unlike Scalar it has no target. A caller that
// wants it to settle somewhere adds that pull itself.
type Spin struct {
	value, velocity float64

	// Drag is the fraction of the speed left after one second, at any step
	// size. (World's damping is per substep, 240 times a second, so its 0.99
	// leaves 9% after a second.)
	Drag float64
}

// NewSpin returns a value at rest.
func NewSpin(value, drag float64) *Spin {
	return &Spin{value: value, Drag: drag}
}

// Value returns the current value.
func (s *Spin) Value() float64 { return s.value }

// Velocity returns the current speed, in units a second.
func (s *Spin) Velocity() float64 { return s.velocity }

// Push adds v to the velocity, in units a second. Pushes accumulate, so a held
// key spins it up.
func (s *Spin) Push(v float64) { s.velocity += v }

// Set moves it to v and stops it.
func (s *Spin) Set(v float64) {
	s.value, s.velocity = v, 0
}

// Step lets it coast for dt seconds under Drag. The distance is the exact
// integral of v0·dragᵗ, v0·(drag^dt - 1)/ln(drag), so the result does not
// depend on the step size.
func (s *Spin) Step(dt float64) {
	if dt <= 0 {
		return
	}
	drag := s.Drag
	if drag < 0 {
		drag = 0
	} else if drag > 1 {
		drag = 1
	}

	k := math.Log(drag)
	if k == 0 {
		// Frictionless.
		s.value += s.velocity * dt
		return
	}
	decay := math.Exp(k * dt)
	s.value += s.velocity * (decay - 1) / k
	s.velocity *= decay
}

// Bound holds it between two stops and kills the motion that carried it into
// one. A bounce would make the ends of a list look springy.
func (s *Spin) Bound(lo, hi float64) {
	if hi < lo {
		lo, hi = hi, lo
	}
	if s.value < lo {
		s.value, s.velocity = lo, 0
	} else if s.value > hi {
		s.value, s.velocity = hi, 0
	}
}
