package lumon

import (
	"math"
	"testing"
)

// Plain rounding of an offset flips every time it crosses a half-column;
// columnStep adds a dead band so a digit near a boundary does not stutter.

func TestColumnStepRoundsWhenClearlyInsideAColumn(t *testing.T) {
	cases := []struct {
		offset float64
		want   int
	}{
		{0, 0},
		{0.1, 0},
		{1.4, 1},
		{-1.4, -1},
		{2.3, 2},
	}
	for _, c := range cases {
		if got := columnStep(c.offset, 0); got != c.want {
			t.Errorf("columnStep(%v, 0) = %d, want %d", c.offset, got, c.want)
		}
	}
}

func TestColumnStepHoldsItsColumnOnTheBoundary(t *testing.T) {
	// Just under and just over a half column, alternating: the pattern that makes
	// plain rounding stutter.
	cur := 0
	flips := 0
	for i := 0; i < 200; i++ {
		offset := 0.49
		if i%2 == 0 {
			offset = 0.51
		}
		next := columnStep(offset, cur)
		if next != cur {
			flips++
		}
		cur = next
	}

	if flips != 0 {
		t.Errorf("column changed %d times while the offset hovered on the boundary", flips)
	}
}

func TestColumnStepMovesOnceTheOffsetClearsTheDeadBand(t *testing.T) {
	if got := columnStep(columnDeadBand+0.51, 0); got != 1 {
		t.Errorf("an offset past the dead band did not advance: got %d", got)
	}
	if got := columnStep(-(columnDeadBand + 0.51), 0); got != -1 {
		t.Errorf("a negative offset past the dead band did not advance: got %d", got)
	}
}

func TestColumnStepIsStickyOnceItHasMoved(t *testing.T) {
	held := columnStep(1.0, 1)
	if held != 1 {
		t.Errorf("column fell back to %d at offset 1.0", held)
	}
	if got := columnStep(0.6, 1); got != 1 {
		t.Errorf("column fell back to %d at offset 0.6", got)
	}
	if got := columnStep(0.1, 1); got != 0 {
		t.Errorf("column stayed at %d once the offset returned to centre", got)
	}
}

func TestColumnStepCatchesUpAfterAJump(t *testing.T) {
	if got := columnStep(4.0, 0); got != 4 {
		t.Errorf("columnStep(4.0, 0) = %d, want it to catch up to 4", got)
	}
	if got := columnStep(-4.0, 0); got != -4 {
		t.Errorf("columnStep(-4.0, 0) = %d, want it to catch up to -4", got)
	}
}

func TestColumnStepIsSymmetric(t *testing.T) {
	for _, o := range []float64{0.3, 0.9, 1.7, 2.4} {
		pos := columnStep(o, 0)
		neg := columnStep(-o, 0)
		if pos != -neg {
			t.Errorf("columnStep(%v)=%d but columnStep(%v)=%d", o, pos, -o, neg)
		}
	}
}

// The lens leaves many cells at its rim with a size right on the threshold.
func TestTheGlyphDecisionDoesNotFlicker(t *testing.T) {
	on := false
	flips := 0
	for i := 0; i < 600; i++ {
		// A height wobbling by a fraction of a row either side of the threshold.
		h := MinGlyphHeight + 0.1*math.Sin(float64(i)/3)
		next := glyphOn(h, on)
		if next != on {
			flips++
		}
		on = next
	}
	if flips > 1 {
		t.Errorf("the glyph decision flipped %d times on a height that never left the dead band", flips)
	}
}

func TestTheGlyphDecisionStillFollowsRealGrowth(t *testing.T) {
	if glyphOn(MinGlyphHeight+glyphBand+1, false) != true {
		t.Error("a digit well above the threshold is not drawn as a glyph")
	}
	if glyphOn(PlainHeight, true) != false {
		t.Error("a digit back at plain size is still being drawn as a glyph")
	}
}

// Otherwise two neighbouring digits could land on the same screen row and one
// would be overdrawn. Checked at every zoom level, since zoom relies on it; a
// pitch of one would leave no blank row at all.
func TestRowSagStaysInsideTheBlankRow(t *testing.T) {
	for _, level := range zoomLevels {
		rowH := float64(level[1])
		sag := maxRowSag(rowH)
		if sag < 1 {
			t.Fatalf("a pitch of %v leaves a digit nowhere to sag", rowH)
		}

		current := 0
		for _, offset := range []float64{-5, -1, -0.4, 0, 0.4, 1, 5, 100, -100} {
			current = rowStep(offset, current, sag)
			if current < 0 || current > sag {
				t.Fatalf("at a pitch of %v, offset %v produced row %d, outside [0,%d]",
					rowH, offset, current, sag)
			}
		}
	}
}
