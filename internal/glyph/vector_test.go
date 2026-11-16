package glyph

import (
	"math"
	"testing"
)

// Digits are stroke outlines in a unit box, rasterized at any size. These
// tests pin the properties the rasterizer relies on.

func TestEveryDigitHasStrokes(t *testing.T) {
	for d := 0; d <= 9; d++ {
		if got := DigitStrokes(d); len(got) == 0 {
			t.Errorf("digit %d has no strokes", d)
		}
	}
}

func TestOutOfRangeDigitsHaveNoStrokes(t *testing.T) {
	for _, d := range []int{-1, 10, 99} {
		if got := DigitStrokes(d); got != nil {
			t.Errorf("DigitStrokes(%d) = %v, want nil", d, got)
		}
	}
}

func TestStrokesHaveAtLeastTwoPoints(t *testing.T) {
	for d := 0; d <= 9; d++ {
		for i, s := range DigitStrokes(d) {
			if len(s) < 2 {
				t.Errorf("digit %d stroke %d has %d points, want at least 2", d, i, len(s))
			}
		}
	}
}

// The rasterizer maps the unit box onto the glyph's cell box, so anything
// outside it would be clipped.
func TestAllPointsLieInsideTheUnitBox(t *testing.T) {
	for d := 0; d <= 9; d++ {
		for i, s := range DigitStrokes(d) {
			for j, p := range s {
				if p.X < 0 || p.X > 1 || p.Y < 0 || p.Y > 1 {
					t.Errorf("digit %d stroke %d point %d is %+v, outside the unit box", d, i, j, p)
				}
			}
		}
	}
}

// Otherwise a digit looks shrunken next to the others.
func TestEachDigitFillsMostOfItsBox(t *testing.T) {
	for d := 0; d <= 9; d++ {
		minX, minY := 1.0, 1.0
		maxX, maxY := 0.0, 0.0
		for _, s := range DigitStrokes(d) {
			for _, p := range s {
				minX = min(minX, p.X)
				maxX = max(maxX, p.X)
				minY = min(minY, p.Y)
				maxY = max(maxY, p.Y)
			}
		}
		if h := maxY - minY; h < 0.7 {
			t.Errorf("digit %d spans %.2f vertically, want it to fill its box", d, h)
		}
		// '1' is narrow; every other digit should have width.
		if w := maxX - minX; d != 1 && w < 0.5 {
			t.Errorf("digit %d spans %.2f horizontally, want more", d, w)
		}
	}
}

func TestStrokeWidthIsPositiveAndSane(t *testing.T) {
	if StrokeHalfWidth <= 0 || StrokeHalfWidth > 0.25 {
		t.Errorf("StrokeHalfWidth = %v, want a small positive fraction of the box", StrokeHalfWidth)
	}
}

// Otherwise 0 and 8 render with a gap.
func TestRoundDigitsClose(t *testing.T) {
	for _, d := range []int{0, 8} {
		for i, s := range DigitStrokes(d) {
			first, last := s[0], s[len(s)-1]
			dx, dy := first.X-last.X, first.Y-last.Y
			if dx*dx+dy*dy > 1e-6 {
				t.Errorf("digit %d stroke %d does not close: %+v vs %+v", d, i, first, last)
			}
		}
	}
}

// The range of heights, in rows, this rasteriser is asked for. They
// approximate lumon.MinGlyphHeight (less its Schmitt band) and
// lumon.MaxGlyphHeight; glyph must not import lumon, and the pen should behave
// beyond today's exact thresholds.
const (
	smallestDrawn = 1.9  // about where a glyph starts being drawn
	largestDrawn  = 11.0 // the largest a digit is drawn
)

// A pen that is a fixed fraction of the box falls under one cell by four rows,
// so a small glyph has no fully covered stroke. It thickens instead, as a
// screen face gets relatively heavier as it gets smaller.
func TestThePenThickensAsTheGlyphShrinks(t *testing.T) {
	big := strokeHalfWidthAt(largestDrawn, true)
	if big != StrokeHalfWidth {
		t.Errorf("at the largest size the pen is %v, want StrokeHalfWidth %v", big, StrokeHalfWidth)
	}

	small := strokeHalfWidthAt(smallestDrawn, true)
	if small <= big {
		t.Errorf("the pen at the smallest drawn glyph is %v, no thicker than %v", small, big)
	}

	// Monotone and within bounds: a pen past penMax closes a digit's counters
	// and the digits look alike. Both renderings take different weights.
	prev := math.Inf(1)
	for h := 0.5; h <= largestDrawn*2; h += 0.05 {
		p := strokeHalfWidthAt(h, true)
		if p < StrokeHalfWidth || p > penMax {
			t.Fatalf("the pen at %.2f rows is %v, outside [%v, %v]", h, p, StrokeHalfWidth, penMax)
		}
		if p > prev+1e-12 {
			t.Fatalf("the pen thickened as the glyph grew, at %.2f rows", h)
		}
		prev = p
	}
}

// A glyph drawn in copies of its own digit shows its identity through the
// character, but a solid silhouette must show it alone, down to the smallest
// size a glyph is drawn.
func TestEveryDigitIsDistinctAtTheSmallestSolidGlyph(t *testing.T) {
	h := smallestDrawn
	seen := map[string]int{}
	for d := 0; d <= 9; d++ {
		key := RasterizeSolid(d, h*CellAspect, h).String()
		if other, ok := seen[key]; ok {
			t.Errorf("%d and %d render identically at %.2f rows", other, d, h)
		}
		seen[key] = d
	}
}

// The pen follows the rendering, not the height. A silhouette blurs if it is
// fattened, and a glyph drawn in copies of its own digit shows the digit only
// where coverage is full, so a thin stroke leaves it almost entirely shading.
func TestTheCharacterPenIsHeavierThanTheSilhouettePen(t *testing.T) {
	for h := smallestDrawn; h <= largestDrawn; h += 0.25 {
		ascii := strokeHalfWidthAt(h, false)
		solid := strokeHalfWidthAt(h, true)
		if ascii < solid {
			t.Fatalf("at %.2f rows the character pen is %v and the silhouette pen %v",
				h, ascii, solid)
		}
		if ascii > penMax || solid > penMax {
			t.Fatalf("at %.2f rows a pen is past penMax: %v, %v", h, ascii, solid)
		}
	}
	// At these heights the character pen is heavier, not equal by clamping.
	// Both renderings occur here: a cluster is solid, and a retired one shrinks
	// through them in characters.
	for _, h := range []float64{smallestDrawn, 3.0, 4.0, 5.0} {
		if strokeHalfWidthAt(h, false) <= strokeHalfWidthAt(h, true) {
			t.Errorf("at %.2f rows the two pens are the same", h)
		}
	}
}

// Ink may step when a digit becomes corrupt, which is a different event from
// growing.
func TestGrowthIsContinuousWithinEachRendering(t *testing.T) {
	for _, solid := range []bool{false, true} {
		prev, worst := -1.0, 0.0
		for h := 3.0; h <= 12.0; h += 0.1 {
			ink := totalCoverage(RasterizeFor(9, h*CellAspect, h, 0, solid))
			if prev >= 0 {
				if jump := math.Abs(ink-prev) / math.Max(prev, 1); jump > worst {
					worst = jump
				}
			}
			prev = ink
		}
		if worst > 0.35 {
			t.Errorf("drawn solid=%v, the largest single-step change in ink was %.0f%%",
				solid, worst*100)
		}
	}
}
