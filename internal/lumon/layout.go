package lumon

import "math"

// Screen chrome, in rows. The grid gets whatever vertical space is left.
const (
	// HeaderHeight is the plain file bar: a border, one line, a border, for a
	// terminal that cannot spare the rows for the mark.
	HeaderHeight = 3

	// HeaderTall is the bar with the Lumon mark closing its right end: a border,
	// the five rows SmallLogo occupies, a border. The mark is never drawn
	// smaller, since at three rows its ring collapses into a smear.
	HeaderTall = 7

	// headerTallCols is how wide a terminal has to be before the bar takes the
	// taller form; headerTallRows, below, is how tall.
	headerTallCols = 64

	RuleHeight = 2 // double rule below the grid

	// TopRuleHeight is the rule over the grid: a rule, the status row where the
	// floor news and LAN requests go, and a rule. The status row is kept when it
	// is empty, so the board never moves when news comes and goes.
	TopRuleHeight = 3
	BinsHeight    = 6 // bin box (3) + percentage box (3)
	// FooterHeight is two rows: the readout and the banner, then the hint on its
	// own row so it never has to truncate to make room for them. There used to be
	// a blank row under the bins; it became the status row over the grid (see
	// TopRuleHeight), so the board is the size it was. headerTallRows derives
	// from this constant.
	FooterHeight = 2

	// binInsetCells is how far inside the board's edges the row of bins is
	// held, in default cells. The bins are the board's destinations, so they
	// are measured against the board; nothing else on screen lines up with a
	// fixed column. Two cells is 14 to 16 columns at each end at ordinary widths.
	binInsetCells = 2

	// binMinWidth is the narrowest a bin may be before the inset is given back:
	// enough for its walls, its two-digit number and a percentage either side of
	// centre.
	binMinWidth = 12

	// CellWidth is how many columns one digit occupies at the default spacing,
	// with the digit centred in it, and RowPitch how many screen rows one grid
	// row takes. A glyph is glyph.CellAspect times as wide as it is tall, so six
	// columns lets two adjacent digits both grow to about five rows. These are
	// zoom level zero; the live spacing is Layout.CellW and Layout.RowH.
	CellWidth = 6
	RowPitch  = 2
)

// zoomLevels is how far apart the digits are set, in columns per cell and screen
// rows per grid row. Level zero is the default board.
//
// Zoom only goes in, so the pitch never falls below two. maxRowSag depends on
// that: at a pitch of two a sagging digit stays in rows {y, y+1} while its
// neighbour owns {y+2, y+3}; at a pitch of one digits overprint.
//
// Every level keeps CellW to RowH*RowAspect at three to two. The lens's reach
// scales off the cell width alone, so any other aspect would make it an
// ellipse. TestTheZoomLevelsKeepTheCellsShape enforces this.
var zoomLevels = [...][2]int{{CellWidth, RowPitch}, {9, 3}, {12, 4}}

const (
	// zoomFrequency is how fast the board settles into a new spacing, in radians
	// a second. It is the Rig's number, since both are a keystroke moving a
	// viewpoint.
	zoomFrequency = rigFrequency

	// zoomSettled and zoomStill are how close counts as arrived, in levels and
	// in levels a second. A spring only approaches its target, so without them
	// the layout is recomputed for ever. A hundredth of a level is three
	// hundredths of a column, so the final snap is invisible; a thousandth
	// would spend two thirds of the journey's recomputes after visible motion
	// stops.
	zoomSettled = 0.01
	zoomStill   = 0.01
)

// Layout is the single source of truth for where things are on screen. Drawing
// and hit-testing both go through it, and ScreenOf and CellAt are tested as
// inverses, so a click can never land somewhere other than where it was drawn.
type Layout struct {
	Width, Height int

	// CellW is how many columns one digit occupies, with the digit centred in
	// it, and RowH how many screen rows one grid row takes. They are per
	// terminal, and on Layout so a viewer changing its spacing never touches the
	// board. They are fractional so a zoom can ease; between levels the cells
	// are not all the same size, so every footprint, boundary and inverse in
	// this file goes through colLeft and rowTop instead of dividing by them.
	CellW float64
	RowH  float64

	// zoom is where CellW and RowH sit in zoomLevels, fractional while a zoom
	// is travelling: 0.5 is half way between the first two levels. want is the
	// target level and wantCellW/wantRowH its spacing; MaxRowSag needs both
	// ends of the journey. The travelling itself is Model's, on a spring.
	zoom, want          float64
	wantCellW, wantRowH float64

	// colAt and rowAt are where each cell's columns and rows begin, relative to
	// GridOriginX and GridTop, with one extra entry for the far edge, and colMid
	// where each cell's centre falls. See bounds.
	colAt, rowAt, colMid []int

	// HeaderH is how many rows the file bar takes: HeaderTall where there is
	// room for the mark, HeaderHeight where there is not.
	HeaderH int

	// BoardX, BoardY, BoardW, BoardH are the screen rectangle the digits are
	// drawn inside: the hole between the two rules.
	//
	// It is measured at the default spacing, never the live one, so a zoom
	// cannot move the chrome: the header, the rules, the bins, the glyph clip
	// and the diversion panel are all measured against it. Only ViewportW,
	// ViewportH and GridOriginX follow the cell. The leftover `BoardH % RowH`
	// rows stay inside the rectangle; deriving BinTop from
	// GridTop + ViewportH*RowH makes the bins jump as the zoom changes.
	BoardX, BoardY int
	BoardW, BoardH int

	GridTop     int // first screen row of the grid; the rectangle's top
	GridOriginX int // screen column of the leftmost cell
	ViewportW   int // visible cells across
	ViewportH   int // visible cells down

	ViewportX int // grid coordinate at the left edge
	ViewportY int // grid coordinate at the top edge

	BinTop     int
	BinCenterX [BinCount]int

	// binWidth and binOrigin tile the width the bins are given. The remainder
	// of the division goes to the leftmost bins, so no gap is left at the right
	// edge.
	binWidth  [BinCount]int
	binOrigin [BinCount]int
}

// clusterRows is how many grid rows the board must be able to hold: a corrupt
// cluster at MaxGlyphHeight is eleven screen rows, six grid rows at RowPitch,
// plus one so it is not drawn flush against a rule.
const clusterRows = 7

// headerTallRows is how tall a terminal has to be before the bar takes the
// taller form. Four extra rows of chrome is two grid rows. It is derived so
// the board can still hold clusterRows at RowPitch beside the tall bar.
//
// It uses RowPitch whatever the zoom: a zoom must never change the header.
// Zoomed in, the board shows fewer rows instead.
const headerTallRows = HeaderTall + TopRuleHeight + RuleHeight + BinsHeight + FooterHeight + clusterRows*RowPitch

// MaxCols and MaxRows are the largest terminal this will believe in.
//
// The size comes from the client: remotely it is a uint32 in a window-change
// request, and the frame is allocated from it at about eighteen bytes a cell.
// A Go out-of-memory is fatal, not a panic, so no recovery middleware catches
// it, and one unauthenticated client asking for 100000x100000 would take down
// every refiner on the server. 500x200 is past any actual terminal and under
// two megabytes a session.
//
// The bound is on Layout, not the buffer, because every drawing decision is
// made against Layout; clamping only the buffer would lay out a board for the
// requested size and draw it into a smaller one.
const (
	MaxCols = 500
	MaxRows = 200
)

// NewLayout computes the layout for a terminal of the given size, at the default
// spacing. Every layout is built here, including on window-change, so the size
// bound is applied here.
func NewLayout(width, height int) *Layout {
	l := &Layout{
		Width:  clampInt(width, 1, MaxCols),
		Height: clampInt(height, 1, MaxRows),
	}
	l.applyZoom(0)
	l.aimZoom(0)
	l.recompute()
	return l
}

// applyZoom sets the spacing and nothing else; recompute turns a spacing into
// a screen. The level is interpolated, not width and height separately, so the
// cell keeps its shape (see zoomLevels) at every spacing in between.
func (l *Layout) applyZoom(z float64) {
	l.zoom = clampF(z, 0, float64(len(zoomLevels)-1))
	l.CellW, l.RowH = spacingAt(l.zoom)
}

// aimZoom records where a zoom is going without moving the board there.
func (l *Layout) aimZoom(z float64) {
	l.want = clampF(z, 0, float64(len(zoomLevels)-1))
	l.wantCellW, l.wantRowH = spacingAt(l.want)
}

// spacingAt is the cell size at a fractional level.
func spacingAt(z float64) (cellW, rowH float64) {
	lo := clampInt(int(z), 0, len(zoomLevels)-2)
	t := z - float64(lo)
	hi := lo + 1
	return float64(zoomLevels[lo][0]) + t*float64(zoomLevels[hi][0]-zoomLevels[lo][0]),
		float64(zoomLevels[lo][1]) + t*float64(zoomLevels[hi][1]-zoomLevels[lo][1])
}

// Zoom is where the board is spaced right now, fractional while a zoom is
// travelling; WantZoom is where it is going and MaxZoom the furthest in it goes.
func (l *Layout) Zoom() float64     { return l.zoom }
func (l *Layout) WantZoom() float64 { return l.want }
func (l *Layout) MaxZoom() int      { return len(zoomLevels) - 1 }

// MaxRowSag is how far a digit may drop below its own row, in whole screen rows.
//
// It is bounded by the narrower of the current and target row heights. With
// the current pitch alone, a digit commits to two rows of sag at a pitch of
// three and overlaps the row below as the pitch returns to two. With the
// target alone, it gets room the board has not made yet. A digit meeting the
// row below vanishes, which is worse than a wobble that does not use all its
// travel.
func (l *Layout) MaxRowSag() int { return maxRowSag(math.Min(l.RowH, l.wantRowH)) }

// SetZoom respaces the board at once, reporting whether anything changed.
// Model.Zoom is the animated path; this is for rebuilding after a resize and
// for tests.
//
// It holds the cell at the viewport's centre in place. Holding a whole cell
// means repeated calls do not creep; re-centring on a fractional centre would
// round by up to a cell each time.
func (l *Layout) SetZoom(z float64) bool {
	z = clampF(z, 0, float64(len(zoomLevels)-1))
	if z == l.zoom && z == l.want {
		return false
	}
	l.applyZoom(z)
	l.aimZoom(z)
	l.respace(l.ViewportX+l.ViewportW/2, l.ViewportY+l.ViewportH/2)
	return true
}

// respace recomputes what the spacing decides, holding the given cell as near
// the middle of the viewport as the grid's edges allow.
func (l *Layout) respace(holdX, holdY int) {
	l.recompute()
	l.ScrollTo(holdX-l.ViewportW/2, holdY-l.ViewportH/2)
}

// recompute is everything that follows from a size and a spacing: the header's
// form, the viewport, the origin, the bins.
func (l *Layout) recompute() {
	width, height := l.Width, l.Height

	l.HeaderH = HeaderHeight
	if height >= headerTallRows && width >= headerTallCols {
		l.HeaderH = HeaderTall
	}

	// The rectangle first, at the default spacing, because every piece of chrome
	// below hangs off it and none of it may move when the cell does.
	l.BoardH = height - l.HeaderH - TopRuleHeight - RuleHeight - BinsHeight - FooterHeight
	if l.BoardH < 1 {
		l.BoardH = 1
	}
	l.BoardY = l.HeaderH + TopRuleHeight
	l.BoardW = clampInt((width-4)/CellWidth, 1, GridSize) * CellWidth
	l.BoardX = (width - l.BoardW) / 2
	if l.BoardX < 0 {
		l.BoardX = 0
	}

	// Then the cells inside it, the only things the spacing moves.
	l.GridTop = l.BoardY
	l.ViewportW = clampInt(int(float64(l.BoardW)/l.CellW), 1, GridSize)
	l.ViewportH = clampInt(int(float64(l.BoardH)/l.RowH), 1, GridSize)
	l.GridOriginX = l.BoardX + (l.BoardW-l.colLeft(l.ViewportW))/2

	l.BinTop = l.BoardY + l.BoardH + RuleHeight

	// The bins stand under the board, inset from its edges in default cells so
	// the zoom does not move them. On a terminal too narrow, first the inset
	// gives way, then the board's edges, since a bin one column wide is worse
	// than a bin against the wall.
	inset := l.BoardX + binInsetCells*CellWidth
	span := l.BoardW - 2*binInsetCells*CellWidth
	if span < binMinWidth*BinCount {
		inset, span = l.BoardX, l.BoardW
	}
	if span < 2*BinCount || inset < 0 {
		inset, span = 0, width
	}
	base, extra := span/BinCount, span%BinCount
	x := inset
	for i := range l.binWidth {
		w := base
		if i < extra {
			w++
		}
		if w < 1 {
			w = 1
		}
		l.binWidth[i] = w
		l.binOrigin[i] = x
		l.BinCenterX[i] = x + w/2
		x += w
	}

	l.bounds()
	l.ScrollTo(l.ViewportX, l.ViewportY)
}

// colLeft is the first screen column of grid column c, counted from
// GridOriginX, and rowTop the first screen row of grid row r, from GridTop.
//
// ScreenOf, CellAt and CellBoxAt all go through these two, so they agree on
// cell boundaries at every spacing. Truncation makes the cells a partition:
// column c owns [colLeft(c), colLeft(c+1)), so every screen column belongs to
// exactly one cell, even between zoom levels where cells differ in width.
func (l *Layout) colLeft(c int) int {
	if c < 0 || c >= len(l.colAt) {
		return int(float64(c) * l.CellW)
	}
	return l.colAt[c]
}

// colCentre is where a cell's digit is drawn: the middle of its own columns.
func (l *Layout) colCentre(c int) int {
	if c < 0 || c >= len(l.colMid) {
		left := l.colLeft(c)
		return left + (l.colLeft(c+1)-left)/2
	}
	return l.colMid[c]
}

func (l *Layout) rowTop(r int) int {
	if r < 0 || r >= len(l.rowAt) {
		return int(float64(r) * l.RowH)
	}
	return l.rowAt[r]
}

// bounds fills the two tables. Consecutive boundaries differ by floor(pitch)
// or ceil(pitch), which at a pitch of two or more is never zero, so the cells
// are a partition. They are tabulated because ScreenOf runs for every visible
// cell every frame; recompute runs only on resize and while a zoom travels.
//
// Outside the table the arithmetic answers instead. CellAt rejects those
// positions, but another refiner's lens may be aimed at a cell that has
// scrolled off, which must project off the board, not onto its edge.
func (l *Layout) bounds() {
	l.colAt = grow(l.colAt, l.ViewportW+1)
	l.rowAt = grow(l.rowAt, l.ViewportH+1)
	l.colMid = grow(l.colMid, l.ViewportW)
	for i := range l.colAt {
		l.colAt[i] = int(float64(i) * l.CellW)
	}
	for i := range l.rowAt {
		l.rowAt[i] = int(float64(i) * l.RowH)
	}
	// The centre comes from the boundaries, so it is inside the cell's own
	// box however they rounded. Tabulated too, as ScreenOf is the hottest path
	// in the package.
	for i := range l.colMid {
		l.colMid[i] = l.colAt[i] + (l.colAt[i+1]-l.colAt[i])/2
	}
}

// grow returns s resized to n, reusing its array when it is big enough, so a
// travelling zoom does not allocate every frame.
func grow(s []int, n int) []int {
	if cap(s) >= n {
		return s[:n]
	}
	return make([]int, n, GridSize+1)
}

// CellBoxAt is a cell's screen footprint, as offsets from the centre ScreenOf
// returns: half-open, so [left, right) columns and [top, bottom) rows. It is per
// cell because between zoom levels adjacent cells differ by a column.
func (l *Layout) CellBoxAt(gridX, gridY int) (left, right, top, bottom int) {
	c, r := gridX-l.ViewportX, gridY-l.ViewportY
	w := l.colLeft(c+1) - l.colLeft(c)
	return -(w / 2), w - w/2, 0, l.rowTop(r+1) - l.rowTop(r)
}

// FisheyeRadius, LensPush and MaxBulge are the lens's reach, in visual columns,
// scaled by the live cell width. LensPush stays under FisheyeRadius/4 at every
// level because both are linear in the cell width (0.5 against 0.667), so the
// lens never folds.
func (l *Layout) FisheyeRadius() float64 { return fisheyeReachCells * l.CellW }
func (l *Layout) LensPush() float64      { return lensPushCells * l.CellW }
func (l *Layout) MaxBulge() float64      { return l.FisheyeRadius() / 4 }

// solidRowGap is how far apart the volume sets its rows, in visual columns.
func (l *Layout) solidRowGap() float64 { return solidRowGapCells * l.CellW }

// Screen converts a displacement in cell units into screen columns and rows.
// Use it everywhere, so the renderer and the mouse agree how far a digit has
// moved.
func (l *Layout) Screen(dx, dy float64) (float64, float64) {
	return dx * l.CellW, dy * l.RowH
}

// ScrollTo moves the viewport, clamping so it always sits fully inside the grid.
// A scroll does not go through recompute, so anything that follows the
// viewport must follow it from here.
func (l *Layout) ScrollTo(x, y int) {
	l.ViewportX = clampInt(x, 0, GridSize-l.ViewportW)
	l.ViewportY = clampInt(y, 0, GridSize-l.ViewportH)
}

// ScreenOf returns the screen position of a grid cell's centre column and the
// top of its rows. A grid row draws on the first of its RowH screen rows and
// leaves the rest blank, which is the space maxRowSag sags into.
func (l *Layout) ScreenOf(gridX, gridY int) (x, y int) {
	col, row := gridX-l.ViewportX, gridY-l.ViewportY
	return l.GridOriginX + l.colCentre(col), l.GridTop + l.rowTop(row)
}

// CellAt returns the grid cell under a screen position, and whether the position
// is over the grid at all.
//
// It searches from the divided estimate, since division alone misses at
// fractional spacing: at a pitch of 3.5 row 3 starts at screen row 10, and
// 10/3.5 is 2.86. The search makes CellAt the exact inverse of ScreenOf (as
// layout_test.go asserts) in one or two comparisons.
func (l *Layout) CellAt(x, y int) (gridX, gridY int, ok bool) {
	dy := y - l.GridTop
	if dy < 0 || dy >= l.rowTop(l.ViewportH) {
		return 0, 0, false
	}
	dx := x - l.GridOriginX
	if dx < 0 || dx >= l.colLeft(l.ViewportW) {
		return 0, 0, false
	}

	col := clampInt(int(float64(dx)/l.CellW), 0, l.ViewportW-1)
	for col > 0 && l.colLeft(col) > dx {
		col--
	}
	for col < l.ViewportW-1 && l.colLeft(col+1) <= dx {
		col++
	}

	// A blank row between numbers selects the number above it; rejecting those
	// rows would make half the grid unclickable.
	row := clampInt(int(float64(dy)/l.RowH), 0, l.ViewportH-1)
	for row > 0 && l.rowTop(row) > dy {
		row--
	}
	for row < l.ViewportH-1 && l.rowTop(row+1) <= dy {
		row++
	}

	return l.ViewportX + col, l.ViewportY + row, true
}

// BinWidth returns how many columns bin i occupies.
func (l *Layout) BinWidth(i int) int { return l.binWidth[i] }

// BinOriginX returns the leftmost column of bin i.
func (l *Layout) BinOriginX(i int) int { return l.binOrigin[i] }

// FooterY is the row with the address readout, the banner and the roster.
// Call sites must use it rather than counting from the bottom.
// row above.
func (l *Layout) FooterY() int { return l.Height - FooterHeight }

// HintY is the last row of the terminal.
func (l *Layout) HintY() int { return l.Height - 1 }

// BinAt returns the bin index under a screen position, and whether the position
// is over the bin row.
func (l *Layout) BinAt(x, y int) (idx int, ok bool) {
	if y < l.BinTop || y >= l.BinTop+BinsHeight {
		return 0, false
	}
	for i := BinCount - 1; i >= 0; i-- {
		if x >= l.binOrigin[i] && x < l.binOrigin[i]+l.binWidth[i] {
			return i, true
		}
	}
	return 0, false
}

func clampInt(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
