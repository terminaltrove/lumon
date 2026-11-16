package lumon

import (
	"github.com/terminaltrove/lumon/internal/theme"
	"math/rand"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// MATRIX: a wave of light falls down the board. It has no physics and moves no
// digits. Each viewport column has a head that travels down the grid rows and
// lights the board's own digits, so the file stays legible underneath.

const (
	// matrixMinSpeed and matrixMaxSpeed are how fast a column falls, in grid
	// rows a second. The spread keeps columns from moving as one sheet.
	matrixMinSpeed = 2.5
	matrixMaxSpeed = 11.0

	// matrixMinTail and matrixMaxTail are how many grid rows of light follow
	// the head. A grid row is two screen rows, and the viewport is only about a
	// dozen tall.
	matrixMinTail = 3
	matrixMaxTail = 9

	// matrixGap is the chance a column waits before falling again, so the
	// columns drift out of step instead of settling into a rhythm.
	matrixGap = 0.6
)

// Matrix is the wave passing down the board.
type Matrix struct {
	rng *rand.Rand

	// One entry per grid column of the viewport.
	head  []float64 // where the leading row is; may be negative
	speed []float64
	tail  []int
	wait  []float64

	rows int // grid rows in the viewport

	// Fade scales the wave's light; the wave always runs at full speed.
	Fade
}

// NewMatrix starts the wave over a viewport.
func NewMatrix(l *Layout, rng *rand.Rand) *Matrix {
	m := &Matrix{rng: rng, rows: l.ViewportH, Fade: NewFade()}
	for x := 0; x < l.ViewportW; x++ {
		m.head = append(m.head, 0)
		m.speed = append(m.speed, 0)
		m.tail = append(m.tail, 0)
		m.wait = append(m.wait, 0)
		m.restart(len(m.head)-1, true)
	}
	return m
}

// restart sends a column back to the top. first spreads the opening positions
// through the viewport, so the columns do not all start on one line.
func (m *Matrix) restart(x int, first bool) {
	m.speed[x] = matrixMinSpeed + m.rng.Float64()*(matrixMaxSpeed-matrixMinSpeed)
	m.tail[x] = matrixMinTail + m.rng.Intn(matrixMaxTail-matrixMinTail+1)

	if first {
		m.head[x] = m.rng.Float64() * float64(m.rows)
		return
	}
	m.head[x] = -float64(m.tail[x])
	if m.rng.Float64() < matrixGap {
		m.wait[x] = m.rng.Float64() * 1.6
	}
}

// Resize refits the wave to a new viewport, keeping the columns it already has
// so it does not restart under the refiner.
func (m *Matrix) Resize(l *Layout) {
	m.rows = l.ViewportH
	for len(m.head) < l.ViewportW {
		m.head = append(m.head, 0)
		m.speed = append(m.speed, 0)
		m.tail = append(m.tail, 0)
		m.wait = append(m.wait, 0)
		m.restart(len(m.head)-1, true)
	}
	m.head = m.head[:l.ViewportW]
	m.speed = m.speed[:l.ViewportW]
	m.tail = m.tail[:l.ViewportW]
	m.wait = m.wait[:l.ViewportW]
}

// Step advances every column.
func (m *Matrix) Step(dt float64) {
	m.StepFade(dt)

	for x := range m.head {
		if m.wait[x] > 0 {
			m.wait[x] -= dt
			continue
		}
		m.head[x] += m.speed[x] * dt
		if m.head[x]-float64(m.tail[x]) > float64(m.rows) {
			m.restart(x, false)
		}
	}
}

// litAt is how far behind a column's head a grid row sits, and whether it is lit
// at all.
func (m *Matrix) litAt(col, row int) (behind int, lit bool) {
	if col < 0 || col >= len(m.head) || m.wait[col] > 0 {
		return 0, false
	}
	behind = int(m.head[col]) - row
	if behind < 0 || behind >= m.tail[col] {
		return 0, false
	}
	return behind, true
}

// Draw paints every cell of the board, coloured by the wave.
func (mx *Matrix) Draw(buf *canvas.Buffer, t *theme.Theme, g *Grid, l *Layout, panel Panel) {
	for row := 0; row < l.ViewportH; row++ {
		for col := 0; col < l.ViewportW; col++ {
			n := g.At(l.ViewportX+col, l.ViewportY+row)
			if n == nil {
				continue
			}
			x, y := l.ScreenOf(l.ViewportX+col, l.ViewportY+row)
			if !panel.Contains(x, y) {
				continue
			}
			buf.Set(x, y, rune('0'+n.Value), mx.attrAt(t, col, row))
		}
	}
}

// attrAt is the colour a cell takes: brightest at the head, stepping back
// through the chrome to the board's resting colour behind the tail.
func (mx *Matrix) attrAt(t *theme.Theme, col, row int) canvas.Attr {
	behind, lit := mx.litAt(col, row)
	if !lit {
		return t.Digit
	}
	tail := mx.tail[col]
	var wave canvas.Attr
	switch {
	case behind == 0:
		wave = t.Hovered
	case behind*3 < tail:
		wave = t.BadLive
	case behind*3 < tail*2:
		wave = t.Chrome
	default:
		wave = t.BinOff
	}

	// Below full level the wave blends toward the resting colour; at zero it is
	// the plain grid.
	level := mx.Level()
	switch {
	case level >= 1:
		return wave
	case level <= 0:
		return t.Digit
	}
	return blendAttr(t.Digit, wave, level)
}

// Digits is every digit on the board, where the grid draws it, since the wave
// moves none of them.
func (mx *Matrix) Digits(g *Grid, l *Layout, panel Panel) []Placed {
	out := make([]Placed, 0, l.ViewportW*l.ViewportH)
	for row := 0; row < l.ViewportH; row++ {
		for col := 0; col < l.ViewportW; col++ {
			n := g.At(l.ViewportX+col, l.ViewportY+row)
			if n == nil {
				continue
			}
			x, y := l.ScreenOf(l.ViewportX+col, l.ViewportY+row)
			if !panel.Contains(x, y) {
				continue
			}
			out = append(out, Placed{X: float64(x), Y: float64(y), Digit: rune('0' + n.Value)})
		}
	}
	return out
}
