package lumon

import (
	"github.com/terminaltrove/lumon/internal/ease"
	"math/rand"

	"github.com/terminaltrove/lumon/internal/physics"
)

// Regen refills a cell after its digit has flown off to a bin. The cell spins
// through random digits and slows to a stop on its final value, fading up as it
// settles. A physics.Scalar converges on 1 and the scramble re-rolls at a rate
// proportional to its velocity, so it stops exactly when the spring does.
const (
	// regenFrequency sets how long a cell takes to settle. Low, so the spin is
	// watchable.
	regenFrequency = 2.6

	// regenSettled is the value at which a cell is considered finished.
	regenSettled = 0.985

	// scrambleRate converts spring velocity into re-rolls per second.
	scrambleRate = 26.0
)

type regenCell struct {
	spring *physics.Scalar
	final  int
	shown  int
	credit float64 // fractional re-rolls accumulated
}

// Regen holds every cell currently spinning up a new number.
type Regen struct {
	cells map[int]*regenCell
	rng   *rand.Rand
}

// NewRegen returns an empty Regen.
func NewRegen(rng *rand.Rand) *Regen {
	return &Regen{cells: make(map[int]*regenCell), rng: rng}
}

// Begin starts a cell spinning toward final.
func (r *Regen) Begin(gridID, final int) {
	r.cells[gridID] = &regenCell{
		spring: physics.NewScalar(0, regenFrequency),
		final:  final,
		shown:  r.rng.Intn(10),
	}
}

// Active reports whether a cell is still spinning.
func (r *Regen) Active(gridID int) bool {
	_, ok := r.cells[gridID]
	return ok
}

// Display returns the digit a spinning cell should show. The second result is
// false for cells that are not spinning, whose actual value should be drawn.
func (r *Regen) Display(gridID int) (int, bool) {
	c, ok := r.cells[gridID]
	if !ok {
		return 0, false
	}
	return c.shown, true
}

// Density returns how far a spinning cell has faded in, in [0,1]. Settled cells
// report 1.
func (r *Regen) Density(gridID int) float64 {
	c, ok := r.cells[gridID]
	if !ok {
		return 1
	}
	return ease.Clamp01(c.spring.Value())
}

// Step advances every spinning cell, retiring those that have settled.
func (r *Regen) Step(dt float64) {
	for id, c := range r.cells {
		c.spring.Step(dt, 1)

		// Re-roll at a rate set by the spring's speed, so the scramble
		// decelerates with it.
		speed := c.spring.Velocity()
		if speed < 0 {
			speed = -speed
		}
		c.credit += speed * scrambleRate * dt
		for c.credit >= 1 {
			c.credit--
			c.shown = r.rng.Intn(10)
		}

		if c.spring.Value() >= regenSettled {
			c.shown = c.final
			delete(r.cells, id)
		}
	}
}
