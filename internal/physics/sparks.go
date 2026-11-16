package physics

import "math"

// Sparks are particles that are thrown, pulled down, slowed by the air, and
// then die. World and Fluid have no lifetime and no removal.
//
// The slices are exported and Step never removes anything, so a caller can
// keep a payload per particle (a rune, say) in step with them. The caller
// walks Life backwards and calls Remove; walking backwards makes the swap
// safe, since the element swapped into the hole was already visited.
//
// There is no particle cap: Add is the only way in, so the count is bounded
// by the caller's payload, and a cap would hide a conservation bug.
type Sparks struct {
	// One entry per particle; all must stay the same length. Life is seconds
	// remaining and may go negative; Span is the life it was given.
	Pos  []Vec2
	Vel  []Vec2
	Life []float64
	Span []float64

	// Drag is per particle, the fraction of the speed left after one second
	// (see Spin.Drag). One is no drag. A firework's willow is its peony burst
	// with more drag.
	Drag []float64

	// Gravity is the pull on all of them, in units a second squared.
	Gravity Vec2
}

// dragFloor is the least drag a particle may be given. Zero makes ln(drag)
// minus infinity and Step produce NaN.
const dragFloor = 1e-6

// NewSparks returns an empty set with no gravity.
func NewSparks() *Sparks { return &Sparks{} }

// Len is how many there are.
func (s *Sparks) Len() int { return len(s.Pos) }

// Add throws one, at index Len()-1. It returns no index, because an index is
// valid only until the next Remove.
func (s *Sparks) Add(at, vel Vec2, life, drag float64) {
	s.Pos = append(s.Pos, at)
	s.Vel = append(s.Vel, vel)
	s.Life = append(s.Life, life)
	s.Span = append(s.Span, life)
	s.Drag = append(s.Drag, drag)
}

// Remove takes one out, swapping the last into its place. A caller keeping a
// payload alongside must make the same swap, or it silently pairs particles
// with the wrong payload; keep a single call site.
func (s *Sparks) Remove(i int) {
	last := len(s.Pos) - 1
	if i < 0 || i > last {
		return
	}
	s.Pos[i], s.Vel[i] = s.Pos[last], s.Vel[last]
	s.Life[i], s.Span[i], s.Drag[i] = s.Life[last], s.Span[last], s.Drag[last]
	s.Pos, s.Vel = s.Pos[:last], s.Vel[:last]
	s.Life, s.Span, s.Drag = s.Life[:last], s.Span[:last], s.Drag[:last]
}

// Fade is how much of a particle's life is left, from one to zero. A spark
// given no life returns zero.
func (s *Sparks) Fade(i int) float64 {
	if i < 0 || i >= len(s.Life) || s.Span[i] <= 0 {
		return 0
	}
	if f := s.Life[i] / s.Span[i]; f > 0 {
		if f > 1 {
			return 1
		}
		return f
	}
	return 0
}

// Step moves everything and ages it by dt seconds. It never removes anything.
//
// The motion is integrated in closed form, as in Spin.Step, so the result does
// not depend on the frame rate. With k = ln(drag), dv/dt = g + kv solves to
//
//	v(t) = (v0 + g/k)·e^(kt) - g/k
//	x(t) = x0 + (v0 + g/k)·(e^(kt) - 1)/k - (g/k)·t
//
// and a particle under drag settles at a terminal speed of -g/k. Near k = 0
// the g/k terms grow and cancel catastrophically, so that case uses plain
// ballistics.
func (s *Sparks) Step(dt float64) {
	if dt <= 0 {
		return
	}
	for i := range s.Pos {
		s.Life[i] -= dt

		drag := s.Drag[i]
		if drag < dragFloor {
			drag = dragFloor
		} else if drag > 1 {
			drag = 1
		}

		if k := math.Log(drag); math.Abs(k) >= 1e-9 {
			decay := math.Exp(k * dt)
			gk := s.Gravity.Scale(1 / k)
			u := s.Vel[i].Add(gk)
			s.Pos[i] = s.Pos[i].Add(u.Scale((decay - 1) / k)).Sub(gk.Scale(dt))
			s.Vel[i] = u.Scale(decay).Sub(gk)
			continue
		}

		// Frictionless: plain ballistics.
		s.Pos[i] = s.Pos[i].
			Add(s.Vel[i].Scale(dt)).
			Add(s.Gravity.Scale(0.5 * dt * dt))
		s.Vel[i] = s.Vel[i].Add(s.Gravity.Scale(dt))
	}
}
