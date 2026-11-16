package canvas

import (
	"encoding/binary"
	"unicode"
)

// A raster is the cell grid Claude Code's terminal draws in a plugin's pane:
// for each cell, row-major, three little-endian uint32s, the code point, the
// foreground and the background, each colour 0x00RRGGBB or rasterDefault.

// rasterDefault is the terminal's own colour in a raster.
const rasterDefault = 0x01000000

// AppendRaster appends the buffer as raster cells to dst. Bold has no place in
// a raster and is dropped; faint is drawn as a dimmer colour.
func (b *Buffer) AppendRaster(dst []byte) []byte {
	var cell [12]byte
	for i, r := range b.runes {
		a := b.attrs[i]
		binary.LittleEndian.PutUint32(cell[0:], uint32(rasterRune(r)))
		binary.LittleEndian.PutUint32(cell[4:], rasterColor(a.FG, a.Faint))
		binary.LittleEndian.PutUint32(cell[8:], rasterColor(a.BG, false))
		dst = append(dst, cell[:]...)
	}
	return dst
}

// rasterRune is r, or a space where a raster would refuse it: a control
// character, a combining mark, or anything not one cell wide in the BMP.
func rasterRune(r rune) rune {
	if r < 0x20 || r >= 0x10000 || !unicode.IsPrint(r) || unicode.Is(unicode.Mn, r) || wide(r) {
		return ' '
	}
	return r
}

// wide reports the East Asian wide and fullwidth ranges of the BMP, which take
// two cells.
func wide(r rune) bool {
	return r >= 0x1100 && r <= 0x115F || r >= 0x2E80 && r <= 0xA4CF ||
		r >= 0xAC00 && r <= 0xD7A3 || r >= 0xF900 && r <= 0xFAFF ||
		r >= 0xFE30 && r <= 0xFE4F || r >= 0xFF00 && r <= 0xFF60 ||
		r >= 0xFFE0 && r <= 0xFFE6
}

// rasterColor is c as 0x00RRGGBB, a palette index by xterm's colours, at
// about 60% when faint.
func rasterColor(c Color, faint bool) uint32 {
	var rgb uint32
	switch c & tagMask {
	case tagRGB:
		rgb = uint32(c &^ tagMask)
	case tagANSI:
		rgb = xtermColor(uint8(c))
	default:
		return rasterDefault
	}
	if faint {
		dim := func(v uint32) uint32 { return v * 3 / 5 }
		rgb = dim(rgb>>16&0xFF)<<16 | dim(rgb>>8&0xFF)<<8 | dim(rgb&0xFF)
	}
	return rgb
}

// xtermBase is the sixteen system colours as xterm sets them.
var xtermBase = [16]uint32{
	0x000000, 0xcd0000, 0x00cd00, 0xcdcd00, 0x0000ee, 0xcd00cd, 0x00cdcd, 0xe5e5e5,
	0x7f7f7f, 0xff0000, 0x00ff00, 0xffff00, 0x5c5cff, 0xff00ff, 0x00ffff, 0xffffff,
}

// xtermColor is a 256-colour palette index as 0x00RRGGBB.
func xtermColor(n uint8) uint32 {
	switch {
	case n < 16:
		return xtermBase[n]
	case n < 232:
		n -= 16
		level := func(v uint8) uint32 {
			if v == 0 {
				return 0
			}
			return uint32(55 + 40*uint32(v))
		}
		return level(n/36)<<16 | level(n/6%6)<<8 | level(n%6)
	default:
		g := uint32(8 + 10*(n-232))
		return g<<16 | g<<8 | g
	}
}
