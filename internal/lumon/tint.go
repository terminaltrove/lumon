package lumon

import (
	"github.com/terminaltrove/lumon/internal/canvas"
	"github.com/terminaltrove/lumon/internal/ease"
	"github.com/terminaltrove/lumon/internal/theme"
)

// Tint eases a digit's colour to a new one: the cursor arriving or leaving, a
// refiner's marker landing, a cluster waking, a cell coming back from a
// refinement. Each slot is a float and a rate, eased by smoothstep; a
// physics.Scalar per visible cell would cost more for the same curve.

// tintRate is how fast a digit crosses to a new colour, in crossings a second:
// about a fifth of a second. Slower reads as lag on a held cursor; faster
// reads as a switch.
const tintRate = 5.5

// tintCell is one viewport slot.
type tintCell struct {
	// id is the grid cell the slot is holding, or -1 for an empty one. A slot
	// stands for a different cell after a scroll; the id is how that is noticed.
	id       int
	from, to canvas.Attr
	p        float64 // 0 at the old colour, 1 at the new
}

// Tint is every visible digit's colour, and how far through a change it is.
//
// It is a flat slice indexed by viewport slot, not a map keyed by grid cell
// (as Magnify is). A scroll abandons every crossing and digits arrive at their
// colour, which is invisible under a moving grid. In exchange there is no map
// traffic on a path that runs for every visible cell twice a frame.
type Tint struct {
	cells []tintCell
	x, y  int
	w, h  int

	// live is how many slots are mid-crossing. It is nearly always zero, so
	// Attr answers with one compare on the per-digit draw path.
	live int
}

// NewTint returns an empty set.
func NewTint() *Tint { return &Tint{} }

// Sync refits the slots to the viewport. Like Drift's, call it before the step.
func (t *Tint) Sync(l *Layout) {
	viewX, viewY := l.ViewportX, l.ViewportY
	viewW, viewH := l.ViewportW, l.ViewportH
	if t.w == viewW && t.h == viewH && t.x == viewX && t.y == viewY {
		return
	}
	n := viewW * viewH
	if len(t.cells) != n {
		t.cells = make([]tintCell, n)
	}
	// The whole viewport has moved, so every slot holds a different cell.
	// p must be 1, not the zero value: live counts slots below one, and an
	// empty slot is arrived.
	for i := range t.cells {
		t.cells[i] = tintCell{id: -1, p: 1}
	}
	t.live = 0
	t.x, t.y, t.w, t.h = viewX, viewY, viewW, viewH
}

// slot is which entry a grid cell occupies, or -1 if it is not on screen.
func (t *Tint) slot(gx, gy int) int {
	dx, dy := gx-t.x, gy-t.y
	if dx < 0 || dy < 0 || dx >= t.w || dy >= t.h {
		return -1
	}
	return dy*t.w + dx
}

// Toward tells a cell what colour it should be. Call it once per visible cell
// per step, before Step. A cell new to its slot (scrolled into view) takes its
// colour at once, so a scroll does not wash the screen.
func (t *Tint) Toward(gx, gy int, want canvas.Attr) {
	s := t.slot(gx, gy)
	if s < 0 {
		return
	}
	id := GridIndex(gx, gy)
	c := &t.cells[s]
	if c.id != id {
		if c.p < 1 {
			t.live--
		}
		*c = tintCell{id: id, from: want, to: want, p: 1}
		return
	}
	if want == c.to {
		return
	}
	if c.p >= 1 {
		t.live++
	}
	// Set off from wherever it had got to, not from the colour it started at:
	// a cursor swept along a row turns cells back mid-crossing.
	c.from, c.to, c.p = tintBlend(c.from, c.to, ease.Smoothstep(c.p)), want, 0
}

// Step advances every crossing.
func (t *Tint) Step(dt float64) {
	if t.live == 0 {
		return
	}
	for i := range t.cells {
		c := &t.cells[i]
		if c.p >= 1 {
			continue
		}
		if c.p += dt * tintRate; c.p >= 1 {
			c.p = 1
			t.live--
		}
	}
}

// Attr is the colour a cell is showing, between the one it had and the one it
// is going to. A cell Toward was never called for gets want unchanged.
func (t *Tint) Attr(gx, gy int, want canvas.Attr) canvas.Attr {
	if t.live == 0 {
		return want
	}
	s := t.slot(gx, gy)
	if s < 0 {
		return want
	}
	// Settled first, so an arrived cell is answered without comparing
	// attributes or recomputing a grid index.
	c := &t.cells[s]
	if c.p >= 1 {
		return want
	}
	if c.id != GridIndex(gx, gy) || c.to != want {
		return want
	}
	return tintBlend(c.from, c.to, ease.Smoothstep(c.p))
}

// tintBlend crosses one digit's attribute to another. Only the foreground
// travels; background and weight take the new value at once. blendAttr instead
// switches them halfway, which here would leave the cursor's block of colour
// arriving a fifth of a second late.
func tintBlend(from, to canvas.Attr, p float64) canvas.Attr {
	return canvas.Attr{FG: theme.BlendColors(from.FG, to.FG, p), BG: to.BG, Bold: to.Bold}
}

// Crossing reports whether a cell is mid-change, for the tests.
func (t *Tint) Crossing(gx, gy int) bool {
	s := t.slot(gx, gy)
	if s < 0 {
		return false
	}
	c := &t.cells[s]
	return c.id == GridIndex(gx, gy) && c.p < 1
}

// Len is how many slots are being tracked, and Live how many are mid-crossing.
func (t *Tint) Len() int  { return len(t.cells) }
func (t *Tint) Live() int { return t.live }
