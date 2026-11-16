package lumon

import (
	"github.com/terminaltrove/lumon/internal/glyph"
	"github.com/terminaltrove/lumon/internal/theme"
	"math"
	"math/rand"
	"time"

	"github.com/terminaltrove/lumon/internal/canvas"
	"github.com/terminaltrove/lumon/internal/physics"
)

// The sandbox: the digits as loose particles. SCATTER lets them drift and
// DESCENT drops them into a heap that can be picked up and thrown; they differ
// only in a few constants and a mouse binding.
//
// Positions are in visual columns: x is a screen column and y is a screen row
// times RowAspect, so contacts are round and gravity looks right on cells about
// twice as tall as wide. They are converted back only to draw.

// Temperament is which of the two this sandbox is.
type Temperament int

const (
	// Scattering: no gravity; the numbers keep their momentum and wander the
	// box.
	Scattering Temperament = iota

	// Descending: they fall, land, and stack.
	Descending
)

const (
	// sandboxRadius is half a contact, in visual columns. A digit is 1x2 in
	// visual columns; this radius lets them pack side by side without
	// overlapping. Every digit shares it, so a digit drawn large would overlap
	// its neighbours.
	sandboxRadius = 1.0

	// scatterSpeed is how fast the numbers are thrown when they let go, in
	// visual columns per second, and descentGravity how hard they are pulled
	// down, in visual columns per second squared.
	scatterSpeed   = 34.0
	descentGravity = 90.0

	// descentSpreadCells is the sideways nudge each digit gets on the way down,
	// in cells per second. Without it they fall in their grid columns, a cell
	// apart, and never meet. It is in cells so it scales with the grid spacing.
	descentSpreadCells = 11.0 / CellWidth

	// descentVary is how much the pull differs from digit to digit, as a
	// fraction either side of descentGravity. It stands in for drag, so the
	// screen does not fall in lockstep as one sheet.
	descentVary = 0.45

	// A double tap scatters what is near it. blastWindow is how close together
	// the two presses must be, blastReach how far the shove carries in visual
	// columns, and blastSpeed how hard it is at the centre.
	blastWindow = 300 * time.Millisecond
	blastReach  = 26.0
	blastSpeed  = 95.0

	// scatterDamping is just under one, so a scattered board drifts and slowly
	// comes to rest. At exactly one it never slows down.
	scatterDamping = 0.9992
	descentDamping = 0.985

	// grabReach is how far from the pointer a digit can be picked up, in visual
	// columns. Generous, because a terminal reports whole cells.
	grabReach = 3.0
)

// Sandbox is the digits as loose particles.
type Sandbox struct {
	temperament Temperament

	world   *physics.World
	bounds  *physics.Bounds
	collide *physics.Collide
	grab    *physics.Grab

	// digit is what each particle is drawn as, and bad whether it was part of a
	// corrupt cluster, so those still glow.
	digit []rune
	bad   []bool

	// gravity is each digit's own pull, and height how tall it is drawn.
	gravity []physics.Vec2
	height  []float64

	// held is whether the pointer has hold of something.
	held bool

	// lastTap is when and where the pointer last went down, for spotting a
	// double tap.
	lastTap   time.Time
	lastTapAt physics.Vec2

	// now is the clock, replaced in tests.
	now func() time.Time
}

// NewSandbox takes the digits currently on screen (the view, not the full
// board) and lets go of them.
func NewSandbox(m *Model, t Temperament, panel Panel, rng *rand.Rand) *Sandbox {
	l := m.Layout
	s := &Sandbox{
		temperament: t,
		world:       physics.NewWorld(),
		collide:     physics.NewCollide(sandboxRadius),
		now:         time.Now,
	}

	s.setBox(panel)

	switch t {
	case Descending:
		s.world.SetDamping(descentDamping)
		s.bounds.Restitution = 0.18
		s.bounds.Friction = 0.5
	default:
		s.world.SetDamping(scatterDamping)
		s.bounds.Restitution = 0.75
		s.bounds.Friction = 0.02
	}

	s.world.AddConstraint(s.collide)
	s.world.AddConstraint(s.bounds)
	s.grab = physics.NewGrab(s.world)

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

			i := s.world.Add(physics.Vec2{X: float64(sx), Y: float64(sy) * RowAspect}, 1)
			s.digit = append(s.digit, rune('0'+n.Value))
			s.bad = append(s.bad, n.BadGroupID >= 0)

			// Its own pull and size.
			pull := descentGravity * (1 - descentVary + 2*descentVary*rng.Float64())
			s.gravity = append(s.gravity, physics.Vec2{Y: pull})

			s.height = append(s.height, PlainHeight)

			switch t {
			case Descending:
				// A sideways nudge, so the columns meet and form a heap (see
				// descentSpreadCells).
				spread := descentSpreadCells * l.CellW
				s.world.SetVelocity(i, physics.Vec2{
					X: (rng.Float64()*2 - 1) * spread,
					Y: rng.Float64() * spread,
				})

			case Scattering:
				// A random direction per digit. Pushing outward from the middle
				// would empty the centre and pack everything against the walls.
				angle := rng.Float64() * 2 * math.Pi
				speed := scatterSpeed * (0.45 + 0.85*rng.Float64())
				s.world.SetVelocity(i, physics.Vec2{
					X: math.Cos(angle) * speed,
					Y: math.Sin(angle) * speed,
				})
			}
		}
	}
	return s
}

// Resize moves the walls when the terminal changes size, keeping the digits.
// Bounds pushes anything now outside back in on the next step.
func (s *Sandbox) Resize(panel Panel) { s.setBox(panel) }

// setBox puts the walls around a panel, so a heap piles up inside the frame
// and not over the bins.
func (s *Sandbox) setBox(panel Panel) {
	min := physics.Vec2{X: float64(panel.X), Y: float64(panel.Y) * RowAspect}
	max := physics.Vec2{
		X: float64(panel.X + panel.W - 1),
		Y: float64(panel.Y+panel.H-1) * RowAspect,
	}
	if s.bounds == nil {
		s.bounds = physics.NewBounds(min, max)
		return
	}
	s.bounds.Min, s.bounds.Max = min, max
}

// Len is how many digits are loose.
func (s *Sandbox) Len() int { return s.world.Len() }

// Step advances the simulation.
func (s *Sandbox) Step(dt float64) {
	if s.temperament != Descending {
		s.world.Step(dt, physics.Vec2{})
		return
	}
	s.world.StepForces(dt, s.gravity)
}

// Tap records a press and reports whether it was the second of a double tap.
// The distance check is loose because a terminal reports whole cells.
func (s *Sandbox) Tap(col, row int) bool {
	at := physics.Vec2{X: float64(col), Y: float64(row) * RowAspect}
	now := s.now()

	double := !s.lastTap.IsZero() &&
		now.Sub(s.lastTap) <= blastWindow &&
		at.Sub(s.lastTapAt).Len() <= 4

	s.lastTap, s.lastTapAt = now, at
	if double {
		// So a triple tap is not two blasts.
		s.lastTap = time.Time{}
		s.Blast(col, row)
	}
	return double
}

// Blast shoves everything near a point outward. The falloff is linear; inverse
// square would fling the nearest digit absurdly fast and barely move the rest.
func (s *Sandbox) Blast(col, row int) {
	at := physics.Vec2{X: float64(col), Y: float64(row) * RowAspect}

	for i := 0; i < s.world.Len(); i++ {
		away := s.world.Pos(i).Sub(at)
		d := away.Len()
		if d > blastReach {
			continue
		}
		if d == 0 {
			away = physics.Vec2{X: 1}
			d = 1
		}
		push := blastSpeed * (1 - d/blastReach)
		v := s.world.Velocity(i, physics.DT)
		s.world.SetVelocity(i, v.Add(away.Scale(push/d)))
	}
}

// Take tries to pick up whatever is under a screen position.
func (s *Sandbox) Take(col, row int) bool {
	s.held = s.grab.Take(physics.Vec2{X: float64(col), Y: float64(row) * RowAspect}, grabReach)
	return s.held
}

// DragTo moves whatever is held.
func (s *Sandbox) DragTo(col, row int) {
	if !s.held {
		return
	}
	s.grab.MoveTo(physics.Vec2{X: float64(col), Y: float64(row) * RowAspect})
}

// Drop lets go, throwing.
func (s *Sandbox) Drop() {
	s.grab.Release()
	s.held = false
}

// Holding reports whether the pointer has hold of a digit.
func (s *Sandbox) Holding() bool { return s.held }

// Digits is every loose digit and where it is, for a transition into or out of
// this diversion.
func (s *Sandbox) Digits() []Placed {
	out := make([]Placed, 0, s.world.Len())
	for i := 0; i < s.world.Len(); i++ {
		p := s.world.Pos(i)
		out = append(out, Placed{X: p.X, Y: p.Y / RowAspect, Digit: s.digit[i]})
	}
	return out
}

// Draw paints every loose digit.
func (s *Sandbox) Draw(buf *canvas.Buffer, t *theme.Theme, panel Panel) {
	held, holding := s.grab.Holding()

	for i := 0; i < s.world.Len(); i++ {
		p := s.world.Pos(i)
		col := int(math.Round(p.X))
		row := int(math.Round(p.Y / RowAspect))
		if !panel.Contains(col, row) {
			continue
		}

		attr := t.Digit
		switch {
		case holding && i == held:
			attr = t.Hovered
		case s.bad[i]:
			attr = t.BadLive
		}

		if h := s.height[i]; h >= MinGlyphHeight {
			cov := glyph.RasterizeFor(int(s.digit[i]-'0'), h*glyph.CellAspect, h, 0, s.bad[i])
			if cov != nil {
				paintCoverage(buf, col-cov.W/2, row-cov.H/2, cov,
					s.digit[i], s.bad[i], attr,
					panel.Y, panel.Y+panel.H-1)
				continue
			}
		}
		buf.Set(col, row, s.digit[i], attr)
	}
}
