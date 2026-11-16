package physics

// Solver constants from inertia's cloth.rs.
const (
	// Substeps splits each Step into smaller integrations. More substeps make
	// stiff constraints behave without raising iteration count.
	Substeps = 4
	// Iterations is how many times the constraint set is relaxed per substep,
	// trading accuracy for time.
	Iterations = 12
	// DefaultDamping is the share of velocity kept per substep.
	DefaultDamping = 0.99
)

// World is a set of Verlet particles, in parallel slices, plus the
// constraints binding them. Velocity is implicit: (pos - prev) is a particle's
// per-step displacement, so constraints move positions directly.
type World struct {
	pos     []Vec2
	prev    []Vec2
	invMass []float64

	constraints []Constraint

	damping float64
}

// NewWorld returns an empty world with inertia's default damping.
func NewWorld() *World {
	return &World{damping: DefaultDamping}
}

// Add appends a particle at pos and returns its index. An invMass of 0 pins the
// particle in place, which is how inertia anchors cloth.
func (w *World) Add(pos Vec2, invMass float64) int {
	w.pos = append(w.pos, pos)
	w.prev = append(w.prev, pos)
	w.invMass = append(w.invMass, invMass)
	return len(w.pos) - 1
}

// Len returns the particle count.
func (w *World) Len() int { return len(w.pos) }

// Pos returns particle i's current position.
func (w *World) Pos(i int) Vec2 { return w.pos[i] }

// SetPos teleports particle i without imparting velocity.
func (w *World) SetPos(i int, p Vec2) {
	w.pos[i] = p
	w.prev[i] = p
}

// InvMass returns particle i's inverse mass; 0 means pinned.
func (w *World) InvMass(i int) float64 { return w.invMass[i] }

// SetInvMass sets particle i's inverse mass. 0 pins the particle; restoring
// the old value releases it (see Grab).
func (w *World) SetInvMass(i int, m float64) { w.invMass[i] = m }

// Velocity returns particle i's implicit velocity in units per second, derived
// from the last step of size h.
func (w *World) Velocity(i int, h float64) Vec2 {
	if h == 0 {
		return Vec2{}
	}
	return w.pos[i].Sub(w.prev[i]).Scale(1 / h)
}

// SetVelocity seeds particle i's implicit velocity in units per second by
// placing its previous position behind it, measured against a 1/60s step.
func (w *World) SetVelocity(i int, v Vec2) {
	w.prev[i] = w.pos[i].Sub(v.Scale(velocityRefStep))
}

// velocityRefStep is the step, in seconds, that SetVelocity converts units per
// second against.
const velocityRefStep = 1.0 / 60

// Damping returns the current per-step velocity retention.
func (w *World) Damping() float64 { return w.damping }

// SetDamping sets per-step velocity retention: 1 conserves energy, lower values
// bleed it.
func (w *World) SetDamping(d float64) { w.damping = d }

// AddConstraint registers a constraint, solved every iteration of every substep.
func (w *World) AddConstraint(c Constraint) { w.constraints = append(w.constraints, c) }

// ClearConstraints drops every constraint, leaving particles free.
func (w *World) ClearConstraints() { w.constraints = w.constraints[:0] }

// Integrate advances every particle by one Verlet step of size h under a shared
// acceleration. Ported from inertia's cloth.rs integrate.
func (w *World) Integrate(h float64, accel Vec2) {
	hh := h * h
	for i := range w.pos {
		if w.invMass[i] == 0 {
			// Pinned. Keep prev in step so releasing does not fling it.
			w.prev[i] = w.pos[i]
			continue
		}
		cur := w.pos[i]
		vel := cur.Sub(w.prev[i]).Scale(w.damping)
		w.pos[i] = cur.Add(vel).Add(accel.Scale(hh))
		w.prev[i] = cur
	}
}

// IntegrateForces advances every particle by one Verlet step of size h,
// applying a separate acceleration to each. Entries beyond len(forces) are
// zero. Do not call Integrate once per particle instead: that advances the
// world N times.
func (w *World) IntegrateForces(h float64, forces []Vec2) {
	hh := h * h
	for i := range w.pos {
		if w.invMass[i] == 0 {
			w.prev[i] = w.pos[i]
			continue
		}
		var a Vec2
		if i < len(forces) {
			a = forces[i]
		}
		cur := w.pos[i]
		vel := cur.Sub(w.prev[i]).Scale(w.damping)
		w.pos[i] = cur.Add(vel).Add(a.Scale(hh))
		w.prev[i] = cur
	}
}

// SolveN relaxes the constraint set n times.
func (w *World) SolveN(n int) {
	for i := 0; i < n; i++ {
		w.Solve()
	}
}

// Solve relaxes every constraint once. Step runs Iterations of these per
// substep.
func (w *World) Solve() {
	for _, c := range w.constraints {
		c.Solve(w)
	}
}

// Step advances the world by dt seconds as inertia's Cloth::step does: each of
// Substeps integrates, then relaxes the constraints Iterations times.
func (w *World) Step(dt float64, accel Vec2) {
	h := dt / Substeps
	for s := 0; s < Substeps; s++ {
		w.Integrate(h, accel)
		for n := 0; n < Iterations; n++ {
			w.Solve()
		}
	}
}

// StepForces is Step with an acceleration per particle. Falling digits each
// fall at their own rate, so they do not come down as one sheet. Entries
// beyond len(accel) are zero.
func (w *World) StepForces(dt float64, accel []Vec2) {
	h := dt / Substeps
	for s := 0; s < Substeps; s++ {
		w.IntegrateForces(h, accel)
		for n := 0; n < Iterations; n++ {
			w.Solve()
		}
	}
}
