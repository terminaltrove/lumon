package physics

import "testing"

func TestAGrabbedParticleFollowsTheHand(t *testing.T) {
	w := NewWorld()
	i := w.Add(Vec2{X: 5, Y: 5}, 1)
	g := NewGrab(w)

	if !g.Take(Vec2{X: 5, Y: 5}, 2) {
		t.Fatal("nothing was picked up from right on top of a particle")
	}
	if got, ok := g.Holding(); !ok || got != i {
		t.Fatalf("holding %d (%v), want %d", got, ok, i)
	}

	for x := 5.0; x < 30; x++ {
		g.MoveTo(Vec2{X: x, Y: 12})
		w.Step(DT, Vec2{Y: 500}) // gravity pulling hard the whole time
	}
	if got := w.Pos(i); got.X != 29 || got.Y != 12 {
		t.Errorf("the held particle is at %v, not where the hand left it", got)
	}
}

func TestAGrabFindsTheNearestAndOnlyWithinReach(t *testing.T) {
	w := NewWorld()
	w.Add(Vec2{X: 0}, 1)
	near := w.Add(Vec2{X: 9}, 1)
	g := NewGrab(w)

	if !g.Take(Vec2{X: 10}, 5) {
		t.Fatal("nothing was picked up from within reach")
	}
	if got, _ := g.Holding(); got != near {
		t.Errorf("picked up %d, want the nearest %d", got, near)
	}

	g.Release()
	if g.Take(Vec2{X: 500}, 5) {
		t.Error("something was picked up from five hundred away")
	}
	if _, ok := g.Holding(); ok {
		t.Error("a failed grab left something held")
	}
}

// Grab is its own type because Pin.Solve snaps prev to pos, which cancels
// velocity: a pin-based drag would drop a thrown particle straight down.
func TestAThrownParticleKeepsItsMomentum(t *testing.T) {
	w := NewWorld()
	i := w.Add(Vec2{}, 1)
	g := NewGrab(w)
	g.Take(Vec2{}, 1)

	for x := 0.0; x < 20; x += 2 {
		g.MoveTo(Vec2{X: x})
	}
	g.Release()

	before := w.Pos(i).X
	for n := 0; n < 30; n++ {
		w.Step(DT, Vec2{})
	}
	if got := w.Pos(i).X; got <= before+1 {
		t.Errorf("a thrown particle travelled from %v to %v; it was released dead", before, got)
	}
}

// A heavy digit picked up must not come back with the default mass.
func TestReleasingRestoresTheParticlesOwnMass(t *testing.T) {
	w := NewWorld()
	i := w.Add(Vec2{}, 0.25)
	g := NewGrab(w)

	g.Take(Vec2{}, 1)
	if got := w.InvMass(i); got != 0 {
		t.Errorf("a held particle has inverse mass %v, want 0 so nothing shoves it", got)
	}
	g.Release()
	if got := w.InvMass(i); got != 0.25 {
		t.Errorf("a released particle has inverse mass %v, want the 0.25 it had", got)
	}
}

func TestTakingSomethingElseReleasesTheFirst(t *testing.T) {
	w := NewWorld()
	a := w.Add(Vec2{}, 1)
	b := w.Add(Vec2{X: 20}, 1)
	g := NewGrab(w)

	g.Take(Vec2{}, 1)
	g.Take(Vec2{X: 20}, 1)

	if got, _ := g.Holding(); got != b {
		t.Errorf("holding %d, want %d", got, b)
	}
	if w.InvMass(a) == 0 {
		t.Error("the first particle is still pinned")
	}
}
