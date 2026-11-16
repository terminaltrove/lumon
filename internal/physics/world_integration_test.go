package physics

import "testing"

// buildCluster wires a small spring-mass body like a bad-number cluster:
// neighbouring cells bound by distance constraints, anchored at one corner.
func buildCluster() *World {
	w := NewWorld()
	var ids []int
	for y := 0; y < 3; y++ {
		for x := 0; x < 3; x++ {
			invMass := 1.0
			if x == 0 && y == 0 {
				invMass = 0 // anchor
			}
			ids = append(ids, w.Add(Vec2{float64(x), float64(y)}, invMass))
		}
	}
	at := func(x, y int) int { return ids[y*3+x] }
	for y := 0; y < 3; y++ {
		for x := 0; x < 3; x++ {
			if x+1 < 3 {
				w.AddConstraint(NewDistance(at(x, y), at(x+1, y), 1, 1))
			}
			if y+1 < 3 {
				w.AddConstraint(NewDistance(at(x, y), at(x, y+1), 1, 1))
			}
		}
	}
	return w
}

// Determinism lets lumon reproduce a session, and these tests depend on it.
func TestSteppingIsDeterministic(t *testing.T) {
	a, b := buildCluster(), buildCluster()

	for n := 0; n < 200; n++ {
		a.Step(DT, Vec2{0, -9.81})
		b.Step(DT, Vec2{0, -9.81})
	}

	for i := 0; i < a.Len(); i++ {
		if a.Pos(i) != b.Pos(i) {
			t.Fatalf("particle %d diverged: %+v vs %+v", i, a.Pos(i), b.Pos(i))
		}
	}
}

func TestConstrainedClusterStaysCoherentUnderGravity(t *testing.T) {
	w := buildCluster()

	for n := 0; n < 600; n++ {
		w.Step(DT, Vec2{0, -9.81})
	}

	for i := 0; i < w.Len(); i++ {
		p := w.Pos(i)
		if p.X != p.X || p.Y != p.Y { // NaN check
			t.Fatalf("particle %d went NaN: %+v", i, p)
		}
	}

	closeToVec(t, w.Pos(0), Vec2{0, 0}, "anchor")

	d := w.Pos(1).Sub(w.Pos(0)).Len()
	if d < 0.8 || d > 1.2 {
		t.Errorf("neighbour spacing after settling = %v, want near the rest length 1", d)
	}
}

// The refine-to-bin effect end to end. Seek, not PointAttractor: an
// inverse-square pull is weak at range and overshoots up close, which suits a
// starfield and not a digit that must land in a bin.
func TestReleasedParticleReachesItsSeekTarget(t *testing.T) {
	w := NewWorld()
	digit := w.Add(Vec2{0, 0}, 1)
	bin := Vec2{20, -5}
	seek := NewSeek(bin, 3.0)

	for n := 0; n < 240; n++ {
		w.Integrate(DT, seek.Force(w.Pos(digit), w.Velocity(digit, DT)))
	}

	if d := w.Pos(digit).Sub(bin).Len(); d > 0.5 {
		t.Errorf("digit ended %v from the bin, want it to arrive within 0.5", d)
	}
}

func TestSeekSettlesWithoutOvershooting(t *testing.T) {
	w := NewWorld()
	digit := w.Add(Vec2{0, 0}, 1)
	target := Vec2{10, 0}
	seek := NewSeek(target, 3.0)

	maxX := 0.0
	for n := 0; n < 600; n++ {
		w.Integrate(DT, seek.Force(w.Pos(digit), w.Velocity(digit, DT)))
		if x := w.Pos(digit).X; x > maxX {
			maxX = x
		}
	}

	if maxX > target.X+0.2 {
		t.Errorf("overshot to X=%v, want no more than %v", maxX, target.X+0.2)
	}
	closeToVecTol(t, w.Pos(digit), target, 0.1, "settled position")
}

// Inverse-square falloff closes distance but does not promise arrival.
func TestPointAttractorClosesDistanceButIsWeakAtRange(t *testing.T) {
	w := NewWorld()
	p := w.Add(Vec2{0, 0}, 1)
	target := Vec2{20, -5}
	a := PointAttractor{At: target, Strength: 400, Softening: 2}

	start := w.Pos(p).Sub(target).Len()
	for n := 0; n < 240; n++ {
		w.Integrate(DT, a.Force(w.Pos(p)))
	}
	end := w.Pos(p).Sub(target).Len()

	if end >= start {
		t.Errorf("distance went %v -> %v, want it to close", start, end)
	}
}

// Clearing constraints is how a refined cluster stops behaving as a body.
func TestClearConstraintsReleasesTheCluster(t *testing.T) {
	w := buildCluster()
	w.SetInvMass(0, 1) // unpin the anchor too
	w.ClearConstraints()

	before := w.Pos(4)
	for n := 0; n < 60; n++ {
		w.Step(DT, Vec2{0, -9.81})
	}

	if w.Pos(4).Y >= before.Y {
		t.Errorf("released particle did not fall: %v -> %v", before, w.Pos(4))
	}
}
