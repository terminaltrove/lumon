package physics

// Bounds keeps every particle inside a rectangle. It is a constraint because
// a wall that particles pile up against has to be relaxed together with the
// other contacts.
type Bounds struct {
	Min, Max Vec2

	// Restitution is the share of the approach speed that comes back: 0 stops
	// dead, 1 bounces for ever.
	Restitution float64

	// Friction is the share of tangential speed each contact takes away.
	Friction float64
}

// NewBounds returns a box with a little bounce and a lot of friction.
func NewBounds(min, max Vec2) *Bounds {
	return &Bounds{Min: min, Max: max, Restitution: 0.25, Friction: 0.35}
}

// Solve pushes every particle back inside and takes the speed out of the
// contact. Verlet velocity is the gap between pos and prev, so a bounce moves
// prev.
//
// Only a particle moving into the wall is reflected; one already heading back
// in is clamped with its velocity unchanged. Solve runs 48 times a step, and
// with Collide pushing a particle into a wall, reflecting every pass flips its
// velocity back and forth until none is left.
func (b *Bounds) Solve(w *World) {
	for i := range w.pos {
		if w.invMass[i] == 0 {
			continue // pinned
		}
		p := w.pos[i]
		v := p.Sub(w.prev[i])
		hit := false

		if p.X < b.Min.X {
			p.X = b.Min.X
			if v.X < 0 {
				v.X = -v.X * b.Restitution
				v.Y *= 1 - b.Friction
			}
			hit = true
		} else if p.X > b.Max.X {
			p.X = b.Max.X
			if v.X > 0 {
				v.X = -v.X * b.Restitution
				v.Y *= 1 - b.Friction
			}
			hit = true
		}

		if p.Y < b.Min.Y {
			p.Y = b.Min.Y
			if v.Y < 0 {
				v.Y = -v.Y * b.Restitution
				v.X *= 1 - b.Friction
			}
			hit = true
		} else if p.Y > b.Max.Y {
			p.Y = b.Max.Y
			if v.Y > 0 {
				v.Y = -v.Y * b.Restitution
				v.X *= 1 - b.Friction
			}
			hit = true
		}

		if hit {
			w.pos[i] = p
			w.prev[i] = p.Sub(v)
		}
	}
}

// Contains reports whether a point is inside.
func (b *Bounds) Contains(p Vec2) bool {
	return p.X >= b.Min.X && p.X <= b.Max.X && p.Y >= b.Min.Y && p.Y <= b.Max.Y
}
