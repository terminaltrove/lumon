package canvas

import (
	"math"
	"strconv"
)

// Color is a terminal colour packed into four bytes, so an Attr is small and
// comparable with a register compare. StringWith compares Attrs once per cell,
// and a floor of sessions is bound by memory bandwidth, so do not make it a
// string.
//
// The layout is a tag in the top byte and the value in the low three:
//
//	0x00000000  unset, and the zero value, so a blank Attr still means unstyled
//	0x01RRGGBB  a true colour
//	0x020000NN  an ANSI palette index, 0 to 255
type Color uint32

const (
	tagRGB  Color = 1 << 24
	tagANSI Color = 2 << 24
	tagMask Color = 0xFF << 24
)

// RGB is a true colour.
func RGB(r, g, b uint8) Color {
	return tagRGB | Color(r)<<16 | Color(g)<<8 | Color(b)
}

// ANSI is a palette index, 0 to 255.
func ANSI(n uint8) Color { return tagANSI | Color(n) }

// Set reports whether a colour was named at all. An unset colour is drawn in
// whatever the terminal is already using.
func (c Color) Set() bool { return c != 0 }

// Parts is the red, green and blue of a true colour. ok is false for an unset
// colour or a palette index.
func (c Color) Parts() (r, g, b uint8, ok bool) {
	if c&tagMask != tagRGB {
		return 0, 0, 0, false
	}
	return uint8(c >> 16), uint8(c >> 8), uint8(c), true
}

// String is the colour in the form lipgloss takes: "#RRGGBB" for a true colour
// and a decimal index for a palette one. It allocates, so keep it out of the
// per-cell path; buildWrap's result is cached per Attr.
func (c Color) String() string {
	switch c & tagMask {
	case tagRGB:
		const hex = "0123456789ABCDEF"
		r, g, b, _ := c.Parts()
		return string([]byte{
			'#',
			hex[r>>4], hex[r&0xF],
			hex[g>>4], hex[g&0xF],
			hex[b>>4], hex[b&0xF],
		})
	case tagANSI:
		return strconv.Itoa(int(c & 0xFF))
	}
	return ""
}

// ParseColor reads the two forms a palette is authored in: "#RGB" or "#RRGGBB"
// for a true colour, and a bare number for an ANSI index. Anything else is unset.
func ParseColor(s string) Color {
	if s == "" {
		return 0
	}
	if s[0] != '#' {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 && n <= 255 {
			return ANSI(uint8(n))
		}
		return 0
	}

	digits := s[1:]
	// The short form doubles each digit, so #0AF is #00AAFF.
	if len(digits) == 3 {
		d := []byte{digits[0], digits[0], digits[1], digits[1], digits[2], digits[2]}
		digits = string(d)
	}
	if len(digits) != 6 {
		return 0
	}

	var v [3]uint8
	for i := 0; i < 3; i++ {
		hi, ok1 := unhex(digits[i*2])
		lo, ok2 := unhex(digits[i*2+1])
		if !ok1 || !ok2 {
			return 0
		}
		v[i] = hi<<4 | lo
	}
	return RGB(v[0], v[1], v[2])
}

func unhex(c byte) (uint8, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// BlendSteps, when non-zero, rounds every blend to that many steps between its
// ends. Zero, the default, blends exactly. The browser sets it: xterm caches a
// glyph per foreground and background pair, and an exact fade would upload a
// fresh glyph for nearly every cell on every frame.
var BlendSteps int

// Blend mixes two colours, t from 0 at a to 1 at b. It runs per cell per frame
// at worst and does not allocate. The ends are returned exactly. A colour with
// no channels (unset or a palette index) cannot be mixed, so the nearer end
// wins.
func Blend(a, b Color, t float64) Color {
	switch {
	case t <= 0:
		return a
	case t >= 1:
		return b
	}
	if BlendSteps > 0 {
		t = math.Round(t*float64(BlendSteps)) / float64(BlendSteps)
	}

	ar, ag, ab, ok1 := a.Parts()
	br, bg, bb, ok2 := b.Parts()
	if !ok1 || !ok2 {
		if t < 0.5 {
			return a
		}
		return b
	}

	// math.Round, not +0.5 and truncate: truncation goes toward zero, so a
	// rising and a falling blend to the same colour would differ by one.
	mix := func(x, y uint8) uint8 {
		return uint8(math.Round(float64(x) + (float64(y)-float64(x))*t))
	}
	return RGB(mix(ar, br), mix(ag, bg), mix(ab, bb))
}

// Bright is a fully saturated, full-value colour at a hue, in turns around the
// wheel from red. hue wraps. It is for the screensaver's bouncing mark, one of
// the few colours not taken from a Palette.
func Bright(hue float64) Color {
	hue -= math.Floor(hue)

	// HSV to RGB at S=V=1: a ramp around six sectors.
	h := hue * 6
	sector := int(h)
	f := h - float64(sector)
	up := uint8(f*255 + 0.5)
	down := uint8((1-f)*255 + 0.5)

	switch sector {
	case 0:
		return RGB(255, up, 0)
	case 1:
		return RGB(down, 255, 0)
	case 2:
		return RGB(0, 255, up)
	case 3:
		return RGB(0, down, 255)
	case 4:
		return RGB(up, 0, 255)
	default:
		return RGB(255, 0, down)
	}
}
