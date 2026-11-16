package lumon

import (
	"math"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// lensLayout is the default spacing, which these tests' numbers are derived
// from. The lens's reach depends on the spacing.
func lensLayout() *Layout { return NewLayout(120, 40) }

// mouseMotion is the pointer moving over a screen position, with no button.
func mouseMotion(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionMotion, Button: tea.MouseButtonNone}
}

// focused returns a lens sitting still at a screen position.
func focused(x, y float64) *Lens {
	l := NewLens()
	l.Warp(x, y)
	return l
}

func TestTheLensLeavesItsFocusWhereItIs(t *testing.T) {
	l := focused(40, 12)

	x, y := l.Displace(40, 12)

	if math.Abs(x-40) > 1e-9 || math.Abs(y-12) > 1e-9 {
		t.Errorf("the focused digit moved to (%v,%v), want it left at (40,12)", x, y)
	}
}

func TestTheLensPushesNeighboursOutward(t *testing.T) {
	l := focused(40, 12)

	for _, p := range []struct{ x, y float64 }{
		{46, 12}, {34, 12}, {40, 14}, {40, 10}, {46, 14},
	} {
		before := math.Hypot(p.x-40, (p.y-12)*RowAspect)
		nx, ny := l.Displace(p.x, p.y)
		after := math.Hypot(nx-40, (ny-12)*RowAspect)
		if after <= before {
			t.Errorf("digit at (%v,%v) sits %v from the focus, was %v, the lens is not making room", p.x, p.y, after, before)
		}
	}
}

func TestTheLensLeavesDistantDigitsAlone(t *testing.T) {
	l := focused(40, 12)

	x, y := l.Displace(40+lensLayout().FisheyeRadius()+1, 12)

	if x != 40+lensLayout().FisheyeRadius()+1 || y != 12 {
		t.Errorf("a digit beyond the lens moved to (%v,%v)", x, y)
	}
}

// A folding lens would draw a digit on top of its neighbour.
func TestOneLensStillDoesNotFoldTheGrid(t *testing.T) {
	prev := -1.0
	for r := 0.0; r <= lensLayout().FisheyeRadius(); r += 0.01 {
		out := r + NewLens().bulge(r)
		if out <= prev {
			t.Fatalf("displaced radius went from %v to %v at r=%v", prev, out, r)
		}
		prev = out
	}
}

// With several lenses there is no analytic inverse, so this is tested end to
// end: a click lands on the digit you aimed at.
func TestTheRenderIndexIsTheRenderersOwnAnswer(t *testing.T) {
	m := ready()
	tick(m, 120)
	drawTo(m)

	l := m.Layout
	checked := 0
	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			id := GridIndex(gx, gy)
			if m.Flights.InFlight(id) || m.enlarged[id] {
				continue // drawn elsewhere, or not at all
			}
			ax, ay := m.PlainAnchor(id)

			// An enlarged neighbour may cover this cell: a big digit is a big click target,
			// and it is what the player sees there.
			if coveredByGlyph(m, ax, ay) {
				continue
			}

			hx, hy, ok := m.HitTest(ax, ay)
			if !ok {
				t.Fatalf("cell (%d,%d) drawn at (%d,%d) hit-tests to nothing", gx, gy, ax, ay)
			}
			if GridIndex(hx, hy) != id {
				t.Fatalf("cell (%d,%d) drawn at (%d,%d) hit-tests to (%d,%d)", gx, gy, ax, ay, hx, hy)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no cells were checked")
	}
}

// coveredByGlyph reports whether an enlarged digit has been painted over a
// screen position.
func coveredByGlyph(m *Model, x, y int) bool {
	for _, b := range m.boxes {
		if x < b.x || x >= b.x+b.w || y < b.y || y >= b.y+b.h {
			continue
		}
		if b.cov.At(x-b.x, y-b.y) > 0 {
			return true
		}
	}
	return false
}

// The index is read during Update against the frame drawn before it, and the
// viewport scrolls on every cursor move.
func TestAStaleIndexIsIgnored(t *testing.T) {
	m := ready()
	tick(m, 60)
	drawTo(m)

	l := m.Layout
	id := GridIndex(l.ViewportX+2, l.ViewportY+1)
	ax, ay := m.ScreenAnchor(id)

	// Scroll, without redrawing.
	m.Layout.ScrollTo(l.ViewportX+10, l.ViewportY+5)

	hx, hy, ok := m.HitTest(ax, ay)
	if !ok {
		t.Fatal("hit-testing gave up entirely after a scroll")
	}
	// It must agree with the live layout, not the index built before it.
	wx, wy, _ := m.Layout.CellAt(ax, ay)
	if GridIndex(hx, hy) != GridIndex(wx, wy) {
		t.Errorf("after scrolling, a click resolved to (%d,%d); the live layout says (%d,%d)",
			hx, hy, wx, wy)
	}
}

// The focus is sprung, so a pointer jumping between whole cells still moves the
// lens continuously.
func TestTheLensFollowsThePointerBetweenCells(t *testing.T) {
	m := ready()
	m.Lens.Warp(20, 10)
	m.Lens.LookAt(40, 10)

	seen := map[float64]bool{}
	for i := 0; i < 20; i++ {
		m.Lens.Step(1.0 / 60)
		seen[m.Lens.Focus().X] = true
	}

	between := 0
	for x := range seen {
		if x > 20.5 && x < 39.5 {
			between++
		}
	}
	if between < 5 {
		t.Errorf("the focus took only %d positions between the two cells, want it gliding", between)
	}
}

func TestTheKeyboardTakesTheLensBackFromTheMouse(t *testing.T) {
	m := ready()
	m.Resize(120, 40)

	m.handleMouse(mouseMotion(70, 20))
	tick(m, 30)
	afterMouse := m.Lens.Focus()

	m.Update(key("left"))
	tick(m, 30)
	afterKey := m.Lens.Focus()

	sx, _ := m.Layout.ScreenOf(m.CursorX, m.CursorY)
	if math.Abs(afterKey.X-float64(sx)) > math.Abs(afterMouse.X-float64(sx)) {
		t.Errorf("the lens is at %v after a key press, no closer to the cursor at %v than the mouse left it at %v",
			afterKey.X, sx, afterMouse.X)
	}
}

func TestClickingABulgedDigitRefinesThatDigit(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}

	target := grp.NumberIDs[0]
	gx, gy := target%GridSize, target/GridSize
	m.CursorX, m.CursorY = gx, gy
	m.focusLens()
	tick(m, 120)

	// Click where the digit appears, which is not where Layout puts it.
	ax, ay := m.ScreenAnchor(target)
	drawTo(m) // so HitTest can see the planned glyph boxes
	hx, hy, ok := m.HitTest(ax, ay)

	if !ok {
		t.Fatalf("nothing found at (%d,%d), where digit %d is drawn", ax, ay, target)
	}
	if GridIndex(hx, hy) != target {
		t.Errorf("clicking (%d,%d) hit digit %d, want %d", ax, ay, GridIndex(hx, hy), target)
	}
}

func TestClickingAnywhereOnABigGlyphHitsIt(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	_, focus := activateClusterWithRoom(t, m)
	tick(m, 900)

	drawTo(m)
	box, ok := plannedBox(m, focus)
	if !ok {
		t.Skip("the focused digit did not reach glyph size")
	}

	hits := 0
	for row := 0; row < box.cov.H; row++ {
		for col := 0; col < box.cov.W; col++ {
			if box.cov.At(col, row) <= 0 {
				continue
			}
			if hx, hy, ok := m.HitTest(box.x+col, box.y+row); ok && GridIndex(hx, hy) == focus {
				hits++
			}
		}
	}
	if hits < 10 {
		t.Errorf("only %d cells of the glyph hit-test to it", hits)
	}
}

// quieten puts every cluster back to sleep and waits for the board to go still:
// no shockwave travelling, and nothing larger than the lens holds up. A cluster
// that has just detonated leaves its digits shrinking for a second, so sleeping
// alone is not enough.
func quieten(m *Model) {
	for i := 0; i < 600; i++ {
		for _, grp := range m.Actives() {
			m.Room.Sleep(grp.ID)
		}
		tick(m, 1)

		if len(m.Room.blasts) > 0 {
			continue
		}

		l, busy := m.Layout, false
		for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH && !busy; gy++ {
			for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
				if m.Magnify.Height(gx, gy) > PlainHeight+1e-9 {
					busy = true
					break
				}
			}
		}
		if !busy {
			return
		}
	}
}

// The lens magnifies nothing; only an awake cluster grows. Both glyphTargets
// (the rule) and the drawn buffer (what a refiner sees) are asserted, since a
// thing can be true in the model and never reach the screen.
func TestOnlyCorruptionMakesADigitBig(t *testing.T) {
	for _, z := range []int{0, 1, 2} {
		m := ready()
		m.Resize(140, 44)
		m.SetZoomNow(float64(z))
		settleZoom(t, m)
		if activateFirstVisibleGroup(m) == nil {
			t.Skip("no bad group visible")
		}

		// Sweep the pointer and the cursor over the board: every way a refiner can
		// point at a digit.
		l := m.Layout
		for _, frac := range []float64{0.15, 0.4, 0.5, 0.6, 0.85} {
			gx := l.ViewportX + int(float64(l.ViewportW-1)*frac)
			gy := l.ViewportY + int(float64(l.ViewportH-1)*frac)
			m.CursorX, m.CursorY = gx, gy
			sx, sy := l.ScreenOf(gx, gy)
			m.Update(mouseMotion(sx, sy))
			tick(m, 30)

			buf := drawTo(m)
			for cy := l.ViewportY; cy < l.ViewportY+l.ViewportH; cy++ {
				for cx := l.ViewportX; cx < l.ViewportX+l.ViewportW; cx++ {
					if awakeAt(m, cx, cy) {
						continue
					}
					if h := m.Magnify.Height(cx, cy); h > PlainHeight {
						t.Fatalf("at zoom %d, pointing at (%d,%d): the ordinary digit "+
							"(%d,%d) is %v rows", z, gx, gy, cx, cy, h)
					}
					if m.enlarged[GridIndex(cx, cy)] {
						t.Fatalf("at zoom %d, pointing at (%d,%d): the ordinary digit "+
							"(%d,%d) was drawn as a glyph", z, gx, gy, cx, cy)
					}
				}
			}
			_ = buf
		}
	}
}

// Two refiners pointing at one ordinary digit cannot between them make it
// look like a cluster.
func TestTwoRefinersCannotMakeAnOrdinaryDigitLookCorrupt(t *testing.T) {
	_, a, b := twoInARoom(t)
	a.Resize(140, 44)
	b.Resize(140, 44)

	l := a.Layout
	gx, gy := l.ViewportX+l.ViewportW/2, l.ViewportY+l.ViewportH/2
	if awakeAt(a, gx, gy) {
		t.Skip("the middle of the board is corrupt")
	}
	sx, sy := l.ScreenOf(gx, gy)
	a.CursorX, a.CursorY = gx, gy
	b.CursorX, b.CursorY = gx, gy
	a.Update(mouseMotion(sx, sy))
	b.Update(mouseMotion(sx, sy))
	tick(a, 60)
	tick(b, 60)

	if h := a.Magnify.Height(gx, gy); h > PlainHeight {
		t.Errorf("two refiners on one ordinary digit drew it %v rows tall", h)
	}
}

// f over an ordinary cell, or over a corrupt one whose cluster is asleep, leaves
// the board exactly as it was (lockUnderCursor has no else branch).
func TestALockedClusterIsTheOnlyThingYouCanEnlarge(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}

	// An ordinary cell first.
	l := m.Layout
	var ox, oy = -1, -1
	for cy := l.ViewportY; cy < l.ViewportY+l.ViewportH && ox < 0; cy++ {
		for cx := l.ViewportX; cx < l.ViewportX+l.ViewportW; cx++ {
			if !awakeAt(m, cx, cy) && m.Grid.At(cx, cy) != nil {
				ox, oy = cx, cy
				break
			}
		}
	}
	if ox < 0 {
		t.Skip("no ordinary cell on screen")
	}
	m.CursorX, m.CursorY = ox, oy
	m.Update(key("f"))
	tick(m, 120)
	if len(m.held) > 0 {
		t.Errorf("f on the ordinary cell (%d,%d) ringfenced %v", ox, oy, m.held)
	}
	if h := m.Magnify.Height(ox, oy); h > PlainHeight {
		t.Errorf("f on an ordinary cell drew it %v rows tall", h)
	}

	// Then the cluster, which does lift.
	id := grp.NumberIDs[0]
	m.CursorX, m.CursorY = id%GridSize, id/GridSize
	m.Update(key("f"))
	tick(m, 120)
	if !m.held[grp.ID] {
		t.Fatalf("f on a cluster member ringfenced %v, want group %d", m.held, grp.ID)
	}
	if peak := clusterPeak(m, grp); peak < MaxGlyphHeight-0.5 {
		t.Errorf("the ringfenced cluster is %v rows, want it at full size", peak)
	}
}
