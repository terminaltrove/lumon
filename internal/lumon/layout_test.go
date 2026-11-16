package lumon

import (
	"math"
	"testing"
)

// Layout is the one mapping between screen and grid, used by both the renderer
// and the mouse. These tests pin it as a round trip.

func TestScreenOfAndCellAtAreInverses(t *testing.T) {
	l := NewLayout(120, 40)
	l.ViewportX, l.ViewportY = 7, 13

	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			sx, sy := l.ScreenOf(gx, gy)
			bx, by, ok := l.CellAt(sx, sy)
			if !ok {
				t.Fatalf("grid (%d,%d) -> screen (%d,%d) did not map back", gx, gy, sx, sy)
			}
			if bx != gx || by != gy {
				t.Fatalf("grid (%d,%d) -> screen (%d,%d) -> grid (%d,%d)", gx, gy, sx, sy, bx, by)
			}
		}
	}
}

func TestCellAtCoversTheFullWidthOfACell(t *testing.T) {
	l := NewLayout(120, 40)

	sx, sy := l.ScreenOf(l.ViewportX, l.ViewportY)
	left := sx - CellWidth/2
	for dx := 0; dx < CellWidth; dx++ {
		gx, gy, ok := l.CellAt(left+dx, sy)
		if !ok || gx != l.ViewportX || gy != l.ViewportY {
			t.Errorf("screen x=%d mapped to (%d,%d,ok=%v), want the first cell", left+dx, gx, gy, ok)
		}
	}
}

func TestCellAtRejectsRowsOutsideTheGrid(t *testing.T) {
	l := NewLayout(120, 40)

	// The header sits above the grid and the bins below it.
	if _, _, ok := l.CellAt(l.GridOriginX, l.GridTop-1); ok {
		t.Error("row above the grid was accepted")
	}
	if _, _, ok := l.CellAt(l.GridOriginX, l.GridTop+l.ViewportH*RowPitch); ok {
		t.Error("row below the grid was accepted")
	}
}

func TestCellAtRejectsColumnsOutsideTheGrid(t *testing.T) {
	l := NewLayout(120, 40)

	if _, _, ok := l.CellAt(l.GridOriginX-1, l.GridTop); ok {
		t.Error("column left of the grid was accepted")
	}
	right := l.GridOriginX + l.ViewportW*CellWidth
	if _, _, ok := l.CellAt(right, l.GridTop); ok {
		t.Error("column right of the grid was accepted")
	}
}

// Chrome above the grid is a header box plus a double rule.
func TestGridTopAccountsForTheChromeAboveIt(t *testing.T) {
	l := NewLayout(120, 40)

	// Uses the layout's own header height: the bar takes the taller form where there
	// are rows for the Lumon mark.
	if l.GridTop != l.HeaderH+TopRuleHeight {
		t.Errorf("GridTop = %d, want %d (header %d + rule %d)",
			l.GridTop, l.HeaderH+TopRuleHeight, l.HeaderH, TopRuleHeight)
	}
	if l.GridTop == 2 {
		t.Error("GridTop is 2, the value v2 hardcoded in its mouse handlers")
	}
}

func TestBinHitTestingUsesTheSameLayout(t *testing.T) {
	l := NewLayout(120, 40)

	for i := 0; i < BinCount; i++ {
		got, ok := l.BinAt(l.BinCenterX[i], l.BinTop+1)
		if !ok {
			t.Fatalf("bin %d centre did not hit a bin", i)
		}
		if got != i {
			t.Errorf("bin %d centre hit bin %d", i, got)
		}
	}
}

func TestBinAtRejectsRowsAboveTheBins(t *testing.T) {
	l := NewLayout(120, 40)

	if _, ok := l.BinAt(l.BinCenterX[0], l.GridTop); ok {
		t.Error("a grid row was accepted as a bin click")
	}
}

func TestViewportIsClampedToTheGrid(t *testing.T) {
	l := NewLayout(4000, 3000) // far larger than the 100x100 grid

	if l.ViewportW > GridSize || l.ViewportH > GridSize {
		t.Errorf("viewport %dx%d exceeds the %d grid", l.ViewportW, l.ViewportH, GridSize)
	}
}

func TestTinyTerminalStillProducesAUsableViewport(t *testing.T) {
	l := NewLayout(10, 6)

	if l.ViewportW < 1 || l.ViewportH < 1 {
		t.Errorf("viewport %dx%d, want at least 1x1", l.ViewportW, l.ViewportH)
	}
}

func TestScrollKeepsTheViewportInsideTheGrid(t *testing.T) {
	l := NewLayout(120, 40)

	l.ScrollTo(-50, -50)
	if l.ViewportX != 0 || l.ViewportY != 0 {
		t.Errorf("after scrolling negative: (%d,%d), want (0,0)", l.ViewportX, l.ViewportY)
	}

	l.ScrollTo(9999, 9999)
	if l.ViewportX+l.ViewportW > GridSize || l.ViewportY+l.ViewportH > GridSize {
		t.Errorf("after scrolling past the end: viewport %d+%d, %d+%d exceeds %d",
			l.ViewportX, l.ViewportW, l.ViewportY, l.ViewportH, GridSize)
	}
}

// The span divided five ways leaves a remainder (114 columns is 22 each with 4
// to spare); the extra columns go to the leftmost bins so no ragged gap is left
// on the right.
func TestBinsSpanTheWidthTheyAreGiven(t *testing.T) {
	for _, width := range []int{78, 80, 100, 120, 160} {
		l := NewLayout(width, 40)

		total := 0
		for i := 0; i < BinCount; i++ {
			total += l.BinWidth(i)
		}
		if total > l.ViewportW*CellWidth {
			t.Errorf("width %d: bins total %d columns against a board of %d",
				width, total, l.ViewportW*CellWidth)
		}
		// The row is centred on the board, so the rest of the board is split evenly
		// between its two ends.
		lead := l.BinOriginX(0) - l.GridOriginX
		tail := l.GridOriginX + l.ViewportW*CellWidth - (l.BinOriginX(BinCount-1) + l.BinWidth(BinCount-1))
		if lead != tail {
			t.Errorf("width %d: the bins sit %d columns inside the board on the left and %d on the right",
				width, lead, tail)
		}
	}
}

// The ends line up with the grid's leftmost and rightmost columns, with a space
// either side. The header's box and rules run edge to edge, so the board is the
// only edge on this screen for the bins to match.
func TestTheBinsStandUnderTheBoard(t *testing.T) {
	for _, width := range []int{90, 100, 120, 160, 200} {
		m := ready()
		m.Resize(width, 40)
		buf := drawTo(m)
		l := m.Layout

		first, last := l.BinOriginX(0), l.BinOriginX(BinCount-1)+l.BinWidth(BinCount-1)

		// binInsetCells grid cells inside the board at each end.
		if want := l.GridOriginX + binInsetCells*CellWidth; first != want {
			t.Errorf("at %d columns the bins start at %d, want %d cells inside the board at %d",
				width, first, binInsetCells, want)
		}

		// The same space at each end, and enough of it to see.
		if right := l.Width - last; first != right {
			t.Errorf("at %d columns the bins are inset %d on the left and %d on the right",
				width, first, right)
		}
		if first < binInsetCells*CellWidth {
			t.Errorf("at %d columns the bins are only %d columns off the wall", width, first)
		}

		// No bin is squeezed past saying what it holds.
		for i := 0; i < BinCount; i++ {
			if got := l.BinWidth(i); got < binMinWidth {
				t.Errorf("at %d columns bin %d is %d columns wide, below the %d it needs",
					width, i+1, got, binMinWidth)
			}
		}

		// Nothing is drawn in either margin on any of the bins' rows.
		for y := l.BinTop; y < l.BinTop+BinsHeight; y++ {
			row := []rune(buf.Row(y))
			for _, x := range []int{0, first - 1, last, l.Width - 1} {
				if x >= 0 && x < len(row) && row[x] != ' ' {
					t.Errorf("at %d columns something is drawn in the bin margin at (%d,%d): %q",
						width, x, y, row[x])
				}
			}
		}
	}
}

// A bin that cannot say "0%  0/55" is worse than one without a margin.
func TestTheBinInsetGivesWayBeforeTheBinsDo(t *testing.T) {
	for _, width := range []int{60, 70, 80, 88} {
		l := NewLayout(width, 40)
		for i := 0; i < BinCount; i++ {
			if got := l.BinWidth(i); got < binMinWidth && l.BinOriginX(0) > l.GridOriginX {
				t.Errorf("at %d columns bin %d is %d columns wide and the inset was kept anyway",
					width, i+1, got)
			}
		}
		if l.BinOriginX(0) < l.GridOriginX {
			t.Errorf("at %d columns the bins start at %d, left of the board at %d",
				width, l.BinOriginX(0), l.GridOriginX)
		}
	}
}

// A bin one column wide is worse than a bin against the wall.
func TestTheBinMarginGivesWayOnANarrowTerminal(t *testing.T) {
	l := NewLayout(2*BinCount+4, 40)
	if got := l.BinOriginX(0); got != 0 {
		t.Errorf("a very narrow terminal still insets its bins to column %d", got)
	}
}

func TestBinWidthsDifferByAtMostOne(t *testing.T) {
	l := NewLayout(78, 40)

	min, max := l.BinWidth(0), l.BinWidth(0)
	for i := 1; i < BinCount; i++ {
		w := l.BinWidth(i)
		if w < min {
			min = w
		}
		if w > max {
			max = w
		}
	}
	if max-min > 1 {
		t.Errorf("bin widths range %d..%d, want them within one column of each other", min, max)
	}
}

func TestBinOriginsTileWithoutGapsOrOverlap(t *testing.T) {
	l := NewLayout(78, 40)

	want := l.BinOriginX(0)
	for i := 0; i < BinCount; i++ {
		if got := l.BinOriginX(i); got != want {
			t.Errorf("bin %d starts at %d, want %d", i, got, want)
		}
		want += l.BinWidth(i)
	}
}

// Every column between the margins hits a bin, including the remainder columns,
// and the margins hit none, since nothing is drawn there.
func TestBinAtCoversEveryColumn(t *testing.T) {
	l := NewLayout(78, 40)

	first := l.BinOriginX(0)
	last := l.BinOriginX(BinCount-1) + l.BinWidth(BinCount-1)
	for _, x := range []int{0, first - 1, last, l.Width - 1} {
		if _, ok := l.BinAt(x, l.BinTop); ok {
			t.Errorf("column %d is in the margin and hit a bin", x)
		}
	}

	for x := first; x < last; x++ {
		idx, ok := l.BinAt(x, l.BinTop)
		if !ok {
			t.Fatalf("column %d of the bin row hit no bin", x)
		}
		if x < l.BinOriginX(idx) || x >= l.BinOriginX(idx)+l.BinWidth(idx) {
			t.Fatalf("column %d mapped to bin %d, which spans %d..%d",
				x, idx, l.BinOriginX(idx), l.BinOriginX(idx)+l.BinWidth(idx))
		}
	}
}

func TestBinCentersLieInsideTheirBin(t *testing.T) {
	l := NewLayout(78, 40)

	for i := 0; i < BinCount; i++ {
		c := l.BinCenterX[i]
		if c < l.BinOriginX(i) || c >= l.BinOriginX(i)+l.BinWidth(i) {
			t.Errorf("bin %d centre %d is outside %d..%d",
				i, c, l.BinOriginX(i), l.BinOriginX(i)+l.BinWidth(i))
		}
	}
}

// Numbers sit RowPitch screen rows apart and CellWidth columns apart, so
// enlarged glyphs have room.
func TestConsecutiveRowsArePitchApart(t *testing.T) {
	l := NewLayout(120, 40)

	_, y0 := l.ScreenOf(l.ViewportX, l.ViewportY)
	_, y1 := l.ScreenOf(l.ViewportX, l.ViewportY+1)

	if y1-y0 != RowPitch {
		t.Errorf("consecutive grid rows are %d screen rows apart, want %d", y1-y0, RowPitch)
	}
}

func TestGridIsSpacedOut(t *testing.T) {
	if RowPitch < 2 {
		t.Errorf("RowPitch = %d, want the grid spaced out vertically", RowPitch)
	}
	if CellWidth < 6 {
		t.Errorf("CellWidth = %d, want the grid spaced out horizontally", CellWidth)
	}
}

// Otherwise half the grid is dead.
func TestClickingABlankRowSelectsTheNumberAbove(t *testing.T) {
	l := NewLayout(120, 40)

	sx, sy := l.ScreenOf(l.ViewportX+2, l.ViewportY+3)
	for d := 1; d < RowPitch; d++ {
		gx, gy, ok := l.CellAt(sx, sy+d)
		if !ok {
			t.Fatalf("blank row %d below a number is not clickable", d)
		}
		if gx != l.ViewportX+2 || gy != l.ViewportY+3 {
			t.Errorf("blank row %d selected (%d,%d), want the number above at (%d,%d)",
				d, gx, gy, l.ViewportX+2, l.ViewportY+3)
		}
	}
}

func TestViewportHeightAccountsForThePitch(t *testing.T) {
	l := NewLayout(120, 40)

	used := l.ViewportH * RowPitch
	available := l.Height - l.HeaderH - TopRuleHeight - RuleHeight - BinsHeight - FooterHeight
	if used > available {
		t.Errorf("viewport needs %d rows but only %d are available", used, available)
	}
}

func TestCellAtRejectsRowsBelowTheSpacedGrid(t *testing.T) {
	l := NewLayout(120, 40)

	if _, _, ok := l.CellAt(l.GridOriginX, l.GridTop+l.ViewportH*RowPitch); ok {
		t.Error("a row past the bottom of the spaced grid was accepted")
	}
}

// The bar takes four extra rows for the Lumon mark, two grid rows. A corrupt
// cluster is drawn at MaxGlyphHeight, eleven screen rows and so six grid rows
// at the default pitch, and a seventh keeps it off the rule; the threshold is
// derived from that.
func TestTheTallBarNeverCostsTheBoardRoomForACluster(t *testing.T) {
	if want := int(math.Ceil(MaxGlyphHeight/RowPitch)) + 1; clusterRows < want {
		t.Errorf("clusterRows is %d, but a %v-row glyph needs %d grid rows and one "+
			"to spare", clusterRows, MaxGlyphHeight, want)
	}
	for h := 4; h <= 80; h++ {
		l := NewLayout(120, h)
		if l.HeaderH < HeaderTall {
			continue
		}
		if l.ViewportH < clusterRows {
			t.Fatalf("a %d-row terminal took the tall bar and left a %d-row viewport;"+
				" a cluster needs %d", h, l.ViewportH, clusterRows)
		}
	}
}

// Two forms: the plain strip, and the one with the mark. The heights are
// asserted because any taller form costs grid rows.
func TestTheBarTakesTheShortFormOnASmallTerminal(t *testing.T) {
	for _, at := range [][2]int{{40, 20}, {40, 60}, {200, 20}} {
		if l := NewLayout(at[0], at[1]); l.HeaderH != HeaderHeight {
			t.Errorf("a %dx%d terminal took a %d-row header", at[0], at[1], l.HeaderH)
		}
	}

	if l := NewLayout(200, 60); l.HeaderH != HeaderTall {
		t.Errorf("a 200x60 terminal took a %d-row header, want the tall %d", l.HeaderH, HeaderTall)
	}
}
