package physics

import "math"

// Collide keeps particles from occupying the same place, so they pile up.
// Unlike Distance it only pushes, and it needs no declared pairs: contacts are
// found afresh each pass through a spatial hash (see hashGrid).
type Collide struct {
	// Radius is half a contact: two particles closer than 2*Radius are pushed
	// apart until they are exactly that far.
	Radius float64

	// Radii, when set, is a radius per particle, with Radius as the fallback.
	// A pair is in contact when closer than the sum of their two radii.
	Radii []float64

	// Stiffness is the share of the overlap a single pass removes. Below one,
	// a deep pile does not explode as the bottom row resolves.
	Stiffness float64

	grid   hashGrid
	widest float64 // the largest radius in play, fixed at each rebuild

	// OnContact, if set, is called for every overlapping pair as it is
	// separated. It fires once per pair per relaxation pass, so many times for
	// a pair that stays in contact; a caller that wants "just met" keeps that
	// state itself.
	OnContact func(i, j int)

	// pass counts relaxations, so the hash is rebuilt once per substep.
	pass int
}

// NewCollide returns contacts of the given radius.
func NewCollide(radius float64) *Collide {
	return &Collide{Radius: radius, Stiffness: 0.5}
}

// Solve separates every overlapping pair once.
func (c *Collide) Solve(w *World) {
	n := len(w.pos)
	if n < 2 || c.Radius <= 0 {
		return
	}

	// Rebuild once per substep. A pass moves a particle by a fraction of an
	// overlap and a bin is a contact diameter across, so nothing changes bin
	// within one substep's relaxations.
	if c.pass%Iterations == 0 || c.grid.size() != n {
		c.rebuild(w)
	}
	c.pass++

	for i := 0; i < n; i++ {
		ri := c.radiusOf(i)
		c.grid.near(w.pos[i], func(j int) {
			// Each pair once: the higher index does the work.
			if j <= i {
				return
			}
			d := ri + c.radiusOf(j)
			c.separate(w, i, j, d, d*d)
		})
	}
}

// radiusOf is a particle's own radius, or the shared one.
func (c *Collide) radiusOf(i int) float64 {
	if i < len(c.Radii) && c.Radii[i] > 0 {
		return c.Radii[i]
	}
	return c.Radius
}

// separate pushes one overlapping pair apart, splitting the correction by
// inverse mass so a heavy particle moves less and a pinned one not at all.
func (c *Collide) separate(w *World, i, j int, d, dd float64) {
	delta := w.pos[j].Sub(w.pos[i])
	distSq := delta.LenSq()
	if distSq >= dd || distSq == 0 {
		// Coincident particles have no direction to separate along. Do not
		// nudge them randomly: the simulation must agree between terminals.
		return
	}

	wi, wj := w.invMass[i], w.invMass[j]
	total := wi + wj
	if total == 0 {
		return
	}

	if c.OnContact != nil {
		c.OnContact(i, j)
	}

	dist := math.Sqrt(distSq)
	push := (d - dist) * c.Stiffness
	dir := delta.Scale(1 / dist)

	w.pos[i] = w.pos[i].Sub(dir.Scale(push * wi / total))
	w.pos[j] = w.pos[j].Add(dir.Scale(push * wj / total))
}

// rebuild bins every particle into the hash.
func (c *Collide) rebuild(w *World) {
	// Bins are twice the widest radius, so the nine-cell walk finds every
	// pair whatever their radii.
	c.widest = c.Radius
	for _, r := range c.Radii {
		if r > c.widest {
			c.widest = r
		}
	}
	c.grid.build(w.pos, 2*c.widest)
}
