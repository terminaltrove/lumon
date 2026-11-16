package lumon

import "github.com/terminaltrove/lumon/internal/physics"

// The board as a sheet, so a detonation ripples through it.
//
// One particle per visible cell on a mass-spring lattice, each pulled toward
// its home and its four neighbours. A detonation throws the particles at its
// centre outward and the wave propagates, reflects off the edges, interferes
// and dies out through damping.
//
// It uses forces, not the position-based constraints Agitation uses: those
// propagate one constraint per iteration, about forty-eight cells a frame,
// which is a jolt, not a wave. With springs the wave speed is c = a·sqrt(k/m)
// for lattice spacing a.

const (
	// rippleSpeed is how fast a disturbance travels, in visual columns a
	// second. It also sets how slack the sheet is, since k = (c/a)²: a stiffer
	// sheet smooths a blast into a one-cell wobble instead of throwing digits
	// apart.
	rippleSpeed = 30.0

	// rippleHome is the stiffness of the spring holding a digit at its own
	// cell, in lattice-spring units. It returns every digit to where it
	// started; weak, so a thrown digit coasts before it is drawn back.
	rippleHome = 11.0

	// rippleDamping is the Verlet damping per substep, tuned by eye with
	// rippleHome so the board is still again within about three seconds.
	rippleDamping = 0.978

	// rippleKick is how hard a detonation throws the digits at its centre, in
	// visual columns a second, with a linear falloff to rippleCoreCells. Both
	// match Sandbox.Blast. The ring is what the sheet makes of the impulse.
	rippleKick = 95.0

	// rippleCoreCells is the kick's reach in cells (28 visual columns at the
	// default spacing). It must scale with the spacing, or at the widest zoom
	// the impulse barely reaches the next digit.
	rippleCoreCells = 28.0 / CellWidth

	// rippleQuiet is how far from home, in visual columns, and rippleStill how
	// fast, in visual columns a second, a particle may be while the sheet
	// counts as still. They are separate thresholds because a residual
	// displacement too small to see can still carry a visible speed. Below
	// both, the simulation is skipped.
	rippleQuiet = 0.02
	rippleStill = 1.5
)

// Ripple is the sheet: a particle per visible cell, sprung to its neighbours and
// to its own place.
type Ripple struct {
	world *physics.World
	home  []physics.Vec2
	force []physics.Vec2

	// x, y, w, h is the viewport the lattice was built for, in grid cells.
	x, y, w, h int

	// cellW and rowH are the spacing the lattice was built at, and core the
	// kick's reach in visual columns. They are kept here, not read from the
	// layout, because displacements must be converted back with the spacing
	// they were built at, even between a zoom and the next Sync.
	cellW, rowH float64
	core        float64

	// kx and ky are the lattice stiffnesses per axis. A cell is not as tall
	// as it is wide, so equal stiffness would make the wave faster on one axis
	// and the ring would not be round on screen.
	kx, ky float64

	// moving is whether anything is displaced, so a still board skips the
	// simulation.
	moving bool

	// seen is the last detonation this view has thrown into its sheet. Each
	// terminal ripples for itself.
	seen int
}

// NewRipple returns an empty sheet. It builds its lattice on the first Sync.
func NewRipple() *Ripple { return &Ripple{} }

// Sync brings the lattice into line with the viewport and the spacing. A
// moved or resized viewport rebuilds the lattice, keeping motion by cell; a
// change of spacing alone (an easing zoom) respaces it in place. Either way a
// wave in flight survives.
func (r *Ripple) Sync(l *Layout) {
	x, y := l.ViewportX, l.ViewportY
	w, h := l.ViewportW, l.ViewportH
	if r.world == nil || r.x != x || r.y != y || r.w != w || r.h != h {
		r.rebuild(l)
		return
	}
	if r.cellW != l.CellW || r.rowH != l.RowH {
		r.respace(l)
	}
}

// rebuild lays a new sheet over the viewport, carrying across the motion at
// every cell the old and new sheets share. It maps by grid cell, not slot
// index, since a slot stands for a different cell once the viewport moves.
// Cells new to the sheet start still.
func (r *Ripple) rebuild(l *Layout) {
	old, ow, oh := r.world, r.w, r.h
	ox, oy := r.x, r.y
	oldHome := r.home

	r.x, r.y = l.ViewportX, l.ViewportY
	r.w, r.h = l.ViewportW, l.ViewportH

	n := r.w * r.h
	r.world = physics.NewWorld()
	r.world.SetDamping(rippleDamping)
	r.home = make([]physics.Vec2, n)
	r.force = make([]physics.Vec2, n)
	r.moving = false

	r.scale(l)
	for i := range r.home {
		r.world.Add(r.home[i], 1)
	}
	if old == nil {
		return
	}

	for gy := 0; gy < r.h; gy++ {
		for gx := 0; gx < r.w; gx++ {
			cx, cy := r.x+gx-ox, r.y+gy-oy
			if cx < 0 || cy < 0 || cx >= ow || cy >= oh {
				continue
			}
			j := cy*ow + cx
			i := gy*r.w + gx

			// Carry velocity too: on the frame of a Kick nothing has moved
			// yet, so offsets alone would drop the blast.
			d := old.Pos(j).Sub(oldHome[j])
			v := old.Velocity(j, rippleRefStep)
			if d.X == 0 && d.Y == 0 && v.X == 0 && v.Y == 0 {
				continue
			}
			r.world.SetPos(i, r.home[i].Add(d))
			r.world.SetVelocity(i, v)
			r.moving = true
		}
	}
}

// respace slides every particle to where its cell now sits, keeping its
// displacement and velocity. Verlet stores velocity as the gap to the previous
// position, which SetPos closes, so it is read out and restored at
// rippleRefStep for an exact round trip.
func (r *Ripple) respace(l *Layout) {
	was := make([]physics.Vec2, len(r.home))
	copy(was, r.home)

	r.scale(l)
	for i := range r.home {
		v := r.world.Velocity(i, rippleRefStep)
		r.world.SetPos(i, r.world.Pos(i).Add(r.home[i].Sub(was[i])))
		r.world.SetVelocity(i, v)
	}
}

// rippleRefStep is the step physics.SetVelocity expresses a velocity against.
// Reading a velocity at the same figure makes respace's round trip exact.
const rippleRefStep = 1.0 / 60

// scale writes the home positions, the stiffnesses and the kick's reach for the
// layout's current spacing. It allocates nothing.
func (r *Ripple) scale(l *Layout) {
	r.cellW, r.rowH = l.CellW, l.RowH
	r.core = rippleCoreCells * r.cellW

	// Homes are in visual columns: a cell is CellW across and RowH rows down,
	// and a row is RowAspect columns tall.
	for gy := 0; gy < r.h; gy++ {
		for gx := 0; gx < r.w; gx++ {
			r.home[gy*r.w+gx] = physics.Vec2{
				X: float64(gx) * r.cellW,
				Y: float64(gy) * r.rowH * RowAspect,
			}
		}
	}

	// c = a·sqrt(k/m) at unit mass, so k = (c/a)² per axis keeps the ring
	// round.
	ax, ay := r.cellW, r.rowH*RowAspect
	r.kx = (rippleSpeed / ax) * (rippleSpeed / ax)
	r.ky = (rippleSpeed / ay) * (rippleSpeed / ay)
}

// Kick throws the sheet outward from a grid cell.
func (r *Ripple) Kick(gx, gy float64) {
	if r.world == nil {
		return
	}
	cx := (gx - float64(r.x)) * float64(r.cellW)
	cy := (gy - float64(r.y)) * float64(r.rowH) * RowAspect

	for i := range r.home {
		d := r.world.Pos(i).Sub(physics.Vec2{X: cx, Y: cy})
		n := d.Len()
		if n > r.core {
			continue
		}
		if n < 1e-6 {
			// Dead centre: nowhere to push it, and the ring closes over it.
			continue
		}
		// Linear falloff to zero at the rim, so the impulse has no edge for
		// the sheet to ring on.
		v := d.Scale(rippleKick * (1 - n/r.core) / n)
		r.world.SetVelocity(i, r.world.Velocity(i, physics.DT).Add(v))
	}
	r.moving = true
}

// Step advances the sheet. It runs its own substep loop because
// World.StepForces evaluates forces once per frame, and spring forces must be
// recomputed from the current positions every substep.
func (r *Ripple) Step(dt float64) {
	if r.world == nil || !r.moving {
		return
	}
	h := dt / physics.Substeps
	for s := 0; s < physics.Substeps; s++ {
		r.accumulate()
		r.world.IntegrateForces(h, r.force)
	}
	r.settle()
}

// accumulate fills the force array: each particle toward its own place, and each
// lattice spring once from the particle on its left or above.
func (r *Ripple) accumulate() {
	for i := range r.force {
		r.force[i] = r.home[i].Sub(r.world.Pos(i)).Scale(rippleHome)
	}

	// Springs act on the difference in displacement, not the distance between
	// particles, so each particle is pulled toward its neighbours' average and
	// a bump spreads as a wave. Distance springs barely propagate anything.
	for gy := 0; gy < r.h; gy++ {
		for gx := 0; gx < r.w; gx++ {
			i := gy*r.w + gx
			ui := r.world.Pos(i).Sub(r.home[i])

			if gx+1 < r.w {
				j := i + 1
				d := r.world.Pos(j).Sub(r.home[j]).Sub(ui).Scale(r.kx)
				r.force[i] = r.force[i].Add(d)
				r.force[j] = r.force[j].Sub(d)
			}
			if gy+1 < r.h {
				j := i + r.w
				d := r.world.Pos(j).Sub(r.home[j]).Sub(ui).Scale(r.ky)
				r.force[i] = r.force[i].Add(d)
				r.force[j] = r.force[j].Sub(d)
			}
		}
	}
}

// settle snaps the sheet home and stops simulating once nothing visible is
// left.
func (r *Ripple) settle() {
	for i := range r.home {
		if r.world.Pos(i).Sub(r.home[i]).LenSq() > rippleQuiet*rippleQuiet {
			return
		}
		if r.world.Velocity(i, physics.DT).LenSq() > rippleStill*rippleStill {
			return
		}
	}
	for i := range r.home {
		r.world.SetPos(i, r.home[i])
		r.world.SetVelocity(i, physics.Vec2{})
	}
	r.moving = false
}

// Offset is how far the sheet has moved the digit at a grid cell, in cells.
func (r *Ripple) Offset(gx, gy int) (dx, dy float64) {
	if r.world == nil || !r.moving {
		return 0, 0
	}
	cx, cy := gx-r.x, gy-r.y
	if cx < 0 || cy < 0 || cx >= r.w || cy >= r.h {
		return 0, 0
	}
	i := cy*r.w + cx
	d := r.world.Pos(i).Sub(r.home[i])

	// Convert to cells, the unit Agitation and Drift return and ScreenAnchor
	// sums.
	return d.X / float64(r.cellW), d.Y / (float64(r.rowH) * RowAspect)
}

// Moving reports whether anything is displaced.
func (r *Ripple) Moving() bool { return r != nil && r.moving }

// Missed writes off every detonation up to seq without rippling for it. A
// blast stays on the board for blastLife, so a refiner joining just after one
// would otherwise be shaken by it.
func (r *Ripple) Missed(seq int) {
	if r != nil && seq > r.seen {
		r.seen = seq
	}
}

// catchBlasts throws every detonation this view has not yet seen into its
// sheet. The room records where a cluster went; each terminal ripples in its
// own screen space, as with flights.
func (m *Model) catchBlasts() {
	if m.Room == nil || m.ripple == nil {
		return
	}
	for _, b := range m.blasts {
		if b.Seq <= m.ripple.seen {
			continue
		}
		m.ripple.seen = b.Seq
		m.ripple.Kick(b.X, b.Y)
	}
}

// rippleOffset is what the sheet has done to one cell, in cells.
func (m *Model) rippleOffset(gx, gy int) (dx, dy float64) {
	if m.ripple == nil {
		return 0, 0
	}
	return m.ripple.Offset(gx, gy)
}
