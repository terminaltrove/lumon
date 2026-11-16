package lumon

import (
	"github.com/terminaltrove/lumon/internal/theme"
	"math"
	"math/rand"

	"github.com/terminaltrove/lumon/internal/canvas"
	"github.com/terminaltrove/lumon/internal/physics"
)

// MATRIX 3D: the file reads itself, and then stands off the screen.
//
// It opens as the flat MATRIX wave, then the camera pulls back and the columns
// separate into depth. It uses the board's digits throughout: deep, they keep
// their grid coordinates and gain a z per column.

const (
	// depthOpen is how long the flat wave runs before the pull begins, in
	// seconds, so it is recognised as MATRIX first.
	depthOpen = 2.6

	// depthFrequency is how fast the camera travels, in radians a second.
	depthFrequency = 0.9

	// depthSpread is how far apart the near and far columns end up and
	// depthDolly the distance to the eye, both in visual columns, the units the
	// cloud is built in. The eye sits a little over the cloud's width back, so
	// near columns look larger without leaving the panel.
	depthSpread = 16.0
	depthDolly  = 150.0

	// depthTilt is how far the cloud is rocked over, in degrees. Depth is per
	// column, so yaw reveals it; pitch mostly foreshortens rows, and at 20
	// degrees the rain no longer reads as columns.
	depthTilt = 9.0

	// depthYaw is how far the deep view turns about the vertical and depthDrift
	// how fast that turn wanders, in degrees and degrees a second. It sways
	// back and forth; a steady spin looks like a turntable.
	depthYaw   = 34.0
	depthDrift = 9.0

	// depthFill is how much of the panel's width the cloud takes.
	depthFill = 0.9

	// depthArrived is how flat counts as flat, on the way back.
	depthArrived = 0.03
)

// Matrix3D is the wave, with somewhere to fall from.
type Matrix3D struct {
	// flat is the wave itself. Depth is a way of viewing it, not a second
	// simulation.
	flat *Matrix

	// depth moves the view from flat (0) to deep (1), and held is its target.
	depth *physics.Scalar
	held  float64

	// out is whether Leave has been called. held cannot answer that: it is 0
	// both before the pull begins and after Leave.
	out bool

	// z is a depth per viewport column, drawn once so a column keeps its place.
	z []float64

	cam   *Camera
	cloud []Pt3

	// Rig is the same camera control the volume uses. Touch is never called,
	// so its idle tour never stops.
	Rig *Rig

	rng *rand.Rand
	t   float64

	Fade
}

// NewMatrix3D starts the wave flat.
func NewMatrix3D(l *Layout, rng *rand.Rand) *Matrix3D {
	m := &Matrix3D{
		flat:  NewMatrix(l, rng),
		depth: physics.NewScalar(0, depthFrequency),
		cam:   NewCamera(),
		Rig:   NewRig(0, 0),
		rng:   rng,
		Fade:  NewFade(),
	}

	// This fade owns the transition, so the inner one starts at full; two
	// rising fades would multiply.
	m.flat.SetLevel(1)

	m.lay(l)
	return m
}

// lay draws a random depth for each column, so near and far columns
// interleave. Ordered depths open into a fan, not a volume.
func (m *Matrix3D) lay(l *Layout) {
	m.z = m.z[:0]
	for x := 0; x < l.ViewportW; x++ {
		m.z = append(m.z, (m.rng.Float64()*2-1)*depthSpread)
	}
}

// Resize refits the wave and the depths.
func (m *Matrix3D) Resize(l *Layout) {
	m.flat.Resize(l)
	if len(m.z) != l.ViewportW {
		m.lay(l)
	}
}

// Depth is how far the view has pulled back: 0 flat, 1 deep.
func (m *Matrix3D) Depth() float64 { return m.depth.Value() }

// Step runs the wave, and pulls the camera back once it has been watched flat.
func (m *Matrix3D) Step(dt float64) {
	m.StepFade(dt)
	m.flat.Step(dt)
	m.Rig.Step(dt)
	m.t += dt

	if m.t >= depthOpen && !m.out {
		m.held = 1
	}
	m.depth.Step(dt, m.held)

	// Fade only once the view is flat again, so the return retraces the pull.
	if m.out && m.depth.Value() < depthArrived {
		m.Fade.Leave()
	}
}

// Leave takes the view back to flat, and only then starts the fade.
func (m *Matrix3D) Leave() { m.out, m.held = true, 0 }

// Leaving reports whether Leave has been called. It reads the local flag, not
// the fade's, which is set only once the view is flat; Gone stays the fade's,
// so the digits are not sent home while the view is still coming back.
func (m *Matrix3D) Leaving() bool { return m.out }

// Digits is where the view currently has the board's digits.
func (m *Matrix3D) Digits(g *Grid, l *Layout, panel Panel) []Placed {
	if m.depth.Value() < depthArrived {
		return m.flat.Digits(g, l, panel)
	}
	out := make([]Placed, 0, l.ViewportW*l.ViewportH)
	for _, s := range m.shots(g, l, panel) {
		out = append(out, Placed{X: float64(s.Col), Y: float64(s.Row), Digit: s.Digit})
	}
	return out
}

// shots builds the cloud from the board and projects it, back to front.
//
// One point per visible cell, with its column's depth scaled by how far the
// view has opened. Each point is tagged with its viewport cell so the wave's
// colour survives the depth sort.
func (m *Matrix3D) shots(g *Grid, l *Layout, panel Panel) []Shot {
	if len(m.z) != l.ViewportW {
		return nil
	}
	d := m.depth.Value()

	m.cloud = m.cloud[:0]
	cx := float64(l.ViewportW-1) / 2
	cy := float64(l.ViewportH-1) / 2
	for row := 0; row < l.ViewportH; row++ {
		for col := 0; col < l.ViewportW; col++ {
			n := g.At(l.ViewportX+col, l.ViewportY+row)
			if n == nil {
				continue
			}
			// In visual columns at the grid's scale: a cell is CellW columns
			// across and a grid row is RowH rows, or RowH*RowAspect columns,
			// tall. A scale of one is the flat wave's picture.
			m.cloud = append(m.cloud, Pt3{
				X:     (float64(col) - cx) * l.CellW,
				Y:     (cy - float64(row)) * l.RowH * RowAspect,
				Z:     m.z[col] * d,
				Digit: rune('0' + n.Value),
				Tag:   row*l.ViewportW + col,
			})
		}
	}
	if len(m.cloud) == 0 {
		return nil
	}

	// Fill the panel's width, then pull back if that makes it too tall.
	across := float64(maxi(l.ViewportW-1, 1)) * l.CellW
	down := float64(maxi(l.ViewportH-1, 1)) * l.RowH
	scale := float64(panel.W) * depthFill / across
	if tall := down * scale; tall > float64(panel.H)*0.95 {
		scale *= float64(panel.H) * 0.95 / tall
	}
	m.cam.Scale = scale

	// The wander scales with depth, so a flat wave does not turn. It is added
	// after the rig's aim, so a refiner steering the camera starts from wherever
	// the wander is.
	m.Rig.Aim(m.cam, depthDolly)
	m.cam.Pitch += depthTilt * d
	m.cam.Yaw += depthYaw * d * math.Sin(m.t*depthDrift*math.Pi/180)

	return m.cam.Shots(m.cloud, panel)
}

// Draw paints the wave, flat or deep.
func (m *Matrix3D) Draw(buf *canvas.Buffer, t *theme.Theme, g *Grid, l *Layout, panel Panel) {
	d := m.depth.Value()
	if d < depthArrived {
		m.flat.Draw(buf, t, g, l, panel)
		return
	}
	for _, s := range m.shots(g, l, panel) {
		col, row := s.Tag%l.ViewportW, s.Tag/l.ViewportW
		buf.Set(s.Col, s.Row, s.Digit, m.shade(t, col, row, s.Depth, d))
	}
}

// shade is the colour a projected digit takes: the wave's, dimmed a step with
// distance, the only depth cue in an unlit scene.
func (m *Matrix3D) shade(t *theme.Theme, col, row int, depth, d float64) canvas.Attr {
	attr := m.flat.attrAt(t, col, row)

	// 0 at the far side of the cloud, 1 at the near.
	near := 0.5 + 0.5*depth/(depthSpread*d+0.001)
	if near > 0.42 {
		return attr
	}
	if attr == t.Digit {
		return t.BinOff
	}
	return t.Digit
}
