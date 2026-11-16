package lumon

import (
	"github.com/terminaltrove/lumon/internal/noise"
	"github.com/terminaltrove/lumon/internal/physics"
)

// Drift is the ambient wobble of every number on screen, as in LumonMDR ("a
// moving Perlin noise map offsets each number"). The noise drives a tethered
// particle, so a number lags the field and overshoots slightly. Drift is much
// weaker than Agitation, so a waking cluster stands out.
const (
	// MaxDriftOffset bounds ambient travel, in cell units. It must comfortably
	// exceed the column dead band or no digit ever moves (0.16 peaked exactly
	// at the threshold).
	MaxDriftOffset = 0.28

	driftTether = 3.2
	driftAccel  = 22.0

	// driftSpeed is slow so a digit rests at an offset for a beat, in noise
	// units per second.
	driftSpeed = 0.16

	// driftFieldScale spreads the noise across the grid. Below 1 so neighbouring
	// cells sample nearby points and drift together.
	driftFieldScale = 0.18
)

// Drift holds a particle per visible cell. The pool is sized to the viewport
// and reassigned as it scrolls. Slot order is viewport scan order, so a cell's
// slot is found by arithmetic.
type Drift struct {
	world *physics.World

	viewX, viewY int
	viewW, viewH int

	home   []physics.Vec2
	forces []physics.Vec2

	// column and row hold each slot's committed offset, for hysteresis.
	column []int
	row    []int

	t float64
}

// NewDrift returns an empty Drift. Call Sync before stepping.
func NewDrift() *Drift {
	return &Drift{world: physics.NewWorld()}
}

// Len returns how many cells are being tracked.
func (d *Drift) Len() int { return d.viewW * d.viewH }

// Sync points the pool at the layout's current viewport, growing it if needed
// and rebasing every slot when the viewport moved. A zoom moves the viewport
// on most of its frames, so the wobble restarts several times during one.
func (d *Drift) Sync(l *Layout) {
	viewX, viewY := l.ViewportX, l.ViewportY
	viewW, viewH := l.ViewportW, l.ViewportH

	need := viewW * viewH
	for d.world.Len() < need {
		d.world.Add(physics.Vec2{}, 1)
		d.home = append(d.home, physics.Vec2{})
		d.column = append(d.column, 0)
		d.row = append(d.row, 0)
	}

	moved := d.viewX != viewX || d.viewY != viewY ||
		d.viewW != viewW || d.viewH != viewH

	d.viewX, d.viewY = viewX, viewY
	d.viewW, d.viewH = viewW, viewH

	if !moved {
		return
	}

	// Each slot now stands for a different cell, so it restarts at that cell
	// without the previous occupant's motion.
	for row := 0; row < d.viewH; row++ {
		for col := 0; col < d.viewW; col++ {
			i := row*d.viewW + col
			home := physics.Vec2{X: float64(d.viewX + col), Y: float64(d.viewY + row)}
			d.home[i] = home
			d.world.SetPos(i, home)
			d.column[i] = 0
			d.row[i] = 0
		}
	}
}

// Step advances every tracked cell by dt, in seconds.
func (d *Drift) Step(dt float64) {
	n := d.Len()
	if n == 0 {
		return
	}

	if cap(d.forces) < d.world.Len() {
		d.forces = make([]physics.Vec2, d.world.Len())
	}
	d.forces = d.forces[:d.world.Len()]
	for i := range d.forces {
		d.forces[i] = physics.Vec2{}
	}

	d.t += dt * driftSpeed

	for i := 0; i < n; i++ {
		home := d.home[i]
		pos := d.world.Pos(i)
		vel := d.world.Velocity(i, physics.DT)

		f := physics.NewSeek(home, driftTether).Force(pos, vel)

		// One axis per digit, in a checkerboard, so neighbours weave instead
		// of the grid sliding as one. The vertical drive is halved because a
		// row is about twice as tall as a column.
		nx := home.X * driftFieldScale
		ny := home.Y * driftFieldScale
		drive := noise.PerlinAnimation.Noise3D(nx, ny, d.t) * driftAccel
		if DriftsHorizontally(int(home.X), int(home.Y)) {
			f = f.Add(physics.Vec2{X: drive})
		} else {
			f = f.Add(physics.Vec2{Y: drive * 0.5})
		}
		d.forces[i] = f
	}

	d.world.IntegrateForces(dt, d.forces)

	for i := 0; i < n; i++ {
		delta := d.world.Pos(i).Sub(d.home[i])
		if l := delta.Len(); l > MaxDriftOffset {
			d.world.SetPos(i, d.home[i].Add(delta.Scale(MaxDriftOffset/l)))
		}
	}
}

// slot returns a cell's slot index, or -1 if it is off screen.
func (d *Drift) slot(gridX, gridY int) int {
	col := gridX - d.viewX
	row := gridY - d.viewY
	if col < 0 || row < 0 || col >= d.viewW || row >= d.viewH {
		return -1
	}
	return row*d.viewW + col
}

// Offset returns a cell's ambient displacement in cell units, or zero if the
// cell is not on screen.
func (d *Drift) Offset(gridX, gridY int) (dx, dy float64) {
	i := d.slot(gridX, gridY)
	if i < 0 {
		return 0, 0
	}
	delta := d.world.Pos(i).Sub(d.home[i])
	return delta.X, delta.Y
}

// ColumnOffset returns how many whole columns a cell is displaced by, with
// hysteresis so a digit on a boundary does not stutter. The renderer uses this;
// Offset is the raw value.
func (d *Drift) ColumnOffset(l *Layout, gridX, gridY int) int {
	i := d.slot(gridX, gridY)
	if i < 0 {
		return 0
	}
	dx := d.world.Pos(i).Sub(d.home[i]).X
	cols, _ := l.Screen(dx, 0)
	d.column[i] = columnStep(cols, d.column[i])
	return d.column[i]
}

// RowOffset returns how many whole rows a cell is displaced by. One cell unit
// of vertical travel is a pitch of rows.
func (d *Drift) RowOffset(l *Layout, gridX, gridY int) int {
	i := d.slot(gridX, gridY)
	if i < 0 {
		return 0
	}
	dy := d.world.Pos(i).Sub(d.home[i]).Y
	_, rows := l.Screen(0, dy)
	d.row[i] = rowStep(rows, d.row[i], l.MaxRowSag())
	return d.row[i]
}
