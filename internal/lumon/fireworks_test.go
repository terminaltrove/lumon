package lumon

import (
	"math"
	"strings"
	"testing"

	"github.com/terminaltrove/lumon/internal/glyph"
	"github.com/terminaltrove/lumon/internal/physics"
)

// mustTake pops a digit from a bin, for the unit tests that build a sky by hand.
func mustTake(t *testing.T, f *Fireworks, b int) rune {
	t.Helper()
	d, ok := f.take(b)
	if !ok {
		t.Fatalf("bin %d had nothing to give", b)
	}
	return d
}

// A digit is queued at a bin, in a rising shell, or in a falling spark, never
// in two and never in none, so Digits always offers the board somewhere to fly
// and the count stays constant. This catches a burst that appends more sparks
// than it drew digits, a release that shortens the simulation without the
// mirrors, and a resize that rebuilds anything.
func TestEveryDigitKeepsItsOwnPlaceInTheFireworks(t *testing.T) {
	m := diverting(t, "FIREWORKS", 120, 40)
	f := m.sky

	flying, waiting := f.Airborne()
	total := flying + waiting
	if total == 0 {
		t.Fatal("no digits at all")
	}

	check := func(when string) {
		t.Helper()
		if len(f.digits) != f.sim.Len() || len(f.from) != f.sim.Len() || len(f.shape) != f.sim.Len() {
			t.Fatalf("%s: %d particles against %d digits, %d bins, %d shapes",
				when, f.sim.Len(), len(f.digits), len(f.from), len(f.shape))
		}
		fly, wait := f.Airborne()
		if fly+wait != total {
			t.Fatalf("%s: %d digits, started with %d; one has been made or lost",
				when, fly+wait, total)
		}
		for _, d := range f.digits {
			if d < '0' || d > '9' {
				t.Fatalf("%s: a particle carries %q", when, d)
			}
		}
	}

	// Long enough to get through the programme twice, so every volley and shape
	// has been fired.
	for i := 0; i < 60*70; i++ {
		tick(m, 1)
		check("running")
	}

	// A resize re-boxes and does not rebuild.
	for _, wh := range [][2]int{{80, 24}, {200, 50}, {60, 18}} {
		m.Resize(wh[0], wh[1])
		tick(m, 40)
		check("after a resize")
	}

	if got := len(f.Digits()); got != total {
		t.Errorf("Digits offered %d places for %d digits", got, total)
	}
}

// A shell coming through a shut box would show the lid and the launch as
// unrelated. The gate is lidClear: Ajar is 1.5 degrees out of 122 and true
// about two frames after the volley asks, so gating on it couples nothing.
func TestABinFiresOnlyWhileItsLidIsOpen(t *testing.T) {
	m := diverting(t, "FIREWORKS", 120, 40)
	f := m.sky

	// How many shells each bin has in the air, so a launch is the count going up.
	shells := func() [BinCount]int {
		var n [BinCount]int
		for i := range f.from {
			if b := f.from[i]; b >= 0 {
				n[b]++
			}
		}
		return n
	}

	was := shells()
	for i := 0; i < 60*45; i++ {
		lids := [BinCount]float64{}
		for b := 0; b < BinCount; b++ {
			lids[b] = m.BinFill.Lid(b)
		}

		tick(m, 1)

		now := shells()
		for b := 0; b < BinCount; b++ {
			if now[b] <= was[b] {
				continue
			}
			if lids[b] < lidClear {
				t.Fatalf("frame %d: bin %d fired with its lid %.1f degrees open, "+
					"under the %.1f it takes to be clear", i, b, lids[b], lidClear)
			}
		}
		was = now
	}
}

// One at a time down the row for ROLL, everything at once for the finale. A
// programme that always opened the same bins would pass every other test here.
func TestTheBinsOpenInSequence(t *testing.T) {
	m := diverting(t, "FIREWORKS", 120, 40)
	f := m.sky

	open := func() string {
		var b strings.Builder
		for i := 0; i < BinCount; i++ {
			if f.wants(i) {
				b.WriteByte('O')
			} else {
				b.WriteByte('.')
			}
		}
		return b.String()
	}

	// Walk one whole ROLL, which is the first volley.
	if f.Volley() != "ROLL" {
		t.Fatalf("the programme opens on %s", f.Volley())
	}
	seen := map[string]bool{}
	for f.Volley() == "ROLL" {
		seen[open()] = true
		if n := strings.Count(open(), "O"); n != 1 {
			t.Fatalf("ROLL has %d bins open at once: %q", n, open())
		}
		tick(m, 1)
	}
	if len(seen) < BinCount {
		t.Errorf("ROLL opened %d different bins over its whole length, want %d: %v",
			len(seen), BinCount, seen)
	}

	// The finale opens them all.
	for i := 0; i < 60*60 && f.Volley() != "FINALE"; i++ {
		tick(m, 1)
	}
	if f.Volley() != "FINALE" {
		t.Fatal("the programme never reached the finale")
	}
	if got := open(); got != strings.Repeat("O", BinCount) {
		t.Errorf("the finale has %q open, want every bin", got)
	}
}

// A window shorter than the lid's rise is a bin that never fires and looks
// like one with nothing queued. The rise is measured off the spring, so
// retuning lidFrequency or lidDamping cannot invalidate the programme unnoticed.
func TestEveryVolleyOpensABinLongEnoughForItsLidToRise(t *testing.T) {
	// How long a lid takes to reach lidClear from shut.
	rise := 0.0
	b := NewBinFill()
	b.SetHold(0, true)
	for i := 0; i < 600 && b.Lid(0) < lidClear; i++ {
		b.Step(1.0/60, nil, nil, nil)
		rise += 1.0 / 60
	}
	if b.Lid(0) < lidClear {
		t.Fatal("a held lid never reaches lidClear at all")
	}

	const dt = 1.0 / 60
	for _, v := range volleys {
		for i := 0; i < BinCount; i++ {
			run, best := 0.0, 0.0
			for s := 0.0; s < v.secs; s += dt {
				if v.open(s, i, BinCount) {
					run += dt
					if run > best {
						best = run
					}
					continue
				}
				run = 0
			}
			if best == 0 {
				t.Errorf("%s never opens bin %d at all", v.name, i)
				continue
			}
			if best < rise {
				t.Errorf("%s holds bin %d open for %.2fs, under the %.2fs its lid "+
					"takes to clear; that bin will silently never fire",
					v.name, i, best, rise)
			}
		}
	}
}

// A shell has no drag, so from a rise of skyRise the speed is sqrt(2gh) and the
// fuse is exactly v/g, the time to the apex. The speed is derived from the
// panel so this holds at any terminal height.
func TestAShellBurstsAtTheTopOfItsClimb(t *testing.T) {
	m := diverting(t, "FIREWORKS", 120, 40)
	f := m.sky

	floor := f.max.Y
	best := 0.0
	for i := 0; i < 60*30; i++ {
		tick(m, 1)
		for j, p := range f.sim.Pos {
			if f.from[j] < 0 {
				continue // a spark, which goes wherever its burst threw it
			}
			if h := floor - p.Y; h > best {
				best = h
			}
		}
	}

	want := (f.max.Y - f.min.Y) * skyRise
	if best < want*0.8 || best > want*1.2 {
		t.Errorf("the highest a shell got was %.1f visual columns above the mouths, "+
			"against the %.1f skyRise asks for", best, want)
	}
}

// Asserted against the rendered buffer, since it is about what is seen. The
// digit holds for most of the fall and only then steps down EmergeRamp, which
// is why the life is bent before it is passed on; raw, the snap comes at about
// 0.63 of a life and the sky is mostly punctuation.
func TestASparkGoesOutRatherThanVanishing(t *testing.T) {
	m := diverting(t, "FIREWORKS", 120, 40)
	panel := m.gridPanel().Play()

	digits, dying := 0, 0
	for i := 0; i < 60*30; i++ {
		tick(m, 1)
		buf := drawTo(m)
		for y := panel.Y; y < panel.Y+panel.H; y++ {
			for _, r := range []rune(buf.Row(y))[panel.X : panel.X+panel.W] {
				switch {
				case r >= '0' && r <= '9':
					digits++
				case strings.ContainsRune(glyph.EmergeRamp, r):
					dying++
				}
			}
		}
	}

	if dying == 0 {
		t.Error("nothing was ever drawn going out; sparks vanish between frames")
	}
	// The sky is numbers: punctuation is the end of a spark's life and should
	// never be most of the screen.
	if digits < dying {
		t.Errorf("%d digit cells against %d shading cells over thirty seconds; the "+
			"file is coming out as punctuation", digits, dying)
	}
}

// Holding a lid refines nothing, so raised drawers would claim work nobody did.
// The lids are held and no flights are made because a flight sets Receiving,
// which raises all five drawers when it falls, and drawDrawer paints an opaque
// rectangle across the sky's panel.
func TestTheDrawersStayDownWhileTheFireworksRun(t *testing.T) {
	m := diverting(t, "FIREWORKS", 120, 40)

	for n := 0; n < 60*20; n++ {
		tick(m, 1)
		for i := 0; i < BinCount; i++ {
			if m.Drawer.Showing(i) {
				t.Fatalf("frame %d: bin %d's drawer came up under the fireworks", n, i)
			}
			if m.BinFill.Receiving(i) {
				t.Fatalf("frame %d: bin %d is lit as receiving; nothing is flying to it", n, i)
			}
		}
	}
}

// A hold lasts lidHold whether or not anything still asks, so without this the
// boxes stand open twelve seconds after leaving.
func TestTheBoxesShutWhenTheFireworksStop(t *testing.T) {
	m := diverting(t, "FIREWORKS", 120, 40)
	tick(m, 120)

	m.Update(key("x"))
	arrive(t, m)
	if m.Diverting() {
		t.Fatal("the fireworks did not stop")
	}
	for i := 0; i < BinCount; i++ {
		if m.BinFill.Holding(i) {
			t.Errorf("bin %d is still held open after the sky went", i)
		}
	}
	tick(m, 240)
	for i := 0; i < BinCount; i++ {
		if m.BinFill.Ajar(i) {
			t.Errorf("bin %d is still %.1f degrees open two seconds later",
				i, m.BinFill.Lid(i))
		}
	}
}

func TestTheFireworksRunAProgramme(t *testing.T) {
	m := diverting(t, "FIREWORKS", 120, 40)
	f := m.sky

	seen := []string{f.Volley()}
	for i := 0; i < 60*90 && len(seen) <= len(volleys); i++ {
		tick(m, 1)
		if now := f.Volley(); now != seen[len(seen)-1] {
			seen = append(seen, now)
		}
	}
	if len(seen) < len(volleys)+1 {
		t.Fatalf("in ninety seconds it got through %v, want all %d and back to the first",
			seen, len(volleys))
	}
	for i, name := range seen[:len(volleys)] {
		if name != volleys[i].name {
			t.Errorf("volley %d was %q, want %q; the programme is out of order",
				i, name, volleys[i].name)
		}
	}
	if seen[len(volleys)] != volleys[0].name {
		t.Errorf("after the last it went to %q, want back to %q",
			seen[len(volleys)], volleys[0].name)
	}
}

func TestTheSkysKeysAnswerAndStayHidden(t *testing.T) {
	m := diverting(t, "FIREWORKS", 120, 40)

	// b walks the four shapes and then returns to the programme.
	if forced, _ := m.sky.Effects(); forced {
		t.Fatal("a shape was already forced")
	}
	first := m.sky.Shape()
	m.Update(key("b"))
	if forced, _ := m.sky.Effects(); !forced {
		t.Error("b did not take the shape")
	}
	if got := m.sky.Shape(); got != first {
		t.Errorf("the first press changed the shape from %q to %q; it should take "+
			"what was already firing", first, got)
	}
	for i := 0; i < int(burstKinds); i++ {
		m.Update(key("b"))
	}
	if forced, _ := m.sky.Effects(); forced {
		t.Error("b never came round past the last shape and gave the programme back")
	}

	// t is the trails.
	if _, trails := m.sky.Effects(); trails {
		t.Fatal("trails were already on")
	}
	m.Update(key("t"))
	if _, trails := m.sky.Effects(); !trails {
		t.Error("t did not turn the trails on")
	}
	m.Update(key("t"))
	if _, trails := m.sky.Effects(); trails {
		t.Error("t did not turn them off again")
	}

	// None of the three left the diversion.
	if m.sky == nil {
		t.Fatal("a key ended the fireworks")
	}

	// The hint lists all three, since nothing else lists them.
	hint := m.divertHint()
	for _, k := range []string{"f ", "b ", "t "} {
		if !strings.Contains(hint, k) {
			t.Errorf("the hint does not mention %q: %q", k, hint)
		}
	}
	for _, g := range Bindings() {
		for _, b := range g.Bindings {
			if strings.Contains(strings.ToUpper(b.Keys+b.What+b.Short), "FIREWORK") {
				t.Errorf("the controls mention the fireworks: %q", b.What)
			}
		}
	}
}

func TestPressingFFiresWithoutWaiting(t *testing.T) {
	m := diverting(t, "FIREWORKS", 120, 40)
	f := m.sky

	// Let a shell go up on its own, so every bin is inside its shellGap.
	tick(m, 30)
	shells := func() int {
		n := 0
		for _, b := range f.from {
			if b >= 0 {
				n++
			}
		}
		return n
	}

	was := shells()
	m.Update(key("f"))
	tick(m, 2)
	if shells() <= was {
		t.Errorf("f added nothing to the sky: %d shells before and %d after", was, shells())
	}
}

// physics.Sparks swaps its last element into the hole, and the three arrays
// alongside must make the same move. Getting it wrong leaves every count right
// and every digit a digit, but a particle then has somebody else's digit, so
// release is tested directly.
func TestReleasingAParticleKeepsTheMirrorsInStep(t *testing.T) {
	m := diverting(t, "FIREWORKS", 120, 40)
	f := m.sky

	for f.sim.Len() > 0 {
		f.queued[0] = append(f.queued[0], f.digits[0])
		f.release(0)
	}

	// Four particles, each identifiable by its position, its bin and its shape.
	for k := 0; k < 4; k++ {
		f.add(physics.Vec2{X: float64(k)}, physics.Vec2{}, float64(k+1), 1,
			int8(k), burstKind(k%int(burstKinds)), mustTake(t, f, 0))
	}
	want := f.digits[3]

	f.release(1)

	if f.sim.Len() != 3 || len(f.digits) != 3 || len(f.from) != 3 || len(f.shape) != 3 {
		t.Fatalf("the arrays came apart: %d particles, %d digits, %d bins, %d shapes",
			f.sim.Len(), len(f.digits), len(f.from), len(f.shape))
	}
	if got := f.sim.Pos[1].X; got != 3 {
		t.Fatalf("slot 1 holds the particle from %v; the last should have been swapped in", got)
	}
	if f.digits[1] != want {
		t.Errorf("slot 1 carries %q, but the particle swapped into it was carrying %q",
			f.digits[1], want)
	}
	if f.from[1] != 3 {
		t.Errorf("slot 1 says it came from bin %d, want 3", f.from[1])
	}
	if f.shape[1] != burstKind(3%int(burstKinds)) {
		t.Errorf("slot 1 says shape %d, want %d", f.shape[1], 3%int(burstKinds))
	}
}

// Remove swaps the tail into the hole, so the walk goes backwards. Forwards, a
// spent spark next to another spent spark survives the pass: a one-frame delay
// invisible on a running display, so it is tested directly.
func TestTheCullTakesOutEverythingSpentInOnePass(t *testing.T) {
	m := diverting(t, "FIREWORKS", 120, 40)
	f := m.sky

	for f.sim.Len() > 0 {
		f.queued[0] = append(f.queued[0], f.digits[0])
		f.release(0)
	}

	// Eight spent sparks, what a burst leaves behind all at once.
	mid := physics.Vec2{X: (f.min.X + f.max.X) / 2, Y: (f.min.Y + f.max.Y) / 2}
	for i := 0; i < 8; i++ {
		f.add(mid, physics.Vec2{}, 0, 1, -1, peony, mustTake(t, f, 0))
	}

	f.settle()

	if got := f.sim.Len(); got != 0 {
		t.Errorf("%d of eight spent sparks survived the pass", got)
	}
}

// The cull walk appends to the arrays it walks. Backwards, anything appended
// lands past the end and is not visited that frame; forwards, a burst's sparks
// would be a frame old as they appear. Tested on settle directly with one spent
// shell: on a running display a shell legitimately moves on its first frame,
// and swap-removal means an index says nothing about age.
func TestASparkBornInABurstIsNotAgedByTheWalkThatMadeIt(t *testing.T) {
	m := diverting(t, "FIREWORKS", 120, 40)
	f := m.sky

	// Empty the sky, putting every digit back where it came from so the total
	// count is not disturbed.
	for f.sim.Len() > 0 {
		f.queued[0] = append(f.queued[0], f.digits[0])
		f.release(0)
	}

	fly, wait := f.Airborne()
	total := fly + wait

	// One shell in the middle with its fuse spent.
	mid := physics.Vec2{X: (f.min.X + f.max.X) / 2, Y: (f.min.Y + f.max.Y) / 2}
	f.add(mid, physics.Vec2{}, 0, 1, 0, peony, mustTake(t, f, 0))

	f.settle()

	if f.sim.Len() == 0 {
		t.Fatal("the shell did not burst")
	}
	for i := 0; i < f.sim.Len(); i++ {
		if f.from[i] >= 0 {
			t.Fatalf("particle %d is still a shell after the burst", i)
		}
		if got := f.sim.Fade(i); got != 1 {
			t.Errorf("spark %d was born %.3f through its life; the walk that made it "+
				"aged it", i, got)
		}
	}

	// The burst made exactly the shape's count of sparks, out of the queue.
	if got := f.sim.Len(); got != burstShapes[peony].count {
		t.Errorf("a peony came out as %d sparks, want %d", got, burstShapes[peony].count)
	}

	// The shell's own digit went home before the burst drew from the queue, so the
	// total is untouched by a burst.
	if fly, wait = f.Airborne(); fly+wait != total {
		t.Errorf("%d digits after the burst, against %d before it", fly+wait, total)
	}
}

// runTo drives the display to a named volley and a little way into it.
func runTo(t *testing.T, m *Model, name string, secs float64) {
	t.Helper()
	for i := 0; i < 60*120; i++ {
		if m.sky.Volley() == name && m.sky.atSecs >= secs {
			return
		}
		tick(m, 1)
	}
	t.Fatalf("the programme never reached %s", name)
}

// The mouths are on the panel's bottom row and gravity is +Y, so anything
// thrown flat leaves the box on the first frame and bursts at its own mouth. A
// comet goes up as well as across: the sideways speed is the width over the
// crossing time and the upward speed is gT/2, which brings it back to its
// starting height as it arrives.
func TestACometCrossesThePanelBeforeItBursts(t *testing.T) {
	m := diverting(t, "FIREWORKS", 120, 40)
	f := m.sky
	panel := m.gridPanel().Play()
	runTo(t, m, "ALTERNATE", 0.5)

	// Follow whichever comets are up and remember how far each got sideways.
	best := 0.0
	from := map[int]float64{}
	for i := 0; i < 60*5; i++ {
		for j := range f.sim.Pos {
			if f.from[j] < 0 {
				continue
			}
			key := int(f.sim.Pos[j].Y*1000) + int(f.from[j])
			if _, seen := from[key]; !seen {
				from[key] = f.sim.Pos[j].X
			}
			if d := math.Abs(f.sim.Pos[j].X - f.mouths[f.from[j]].X); d > best {
				best = d
			}
			// It never leaves the top, which would burst it early.
			if f.sim.Pos[j].Y < f.min.Y {
				t.Fatalf("a comet reached %.1f, above the panel's ceiling %.1f",
					f.sim.Pos[j].Y, f.min.Y)
			}
		}
		tick(m, 1)
	}

	if want := float64(panel.W) * 0.4; best < want {
		t.Errorf("the furthest a comet got from its own bin was %.0f columns, want at "+
			"least %.0f of a panel %d wide", best, want, panel.W)
	}
}

// Thrown at the wall, it leaves the panel at once and bursts where it started.
func TestACometIsThrownAwayFromTheWallItStartsAt(t *testing.T) {
	m := diverting(t, "FIREWORKS", 120, 40)
	f := m.sky

	for b := 0; b < BinCount; b++ {
		vel, fuse := f.cometThrow(b)
		if fuse <= 0 {
			t.Errorf("bin %d's comet has a fuse of %v", b, fuse)
		}
		if vel.Y >= 0 {
			t.Errorf("bin %d's comet is thrown downward (%v); it would leave the box "+
				"on the first frame", b, vel)
		}
		middle := (f.min.X + f.max.X) / 2
		if f.mouths[b].X < middle && vel.X <= 0 {
			t.Errorf("bin %d is left of the middle and its comet goes left", b)
		}
		if f.mouths[b].X > middle && vel.X >= 0 {
			t.Errorf("bin %d is right of the middle and its comet goes right", b)
		}
	}
}

// So `from` means one thing: "this will burst when its fuse runs out", and a
// mine has already burst.
func TestAMineGoesOffAtItsOwnMouthWithNoShell(t *testing.T) {
	m := diverting(t, "FIREWORKS", 120, 40)
	f := m.sky
	runTo(t, m, "OUTWARD", 0.1)

	shells, up, total := 0, 0, 0
	for i := 0; i < 60*5; i++ {
		before := f.sim.Len()
		tick(m, 1)
		for j := before; j < f.sim.Len(); j++ {
			total++
			if f.from[j] >= 0 {
				shells++
			}
			if f.sim.Vel[j].Y < 0 {
				up++
			}
		}
		if f.Volley() != "OUTWARD" {
			break
		}
	}

	if total == 0 {
		t.Fatal("the mines made nothing at all")
	}
	if shells > 0 {
		t.Errorf("%d of %d things a mine made were shells; a mine has already burst",
			shells, total)
	}
	// A curtain, not a ring on the floor: the cone is thrown upward.
	if up < total*3/4 {
		t.Errorf("only %d of %d of a mine's sparks were thrown upward", up, total)
	}
}

// A fan emits without launching, so it can reach take with an empty queue,
// which fire and burst otherwise guard. take must not invent a digit either,
// since that breaks the count, so an empty bin adds nothing.
func TestABinNeverGivesOutADigitItHasNot(t *testing.T) {
	m := diverting(t, "FIREWORKS", 120, 40)
	f := m.sky

	if _, ok := f.take(0); !ok {
		t.Fatal("a full bin refused to give a digit")
	}
	if _, ok := f.take(-1); ok {
		t.Error("a bin that does not exist gave a digit")
	}

	// Drain one and keep firing it.
	fly, wait := f.Airborne()
	total := fly + wait
	for {
		d, ok := f.take(0)
		if !ok {
			break
		}
		f.queued[1] = append(f.queued[1], d)
	}
	if _, ok := f.take(0); ok {
		t.Fatal("an empty bin gave a digit")
	}

	for i := 0; i < 60*40; i++ {
		tick(m, 1)
		fly, wait = f.Airborne()
		if fly+wait != total {
			t.Fatalf("frame %d: %d digits against %d at the start; a drained bin has "+
				"invented or lost one", i, fly+wait, total)
		}
	}
}

func TestEachFigureLeavesTheBinItsOwnWay(t *testing.T) {
	seen := map[figure]bool{}
	for _, v := range volleys {
		seen[v.how] = true
	}
	for _, want := range []figure{shellUp, comet, mine, fan} {
		if !seen[want] {
			t.Errorf("no volley fires figure %d; it is machinery nothing reaches", want)
		}
	}

	// A fan is a stream, more than the two a second shellGap would allow.
	m := diverting(t, "FIREWORKS", 120, 40)
	f := m.sky
	runTo(t, m, "SWEEP", 1.0)

	made, frames := 0, 0
	for i := 0; i < 60*4 && f.Volley() == "SWEEP"; i++ {
		before := f.sim.Len()
		tick(m, 1)
		if f.sim.Len() > before {
			made += f.sim.Len() - before
		}
		frames++
	}
	if rate := float64(made) / (float64(frames) / 60); rate < 6 {
		t.Errorf("the fan emitted %.1f sparks a second; that is a trickle rather than "+
			"a spray, and two a second is what shellGap alone would give", rate)
	}
}

// Step decides the lid from what the bin can fire, keeping the timing gates,
// since a bin inside its gap fires again in a moment and shutting the lid for
// that would make it flap. Run at 80x24 as well: the viewport is 12 by 5 there,
// so twelve digits a bin, fewer than a shape's own count of 16 to 20.
// A launch is read off f.since, which fire and spray both zero. An episode is
// one unbroken run of the bin's own lid standing clear, read off BinFill, since
// asking f.wants and f.ready would check the fix with itself.
func TestNoBoxOpensOnAnEmptyBin(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {80, 24}} {
		m := diverting(t, "FIREWORKS", size[0], size[1])
		f := m.sky
		if f == nil {
			t.Fatalf("%dx%d: the fireworks are not running", size[0], size[1])
		}

		// The programme twice over, so every volley is seen from a bin that has
		// already been drawn on.
		var total float64
		for _, v := range volleys {
			total += v.secs
		}
		frames := int(total*2*60) + 1

		open := [BinCount]bool{}
		fired := [BinCount]bool{}
		began := [BinCount]string{}
		since := [BinCount]float64{}
		episodes := 0

		for n := 0; n < frames; n++ {
			was := f.since
			name := f.Volley()
			tick(m, 1)

			for b := 0; b < BinCount; b++ {
				if f.since[b] < was[b] {
					fired[b] = true
				}
				held := m.BinFill.Lid(b) >= lidClear
				switch {
				case held && !open[b]:
					open[b], fired[b], began[b] = true, false, name
					since[b] = 0
				case held:
					since[b] += 1.0 / 60
				case open[b]:
					// The lid has closed again. It had its chance.
					if !fired[b] && since[b] > shellGap*2 {
						t.Errorf("%dx%d: bin %d held its lid open for %.1fs during %s "+
							"and fired nothing", size[0], size[1], b, since[b], began[b])
					}
					if since[b] > shellGap*2 {
						episodes++
					}
					open[b] = false
				}
			}
		}

		// The display is running, or the loop above proves nothing by never opening a
		// lid.
		if episodes < len(volleys) {
			t.Errorf("%dx%d: only %d bins ever held a lid open long enough to fire; "+
				"the programme is not running", size[0], size[1], episodes)
		}
	}
}

// burst clamps to what the queue holds, so a shell does not need the shape's
// whole count to launch. A thin burst is still a firework.
func TestABinFiresWhateverItHasRatherThanNothing(t *testing.T) {
	m := diverting(t, "FIREWORKS", 120, 40)
	f := m.sky

	// Down to the fewest that fire, under every shape's own count.
	b := 2
	for len(f.queued[b]) > burstLeast+1 {
		mustTake(t, f, b)
	}
	if len(f.queued[b]) >= burstShapes[peony].count {
		t.Fatalf("bin %d still holds %d digits, which is not the thin case",
			b, len(f.queued[b]))
	}
	if !f.ready(b) {
		t.Fatalf("bin %d holding %d is not ready to fire; the gate is still the "+
			"shape's whole count", b, len(f.queued[b]))
	}

	// Fire it, run the shell up, and see the burst.
	before := f.sim.Len()
	f.now = true
	f.fire(b)
	if f.sim.Len() == before {
		t.Fatal("a bin holding a burst's worth of digits launched nothing")
	}
	for n := 0; n < 240 && f.sim.Len() > 0; n++ {
		f.Step(1.0/60, nil)
	}

	// One digit short of the minimum it holds fire, or the gate does nothing.
	for len(f.queued[b]) >= burstLeast+1 {
		mustTake(t, f, b)
	}
	if f.ready(b) {
		t.Errorf("bin %d holding %d is still ready; there is no floor at all",
			b, len(f.queued[b]))
	}
}

// Three sizes, asserting the fraction of the panel each figure reaches. A
// fixed height passes at the size it was chosen on and fails at the others.
func TestTheShellsAndTheCometsScaleWithThePanel(t *testing.T) {
	type reading struct {
		shell, comet, spark float64
	}
	got := make([]reading, 0, 3)

	for _, size := range [][2]int{{120, 40}, {100, 30}, {80, 24}} {
		m := diverting(t, "FIREWORKS", size[0], size[1])
		f := m.sky
		span := f.max.Y - f.min.Y

		// A shell's climb: no drag, so v squared over 2g.
		shell := f.speed * f.speed / (2 * skyGravity)

		// A comet's apex is g T squared over eight, and cometThrow returns T.
		_, cross := f.cometThrow(0)
		comet := skyGravity * cross * cross / 8

		got = append(got, reading{
			shell: shell / span,
			comet: comet / span,
			spark: burstShapes[peony].speed * f.spark / span,
		})
	}

	for i, r := range got[1:] {
		first := got[0]
		if math.Abs(r.shell-first.shell) > 0.02 {
			t.Errorf("size %d: a shell climbs %.2f of the panel against %.2f at the "+
				"first size", i+1, r.shell, first.shell)
		}
		if math.Abs(r.comet-first.comet) > 0.02 {
			t.Errorf("size %d: a comet reaches %.2f of the panel against %.2f at the "+
				"first size", i+1, r.comet, first.comet)
		}
		if math.Abs(r.spark-first.spark) > 0.02 {
			t.Errorf("size %d: a spark leaves at %.2f of the panel a second against "+
				"%.2f at the first size", i+1, r.spark, first.spark)
		}
	}

	// They are tall: a shell climbing only a third of its sky fails.
	if got[0].shell < 0.75 {
		t.Errorf("a shell climbs %.2f of the panel; that is not most of the way up",
			got[0].shell)
	}
	if got[0].comet < 0.35 {
		t.Errorf("a comet reaches %.2f of the panel; it is skimming the floor",
			got[0].comet)
	}
}
