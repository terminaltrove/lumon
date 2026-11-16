package physics

// Constraint is a positional rule relaxed by the solver each iteration. In
// position-based dynamics a constraint moves particles directly; it applies no
// force.
type Constraint interface {
	Solve(w *World)
}

// Distance keeps two particles a fixed distance apart (inertia's cloth.rs
// build_constraints). Lumon uses it to make a bad-number cluster wobble as one
// body. Stiffness in [0,1] is the share of the error corrected per pass; below
// 1 the constraint relaxes over several iterations and looks soft.
type Distance struct {
	A, B      int
	Rest      float64
	Stiffness float64
}

// NewDistance returns a Distance constraint between particles a and b.
func NewDistance(a, b int, rest, stiffness float64) *Distance {
	return &Distance{A: a, B: b, Rest: rest, Stiffness: stiffness}
}

// Solve moves the pair toward Rest, splitting the correction in proportion to
// inverse mass so heavier particles yield less and pinned ones not at all.
func (d *Distance) Solve(w *World) {
	pa, pb := w.pos[d.A], w.pos[d.B]
	ima, imb := w.invMass[d.A], w.invMass[d.B]

	total := ima + imb
	if total == 0 {
		// Both pinned.
		return
	}

	delta := pb.Sub(pa)
	length := delta.Len()
	if length == 0 {
		// Coincident particles have no separation direction; a later step
		// breaks the tie.
		return
	}

	correction := (length - d.Rest) / length * d.Stiffness
	offset := delta.Scale(correction / total)

	w.pos[d.A] = pa.Add(offset.Scale(ima))
	w.pos[d.B] = pb.Sub(offset.Scale(imb))
}

// Pin holds a particle at a fixed point. Unlike an invMass of 0, a Pin can be
// moved every frame, as inertia drags a grabbed cloth vertex (cloth.rs
// drag_to).
type Pin struct {
	I  int
	At Vec2
}

// NewPin returns a Pin holding particle i at the given point.
func NewPin(i int, at Vec2) *Pin { return &Pin{I: i, At: at} }

// Solve snaps the particle to its target and moves its previous position too,
// cancelling its velocity.
func (p *Pin) Solve(w *World) {
	w.pos[p.I] = p.At
	w.prev[p.I] = p.At
}
