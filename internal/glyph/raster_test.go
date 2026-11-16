package glyph

import "testing"

func totalCoverage(c *Coverage) float64 {
	sum := 0.0
	for _, v := range c.Cells {
		sum += v
	}
	return sum
}

func TestRasterizeProducesABoxBigEnoughForTheDigit(t *testing.T) {
	c := Rasterize(8, 7, 5)

	if c.W < 7 || c.H < 5 {
		t.Errorf("box is %dx%d, want at least 7x5", c.W, c.H)
	}
	if len(c.Cells) != c.W*c.H {
		t.Errorf("%d cells for a %dx%d box", len(c.Cells), c.W, c.H)
	}
}

func TestCoverageIsAFraction(t *testing.T) {
	for d := 0; d <= 9; d++ {
		c := Rasterize(d, 9, 7)
		for i, v := range c.Cells {
			if v < 0 || v > 1 {
				t.Fatalf("digit %d cell %d has coverage %v, want 0..1", d, i, v)
			}
		}
	}
}

func TestEveryDigitRastersToSomething(t *testing.T) {
	for h := 3.0; h <= 12; h++ {
		for d := 0; d <= 9; d++ {
			c := Rasterize(d, h*CellAspect, h)
			if totalCoverage(c) <= 0 {
				t.Errorf("digit %d at height %v rendered blank", d, h)
			}
		}
	}
}

func TestDigitsAreDistinctAtEveryUsefulSize(t *testing.T) {
	for h := 4.0; h <= 12; h++ {
		seen := map[string]int{}
		for d := 0; d <= 9; d++ {
			c := Rasterize(d, h*CellAspect, h)
			key := c.String()
			if prev, dup := seen[key]; dup {
				t.Errorf("at height %v digits %d and %d rasterize identically", h, prev, d)
			}
			seen[key] = d
		}
	}
}

func TestInkGrowsWithSize(t *testing.T) {
	small := totalCoverage(Rasterize(9, 3*CellAspect, 3))
	mid := totalCoverage(Rasterize(9, 7*CellAspect, 7))
	large := totalCoverage(Rasterize(9, 12*CellAspect, 12))

	if !(small < mid && mid < large) {
		t.Errorf("ink did not grow with size: %v, %v, %v", small, mid, large)
	}
}

// Fractional heights must antialias, not snap to the next whole cell.
func TestFractionalSizesDifferFromWholeOnes(t *testing.T) {
	a := Rasterize(9, 6*CellAspect, 6)
	b := Rasterize(9, 6.5*CellAspect, 6.5)

	if a.String() == b.String() {
		t.Error("6.0 and 6.5 rasterize identically; fractional size is being rounded away")
	}
}

// A terminal cell is about twice as tall as it is wide, so a digit needs more
// columns than rows or it renders tall and skinny.
func TestCellAspectMakesGlyphsWiderThanTheyAreTall(t *testing.T) {
	if CellAspect <= 1 {
		t.Fatalf("CellAspect = %v, want greater than 1", CellAspect)
	}
	c := Rasterize(8, 9*CellAspect, 9)
	if c.W <= c.H {
		t.Errorf("a height-9 digit is %dx%d; want more columns than rows", c.W, c.H)
	}
}

func TestRasterizeRejectsOutOfRangeDigits(t *testing.T) {
	if got := Rasterize(10, 8, 6); got != nil {
		t.Error("Rasterize(10) returned a coverage grid")
	}
}

func TestRasterizeHandlesDegenerateSizes(t *testing.T) {
	for _, sz := range [][2]float64{{0, 0}, {-3, -3}, {0.2, 0.2}} {
		c := Rasterize(5, sz[0], sz[1])
		if c == nil {
			continue
		}
		if c.W < 1 || c.H < 1 || len(c.Cells) != c.W*c.H {
			t.Errorf("size %v produced a malformed box %dx%d", sz, c.W, c.H)
		}
	}
}

func TestRasterizingTheSameSizeTwiceGivesTheSameResult(t *testing.T) {
	a := Rasterize(4, 8.25, 6.75)
	b := Rasterize(4, 8.25, 6.75)

	if a.String() != b.String() {
		t.Error("two rasterizations of the same size differ")
	}
}

// Callers hold the result across a frame, so a cached grid must not be
// mutable from outside.
func TestCoverageAtIsBoundsChecked(t *testing.T) {
	c := Rasterize(3, 8, 6)

	for _, p := range [][2]int{{-1, 0}, {0, -1}, {c.W, 0}, {0, c.H}} {
		if got := c.At(p[0], p[1]); got != 0 {
			t.Errorf("At(%d,%d) = %v, want 0", p[0], p[1], got)
		}
	}
}

func TestCacheServesRepeatedSizes(t *testing.T) {
	ResetRasterStats()
	Rasterize(7, 9, 7)
	first := RasterMisses()

	for i := 0; i < 50; i++ {
		Rasterize(7, 9, 7)
	}

	if got := RasterMisses(); got != first {
		t.Errorf("misses went %d -> %d over 50 repeats; the cache is not serving", first, got)
	}
}

// Rotation lets a refined digit tilt as it collapses.

func TestZeroRotationMatchesTheUnrotatedGlyph(t *testing.T) {
	a := RasterizeRotated(5, 9*CellAspect, 9, 0)
	b := Rasterize(5, 9*CellAspect, 9)

	if a.String() != b.String() {
		t.Error("rotating by zero changed the glyph")
	}
}

func TestRotationChangesTheGlyph(t *testing.T) {
	upright := RasterizeRotated(4, 9*CellAspect, 9, 0)
	tilted := RasterizeRotated(4, 9*CellAspect, 9, 12)

	if upright.String() == tilted.String() {
		t.Error("a 12 degree tilt produced an identical glyph")
	}
}

func TestOppositeRotationsDiffer(t *testing.T) {
	left := RasterizeRotated(7, 9*CellAspect, 9, -10)
	right := RasterizeRotated(7, 9*CellAspect, 9, 10)

	if left.String() == right.String() {
		t.Error("tilting left and right produced the same glyph")
	}
}

func TestRotatedGlyphsStayLegible(t *testing.T) {
	for _, angle := range []float64{-14, -7, 7, 14} {
		for d := 0; d <= 9; d++ {
			c := RasterizeRotated(d, 9*CellAspect, 9, angle)
			if totalCoverage(c) <= 0 {
				t.Errorf("digit %d at %v degrees rendered blank", d, angle)
			}
			if len(c.Cells) != c.W*c.H {
				t.Errorf("digit %d at %v degrees has a malformed box", d, angle)
			}
		}
	}
}

// Otherwise a tilted glyph would be served an upright one from a previous
// frame.
func TestCacheDistinguishesAngles(t *testing.T) {
	ResetRasterStats()
	RasterizeRotated(3, 8, 6, 0)
	RasterizeRotated(3, 8, 6, 9)
	twoAngles := RasterMisses()

	RasterizeRotated(3, 8, 6, 0)
	RasterizeRotated(3, 8, 6, 9)

	if RasterMisses() != twoAngles {
		t.Error("repeating the same two angles missed the cache")
	}
	if twoAngles < 2 {
		t.Errorf("two distinct angles produced %d misses, want 2", twoAngles)
	}
}

// The sweep goes to twenty degrees, past lumon.MaxCollapseTilt (fourteen). It
// is a number here because glyph must not import what it draws for, and the
// rasteriser should stay gradual past today's largest angle.
func TestRotationIsGradual(t *testing.T) {
	const past = 20.0

	prev := -1.0
	for angle := 0.0; angle <= past; angle += 1 {
		ink := totalCoverage(RasterizeRotated(8, 9*CellAspect, 9, angle))
		if prev >= 0 {
			if change := (ink - prev) / prev; change > 0.25 || change < -0.25 {
				t.Errorf("ink jumped %.0f%% between %v and %v degrees", change*100, angle-1, angle)
			}
		}
		prev = ink
	}
}

// The rasteriser takes a rune, and the digit callers pass an int. A mistake in
// the wrapper between them would go unnoticed: a subtly different coverage, or
// nil where there should be a shape.
func TestTheRasterisersDigitEntryPointsAreUnchanged(t *testing.T) {
	for d := 0; d <= 9; d++ {
		for _, h := range []float64{2.2, 4.0, 5.5, 11.0} {
			for _, solid := range []bool{false, true} {
				want := RasterizeFor(d, h*CellAspect, h, 0, solid)
				got := RasterizeRuneFor(rune('0'+d), h*CellAspect, h, 0, solid)
				if want == nil || got == nil {
					t.Fatalf("digit %d at %.1f rows: int gave %v, rune gave %v", d, h, want, got)
				}
				if want.W != got.W || want.H != got.H {
					t.Fatalf("digit %d at %.1f rows: %dx%d by int, %dx%d by rune",
						d, h, want.W, want.H, got.W, got.H)
				}
				for i := range want.Cells {
					if want.Cells[i] != got.Cells[i] {
						t.Fatalf("digit %d at %.1f rows: cell %d is %v by int and %v by rune",
							d, h, i, want.Cells[i], got.Cells[i])
					}
				}
			}
		}
	}

	// Out of range is nil by its own rule: DigitStrokes(12) is nil because
	// twelve is not a digit, while rune('0'+12) is '<', nil only because no
	// less-than is authored.
	for _, d := range []int{-8, -1, 10, 12, 99} {
		if RasterizeFor(d, 5, 4, 0, false) != nil {
			t.Errorf("RasterizeFor(%d) drew something", d)
		}
	}
}

// Every character the founder's words need has a shape. set() skips what it
// cannot draw, so a missing shape would drop a letter from a passage.
func TestEveryAuthoredLetterRasterises(t *testing.T) {
	for _, r := range "PRAISEKBCDFGHLMNOTUVWYZ.," {
		cov := RasterizeRuneFor(r, 4*CellAspect, 4, 0, true)
		if cov == nil {
			t.Errorf("%q has an outline but rasterises to nothing", r)
			continue
		}
		ink := 0.0
		for _, c := range cov.Cells {
			ink += c
		}
		if ink <= 0 {
			t.Errorf("%q rasterised to a box with no ink in it", r)
		}
	}

	// A character nobody authored is nil, not blank. J, Q and X are absent
	// because nothing set large needs them.
	for _, r := range []rune{'J', 'Q', 'X', '!'} {
		if RasterizeRuneFor(r, 4*CellAspect, 4, 0, true) != nil {
			t.Errorf("%q is not in the alphabet but drew something", r)
		}
	}
}

// LetterStrokes(' ') returns an empty slice, not nil, which is right for text
// layout. Rasterising it would cache an all-zero coverage per size.
func TestASpaceRasterisesToNothingRatherThanToBlank(t *testing.T) {
	if got := LetterStrokes(' '); got == nil {
		t.Fatal("LetterStrokes(' ') is nil; this test is about it being empty")
	}
	if cov := RasterizeRuneFor(' ', 4*CellAspect, 4, 0, true); cov != nil {
		t.Errorf("a space rasterised to a %dx%d coverage", cov.W, cov.H)
	}
}

// The cache is keyed on the character, the quantised size, the angle and the
// pen, so two dozen letters at one height are two dozen entries. Animating a
// letter's scale or tilt would make the key space unbounded.
func TestAnAlphabetCostsOneCacheEntryPerCharacterAndSize(t *testing.T) {
	const text = "PRAISEKBCDFGHLMNOTUVWYZ"

	ResetRasterStats()
	for pass := 0; pass < 20; pass++ {
		for _, r := range text {
			RasterizeRuneFor(r, 4*CellAspect, 4, 0, true)
		}
	}
	if got := RasterMisses(); got > len(text) {
		t.Errorf("twenty passes over %d characters cost %d misses; the cache is not "+
			"holding them", len(text), got)
	}
}
