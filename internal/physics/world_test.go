package physics

import (
	"math"
	"testing"
)

func TestWorldAddParticleReturnsSequentialIndices(t *testing.T) {
	w := NewWorld()

	if got := w.Add(Vec2{1, 2}, 1); got != 0 {
		t.Errorf("first Add = %d, want 0", got)
	}
	if got := w.Add(Vec2{3, 4}, 1); got != 1 {
		t.Errorf("second Add = %d, want 1", got)
	}
	if got := w.Len(); got != 2 {
		t.Errorf("Len = %d, want 2", got)
	}
	closeToVec(t, w.Pos(1), Vec2{3, 4}, "Pos(1)")
}

// Verlet stores velocity implicitly as (pos - prev), so a particle created at
// rest with no force on it must not drift.
func TestIntegrateWithNoForceLeavesParticleAtRest(t *testing.T) {
	w := NewWorld()
	i := w.Add(Vec2{5, 5}, 1)

	for n := 0; n < 100; n++ {
		w.Integrate(1.0/60, Vec2{})
	}

	closeToVec(t, w.Pos(i), Vec2{5, 5}, "Pos after 100 idle steps")
}

// Verlet started from rest has an exact discrete solution,
// x_n = a*h^2*n*(n+1)/2, which lags the continuous s = 1/2*a*t^2 by a*h*t/2.
// Asserting it exactly is tighter than a tolerance around the continuous one.
func TestIntegrateUnderConstantAccelerationMatchesDiscreteClosedForm(t *testing.T) {
	w := NewWorld()
	i := w.Add(Vec2{}, 1)
	w.SetDamping(1) // isolate integration from damping

	const h = 1.0 / 60
	const steps = 60
	const a = -10.0

	for n := 0; n < steps; n++ {
		w.Integrate(h, Vec2{0, a})
	}

	want := a * h * h * steps * (steps + 1) / 2
	closeTo(t, w.Pos(i).Y, want, "Y after 60 steps")
}

// Halving h should roughly halve the error against the continuous solution.
func TestIntegrateConvergesToContinuousSolutionAsStepShrinks(t *testing.T) {
	const a = -10.0
	const duration = 1.0
	continuous := 0.5 * a * duration * duration

	errorAt := func(h float64) float64 {
		w := NewWorld()
		i := w.Add(Vec2{}, 1)
		w.SetDamping(1)
		for elapsed := 0.0; elapsed < duration-1e-12; elapsed += h {
			w.Integrate(h, Vec2{0, a})
		}
		return math.Abs(w.Pos(i).Y - continuous)
	}

	coarse := errorAt(1.0 / 60)
	fine := errorAt(1.0 / 120)

	if fine >= coarse*0.75 {
		t.Errorf("halving the step cut error from %v to %v; expected roughly half", coarse, fine)
	}
}

// Infinite mass is how inertia pins cloth particles (cloth.rs sets inv_mass to
// 0), and lumon needs the same for anchored digits.
func TestIntegrateDoesNotMoveInfiniteMassParticle(t *testing.T) {
	w := NewWorld()
	i := w.Add(Vec2{2, 2}, 0) // invMass 0 == pinned

	for n := 0; n < 10; n++ {
		w.Integrate(1.0/60, Vec2{0, -10})
	}

	closeToVec(t, w.Pos(i), Vec2{2, 2}, "pinned particle")
}

func TestSetVelocitySeedsTheImplicitVelocity(t *testing.T) {
	w := NewWorld()
	i := w.Add(Vec2{}, 1)
	w.SetDamping(1)
	w.SetVelocity(i, Vec2{6, 0}) // 6 units/sec

	w.Integrate(1.0/60, Vec2{})

	closeToVec(t, w.Pos(i), Vec2{0.1, 0}, "position after one step at 6/sec")
}

// Damping below 1 bleeds energy each step, which keeps lumon's jitter from
// running away.
func TestDampingReducesSpeedOverTime(t *testing.T) {
	w := NewWorld()
	i := w.Add(Vec2{}, 1)
	w.SetDamping(0.9)
	w.SetVelocity(i, Vec2{6, 0})

	w.Integrate(1.0/60, Vec2{})
	first := w.Pos(i).X
	w.Integrate(1.0/60, Vec2{})
	second := w.Pos(i).X - first

	if second >= first {
		t.Errorf("second step moved %v, want less than first step %v", second, first)
	}
}

// IntegrateForces is the per-particle form of Integrate, for a tether pulling
// each digit to its own cell or a random shake.
func TestIntegrateForcesAppliesADifferentForceToEachParticle(t *testing.T) {
	w := NewWorld()
	a := w.Add(Vec2{}, 1)
	b := w.Add(Vec2{}, 1)
	w.SetDamping(1)

	w.IntegrateForces(1.0/60, []Vec2{{X: 60}, {X: -60}})

	if w.Pos(a).X <= 0 {
		t.Errorf("particle a moved to %v, want +X", w.Pos(a))
	}
	if w.Pos(b).X >= 0 {
		t.Errorf("particle b moved to %v, want -X", w.Pos(b))
	}
	closeTo(t, w.Pos(a).X, -w.Pos(b).X, "equal and opposite displacement")
}

// Calling Integrate per particle would advance the world once per particle.
func TestIntegrateForcesAdvancesTheWorldExactlyOnce(t *testing.T) {
	shared := NewWorld()
	shared.Add(Vec2{}, 1)
	shared.Add(Vec2{}, 1)
	shared.SetDamping(1)

	single := NewWorld()
	single.Add(Vec2{}, 1)
	single.SetDamping(1)

	accel := Vec2{Y: -10}
	for n := 0; n < 30; n++ {
		shared.IntegrateForces(1.0/60, []Vec2{accel, accel})
		single.Integrate(1.0/60, accel)
	}

	closeTo(t, shared.Pos(0).Y, single.Pos(0).Y, "two-particle world vs one-particle world")
}

func TestIntegrateForcesTreatsMissingEntriesAsZero(t *testing.T) {
	w := NewWorld()
	w.Add(Vec2{}, 1)
	b := w.Add(Vec2{5, 5}, 1)
	w.SetDamping(1)

	w.IntegrateForces(1.0/60, []Vec2{{X: 60}})

	closeToVec(t, w.Pos(b), Vec2{5, 5}, "particle with no force entry")
}

func TestIntegrateForcesIgnoresPinnedParticles(t *testing.T) {
	w := NewWorld()
	p := w.Add(Vec2{3, 3}, 0)

	w.IntegrateForces(1.0/60, []Vec2{{X: 1000, Y: 1000}})

	closeToVec(t, w.Pos(p), Vec2{3, 3}, "pinned particle")
}

// SolveN runs several relaxation passes without Step's substep loop.
func TestSolveNRelaxesConstraintsRepeatedly(t *testing.T) {
	build := func() (*World, int, int) {
		w := NewWorld()
		a := w.Add(Vec2{0, 0}, 1)
		b := w.Add(Vec2{10, 0}, 1)
		w.AddConstraint(NewDistance(a, b, 4, 0.3))
		return w, a, b
	}

	one, a1, b1 := build()
	one.SolveN(1)
	many, a2, b2 := build()
	many.SolveN(Iterations)

	oneErr := absFloat(many.Pos(b1).Sub(one.Pos(a1)).Len())
	_ = oneErr
	after1 := absFloat(one.Pos(b1).Sub(one.Pos(a1)).Len() - 4)
	afterN := absFloat(many.Pos(b2).Sub(many.Pos(a2)).Len() - 4)

	if afterN >= after1 {
		t.Errorf("after %d passes error was %v, after 1 pass %v; want convergence", Iterations, afterN, after1)
	}
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func TestStepForcesLetsThingsFallAtDifferentRates(t *testing.T) {
	w := NewWorld()
	slow := w.Add(Vec2{}, 1)
	fast := w.Add(Vec2{X: 10}, 1)

	accel := []Vec2{{Y: 100}, {Y: 400}}
	for i := 0; i < 60; i++ {
		w.StepForces(DT, accel)
	}

	sy, fy := w.Pos(slow).Y, w.Pos(fast).Y
	if fy <= sy {
		t.Errorf("the one pulled four times harder is at %v against %v; it should be further down", fy, sy)
	}
	if sy <= 0 {
		t.Errorf("the slow one did not fall at all; it is at %v", sy)
	}
}

func TestStepForcesStillSolvesConstraints(t *testing.T) {
	w := NewWorld()
	i := w.Add(Vec2{}, 1)
	w.AddConstraint(NewBounds(Vec2{X: -100, Y: -100}, Vec2{X: 100, Y: 5}))

	for n := 0; n < 300; n++ {
		w.StepForces(DT, []Vec2{{Y: 500}})
	}
	if got := w.Pos(i).Y; got > 5.001 {
		t.Errorf("the particle is at %v, through a floor at 5", got)
	}
}

func TestStepForcesLeavesUnlistedParticlesAlone(t *testing.T) {
	w := NewWorld()
	a := w.Add(Vec2{}, 1)
	b := w.Add(Vec2{X: 10}, 1)

	for i := 0; i < 60; i++ {
		w.StepForces(DT, []Vec2{{Y: 300}}) // only a is listed
	}
	if w.Pos(a).Y <= 0 {
		t.Error("the listed particle did not move")
	}
	if got := w.Pos(b).Y; got != 0 {
		t.Errorf("the unlisted particle moved to %v", got)
	}
}
