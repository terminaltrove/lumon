package lumon

import "testing"

// This asks the rendered buffer, not Ripple.Offset. drawGrid places an ordinary
// digit with PlainAnchor, while ScreenAnchor is reached only by planGlyphs, so a
// ripple applied to ScreenAnchor alone moves the enlarged glyphs and leaves
// nearly the board still. The digit checked is one nothing is magnifying.
func TestADetonationMovesThePlainDigitsOnScreen(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	parkCursorAwayHelper(m)
	tick(m, 4)

	l := m.Layout
	before := drawTo(m)

	// Somewhere in the middle, well clear of the cursor's lens.
	cx, cy := l.ViewportX+l.ViewportW/2, l.ViewportY+l.ViewportH/2
	m.ripple.Kick(float64(cx), float64(cy))

	// The frame with the most of the board displaced.
	best := 0
	for i := 0; i < 90; i++ {
		tick(m, 1)
		after := drawTo(m)

		moved := 0
		for y := l.GridTop; y < l.GridTop+l.ViewportH*RowPitch; y++ {
			if before.Row(y) != after.Row(y) {
				moved++
			}
		}
		if moved > best {
			best = moved
		}
	}
	if best < 4 {
		t.Errorf("a detonation changed at most %d rows of the grid; the board is not moving", best)
	}
}

// PlainAnchor commits ambient motion through rowStep, which clamps to
// [0, maxRowSag] (down only) so a sag never lets a digit meet the one below.
// A blast must move digits up too, so the shockwave is added outside the
// triggers.
func TestABlastCanThrowADigitUpward(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	parkCursorAwayHelper(m)
	tick(m, 4)

	l := m.Layout
	cx, cy := l.ViewportX+l.ViewportW/2, l.ViewportY+l.ViewportH/2
	m.ripple.Kick(float64(cx), float64(cy))

	// A cell above the blast should be pushed further up the screen than the layout
	// puts it: a negative row offset.
	id := GridIndex(cx, cy-2)
	_, want := l.ScreenOf(cx, cy-2)

	rose := false
	for i := 0; i < 120 && !rose; i++ {
		tick(m, 1)
		if _, got := m.PlainAnchor(id); got < want {
			rose = true
		}
	}
	if !rose {
		t.Errorf("the digit above the blast never went above row %d;"+
			" maxRowSag is %d and the blast is going through it", want, maxRowSag(m.Layout.RowH))
	}
}
