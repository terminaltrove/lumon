package physics

import (
	"math"
	"testing"
)

func TestBruteForceIsEqualAndOpposite(t *testing.T) {
	bodies := []Body{
		{Pos: Vec2{0, 0}, Mass: 2},
		{Pos: Vec2{3, 4}, Mass: 5},
	}

	f := BruteForce(bodies, 1, 0)

	closeToVec(t, f[0], f[1].Scale(-1), "f[0] vs -f[1]")
}

func TestBruteForceAttractsTowardTheOtherBody(t *testing.T) {
	bodies := []Body{
		{Pos: Vec2{0, 0}, Mass: 1},
		{Pos: Vec2{10, 0}, Mass: 1},
	}

	f := BruteForce(bodies, 1, 0)

	if f[0].X <= 0 {
		t.Errorf("f[0].X = %v, want positive (toward the body at +X)", f[0].X)
	}
	closeTo(t, f[0].Y, 0, "f[0].Y")
}

// Softened Newtonian gravity: |f| = G*m1*m2 / (r^2 + soft^2)^(3/2) * r, which
// with soft=0 reduces to the familiar inverse square.
func TestBruteForceFollowsInverseSquareLaw(t *testing.T) {
	at := func(r float64) float64 {
		f := BruteForce([]Body{
			{Pos: Vec2{0, 0}, Mass: 1},
			{Pos: Vec2{r, 0}, Mass: 1},
		}, 1, 0)
		return f[0].Len()
	}

	closeTo(t, at(1), 1, "force at r=1")
	closeTo(t, at(2), 0.25, "force at r=2")
	closeTo(t, at(4), 0.0625, "force at r=4")
}

func TestBruteForceScalesWithMassProduct(t *testing.T) {
	light := BruteForce([]Body{
		{Pos: Vec2{0, 0}, Mass: 1},
		{Pos: Vec2{1, 0}, Mass: 1},
	}, 1, 0)
	heavy := BruteForce([]Body{
		{Pos: Vec2{0, 0}, Mass: 2},
		{Pos: Vec2{1, 0}, Mass: 3},
	}, 1, 0)

	closeTo(t, heavy[0].Len(), light[0].Len()*6, "force with 6x mass product")
}

// Without the softening term, stars passing through each other would divide by
// zero.
func TestBruteForceSofteningKeepsCoincidentBodiesFinite(t *testing.T) {
	f := BruteForce([]Body{
		{Pos: Vec2{5, 5}, Mass: 1},
		{Pos: Vec2{5, 5}, Mass: 1},
	}, 1, 0.25)

	if math.IsNaN(f[0].X) || math.IsInf(f[0].X, 0) {
		t.Fatalf("coincident bodies produced %v", f[0])
	}
}

func TestBruteForceSofteningWeakensCloseRangeAttraction(t *testing.T) {
	hard := BruteForce([]Body{
		{Pos: Vec2{0, 0}, Mass: 1},
		{Pos: Vec2{0.1, 0}, Mass: 1},
	}, 1, 0)
	soft := BruteForce([]Body{
		{Pos: Vec2{0, 0}, Mass: 1},
		{Pos: Vec2{0.1, 0}, Mass: 1},
	}, 1, 0.25)

	if soft[0].Len() >= hard[0].Len() {
		t.Errorf("softened force %v should be weaker than unsoftened %v", soft[0].Len(), hard[0].Len())
	}
}

func TestBruteForceHandlesEmptyAndSingleBody(t *testing.T) {
	if got := BruteForce(nil, 1, 0); len(got) != 0 {
		t.Errorf("BruteForce(nil) = %v, want empty", got)
	}
	got := BruteForce([]Body{{Pos: Vec2{1, 1}, Mass: 1}}, 1, 0)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	closeToVec(t, got[0], Vec2{}, "force on a lone body")
}

func TestPointAttractorPullsTowardItsTarget(t *testing.T) {
	a := PointAttractor{At: Vec2{10, 0}, Strength: 4, Softening: 0.5}

	f := a.Force(Vec2{0, 0})

	if f.X <= 0 {
		t.Errorf("force %v should point toward +X", f)
	}
	closeTo(t, f.Y, 0, "force.Y")
}

func TestPointAttractorStrengthensWithProximity(t *testing.T) {
	a := PointAttractor{At: Vec2{10, 0}, Strength: 4, Softening: 0.5}

	near := a.Force(Vec2{9, 0}).Len()
	far := a.Force(Vec2{0, 0}).Len()

	if near <= far {
		t.Errorf("force at distance 1 (%v) should exceed force at distance 10 (%v)", near, far)
	}
}

func TestPointAttractorAtTargetProducesNoForce(t *testing.T) {
	a := PointAttractor{At: Vec2{3, 3}, Strength: 4, Softening: 0.5}

	f := a.Force(Vec2{3, 3})

	closeToVec(t, f, Vec2{}, "force at the target")
}
