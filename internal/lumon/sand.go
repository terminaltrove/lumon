package lumon

import (
	"github.com/terminaltrove/lumon/internal/theme"
	"math/rand"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// SAND: they settle in heaps, like grain.
//
// A grain is one cell, so this is a cellular automaton: each grain tries to
// move down, then diagonally down, which gives heaps an angle of repose. The
// Verlet solver DESCENT uses would cost far more and leave grains resting at
// fractions of a cell.

const (
	// sandRate is how many times a second the grains move. A grain moves one
	// cell per update, so at the frame rate sand would fall as a sheet.
	sandRate = 26.0

	// pourRadius is how wide the stream is when the pointer pours, in cells.
	pourRadius = 2
)

// Sand is a grid of grains.
type Sand struct {
	// x and y are where the box sits on screen; w and h are its size.
	x, y  int
	w, h  int
	grain []rune // 0 for empty
	rng   *rand.Rand

	// carry accumulates elapsed time into whole updates, as Clock does for the
	// simulation.
	carry float64

	// pouring is where the pointer is holding the stream, or off.
	pourX, pourY int
	pouring      bool
}

// NewSand pours the digits currently on screen into a box.
func NewSand(m *Model, panel Panel, rng *rand.Rand) *Sand {
	l := m.Layout
	s := &Sand{x: panel.X, y: panel.Y, w: panel.W, h: panel.H, rng: rng}
	if s.w < 1 {
		s.w = 1
	}
	if s.h < 1 {
		s.h = 1
	}
	s.grain = make([]rune, s.w*s.h)

	// The grains start where the grid drew them. With the grid's column
	// spacing they land as separate towers; pouring builds dunes.
	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			n := m.Grid.At(gx, gy)
			if n == nil {
				continue
			}
			sx, sy := l.ScreenOf(gx, gy)
			if !panel.Contains(sx, sy) {
				continue
			}
			s.grain[(sy-s.y)*s.w+(sx-s.x)] = rune('0' + n.Value)
		}
	}
	return s
}

// Resize refits the box, keeping the grains that still fit.
func (s *Sand) Resize(panel Panel) {
	w, h := panel.W, panel.H
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	next := make([]rune, w*h)
	for y := 0; y < h && y < s.h; y++ {
		for x := 0; x < w && x < s.w; x++ {
			next[y*w+x] = s.grain[y*s.w+x]
		}
	}
	s.x, s.y, s.w, s.h, s.grain = panel.X, panel.Y, w, h, next
}

// at reads a cell, treating everything outside the box as solid, which makes
// the walls and floor.
func (s *Sand) at(x, y int) rune {
	if x < 0 || y < 0 || x >= s.w || y >= s.h {
		return '#'
	}
	return s.grain[y*s.w+x]
}

// Pour turns the stream on at a screen position, which it takes into the box's
// own coordinates.
func (s *Sand) Pour(x, y int) { s.pourX, s.pourY, s.pouring = x-s.x, y-s.y, true }

// StopPouring turns it off.
func (s *Sand) StopPouring() { s.pouring = false }

// Step settles the grains.
func (s *Sand) Step(dt float64) {
	s.carry += dt * sandRate
	for s.carry >= 1 {
		s.carry--
		s.settle()
		if s.pouring {
			s.pour()
		}
	}
}

// pour drops a few grains from the pointer.
func (s *Sand) pour() {
	for i := 0; i < 3; i++ {
		x := s.pourX + s.rng.Intn(pourRadius*2+1) - pourRadius
		y := s.pourY
		if s.at(x, y) == 0 {
			s.grain[y*s.w+x] = rune('0' + s.rng.Intn(10))
		}
	}
}

// settle runs one update of the box, bottom row first. Top-down would let a
// grain fall the full height in one update, into space the row below had just
// vacated.
func (s *Sand) settle() {
	for y := s.h - 2; y >= 0; y-- {
		// Alternate the scan direction, or heaps lean toward the side that
		// always moves first.
		left := s.rng.Intn(2) == 0
		for k := 0; k < s.w; k++ {
			x := k
			if !left {
				x = s.w - 1 - k
			}
			s.fall(x, y)
		}
	}
}

// fall moves one grain, if it can.
func (s *Sand) fall(x, y int) {
	g := s.grain[y*s.w+x]
	if g == 0 {
		return
	}

	if s.at(x, y+1) == 0 {
		s.grain[(y+1)*s.w+x], s.grain[y*s.w+x] = g, 0
		return
	}

	// Blocked below, so try the diagonals in random order, or every heap
	// leans the same way.
	first, second := -1, 1
	if s.rng.Intn(2) == 0 {
		first, second = 1, -1
	}
	for _, dx := range [2]int{first, second} {
		if s.at(x+dx, y+1) == 0 && s.at(x+dx, y) == 0 {
			s.grain[(y+1)*s.w+x+dx], s.grain[y*s.w+x] = g, 0
			return
		}
	}
}

// Count is how many grains there are.
func (s *Sand) Count() int {
	n := 0
	for _, g := range s.grain {
		if g != 0 {
			n++
		}
	}
	return n
}

// Digits is every grain and where it is.
func (s *Sand) Digits() []Placed {
	out := make([]Placed, 0, 64)
	for y := 0; y < s.h; y++ {
		for x := 0; x < s.w; x++ {
			if g := s.grain[y*s.w+x]; g != 0 {
				out = append(out, Placed{X: float64(s.x + x), Y: float64(s.y + y), Digit: g})
			}
		}
	}
	return out
}

// Draw paints the box.
func (s *Sand) Draw(buf *canvas.Buffer, t *theme.Theme, panel Panel) {
	for y := 0; y < s.h; y++ {
		for x := 0; x < s.w; x++ {
			if g := s.grain[y*s.w+x]; g != 0 && panel.Contains(s.x+x, s.y+y) {
				buf.Set(s.x+x, s.y+y, g, t.Digit)
			}
		}
	}
}
