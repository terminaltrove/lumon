package lumon

import "testing"

// Magnify state is keyed by grid cell, so growth does not depend on where the
// viewport is.

func targetsFor(id int, h float64) map[int]float64 { return map[int]float64{id: h} }

func runMagnify(g *Magnify, targets map[int]float64, frames int) {
	for i := 0; i < frames; i++ {
		g.Update(1.0/60, targets)
	}
}

func TestUnknownCellsAreUnmagnified(t *testing.T) {
	g := NewMagnify()

	if got := g.Height(3, 4); got != PlainHeight {
		t.Errorf("an untracked cell is %v, want %v", got, PlainHeight)
	}
}

// magnifySettle is enough frames for a cell at the default rate to arrive. A
// critically damped physics.Scalar is within 1% of its target after about
// 6.6/omega seconds, so the count is derived from magnifyFrequency and a literal
// would stop being enough when the rate changes.
func magnifySettle() int {
	secs := 8 / magnifyFrequency
	return int(60 * secs)
}

func TestATargetedCellGrows(t *testing.T) {
	g := NewMagnify()
	id := GridIndex(3, 4)

	runMagnify(g, targetsFor(id, MaxGlyphHeight), magnifySettle())

	if got := g.Height(3, 4); got < MaxGlyphHeight-0.5 {
		t.Errorf("targeted cell reached %v, want close to %v", got, MaxGlyphHeight)
	}
}

func TestUntargetedCellsNeverGrow(t *testing.T) {
	g := NewMagnify()
	runMagnify(g, targetsFor(GridIndex(3, 4), MaxGlyphHeight), 900)

	if got := g.Height(4, 4); got != PlainHeight {
		t.Errorf("a neighbouring cell grew to %v", got)
	}
}

func TestGrowthSurvivesScrolling(t *testing.T) {
	l := NewLayout(120, 40)
	g := NewMagnify()
	id := GridIndex(l.ViewportX+3, l.ViewportY+3)
	runMagnify(g, targetsFor(id, MaxGlyphHeight), 900)
	before := g.Height(id%GridSize, id/GridSize)

	l.ScrollTo(l.ViewportX+7, l.ViewportY+5)

	if got := g.Height(id%GridSize, id/GridSize); got != before {
		t.Errorf("height went %v -> %v after scrolling", before, got)
	}
}

func TestGrowthSurvivesLeavingAndReenteringTheViewport(t *testing.T) {
	l := NewLayout(120, 40)
	g := NewMagnify()
	id := GridIndex(l.ViewportX+3, l.ViewportY+3)
	targets := targetsFor(id, MaxGlyphHeight)
	runMagnify(g, targets, 900)
	before := g.Height(id%GridSize, id/GridSize)

	l.ScrollTo(l.ViewportX+60, l.ViewportY+40) // well off screen
	runMagnify(g, targets, 60)
	l.ScrollTo(0, 0)

	if got := g.Height(id%GridSize, id/GridSize); got < before-0.01 {
		t.Errorf("height went %v -> %v after leaving and re-entering", before, got)
	}
}

func TestDroppingATargetShrinksAndForgetsTheCell(t *testing.T) {
	g := NewMagnify()
	id := GridIndex(3, 4)
	runMagnify(g, targetsFor(id, MaxGlyphHeight), magnifySettle())

	runMagnify(g, map[int]float64{}, 2*magnifySettle())

	if got := g.Height(3, 4); got != PlainHeight {
		t.Errorf("height is %v after the target went away, want %v", got, PlainHeight)
	}
	if g.Len() != 0 {
		t.Errorf("%d cells still tracked", g.Len())
	}
}

// The candidate set stays small however big the grid is.
func TestOnlyGrowingCellsAreTracked(t *testing.T) {
	g := NewMagnify()
	runMagnify(g, map[int]float64{
		GridIndex(1, 1): MaxGlyphHeight,
		GridIndex(2, 2): MaxGlyphHeight,
	}, 300)

	if got := g.Len(); got != 2 {
		t.Errorf("tracking %d cells, want 2", got)
	}
}

func TestGrowthIsSlow(t *testing.T) {
	g := NewMagnify()
	id := GridIndex(3, 4)

	runMagnify(g, targetsFor(id, MaxGlyphHeight), 12)
	if got := g.Height(3, 4); got > MinGlyphHeight {
		t.Errorf("reached %v within a fifth of a second; growth should be watchable", got)
	}
}

// A collapsing digit has to shrink faster than it grew.
func TestSetFrequencyChangesHowFastACellMoves(t *testing.T) {
	slow, fast := NewMagnify(), NewMagnify()
	id := GridIndex(3, 4)
	runMagnify(slow, targetsFor(id, MaxGlyphHeight), 900)
	runMagnify(fast, targetsFor(id, MaxGlyphHeight), 900)

	fast.SetFrequency(id, magnifyFrequency*6)
	runMagnify(slow, targetsFor(id, PlainHeight), 30)
	runMagnify(fast, targetsFor(id, PlainHeight), 30)

	if fast.Height(3, 4) >= slow.Height(3, 4) {
		t.Errorf("raised frequency did not shrink faster: fast %v, slow %v",
			fast.Height(3, 4), slow.Height(3, 4))
	}
}

func TestSetFrequencyOnAnUnknownCellIsHarmless(t *testing.T) {
	g := NewMagnify()
	g.SetFrequency(GridIndex(9, 9), 5)

	if got := g.Height(9, 9); got != PlainHeight {
		t.Errorf("height is %v, want %v", got, PlainHeight)
	}
}

func TestDropResetsACellAtOnce(t *testing.T) {
	g := NewMagnify()
	id := GridIndex(3, 4)
	runMagnify(g, targetsFor(id, MaxGlyphHeight), 900)

	g.Drop(id)

	if got := g.Height(3, 4); got != PlainHeight {
		t.Errorf("dropped cell is %v, want %v", got, PlainHeight)
	}
}

func TestHeightIsBounded(t *testing.T) {
	g := NewMagnify()
	id := GridIndex(3, 4)

	for i := 0; i < 1800; i++ {
		g.Update(1.0/60, targetsFor(id, MaxGlyphHeight*4))
		if h := g.Height(3, 4); h > MaxGlyphHeight+1e-6 {
			t.Fatalf("reached %v, above the %v cap", h, MaxGlyphHeight)
		}
	}
}

func TestMagnifyIsDeterministic(t *testing.T) {
	run := func() float64 {
		g := NewMagnify()
		runMagnify(g, targetsFor(GridIndex(3, 4), MaxGlyphHeight), 137)
		return g.Height(3, 4)
	}
	if a, b := run(), run(); a != b {
		t.Errorf("%v vs %v", a, b)
	}
}
