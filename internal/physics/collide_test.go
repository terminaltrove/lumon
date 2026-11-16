package physics

import (
	"math"
	"testing"
)

// Distance cannot prevent overlap: it pulls as hard as it pushes.
func TestTwoOverlappingParticlesAreSeparated(t *testing.T) {
	w := NewWorld()
	a := w.Add(Vec2{X: 0, Y: 0}, 1)
	b := w.Add(Vec2{X: 0.4, Y: 0}, 1)
	w.AddConstraint(NewCollide(1))

	w.SolveN(40)

	if got := w.Pos(a).Sub(w.Pos(b)).Len(); got < 1.9 {
		t.Errorf("two particles of radius 1 settled %v apart, want about 2", got)
	}
}

// A contact is not a spring.
func TestParticlesOutOfReachAreLeftAlone(t *testing.T) {
	w := NewWorld()
	a := w.Add(Vec2{}, 1)
	b := w.Add(Vec2{X: 50}, 1)
	w.AddConstraint(NewCollide(1))

	before := w.Pos(a).Sub(w.Pos(b)).Len()
	w.SolveN(40)
	if after := w.Pos(a).Sub(w.Pos(b)).Len(); math.Abs(after-before) > 1e-9 {
		t.Errorf("two particles fifty apart were pulled to %v; contacts must not attract", after)
	}
}

func TestDroppedParticlesStackRatherThanMerge(t *testing.T) {
	const n = 12
	w := NewWorld()
	for i := 0; i < n; i++ {
		w.Add(Vec2{X: 0, Y: float64(-i) * 3}, 1)
	}
	w.AddConstraint(NewCollide(1))
	w.AddConstraint(NewBounds(Vec2{X: -50, Y: -1000}, Vec2{X: 50, Y: 0}))

	for s := 0; s < 1200; s++ {
		w.Step(DT, Vec2{Y: 300})
	}

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if d := w.Pos(i).Sub(w.Pos(j)).Len(); d < 1.6 {
				t.Fatalf("particles %d and %d ended %v apart; they are inside each other", i, j, d)
			}
		}
	}

	// The highest particle is well above the floor.
	top := 0.0
	for i := 0; i < n; i++ {
		if y := w.Pos(i).Y; y < top {
			top = y
		}
	}
	if top > -8 {
		t.Errorf("twelve particles piled only %v high; they are not stacking", -top)
	}
}

// This is what makes a dragged digit hold its place.
func TestAPinnedParticleIsNotPushedByContacts(t *testing.T) {
	w := NewWorld()
	held := w.Add(Vec2{}, 0)
	w.Add(Vec2{X: 0.1}, 1)
	w.AddConstraint(NewCollide(1))

	w.SolveN(40)
	if got := w.Pos(held); got.X != 0 || got.Y != 0 {
		t.Errorf("a held particle was pushed to %v by something resting against it", got)
	}
}

func TestTheBroadPhaseFindsEveryContact(t *testing.T) {
	w := NewWorld()
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			w.Add(Vec2{X: float64(x) * 1.2, Y: float64(y) * 1.2}, 1)
		}
	}
	w.AddConstraint(NewCollide(1))
	w.SolveN(60)

	n := w.Len()
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if d := w.Pos(i).Sub(w.Pos(j)).Len(); d < 1.5 {
				t.Fatalf("particles %d and %d are still %v apart after sixty passes", i, j, d)
			}
		}
	}
}

// Contacts come from positions, not declared pairs, so they must be found after
// a particle moves into a new bin.
func TestContactsAreFoundAfterParticlesMove(t *testing.T) {
	w := NewWorld()
	a := w.Add(Vec2{}, 1)
	b := w.Add(Vec2{X: 100}, 1)
	w.AddConstraint(NewCollide(1))
	w.SolveN(5) // far apart, nothing happens

	w.SetPos(b, Vec2{X: 0.2}) // now on top of a
	w.SolveN(40)

	if got := w.Pos(a).Sub(w.Pos(b)).Len(); got < 1.9 {
		t.Errorf("a particle moved into contact was not separated; %v apart", got)
	}
}

// Six hundred digits is the board size, and the constraint runs forty-eight
// times a step; all-pairs would be eight and a half million tests a frame.
func BenchmarkCollideAtBoardSize(b *testing.B) {
	w := NewWorld()
	for i := 0; i < 600; i++ {
		w.Add(Vec2{X: float64(i%30) * 2.1, Y: float64(i/30) * 2.1}, 1)
	}
	c := NewCollide(1)
	w.AddConstraint(c)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Solve()
	}
}

// Contact is discovered inside the broad phase; finding it outside would need
// a second spatial hash.
func TestContactsAreReported(t *testing.T) {
	w := NewWorld()
	a := w.Add(Vec2{}, 1)
	b := w.Add(Vec2{X: 0.5}, 1)
	w.Add(Vec2{X: 50}, 1) // far away, must never be reported

	c := NewCollide(1)
	var pairs [][2]int
	c.OnContact = func(i, j int) { pairs = append(pairs, [2]int{i, j}) }
	w.AddConstraint(c)

	w.Solve()

	if len(pairs) == 0 {
		t.Fatal("two overlapping particles reported no contact")
	}
	for _, p := range pairs {
		if (p[0] != a || p[1] != b) && (p[0] != b || p[1] != a) {
			t.Errorf("reported a contact between %d and %d; only %d and %d are touching",
				p[0], p[1], a, b)
		}
	}
}

func TestCollideWithNoHookBehavesAsBefore(t *testing.T) {
	run := func(hook func(i, j int)) float64 {
		w := NewWorld()
		a := w.Add(Vec2{}, 1)
		b := w.Add(Vec2{X: 0.4}, 1)
		c := NewCollide(1)
		c.OnContact = hook
		w.AddConstraint(c)
		w.SolveN(40)
		return w.Pos(a).Sub(w.Pos(b)).Len()
	}
	quiet := run(nil)
	noisy := run(func(int, int) {})
	if quiet != noisy {
		t.Errorf("separation differs with a hook attached: %v against %v", quiet, noisy)
	}
}

// A pair is in contact when closer than the sum of their two radii.
func TestAWideParticleKeepsMoreRoomThanANarrowOne(t *testing.T) {
	// The same pair and start, once both narrow, once with one wide. A
	// settled distance cannot be asserted: separation is position-based, so
	// the correction becomes velocity and an overlapping pair springs apart
	// instead of resting in contact. This compares how far.
	apart := func(radii []float64) float64 {
		w := NewWorld()
		w.SetDamping(0.9)
		a := w.Add(Vec2{X: 0, Y: 0}, 1)
		b := w.Add(Vec2{X: 1.5, Y: 0}, 1)
		c := NewCollide(1)
		c.Radii = radii
		w.AddConstraint(c)
		for i := 0; i < 60; i++ {
			w.Step(1.0/60, Vec2{})
		}
		return w.Pos(b).Sub(w.Pos(a)).Len()
	}

	narrow := apart(nil)
	wide := apart([]float64{2.5, 1})

	if narrow < 2 {
		t.Errorf("two ordinary particles stayed %.2f apart, inside their contact", narrow)
	}
	if wide < narrow*1.3 {
		t.Errorf("a wide particle pushed to %.2f where a narrow one reached %.2f;"+
			" the extra radius did nothing", wide, narrow)
	}
}

// Radii covering only some of the board leaves the rest at the shared radius.
func TestRadiiOnlyOverridesWhatItCovers(t *testing.T) {
	w := NewWorld()
	w.SetDamping(0.9)
	a := w.Add(Vec2{X: 0, Y: 0}, 1)
	b := w.Add(Vec2{X: 1.5, Y: 0}, 1)
	c := NewCollide(1)
	c.Radii = []float64{0} // present, but says nothing about either
	w.AddConstraint(c)

	for i := 0; i < 60; i++ {
		w.Step(1.0/60, Vec2{})
	}
	if got := w.Pos(b).Sub(w.Pos(a)).Len(); got < 2 {
		t.Errorf("an uncovered pair stayed %.2f apart, inside their contact", got)
	}
}

// The hash is binned by the largest radius in play. Binned by the shared one, a
// wide particle's contacts would reach past the nine cells the walk checks and
// be missed.
func TestAWideParticleIsStillFoundAcrossTheHash(t *testing.T) {
	w := NewWorld()
	w.SetDamping(1)
	a := w.Add(Vec2{X: 0, Y: 0}, 1)
	b := w.Add(Vec2{X: 5, Y: 0}, 1)
	for i := 0; i < 40; i++ {
		w.Add(Vec2{X: float64(20 + i), Y: 40}, 1) // enough to make a grid
	}

	c := NewCollide(0.5)
	c.Radii = []float64{4, 4}
	c.Stiffness = 1
	w.AddConstraint(c)

	for i := 0; i < 60; i++ {
		w.Step(1.0/60, Vec2{})
	}
	if got := w.Pos(b).Sub(w.Pos(a)).Len(); got < 7.5 {
		t.Errorf("two wide particles stayed %.2f apart, want about 8", got)
	}
}
