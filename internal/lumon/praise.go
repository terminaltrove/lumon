package lumon

import (
	"github.com/terminaltrove/lumon/internal/glyph"
	"github.com/terminaltrove/lumon/internal/theme"
	"math"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// PRAISE: a word from the founder, turning in space. The letters are sampled
// into a cloud of points, spun about the vertical and projected onto the cells
// (camera.go). Each point is drawn as a digit from the board.

// praiseLines is the word, set on two lines. On one line each letter is about
// four rows tall, too few to resolve a P's bowl or an A's crossbar.
var praiseLines = []string{"PRAISE", "KEIR"}

// praiseLead is the gap between the two lines, in unit boxes.
const praiseLead = 1.15

const (
	// praiseAdvance is how far the pen moves between letters, in unit boxes.
	// Under one, so the word is set tight.
	praiseAdvance = 0.66

	// praiseSpin is how fast the word turns, in degrees a second, and
	// praiseNod how far it rocks toward and away from the viewer.
	praiseSpin = 42.0
	praiseNod  = 13.0

	// praiseDepth is how far the word is extruded toward the viewer. Two
	// shells, front and back, are enough to read as solid edge-on.
	praiseDepth = 0.16

	// praiseCamera is the distance to the eye, in the same units as the word is
	// wide. Larger is flatter.
	praiseCamera = 9.0

	// praiseSample is roughly how far apart the sampled points are along a
	// stroke, in unit-box widths.
	praiseSample = 0.055
)

// Praise is the turning word.
type Praise struct {
	// pts is the word sampled into a cloud, in the word's own space. Each
	// point's digit is fixed at build time so the word does not shimmer.
	pts []Pt3

	angle float64
	cam   *Camera
}

// NewPraise samples the word and takes its digits from the board.
func NewPraise(m *Model) *Praise {
	p := &Praise{cam: NewCamera()}
	p.cam.Dolly = praiseCamera

	next := 0
	digitAt := func() rune {
		l := m.Layout
		n := m.Grid.At(
			l.ViewportX+next%maxi(l.ViewportW, 1),
			l.ViewportY+(next/maxi(l.ViewportW, 1))%maxi(l.ViewportH, 1),
		)
		next++
		if n == nil {
			return '0'
		}
		return rune('0' + n.Value)
	}

	// Each line centred on the origin, and the block of lines centred too.
	top := -(float64(len(praiseLines)-1) * praiseLead) / 2

	for li, text := range praiseLines {
		originX := -praiseLineWidth(text) / 2
		dy := top + float64(li)*praiseLead

		for i, r := range text {
			strokes := glyph.LetterStrokes(r)
			if strokes == nil {
				continue
			}
			dx := originX + float64(i)*praiseAdvance

			for _, st := range strokes {
				for _, pt := range samplePoints(st, praiseSample) {
					// y is flipped: the unit box runs downward and the
					// camera's does not.
					x := dx + pt.X
					y := (0.5 - pt.Y) - dy
					for _, z := range [2]float64{-praiseDepth / 2, praiseDepth / 2} {
						p.pts = append(p.pts, Pt3{X: x, Y: y, Z: z, Digit: digitAt()})
					}
				}
			}
		}
	}
	return p
}

// praiseLineWidth is how wide a line of the word is, in unit boxes.
func praiseLineWidth(text string) float64 {
	if text == "" {
		return 0
	}
	return float64(len([]rune(text))-1)*praiseAdvance + 1
}

// praiseWidest is the widest line, which is what the scale is fitted to.
func praiseWidest() float64 {
	w := 0.0
	for _, l := range praiseLines {
		if n := praiseLineWidth(l); n > w {
			w = n
		}
	}
	return w
}

// maxi is the larger of two ints, and mini the smaller.
func maxi(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func mini(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// samplePoints walks a polyline, dropping a point every step along it.
func samplePoints(s glyph.Stroke, step float64) []glyph.Point {
	if len(s) == 0 {
		return nil
	}
	out := []glyph.Point{s[0]}
	carry := 0.0
	for i := 1; i < len(s); i++ {
		a, b := s[i-1], s[i]
		dx, dy := b.X-a.X, b.Y-a.Y
		seg := math.Hypot(dx, dy)
		if seg == 0 {
			continue
		}
		for carry+step <= seg {
			carry += step
			t := carry / seg
			out = append(out, glyph.Point{X: a.X + dx*t, Y: a.Y + dy*t})
		}
		carry -= seg
	}
	return out
}

// Step turns the word.
func (p *Praise) Step(dt float64) {
	p.angle += praiseSpin * dt
	if p.angle >= 360 {
		p.angle -= 360
	}
}

// project fits the word to the panel and runs it through the camera, passing
// each visible point to emit, nearest last. Draw and Digits share it so the
// transition uses the same projection. Fitting stays here, not in Camera,
// because a word fits to width where a grid keeps its shape.
func (p *Praise) project(panel Panel, emit func(col, row int, depth float64, digit rune)) {
	w, h := panel.W, panel.H
	if w < 20 || h < 6 || len(p.pts) == 0 {
		return
	}

	ry := p.angle * math.Pi / 180
	p.cam.Yaw = p.angle
	p.cam.Pitch = praiseNod * math.Sin(ry*2)

	// Fill the panel width with a margin; rows are half as many because a
	// cell is twice as tall as it is wide.
	scale := float64(w) * 0.78 / praiseWidest()
	if tall := float64(len(praiseLines)) * praiseLead * scale / RowAspect; tall > float64(h)*0.86 {
		scale *= float64(h) * 0.86 / tall
	}
	p.cam.Scale = scale

	p.cam.Project(p.pts, panel, func(s Shot) {
		emit(s.Col, s.Row, s.Depth, s.Digit)
	})
}

// Digits is where the word currently has its points on screen, so the word can
// assemble out of the grid and fall back into it.
func (p *Praise) Digits(panel Panel) []Placed {
	out := make([]Placed, 0, len(p.pts))
	p.project(panel, func(col, row int, _ float64, digit rune) {
		out = append(out, Placed{X: float64(col), Y: float64(row), Digit: digit})
	})
	return out
}

// Draw paints the word.
func (p *Praise) Draw(buf *canvas.Buffer, t *theme.Theme, panel Panel) {
	p.project(panel, func(col, row int, depth float64, digit rune) {
		attr := t.Digit
		switch {
		case depth > praiseDepth*0.2:
			attr = t.Hovered
		case depth > -praiseDepth*0.2:
			attr = t.BadLive
		}
		buf.Set(col, row, digit, attr)
	})
}
