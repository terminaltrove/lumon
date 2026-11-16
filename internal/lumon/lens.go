package lumon

import (
	"math"

	"github.com/terminaltrove/lumon/internal/physics"
)

// The cursor lens. It bulges the grid around the pointer without magnifying:
// cells near the focus are pushed outward and the digits keep their size.
// Size is reserved for corruption, so the lens must never make a digit big.
const (
	// RowAspect is how many columns tall one screen row is. A terminal cell is
	// about twice as tall as it is wide; without this the lens is a tall thin
	// ellipse.
	RowAspect = 2.0

	// fisheyeReachCells is how far the displacement reaches and lensPushCells
	// how far one lens pushes a digit, both in cells. Layout.FisheyeRadius and
	// LensPush scale them by the cell width for the current zoom.
	//
	// The push must stay under a quarter of the reach, or the map stops being
	// monotonic and the grid folds over itself, so two cells land on one spot
	// and a click is ambiguous. TestTheLensNeverFoldsTheGridAtAnyZoom pins it.
	fisheyeReachCells = 16.0 / CellWidth
	lensPushCells     = 3.0 / CellWidth
)

const (
	// lensFrequency is how fast the focus chases the pointer.
	lensFrequency = 9.0
)

// Lens is the cursor's fisheye: where it is looking, and how far it pushes the
// board apart around that.
type Lens struct {
	world *physics.World
	p     int

	target physics.Vec2

	// The grid cell this lens is aimed at, for lenses that follow somebody
	// else's cursor, so a resize or scroll can re-aim it. It is kept here
	// because reading it off the Desk would need the room's lock.
	cell   [2]int
	onCell bool

	// The lens's reach and push in visual columns, cached by Fit because
	// bulge runs for every visible cell, per lens, every frame.
	rReach, rPush float64
}

// NewLens returns a lens focused on the origin, fitted to the default spacing.
// Without a fit its reach would be zero and it would displace nothing.
func NewLens() *Lens {
	w := physics.NewWorld()
	l := &Lens{world: w, p: w.Add(physics.Vec2{}, 1)}
	l.Fit(NewLayout(120, 40))
	return l
}

// Fit gives the lens the spacing of the board it is looking at.
func (l *Lens) Fit(lay *Layout) {
	l.rReach, l.rPush = lay.FisheyeRadius(), lay.LensPush()
}

// LookAt aims the lens at a screen position. The focus springs there, which
// turns whole-cell mouse reports into continuous motion.
func (l *Lens) LookAt(x, y float64) { l.target = physics.Vec2{X: x, Y: y} }

// Focus is where the lens is currently centred, in fractional screen cells.
func (l *Lens) Focus() physics.Vec2 { return l.world.Pos(l.p) }

// Warp puts the focus somewhere with no travel, for a resize or a scroll where
// springing across the screen would look like a swipe.
func (l *Lens) Warp(x, y float64) {
	l.target = physics.Vec2{X: x, Y: y}
	l.world.SetPos(l.p, l.target)
	l.world.SetVelocity(l.p, physics.Vec2{})
}

// AimAtCell points the lens at a grid cell, given where this viewer's layout
// puts it. The cell is remembered so Reproject can find it again after the
// screen has moved under it.
func (l *Lens) AimAtCell(gx, gy int, sx, sy float64) {
	l.cell, l.onCell = [2]int{gx, gy}, true
	l.LookAt(sx, sy)
}

// Reproject moves the lens to wherever its cell now falls, with no travel.
func (l *Lens) Reproject(layout *Layout) {
	if !l.onCell {
		return
	}
	sx, sy := layout.ScreenOf(l.cell[0], l.cell[1])
	l.Warp(float64(sx), float64(sy))
}

// Step advances the focus toward wherever it was last aimed.
func (l *Lens) Step(dt float64) {
	pos := l.world.Pos(l.p)
	vel := l.world.Velocity(l.p, physics.DT)
	f := physics.NewSeek(l.target, lensFrequency).Force(pos, vel)
	l.world.IntegrateForces(dt, []physics.Vec2{f})
}

// radius is the distance from the focus to a screen position, in visual columns.
func (l *Lens) radius(x, y float64) (r, dx, dy float64) {
	f := l.Focus()
	dx = x - f.X
	dy = (y - f.Y) * RowAspect
	return math.Hypot(dx, dy), dx, dy
}

// bulge is how much further out a point at radius r is drawn. It is zero at
// the focus and at the rim, so the focused digit and everything outside the
// lens stay put.
func (l *Lens) bulge(r float64) float64 {
	if r <= 0 || r >= l.rReach {
		return 0
	}
	q := r / l.rReach
	return l.rPush * 4 * q * (1 - q)
}

// Displace returns where a digit is drawn, given where the layout puts it.
// Always pass the undisplaced position, or the push depends on its own output.
func (l *Lens) Displace(x, y float64) (float64, float64) {
	ox, oy := l.Offset(x, y)
	return x + ox, y + oy
}

// Offset is how far this lens moves a point. Several lenses compose by summing
// offsets, so two lenses on opposite sides of a digit partly cancel.
func (l *Lens) Offset(x, y float64) (dx, dy float64) {
	r, rx, ry := l.radius(x, y)
	if r <= 0 || r >= l.rReach {
		return 0, 0
	}
	push := l.bulge(r) / r
	return rx * push, ry * push / RowAspect
}
