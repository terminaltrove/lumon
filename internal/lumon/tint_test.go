package lumon

import (
	"testing"

	"github.com/terminaltrove/lumon/internal/canvas"
)

func TestADigitsColourCrossesRatherThanSwitching(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	tick(m, 60)

	from := GridIndex(m.CursorX, m.CursorY)
	m.Update(key("right"))
	to := GridIndex(m.CursorX, m.CursorY)
	if from == to {
		t.Skip("the cursor did not move")
	}

	// Both the cell arriving and the cell being left are mid-crossing.
	tick(m, 1)
	if !m.tint.Crossing(to%GridSize, to/GridSize) {
		t.Error("the cell the cursor arrived on took its new colour at once")
	}
	if !m.tint.Crossing(from%GridSize, from/GridSize) {
		t.Error("the cell the cursor left gave up its colour at once")
	}

	n := &m.Grid.Cells[to]
	gx, gy := to%GridSize, to/GridSize
	if got, want := m.cellAttr(n, gx, gy), m.cellTarget(n, gx, gy); got.FG == want.FG {
		t.Error("a frame after the cursor moved the digit was already its final colour")
	}

	// It arrives, in about the time it should.
	frames := 0
	for ; frames < 120 && m.tint.Crossing(to%GridSize, to/GridSize); frames++ {
		tick(m, 1)
	}
	if frames < 4 {
		t.Errorf("the crossing took %d frames; that is a switch", frames)
	}
	if frames > 40 {
		t.Errorf("the crossing took %d frames; that is a lag", frames)
	}
	if got, want := m.cellAttr(n, gx, gy), m.cellTarget(n, gx, gy); got != want {
		t.Errorf("the crossing finished at %v, not at the target %v", got, want)
	}
}

// A block arriving a tenth of a second late leaves the cell you point at
// ambiguous for that long.
func TestTheCursorsBlockLandsWithTheCursor(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	tick(m, 60)
	m.Update(key("right"))
	tick(m, 1)

	to := GridIndex(m.CursorX, m.CursorY)
	n := &m.Grid.Cells[to]
	gx, gy := to%GridSize, to/GridSize
	want := m.cellTarget(n, gx, gy)
	if want.BG.Set() == false {
		t.Skip("this theme's cursor is not a block")
	}
	got := m.cellAttr(n, gx, gy)
	if got.BG != want.BG {
		t.Errorf("the cursor's block is %v one frame in, want %v at once", got.BG, want.BG)
	}
	if got.Bold != want.Bold {
		t.Error("the cursor's weight lagged behind the cursor")
	}
}

// Easing a scrolled-in digit up from nothing would wash over every scroll.
func TestADigitScrollingIntoViewArrivesAtItsColour(t *testing.T) {
	tn := NewTint()
	tn.Sync(&Layout{ViewportW: 4, ViewportH: 4})
	want := canvas.Attr{FG: canvas.ParseColor("#abcdef")}
	tn.Toward(1, 1, want)
	tn.Step(1.0 / 60)

	if tn.Crossing(1, 1) {
		t.Error("a cell seen for the first time faded in")
	}
	if got := tn.Attr(1, 1, want); got != want {
		t.Errorf("a cell seen for the first time showed %v, want %v", got, want)
	}
}

// A cursor swept along a row turns each cell back before it finishes. Starting
// again from the original colour would jump.
func TestAnInterruptedCrossingSetsOffFromWhereItGotTo(t *testing.T) {
	tn := NewTint()
	tn.Sync(&Layout{ViewportW: 4, ViewportH: 4})
	dim := canvas.Attr{FG: canvas.ParseColor("#000000")}
	lit := canvas.Attr{FG: canvas.ParseColor("#ffffff")}

	tn.Toward(1, 1, dim)
	tn.Step(1.0 / 60)
	tn.Toward(1, 1, lit)
	for i := 0; i < 4; i++ {
		tn.Step(1.0 / 60)
		tn.Toward(1, 1, lit)
	}
	half := tn.Attr(1, 1, lit)
	if half.FG == dim.FG || half.FG == lit.FG {
		t.Fatalf("the cell is at %v, neither travelling nor arrived", half.FG)
	}

	tn.Toward(1, 1, dim)
	tn.Step(1.0 / 60)
	if got := tn.Attr(1, 1, dim); got.FG == dim.FG {
		t.Error("turning back jumped straight to the far colour")
	}
}

// One slot per visible cell: a slot is not a grid cell, which is why it is a
// slice.
func TestTheTintIsBoundedByTheViewport(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	tick(m, 60)
	l := m.Layout
	want := l.ViewportW * l.ViewportH
	if got := m.tint.Len(); got != want {
		t.Errorf("the tint holds %d slots for a %dx%d viewport, want %d",
			got, l.ViewportW, l.ViewportH, want)
	}

	for i := 0; i < 40; i++ {
		m.Update(key("down"))
		tick(m, 2)
	}
	if got := m.tint.Len(); got != want {
		t.Errorf("after scrolling forty rows the tint holds %d slots, want %d", got, want)
	}
}

// A slot stands for a different cell after a scroll; a carried-over crossing
// would smear the old cell's colour across the new digit.
func TestAScrollDoesNotSmearColoursBetweenCells(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	tick(m, 60)

	// Put the cursor somewhere, then scroll the viewport under it.
	before := m.Layout.ViewportY
	for i := 0; i < 40 && m.Layout.ViewportY == before; i++ {
		m.Update(key("down"))
		tick(m, 1)
	}
	if m.Layout.ViewportY == before {
		t.Skip("the viewport never scrolled")
	}
	tick(m, 1)

	l := m.Layout
	crossing := 0
	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			if m.tint.Crossing(gx, gy) {
				crossing++
			}
		}
	}
	// Only the cursor's own cell and the one it left may still be travelling, and
	// both are inside the viewport.
	if crossing > 2 {
		t.Errorf("%d digits were mid-crossing the frame after a scroll", crossing)
	}
}

// live lets the draw skip the slots. An empty slot has p at its zero value and
// the counter counts slots below one, so fresh slots must not be counted, or the
// fast path is never taken. Nothing shows on screen when this is wrong.
func TestNothingIsCrossingOnASettledBoard(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	tick(m, 300)

	for f := 0; f < 300; f++ {
		tick(m, 1)
		if live := m.tint.Live(); live != 0 {
			t.Fatalf("%d digits were crossing on a board where nothing had changed", live)
		}
	}

	// It comes back to nought after something does change.
	m.Update(key("right"))
	tick(m, 1)
	if m.tint.Live() == 0 {
		t.Fatal("moving the cursor started no crossing at all")
	}
	for f := 0; f < 120 && m.tint.Live() != 0; f++ {
		tick(m, 1)
	}
	if live := m.tint.Live(); live != 0 {
		t.Errorf("two seconds after a cursor move %d digits are still crossing", live)
	}
}

// A negative count reads as "something is crossing" for ever.
func TestTheCrossingCountNeverGoesNegative(t *testing.T) {
	m := ready()
	m.Resize(120, 40)

	for f := 0; f < 400; f++ {
		switch f % 7 {
		case 0:
			m.Update(key("down"))
		case 3:
			m.Update(key("right"))
		case 5:
			m.Resize(100+f%20, 30+f%6)
		}
		tick(m, 1)
		if live := m.tint.Live(); live < 0 {
			t.Fatalf("the crossing count went to %d on frame %d", live, f)
		}
	}
}
