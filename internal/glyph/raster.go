package glyph

import (
	"math"
	"strings"
	"sync"
)

// CellAspect is a glyph's width in columns per row of height. A terminal cell
// is roughly twice as tall as it is wide and digits are taller than they are
// wide, which lands around 1.2.
const CellAspect = 1.2

// rasterSamples is the supersampling grid per cell, per axis: nine samples
// give ten coverage levels, more than the density ramp can show.
const rasterSamples = 3

// Coverage is how much of each cell in a glyph's box is inked, from 0 to 1.
// Partial values at the edges antialias a fractional size.
type Coverage struct {
	W, H  int
	Cells []float64
}

// At returns a cell's coverage, or 0 outside the box.
func (c *Coverage) At(x, y int) float64 {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return 0
	}
	return c.Cells[y*c.W+x]
}

// String renders the coverage as text, for tests and debugging.
func (c *Coverage) String() string {
	var b strings.Builder
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W; x++ {
			if v := c.At(x, y); v <= 0 {
				b.WriteByte(' ')
			} else {
				b.WriteRune(RampRune(v))
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// Rasterize renders a digit into a coverage grid sized to hold a w x h box, in
// cells. Each cell's coverage is the fraction of its samples within a stroke.
// Results are cached; callers must treat them as read-only.
func Rasterize(digit int, w, h float64) *Coverage {
	return RasterizeRotated(digit, w, h, 0)
}

// RasterizeSolid renders a digit with the silhouette pen, for a glyph drawn as
// a silhouette, not in characters.
func RasterizeSolid(digit int, w, h float64) *Coverage {
	return RasterizeFor(digit, w, h, 0, true)
}

// RasterizeRotated renders a digit tilted by angle degrees about the centre of
// its box. Positive angles lean clockwise on screen.
func RasterizeRotated(digit int, w, h, angle float64) *Coverage {
	return RasterizeFor(digit, w, h, angle, false)
}

// RasterizeFor renders a digit with the silhouette pen (solid) or the pen for
// drawing in characters.
func RasterizeFor(digit int, w, h, angle float64, solid bool) *Coverage {
	if digit < 0 || digit > 9 {
		return nil
	}
	return rasterize(rune('0'+digit), w, h, angle, solid)
}

// StrokesFor is the outline of any character this package can draw: a digit
// from vector.go or a letter from letters.go.
func StrokesFor(r rune) []Stroke {
	if r >= '0' && r <= '9' {
		return DigitStrokes(int(r - '0'))
	}
	return LetterStrokes(r)
}

// RasterizeRuneFor renders any character, including letters. The digit entry
// points keep their own range check: without it RasterizeFor(12) would render
// rune('0'+12), '<', if one were ever authored.
func RasterizeRuneFor(r rune, w, h, angle float64, solid bool) *Coverage {
	return rasterize(r, w, h, angle, solid)
}

func rasterize(char rune, w, h, angle float64, solid bool) *Coverage {
	strokes := StrokesFor(char)

	// Nil is "no such character" and empty is a space; neither is rasterised,
	// or a space would cache an all-zero coverage per size.
	if len(strokes) == 0 {
		return nil
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}

	key := rasterKey{
		solid: solid,
		ch:    char,
		w:     int(math.Round(w * 4)),
		h:     int(math.Round(h * 4)),
		// Quantise the angle too, or a continuously changing tilt would miss the
		// cache on every frame.
		angle: int(math.Round(angle * 2)),
	}
	if c, ok := rasterLoad(key); ok {
		return c
	}

	if key.angle != 0 {
		strokes = rotateStrokes(strokes, float64(key.angle)/2)
	}

	// Render at the quantised size the key describes.
	qw, qh := float64(key.w)/4, float64(key.h)/4

	cw, ch := int(math.Ceil(qw)), int(math.Ceil(qh))
	cov := &Coverage{W: cw, H: ch, Cells: make([]float64, cw*ch)}

	// Centre the digit, splitting the slack from rounding up to whole cells.
	offX := (float64(cw) - qw) / 2
	offY := (float64(ch) - qh) / 2

	// The pen is in unit-box units, like the distances below, so it must not
	// be scaled by the glyph height (every digit becomes a solid block). It
	// depends only on the quantised height, so the cache key covers it.
	pen := strokeHalfWidthAt(qh, solid)
	half2 := pen * pen

	const inv = 1.0 / rasterSamples
	for cy := 0; cy < ch; cy++ {
		for cx := 0; cx < cw; cx++ {
			hits := 0
			for sy := 0; sy < rasterSamples; sy++ {
				for sx := 0; sx < rasterSamples; sx++ {
					// Sample at the centre of each subcell.
					px := float64(cx) + (float64(sx)+0.5)*inv
					py := float64(cy) + (float64(sy)+0.5)*inv

					// Into unit-box coordinates.
					ux := (px - offX) / qw
					uy := (py - offY) / qh
					if ux < -0.5 || ux > 1.5 || uy < -0.5 || uy > 1.5 {
						continue
					}

					// Measured in glyph-height units so the stroke is as thick
					// on both axes.
					if inked(strokes, ux, uy, qw/qh, half2) {
						hits++
					}
				}
			}
			cov.Cells[cy*cw+cx] = float64(hits) / (rasterSamples * rasterSamples)
		}
	}

	rasterStore(key, cov)
	return cov
}

// rotateStrokes turns an outline about the centre of the unit box. The box is
// not visually square, so x is scaled into height units before rotating and
// back afterwards; rotating raw box coordinates would shear the digit.
func rotateStrokes(strokes []Stroke, degrees float64) []Stroke {
	rad := degrees * math.Pi / 180
	sin, cos := math.Sin(rad), math.Cos(rad)

	out := make([]Stroke, len(strokes))
	for i, s := range strokes {
		r := make(Stroke, len(s))
		for j, p := range s {
			x := (p.X - 0.5) * CellAspect
			y := p.Y - 0.5
			r[j] = Point{
				X: (x*cos-y*sin)/CellAspect + 0.5,
				Y: x*sin + y*cos + 0.5,
			}
		}
		out[i] = r
	}
	return out
}

// inked reports whether a point in unit-box space falls within a stroke.
// aspect scales x into y's units, so strokes are as thick on both axes.
func inked(strokes []Stroke, ux, uy, aspect, half2 float64) bool {
	for _, s := range strokes {
		for i := 0; i+1 < len(s); i++ {
			if segDist2(s[i], s[i+1], ux, uy, aspect) <= half2 {
				return true
			}
		}
	}
	return false
}

// segDist2 is the squared distance from a point to a segment, with x scaled by
// aspect so the measurement is isotropic in glyph-height units.
func segDist2(a, b Point, px, py, aspect float64) float64 {
	ax, ay := a.X*aspect, a.Y
	bx, by := b.X*aspect, b.Y
	x, y := px*aspect, py

	dx, dy := bx-ax, by-ay
	if l2 := dx*dx + dy*dy; l2 > 0 {
		t := ((x-ax)*dx + (y-ay)*dy) / l2
		if t < 0 {
			t = 0
		} else if t > 1 {
			t = 1
		}
		ax, ay = ax+t*dx, ay+t*dy
	}
	ex, ey := x-ax, y-ay
	return ex*ex + ey*ey
}

// rasterKey keys the coverage cache. Sizes repeat across frames and between
// digits, and rasterising is the costly part of drawing a glyph.
type rasterKey struct {
	ch rune

	w, h, angle int

	// solid is which pen the glyph was drawn with.
	solid bool
}

// rasterCacheLimit bounds the cache, whose key space includes every size and
// tilt. A full cache is cleared wholesale; the working set rebuilds within a
// second or so.
const rasterCacheLimit = 8192

var (
	rasterMu    sync.RWMutex
	rasterCache = map[rasterKey]*Coverage{}
	rasterMiss  int
)

func rasterLoad(k rasterKey) (*Coverage, bool) {
	rasterMu.RLock()
	c, ok := rasterCache[k]
	rasterMu.RUnlock()
	return c, ok
}

func rasterStore(k rasterKey, c *Coverage) {
	rasterMu.Lock()
	if len(rasterCache) >= rasterCacheLimit {
		rasterCache = make(map[rasterKey]*Coverage, rasterCacheLimit)
	}
	rasterCache[k] = c
	rasterMiss++
	rasterMu.Unlock()
}

// RasterMisses reports how many coverages have been rendered, for tests of the
// cache.
func RasterMisses() int {
	rasterMu.RLock()
	defer rasterMu.RUnlock()
	return rasterMiss
}

// ResetRasterStats empties the cache and clears the miss counter. Diagnostics
// and tests only.
func ResetRasterStats() {
	rasterMu.Lock()
	rasterCache = map[rasterKey]*Coverage{}
	rasterMiss = 0
	rasterMu.Unlock()
}
