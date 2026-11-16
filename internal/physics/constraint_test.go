package physics

import (
	"math"
	"testing"
)

func dist(w *World, a, b int) float64 { return w.Pos(b).Sub(w.Pos(a)).Len() }

func TestDistanceConstraintPullsParticlesToRestLength(t *testing.T) {
	w := NewWorld()
	a := w.Add(Vec2{0, 0}, 1)
	b := w.Add(Vec2{10, 0}, 1)
	w.AddConstraint(NewDistance(a, b, 4, 1))

	for n := 0; n < Iterations; n++ {
		w.Solve()
	}

	closeTo(t, dist(w, a, b), 4, "distance after solving")
}

func TestDistanceConstraintPushesApartWhenTooClose(t *testing.T) {
	w := NewWorld()
	a := w.Add(Vec2{0, 0}, 1)
	b := w.Add(Vec2{1, 0}, 1)
	w.AddConstraint(NewDistance(a, b, 5, 1))

	for n := 0; n < Iterations; n++ {
		w.Solve()
	}

	closeTo(t, dist(w, a, b), 5, "distance after solving")
}

func TestDistanceConstraintSplitsCorrectionByEqualMass(t *testing.T) {
	w := NewWorld()
	a := w.Add(Vec2{0, 0}, 1)
	b := w.Add(Vec2{10, 0}, 1)
	w.AddConstraint(NewDistance(a, b, 4, 1))

	w.Solve()

	closeTo(t, w.Pos(a).X, 3, "a.X")
	closeTo(t, w.Pos(b).X, 7, "b.X")
}

// A pinned neighbour (invMass 0) absorbs none of the correction. This anchors a
// lumon cluster.
func TestDistanceConstraintMovesOnlyTheFreeParticle(t *testing.T) {
	w := NewWorld()
	anchor := w.Add(Vec2{0, 0}, 0)
	free := w.Add(Vec2{10, 0}, 1)
	w.AddConstraint(NewDistance(anchor, free, 4, 1))

	w.Solve()

	closeToVec(t, w.Pos(anchor), Vec2{0, 0}, "anchor")
	closeTo(t, w.Pos(free).X, 4, "free.X")
}

// The solver must not divide by a zero total inverse mass.
func TestDistanceConstraintLeavesTwoPinnedParticlesUntouched(t *testing.T) {
	w := NewWorld()
	a := w.Add(Vec2{0, 0}, 0)
	b := w.Add(Vec2{10, 0}, 0)
	w.AddConstraint(NewDistance(a, b, 4, 1))

	w.Solve()

	closeToVec(t, w.Pos(a), Vec2{0, 0}, "a")
	closeToVec(t, w.Pos(b), Vec2{10, 0}, "b")
}

// Coincident particles have no direction to separate along.
func TestDistanceConstraintSurvivesCoincidentParticles(t *testing.T) {
	w := NewWorld()
	a := w.Add(Vec2{3, 3}, 1)
	b := w.Add(Vec2{3, 3}, 1)
	w.AddConstraint(NewDistance(a, b, 4, 1))

	w.Solve()

	if math.IsNaN(w.Pos(a).X) || math.IsNaN(w.Pos(b).X) {
		t.Fatalf("solver produced NaN: a=%+v b=%+v", w.Pos(a), w.Pos(b))
	}
}

// Softer constraints apply a fraction of the correction per pass, making some
// clusters wobblier than others.
func TestDistanceConstraintStiffnessControlsCorrectionRate(t *testing.T) {
	solveOnce := func(stiffness float64) float64 {
		w := NewWorld()
		a := w.Add(Vec2{0, 0}, 1)
		b := w.Add(Vec2{10, 0}, 1)
		w.AddConstraint(NewDistance(a, b, 4, stiffness))
		w.Solve()
		return dist(w, a, b)
	}

	stiff := solveOnce(1.0)
	soft := solveOnce(0.25)

	closeTo(t, stiff, 4, "stiff distance after one pass")
	if soft <= 4 || soft >= 10 {
		t.Errorf("soft distance after one pass = %v, want between 4 and 10", soft)
	}
}

func TestPinHoldsParticleAtTarget(t *testing.T) {
	w := NewWorld()
	i := w.Add(Vec2{0, 0}, 1)
	w.AddConstraint(NewPin(i, Vec2{7, 7}))

	w.Step(1.0/60, Vec2{0, -10})

	closeToVec(t, w.Pos(i), Vec2{7, 7}, "pinned position")
}

// Step runs Iterations passes per substep, so a stiff constraint must be
// satisfied when Step returns.
func TestStepConvergesStiffConstraintWithinItsIterationBudget(t *testing.T) {
	w := NewWorld()
	a := w.Add(Vec2{0, 0}, 1)
	b := w.Add(Vec2{10, 0}, 1)
	w.AddConstraint(NewDistance(a, b, 4, 1))

	w.Step(1.0/60, Vec2{})

	closeTo(t, dist(w, a, b), 4, "distance after one Step")
}
