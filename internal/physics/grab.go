package physics

// Grab holds one particle at a moving point, and lets go of it with the speed
// it was travelling at. A Pin cannot do this: Pin.Solve cancels velocity, so a
// pinned particle is released stationary.
type Grab struct {
	world *World

	// held is the particle, or -1 for nothing held.
	held int

	// mass is the particle's inverse mass before it was picked up.
	mass float64

	// at is the hold point and vel its smoothed speed per step. A pointer
	// reports whole cells, so the raw difference is all jumps and stalls.
	at  Vec2
	vel Vec2
}

// grabSmoothing is the share of the previous hold speed kept each move, so a
// flick's speed does not depend on the pointer's last frame alone.
const grabSmoothing = 0.6

// NewGrab returns a hand that has hold of nothing.
func NewGrab(w *World) *Grab { return &Grab{world: w, held: -1} }

// Holding reports whether anything is held, and which.
func (g *Grab) Holding() (int, bool) { return g.held, g.held >= 0 }

// Take picks up the nearest particle within reach of a point, and reports
// whether it found one.
func (g *Grab) Take(at Vec2, reach float64) bool {
	if g.world == nil {
		return false
	}
	g.Release()

	i := Nearest(g.world, at, reach)
	if i < 0 {
		return false
	}

	g.held = i
	g.mass = g.world.invMass[i]
	g.at, g.vel = at, Vec2{}

	// Zero inverse mass makes the integrator and mass-weighted constraints skip
	// it, so the pile it is dragged through cannot push it around.
	g.world.SetInvMass(i, 0)
	return true
}

// MoveTo drags the held particle to a point.
func (g *Grab) MoveTo(at Vec2) {
	if g.held < 0 {
		return
	}
	delta := at.Sub(g.at)
	g.vel = g.vel.Scale(grabSmoothing).Add(delta.Scale(1 - grabSmoothing))
	g.at = at
	g.world.SetPos(g.held, at)
}

// Release lets go, giving the particle the speed the hand was moving at.
func (g *Grab) Release() {
	if g.held < 0 {
		return
	}
	i := g.held
	g.held = -1

	g.world.SetInvMass(i, g.mass)

	// MoveTo's SetPos zeroes the implicit velocity, so it must be restored.
	g.world.SetVelocity(i, g.vel.Scale(1/velocityRefStep))
}

// Nearest is the particle closest to a point within a radius, or -1. A linear
// scan is enough: it runs once per click over a few hundred particles.
func Nearest(w *World, at Vec2, within float64) int {
	best, bestSq := -1, within*within
	for i, p := range w.pos {
		if d := p.Sub(at).LenSq(); d <= bestSq {
			best, bestSq = i, d
		}
	}
	return best
}
