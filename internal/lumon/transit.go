package lumon

import (
	"github.com/terminaltrove/lumon/internal/ease"
	"github.com/terminaltrove/lumon/internal/glyph"
	"github.com/terminaltrove/lumon/internal/theme"
	"math"

	"github.com/terminaltrove/lumon/internal/canvas"
	"github.com/terminaltrove/lumon/internal/physics"
)

// Getting into and out of a diversion: each visible digit springs between its
// cell and wherever the diversion has it, in the same idiom as Flights. One
// particle per visible cell, a few hundred, the same order of work as Drift.

const (
	// transitFrequency is how hard each digit is pulled toward where it is
	// going. Seek is critically damped, so this settles without overshoot in
	// under a second.
	transitFrequency = 6.5

	// transitSizeFrequency is how fast a swollen digit changes size on the way.
	// Quicker than the travel, so a cluster is plain before it arrives.
	transitSizeFrequency = 8.0

	// transitArrived is how close, in screen cells, counts as landed.
	transitArrived = 0.7

	// transitBlend is how long the colour and the lean take to cross over, in
	// seconds. It is a shared time ramp: most digits are plain at both ends, so
	// a per-digit size spring would report them arrived on the first frame and
	// switch their colour instantly.
	transitBlend = 0.45

	// transitLimit is the longest a trip may take, in seconds. Without it a
	// particle that never quite settles (say, aimed at a place another is
	// sitting on) would strand the grid behind a transition for ever.
	transitLimit = 2.0
)

// Placed is a digit a diversion is showing, and where it has it.
type Placed struct {
	X, Y  float64
	Digit rune
}

// transitEntry is one digit in the air. It keeps the colour and lean it left
// with as well as those it is arriving at, because a transition must begin
// exactly as the frame on screen; re-deriving them (escalation red, a dormant
// cluster's shade) pops on the first frame.
type transitEntry struct {
	particle int
	gridID   int
	digit    rune

	target physics.Vec2
	height *physics.Scalar
	toward float64

	fromAttr, toAttr canvas.Attr
	fromTilt, toTilt float64

	// solid is whether the digit is drawn as a silhouette, copied from the
	// frame it set out from (m.boxes), not worked out from its height.
	solid bool
}

// Transit is the whole grid on its way somewhere.
type Transit struct {
	world   *physics.World
	entries []transitEntry
	forces  []physics.Vec2

	// back reports which way this trip is going: false out of the grid into a
	// diversion, true out of a diversion back into the grid.
	back bool

	elapsed float64
}

// NewTransit sends the visible digits between their cells and a set of places.
// Outbound it starts at the cells and aims at the places; with back set it
// goes the other way, aiming at wherever the grid would draw each digit now,
// so a cluster that kept swelling is grown back into.
func NewTransit(m *Model, places []Placed, back bool) *Transit {
	t := &Transit{world: physics.NewWorld(), back: back}
	t.world.SetDamping(1) // the spring does the damping

	home := m.drawnFrame()
	for n, h := range home {
		// Counts need not match the cells (fewer digits, or more once sand
		// has been poured), so places cycle. Digits sharing a start point
		// separate as they fly.
		away := h.at
		digit := h.digit
		if len(places) > 0 {
			p := places[n%len(places)]
			away = physics.Vec2{X: p.X, Y: p.Y}
			if back {
				digit = p.Digit
			}
		}

		// The diversions all draw plain characters.
		plain := m.Theme.Digit

		from, to := h.at, away
		fromH, toH := h.height, PlainHeight
		fromA, toA := h.attr, plain
		fromT, toT := h.tilt, 0.0
		if back {
			from, to = away, h.at
			fromH, toH = PlainHeight, h.height
			fromA, toA = plain, h.attr
			fromT, toT = 0.0, h.tilt
		}

		t.entries = append(t.entries, transitEntry{
			particle: t.world.Add(from, 1),
			gridID:   h.id,
			digit:    digit,
			target:   to,
			height:   physics.NewScalar(fromH, transitSizeFrequency),
			toward:   toH,
			fromAttr: fromA,
			toAttr:   toA,
			fromTilt: fromT,
			toTilt:   toT,
			solid:    h.solid,
		})
	}
	return t
}

// drawn is one digit as the renderer had it on the last frame.
type drawn struct {
	id     int
	digit  rune
	at     physics.Vec2
	height float64
	tilt   float64
	attr   canvas.Attr
	solid  bool
}

// drawnFrame is every visible digit exactly as the grid last drew it: either
// one of the boxes planGlyphs kept after overlap culling, or a plain character
// at PlainAnchor in cellAttr's colour. Asking Magnify instead disagrees with
// the screen (culled members, colours, tilt). m.boxes holds the previous
// frame, which is what is on screen when Update starts a transition.
func (m *Model) drawnFrame() []drawn {
	l := m.Layout
	out := make([]drawn, 0, l.ViewportW*l.ViewportH)

	big := make(map[int]glyphBox, len(m.boxes))
	for _, b := range m.boxes {
		big[b.id] = b
	}

	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			cell := m.Grid.At(gx, gy)
			if cell == nil {
				continue
			}
			id := GridIndex(gx, gy)
			d := drawn{id: id, digit: rune('0' + cell.Value)}

			if b, ok := big[id]; ok {
				d.at = physics.Vec2{X: float64(b.x + b.w/2), Y: float64(b.y + b.h/2)}
				d.height = b.height
				d.tilt = m.glyphTilt(cell)
				d.attr = m.glyphAttr(id)
				d.solid = b.solid
			} else {
				sx, sy := m.PlainAnchor(id)
				d.at = physics.Vec2{X: float64(sx), Y: float64(sy)}
				d.height = PlainHeight
				d.attr = m.cellAttr(cell, gx, gy)
			}
			out = append(out, d)
		}
	}
	return out
}

// progress is how far the colour and the lean have crossed over, 0 to 1.
func (t *Transit) progress() float64 { return ease.Clamp01(t.elapsed / transitBlend) }

// Len is how many digits are travelling.
func (t *Transit) Len() int { return len(t.entries) }

// Back reports whether this trip is heading back into the grid.
func (t *Transit) Back() bool { return t.back }

// Reverse turns the trip around where it stands.
func (t *Transit) Reverse(m *Model) {
	t.back = !t.back
	t.elapsed = 0

	// Each digit's current colour and lean become where it sets off from.
	held := t.progress()
	for i := range t.entries {
		e := &t.entries[i]
		e.fromAttr, e.toAttr = blendAttr(e.fromAttr, e.toAttr, held), e.fromAttr
		e.fromTilt, e.toTilt = e.fromTilt+(e.toTilt-e.fromTilt)*held, e.fromTilt
	}

	for i := range t.entries {
		e := &t.entries[i]
		gx, gy := e.gridID%GridSize, e.gridID/GridSize

		if t.back {
			// Homeward: aim at the cell, and grow back to whatever the board
			// has that digit at now.
			sx, sy := m.PlainAnchor(e.gridID)
			e.target = physics.Vec2{X: float64(sx), Y: float64(sy)}
			e.toward = m.Magnify.Height(gx, gy)
			continue
		}
		// Outbound again: it was heading home, so its old start is where the
		// diversion wanted it.
		e.target = t.world.Pos(e.particle)
		e.toward = PlainHeight
	}
}

// Step advances every digit toward where it is going.
func (t *Transit) Step(dt float64) {
	t.elapsed += dt
	if len(t.entries) == 0 {
		return
	}

	n := t.world.Len()
	if cap(t.forces) < n {
		t.forces = make([]physics.Vec2, n)
	}
	t.forces = t.forces[:n]
	for i := range t.forces {
		t.forces[i] = physics.Vec2{}
	}

	for _, e := range t.entries {
		pos := t.world.Pos(e.particle)
		vel := t.world.Velocity(e.particle, physics.DT)
		t.forces[e.particle] = physics.NewSeek(e.target, transitFrequency).Force(pos, vel)
	}
	t.world.IntegrateForces(dt, t.forces)

	for i := range t.entries {
		t.entries[i].height.Step(dt, t.entries[i].toward)
	}
}

// Done reports whether everything has arrived, or transitLimit has passed.
func (t *Transit) Done() bool {
	if t.elapsed >= transitLimit {
		return true
	}
	for _, e := range t.entries {
		if t.world.Pos(e.particle).Sub(e.target).Len() > transitArrived {
			return false
		}
	}
	return true
}

// Draw paints the digits wherever they have got to, as drawFlights does:
// below MinGlyphHeight a plain character, above it a rasterised glyph.
func (t *Transit) Draw(buf *canvas.Buffer, panel Panel) {
	top, bottom := panel.Y, panel.Y+panel.H-1

	for i := range t.entries {
		e := &t.entries[i]
		pos := t.world.Pos(e.particle)
		h := e.height.Value()

		// Colour and lean cross over during the trip, so a red cluster fades
		// as it leaves and a rocking one unwinds instead of snapping upright.
		attr := blendAttr(e.fromAttr, e.toAttr, t.progress())

		if h < MinGlyphHeight {
			x, y := int(pos.X+0.5), int(pos.Y+0.5)
			if panel.Contains(x, y) {
				buf.Set(x, y, e.digit, attr)
			}
			continue
		}

		tilt := e.fromTilt + (e.toTilt-e.fromTilt)*t.progress()
		cov := glyph.RasterizeFor(int(e.digit-'0'), h*glyph.CellAspect, h,
			math.Round(tilt/GlyphTiltStep)*GlyphTiltStep, e.solid)
		if cov == nil {
			continue
		}
		paintCoverage(buf,
			int(pos.X+0.5)-cov.W/2, int(pos.Y+0.5)-cov.H/2,
			cov, e.digit, e.solid, attr, top, bottom)
	}
}

// Places is where every digit currently is, so an unfinished trip can be
// handed straight to the next one.
func (t *Transit) Places() []Placed {
	out := make([]Placed, 0, len(t.entries))
	for _, e := range t.entries {
		p := t.world.Pos(e.particle)
		out = append(out, Placed{X: p.X, Y: p.Y, Digit: e.digit})
	}
	return out
}

// blendAttr crosses one attribute over to another. The foreground
// interpolates; the background and bold cannot, so they swap at the halfway
// mark.
func blendAttr(from, to canvas.Attr, p float64) canvas.Attr {
	out := canvas.Attr{FG: theme.BlendColors(from.FG, to.FG, p)}
	if p < 0.5 {
		out.BG, out.Bold = from.BG, from.Bold
	} else {
		out.BG, out.Bold = to.BG, to.Bold
	}
	return out
}

// clampPlaced keeps a set of places inside a panel, so a diversion that has let
// something drift off screen cannot fling a returning digit at it.
func clampPlaced(places []Placed, panel Panel) []Placed {
	for i := range places {
		places[i].X = math.Max(float64(panel.X), math.Min(float64(panel.X+panel.W-1), places[i].X))
		places[i].Y = math.Max(float64(panel.Y), math.Min(float64(panel.Y+panel.H-1), places[i].Y))
	}
	return places
}
