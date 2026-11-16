package lumon

import (
	"math"
	"slices"

	"github.com/terminaltrove/lumon/internal/ease"
	"github.com/terminaltrove/lumon/internal/physics"
)

// Flight is one digit travelling from its grid cell to a bin. Positions are in
// screen cells, since a flight crosses the grid, the rules and the bin row.
type Flight struct {
	ID     int // grid ID of the digit that left
	Digit  rune
	BinIdx int
	X, Y   float64

	// Height is how large the digit is currently drawn, in rows, and Tilt how
	// far it leans, in degrees. A digit leaves at its size and shrinks on the way.
	Height float64
	Tilt   float64

	// Mine is whether this refiner refined it, not another.
	Mine bool
}

// A refined cluster holds where it was refined for flightHold, then each digit
// leaves for its bin, the nearest first, each flightStagger after the one
// before and the last within flightSpread of the first. They arrive together,
// flightTravel after the last has left: a digit that leaves sooner takes that
// much longer.
const (
	flightHold    = 0.15 // seconds
	flightStagger = 0.03 // seconds
	flightSpread  = 0.12 // seconds
	flightTravel  = 1.8  // seconds
)

// A flight gathers speed over its first flightStart of the way, then eases
// out exponentially (flightExpo) into the bin, and lands once flightLanded of
// the way is covered.
const (
	flightStart  = 0.3
	flightExpo   = 4.0
	flightLanded = 0.985
)

// leanIn is how long, in seconds, a digit takes to lean as it leaves.
const leanIn = 0.12

// A digit keeps shaking as it was on the board, by up to shakeX columns and
// shakeY rows, while it waits and until shakeOut of its way is covered.
const (
	shakeX   = 0.5
	shakeY   = 0.25
	shakeOut = 0.35
)

// Departure describes a digit leaving the grid for a bin.
type Departure struct {
	GridID int
	Digit  rune
	From   physics.Vec2
	To     physics.Vec2
	BinIdx int

	// Height is the size the digit is leaving at, in rows, and Tilt the lean it
	// takes on the way, in degrees. Both ease out over the journey.
	Height float64
	Tilt   float64

	Mine bool
}

// Flights owns every digit currently in the air. Each follows a fixed path,
// timed from its launch, so every view of a refine draws it the same way.
type Flights struct {
	entries  []flightEntry
	landed   []Flight
	airborne map[int]bool

	// active is reused so drawing a frame allocates nothing.
	active []Flight

	// halfway is whose each cluster was that has come halfway since the last
	// TakeHalfway.
	halfway []bool
}

type flightEntry struct {
	id     int
	digit  rune
	binIdx int
	from   physics.Vec2
	target physics.Vec2
	mine   bool

	// leads marks the cluster's first digit to leave, whose halfway is the
	// cluster's; heard is set once that has been reported.
	leads, heard bool

	// height and tilt are what the digit left with; wait is how long it holds
	// before it moves, and age how long since it was launched, in seconds.
	height, tilt      float64
	wait, travel, age float64
}

// NewFlights returns an empty Flights.
func NewFlights() *Flights {
	return &Flights{airborne: make(map[int]bool)}
}

// Launch sends one refined cluster's digits toward their bins: they hold, then
// leave the nearest first.
func (f *Flights) Launch(cluster ...Departure) {
	order := make([]int, len(cluster))
	for i := range order {
		order[i] = i
	}
	distance := func(i int) float64 { return cluster[i].To.Sub(cluster[i].From).Len() }
	slices.SortStableFunc(order, func(a, b int) int {
		da, db := distance(a), distance(b)
		switch {
		case da < db:
			return -1
		case da > db:
			return 1
		}
		return 0
	})
	gap := flightStagger
	if n := len(cluster); n > 1 {
		gap = math.Min(flightStagger, flightSpread/float64(n-1))
	}
	last := float64(len(cluster)-1) * gap

	for rank, i := range order {
		d := cluster[i]
		height := d.Height
		if height < PlainHeight {
			height = PlainHeight
		}
		f.entries = append(f.entries, flightEntry{
			id:     d.GridID,
			digit:  d.Digit,
			binIdx: d.BinIdx,
			from:   d.From,
			target: d.To,
			mine:   d.Mine,
			leads:  rank == 0,
			height: height,
			tilt:   d.Tilt,
			wait:   flightHold + float64(rank)*gap,
			travel: flightTravel + last - float64(rank)*gap,
		})
		f.airborne[d.GridID] = true
	}
}

// done is how far along its journey e is, from 0 before it leaves to 1 at the
// bin.
func (e *flightEntry) done() float64 { return flightCurve((e.age - e.wait) / e.travel) }

// flightCurve is how far along a flight is t of the way through its time:
// speeding up from rest over flightStart, then easing out.
func flightCurve(t float64) float64 {
	t = ease.Clamp01(t)
	const a = flightStart
	r := t - a/2
	if t < a {
		r = t * t / (2 * a)
	}
	r /= 1 - a/2
	return (1 - math.Exp2(-flightExpo*r)) / (1 - math.Exp2(-flightExpo))
}

// at is where e is.
func (e *flightEntry) at() physics.Vec2 {
	done := e.done()
	return e.from.Add(e.target.Sub(e.from).Scale(done)).Add(e.shake(done))
}

// shake is how far e is shaken from its path: none at launch, so it leaves
// from where it was drawn, and none by shakeOut of the way.
func (e *flightEntry) shake(done float64) physics.Vec2 {
	k := 1 - ease.Smoothstep(ease.Clamp01(done/shakeOut))
	if k <= 0 {
		return physics.Vec2{}
	}
	// Two unrelated wobbles a digit, phased by its grid ID.
	p := float64(e.id%97) * 0.61
	wobble := func(a, b, phase float64) float64 {
		t := e.age
		return 0.6*(math.Sin(a*t+phase)-math.Sin(phase)) + 0.4*(math.Sin(b*t+2*phase)-math.Sin(2*phase))
	}
	return physics.Vec2{X: shakeX * k * wobble(44, 69, p), Y: shakeY * k * wobble(51, 37, p+1.7)}
}

// sizeAt is the height and lean of a digit that has come this far. Size follows
// distance covered, so a digit is plain exactly when it reaches its bin; it
// holds upright and leans as it leaves.
func (e *flightEntry) sizeAt(done float64) (height, tilt float64) {
	remaining := 1 - ease.Smoothstep(done)
	lean := ease.Clamp01((e.age - e.wait) / leanIn)
	return PlainHeight + (e.height-PlainHeight)*remaining, e.tilt * remaining * lean
}

// Step advances every flight, retiring those that have arrived.
func (f *Flights) Step(dt float64) {
	kept := f.entries[:0]
	for i := range f.entries {
		e := &f.entries[i]
		e.age += dt
		done := e.done()
		if done >= 0.5 {
			f.reportHalfway(e)
		}
		if done >= flightLanded {
			f.landed = append(f.landed, Flight{
				ID:     e.id,
				Digit:  e.digit,
				BinIdx: e.binIdx,
				X:      e.target.X,
				Y:      e.target.Y,
				Height: PlainHeight,
			})
			delete(f.airborne, e.id)
			continue
		}
		kept = append(kept, *e)
	}
	f.entries = kept
}

// reportHalfway notes that e's cluster has come halfway, once.
func (f *Flights) reportHalfway(e *flightEntry) {
	if e.leads && !e.heard {
		e.heard = true
		f.halfway = append(f.halfway, e.mine)
	}
}

// TakeHalfway returns whose each cluster was that has come halfway to its bin
// since the last call: true for this refiner's own. The slice is reused, so
// the caller must not retain it.
func (f *Flights) TakeHalfway() []bool {
	out := f.halfway
	f.halfway = f.halfway[:0]
	return out
}

// Active returns the flights still in the air, for the renderer to draw. The
// slice is reused between calls, so the caller must not retain it.
func (f *Flights) Active() []Flight {
	f.active = f.active[:0]
	for i := range f.entries {
		e := &f.entries[i]
		pos := e.at()
		h, tilt := e.sizeAt(e.done())
		f.active = append(f.active, Flight{
			ID:     e.id,
			Digit:  e.digit,
			BinIdx: e.binIdx,
			X:      pos.X,
			Y:      pos.Y,
			Height: h,
			Tilt:   tilt,
			Mine:   e.mine,
		})
	}
	return f.active
}

// TakeLanded returns the flights that arrived since the last call and forgets
// them. The caller credits the bin and regenerates the vacated cell.
func (f *Flights) TakeLanded() []Flight {
	if len(f.landed) == 0 {
		return nil
	}
	out := f.landed
	f.landed = nil
	return out
}

// InFlight reports whether a digit is airborne, so the grid renderer can skip it
// and avoid drawing the same digit in two places.
func (f *Flights) InFlight(gridID int) bool { return f.airborne[gridID] }
