package lumon

import (
	"fmt"
	"math"
	"testing"
)

// settleZoom ticks until the spacing has arrived. physics.Scalar asymptotes, so
// the time to come inside zoomSettled depends on the frequency; it waits on the
// condition.
func settleZoom(t *testing.T, m *Model) {
	t.Helper()
	for i := 0; m.zooming(); i++ {
		if i > 600 {
			t.Fatalf("the spacing never settled: at %v heading for %v",
				m.Layout.Zoom(), m.Layout.WantZoom())
		}
		tick(m, 1)
	}
}

// everyZoom runs f at each spacing the board has.
func everyZoom(t *testing.T, m *Model, f func(t *testing.T, l *Layout)) {
	t.Helper()
	for z := 0; z <= m.Layout.MaxZoom(); z++ {
		m.SetZoomNow(float64(z))
		m.afterLayout()
		tick(m, 20)
		l := m.Layout
		t.Run(fmt.Sprintf("%vx%v", l.CellW, l.RowH), func(t *testing.T) { f(t, l) })
	}
}

// There is no level tighter than the starting one; maxRowSag relies on that,
// being meaningless at a pitch of one.
func TestZoomOnlyEverSpreadsTheGridOut(t *testing.T) {
	l := NewLayout(120, 40)
	if l.CellW != CellWidth || l.RowH != RowPitch {
		t.Fatalf("a fresh board is spaced %vx%v, want the default %vx%v",
			l.CellW, l.RowH, CellWidth, RowPitch)
	}

	// Out from the default is a no-op.
	if l.SetZoom(-1) || l.CellW != CellWidth {
		t.Errorf("zooming out from the default gave %vx%v", l.CellW, l.RowH)
	}

	prevW, prevH := l.CellW, l.RowH
	for z := 1; z <= l.MaxZoom(); z++ {
		if !l.SetZoom(float64(z)) {
			t.Fatalf("level %d did not change the spacing", z)
		}
		if l.CellW < prevW || l.RowH < prevH || l.CellW*l.RowH <= prevW*prevH {
			t.Errorf("level %d is %vx%v against the previous %vx%v; every step must "+
				"spread the digits further apart", z, l.CellW, l.RowH, prevW, prevH)
		}
		prevW, prevH = l.CellW, l.RowH
	}

	// In from the furthest is a no-op, and the round trip is exact.
	if l.SetZoom(float64(l.MaxZoom() + 1)) {
		t.Error("zooming past the last level changed the spacing")
	}
	l.SetZoom(0)
	if l.CellW != CellWidth || l.RowH != RowPitch {
		t.Errorf("coming back gave %vx%v, want the default %vx%v",
			l.CellW, l.RowH, CellWidth, RowPitch)
	}
}

// The lens's reach scales off the cell width alone, so the bulge is round on
// screen only while a cell's width-to-height ratio is fixed. A level of eight by
// three would make it an ellipse.
func TestTheZoomLevelsKeepTheCellsShape(t *testing.T) {
	want := float64(zoomLevels[0][0]) / (float64(zoomLevels[0][1]) * RowAspect)
	for _, lv := range zoomLevels {
		got := float64(lv[0]) / (float64(lv[1]) * RowAspect)
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("a %vx%v cell is %v wide per unit tall, against %v for the default; "+
				"the lens is only round while every level shares one shape",
				lv[0], lv[1], got, want)
		}
	}
}

// A zoom without a resize leaves the terminal, viewport, room and file
// unchanged, and width*height too, so resetAnchors would not reallocate. Unless
// the stamp includes the spacing, HitTest answers a click on a board drawn
// twelve columns apart from boxes stamped six apart, confidently wrong where
// falling through to Layout.CellAt would be right.
func TestAZoomedBoardDoesNotAnswerClicksAgainstTheOldSpacing(t *testing.T) {
	m := ready()
	m.Resize(160, 50)
	tick(m, 30)
	drawTo(m) // stamps the boxes and the index at the default spacing

	for z := 1; z <= m.Layout.MaxZoom(); z++ {
		m.Zoom(1)
		settleZoom(t, m)

		l := m.Layout
		for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
			for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
				sx, sy := l.ScreenOf(gx, gy)
				hx, hy, ok := m.HitTest(sx, sy)
				if !ok || hx != gx || hy != gy {
					t.Fatalf("at %vx%v a click on the centre of (%d,%d) answered "+
						"(%d,%d,ok=%v); the index is still spaced for the old board",
						l.CellW, l.RowH, gx, gy, hx, hy, ok)
				}
			}
		}
	}
}

// markAnchor's footprint and CellAt both derive from CellBox; this keeps them
// agreeing.
func TestTheAnchorFootprintIsTheCellCellAtClaims(t *testing.T) {
	m := ready()
	m.Resize(140, 44)
	everyZoom(t, m, func(t *testing.T, l *Layout) {
		gx, gy := l.ViewportX+2, l.ViewportY+2
		cx, cy := l.ScreenOf(gx, gy)
		left, right, top, bottom := l.CellBoxAt(gx, gy)

		for dy := top; dy < bottom; dy++ {
			for dx := left; dx < right; dx++ {
				ax, ay, ok := l.CellAt(cx+dx, cy+dy)
				if !ok || ax != gx || ay != gy {
					t.Fatalf("offset (%d,%d) inside the cell's own box lands on "+
						"(%d,%d,ok=%v), not (%d,%d)", dx, dy, ax, ay, ok, gx, gy)
				}
			}
		}

		// And one column past the box belongs to somebody else.
		if ax, _, ok := l.CellAt(cx+right, cy); ok && ax == gx {
			t.Errorf("the column at +%d is outside the box and still claimed by the "+
				"same cell", right)
		}
	})
}

// The push stays under a quarter of the reach at every level, so the grid never
// folds over itself. Both are linear in the cell width, so it follows by
// arithmetic.
func TestTheLensNeverFoldsTheGridAtAnyZoom(t *testing.T) {
	l := NewLayout(140, 44)
	for z := 0; z <= l.MaxZoom(); z++ {
		l.SetZoom(float64(z))
		if l.LensPush() >= l.FisheyeRadius()/4 {
			t.Fatalf("at %vx%v the push is %v against a quarter-reach of %v; the map "+
				"stops being monotonic and two cells land on one spot",
				l.CellW, l.RowH, l.LensPush(), l.FisheyeRadius()/4)
		}

		// And the mapping is strictly increasing out to the rim.
		ln := NewLens()
		ln.Fit(l)
		prev := -1.0
		for r := 0.0; r <= l.FisheyeRadius(); r += 0.05 {
			out := r + ln.bulge(r)
			if out <= prev {
				t.Fatalf("at %vx%v the lens folds at radius %v", l.CellW, l.RowH, r)
			}
			prev = out
		}
	}
}

// Remotely the board is shared and the spacing is not: one refiner zooming must
// not respace anybody else's screen, or touch the board at all.
func TestZoomIsOneTerminalsBusiness(t *testing.T) {
	_, a, b := twoInARoom(t)
	a.Resize(140, 44)
	b.Resize(140, 44)

	a.Zoom(2)
	settleZoom(t, a)
	if a.Layout.Zoom() == b.Layout.Zoom() {
		t.Fatal("both terminals are at the same level; this is not testing what it means to")
	}
	if b.Layout.CellW != CellWidth || b.Layout.RowH != RowPitch {
		t.Errorf("one refiner zoomed and the other's board is now spaced %vx%v",
			b.Layout.CellW, b.Layout.RowH)
	}
}

// Zoom is a choice, not a consequence of the terminal's size.
func TestZoomSurvivesAResize(t *testing.T) {
	m := ready()
	m.Resize(140, 44)
	m.Zoom(2)
	settleZoom(t, m)
	want := m.Layout.CellW

	m.Resize(100, 40)
	if m.Layout.Zoom() != 2 || m.Layout.CellW != want {
		t.Errorf("after a resize the board is at level %v spaced %v, want level 2 spaced %v",
			m.Layout.Zoom(), m.Layout.CellW, want)
	}
}

// The endpoints of an ease were computed against a screen that no longer
// exists, as with a transit.
func TestAResizeMidZoomArrives(t *testing.T) {
	m := ready()
	m.Resize(140, 44)
	m.Zoom(1)
	tick(m, 6) // travelling, nowhere near arrived
	if !m.zooming() {
		t.Fatal("the board had already settled; this is not testing anything")
	}

	m.Resize(100, 40)
	if m.zooming() {
		t.Error("the board is still easing towards a level against a screen that changed")
	}
	if m.Layout.CellW != float64(zoomLevels[1][0]) {
		t.Errorf("the spacing is %v, want the level it was heading for, %d",
			m.Layout.CellW, zoomLevels[1][0])
	}
}

// A zoom pinned at the corner would throw the middle of the board off the
// bottom right.
func TestZoomKeepsTheCursorOnScreen(t *testing.T) {
	m := ready()
	m.Resize(140, 44)
	l := m.Layout
	m.CursorX, m.CursorY = l.ViewportX+l.ViewportW-1, l.ViewportY+l.ViewportH-1

	for z := 1; z <= m.Layout.MaxZoom(); z++ {
		m.Zoom(1)
		settleZoom(t, m)
		l = m.Layout
		if m.CursorX < l.ViewportX || m.CursorX >= l.ViewportX+l.ViewportW ||
			m.CursorY < l.ViewportY || m.CursorY >= l.ViewportY+l.ViewportH {
			t.Fatalf("at %vx%v the cursor (%d,%d) is outside the viewport %d..%d, %d..%d",
				l.CellW, l.RowH, m.CursorX, m.CursorY,
				l.ViewportX, l.ViewportX+l.ViewportW, l.ViewportY, l.ViewportY+l.ViewportH)
		}
	}
}

// Every zoom today goes through Model.Zoom, which drops the index, and SetZoom
// recentres the viewport anyway, so this checks the field itself: two boards
// alike in every other respect but spaced differently must not stamp the same.
// A second way to respace the board would otherwise have an index six columns
// apart answering clicks on a board twelve apart. resetAnchors does not
// reallocate, since width*height is unchanged.
func TestTheAnchorStampCarriesTheSpacing(t *testing.T) {
	m := ready()
	m.Resize(160, 50)

	seen := map[anchorStamp]int{}
	for z := 0; z <= m.Layout.MaxZoom(); z++ {
		m.Layout.applyZoom(float64(z))

		// Only the spacing differs: the viewport is put back by hand, so this cannot
		// pass on a field that moved for another reason.
		m.Layout.ViewportX, m.Layout.ViewportY = 0, 0
		st := m.stampNow()
		if prev, ok := seen[st]; ok {
			t.Fatalf("levels %d and %d stamp identically; an index built at one "+
				"would be trusted by the other", prev, z)
		}
		seen[st] = z
	}
}

// The chrome is laid out from the terminal, not the cell, so spacing the digits
// out must not lift the header, move the rules or shift the bins. 120x34 is
// the case to watch: there level 1 can give the header's four rows back and
// level 2's coarser flooring take them again, moving BinTop.
func TestTheChromeStandsStillWhileTheBoardIsRespaced(t *testing.T) {
	for _, sz := range [][2]int{{120, 40}, {120, 34}, {100, 30}, {80, 24}, {200, 60}} {
		l := NewLayout(sz[0], sz[1])

		type chrome struct {
			headerH, gridTop, rule, binTop int
			binX, binW                     [BinCount]int
		}
		read := func() chrome {
			c := chrome{l.HeaderH, l.GridTop, l.BoardY + l.BoardH, l.BinTop, [BinCount]int{}, [BinCount]int{}}
			for i := 0; i < BinCount; i++ {
				c.binX[i], c.binW[i] = l.BinOriginX(i), l.BinWidth(i)
			}
			return c
		}
		want := read()

		// Every spacing the ease passes through, including those between levels.
		for z := 0.0; z <= float64(l.MaxZoom()); z += 1.0 / 32 {
			l.applyZoom(z)
			l.recompute()
			if got := read(); got != want {
				t.Errorf("%dx%d at level %v: the chrome is %+v, want %+v, a zoom "+
					"respaces the board and nothing else", sz[0], sz[1], z, got, want)
				break
			}
		}
	}
}

// This rules out snapping the cell count to the target level: coming back out,
// the target count is the larger one, so nineteen cells at a pitch of twelve is
// 228 columns inside a rectangle 114 wide, and vertically it would spill over
// the rule into the bins.
func TestTheBoardNeverDrawsOutsideItsRectangle(t *testing.T) {
	l := NewLayout(120, 40)
	for z := 0.0; z <= float64(l.MaxZoom()); z += 1.0 / 64 {
		l.applyZoom(z)
		l.recompute()

		if l.GridOriginX < l.BoardX || l.GridOriginX+l.colLeft(l.ViewportW) > l.BoardX+l.BoardW {
			t.Fatalf("at level %v the board runs %d..%d inside a rectangle %d..%d",
				z, l.GridOriginX, l.GridOriginX+l.colLeft(l.ViewportW),
				l.BoardX, l.BoardX+l.BoardW)
		}
		if l.rowTop(l.ViewportH) > l.BoardH {
			t.Fatalf("at level %v the board is %d rows deep in a rectangle of %d",
				z, l.rowTop(l.ViewportH), l.BoardH)
		}
	}
}

// Not only at the three levels it rests at. A column has half a cell of margin
// either side of ScreenOf's centre, so it survives most rounding; a row has
// none, since ScreenOf returns its first screen row. At a pitch of 3.5 row three
// begins at row ten, and 10/3.5 = 2.86 falls in the row above. Both directions
// read the same boundary table, so this is exact.
func TestScreenOfAndCellAtAreInversesAtEveryPitch(t *testing.T) {
	l := NewLayout(120, 40)
	l.ViewportX, l.ViewportY = 7, 13

	for z := 0.0; z <= float64(l.MaxZoom()); z += 1.0 / 64 {
		l.applyZoom(z)
		l.recompute()
		l.ScrollTo(7, 13)

		for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
			for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
				sx, sy := l.ScreenOf(gx, gy)
				bx, by, ok := l.CellAt(sx, sy)
				if !ok || bx != gx || by != gy {
					t.Fatalf("at %vx%v, (%d,%d) -> screen (%d,%d) -> (%d,%d,ok=%v)",
						l.CellW, l.RowH, gx, gy, sx, sy, bx, by, ok)
				}
			}
		}
	}
}

// No screen cell inside the board is claimed by two grid cells, and none by
// nobody. With a fractional pitch the spans differ (at 7.5 columns they run
// eight, seven, eight), so one shared box stamped by markAnchor would give a
// strip of columns to the next cell.
func TestTheCellBoxesTileTheBoardAtEveryPitch(t *testing.T) {
	l := NewLayout(120, 40)

	for z := 0.0; z <= float64(l.MaxZoom()); z += 1.0 / 16 {
		l.applyZoom(z)
		l.recompute()

		owner := map[[2]int][2]int{}
		for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
			for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
				cx, cy := l.ScreenOf(gx, gy)
				left, right, top, bottom := l.CellBoxAt(gx, gy)
				for dy := top; dy < bottom; dy++ {
					for dx := left; dx < right; dx++ {
						at := [2]int{cx + dx, cy + dy}
						if prev, taken := owner[at]; taken {
							t.Fatalf("at %vx%v screen (%d,%d) is claimed by both (%d,%d) and (%d,%d)",
								l.CellW, l.RowH, at[0], at[1], prev[0], prev[1], gx, gy)
						}
						owner[at] = [2]int{gx, gy}

						if ax, ay, ok := l.CellAt(at[0], at[1]); !ok || ax != gx || ay != gy {
							t.Fatalf("at %vx%v screen (%d,%d) is inside (%d,%d)'s box and "+
								"CellAt says (%d,%d,ok=%v)",
								l.CellW, l.RowH, at[0], at[1], gx, gy, ax, ay, ok)
						}
					}
				}
			}
		}

		// And the boxes leave no hole: every column and row of the drawn board
		// belongs to somebody.
		want := l.colLeft(l.ViewportW) * l.rowTop(l.ViewportH)
		if len(owner) != want {
			t.Errorf("at %vx%v the boxes cover %d screen cells of %d",
				l.CellW, l.RowH, len(owner), want)
		}
	}
}

// physics.Scalar is critically damped, so its step response leaves rest, rises
// to a single peak and decays. The frame-to-frame change must rise, peak once
// and fall, with both ends a small fraction of the middle. A snap would be one
// delta of three columns and the rest nought.
func TestTheZoomEasesInAndOutRatherThanJumping(t *testing.T) {
	m := ready()
	m.Resize(140, 44)

	from := m.Layout.CellW
	m.Update(key("+"))

	var deltas []float64
	prev := from
	for m.zooming() {
		tick(m, 1)
		deltas = append(deltas, m.Layout.CellW-prev)
		prev = m.Layout.CellW
	}
	if len(deltas) < 10 {
		t.Fatalf("the whole journey took %d frames; that is a jump, not an ease", len(deltas))
	}

	peak, at := 0.0, 0
	for i, d := range deltas {
		if d < -1e-12 {
			t.Fatalf("frame %d moved the spacing backwards by %v; a critically damped "+
				"spring never overshoots", i, -d)
		}
		if d > peak {
			peak, at = d, i
		}
	}

	// Only the part of the journey that is moving. The spring asymptotes, so the
	// tail is a thousandth of a column a frame and its ordering is float noise.
	last := at
	for last+1 < len(deltas) && deltas[last+1] >= peak/100 {
		last++
	}
	for i := 1; i <= at; i++ {
		if deltas[i] < deltas[i-1]-1e-12 {
			t.Errorf("the spacing slowed at frame %d, before its peak at %d", i, at)
		}
	}
	for i := at + 1; i <= last; i++ {
		if deltas[i] > deltas[i-1]+1e-12 {
			t.Errorf("the spacing quickened at frame %d, after its peak at %d", i, at)
		}
	}

	// Both ends are gentle, and the peak is in the middle of the journey, not at
	// either end.
	if at == 0 || at > last {
		t.Errorf("the fastest frame is %d of %d; an ease peaks in the middle", at, last)
	}
	if deltas[0] > peak/2 {
		t.Errorf("the first frame moved %v against a peak of %v; that is not an ease in",
			deltas[0], peak)
	}
	if deltas[last] > peak/10 {
		t.Errorf("the last moving frame moved %v against a peak of %v; that is not an "+
			"ease out", deltas[last], peak)
	}

	// And it lands exactly on the level.
	if m.Layout.CellW != float64(zoomLevels[1][0]) || m.Layout.RowH != float64(zoomLevels[1][1]) {
		t.Errorf("it settled at %vx%v, want exactly %dx%d",
			m.Layout.CellW, m.Layout.RowH, zoomLevels[1][0], zoomLevels[1][1])
	}
}

// TestTheChromeStandsStillWhileTheBoardIsRespaced, driven through the key
// handler and the spring instead of by setting a level.
func TestNothingButTheBoardMovesWhileAZoomEases(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	tick(m, 10)

	l := m.Layout
	headerH, gridTop, rule, binTop := l.HeaderH, l.GridTop, l.BoardY+l.BoardH, l.BinTop
	var binX, binW [BinCount]int
	for i := 0; i < BinCount; i++ {
		binX[i], binW[i] = l.BinOriginX(i), l.BinWidth(i)
	}

	m.Update(key("+"))
	m.Update(key("+"))
	for frames := 0; m.zooming(); frames++ {
		tick(m, 1)
		l = m.Layout
		if l.HeaderH != headerH || l.GridTop != gridTop ||
			l.BoardY+l.BoardH != rule || l.BinTop != binTop {
			t.Fatalf("frame %d of the ease moved the chrome: header %d, top %d, rule %d, "+
				"bins %d, want %d, %d, %d, %d", frames,
				l.HeaderH, l.GridTop, l.BoardY+l.BoardH, l.BinTop,
				headerH, gridTop, rule, binTop)
		}
		for i := 0; i < BinCount; i++ {
			if l.BinOriginX(i) != binX[i] || l.BinWidth(i) != binW[i] {
				t.Fatalf("frame %d of the ease moved bin %d to %d wide %d, want %d wide %d",
					frames, i, l.BinOriginX(i), l.BinWidth(i), binX[i], binW[i])
			}
		}
	}
}

// Both tiers above Layout.CellAt record where a digit was drawn, so hit-testing
// survives a moving spacing as long as the stamp includes the eased pitch. This
// asserts the index is live mid-ease. Two digits can share a screen cell when
// the lens pushes them about (the later stamp wins), so which one a click names
// is the renderer's business.
func TestAClickDuringAZoomLandsOnTheDigitUnderIt(t *testing.T) {
	m := ready()
	m.Resize(160, 50)
	tick(m, 20)

	m.Update(key("+"))
	for frames := 0; m.zooming() && frames < 8; frames++ {
		tick(m, 1)
	}
	if !m.zooming() {
		t.Fatal("the board settled before the check; this is not testing an ease")
	}
	drawTo(m)

	if m.anchorAt != m.stampNow() {
		t.Fatal("the frame that was just drawn does not match the board it was drawn " +
			"against; every click mid-zoom falls through to the flat mapping")
	}
	if m.anchorAt.cellW != m.Layout.CellW || m.anchorAt.rowH != m.Layout.RowH {
		t.Errorf("the index is stamped for a %vx%v cell on a board spaced %vx%v",
			m.anchorAt.cellW, m.anchorAt.rowH, m.Layout.CellW, m.Layout.RowH)
	}

	// And every digit the renderer drew answers for itself: whatever cell a click
	// on it names, that cell was drawn there too.
	l := m.Layout
	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			sx, sy := m.PlainAnchor(GridIndex(gx, gy))
			hx, hy, ok := m.HitTest(sx, sy)
			if !ok {
				t.Fatalf("mid-ease at %vx%v, the digit drawn for (%d,%d) at (%d,%d) "+
					"answers no cell at all", l.CellW, l.RowH, gx, gy, sx, sy)
			}
			if hx == gx && hy == gy {
				continue
			}
			if ax, ay := m.PlainAnchor(GridIndex(hx, hy)); ax != sx || ay != sy {
				if !m.enlarged[GridIndex(hx, hy)] {
					t.Fatalf("mid-ease at %vx%v, a click at (%d,%d) named (%d,%d), "+
						"which is drawn at (%d,%d), nowhere near it",
						l.CellW, l.RowH, sx, sy, hx, hy, ax, ay)
				}
			}
		}
	}
}

// Coming back out, a digit with two rows of sag at a pitch of three is in a
// band two rows tall as the pitch reaches two, and lands on its neighbour.
// Reading the live pitch alone allows that; the bound is the narrowest row
// between here and the zoom's target. The sag is checked, not the drawn
// position, since the lens bulge may move a digit.
func TestADigitsSagNeverReachesTheRowBelowWhileAZoomEases(t *testing.T) {
	m := ready()
	m.Resize(140, 44)
	m.Zoom(2)
	settleZoom(t, m)

	m.Update(key("-"))
	m.Update(key("-"))
	for frames := 0; m.zooming(); frames++ {
		tick(m, 1)
		l := m.Layout

		// Every row on screen, now and at every instant still to come, is at least
		// this many screen rows deep.
		narrowest := int(math.Min(l.RowH, l.wantRowH))
		if sag := l.MaxRowSag(); sag >= narrowest {
			t.Fatalf("frame %d at a pitch of %v heading for %v: a digit may sag %d rows "+
				"into a band that is %d deep, so it lands on the digit below",
				frames, l.RowH, l.wantRowH, sag, narrowest)
		}
	}
}

// Under an ease the viewport and spacing change on most frames, so the lattice
// must survive them, or a blast set off as somebody pressed the key would be
// rebuilt out of existence.
func TestABlastRipplesThroughAZoom(t *testing.T) {
	m := ready()
	m.Resize(140, 44)
	tick(m, 10)

	l := m.Layout
	m.ripple.Kick(float64(l.ViewportX+l.ViewportW/2), float64(l.ViewportY+l.ViewportH/2))
	m.Update(key("+"))

	moved := false
	for frames := 0; m.zooming(); frames++ {
		tick(m, 1)
		l = m.Layout
		for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH && !moved; gy++ {
			for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
				if dx, dy := m.ripple.Offset(gx, gy); dx*dx+dy*dy > 1e-6 {
					moved = true
					break
				}
			}
		}
	}
	if !moved {
		t.Error("the board was flat for the whole of the zoom; the blast was rebuilt away")
	}
}
