package physics

import "math"

// Body is a point mass for the n-body solver.
type Body struct {
	Pos  Vec2
	Mass float64
}

// BruteForce computes the mutual gravitational force on every body in O(n²),
// a port of inertia's nbody.rs brute_force. Each pair is visited once.
//
// soft2 is the squared softening length: the denominator is
// (r² + soft²)^(3/2), so bodies passing through each other feel a large but
// finite pull. Pass 0 for exact inverse-square when bodies cannot coincide.
func BruteForce(bodies []Body, g, soft2 float64) []Vec2 {
	forces := make([]Vec2, len(bodies))
	for i := range bodies {
		for j := i + 1; j < len(bodies); j++ {
			d := bodies[j].Pos.Sub(bodies[i].Pos)
			r2 := d.LenSq() + soft2
			if r2 == 0 {
				// Coincident with no softening: no direction, no force.
				continue
			}
			invR3 := 1 / (r2 * math.Sqrt(r2))
			f := d.Scale(g * bodies[i].Mass * bodies[j].Mass * invR3)
			forces[i] = forces[i].Add(f)
			forces[j] = forces[j].Sub(f)
		}
	}
	return forces
}

// PointAttractor pulls toward a fixed target with the same softened
// inverse-square falloff as BruteForce: the single-target case.
type PointAttractor struct {
	At        Vec2
	Strength  float64
	Softening float64
}

// Force returns the acceleration this attractor imparts on a particle at pos.
func (a PointAttractor) Force(pos Vec2) Vec2 {
	d := a.At.Sub(pos)
	r2 := d.LenSq() + a.Softening*a.Softening
	if r2 == 0 {
		return Vec2{}
	}
	return d.Scale(a.Strength / (r2 * math.Sqrt(r2)))
}

// Seek pulls a particle to a target as a critically damped spring, so it
// arrives from any distance without overshoot. Gravity (PointAttractor) is
// weak at range and violent up close, which suits a starfield and does not
// suit a digit flying to its bin. Frequency is in radians per second.
type Seek struct {
	At        Vec2
	Frequency float64
}

// NewSeek returns a Seek converging on target at the given angular frequency.
// Around 3 rad/s gives a brisk glide.
func NewSeek(target Vec2, frequency float64) Seek {
	return Seek{At: target, Frequency: frequency}
}

// Force returns the acceleration to apply given a particle's current position
// and velocity. The velocity feeds the damping that stops it at the target.
func (s Seek) Force(pos, vel Vec2) Vec2 {
	omega := s.Frequency
	// Critical damping: c = 2*omega.
	stiffness := omega * omega
	damping := 2 * omega
	return s.At.Sub(pos).Scale(stiffness).Sub(vel.Scale(damping))
}
