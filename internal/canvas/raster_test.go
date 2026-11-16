package canvas

import (
	"encoding/binary"
	"testing"
)

// cell reads one encoded cell back: code point, foreground, background.
func cell(t *testing.T, raster []byte, i int) [3]uint32 {
	t.Helper()
	if len(raster) < (i+1)*12 {
		t.Fatalf("raster of %d bytes has no cell %d", len(raster), i)
	}
	var c [3]uint32
	for k := range c {
		c[k] = binary.LittleEndian.Uint32(raster[i*12+k*4:])
	}
	return c
}

// Each cell becomes a code point and two 0x00RRGGBB colours, row-major, with
// an unset colour as the terminal's own.
func TestARasterCarriesEachCellsGlyphAndColours(t *testing.T) {
	b := NewBuffer(2, 2)
	b.Set(0, 0, '7', Attr{FG: RGB(0x12, 0x34, 0x56), BG: RGB(1, 2, 3)})
	b.Set(1, 0, '⣿', Attr{FG: ANSI(9)})
	b.Set(0, 1, '│', Attr{})

	r := b.AppendRaster(nil)
	if len(r) != 4*12 {
		t.Fatalf("%d bytes for four cells", len(r))
	}
	for i, want := range [][3]uint32{
		{'7', 0x123456, 0x010203},
		{'⣿', 0xff0000, rasterDefault},
		{'│', rasterDefault, rasterDefault},
		{' ', rasterDefault, rasterDefault},
	} {
		if got := cell(t, r, i); got != want {
			t.Errorf("cell %d: %#x, want %#x", i, got, want)
		}
	}
}

// A faint cell has no attribute of its own in a raster, so it is drawn dimmer.
func TestAFaintCellIsDrawnDimmer(t *testing.T) {
	b := NewBuffer(1, 1)
	b.Set(0, 0, 'x', Attr{FG: RGB(200, 100, 50), Faint: true})
	if got := cell(t, b.AppendRaster(nil), 0)[1]; got == 0xc86432 || got == rasterDefault {
		t.Errorf("a faint cell kept its full colour: %#x", got)
	}
}

// A raster takes one printable, single-width character per cell; anything else
// would make the whole frame be refused, so it is replaced.
func TestARasterNeverCarriesACharacterItCannotDraw(t *testing.T) {
	b := NewBuffer(4, 1)
	for i, r := range []rune{'\x1b', 0, '😀', '中'} {
		b.Set(i, 0, r, Attr{})
	}
	raster := b.AppendRaster(nil)
	for i := range 4 {
		if got := cell(t, raster, i)[0]; got != ' ' {
			t.Errorf("cell %d carries %U", i, got)
		}
	}
}
