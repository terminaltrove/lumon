package lumon

import (
	"math"
	"testing"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// wakeAndGrow gets a cluster swollen, so there is something large on screen.
func wakeAndGrow(t *testing.T, m *Model) bool {
	t.Helper()
	for i := 0; i < 60*25 && m.FirstActive() == nil; i++ {
		tick(m, 1)
	}
	g := m.FirstActive()
	if g == nil {
		return false
	}
	id := g.NumberIDs[0]
	m.CursorX, m.CursorY = id%GridSize, id/GridSize

	// Wait on the size, not a frame count: the digit grows on the cluster's own
	// swell, which takes most of a minute.
	for i := 0; i < 60*40; i++ {
		tick(m, 1)
		if m.Magnify.Height(m.CursorX, m.CursorY) >= MinGlyphHeight+glyphBand {
			break
		}
	}

	// planGlyphs runs inside Draw, so m.boxes, which a transition snapshots, is
	// filled only once a frame has been rendered. Ticking alone does not draw.
	drawTo(m)
	return m.Magnify.Height(m.CursorX, m.CursorY) >= MinGlyphHeight
}

// Every one of the six travels both ways.
func TestEnteringAndLeavingAreJourneys(t *testing.T) {
	for i, d := range diversions {
		t.Run(d.Name, func(t *testing.T) {
			if d.Mode {
				// A mode is entered, not started, and once the digits have landed a stray key
				// plays the game. Its journey is covered in grid3d_test.go.
				t.Skip("not a diversion")
			}
			if d.Refuses {
				// Neither started nor entered: nothing happens, so there is nothing to travel
				// to. See escape.go.
				t.Skip("not a diversion; it refuses")
			}
			m := ready()
			m.Resize(110, 30)
			typeWord(m, theWord)
			m.divertAt = i

			m.Update(key("enter"))
			if m.transit == nil {
				t.Fatalf("entering %s was instant", d.Name)
			}
			if m.transit.Back() {
				t.Error("entering set off in the wrong direction")
			}
			// It takes a while, and it is not stepping the diversion yet.
			frames := 0
			for ; frames < 60*4 && m.transit != nil; frames++ {
				tick(m, 1)
			}
			if frames < 8 {
				t.Errorf("entering %s took %d frames; that is a cut", d.Name, frames)
			}
			arrive(t, m)
			tick(m, 60)

			m.Update(key("x"))
			if !m.transitioning() {
				t.Fatalf("leaving %s was instant", d.Name)
			}

			// MATRIX leaves in two beats: the wave falls first, and only then do the digits
			// go home. It moves none of them, so the flight alone would be over in a frame.
			frames = 0
			for ; frames < 60*4 && m.transit == nil && m.transitioning(); frames++ {
				tick(m, 1)
			}
			if m.transit == nil {
				t.Fatalf("leaving %s never set off", d.Name)
			}
			if !m.transit.Back() {
				t.Error("leaving set off in the wrong direction")
			}
			// The simulation is dropped as the return begins, so nothing invisible is
			// being stepped.
			if m.sandbox != nil || m.matrix != nil || m.sand != nil ||
				m.primes != nil || m.praise != nil {
				t.Error("the simulation is still running behind the return trip")
			}

			for ; frames < 60*4 && m.transit != nil; frames++ {
				tick(m, 1)
			}
			if frames < 8 {
				t.Errorf("leaving %s took %d frames; that is a cut", d.Name, frames)
			}
			if m.Diverting() {
				t.Error("the trip finished and the grid did not come back")
			}
		})
	}
}

// Otherwise it pops between sizes.
func TestASwollenClusterShrinksOnTheWayOutAndGrowsBack(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	if !wakeAndGrow(t, m) {
		t.Skip("nothing grew")
	}

	typeWord(m, theWord)
	m.divertAt = 0
	m.Update(key("enter"))
	if m.transit == nil {
		t.Fatal("entering was instant")
	}

	// The tallest digit in the air, sampled as it goes.
	tallest := func() float64 {
		h := 0.0
		for _, e := range m.transit.entries {
			if v := e.height.Value(); v > h {
				h = v
			}
		}
		return h
	}

	start := tallest()
	if start < MinGlyphHeight {
		t.Fatalf("nothing set off large; the tallest was %v", start)
	}

	var mid float64
	for i := 0; i < 60*4 && m.transit != nil; i++ {
		tick(m, 1)
		if m.transit != nil && i == 5 {
			mid = tallest()
		}
	}
	if mid >= start {
		t.Errorf("the cluster was %v and then %v; it should be shrinking", start, mid)
	}

	// On the way home it grows back to whatever the board has it at now.
	tick(m, 30)
	m.Update(key("x"))
	if m.transit == nil {
		t.Fatal("leaving was instant")
	}
	low := tallest()
	for i := 0; i < 60*4 && m.transit != nil; i++ {
		tick(m, 1)
		if m.transit != nil && i == 20 {
			if got := tallest(); got <= low {
				t.Errorf("the cluster came home at %v having left at %v; it should be growing",
					got, low)
			}
			break
		}
	}
}

func TestAKeyMidJourneyTurnsTheTripAround(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	typeWord(m, theWord)
	m.divertAt = 1 // DESCENT
	m.Update(key("enter"))
	tick(m, 6)

	if m.transit == nil || m.transit.Back() {
		t.Fatal("not on an outbound trip")
	}
	m.Update(key("x"))

	if m.transit == nil {
		t.Fatal("the trip was abandoned rather than reversed")
	}
	if !m.transit.Back() {
		t.Error("a key mid-journey did not turn the trip around")
	}
	if m.diverting != "" {
		t.Error("the diversion is still named after being turned back")
	}
	if m.sandbox != nil {
		t.Error("the simulation survived being turned back")
	}

	arrive(t, m)
	if m.Diverting() {
		t.Error("the reversed trip did not land back at the grid")
	}
}

// Otherwise the grid is stranded behind a transition that never finishes.
func TestATripAlwaysFinishes(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	typeWord(m, theWord)
	m.divertAt = 0
	m.Update(key("enter"))
	if m.transit == nil {
		t.Fatal("no trip")
	}

	// Aim every digit somewhere it can never reach, so only the time limit can
	// end this.
	unreachable := func(tr *Transit) {
		for i := range tr.entries {
			tr.entries[i].target.X = 1e9
		}
	}
	unreachable(m.transit)
	for i := 0; i < 60*5 && m.transit != nil; i++ {
		tick(m, 1)
	}
	if m.transit != nil {
		t.Fatal("an outbound trip that could not arrive never gave up")
	}
	// Outbound: the diversion is now running, so the grid has been handed over.
	if !m.Diverting() {
		t.Error("the outbound trip landed on nothing")
	}

	// The return has to give up too, or that is what strands the grid.
	m.Update(key("x"))
	if m.transit == nil {
		t.Fatal("leaving was instant")
	}
	unreachable(m.transit)
	for i := 0; i < 60*5 && m.transit != nil; i++ {
		tick(m, 1)
	}
	if m.transit != nil {
		t.Fatal("a return trip that could not arrive never gave up")
	}
	if m.Diverting() {
		t.Error("the grid is stranded behind a finished return trip")
	}
}

// The chrome runs while the digits are in the air, as it does during a
// diversion.
func TestTheChromeKeepsMovingDuringATrip(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	m.Room.mu.Lock()
	m.Grid.Bins[0].Refined = m.Grid.Bins[0].Quota / 2
	m.Room.mu.Unlock()

	typeWord(m, theWord)
	m.divertAt = 3
	m.Update(key("enter"))
	if m.transit == nil {
		t.Fatal("no trip")
	}

	before := m.File.Fill()
	for i := 0; i < 20 && m.transit != nil; i++ {
		tick(m, 1)
	}
	if m.File.Fill() <= before {
		t.Errorf("the file bar sat at %v while the digits were in the air", m.File.Fill())
	}
}

// Its endpoints were worked out against a screen that no longer exists.
func TestAResizeAbandonsATripRatherThanFlingingDigitsAtNothing(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	typeWord(m, theWord)
	m.divertAt = 0
	m.Update(key("enter"))
	tick(m, 4)
	if m.transit == nil {
		t.Fatal("no trip")
	}

	m.Resize(70, 20)
	if m.transit != nil {
		t.Error("a trip survived a resize; its endpoints are off the old screen")
	}
	tick(m, 60)
	_ = drawTo(m)
}

// Drawing the grid as well would show every number twice.
func TestTheGridStandsDownWhileTheDigitsAreInTheAir(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	typeWord(m, theWord)
	m.divertAt = 5 // PRAISE, which shares no positions with the grid
	m.Update(key("enter"))
	tick(m, 10)
	if m.transit == nil {
		t.Fatal("no trip")
	}
	if !m.Diverting() {
		t.Fatal("a trip does not stand the grid down")
	}

	// Every digit on screen inside the panel should be one of the travellers.
	panel := m.gridPanel()
	buf := drawTo(m)
	inAir := map[[2]int]bool{}
	for _, e := range m.transit.entries {
		p := m.transit.world.Pos(e.particle)
		inAir[[2]int{int(math.Round(p.X)), int(math.Round(p.Y))}] = true
	}

	stray := 0
	for y := panel.Y; y < panel.Y+panel.H-1; y++ {
		for x := 0; x < m.Layout.Width; x++ {
			r := buf.RuneAt(x, y)
			if r < '0' || r > '9' {
				continue
			}
			if !inAir[[2]int{x, y}] {
				stray++
			}
		}
	}
	// Large digits are rasterised across several cells, so a few cells have ink
	// away from a traveller's centre. The grid must not be drawn underneath.
	if stray > m.transit.Len()/2 {
		t.Errorf("%d digits on screen belong to neither the trip nor a glyph; "+
			"the grid is being drawn under it", stray)
	}
}

// gridThenTransition returns the grid as it stands and the first frame of the
// trip out of it, with the menu modal suppressed.
func gridThenTransition(t *testing.T, m *Model, which int) (before, after *canvas.Buffer) {
	t.Helper()
	typeWord(m, theWord)
	m.divertAt = which

	m.divertOpen = false
	before = drawTo(m)
	m.divertOpen = true

	m.Update(key("enter"))
	if m.transit == nil {
		t.Fatal("entering was instant")
	}
	return before, drawTo(m)
}

// The transition snapshots what was drawn. Re-deriving the frame from Magnify
// and an is-this-corrupt colour disagrees with the renderer: culled members
// appear as glyphs, dormant clusters light up, escalated ones go flat and
// rocking ones snap upright.
func TestATransitionBeginsAsTheFrameOnScreen(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	if !wakeAndGrow(t, m) {
		t.Skip("nothing grew")
	}
	tick(m, 60*6) // let it redden

	before, after := gridThenTransition(t, m, 0)

	panel := m.gridPanel()
	total, diff := 0, 0
	for y := panel.Y; y < panel.Y+panel.H-1; y++ {
		for x := 0; x < m.Layout.Width; x++ {
			total++
			if before.RuneAt(x, y) != after.RuneAt(x, y) ||
				before.AttrAt(x, y) != after.AttrAt(x, y) {
				diff++
			}
		}
	}

	// A few cells differ, since a rasterised glyph's coverage rounds differently
	// as a particle than as a box. Hundreds must not.
	if diff > total/100 {
		t.Errorf("%d of %d cells changed on the first frame of the transition; "+
			"it is starting from somewhere other than the screen", diff, total)
	}
}

// A dormant cluster is drawn a shade off ordinary so it hides in plain sight;
// colouring by "is this corrupt" would light every one of them up.
func TestADormantClusterDoesNotLightUpWhenATransitionStarts(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	tick(m, 60)

	// A corrupt cell whose cluster is asleep.
	l := m.Layout
	var gx, gy int
	found := false
	for y := l.ViewportY; y < l.ViewportY+l.ViewportH && !found; y++ {
		for x := l.ViewportX; x < l.ViewportX+l.ViewportW; x++ {
			n := m.Grid.At(x, y)
			if n != nil && n.BadGroupID >= 0 && !m.IsActive(n.BadGroupID) {
				gx, gy, found = x, y, true
				break
			}
		}
	}
	if !found {
		t.Skip("no dormant corrupt cell on screen")
	}

	sx, sy := m.PlainAnchor(GridIndex(gx, gy))
	_, after := gridThenTransition(t, m, 0)

	if got := after.AttrAt(sx, sy); got != m.Theme.Bad {
		t.Errorf("a dormant corrupt digit is drawn %v on the first transition frame, want the "+
			"dim %v it was hiding in", got, m.Theme.Bad)
	}
}

// The transition has to leave with the escalation red, not a flat colour.
func TestAnEscalatedClusterKeepsItsRedIntoTheTransition(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	if !wakeAndGrow(t, m) {
		t.Skip("nothing grew")
	}
	// Step off the cluster: wakeAndGrow parks the cursor on it, and the digit under
	// the cursor is white, so there would be nothing reddened to look at.
	for i := 0; i < 6; i++ {
		m.Update(key("down"))
	}
	tick(m, 60*8) // well up the ramp
	drawTo(m)

	if len(m.boxes) == 0 {
		t.Skip("nothing is drawn large")
	}
	// A glyph of the woken cluster that is not under the cursor (white). The lens
	// enlarges ordinary cells too, drawn plain-swollen, so any box will not do.
	var box glyphBox
	for _, b := range m.boxes {
		n := &m.Grid.Cells[b.id]
		if b.id != GridIndex(m.CursorX, m.CursorY) && m.IsActive(n.BadGroupID) {
			box = b
			break
		}
	}
	if box.cov == nil {
		t.Skip("no cluster glyph away from the cursor")
	}
	want := m.glyphAttr(box.id)
	if want.FG == m.Theme.Swollen.FG {
		t.Skipf("the cluster has not reddened; it is %v", want.FG)
	}

	_, after := gridThenTransition(t, m, 0)

	// Somewhere in that glyph's box the ink should still be its own colour.
	kept := false
	for y := box.y; y < box.y+box.h && !kept; y++ {
		for x := box.x; x < box.x+box.w; x++ {
			if after.AttrAt(x, y).FG == want.FG {
				kept = true
				break
			}
		}
	}
	if !kept {
		t.Errorf("an escalated cluster lost its %v on the first transition frame", want.FG)
	}
}

// planGlyphs culls overlapping boxes, and re-deriving from Magnify would put the
// culled members back.
func TestTheTransitionDrawsTheSameDigitsLarge(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	if !wakeAndGrow(t, m) {
		t.Skip("nothing grew")
	}
	tick(m, 60*3)
	drawTo(m)

	wantBig := len(m.boxes)
	if wantBig == 0 {
		t.Skip("nothing is drawn large")
	}

	typeWord(m, theWord)
	m.divertAt = 0
	m.Update(key("enter"))

	gotBig := 0
	for i := range m.transit.entries {
		if m.transit.entries[i].height.Value() >= MinGlyphHeight {
			gotBig++
		}
	}
	if gotBig != wantBig {
		t.Errorf("the grid had %d digits large and the transition set off with %d",
			wantBig, gotBig)
	}
}

// The cursor is a coloured block on an ordinary cell and a white glyph on a
// cluster member, and only members are drawn large, so this puts the cursor on
// a cluster.
func TestTheCursorsDigitKeepsItsWhiteIntoTheTransition(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	tick(m, 60*40) // let the cluster swell to glyph size

	// Park the cursor on a member that got a box: a dense cluster has some of its
	// digits culled by the overlap rule.
	drawTo(m)
	found := -1
	for _, b := range m.boxes {
		for _, id := range grp.NumberIDs {
			if b.id == id {
				found = id
				m.CursorX, m.CursorY = id%GridSize, id/GridSize
				break
			}
		}
		if found >= 0 {
			break
		}
	}
	if found < 0 {
		t.Skip("no cluster member was drawn large")
	}
	m.focusLens()

	m.divertOpen = true
	m.divertAt = 3
	tick(m, 60)
	drawTo(m)

	// Re-read the box on the frame the trip sets off from. It is still growing, so
	// the one found sixty frames ago is out of date; this is why the transition
	// snapshots what was drawn.
	var box glyphBox
	for _, b := range m.boxes {
		if b.id == found {
			box = b
			break
		}
	}
	if box.cov == nil {
		t.Skip("the cursor's digit is no longer drawn large")
	}

	m.Update(key("enter"))
	after := drawTo(m)

	kept := false
	for y := box.y; y < box.y+box.h && !kept; y++ {
		for x := box.x; x < box.x+box.w; x++ {
			if after.AttrAt(x, y).FG == m.Theme.Hovered.FG {
				kept = true
				break
			}
		}
	}
	if !kept {
		t.Errorf("the cursor's digit lost its %v on the first transition frame",
			m.Theme.Hovered.FG)
	}
}
