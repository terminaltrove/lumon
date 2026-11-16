// Package canvas holds lumon's rendering primitives: a flat cell buffer the
// screen composites into, and a braille layer that gives it 2x4 sub-cell
// resolution (Bubble Tea has no equivalent of ratatui's braille Canvas).
package canvas

// Braille dot geometry. Each terminal cell holds a 2x4 grid of dots encoded in
// the low byte of a U+28xx code point.
const (
	DotsX = 2
	DotsY = 4

	// brailleBase is U+2800, the blank braille pattern. Setting bit n of the low
	// byte lights dot n+1.
	brailleBase = 0x2800
)

// BrailleBlank is U+2800, the pattern with no dots lit. BlitBraille writes
// blanks too, so a caller compositing over existing content tests against this
// to blit only the cells it marked.
const BrailleBlank = rune(brailleBase)

// dotBit maps a dot's (x, y) position within a cell to its bit in the code
// point. It is not row-major: Unicode kept the six-dot layout (dots 1-6, bits
// 0-5) and added the fourth row as dots 7-8 (bits 6-7).
var dotBit = [DotsX][DotsY]uint8{
	{0, 1, 2, 6}, // left column: dots 1,2,3,7
	{3, 4, 5, 7}, // right column: dots 4,5,6,8
}

// DotBit is which bit of a cell's low byte lights the dot at (x, y) within it,
// for reading braille text (such as the Lumon mark) back into dots. Out of
// range returns 0, which is dot one, so callers must stay in range.
func DotBit(x, y int) uint8 {
	if x < 0 || y < 0 || x >= DotsX || y >= DotsY {
		return 0
	}
	return dotBit[x][y]
}

// Braille is a monochrome dot bitmap addressed in dots and read out in cells.
type Braille struct {
	cellW, cellH int
	masks        []uint8
}

// NewBraille returns a canvas covering cellW x cellH terminal cells, giving
// cellW*2 by cellH*4 addressable dots.
func NewBraille(cellW, cellH int) *Braille {
	if cellW < 0 {
		cellW = 0
	}
	if cellH < 0 {
		cellH = 0
	}
	return &Braille{
		cellW: cellW,
		cellH: cellH,
		masks: make([]uint8, cellW*cellH),
	}
}

// CellSize returns the canvas size in terminal cells.
func (b *Braille) CellSize() (w, h int) { return b.cellW, b.cellH }

// DotSize returns the canvas size in addressable dots.
func (b *Braille) DotSize() (w, h int) { return b.cellW * DotsX, b.cellH * DotsY }

// Set lights the dot at integer dot coordinates. Out-of-bounds dots are
// dropped.
func (b *Braille) Set(x, y int) {
	w, h := b.DotSize()
	if x < 0 || y < 0 || x >= w || y >= h {
		return
	}
	cx, cy := x/DotsX, y/DotsY
	b.masks[cy*b.cellW+cx] |= 1 << dotBit[x%DotsX][y%DotsY]
}

// Plot lights the dot containing a continuous dot coordinate.
func (b *Braille) Plot(x, y float64) {
	if x < 0 || y < 0 {
		return
	}
	b.Set(int(x), int(y))
}

// Rune returns the braille character for a cell, U+2800 when no dot is lit.
func (b *Braille) Rune(cx, cy int) rune {
	if cx < 0 || cy < 0 || cx >= b.cellW || cy >= b.cellH {
		return brailleBase
	}
	return rune(brailleBase + int(b.masks[cy*b.cellW+cx]))
}

// Clear turns off every dot, reusing the backing array.
func (b *Braille) Clear() {
	for i := range b.masks {
		b.masks[i] = 0
	}
}
