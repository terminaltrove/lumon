package lumon

import (
	"github.com/terminaltrove/lumon/internal/theme"
	"math"
	"math/rand"
	"strconv"

	"github.com/terminaltrove/lumon/internal/canvas"
	"github.com/terminaltrove/lumon/internal/physics"
)

// PRIMES: a diversion where drifting digits add up when two of them touch.
// Every prime value is lit and the rest sit at the ordinary colour; the player
// shoves digits together to make more primes.
//
// It plays on a copy of the visible digits, never on the file: refining is
// credited, shared and persisted, and a private diversion must not touch it.

const (
	// primeDrift is how fast the digits wander, in visual columns a second.
	primeDrift = 7.0

	// primeDamping is close enough to one that the board keeps moving for a
	// long while without needing to be stirred.
	primeDamping = 0.9995

	// primeCeiling is where a number stops growing. Every merge adds to the
	// board's total, so without it a number climbs without limit and grows
	// wider than the screen. Past the ceiling it is banked (counted) and reset
	// to a single digit.
	primeCeiling = 9999

	// primeClear is how much room a number keeps beyond its own ink, in visual
	// columns. Without it "13" and "77" settle touching and read as 1377.
	primeClear = 1.0

	// mergeCooldown is how long a digit ignores further contacts after a
	// merge, in seconds. Collide.OnContact fires once per overlapping pair per
	// relaxation pass (Substeps x Iterations = 48 times a step), so without it
	// a touching pair would merge dozens of times in one frame.
	mergeCooldown = 0.35
)

// itoa is strconv.Itoa under a shorter name, for the readouts.
func itoa(n int) string { return strconv.Itoa(n) }

// prime reports whether n is prime. Numbers here grow well past a single
// digit, so it is a trial division, not a lookup of 2, 3, 5, 7.
func prime(n int) bool {
	if n < 2 {
		return false
	}
	if n%2 == 0 {
		return n == 2
	}
	for f := 3; f*f <= n; f += 2 {
		if n%f == 0 {
			return false
		}
	}
	return true
}

// Primes is the merge game.
type Primes struct {
	world   *physics.World
	bounds  *physics.Bounds
	collide *physics.Collide
	grab    *physics.Grab
	rng     *rand.Rand

	value []int
	deaf  []float64 // seconds left before this digit will merge again

	// pending holds the pairs the solver reported this step. They are merged
	// after the step, because changing particles mid-pass leaves the solver's
	// spatial hash stale.
	pending [][2]int

	made   int // primes made
	merge  int // merges of any kind
	banked int // numbers that reached the ceiling

	held bool
}

// NewPrimes lets the visible digits loose.
func NewPrimes(m *Model, panel Panel, rng *rand.Rand) *Primes {
	l := m.Layout
	p := &Primes{world: physics.NewWorld(), rng: rng}
	p.world.SetDamping(primeDamping)

	p.bounds = physics.NewBounds(
		physics.Vec2{X: float64(panel.X), Y: float64(panel.Y) * RowAspect},
		physics.Vec2{X: float64(panel.X + panel.W - 1), Y: float64(panel.Y+panel.H-1) * RowAspect},
	)
	p.bounds.Restitution, p.bounds.Friction = 0.9, 0

	p.collide = physics.NewCollide(sandboxRadius)
	p.collide.OnContact = func(i, j int) { p.pending = append(p.pending, [2]int{i, j}) }

	p.world.AddConstraint(p.collide)
	p.world.AddConstraint(p.bounds)
	p.grab = physics.NewGrab(p.world)

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
			i := p.world.Add(physics.Vec2{X: float64(sx), Y: float64(sy) * RowAspect}, 1)
			p.value = append(p.value, n.Value)
			p.deaf = append(p.deaf, 0)
			p.collide.Radii = append(p.collide.Radii, 0)
			p.resize(len(p.value) - 1)

			angle := rng.Float64() * 2 * math.Pi
			speed := primeDrift * (0.4 + 0.9*rng.Float64())
			p.world.SetVelocity(i, physics.Vec2{
				X: math.Cos(angle) * speed,
				Y: math.Sin(angle) * speed,
			})
		}
	}
	return p
}

// resize gives a number a contact radius as wide as its text, since it is
// drawn centred on its particle. A uniform radius lets a long number overlap
// its neighbours.
func (p *Primes) resize(i int) {
	half := float64(len(itoa(p.value[i]))) / 2
	p.collide.Radii[i] = half + primeClear
}

// Len is how many digits are in play. It never changes: a merge gives the
// consumed digit a fresh value, so the board never drains.
func (p *Primes) Len() int { return len(p.value) }

// Made is how many primes have been made, Merges how many collisions have added
// up at all, and Largest the biggest number on the board.
func (p *Primes) Made() int   { return p.made }
func (p *Primes) Merges() int { return p.merge }

func (p *Primes) Largest() int {
	n := 0
	for _, v := range p.value {
		if v > n {
			n = v
		}
	}
	return n
}

// Resize moves the walls.
func (p *Primes) Resize(panel Panel) {
	p.bounds.Min = physics.Vec2{X: float64(panel.X), Y: float64(panel.Y) * RowAspect}
	p.bounds.Max = physics.Vec2{
		X: float64(panel.X + panel.W - 1),
		Y: float64(panel.Y+panel.H-1) * RowAspect,
	}
}

// Step drifts everything and settles whatever collided.
func (p *Primes) Step(dt float64) {
	for i := range p.deaf {
		if p.deaf[i] > 0 {
			p.deaf[i] -= dt
		}
	}

	p.pending = p.pending[:0]
	p.world.Step(dt, physics.Vec2{})
	p.settle()
}

// settle takes the merges the solver reported: each pair once, and only if both
// digits are out of their cooldown.
func (p *Primes) settle() {
	for _, pair := range p.pending {
		i, j := pair[0], pair[1]
		if i >= len(p.value) || j >= len(p.value) {
			continue
		}
		if p.deaf[i] > 0 || p.deaf[j] > 0 {
			continue
		}

		sum := p.value[i] + p.value[j]
		p.merge++
		if prime(sum) {
			p.made++
		}
		if sum > primeCeiling {
			p.banked++
			sum = 1 + p.rng.Intn(9)
		}
		p.value[i] = sum

		// The other takes a fresh value and stays on the board.
		p.value[j] = 1 + p.rng.Intn(9)

		p.resize(i)
		p.resize(j)
		p.deaf[i], p.deaf[j] = mergeCooldown, mergeCooldown
	}
	p.pending = p.pending[:0]
}

// Take grabs the digit nearest a cell, so two digits can be shoved together.
func (p *Primes) Take(col, row int) bool {
	p.held = p.grab.Take(physics.Vec2{X: float64(col), Y: float64(row) * RowAspect}, grabReach)
	return p.held
}

// DragTo moves whatever is held.
func (p *Primes) DragTo(col, row int) {
	if p.held {
		p.grab.MoveTo(physics.Vec2{X: float64(col), Y: float64(row) * RowAspect})
	}
}

// Drop lets go.
func (p *Primes) Drop() {
	p.grab.Release()
	p.held = false
}

// Holding reports whether the pointer has hold of a digit.
func (p *Primes) Holding() bool { return p.held }

// Draw paints the board: the indivisible lit, the rest ordinary.
func (p *Primes) Draw(buf *canvas.Buffer, t *theme.Theme, panel Panel) {
	held, holding := p.grab.Holding()

	for i := 0; i < p.world.Len(); i++ {
		pos := p.world.Pos(i)
		col := int(math.Round(pos.X))
		row := int(math.Round(pos.Y / RowAspect))
		if !panel.Contains(col, row) {
			continue
		}

		attr := t.Digit
		switch {
		case holding && i == held:
			attr = t.Hovered
		case prime(p.value[i]):
			attr = t.BadLive
		}

		// Centred on its particle, so a number grows in both directions.
		text := itoa(p.value[i])
		x := col - len(text)/2
		for k := 0; k < len(text); k++ {
			if panel.Contains(x+k, row) {
				buf.Set(x+k, row, rune(text[k]), attr)
			}
		}
	}
}

// Digits is every digit and where it is, for a transition into or out of this.
func (p *Primes) Digits() []Placed {
	out := make([]Placed, 0, p.world.Len())
	for i := 0; i < p.world.Len(); i++ {
		pos := p.world.Pos(i)
		// One entry per character, so a long number comes apart into digits
		// on the way home.
		text := itoa(p.value[i])
		for k := 0; k < len(text); k++ {
			out = append(out, Placed{
				X:     pos.X + float64(k-len(text)/2),
				Y:     pos.Y / RowAspect,
				Digit: rune(text[k]),
			})
		}
	}
	return out
}
