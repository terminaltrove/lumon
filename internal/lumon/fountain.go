package lumon

import (
	"math"
	"math/rand"

	"github.com/terminaltrove/lumon/internal/canvas"
	"github.com/terminaltrove/lumon/internal/ease"
	"github.com/terminaltrove/lumon/internal/physics"
	"github.com/terminaltrove/lumon/internal/theme"
)

// FOUNTAIN: five jets, one over each bin, throw the file's digits up into the
// panel to fall back and pool. The solver is physics.Fluid, smoothed particle
// hydrodynamics ported from inertia's fluid.rs. Below, "fluid" means the
// substance or the solver, never this diversion.
//
// It works in visual columns: x is a column and y is a row times RowAspect.
// A terminal row is about two columns tall, so a fluid solved in raw cells
// would be twice as viscous vertically as horizontally.
//
// The jets rise from the panel's bottom edge, not from the bins. Diversions
// may not draw below the panel (several tests check this, clampPlaced and Draw
// clip to it, and view.go paints the bins over the diversion anyway). Each jet
// stands at Layout.BinCenterX, so it is directly over its bin.

const (
	// fountainGravity is the pull, in visual columns a second squared.
	fountainGravity = 26.0

	// fountainRise is how much of the panel a jet climbs, as a fraction. The
	// speed follows from it, sqrt(2gh), as skyRise does. The routine's lift and
	// the throw's jitter go on top: at 120x40, 100x30 and 80x24 the tallest
	// jet reaches 0.92 to 0.97 of the panel. Any higher and digits pile against
	// the ceiling and never return to the pool (0.85 does at two of the three).
	fountainRise = 0.79

	// fountainWall is how much of its speed a particle keeps off a wall. Low: a
	// liquid does not bounce, it slaps and runs.
	fountainWall = 0.2

	// fountainSubsteps is how many solver steps a frame is cut into. SPH is
	// stiff: a step that moves a particle past its neighbour lands it where the
	// pressure is larger still, and the fluid explodes. Two suffices here; four
	// settles to the same depth at a higher cost.
	fountainSubsteps = 2

	// fountainFill is how much of the panel the digits would occupy if all were
	// in the air at once, which sets the smoothing radius. It is sized from the
	// total because NewFluidAt fixes the kernel normalisations, rest density and
	// Gas at construction. That is only correct because every digit exists from
	// the first frame, queued at a mouth. See Fountain.queued.
	fountainFill = 0.34

	// fountainSpread is how much sideways speed a digit leaves with, in visual
	// columns a second.
	fountainSpread = 22.0

	// queueReach is how near a mouth a fallen digit must come to be taken back
	// into its queue, in visual columns.
	queueReach = 3.0
)

// radiusFor is the smoothing radius that makes n particles fill fountainFill of
// a panel. Each particle claims about spacing squared of area, and the spacing
// is a fixed fraction of the radius.
func radiusFor(n int, panel Panel) float64 {
	if n < 1 || panel.W < 1 || panel.H < 1 {
		return physics.DefaultH
	}
	area := float64(panel.W) * float64(panel.H) * RowAspect * fountainFill
	spacing := math.Sqrt(area / float64(n))
	return math.Max(physics.DefaultH, spacing/0.5)
}

// Fountain is the board thrown up out of the bins.
type Fountain struct {
	sim *physics.Fluid

	// digits runs parallel to sim.Pos and must stay that way: Digits and Draw
	// index it by particle, so a missed append panics a frame later. It is the
	// invariant inertia's push_particle keeps for its arrays.
	digits []rune

	// queued are the digits waiting at each mouth, and mouths where the jets are.
	// A digit is in exactly one of queued or the simulation. Every digit has a
	// place from the first frame, so the board's arrival flight has somewhere to
	// fly to: five knots over the bins.
	queued [BinCount][]rune
	mouths []physics.Emitter

	// show is which routine is running and showAt how far into it, in seconds.
	show   int
	showAt float64

	min, max physics.Vec2
	panel    Panel

	// speed is how hard a jet throws, read off the panel by Resize.
	speed float64
	rng   *rand.Rand
}

// NewFountain gathers the visible digits at the five mouths.
func NewFountain(m *Model, panel Panel, rng *rand.Rand) *Fountain {
	// Seeded from the drawn frame, so what pours is what was on screen,
	// including digits enlarged by the lens.
	drawn := m.drawnFrame()

	f := &Fountain{sim: physics.NewFluidAt(radiusFor(len(drawn), panel)), rng: rng}
	f.sim.Gravity = physics.Vec2{Y: fountainGravity}
	f.Resize(panel)
	f.Aim(m.Layout)

	for i, h := range drawn {
		f.queued[i%BinCount] = append(f.queued[i%BinCount], h.digit)
	}
	return f
}

// Aim puts a mouth over each bin's centre column, on the panel's bottom edge.
// It is separate from Resize because it needs the layout.
func (f *Fountain) Aim(l *Layout) {
	if l == nil {
		return
	}
	f.mouths = f.mouths[:0]
	y := float64(f.panel.Y+f.panel.H-1) * RowAspect
	for i := 0; i < BinCount; i++ {
		x := float64(l.BinCenterX[i])
		if x < float64(f.panel.X) || x > float64(f.panel.X+f.panel.W-1) {
			continue
		}
		f.mouths = append(f.mouths, physics.Emitter{
			At:    physics.Vec2{X: x, Y: y},
			Vel:   physics.Vec2{Y: -f.speed},
			Reach: float64(l.BinWidth(i)) / 2.2,
		})
	}
}

// mouthOf is where one bin's queue waits, for Digits and for the tests.
func (f *Fountain) mouthOf(i int) physics.Vec2 {
	if i < len(f.mouths) {
		return f.mouths[i].At
	}
	return physics.Vec2{
		X: float64(f.panel.X + f.panel.W/2),
		Y: float64(f.panel.Y+f.panel.H-1) * RowAspect,
	}
}

// Resize re-boxes the fluid.
func (f *Fountain) Resize(panel Panel) {
	f.panel = panel
	f.min = physics.Vec2{X: float64(panel.X), Y: float64(panel.Y) * RowAspect}
	f.max = physics.Vec2{
		X: float64(panel.X + panel.W - 1),
		Y: float64(panel.Y+panel.H-1) * RowAspect,
	}
	f.speed = math.Sqrt(2 * fountainGravity * math.Max(f.max.Y-f.min.Y, 1) * fountainRise)
}

// routine is one act of the fountain show. Its strength answers how hard jet i
// of n is throwing t seconds in, from 0 to 1. Strength scales launch speed, so
// a jet at half strength reaches a quarter as high. Routines run in order and
// are deterministic, so a test can name one and watch it.
type routine struct {
	name string
	secs float64

	// strength is how hard jet i of n throws, t seconds in.
	strength func(t float64, i, n int) float64
}

// jetFloor is the strength below which a jet stops throwing. Weaker throws
// barely clear the mouth and are gathered again next frame. The lull in the
// finale needs jets to be fully off.
const jetFloor = 0.12

// routines is the programme, in order.
var routines = []routine{
	{name: "TOGETHER", secs: 6, strength: func(t float64, i, n int) float64 {
		// All five in unison.
		return 0.55 + 0.45*math.Sin(t/6*4*math.Pi)
	}},
	{name: "WAVE", secs: 7, strength: func(t float64, i, n int) float64 {
		// A swell running along the row and back, never fully off.
		u := t / 7 * 2
		if u > 1 {
			u = 2 - u
		}
		at := u * float64(n-1)
		d := math.Abs(float64(i) - at)
		return 0.18 + 0.82*math.Max(0, 1-d/1.3)
	}},
	{name: "ALTERNATE", secs: 6, strength: func(t float64, i, n int) float64 {
		// Odds and evens trading.
		phase := t / 6 * 6 * math.Pi
		if i%2 == 1 {
			phase += math.Pi
		}
		return 0.15 + 0.85*(0.5+0.5*math.Sin(phase))
	}},
	{name: "ARCH", secs: 6, strength: func(t float64, i, n int) float64 {
		// Tallest in the middle, falling away to the ends, eased up and down.
		half := math.Max(float64(n-1)/2, 1)
		shape := 1 - 0.6*math.Abs(float64(i)-half)/half
		return shape * ease.Smoothstep(ease.Clamp01(math.Min(t, 6-t)/1.2))
	}},
	{name: "CASCADE", secs: 7, strength: func(t float64, i, n int) float64 {
		// Each in turn, staying up, then all go out at once.
		up := float64(i) / math.Max(float64(n), 1) * 4
		if t < up {
			return 0
		}
		return ease.Smoothstep(ease.Clamp01(math.Min(t-up, 6.6-t) / 0.6))
	}},
	{name: "FINALE", secs: 6, strength: func(t float64, i, n int) float64 {
		// A lull, so the pool goes flat, then everything at once.
		if t < 0.9 {
			return 0
		}
		return ease.Smoothstep(ease.Clamp01((t - 0.9) / 0.5))
	}},
}

// strength is how hard jet i is throwing now.
func (f *Fountain) strength(i int) float64 {
	r := routines[f.show%len(routines)]
	n := len(f.mouths)
	if n < 1 {
		n = 1
	}
	return ease.Clamp01(r.strength(f.showAt, i, n))
}

// Routine is which one is running, for the tests.
func (f *Fountain) Routine() string { return routines[f.show%len(routines)].name }

// advanceShow moves the programme on.
func (f *Fountain) advanceShow(dt float64) {
	f.showAt += dt
	for f.showAt >= routines[f.show%len(routines)].secs {
		f.showAt -= routines[f.show%len(routines)].secs
		f.show = (f.show + 1) % len(routines)
	}
}

// Step catches what has come down, throws what will fit, and solves.
func (f *Fountain) Step(dt float64) {
	f.advanceShow(dt)
	f.gather()
	f.throw()

	// Throw once a frame, solve per substep. Emit regulates itself by refusing
	// a crowded site, so calling it per substep would double the fountain.
	h := dt / fountainSubsteps
	for i := 0; i < fountainSubsteps; i++ {
		f.sim.Step(h, f.min, f.max, fountainWall)
	}
}

// gather takes a digit that has come back down near a mouth out of the
// simulation and puts it back in that mouth's queue. Without it the pool
// fills, every mouth stays crowded, and the jets stop.
func (f *Fountain) gather() {
	if len(f.mouths) == 0 {
		return
	}
	floor := f.max.Y - queueReach
	for i := len(f.sim.Pos) - 1; i >= 0; i-- {
		if f.sim.Pos[i].Y < floor {
			continue
		}
		// Descending only. A just-thrown particle is still within queueReach
		// of its mouth for several frames, so without this every jet takes its
		// own output straight back.
		if f.sim.Vel[i].Y <= 0 {
			continue
		}
		// The nearest mouth, not only one within reach: most digits land
		// between two bins and would otherwise never be picked up.
		near, best := 0, math.Inf(1)
		for b := range f.mouths {
			if d := math.Abs(f.sim.Pos[i].X - f.mouths[b].At.X); d < best {
				near, best = b, d
			}
		}
		f.queued[near] = append(f.queued[near], f.digits[i])
		f.remove(i)
	}
}

// remove takes one particle out, keeping digits in step with the simulation.
//
// remove takes one particle out, keeping digits in step with the simulation.
// It swaps with the last, which permutes one other index; nothing here holds
// an index across a frame. inertia's cull_below does the same.
func (f *Fountain) remove(i int) {
	last := len(f.sim.Pos) - 1
	f.sim.Pos[i], f.sim.Vel[i] = f.sim.Pos[last], f.sim.Vel[last]
	f.digits[i] = f.digits[last]
	f.sim.Pos = f.sim.Pos[:last]
	f.sim.Vel = f.sim.Vel[:last]
	f.digits = f.digits[:last]
}

// throw offers each mouth its own queue, and lets the emitter decide how much of
// it will fit.
func (f *Fountain) throw() {
	// One mouth at a time, so a bin with nothing queued cannot spend another
	// bin's digits.
	for b := range f.mouths {
		if len(f.queued[b]) == 0 {
			continue
		}
		// Off is a distinct state, not a very weak jet: see jetFloor.
		lift := f.strength(b)
		if lift < jetFloor {
			continue
		}
		was := f.sim.Len()
		f.sim.Emit(f.mouths[b : b+1])

		for i := was; i < f.sim.Len(); i++ {
			if len(f.queued[b]) == 0 {
				// The mouth had room for more than this bin has digits.
				f.sim.Pos = f.sim.Pos[:i]
				f.sim.Vel = f.sim.Vel[:i]
				break
			}
			f.digits = append(f.digits, f.queued[b][0])
			f.queued[b] = f.queued[b][1:]

			// Its own sideways kick and speed jitter. The emitter gives every
			// particle its mouth's exact velocity, which makes the digits march
			// up and down in lockstep columns.
			f.sim.Vel[i] = physics.Vec2{
				X: (f.rng.Float64() - 0.5) * fountainSpread * lift,
				Y: -f.speed * lift * (0.75 + 0.5*f.rng.Float64()),
			}
		}
	}
}

// Airborne is how many digits are in the air, against how many are waiting.
func (f *Fountain) Airborne() (flying, waiting int) {
	flying = f.sim.Len()
	for b := range f.queued {
		waiting += len(f.queued[b])
	}
	return
}

// Digits is every digit, queued or airborne, so the board has somewhere to fly
// to on the way in and the flight home starts from where the fluid left it.
func (f *Fountain) Digits() []Placed {
	flying, waiting := f.Airborne()
	out := make([]Placed, 0, flying+waiting)

	for i, p := range f.sim.Pos {
		out = append(out, Placed{X: p.X, Y: p.Y / RowAspect, Digit: f.digits[i]})
	}
	for b := range f.queued {
		at := f.mouthOf(b)
		for j, d := range f.queued[b] {
			// Stacked a little above the mouth, so a queue reads as a knot.
			out = append(out, Placed{
				X:     at.X + float64(j%3) - 1,
				Y:     at.Y/RowAspect - float64(j/3)*0.34,
				Digit: d,
			})
		}
	}
	return out
}

// Draw paints the digits where the fluid has them, coloured by speed first and
// then density. Everything in the air is sparse, so density alone draws the
// jets in one flat shade; speed shows a digit leaving the mouth and slowing at
// the top of its arc. Density still shades the pool.
func (f *Fountain) Draw(buf *canvas.Buffer, t *theme.Theme, panel Panel) {
	rest := f.sim.RestDensity()
	fast := f.speed * 0.8

	for i, p := range f.sim.Pos {
		x := int(math.Round(p.X))
		y := int(math.Round(p.Y / RowAspect))
		if !panel.Contains(x, y) {
			continue
		}

		attr := t.Digit
		if s := f.sim.Vel[i].Len(); s > fast*0.35 {
			// Up the jet: brightest as it leaves, cooling as it slows.
			attr = t.On(theme.BlendColors(t.Live, t.Peak,
				ease.Clamp01((s-fast*0.35)/(fast-fast*0.35))), true)
		} else if rest > 0 {
			switch d := f.sim.Density(i) / rest; {
			case d > 1.05:
				attr = t.BadLive
			case d > 0.75:
				attr = t.Hovered
			}
		}
		buf.Set(x, y, f.digits[i], attr)
	}
}
