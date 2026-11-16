package lumon

import (
	"math"

	"github.com/terminaltrove/lumon/internal/glyph"
)

// ScreenAnchor is where a digit is drawn this frame: the cell Layout gives it,
// bulged by every lens in the room, then displaced by its own motion.
//
// Layout stays undistorted, so its ScreenOf and CellAt are exact inverses and
// the renderer and mouse handler cannot disagree about the grid origin. The
// distortion is applied here, in one place.
func (m *Model) ScreenAnchor(gridID int) (x, y int) {
	gx, gy := gridID%GridSize, gridID/GridSize
	sx, sy := m.Layout.ScreenOf(gx, gy)

	fx, fy := m.Bulge(float64(sx), float64(sy))

	// Agitation replaces ambient drift for a woken cluster.
	dx, dy := m.Agitation.Offset(gridID)
	if dx == 0 && dy == 0 {
		dx, dy = m.Drift.Offset(gx, gy)
	}

	// The ripple adds to either, so a cluster keeps shaking as a wave passes.
	bx, by := m.rippleOffset(gx, gy)
	dx, dy = dx+bx, dy+by

	ox, oy := m.Layout.Screen(dx, dy)
	return int(math.Round(fx + ox)), int(math.Round(fy + oy))
}

// Bulge applies every lens in the room to a screen position.
//
// The offsets are summed as vectors, so refiners on opposite sides of a digit
// partly cancel, and the total is limited so several lenses on one cell cannot
// fold the grid over itself. A single lens is left untouched, so more refiners
// never change how the board feels to one.
func (m *Model) Bulge(x, y float64) (float64, float64) {
	var dx, dy float64
	for _, l := range m.lenses() {
		ox, oy := l.Offset(x, y)
		dx, dy = dx+ox, dy+oy
	}
	if dx == 0 && dy == 0 {
		return x, y
	}

	// Read once: this runs for every visible cell.
	push, ceiling := m.Layout.LensPush(), m.Layout.MaxBulge()

	// Measured in visual columns, where a row counts for RowAspect. Below
	// LensPush this is the identity; past it the push eases into MaxBulge with
	// tanh, whose slope of one at zero keeps the join smooth. A hard clamp
	// would leave a visible crease.
	mag := math.Hypot(dx, dy*RowAspect)
	if mag > push {
		room := ceiling - push
		capped := push + room*math.Tanh((mag-push)/room)
		dx, dy = dx*capped/mag, dy*capped/mag
	}
	return x + dx, y + dy
}

// HitTest returns the digit under a screen position.
//
// It checks, in order: the enlarged glyph boxes the renderer placed, topmost
// first; the anchor index recorded while drawing, which is exact for every lens,
// agitation and drift; and finally Layout, for a frame not drawn yet.
func (m *Model) HitTest(sx, sy int) (gridX, gridY int, ok bool) {
	// The boxes and index come from the last Draw. If the board has changed
	// since, neither may answer, and the guard must sit above the boxes because
	// they are consulted first.
	if m.anchorAt != m.stampNow() {
		return m.Layout.CellAt(sx, sy)
	}

	for i := len(m.boxes) - 1; i >= 0; i-- {
		b := m.boxes[i]
		if sx < b.x || sx >= b.x+b.w || sy < b.y || sy >= b.y+b.h {
			continue
		}
		if b.cov.At(sx-b.x, sy-b.y) <= 0 {
			continue
		}
		return b.id % GridSize, b.id / GridSize, true
	}

	if id, found := m.anchorLookup(sx, sy); found {
		return id % GridSize, id / GridSize, true
	}

	// Nothing was drawn there. On the flat grid the gaps between digits belong
	// to a cell; in the projected view they do not, and CellAt's flat mapping
	// would name a cell nowhere near the pointer.
	if m.solid != nil {
		return 0, 0, false
	}
	return m.Layout.CellAt(sx, sy)
}

// anchorStamp is the layout an index was built against. The index is read
// during Update against the previous frame, and the viewport scrolls on every
// cursor move, so a mismatched stamp sends HitTest to the live Layout instead.
type anchorStamp struct {
	viewX, viewY  int
	width, height int

	// The eased spacing. The board can be respaced without the terminal
	// changing size, and resetAnchors reallocates only when width*height
	// changes. Compared exactly: these are copies of one value, never
	// recomputed, and nothing is stepped between a Draw and a mouse message.
	cellW, rowH float64

	// Which board, and which file of it. Draw returns early while a finished
	// file is held, and the room then swaps the grid with the geometry
	// unchanged; rejoining a room at the same size does the same.
	room  *Room
	epoch int

	// Whether the index was built by the projected view, which places cells
	// differently from the flat grid.
	solid bool
}

// stampNow is the board and layout as they stand.
func (m *Model) stampNow() anchorStamp {
	l := m.Layout
	st := anchorStamp{
		viewX: l.ViewportX, viewY: l.ViewportY,
		width: l.Width, height: l.Height,
		cellW: l.CellW, rowH: l.RowH,
	}
	if m.Room != nil {
		st.room, st.epoch = m.Room, m.Room.Epoch()
	}
	st.solid = m.solid != nil
	return st
}

// dropIndex abandons everything the last frame recorded, for a view now looking
// at a different board. It clears the glyph boxes too, because HitTest
// consults them first.
func (m *Model) dropIndex() {
	clear(m.anchors)
	m.boxes = m.boxes[:0]
	m.anchorAt = anchorStamp{}
}

// resetAnchors empties the index for a new frame, growing it if the terminal
// has. The index is a flat screen-sized slice because a map, written across
// every cell's footprint, cost about a third of the frame budget.
func (m *Model) resetAnchors() {
	w, h := m.Layout.Width, m.Layout.Height
	if n := w * h; len(m.anchors) != n {
		m.anchors = make([]int32, n)
		m.anchorFar = make([]uint8, n)
	}
	clear(m.anchors)
	clear(m.anchorFar)
	m.anchorAt = m.stampNow()

	// Nameplates are per frame too; leftovers would accumulate all session.
	m.plates = m.plates[:0]
}

// markAnchor records that a cell was drawn at a screen position, over the cell's
// whole footprint, so a click in the gap between two digits picks the nearer
// one, as CellAt does.
//
// Cells are stored as grid ID + 1 so that zero means "nothing drawn" and the
// clear is a memclr.
//
// The lens compresses digits closer than a cell apart, so footprints overlap.
// Each stamp records its distance from its own digit, and a nearer claim is
// never overwritten by a further one; otherwise a click on a digit could name
// its neighbour.
func (m *Model) markAnchor(gridID, x, y int) {
	w, h := m.Layout.Width, m.Layout.Height
	if len(m.anchors) != w*h {
		// planGlyphs can run without a full Draw (tests do), leaving no index.
		// HitTest then falls back to Layout.
		return
	}
	left, right, top, bottom := m.Layout.CellBoxAt(gridID%GridSize, gridID/GridSize)
	for dy := top; dy < bottom; dy++ {
		yy := y + dy
		if yy < 0 || yy >= h {
			continue
		}
		row := yy * w
		for dx := left; dx < right; dx++ {
			xx := x + dx
			if xx < 0 || xx >= w {
				continue
			}
			far := uint8(min(abs(dx)+abs(dy), 255))
			if m.anchors[row+xx] != 0 && m.anchorFar[row+xx] <= far {
				continue
			}
			m.anchors[row+xx] = int32(gridID) + 1
			m.anchorFar[row+xx] = far
		}
	}
}

// markAnchorAt records a digit at exactly the cell it was drawn in, with no
// footprint, for the projected view where the gaps between digits belong to no
// cell. It stamps at distance zero, so it always wins a contested cell.
func (m *Model) markAnchorAt(gridID, x, y int) {
	w, h := m.Layout.Width, m.Layout.Height
	if len(m.anchors) != w*h || x < 0 || y < 0 || x >= w || y >= h {
		return
	}
	m.anchors[y*w+x] = int32(gridID) + 1
	m.anchorFar[y*w+x] = 0
}

// markAnchorBox records a glyph over the cells it inked, skipping empty ones.
// This matches HitTest's glyph-box check, where a click through the hole in an
// 8 hits nothing; the projected view has no planned boxes, so the index applies
// the same rule.
func (m *Model) markAnchorBox(gridID, x, y int, cov *glyph.Coverage) {
	for dy := 0; dy < cov.H; dy++ {
		for dx := 0; dx < cov.W; dx++ {
			if cov.At(dx, dy) > 0 {
				m.markAnchorAt(gridID, x+dx, y+dy)
			}
		}
	}
}

// anchorLookup is which cell was drawn at a screen position last frame.
func (m *Model) anchorLookup(sx, sy int) (gridID int, ok bool) {
	w, h := m.Layout.Width, m.Layout.Height
	if len(m.anchors) != w*h {
		return 0, false
	}
	if sx < 0 || sx >= w || sy < 0 || sy >= h {
		return 0, false
	}
	if id := m.anchors[sy*w+sx]; id > 0 {
		return int(id) - 1, true
	}
	return 0, false
}

// PlainAnchor is where a digit drawn as an ordinary character ends up.
//
// It differs from ScreenAnchor only in that a single character lands on a whole
// cell, so its own motion goes through the Schmitt triggers that stop it
// flickering between two columns. Both must apply the lenses (through Bulge)
// and the ripple identically, or a click resolves against a different place
// from the one the player sees.
func (m *Model) PlainAnchor(gridID int) (x, y int) {
	gx, gy := gridID%GridSize, gridID/GridSize
	sx, sy := m.Layout.ScreenOf(gx, gy)

	bx, by := m.Bulge(float64(sx), float64(sy))
	x, y = int(math.Round(bx)), int(math.Round(by))

	shift := m.Agitation.ColumnOffset(m.Layout, gridID)
	sag := m.Agitation.RowOffset(m.Layout, gridID)
	if shift == 0 && sag == 0 {
		shift = m.Drift.ColumnOffset(m.Layout, gx, gy)
		sag = m.Drift.RowOffset(m.Layout, gx, gy)
	}

	// The ripple bypasses the Schmitt triggers: it is large and brief and
	// returns exactly home, so there is no resting flicker to suppress. rowStep
	// also clamps to [0, maxRowSag], which would stop it throwing a digit up.
	rx, ry := m.rippleOffset(gx, gy)
	ox, oy := m.Layout.Screen(rx, ry)
	x += int(math.Round(ox))
	y += int(math.Round(oy))

	// Keep the digit off the rules above and below the board, which the radial
	// bulge can otherwise push the top or bottom row onto. Clamped here, not in
	// the draw, so the anchor index agrees with what is drawn.
	l := m.Layout
	return x + shift, clampInt(y+sag, l.BoardY, l.BoardY+l.BoardH-1)
}
