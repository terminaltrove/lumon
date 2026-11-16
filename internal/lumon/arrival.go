package lumon

import (
	"github.com/terminaltrove/lumon/internal/ease"
	"github.com/terminaltrove/lumon/internal/theme"
	"math"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// How a board arrives: the chrome (rules, bins, header) comes up as a plain
// fade, and the digits arrive in a wavefront from the middle of the viewport
// outward.

const (
	// arrivalSeconds is how long the wavefront takes to reach the furthest
	// corner of the board. The chrome's fade settles within it.
	arrivalSeconds = 1.15

	// arrivalBand is the soft edge of the wavefront, in visual columns. Without
	// it the reveal is a hard expanding ring that reads as a wipe.
	arrivalBand = 2 * CellWidth
)

// Arrival is a board coming up, and how far across it has got.
//
// It is nil whenever nothing is arriving, which keeps it off the hot path:
// cellAttr runs for every visible digit every frame, and a settled board pays
// one nil check there.
type Arrival struct {
	// Fade is the chrome, rising 0 → 1. Critically damped, because an arrival
	// that overshot and came back would read as a flash.
	Fade

	// elapsed is the wavefront's clock. The radius is linear in it, so the
	// ring travels at one speed; a spring would race out and crawl to the rim.
	elapsed float64

	// The middle of the visible block, in cells, and how far its furthest
	// corner lies, in visual columns.
	cx, cy, far float64

	// The spacing the board is drawn at, so Reach measures in the same units
	// as far. It changes with the zoom.
	cellW, rowH float64
}

// NewArrival starts a board dark, with nothing yet reached.
func NewArrival(l *Layout) *Arrival {
	a := &Arrival{Fade: NewFade()}
	a.Sync(l)
	return a
}

// Sync re-measures the board. It does not touch the clock, so a resize part
// way through does not replay the arrival (the same rule afterLayout follows).
func (a *Arrival) Sync(l *Layout) {
	if a == nil {
		return
	}

	// Distances are in visual columns: dy is scaled by RowAspect (as the lens
	// and Ripple.Kick do), or the circle would be a wide ellipse. The centre is
	// the middle cell, (w-1)/2; Reach is asked about whole cells, and a centre
	// at w/2 would make the ring lopsided.
	w, h := float64(l.ViewportW), float64(l.ViewportH)
	a.cx, a.cy = (w-1)/2, (h-1)/2
	a.far = math.Hypot(a.cx*float64(l.CellW), a.cy*float64(l.RowH)*RowAspect)
	if a.far < 1 {
		a.far = 1
	}
	a.cellW, a.rowH = float64(l.CellW), float64(l.RowH)
}

// Step advances both motions.
func (a *Arrival) Step(dt float64) {
	if a == nil {
		return
	}
	a.elapsed += dt
	a.StepFade(dt)
}

// Done reports whether there is nothing left to watch: the ring has cleared the
// furthest corner and the chrome has come all the way up.
func (a *Arrival) Done() bool {
	return a != nil && a.radius() > a.far+arrivalBand && a.Level() > 1-fadeGone
}

// radius is how far the wavefront has got, in visual columns.
func (a *Arrival) radius() float64 {
	return a.elapsed / arrivalSeconds * a.far
}

// Reach is how arrived one cell is, from 0 (not yet) to 1 (fully). vx, vy are
// viewport cells, so the ripple starts in the middle of what is on screen.
func (a *Arrival) Reach(vx, vy int) float64 {
	if a == nil {
		return 1
	}
	dx := (float64(vx) - a.cx) * a.cellW
	dy := (float64(vy) - a.cy) * a.rowH * RowAspect
	d := math.Hypot(dx, dy)

	lead := a.radius()
	if d >= lead {
		return 0
	}
	if d <= lead-arrivalBand {
		return 1
	}
	return ease.Smoothstep((lead - d) / arrivalBand)
}

// dim pulls one cell's colour back toward the backdrop by how far the wavefront
// has got to it. The chrome's fade is a palette swap that reaches this too;
// dim adds only the spatial part.
func (a *Arrival) dim(want canvas.Attr, vx, vy int, back canvas.Color) canvas.Attr {
	if r := a.Reach(vx, vy); r < 1 {
		want.FG = theme.BlendColors(back, want.FG, r)
		// Backgrounds too (the cursor's plate, a held cluster's), or they show
		// at full strength while the board around them is still dark. Most
		// attributes have no background, and an unset colour must stay unset.
		if want.BG.Set() {
			want.BG = theme.BlendColors(back, want.BG, r)
		}
	}
	return want
}
