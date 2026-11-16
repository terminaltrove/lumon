package canvas

import "testing"

// Braille cells encode a 2x4 dot grid in the low 8 bits of U+2800..U+28FF.
func TestEmptyBrailleCellIsTheBlankBraillePattern(t *testing.T) {
	b := NewBraille(2, 1) // 2x1 cells

	if got := b.Rune(0, 0); got != '⠀' {
		t.Errorf("empty cell = %q (U+%04X), want U+2800", got, got)
	}
}

func TestBrailleReportsItsDotResolution(t *testing.T) {
	b := NewBraille(10, 4) // cells

	if w, h := b.DotSize(); w != 20 || h != 16 {
		t.Errorf("DotSize = %dx%d, want 20x16 (2x4 dots per cell)", w, h)
	}
}

// The dot-to-bit mapping is fixed by Unicode: dots 1-6 are the historical 3x2
// layout, and dots 7-8 were appended below for the 8-dot extension. That makes
// the bit order for the left column 0,1,2,6 top to bottom, not 0,1,2,3.
func TestBrailleDotBitsFollowTheUnicodeLayout(t *testing.T) {
	cases := []struct {
		x, y int
		want rune
	}{
		{0, 0, '⠁'}, // dot 1, bit 0
		{0, 1, '⠂'}, // dot 2, bit 1
		{0, 2, '⠄'}, // dot 3, bit 2
		{1, 0, '⠈'}, // dot 4, bit 3
		{1, 1, '⠐'}, // dot 5, bit 4
		{1, 2, '⠠'}, // dot 6, bit 5
		{0, 3, '⡀'}, // dot 7, bit 6
		{1, 3, '⢀'}, // dot 8, bit 7
	}

	for _, c := range cases {
		b := NewBraille(1, 1)
		b.Set(c.x, c.y)
		if got := b.Rune(0, 0); got != c.want {
			t.Errorf("Set(%d,%d) = U+%04X, want U+%04X", c.x, c.y, got, c.want)
		}
	}
}

func TestBrailleAccumulatesDotsInOneCell(t *testing.T) {
	b := NewBraille(1, 1)

	b.Set(0, 0) // bit 0
	b.Set(1, 3) // bit 7

	if got := b.Rune(0, 0); got != '⢁' {
		t.Errorf("two dots = U+%04X, want U+2881", got)
	}
}

func TestBrailleAllDotsSetIsTheFullCell(t *testing.T) {
	b := NewBraille(1, 1)

	for x := 0; x < 2; x++ {
		for y := 0; y < 4; y++ {
			b.Set(x, y)
		}
	}

	if got := b.Rune(0, 0); got != '⣿' {
		t.Errorf("full cell = U+%04X, want U+28FF", got)
	}
}

func TestBrailleSetsDotsInTheCorrectCell(t *testing.T) {
	b := NewBraille(3, 2)

	b.Set(4, 5) // cell (2,1), dot (0,1) within it

	if got := b.Rune(2, 1); got != '⠂' {
		t.Errorf("cell (2,1) = U+%04X, want U+2802", got)
	}
	if got := b.Rune(0, 0); got != '⠀' {
		t.Errorf("cell (0,0) = U+%04X, want it untouched", got)
	}
}

// Physics routinely puts particles off screen, so plotting must clip, not
// panic.
func TestBrailleIgnoresOutOfBoundsDots(t *testing.T) {
	b := NewBraille(2, 1)

	b.Set(-1, 0)
	b.Set(0, -1)
	b.Set(999, 0)
	b.Set(0, 999)

	for x := 0; x < 2; x++ {
		if got := b.Rune(x, 0); got != '⠀' {
			t.Errorf("cell %d = U+%04X, want it untouched", x, got)
		}
	}
}

func TestPlotMapsContinuousCoordinatesToDots(t *testing.T) {
	b := NewBraille(2, 1)

	b.Plot(0.9, 0.9) // truncates to dot (0,0)
	b.Plot(1.2, 3.7) // truncates to dot (1,3)

	if got := b.Rune(0, 0); got != '⢁' {
		t.Errorf("cell = U+%04X, want U+2881", got)
	}
}

func TestBrailleClearResetsEveryCell(t *testing.T) {
	b := NewBraille(2, 2)
	b.Set(0, 0)
	b.Set(3, 7)

	b.Clear()

	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			if got := b.Rune(x, y); got != '⠀' {
				t.Errorf("cell (%d,%d) = U+%04X after Clear, want U+2800", x, y, got)
			}
		}
	}
}
