package lumon

import (
	"math"
	"testing"
)

// BinTop is three rows below the last row a diversion may draw in, and the rule
// and bins are painted over the diversion. So the jets rise from the panel's
// bottom edge at each bin's centre column, where a refined digit's flight aims.
func TestTheFountainsRiseFromAboveEachBin(t *testing.T) {
	m := diverting(t, "FOUNTAIN", 120, 40)
	f := m.fountain
	if f == nil {
		t.Fatal("the fluid is not running")
	}
	l := m.Layout
	panel := m.gridPanel().Play()

	if len(f.mouths) != BinCount {
		t.Fatalf("%d jets for %d bins", len(f.mouths), BinCount)
	}
	for i, e := range f.mouths {
		if got, want := int(e.At.X), l.BinCenterX[i]; got != want {
			t.Errorf("jet %d is at column %d, want bin %d's centre at %d", i, got, i, want)
		}
		if row := int(e.At.Y / RowAspect); row != panel.Y+panel.H-1 {
			t.Errorf("jet %d springs from row %d, want the panel's last row %d",
				i, row, panel.Y+panel.H-1)
		}
		if e.Vel.Y >= 0 {
			t.Errorf("jet %d throws downward (%v)", i, e.Vel)
		}
	}

	// Nothing the simulation holds ever reaches the bins.
	for n := 0; n < 400; n++ {
		tick(m, 1)
		for i, p := range f.sim.Pos {
			if row := int(p.Y / RowAspect); row >= l.BinTop {
				t.Fatalf("frame %d: digit %d reached row %d, at or below BinTop %d",
					n, i, row, l.BinTop)
			}
		}
	}
}

func TestTheFountainsThrowTheDigitsUp(t *testing.T) {
	m := diverting(t, "FOUNTAIN", 120, 40)
	f := m.fountain
	panel := m.gridPanel().Play()

	best := math.Inf(1)
	for i := 0; i < 400; i++ {
		tick(m, 1)
		for _, p := range f.sim.Pos {
			best = math.Min(best, p.Y)
		}
	}
	rows := (f.max.Y - best) / RowAspect
	if rows < float64(panel.H)/3 {
		t.Errorf("nothing got higher than %.1f rows in a panel %d deep; the jets are "+
			"a puddle bubbling rather than fountains", rows, panel.H)
	}
}

// A digit that comes back down near a mouth is queued and thrown again.
// Without that the pool fills, every mouth stays crowded and the jets stop.
func TestTheFountainsNeverRunDry(t *testing.T) {
	m := diverting(t, "FOUNTAIN", 120, 40)
	f := m.fountain

	tick(m, 200)
	rising := 0
	for i := 0; i < 900; i++ {
		tick(m, 1)
		for _, v := range f.sim.Vel {
			if v.Y < -1 {
				rising++
				break
			}
		}
	}
	if rising < 600 {
		t.Errorf("something was on its way up on only %d of 900 frames; the fountains "+
			"have stopped", rising)
	}
}

// digits runs parallel to the simulation's positions and both Digits and Draw
// index it by particle, so an append to one without the other panics a frame
// later. Emission and the queue both move digits between the two.
func TestEveryDigitKeepsItsOwnPlaceInTheFountain(t *testing.T) {
	m := diverting(t, "FOUNTAIN", 120, 40)
	f := m.fountain

	flying, waiting := f.Airborne()
	total := flying + waiting
	if total == 0 {
		t.Fatal("no digits at all")
	}

	for i := 0; i < 600; i++ {
		tick(m, 1)
		if len(f.digits) != f.sim.Len() {
			t.Fatalf("frame %d: %d digits against %d particles", i, len(f.digits), f.sim.Len())
		}
		fly, wait := f.Airborne()
		if fly+wait != total {
			t.Fatalf("frame %d: %d digits, started with %d; one has been lost or made",
				i, fly+wait, total)
		}
		for _, d := range f.digits {
			if d < '0' || d > '9' {
				t.Fatalf("frame %d: a particle carries %q", i, d)
			}
		}
	}

	// Digits offers every one of them, queued or airborne, so the board always has
	// somewhere to fly to.
	if got := len(f.Digits()); got != total {
		t.Errorf("Digits offered %d places for %d digits", got, total)
	}
}

func TestTheFountainIsBounded(t *testing.T) {
	m := diverting(t, "FOUNTAIN", 200, 50)
	f := m.fountain
	flying, waiting := f.Airborne()
	total := flying + waiting

	tick(m, 1500)
	fly, wait := f.Airborne()
	if fly+wait != total {
		t.Errorf("the fluid holds %d digits after 1500 frames, having started with %d",
			fly+wait, total)
	}
	for i, p := range f.sim.Pos {
		if math.IsNaN(p.X) || math.IsNaN(p.Y) {
			t.Fatalf("particle %d went to NaN", i)
		}
	}
}

// The jets cannot reach the bins, so the open lids join the two halves. Hold
// toggles, so the fountain holds a lid only when it is not already held. Without
// that guard held flips every frame, yet the lid spring still sits at LidOpen,
// so this asserts the picture and not the mechanism.
func TestTheBoxesAreOpenWhileTheFountainsRun(t *testing.T) {
	m := diverting(t, "FOUNTAIN", 120, 40)
	tick(m, 90)

	for i := 0; i < BinCount; i++ {
		if !m.BinFill.Holding(i) {
			t.Errorf("bin %d is not being held open under its fountain", i)
		}
		if !m.BinFill.Ajar(i) {
			t.Errorf("bin %d is only %.1f degrees open; it still reads as shut",
				i, m.BinFill.Lid(i))
		}
	}

	// It stays open: the hold runs down after lidHold seconds, so a fountain that
	// opened them only once would find them shut again.
	tick(m, 60*int(lidHold)+120)
	for i := 0; i < BinCount; i++ {
		if !m.BinFill.Ajar(i) {
			t.Errorf("bin %d shut after the hold ran out; the fountain is not re-arming it", i)
		}
	}
}

// Holding a lid refines nothing, so raised drawers would claim work nobody did.
// The lids are held and no flights are made because a flight sets Receiving,
// which raises all five drawers when it falls, and drawDrawer paints an opaque
// rectangle across the panel rows the fountains use.
func TestTheDrawersStayDownWhileTheFountainsRun(t *testing.T) {
	m := diverting(t, "FOUNTAIN", 120, 40)

	for n := 0; n < 400; n++ {
		tick(m, 1)
		for i := 0; i < BinCount; i++ {
			if m.Drawer.Showing(i) {
				t.Fatalf("frame %d: bin %d's drawer came up under a fountain, so the "+
					"lids are being opened by something that claims work was done", n, i)
			}
			if m.BinFill.Receiving(i) {
				t.Fatalf("frame %d: bin %d is lit as receiving; nothing is flying to it", n, i)
			}
		}
	}
}

// lidHold is twelve seconds, so without this the bins stand open long after the
// digits have gone.
func TestTheBoxesShutWhenTheFountainsStop(t *testing.T) {
	m := diverting(t, "FOUNTAIN", 120, 40)
	tick(m, 90)

	m.Update(key("x"))
	arrive(t, m)
	if m.Diverting() {
		t.Fatal("the fountains did not stop")
	}
	for i := 0; i < BinCount; i++ {
		if m.BinFill.Holding(i) {
			t.Errorf("bin %d is still held open after the fountains went", i)
		}
	}

	// The lids come down, and are not merely unheld.
	tick(m, 240)
	for i := 0; i < BinCount; i++ {
		if m.BinFill.Ajar(i) {
			t.Errorf("bin %d is still %.1f degrees open two seconds later",
				i, m.BinFill.Lid(i))
		}
	}
}

// The jets take turns, run along the row, arch and drop away.
func TestTheFountainsRunAProgramme(t *testing.T) {
	m := diverting(t, "FOUNTAIN", 120, 40)
	f := m.fountain

	// Every routine comes round, in order, and none is skipped.
	seen := []string{f.Routine()}
	for i := 0; i < 60*90 && len(seen) <= len(routines); i++ {
		tick(m, 1)
		if now := f.Routine(); now != seen[len(seen)-1] {
			seen = append(seen, now)
		}
	}
	if len(seen) < len(routines)+1 {
		t.Fatalf("in ninety seconds the show got through %v, want all %d routines "+
			"and back to the first", seen, len(routines))
	}
	for i, name := range seen[:len(routines)] {
		if name != routines[i].name {
			t.Errorf("routine %d was %q, want %q; the programme is out of order",
				i, name, routines[i].name)
		}
	}
	if seen[len(routines)] != routines[0].name {
		t.Errorf("after the last routine it went to %q, want back to %q",
			seen[len(routines)], routines[0].name)
	}
}

// A programme that runs on time but gives every jet the same number would pass
// every other test.
func TestEachRoutineThrowsTheJetsDifferently(t *testing.T) {
	m := diverting(t, "FOUNTAIN", 120, 40)
	f := m.fountain
	if len(f.mouths) < 2 {
		t.Fatalf("%d jets; there is nothing to choreograph", len(f.mouths))
	}

	// A routine is uneven if it ever asks two jets for different strengths, and
	// alive if any jet's own strength changes over it. ARCH is uneven without being
	// alive, and TOGETHER is the reverse.
	for at := range routines {
		f.show, f.showAt = at, 0
		uneven, alive := false, false
		was := f.strength(0)
		for s := 0.0; s < routines[at].secs; s += 1.0 / 60 {
			f.showAt = s
			first := f.strength(0)
			if first != was {
				alive = true
			}
			for i := 1; i < len(f.mouths); i++ {
				if f.strength(i) != first {
					uneven = true
				}
			}
		}
		if !uneven && !alive {
			t.Errorf("%s asks every jet for the same unchanging strength; it is not "+
				"a routine", routines[at].name)
		}
	}

	// Something is off at some point and something is flat out at another, or the
	// show has no dynamics.
	lo, hi := 1.0, 0.0
	for at := range routines {
		f.show = at
		for s := 0.0; s < routines[at].secs; s += 1.0 / 60 {
			f.showAt = s
			for i := range f.mouths {
				v := f.strength(i)
				lo, hi = math.Min(lo, v), math.Max(hi, v)
			}
		}
	}
	if lo >= jetFloor {
		t.Errorf("the weakest any jet ever gets is %.2f, never below the %.2f that "+
			"turns it off; nothing in the show ever stops", lo, jetFloor)
	}
	if hi < 0.98 {
		t.Errorf("the strongest any jet ever gets is %.2f; nothing in the show ever "+
			"goes flat out", hi)
	}
}

// Below jetFloor a digit would barely clear the mouth and be gathered again the
// next frame. The finale's lull needs the pool to go flat.
func TestAJetTheShowHasStoppedThrowsNothing(t *testing.T) {
	m := diverting(t, "FOUNTAIN", 120, 40)
	f := m.fountain

	// Put the show at the finale's lull, where every jet is off.
	for i, r := range routines {
		if r.name == "FINALE" {
			f.show = i
		}
	}
	f.showAt = 0
	for i := range f.mouths {
		if s := f.strength(i); s >= jetFloor {
			t.Fatalf("jet %d is at %.2f during the lull, want under %.2f", i, s, jetFloor)
		}
	}

	// Hold it there and let the air clear. The show is pinned after every tick
	// because Step advances it; this tests a stopped jet, not the timing.
	hold := func() {
		f.showAt = 0.1
		tick(m, 1)
		f.showAt = 0.1
	}
	for n := 0; n < 200; n++ {
		hold()
	}

	// Everything ends up back in a queue: the lull the finale is built on.
	flying, waiting := f.Airborne()
	if flying > 0 {
		t.Errorf("%d of %d digits are still in the air after three seconds with every "+
			"jet off; they are still being thrown", flying, flying+waiting)
	}

	// Nothing rises at all, so the pool is at rest.
	for n := 0; n < 120; n++ {
		hold()
		for _, v := range f.sim.Vel {
			if v.Y < 0 {
				t.Fatalf("frame %d: something is going up at %.3f columns a second "+
					"while every jet is off", n, v.Y)
			}
		}
	}
}

// The jet height is a fraction of the panel and the speed is derived from it;
// a fixed speed throws through the ceiling on a tall panel, where digits pile
// up and never return to the pool. Asserted as a fraction at three sizes, and
// asserted to be tall.
func TestTheJetsScaleWithThePanel(t *testing.T) {
	climbs := make([]float64, 0, 3)
	for _, size := range [][2]int{{120, 40}, {100, 30}, {80, 24}} {
		m := diverting(t, "FOUNTAIN", size[0], size[1])
		f := m.fountain
		if f == nil {
			t.Fatalf("%dx%d: the fountain is not running", size[0], size[1])
		}
		// No drag on the way up, so the climb is v squared over 2g.
		span := f.max.Y - f.min.Y
		climbs = append(climbs, f.speed*f.speed/(2*fountainGravity)/span)
	}

	for i, c := range climbs[1:] {
		if math.Abs(c-climbs[0]) > 0.02 {
			t.Errorf("size %d: a jet climbs %.2f of the panel against %.2f at the "+
				"first size; the speed is written down rather than derived",
				i+1, c, climbs[0])
		}
	}
	if climbs[0] < 0.75 {
		t.Errorf("a jet climbs %.2f of the panel; that is not most of the way up",
			climbs[0])
	}

	// And not through the roof. The routine's lift and the throw's jitter both
	// multiply the base, so the tallest jet has to be measured. A jet that meets the
	// ceiling piles against it, and the pool never gets those digits back.
	m := diverting(t, "FOUNTAIN", 120, 40)
	f := m.fountain
	best, pinned := 0.0, 0
	span := f.max.Y - f.min.Y
	for n := 0; n < 60*30; n++ {
		tick(m, 1)
		for _, p := range f.sim.Pos {
			if h := (f.max.Y - p.Y) / span; h > best {
				best = h
			}
			if p.Y <= f.min.Y+0.5 {
				pinned++
			}
		}
	}
	if best < 0.85 {
		t.Errorf("the tallest jet of thirty seconds reached %.2f of the panel; the "+
			"fountain is not using the room it has", best)
	}
	if pinned > 0 {
		t.Errorf("digits spent %d frames against the ceiling; they pile there and the "+
			"pool never gets them back", pinned)
	}
}
