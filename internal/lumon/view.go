package lumon

import (
	"fmt"
	"github.com/terminaltrove/lumon/internal/glyph"
	"github.com/terminaltrove/lumon/internal/theme"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// View renders a full frame. It holds the board's read lock, so players can
// draw concurrently; everything Draw writes (glyph boxes, hysteresis maps) is
// this player's own.
func (m *Model) View() string {
	if m.Quitting {
		return ""
	}
	if m.Room != nil {
		m.Room.mu.RLock()
		defer m.Room.mu.RUnlock()
	}
	// The frame is reused between calls. Safe because Bubble Tea calls Update
	// and View sequentially per session and nothing keeps a buffer past the
	// call that drew it.
	m.frame = m.frame.Reuse(m.Layout.Width, m.Layout.Height, m.Theme.Page)
	m.Draw(m.frame)
	m.sweepMark(m.frame)
	return m.frame.StringWith(m.styler)
}

// ViewCells draws a frame and returns its cells, for a host that sends cells
// instead of text (a Claude Code pane). The buffer is reused by the next call.
func (m *Model) ViewCells() *canvas.Buffer {
	if m.Room != nil {
		m.Room.mu.RLock()
		defer m.Room.mu.RUnlock()
	}
	m.frame = m.frame.Reuse(m.Layout.Width, m.Layout.Height, m.Theme.Page)
	m.Draw(m.frame)
	return m.frame
}

// ViewChanged draws a frame and returns only the rows that differ from the last
// one d saw, positioned with cursor moves. The browser host uses it. It must
// stay byte-identical to Render(View())
// (TestTheBrowserSendsTheSameBytesWhicheverWayItDiffs).
func (m *Model) ViewChanged(d *canvas.FrameDiffer) string {
	if m.Quitting {
		return ""
	}
	if m.Room != nil {
		m.Room.mu.RLock()
		defer m.Room.mu.RUnlock()
	}
	m.frame = m.frame.Reuse(m.Layout.Width, m.Layout.Height, m.Theme.Page)
	m.Draw(m.frame)
	m.sweepMark(m.frame)
	return d.RenderBuffer(m.frame, m.styler)
}

// Draw composites the screen into one buffer, so enlarged digits and
// flights can draw across region boundaries.
func (m *Model) Draw(buf *canvas.Buffer) {
	m.draw(buf)

	// Last, over whatever screen was drawn.
	if m.Room != nil && m.Screen == ScreenMain && m.Room.Complete() {
		// Fireworks first, so the shells pass over the board and under the
		// notice.
		if m.finale != nil {
			m.finale.Draw(buf, m.Theme, m.gridPanel().Play())
		}
		m.drawComplete(buf)
	}
	if m.divertOpen {
		m.drawDiversions(buf)
	}
	if m.About {
		m.drawAbout(buf)
	}
	if m.Helping {
		m.drawHelp(buf)
	}
	if m.exitOpen {
		m.drawExit(buf)
	}
	if m.calloutOpen {
		m.drawCalloutMenu(buf)
	}
	if m.Screen == ScreenLobby {
		m.drawLANBanner(buf)
	}
	if m.lanUI.step != lanClosed {
		m.drawLAN(buf)
	}
	// Over even the controls, since escape can be pressed at them.
	if m.Escaped {
		m.drawEscape(buf)
	}
	// Over everything: it arrives unasked, so it must not hide under another.
	if m.agentNote != "" {
		m.drawNotice(buf, agentTitle, m.agentNote, agentShort, m.Theme.Header)
	}
	// Somebody is waiting on this, so nothing covers it.
	m.drawLANRequest(buf)
}

func (m *Model) draw(buf *canvas.Buffer) {
	// A board still arriving is drawn in a dimmed palette, which fades all the
	// chrome together. See theme.Theme.Dimmed.
	if m.arrival != nil {
		defer func(t *theme.Theme) { m.Theme = t }(m.Theme)
		m.Theme = m.Theme.Dimmed(m.arrival.Level())
	}

	if m.Screen == ScreenOff {
		m.drawOff(buf)
		return
	}
	if m.Screen == ScreenIntro && m.Intro != nil {
		m.drawIntro(buf)
		return
	}
	if m.Screen == ScreenName {
		m.drawName(buf)
		return
	}
	if m.Screen == ScreenLobby {
		m.drawLobby(buf)
		return
	}
	if m.Screen == ScreenScores {
		m.drawScores(buf)
		return
	}
	if m.Screen == ScreenIdle {
		m.drawIdle(buf)
		return
	}

	// The fullscreen mark (see markPanel) returns here, because drawDiversion
	// is followed by the bins and footer, which would paint over it.
	if m.mark != nil && m.mark.Fullscreen() {
		m.mark.Draw(buf, m.Theme, m.markPanel())
		return
	}
	// A session that has not taken a desk has no board to draw.
	if m.Room == nil {
		return
	}
	// A finished file still draws the board, with the notice as a modal over
	// it (drawComplete); advanceFile has put every cluster to sleep.
	l := m.Layout

	// The index clicks are resolved against is rebuilt every frame.
	m.resetAnchors()

	// Plan glyphs first so the grid pass can leave out digits drawn large. The
	// projected view paints no glyphs, and HitTest consults planned boxes
	// before the index, so plan none there.
	if m.solid != nil {
		m.boxes = m.boxes[:0]
		clear(m.enlarged)
	} else {
		m.planGlyphs()
	}

	m.drawHeader(buf)
	m.drawTopRule(buf)
	m.drawFloorNews(buf)

	// A diversion replaces only the grid; the bar and bins stay live.
	switch {
	case m.Diverting():
		m.drawDiversion(buf)
	case m.solid != nil:
		m.drawSolid(buf)
	default:
		m.drawGrid(buf)
	}

	m.drawRule(buf, l.BoardY+l.BoardH)
	m.drawBins(buf)

	// The diversion's hint is painted after the bins: an open lid stands
	// lidRise rows above BinTop, inside the panel, and its flaps overhang the
	// hint's row.
	if m.Diverting() {
		m.drawHint(buf, m.gridPanel())
	}

	m.drawMinimap(buf)
	m.drawFooter(buf)

	// Enlarged digits sit above the grid and flights above those, so both pass
	// over the rules and the bins. A diversion has neither.
	//
	// Glyphs are placed by the flat layout, so the projected view skips them
	// and draws its own. Flights are drawn in both: they start from wherever
	// the digit was drawn and land in the bins, which are flat screen space.
	if !m.Diverting() {
		if m.solid == nil {
			m.paintGlyphs(buf)
		}
		m.drawSweep(buf)
		m.drawFlights(buf)

		// Nameplates go on top of everything, flights included, so they stay
		// readable.
		m.drawNameplates(buf)
	}
}

// drawNameplates puts every refiner's name over their cursor when more than
// one is seated, and draws their callouts. Your own tag says YOU.
func (m *Model) drawNameplates(buf *canvas.Buffer) {
	if m.Room == nil {
		return
	}
	l := m.Layout
	company := m.Seated() > 1
	for _, d := range m.Roster() {
		if d == nil {
			continue
		}
		// Their cursor is outside this terminal's viewport.
		if d.CursorX < l.ViewportX || d.CursorX >= l.ViewportX+l.ViewportW ||
			d.CursorY < l.ViewportY || d.CursorY >= l.ViewportY+l.ViewportH {
			continue
		}
		x, y := m.plateAnchor(d)
		m.drawCallout(buf, d, x, y)
		if company {
			m.drawPlate(buf, x, y-1, d)
		}
	}
}

// plateAnchor is where a desk's tag hangs from: the sprung focus of that
// refiner's lens (m.Lens or m.remoteLens), in screen cells.
//
// Reusing the lens keeps the tag on its owner's fisheye and inherits its
// handling: trackRemoteLenses places a new lens at its owner's cursor, and
// reprojectLenses moves lenses without travel on a resize or scroll. A
// separate spring would sweep in from the corner or slide on every scroll.
func (m *Model) plateAnchor(d *Desk) (x, y int) {
	lens := m.Lens
	if d != m.Desk {
		lens = m.remoteLens[d.Seat]
	}
	// trackRemoteLenses sets lenses each frame; tests may draw without
	// stepping first.
	if lens == nil {
		sx, sy := m.Layout.ScreenOf(d.CursorX, d.CursorY)
		return sx, sy
	}
	f := lens.Focus()
	return int(math.Round(f.X)), int(math.Round(f.Y))
}

// drawPlate writes one tag centred over a column, clipped into the board.
func (m *Model) drawPlate(buf *canvas.Buffer, cx, y int, d *Desk) {
	l := m.Layout

	// A row above the cursor, or below it on the board's first row, and never
	// off the board.
	if y < l.GridTop {
		y += 2
	}
	if y < l.GridTop || y >= l.BoardY+l.BoardH {
		return
	}

	name := plateName(d, m.Desk, l.Width)
	wide := len([]rune(name))
	x := cx - wide/2
	if x < 0 {
		x = 0
	}
	if x+wide > l.Width {
		x = l.Width - wide
	}
	if x < 0 {
		return
	}

	buf.Text(x, y, name, m.Theme.DeskCursor(d.Seat))

	// Recorded because tags differ between two refiners in one room, and the
	// test comparing two terminals rune for rune must skip them.
	m.plates = append(m.plates, plateRect{Seat: d.Seat, X: x, Y: y, W: wide})
}

// plateRect is where one tag was drawn.
type plateRect struct{ Seat, X, Y, W int }

// plateName is what a tag says, padded with a space either side so the colour
// reads as a plate. The full name is used, truncated only on a terminal too
// narrow for it.
func plateName(d, mine *Desk, width int) string {
	name := "YOU"
	if d != mine {
		name = strings.ToUpper(strings.TrimSpace(d.Name))
		if name == "" {
			name = "?"
		}
	}
	if max := width - 2; max > 0 && len([]rune(name)) > max {
		name = string([]rune(name)[:max])
	}
	return " " + name + " "
}

// drawSweep outlines the ringfence while it is being drawn. It shows only the
// gesture; a ringfenced cluster is shown by its digits being enlarged.
//
// Dashed and in Dim so it stays quieter than the selection, and clipped to the
// board so it cannot draw through the rules or the bins.
func (m *Model) drawSweep(buf *canvas.Buffer) {
	if m.sweep == nil || m.solid != nil {
		return
	}
	l := m.Layout
	gx0, gy0, gx1, gy1 := m.sweep.bounds()

	x0, y0 := l.ScreenOf(gx0, gy0)
	x1, y1 := l.ScreenOf(gx1, gy1)
	left, _, top, _ := l.CellBoxAt(gx0, gy0)
	_, r1, _, b1 := l.CellBoxAt(gx1, gy1)
	x0, y0 = x0+left-1, y0+top-1
	x1, y1 = x1+r1, y1+b1

	attr := canvas.Attr{FG: m.Theme.Dim}
	clipTop, clipBottom := l.BoardY, l.BoardY+l.BoardH-1
	row := func(y int, r rune) {
		if y < clipTop || y > clipBottom {
			return
		}
		for x := x0; x <= x1; x++ {
			buf.Set(x, y, r, attr)
		}
	}
	row(y0, '┄')
	row(y1, '┄')
	for y := y0 + 1; y < y1; y++ {
		if y < clipTop || y > clipBottom {
			continue
		}
		buf.Set(x0, y, '┊', attr)
		buf.Set(x1, y, '┊', attr)
	}
	if y0 >= clipTop && y0 <= clipBottom {
		buf.Set(x0, y0, '┌', attr)
		buf.Set(x1, y0, '┐', attr)
	}
	if y1 >= clipTop && y1 <= clipBottom {
		buf.Set(x0, y1, '└', attr)
		buf.Set(x1, y1, '┘', attr)
	}
}

// glyphBox is one enlarged digit's footprint on screen.
type glyphBox struct {
	id         int
	x, y, w, h int
	height     float64
	cov        *glyph.Coverage

	// solid is which rendering it takes: a silhouette in blocks, or its
	// outline drawn in copies of its own digit. Corrupt (awake) digits are
	// solid. It is decided by asking the board, not by height, and must be
	// known before rasterising because the two renderings use different pens.
	solid bool
}

// swallowedBy reports whether a's centre falls inside b. Glyphs may overlap;
// only covering a centre is refused, so a cluster reads as one dense mass with
// each digit's core legible.
func (a glyphBox) swallowedBy(b glyphBox) bool {
	cx, cy := a.x+a.w/2, a.y+a.h/2
	return cx >= b.x && cx < b.x+b.w && cy >= b.y && cy < b.y+b.h
}

// planGlyphs decides which growing digits are drawn large and where.
//
// A glyph is several rows tall and cluster members are adjacent, so the
// largest is placed first and each smaller one is skipped if it would swallow,
// or be swallowed by, one already placed. Skipped members stay ordinary
// one-cell digits.
func (m *Model) planGlyphs() {
	l := m.Layout
	m.boxes = m.boxes[:0]
	clear(m.enlarged)

	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			h := m.Magnify.Height(gx, gy)
			id := GridIndex(gx, gy)

			// glyphOn holds the decision through a dead band (hysteresis.go).
			on := glyphOn(h, m.glyphing[id])
			if !on {
				delete(m.glyphing, id)
				continue
			}
			m.glyphing[id] = true

			n := m.Grid.At(gx, gy)
			if n == nil || m.Flights.InFlight(n.ID) {
				delete(m.glyphing, id)
				continue
			}

			// See glyphBox.solid.
			solid := m.IsActive(n.BadGroupID)
			cov := glyph.RasterizeFor(n.Value, h*glyph.CellAspect, h, m.glyphTilt(n), solid)
			if cov == nil {
				continue
			}

			// Anchored like any digit, so it bulges and shakes with its cluster.
			cx, cy := m.ScreenAnchor(n.ID)
			m.markAnchor(n.ID, cx, cy)

			m.boxes = append(m.boxes, glyphBox{
				id:     n.ID,
				x:      cx - cov.W/2,
				y:      cy - cov.H/2,
				w:      cov.W,
				h:      cov.H,
				height: h,
				cov:    cov,
				solid:  solid,
			})
		}
	}

	// Largest first, so the biggest digit claims its space. A cluster's
	// members share one swell and so tie often; ties break toward this
	// refiner's own focus (not the nearest of everybody's), so the digit under
	// your own cursor wins.
	distToFocus := func(b glyphBox) float64 {
		cx, cy := float64(b.x+b.w/2), float64(b.y+b.h/2)
		f := m.Lens.Focus()
		dx, dy := cx-f.X, (cy-f.Y)*RowAspect
		return dx*dx + dy*dy
	}
	sort.Slice(m.boxes, func(i, j int) bool {
		a, b := m.boxes[i], m.boxes[j]
		if a.height != b.height {
			return a.height > b.height
		}
		if da, db := distToFocus(a), distToFocus(b); da != db {
			return da < db
		}
		return a.id < b.id
	})

	// Keep only the boxes whose centres survive, in place.
	kept := 0
	for _, b := range m.boxes {
		clash := false
		for _, p := range m.boxes[:kept] {
			if b.swallowedBy(p) || p.swallowedBy(b) {
				clash = true
				break
			}
		}
		if clash {
			continue
		}
		m.boxes[kept] = b
		kept++
		m.enlarged[b.id] = true
	}
	m.boxes = m.boxes[:kept]
}

// paintGlyphs draws the placement computed by planGlyphs. Every box is blanked
// before any is painted, or a box could clear an overlapping neighbour already
// drawn. Painting runs smallest first so the larger digit ends on top.
func (m *Model) paintGlyphs(buf *canvas.Buffer) {
	for _, b := range m.boxes {
		m.clearGlyphBox(buf, b)
	}
	for i := len(m.boxes) - 1; i >= 0; i-- {
		m.drawGlyph(buf, m.boxes[i])
	}
}

// clearGlyphBox blanks the grid behind a glyph so its shape reads, within the
// board's rows only.
func (m *Model) clearGlyphBox(buf *canvas.Buffer, b glyphBox) {
	l := m.Layout
	gridTop, gridBottom := l.BoardY, l.BoardY+l.BoardH-1
	for row := 0; row < b.h; row++ {
		y := b.y + row
		if y < gridTop || y > gridBottom {
			continue
		}
		buf.HLine(b.x, y, b.w, ' ', m.Theme.Page)
	}
}

// drawGlyph paints one enlarged digit from its coverage grid. Each cell's
// character comes from the density ramp, so the antialiased edge fades out.
func (m *Model) drawGlyph(buf *canvas.Buffer, b glyphBox) {
	l := m.Layout

	gridTop, gridBottom := l.BoardY, l.BoardY+l.BoardH-1
	digit := rune('0' + m.Grid.Cells[b.id].Value)
	paintCoverage(buf, b.x, b.y, b.cov, digit, b.solid, m.glyphAttr(b.id), gridTop, gridBottom)
}

// glyphAttr is the colour an enlarged digit is drawn in. The diversion
// transition also calls it, so a digit leaves in the colour on screen.
//
// The digit under the cursor is Hovered. Otherwise an awake cluster (not merely
// a corrupt one) reddens with how long it has been ignored.
func (m *Model) glyphAttr(id int) canvas.Attr {
	n := &m.Grid.Cells[id]
	switch {
	case id == GridIndex(m.CursorX, m.CursorY):
		return m.Theme.Hovered
	case m.deskAt(id%GridSize, id/GridSize) != nil:
		// Another refiner's cursor is on this digit.
		return m.Theme.On(m.deskColour(m.deskAt(id%GridSize, id/GridSize)), true)
	case m.IsActive(n.BadGroupID):
		// Each awake cluster reddens on its own clock.
		red := m.Theme.ScaryColour(m.Awake(n.BadGroupID))
		// A reached cluster keeps its red and gets a background plate: Plate
		// for your own reach, the reacher's desk colour for anyone else's.
		var plate canvas.Color
		if seat, reached := m.reachSeat[n.BadGroupID]; reached {
			if seat == mineSeat {
				plate = m.Theme.Plate
			} else {
				plate = m.Theme.DeskColour(seat)
			}
		}

		// Once the fuse is lit, the red flashes toward Peak.
		if left, lit := m.Fusing(n.BadGroupID); lit {
			red = theme.BlendColors(red, m.Theme.Peak, FuseFlash(left))
		}
		attr := m.Theme.On(red, true)
		attr.BG = plate
		return m.arrivalDim(attr, id)
	}
	return m.arrivalDim(m.Theme.Swollen, id)
}

// arrivalDim dims an enlarged glyph with the rest of the board while it is
// arriving, so a woken cluster does not show ahead of the wavefront.
func (m *Model) arrivalDim(attr canvas.Attr, id int) canvas.Attr {
	if m.arrival == nil {
		return attr
	}
	l := m.Layout
	return m.arrival.dim(attr,
		id%GridSize-l.ViewportX, id/GridSize-l.ViewportY, m.Theme.Backdrop)
}

// paintCoverage writes one rasterized digit into the buffer, a character per
// inked cell, clipped to the rows between top and bottom. It draws both grid
// glyphs and airborne digits; only the grid case blanks its box first, so a
// flight does not erase the ones beside it.
//
// solid picks blocks, or the digit repeated through the character ramp (see
// glyphBox.solid). The character rendering is rare (a cluster shrinking back
// after it is retired or detonated, some Transits) but still used.
func paintCoverage(buf *canvas.Buffer, x, y int, cov *glyph.Coverage, digit rune, solid bool, attr canvas.Attr, top, bottom int) {
	for row := 0; row < cov.H; row++ {
		ry := y + row
		if ry < top || ry > bottom {
			continue
		}
		for col := 0; col < cov.W; col++ {
			c := cov.At(col, row)
			if c <= 0 {
				continue
			}
			r := glyph.EmergeRune(digit, c)
			if solid {
				r = glyph.RampRune(c)
			}
			buf.Set(x+col, ry, r, attr)
		}
	}
}

// drawFlights draws digits in transit to their bin. A refined digit leaves at
// its size and shrinks on the way, drawn as a glyph until it falls below
// MinGlyphHeight.
func (m *Model) drawFlights(buf *canvas.Buffer) {
	flights := m.Flights.Active()

	// Smallest first, so the most recently launched ends up on top.
	sort.SliceStable(flights, func(i, j int) bool { return flights[i].Height < flights[j].Height })

	// A flight may cross the lower rule into a bin, but is clipped at the
	// grid's top so a tall digit from the top row does not cover the banner.
	_, bottom := buf.Size()
	top := m.Layout.GridTop
	for _, f := range flights {
		if f.Height < MinGlyphHeight {
			if y := int(f.Y + 0.5); y >= top && y < bottom {
				buf.Set(int(f.X+0.5), y, f.Digit, m.Theme.BadLive)
			}
			continue
		}
		cov := glyph.RasterizeFor(int(f.Digit-'0'), f.Height*glyph.CellAspect, f.Height, f.Tilt, true)
		if cov == nil {
			continue
		}
		// A flight is from a refined cluster, so it stays solid.
		paintCoverage(buf, int(f.X+0.5)-cov.W/2, int(f.Y+0.5)-cov.H/2, cov,
			f.Digit, true, m.Theme.BadLive, top, bottom-1)
	}
}

// headerBands is which columns the file bar gave to each of the things on it,
// as of the last frame drawn. Every x1 is exclusive, and a band nothing was
// drawn in is empty (x0 == x1).
type headerBands struct {
	name, ticks, pct, mark band
}

// band is a run of columns, x1 exclusive.
type band [2]int

func (b band) empty() bool    { return b[1] <= b[0] }
func (b band) has(x int) bool { return x >= b[0] && x < b[1] }
func (b band) overlaps(o band) bool {
	return !b.empty() && !o.empty() && b[0] < o[1] && o[0] < b[1]
}

// drawHeader draws the file bar as the show lays it out, in separate lanes:
// the file's name on the left, then progress ticks, then the completion and the
// Lumon mark on the right. The lanes never overlap, so the percentage is always
// readable. The ticks follow the eased File.Fill, with a partial leading tick.
func (m *Model) drawHeader(buf *canvas.Buffer) {
	l := m.Layout
	buf.Box(0, 0, l.Width, l.HeaderH, m.Theme.Chrome)
	m.header = headerBands{}
	m.drawAgentStrip(buf)

	if l.Width < 12 || l.HeaderH < 3 {
		return
	}

	// The words sit on the bar's middle row, centred against the mark.
	mid := l.HeaderH / 2

	name := []rune(m.File.Name)

	// One row of plain text in mixed case, as the show writes it ("Complete",
	// not "COMPLETE"). Large type spends too much of the bar on a caption.
	pct := strconv.Itoa(shownPercent(m.File.Name, m.File.Percent(m.Grid))) + "% Complete"

	// On a narrow terminal the mark gives way first.
	right := l.Width - 2
	need := 2 + len(name) + 2 + len(pct) + fileTickPitch*fileTickMinRun
	if w := m.fileMarkWidth(); w > 0 && need+w+1 <= right+1 {
		m.drawFileMark(buf, right-w+1, w)
		m.header.mark = band{right - w + 1, right + 1}
		right -= w + 1
	}

	// Then the name is truncated, down to fileNameMin, before the completion
	// is dropped.
	pctX := right - len(pct) + 1
	if pctX-4 >= fileNameMin {
		buf.Text(pctX, mid, pct, m.Theme.Header)
		m.header.pct = band{pctX, pctX + len(pct)}
	} else {
		// Not wide enough for both: drop the completion, keep the name.
		pctX = right + 2
	}

	if n := pctX - 4; n < len(name) {
		if n < 1 {
			name = nil
		} else {
			name = name[:n]
		}
	}
	if len(name) > 0 {
		buf.Text(2, mid, string(name), m.Theme.Header)
		m.header.name = band{2, 2 + len(name)}
	}

	m.header.ticks = m.drawFileTicks(buf, 2+len(name)+2, pctX-2, fileTickMinRun)
}

// fileTick is a lit division of the bar and fileTickGap an unlit one; ticks are
// laid every fileTickPitch columns so they read as separate marks, not a solid
// rule.
const (
	fileTick       = '█'
	fileTickGap    = '│'
	fileTickPitch  = 2
	fileTickInset  = 1 // rows left clear above and below, so ticks do not touch the border
	fileTickMinRun = 3 // fewer divisions than this are not drawn

	// fileNameMin is how much of the file's name the bar keeps. Below it the
	// completion is dropped instead.
	fileNameMin = 4
)

// drawFileTicks fills the lane between the name and the completion, and returns
// the columns it used. If the lane holds fewer than min divisions it is left
// empty.
func (m *Model) drawFileTicks(buf *canvas.Buffer, x0, x1, min int) band {
	l := m.Layout
	top, bottom := 1, l.HeaderH-2
	if l.HeaderH > HeaderHeight {
		top, bottom = top+fileTickInset, bottom-fileTickInset
	}
	if x1 <= x0 || bottom < top {
		return band{}
	}

	n := (x1 - x0 + 1) / fileTickPitch
	if n < min {
		return band{}
	}

	// The leading tick is a partial block, so the bar moves smoothly instead of
	// jumping a whole tick at a time.
	exact := m.File.Fill() * float64(n)
	whole := int(exact)
	edge := partialBlock(exact - float64(whole))

	for i := 0; i < n; i++ {
		r, attr := fileTickGap, m.Theme.Number
		switch {
		case i < whole:
			r, attr = fileTick, m.Theme.Header
		case i == whole && edge != 0:
			r, attr = edge, m.Theme.Header
		}
		for y := top; y <= bottom; y++ {
			buf.Set(x0+i*fileTickPitch, y, r, attr)
		}
	}
	return band{x0, x0 + (n-1)*fileTickPitch + 1}
}

// fileMark is the word the bar closes with where there is no room for the globe.
const fileMark = "LUMON"

// fileMarkWidth is how many columns the mark wants, globe or word.
func (m *Model) fileMarkWidth() int {
	if m.Layout.HeaderH < HeaderTall {
		return len(fileMark)
	}
	w := 0
	for _, row := range SmallLogo {
		if n := len([]rune(row)); n > w {
			w = n
		}
	}
	return w
}

// drawFileMark closes the right end of the bar with the intro's globe mark, or
// the word where the bar is too short for it.
func (m *Model) drawFileMark(buf *canvas.Buffer, x, w int) {
	if m.Layout.HeaderH < HeaderTall {
		buf.Text(x, m.Layout.HeaderH/2, fileMark, m.Theme.Header)
		return
	}

	// Centred vertically in the bar.
	top := (m.Layout.HeaderH - MarkH) / 2
	if top < 1 {
		top = 1
	}
	m.drawMark(buf, x, top, m.Theme.Chrome)
}

// drawOverFill writes a label across the bar, switching attributes at the fill
// edge so the part over the fill reads against it and the rest reads against the
// background.
func (m *Model) drawOverFill(buf *canvas.Buffer, x int, s string, fillEnd int) {
	for i, r := range []rune(s) {
		attr := m.Theme.Header
		if x+i < 1+fillEnd {
			attr = m.Theme.FileFillText
		}
		buf.Set(x+i, 1, r, attr)
	}
}

// fillBlocks are the eighth-width blocks, lightest first, for the fill's leading
// edge.
var fillBlocks = []rune{'▏', '▎', '▍', '▌', '▋', '▊', '▉'}

// partialBlock picks the block that best represents a fraction of a cell, or 0
// if there is too little to show.
func partialBlock(frac float64) rune {
	i := int(frac * float64(len(fillBlocks)+1))
	if i <= 0 {
		return 0
	}
	if i > len(fillBlocks) {
		i = len(fillBlocks)
	}
	return fillBlocks[i-1]
}

// drawTopRule is the rule over the grid, with the status row between its two
// lines left clear for the news (see statusRow).
func (m *Model) drawTopRule(buf *canvas.Buffer) {
	top := m.Layout.GridTop - TopRuleHeight
	buf.HLine(0, top, m.Layout.Width, '═', m.Theme.Chrome)
	buf.HLine(0, top+1, m.Layout.Width, ' ', m.Theme.Page)
	buf.HLine(0, top+2, m.Layout.Width, '═', m.Theme.Chrome)
}

func (m *Model) drawRule(buf *canvas.Buffer, y int) {
	buf.HLine(0, y, m.Layout.Width, '═', m.Theme.Chrome)
	buf.HLine(0, y+1, m.Layout.Width, '═', m.Theme.Chrome)
}

func (m *Model) drawGrid(buf *canvas.Buffer) {
	l := m.Layout
	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			n := m.Grid.At(gx, gy)
			if n == nil {
				continue
			}
			// drawFlights draws digits in transit.
			if m.Flights.InFlight(n.ID) {
				continue
			}

			// paintGlyphs draws enlarged digits; members it crowded out are
			// drawn here.
			if m.enlarged[n.ID] {
				continue
			}

			// Whole cells, through the Schmitt triggers; vertical travel is
			// downward only, at most maxRowSag (see rowStep).
			x, y := m.PlainAnchor(n.ID)
			m.markAnchor(n.ID, x, y)

			// A cell refilling after a refinement spins through digits and fades
			// up.
			ch := rune('0' + n.Value)
			if v, spinning := m.Regen.Display(n.ID); spinning {
				ch = glyph.EmergeRune(rune('0'+v), m.Regen.Density(n.ID))
			}

			// A board still arriving comes up from the middle outward with the
			// same ramp. Cells the wavefront has not reached are not drawn.
			if m.arrival != nil {
				reach := m.arrival.Reach(gx-l.ViewportX, gy-l.ViewportY)
				if reach <= 0 {
					continue
				}
				if reach < 1 {
					ch = glyph.EmergeRune(ch, reach)
				}
			}

			buf.Set(x, y, ch, m.cellAttr(n, gx, gy))
		}
	}
}

// cellAttr is the colour a digit is showing, which is somewhere between the
// one it had and the one it is going to. See tint.go.
func (m *Model) cellAttr(n *Number, gx, gy int) canvas.Attr {
	want := m.cellTarget(n, gx, gy)
	// Checked here to skip the call on a still board: this runs for every
	// visible digit every frame.
	if m.tint.live > 0 {
		want = m.tint.Attr(gx, gy, want)
	}
	if m.arrival != nil {
		want = m.arrival.dim(want,
			gx-m.Layout.ViewportX, gy-m.Layout.ViewportY, m.Theme.Backdrop)
	}
	return want
}

// cellTarget picks a digit's styling. Corrupt digits stand out only once their
// cluster is active.
//
// It is pure: the tint crossing is advanced in the step and only read in the
// draw, because View runs after every message and easing there would speed up
// with input.
func (m *Model) cellTarget(n *Number, gx, gy int) canvas.Attr {
	if gx == m.CursorX && gy == m.CursorY {
		// In company your cursor is your seat colour; alone it is the faint
		// Cursor plate (see theme.cursorPlate).
		if m.Seated() > 1 && m.Desk != nil {
			return m.Theme.DeskCursor(m.Desk.Seat)
		}
		return m.Theme.Cursor
	}
	// Ringfenced digits are drawn like selected text. Below the cursor so it
	// stays findable; above desks and corruption, whose red still shows through
	// the glyph path.
	if n.BadGroupID >= 0 && m.held[n.BadGroupID] && m.IsActive(n.BadGroupID) {
		return m.Theme.Selected
	}
	// Another refiner's cursor, in their colour.
	if d := m.deskAt(gx, gy); d != nil {
		return m.Theme.DeskCursor(d.Seat)
	}
	if n.BadGroupID >= 0 {
		if m.IsActive(n.BadGroupID) {
			return m.Theme.BadLive
		}
		return m.Theme.Bad
	}
	return m.Theme.Digit
}

func (m *Model) drawBins(buf *canvas.Buffer) {
	l := m.Layout
	for i, bin := range m.Grid.Bins {
		x, w := l.BinOriginX(i), l.BinWidth(i)
		if w <= 2 {
			continue
		}

		// In company a bin takes the colour of whoever is feeding it, bold if
		// it is yours. Alone the selected bin is BinOn.
		feeding := m.feeding(i)
		attr := m.Theme.BinOff
		switch {
		case len(feeding) > 0:
			attr = m.Theme.On(m.deskColour(feeding[0]), feeding[0] == m.Desk)
		case i == m.SelectedBin:
			attr = m.Theme.BinOn
		}

		// A bin is lit while digits are in the air toward it.
		if m.BinFill.Receiving(i) {
			attr.Bold = true
			if attr.FG == m.Theme.BinOff.FG {
				attr.FG = m.Theme.BinOn.FG
			}
		}

		buf.Box(x, l.BinTop, w, 3, attr)

		// While the lid is up, clear the box's top edge between the walls, or
		// the bin shows a shut lid and a swinging one.
		open := m.BinFill.Ajar(i)
		if open {
			for cx := x + 1; cx < x+w-1; cx++ {
				buf.Set(cx, l.BinTop, ' ', attr)
			}
		}
		// Centred on Layout.BinCenterX, because bins differ in width.
		label := clipBin(binLabel(bin.ID), w)
		buf.Text(binTextX(l.BinCenterX[i], len(label), x, w), l.BinTop+1, label, attr)

		buf.Box(x, l.BinTop+3, w, 3, attr)
		m.drawBinLevel(buf, x, w, i, bin, attr)

		// Several refiners on one bin: split its edges between them.
		if len(feeding) > 1 {
			m.stripeBin(buf, x, w, feeding)
		}

		// The drawer before the lid, so the flaps pass over its top corners.
		m.drawDrawer(buf, x, w, i)

		// The lid next, so it passes over the box and the rule above it.
		m.drawBinLid(buf, x, w, i)

		// Then the walls, which win at the top row: the lid's lowest dot lands
		// on the left wall.
		if open {
			buf.Set(x, l.BinTop, '│', attr)
			buf.Set(x+w-1, l.BinTop, '│', attr)
		}
	}
}

// drawBinLevel fills the lower box in proportion to what the bin holds, with a
// partial-block leading edge, and writes the readout across it.
func (m *Model) drawBinLevel(buf *canvas.Buffer, x, w, i int, bin *Bin, attr canvas.Attr) {
	l := m.Layout
	y := l.BinTop + 4

	inner := w - 2
	if inner < 1 {
		return
	}

	exact := m.BinFill.Fill(i) * float64(inner)
	whole := int(exact)
	if whole > inner {
		whole = inner
	}
	// BinFull, not FileFill: FileFill is the dark Backdrop and looks like an
	// empty bin.
	if whole > 0 {
		buf.HLine(x+1, y, whole, ' ', m.Theme.BinFull)
	}
	if whole < inner {
		// The partial leading edge is drawn in the fill's colour.
		if edge := partialBlock(exact - float64(whole)); edge != 0 {
			buf.Set(x+1+whole, y, edge, m.Theme.On(m.Theme.Primary, false))
		}
	}

	// The percentage, as the show's bins have. Appended into scratch to avoid
	// allocating every frame. All ASCII, so bytes index as characters.
	b := m.binLabel[:0]
	b = strconv.AppendInt(b, int64(bin.PercentInt()), 10)
	b = append(b, '%')
	if room := w - 3; room >= 1 && len(b) > room {
		b = b[:room]
	}
	m.binLabel = b

	// Over the fill the text is inverted, dark on the bright fill.
	over := canvas.Attr{FG: m.Theme.BinFullText.FG, BG: m.Theme.BinFullText.BG, Bold: true}

	// Left-aligned against the wall, as in the C++ reference, so the fill
	// grows out from under the text instead of its edge crossing it.
	x0 := x + 1
	for k := 0; k < len(b); k++ {
		cx := x0 + k
		if cx >= x+w-1 {
			break
		}
		if cx-(x+1) < whole {
			buf.Set(cx, y, rune(b[k]), over)
			continue
		}
		buf.Set(cx, y, rune(b[k]), attr)
	}
}

// binLabel is how a bin is numbered on screen: zero-padded to two digits, as
// the show has.
func binLabel(id int) string {
	// From a table, to avoid allocating every frame.
	if id >= 0 && id < len(binLabels) {
		return binLabels[id]
	}
	return strconv.Itoa(id)
}

var binLabels = [...]string{"00", "01", "02", "03", "04", "05", "06", "07", "08", "09"}

// binTextX centres n characters on a bin's centre (Layout's, the one flights
// aim at) without letting them cross its walls.
func binTextX(centre, n, x, w int) int {
	at := centre - n/2
	if lo := x + 1; at < lo {
		at = lo
	}
	if hi := x + w - 1 - n; at > hi {
		at = hi
	}
	if at < x+1 {
		at = x + 1
	}
	return at
}

// clipBin trims a bin's label to w-3 runes, the room inside a bin of width w.
func clipBin(s string, w int) string {
	room := w - 3
	if room < 1 {
		return ""
	}
	if r := []rune(s); len(r) > room {
		return string(r[:room])
	}
	return s
}

// feeding is who is sending clusters to a bin, in seat order with this refiner
// first. Nil unless the room has company.
func (m *Model) feeding(bin int) []*Desk {
	if m.Room == nil || m.Seated() < 2 {
		return nil
	}
	var out []*Desk
	for _, d := range m.Roster() {
		if d.Bin != bin {
			continue
		}
		if d == m.Desk {
			out = append([]*Desk{d}, out...)
			continue
		}
		out = append(out, d)
	}
	return out
}

// stripeBin divides a bin's horizontal edges between the refiners feeding it.
// Buffer.Box paints one colour, so the split is drawn over it afterwards.
func (m *Model) stripeBin(buf *canvas.Buffer, x, w int, feeding []*Desk) {
	l := m.Layout
	inner := w - 2
	if inner < len(feeding) {
		return // too narrow to say anything legible
	}

	// Equal runs, the remainder to the leftmost.
	base, extra := inner/len(feeding), inner%len(feeding)
	at := x + 1
	for i, d := range feeding {
		run := base
		if i < extra {
			run++
		}
		attr := m.Theme.On(m.deskColour(d), false)
		if d == m.Desk {
			attr.Bold = true
		}
		// A bin is two stacked boxes, so it has four horizontal edges.
		for _, y := range [...]int{l.BinTop, l.BinTop + 2, l.BinTop + 3, l.BinTop + 5} {
			buf.HLine(at, y, run, '─', attr)
		}
		at += run
	}
}

func (m *Model) drawFooter(buf *canvas.Buffer) {
	l := m.Layout

	// Three rows: a blank, the address or roster (y), and the keys.
	y := l.FooterY()

	// The link light claims the corner first and returns where the rest must
	// stop. It is absent on a local run.
	edge := m.drawLinkLight(buf, y)

	// In company, the roster; alone, the address and banner.
	if m.Seated() > 1 {
		m.drawRoster(buf, y, edge)

		// A watcher sees their place in the queue instead of the keys.
		if line := m.watchLine(); line != "" {
			m.centreFooter(buf, y+1, clipTo(line, l.Width-2), m.Theme.BinOn)
			return
		}
		m.centreFooter(buf, y+1, m.hintLine(l.Width-2), m.Theme.Hint)
		return
	}

	// The address readout is placed first, since the show has it; the banner
	// is ours and gives way.
	_, used := m.drawAddress(buf, y, edge)

	const banner = "LUMON INDUSTRIES, MDR"
	// Right-aligned against edge, only if it clears the readout.
	if x := edge - len([]rune(banner)); x > 44 && x > used {
		buf.Text(x, y, banner, m.Theme.Footer)
	}

	// The keys get their own row and the full width.
	m.centreFooter(buf, y+1, m.hintLine(l.Width-2), m.Theme.Hint)
}

// centreFooter writes one line centred across the terminal, on the same axis
// as the address readout above it.
func (m *Model) centreFooter(buf *canvas.Buffer, y int, s string, a canvas.Attr) {
	if s == "" {
		return
	}
	x := (m.Layout.Width - len([]rune(s))) / 2
	if x < 1 {
		x = 1
	}
	buf.Text(x, y, s, a)
}

// hintLine is the footer's keys line: the game's controls, or the camera's in
// the projected view. The camera keys appear only here, never in the controls
// overlay, because the 3D view is a hidden mode.
func (m *Model) hintLine(room int) string {
	if m.solid == nil {
		switch {
		case m.touchScreen:
			return pick(touchFooterLadder, room)
		case m.keepOpen:
			return pick(browserFooterLadder, room)
		case m.allowBots:
			return pick(companyFooterLadder, room)
		}
		return footerHint(room)
	}
	// Name q as the way out. Retyping "3d" also works, but its first
	// keystroke picks a bin on the way.
	line := RigKeys + " · q back"
	if len([]rune(line)) > room {
		return RigKeys
	}
	return line
}

// drawAddress centres the address readout in the footer, as the show does, if
// it fits left of room. It returns the room left to its left, and end, the
// column one past its right-hand end (0 if not drawn), which the banner must
// clear (TestTheBannerNeverPrintsOverTheAddress).
func (m *Model) drawAddress(buf *canvas.Buffer, y, room int) (left, end int) {
	line := m.addressLine()
	n := len([]rune(line))

	// Without room for itself and a margin, the readout gives way.
	x := (m.Layout.Width - n) / 2
	if x < 14 || x+n+1 > room {
		return room, 0
	}
	// Chrome, not Footer: Footer is muted and looks unlit here.
	buf.Text(x, y, line, m.Theme.Chrome)
	return x - 2, x + n
}

// drawRoster writes the nameplates along the footer, ending at edge: who is at
// the board, the bin they are feeding, and what they have put in it. It returns
// the leftmost column it used.
//
// On a narrow terminal the plates shorten before they disappear: the tally goes
// first, then the name to an initial.
func (m *Model) drawRoster(buf *canvas.Buffer, y, edge int) int {
	desks := m.Roster()

	forms := []func(*Desk) string{
		func(d *Desk) string { return fmt.Sprintf("%s %d·%d", d.Name, d.Bin+1, d.Credit) },
		func(d *Desk) string { return fmt.Sprintf("%s %d", d.Name, d.Bin+1) },
		func(d *Desk) string { return fmt.Sprintf("%s%d", initials(d.Name), d.Bin+1) },
	}

	for _, form := range forms {
		plates := make([]string, 0, len(desks))
		width := 0
		for _, d := range desks {
			p := form(d)
			plates = append(plates, p)
			width += len([]rune(p)) + 2
		}

		x := edge - width
		if x < 2 {
			continue // even this form is too wide
		}
		start := x
		for i, d := range desks {
			attr := m.Theme.On(m.deskColour(d), false)
			if d == m.Desk {
				attr.Bold = true // your own plate, so you can find yourself
			}
			if m.watchedDesk(d) {
				// Inverted, a cell wider each side: the refiner the camera shows.
				attr = canvas.Attr{FG: m.Theme.Ink, BG: m.deskColour(d), Bold: true}
				buf.Text(x-1, y, " "+plates[i]+" ", attr)
				if i == 0 {
					start--
				}
			} else {
				buf.Text(x, y, plates[i], attr)
			}
			x += len([]rune(plates[i])) + 2
		}
		return start
	}
	return edge
}

// initials shortens a name for a crowded footer: "MARK S." becomes "M.".
func initials(name string) string {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return "?"
	}
	return string([]rune(fields[0])[:1]) + "."
}

// deskAt is whoever else has their cursor on a cell, or nil. Your own desk is
// skipped; callers draw your cursor separately.
func (m *Model) deskAt(gx, gy int) *Desk {
	if m.Seated() < 2 {
		return nil
	}
	for _, d := range m.Roster() {
		if d != m.Desk && d.CursorX == gx && d.CursorY == gy {
			return d
		}
	}
	return nil
}

// glyphTilt is how far a digit drawn large is leaning, in degrees. Only an
// awake cluster leans, growing with escalation and driven by the digit's
// agitation offset so it rocks in step with the shake.
//
// The angle is snapped to GlyphTiltStep because each distinct angle is a
// separate rasterizer cache entry; continuous angles would grow the cache
// without bound.
func (m *Model) glyphTilt(n *Number) float64 {
	if !m.IsActive(n.BadGroupID) {
		return 0
	}
	dx, _ := m.Agitation.Offset(n.ID)
	lean := clampF(dx/MaxAgitationOffset, -1, 1) * MaxGlyphTilt * m.escalation(n.BadGroupID)
	return math.Round(lean/GlyphTiltStep) * GlyphTiltStep
}

// clampF bounds v to [lo, hi].
func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// offBlink is how long each half of the dark set's cursor blink lasts, in
// seconds.
const offBlink = 0.5

// drawOff renders the dark set: one centred line asking to switch it on, and a
// blinking block cursor a space after it.
func (m *Model) drawOff(buf *canvas.Buffer) {
	w, h := buf.Size()
	prompt := "PRESS ANY KEY TO POWER ON"
	if m.touchScreen {
		prompt = "TAP TO POWER ON"
	}
	n := len(prompt) + 2
	if w < n || h < 1 {
		return
	}
	x, y := (w-n)/2, h/2
	buf.Text(x, y, prompt, m.Theme.Boot)
	if int(m.sessionAge/offBlink)%2 == 0 {
		buf.Set(x+len(prompt)+1, y, ' ', m.Theme.BootCursor)
	}
}

// drawIntro renders the boot sequence: the POST log while it scrolls, then the
// Lumon logo drawing itself.
func (m *Model) drawIntro(buf *canvas.Buffer) {
	switch m.Intro.Stage() {
	case stageLogo:
		m.drawIntroLogo(buf)
	default:
		m.drawBootLog(buf)
	}
}

// bootPadX and bootPadY keep the boot's log and cursor off the screen's edge,
// in columns and rows.
const bootPadX, bootPadY = 2, 1

// drawBootLog writes the log bottom-anchored, showing only the lines that fit,
// so the newest line is always on screen.
func (m *Model) drawBootLog(buf *canvas.Buffer) {
	w, h := buf.Size()
	lines := m.Intro.Lines()
	x0, y0 := bootPadX, bootPadY
	if w < 2*bootPadX+1 || h < 2*bootPadY+1 {
		x0, y0 = 0, 0
	}
	w, h = w-2*x0, h-2*y0
	if h < 1 || w < 1 {
		return
	}
	if waking, lit := m.Intro.Waking(); waking {
		if lit {
			buf.Set(x0, y0, ' ', m.Theme.BootCursor)
		}
		return
	}

	scrolled := 0
	if len(lines) > h {
		scrolled = len(lines) - h
		lines = lines[scrolled:]
	}

	// With graphics, the mark's MarkH log rows are left blank and the picture
	// is placed over them, but only while all of them are on screen and
	// revealed: an image placement cannot be clipped. Otherwise the rows print
	// as braille text.
	markTop := -1
	if m.graphics != GraphicsOff && m.Intro.MarkAt() >= 0 {
		top := m.Intro.MarkAt() - scrolled
		if top >= 0 && top+MarkH <= h && top+MarkH <= len(lines) && MarkW <= w {
			markTop = top
		}
	}

	for i, line := range lines {
		if markTop >= 0 && i >= markTop && i < markTop+MarkH {
			continue
		}
		if len([]rune(line)) > w {
			line = string([]rune(line)[:w])
		}
		buf.Text(x0, y0+i, line, m.Theme.Boot)
	}
	if markTop >= 0 {
		m.drawMark(buf, x0, y0+markTop, m.Theme.Boot)
	}

	// The cursor shows only while the authorize line is typing.
	if m.Intro.Typing() && m.Intro.CursorOn() && len(lines) > 0 {
		y := len(lines) - 1
		if x := len([]rune(lines[y])); x < w {
			buf.Set(x0+x, y0+y, ' ', m.Theme.BootCursor)
		}
	}
}

// drawIntroLogo centres the current animation frame, falling back to the small
// static mark on a terminal too small to hold it.
func (m *Model) drawIntroLogo(buf *canvas.Buffer) {
	w, h := buf.Size()
	lw, lh, ok := LogoSize()

	if !ok || w < lw || h < lh {
		m.drawSmallLogo(buf)
		return
	}

	frame := LogoFrame(m.Intro.Frame())
	if t, morph := m.Intro.Between(); morph {
		frame = LogoMorph(m.Intro.Frame(), t)
	}
	if frame == nil {
		m.drawSmallLogo(buf)
		return
	}

	ink := m.introInk()
	ox, oy := (w-lw)/2, (h-lh)/2
	for row := 0; row < lh; row++ {
		for col := 0; col < lw; col++ {
			r := frame[row*lw+col]
			if r == '⠀' {
				continue // blank braille: leave the background alone
			}
			buf.Set(ox+col, oy+row, r, ink)
		}
	}
}

// introInk is the mark's colour while it fades in. It blends from the page
// colour, not black, so it also works on a light theme. A theme with no page
// colour uses the terminal's default, which cannot be blended, so black stands
// in.
func (m *Model) introInk() canvas.Attr {
	a := m.Theme.Boot
	if m.Intro == nil {
		return a
	}
	t := m.Intro.LogoFade()
	if t >= 1 {
		return a
	}
	from := m.Theme.Page.BG
	if !from.Set() {
		from = canvas.RGB(0, 0, 0)
	}
	a.FG = canvas.Blend(from, a.FG, t)
	return a
}

// drawSmallLogo centres the compact mark, with the wordmark under it.
func (m *Model) drawSmallLogo(buf *canvas.Buffer) {
	w, h := buf.Size()
	const word = "LUMON INDUSTRIES"

	lw := MarkW
	lh := MarkH + 2
	if w < lw || h < lh {
		return
	}

	ox, oy := (w-lw)/2, (h-lh)/2
	m.drawMark(buf, ox, oy, m.introInk())
	buf.Text((w-len(word))/2, oy+MarkH+1, word, m.Theme.Header)
}

// lidRise is how many rows a fully open flap stands up: the rotation seen from
// slightly above, foreshortened vertically. It reaches over the rule into the
// grid; only the cells it marks are painted, so digits show around it.
const lidRise = 6.0

// lidThickness is how many braille dots across a flap is drawn. Two fills a
// cell's width, so a standing flap is a solid bar, matching the two parallel
// lines of the reference sprite.
const lidThickness = 2

// drawBinLid draws the two flaps on top of a bin, part way through their swing.
// They are hinged at the bin's top corners and open up and outward into a V,
// overhanging the sides (from the C++ reference's bin-open.png). Drawn on the
// braille layer so the swing is smooth.
func (m *Model) drawBinLid(buf *canvas.Buffer, x, w, i int) {
	// The same threshold drawBins uses for the box's top edge, so exactly one
	// of the two shows.
	if !m.BinFill.Ajar(i) {
		return
	}
	if w < 2 {
		return
	}

	// The spring overshoots in time but not in geometry.
	deg := math.Min(m.BinFill.Lid(i), LidOpen)
	rad := deg * math.Pi / 180

	span := float64(w) * LidSpan

	// The free end, relative to its hinge: inward when shut, over the hinge at
	// 90 degrees, outward beyond that.
	reachIn := span * math.Cos(rad)
	rise := lidRise * math.Sin(rad)

	// The layer spans the bin plus whatever the flaps overhang on either side.
	over := 0
	if reachIn < 0 {
		over = int(math.Ceil(-reachIn)) + 1
	}
	left := x - over
	if left < 0 {
		left = 0
	}
	right := x + w + over
	if right > m.Layout.Width {
		right = m.Layout.Width
	}
	cols := right - left
	if cols < 2 {
		return
	}

	top := m.Layout.BinTop - int(lidRise) - 1
	if top < 0 {
		top = 0
	}
	rows := m.Layout.BinTop - top + 1
	if rows < 1 {
		return
	}

	br := canvas.NewBraille(cols, rows)
	hingeY := float64(m.Layout.BinTop-top) * canvas.DotsY
	tipY := hingeY - rise*canvas.DotsY

	// The left flap reaches right when shut; the right one mirrors it.
	for _, f := range [2]struct{ hinge, dir float64 }{
		{hinge: float64(x - left), dir: 1},
		{hinge: float64(x + w - 1 - left), dir: -1},
	} {
		hx := f.hinge * canvas.DotsX
		tx := hx + f.dir*reachIn*canvas.DotsX
		plotLine(br, hx, hingeY, tx, tipY, lidThickness)
	}

	// Blit only marked cells, so the flaps overhang the rule and neighbours
	// without erasing them.
	buf.BlitBrailleOver(left, top, br, m.Theme.BinOn)
}

// plotLine draws a straight segment onto a braille layer, in dots. Thickness is
// applied perpendicular to the segment so it looks the same at any angle.
func plotLine(br *canvas.Braille, hx, hy, tx, ty float64, thickness int) {
	dx, dy := tx-hx, ty-hy
	length := math.Hypot(dx, dy)
	if length == 0 {
		return
	}
	nx, ny := -dy/length, dx/length

	steps := int(math.Max(math.Abs(dx), math.Abs(dy))) + 1
	for s := 0; s <= steps; s++ {
		t := float64(s) / float64(steps)
		px, py := hx+dx*t, hy+dy*t
		for k := 0; k < thickness; k++ {
			o := float64(k)
			br.Plot(px+nx*o, py+ny*o)
		}
	}
}

// drawerRows is how tall a bin's drawer is: a border, the bin's number in a box
// of its own (repeated, as in the show), the four bars, and a border.
const drawerRows = 2 + 3 + TemperCount

// drawDrawer paints the four tempers bin i holds, as far out as it has slid.
// The foot sits on the bin's top edge and it emerges upward, so a drawer half
// out shows the bottom half of the panel.
func (m *Model) drawDrawer(buf *canvas.Buffer, x, w, i int) {
	if m.Drawer == nil || !m.Drawer.Showing(i) || w < 10 {
		return
	}

	// How many of its rows are out, starting one row above the bin.
	shown := int(float64(drawerRows)*m.Drawer.Out(i) + 0.5)
	if shown < 1 {
		return
	}
	if shown > drawerRows {
		shown = drawerRows
	}
	bottom := m.Layout.BinTop - 1
	top := bottom - shown + 1
	if top < m.Layout.GridTop {
		return
	}

	// Clear behind it, since Box paints only edges and the drawer is opaque.
	for row := top; row <= bottom; row++ {
		for col := x; col < x+w; col++ {
			buf.Set(col, row, ' ', m.Theme.Surface)
		}
	}

	attr := m.Theme.Chrome
	buf.Box(x, top, w, shown, attr)

	// row maps a panel row, numbered from the panel's top, to the screen.
	row := func(n int) int { return bottom - (drawerRows - 1 - n) }

	// The number's box spans rows 1 to 3, text on row 2, shown once the panel
	// has risen far enough.
	label := clipBin(binLabel(m.Grid.Bins[i].ID), w)
	if y := row(2); y-1 >= top {
		buf.Box(x+2, y-1, w-4, 3, attr)
		buf.Text(binTextX(m.Layout.BinCenterX[i], len(label), x, w), y, label, attr)
	}

	for t := 0; t < TemperCount; t++ {
		if y := row(4 + t); y >= top && y <= bottom {
			m.drawTemperBar(buf, x, w, i, Temper(t), y)
		}
	}
}

// drawTemperBar paints one temper on row y: its two letters, then a bar in
// its colour, outlined, with the share this bin holds filled in, as the show's
// are. The outline's sides are hairlines on the inside edges of their cells;
// its foot is the hairline along each empty cell's foot, and its top an
// overline, which a terminal that cannot draw one leaves off. No percentage:
// the bars are compared against each other.
func (m *Model) drawTemperBar(buf *canvas.Buffer, x, w, i int, t Temper, y int) {
	if m.Grid.Bins[i] == nil {
		return
	}
	lit := m.Theme.On(m.Theme.Tempers[t], true)
	buf.Text(x+3, y, temperNames[t], lit)

	// A blink takes the bars off and leaves their names, as in the show.
	if m.Drawer.Dark(i) {
		return
	}

	left := x + 3 + len(temperNames[t]) + 1
	right := x + w - 4
	inner := right - left - 1
	if inner < 1 {
		return
	}
	buf.Set(left, y, '▕', lit)
	buf.Set(right, y, '▏', lit)

	// Each temper's full bar is its share of the bin (Bin.TemperShare), grown
	// toward as digits land.
	exact := m.Drawer.Share(i, t) * float64(inner)
	whole := min(int(exact), inner)
	empty := lit
	empty.Over = true
	for k := 0; k < inner; k++ {
		switch {
		case k < whole:
			buf.Set(left+1+k, y, '█', lit)
		case k == whole && partialBlock(exact-float64(whole)) != 0:
			buf.Set(left+1+k, y, partialBlock(exact-float64(whole)), empty)
		default:
			buf.Set(left+1+k, y, '▁', empty)
		}
	}
}
