package lumon

import (
	"math"
	"math/rand"
	"testing"
)

func newTestAgitation() (*Agitation, *Grid, *BadGroup) {
	g := NewGrid(1)
	// Group 0 is the 7-cell cluster starting at (7,0).
	return NewAgitation(rand.New(rand.NewSource(7))), g, g.Groups[0]
}

func maxOffset(a *Agitation, grp *BadGroup) float64 {
	worst := 0.0
	for _, id := range grp.NumberIDs {
		dx, dy := a.Offset(id)
		if d := math.Hypot(dx, dy); d > worst {
			worst = d
		}
	}
	return worst
}

func TestUntrackedNumberHasNoOffset(t *testing.T) {
	a, _, _ := newTestAgitation()

	dx, dy := a.Offset(12345)

	if dx != 0 || dy != 0 {
		t.Errorf("Offset of an untracked number = (%v,%v), want (0,0)", dx, dy)
	}
}

// Corrupt digits hide in the grid until their group activates.
func TestDormantClusterStaysPutAndDoesNotBetrayItself(t *testing.T) {
	a, g, grp := newTestAgitation()
	a.Track(grp, g)

	for i := 0; i < 120; i++ {
		a.Step(1.0 / 60)
	}

	if got := maxOffset(a, grp); got > 0.01 {
		t.Errorf("dormant cluster drifted %v cells, want it still", got)
	}
}

func TestActivatingAClusterMakesItMove(t *testing.T) {
	a, g, grp := newTestAgitation()
	a.Track(grp, g)
	a.SetIntensity(grp.ID, 1)

	for i := 0; i < 120; i++ {
		a.Step(1.0 / 60)
	}

	if got := maxOffset(a, grp); got < 0.02 {
		t.Errorf("active cluster moved only %v cells, want visible agitation", got)
	}
}

// Hovering an active cluster makes it "super active", as in the show and the
// C++ reference.
func TestHigherIntensityAgitatesMore(t *testing.T) {
	measure := func(intensity float64) float64 {
		a, g, grp := newTestAgitation()
		a.Track(grp, g)
		a.SetIntensity(grp.ID, intensity)
		sum := 0.0
		for i := 0; i < 600; i++ {
			a.Step(1.0 / 60)
			sum += maxOffset(a, grp)
		}
		return sum / 600
	}

	calm := measure(0.3)
	frantic := measure(1.0)

	if frantic <= calm {
		t.Errorf("intensity 1.0 averaged %v, intensity 0.3 averaged %v; want more", frantic, calm)
	}
}

func TestAgitationStaysBounded(t *testing.T) {
	a, g, grp := newTestAgitation()
	a.Track(grp, g)
	a.SetIntensity(grp.ID, 1)

	// The clamp normalises and rescales, so the bound holds only to within
	// floating point error.
	const tolerance = 1e-9

	for i := 0; i < 3000; i++ {
		a.Step(1.0 / 60)
		if got := maxOffset(a, grp); got > MaxAgitationOffset+tolerance {
			t.Fatalf("offset reached %v cells at step %d, want at most %v", got, i, MaxAgitationOffset)
		}
	}
}

// Neighbours are bound by distance constraints, so they stay roughly a cell
// apart.
func TestClusterMembersHoldTogether(t *testing.T) {
	a, g, grp := newTestAgitation()
	a.Track(grp, g)
	a.SetIntensity(grp.ID, 1)

	for i := 0; i < 600; i++ {
		a.Step(1.0 / 60)
	}

	for _, id := range grp.NumberIDs {
		x, y := id%GridSize, id/GridSize
		dx, dy := a.Offset(id)
		for _, other := range grp.NumberIDs {
			if other == id {
				continue
			}
			ox, oy := other%GridSize, other/GridSize
			if abs(ox-x) > 1 || abs(oy-y) > 1 {
				continue // not a neighbour
			}
			odx, ody := a.Offset(other)
			home := math.Hypot(float64(ox-x), float64(oy-y))
			now := math.Hypot(float64(ox-x)+odx-dx, float64(oy-y)+ody-dy)
			if math.Abs(now-home) > 0.6 {
				t.Fatalf("neighbours %d and %d are %v apart, want near %v", id, other, now, home)
			}
		}
	}
}

func TestAgitationIsDeterministicForASeed(t *testing.T) {
	run := func() []float64 {
		g := NewGrid(1)
		a := NewAgitation(rand.New(rand.NewSource(99)))
		grp := g.Groups[0]
		a.Track(grp, g)
		a.SetIntensity(grp.ID, 1)
		for i := 0; i < 300; i++ {
			a.Step(1.0 / 60)
		}
		var out []float64
		for _, id := range grp.NumberIDs {
			dx, dy := a.Offset(id)
			out = append(out, dx, dy)
		}
		return out
	}

	a, b := run(), run()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("value %d differs: %v vs %v", i, a[i], b[i])
		}
	}
}

func TestReleaseStopsTrackingACluster(t *testing.T) {
	a, g, grp := newTestAgitation()
	a.Track(grp, g)
	a.SetIntensity(grp.ID, 1)
	for i := 0; i < 60; i++ {
		a.Step(1.0 / 60)
	}

	a.Release(grp.ID)

	for _, id := range grp.NumberIDs {
		if dx, dy := a.Offset(id); dx != 0 || dy != 0 {
			t.Errorf("number %d still offset after release: (%v,%v)", id, dx, dy)
		}
	}
}

func TestTrackingIsIdempotent(t *testing.T) {
	a, g, grp := newTestAgitation()

	a.Track(grp, g)
	a.Track(grp, g)
	a.SetIntensity(grp.ID, 1)
	for i := 0; i < 60; i++ {
		a.Step(1.0 / 60)
	}

	if got := maxOffset(a, grp); got > MaxAgitationOffset {
		t.Errorf("double-tracking doubled the forces: offset %v", got)
	}
}

func TestClusterReturnsHomeWhenIntensityDropsToZero(t *testing.T) {
	a, g, grp := newTestAgitation()
	a.Track(grp, g)
	a.SetIntensity(grp.ID, 1)
	for i := 0; i < 300; i++ {
		a.Step(1.0 / 60)
	}

	a.SetIntensity(grp.ID, 0)
	for i := 0; i < 600; i++ {
		a.Step(1.0 / 60)
	}

	if got := maxOffset(a, grp); got > 0.02 {
		t.Errorf("cluster settled %v cells from home, want it to return", got)
	}
}

func TestAgitatedDigitsChangeColumnMoreThanBackgroundButNotFrantically(t *testing.T) {
	a, g, grp := newTestAgitation()
	a.Track(grp, g)
	a.SetIntensity(grp.ID, activeIntensity)

	const frames = 900
	last := map[int]int{}
	flips := map[int]int{}
	for i := 0; i < frames; i++ {
		a.Step(1.0 / 60)
		for _, id := range grp.NumberIDs {
			c := a.ColumnOffset(NewLayout(120, 40), id)
			if prev, seen := last[id]; seen && prev != c {
				flips[id]++
			}
			last[id] = c
		}
	}

	worst, total := 0, 0
	for _, n := range flips {
		if n > worst {
			worst = n
		}
		total += n
	}
	if total == 0 {
		t.Error("an active cluster never moved a column")
	}
	if worst > 40 {
		t.Errorf("a cluster digit changed column %d times in 15s; want insistent, not frantic", worst)
	}
}

func TestColumnOffsetOfAnUntrackedDigitIsZero(t *testing.T) {
	a, _, _ := newTestAgitation()

	if got := a.ColumnOffset(NewLayout(120, 40), 4242); got != 0 {
		t.Errorf("ColumnOffset of an untracked digit = %d, want 0", got)
	}
}

// Momentum is passed on to a flight, so it must be readable.
func TestVelocityReportsAnAgitatingDigitsMotion(t *testing.T) {
	a, g, grp := newTestAgitation()
	a.Track(grp, g)
	a.SetIntensity(grp.ID, superActiveIntensity)
	for i := 0; i < 120; i++ {
		a.Step(1.0 / 60)
	}

	moving := false
	for _, id := range grp.NumberIDs {
		v := a.Velocity(id)
		if v.X != 0 || v.Y != 0 {
			moving = true
		}
	}
	if !moving {
		t.Error("no cluster digit reported any velocity")
	}

	if v := a.Velocity(4242); v.X != 0 || v.Y != 0 {
		t.Errorf("an untracked digit reported velocity %+v", v)
	}
}

func seededRand(seed int64) *rand.Rand { return rand.New(rand.NewSource(seed)) }
