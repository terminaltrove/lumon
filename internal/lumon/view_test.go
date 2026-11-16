package lumon

import (
	"github.com/terminaltrove/lumon/internal/glyph"
	"github.com/terminaltrove/lumon/internal/theme"
	"strconv"
	"strings"
	"testing"

	"github.com/terminaltrove/lumon/internal/canvas"
)

func drawTo(m *Model) *canvas.Buffer {
	// Built the way View builds it, page and all: the page is laid down at
	// allocation, so a plain buffer would leave a paper theme untested.
	buf := canvas.NewBufferWith(m.Layout.Width, m.Layout.Height, m.Theme.Page)
	m.Draw(buf)
	// The sweep is part of a frame: it returns the mark's placement ids. Without it
	// a test that draws repeatedly runs out of ids after four frames and every mark
	// after that comes out as braille.
	m.sweepMark(buf)
	return buf
}

func TestViewProducesExactlyOneLinePerRow(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)

	lines := strings.Split(m.View(), "\n")

	if len(lines) != 40 {
		t.Errorf("View() produced %d lines, want 40", len(lines))
	}
}

func TestHeaderShowsTheFileAndItsProgress(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)

	buf := drawTo(m)

	var header string
	for y := 0; y < m.Layout.HeaderH; y++ {
		header += buf.Row(y)
	}
	if m.header.name.empty() {
		t.Errorf("header does not name the file %q:\n%s", m.File.Name, header)
	}
	// The completion is a band: on a bar this wide it is set in glyphs, so there is
	// no "% Complete" in the screen to look for.
	if m.header.pct.empty() {
		t.Errorf("header does not report progress:\n%s", header)
	}
}

// The banner is in the footer, since the header is the file bar.
func TestTheBannerIsStillOnScreen(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)

	buf := drawTo(m)

	var screen string
	_, h := buf.Size()
	for y := 0; y < h; y++ {
		screen += buf.Row(y)
	}
	if !strings.Contains(screen, "LUMON") {
		t.Error("nothing on screen mentions LUMON")
	}
}

func TestRulesFrameTheGrid(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	l := m.Layout

	buf := drawTo(m)

	// Below the board's rectangle, where the rule is drawn, not GridTop +
	// ViewportH*RowPitch. Layout keeps the leftover BoardH%RowH rows inside the
	// rectangle so the bins do not move by a row as the board is zoomed, so the two
	// agree only when the spacing divides it.
	above := buf.Row(l.GridTop - 1)
	below := buf.Row(l.BoardY + l.BoardH)
	if !strings.Contains(above, "═") {
		t.Errorf("no rule directly above the grid: %q", above)
	}
	if !strings.Contains(below, "═") {
		t.Errorf("no rule directly below the grid: %q", below)
	}
}

// The lens is parked off screen first: it moves digits away from their layout
// position, so near the grid this identity should fail
// (TestTheLensPushesNeighboursOutward).
func TestEveryVisibleDigitIsDrawnWhereLayoutSaysItIs(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	l := m.Layout
	m.Lens.Warp(-1000, -1000)

	buf := drawTo(m)

	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			sx, sy := l.ScreenOf(gx, gy)
			want := rune('0' + m.Grid.At(gx, gy).Value)
			if got := buf.RuneAt(sx, sy); got != want {
				t.Fatalf("cell (%d,%d) at screen (%d,%d) = %q, want %q", gx, gy, sx, sy, got, want)
			}
			if ax, ay := m.ScreenAnchor(GridIndex(gx, gy)); ax != sx || ay != sy {
				t.Fatalf("cell (%d,%d) anchors at (%d,%d) with no lens, want Layout's (%d,%d)", gx, gy, ax, ay, sx, sy)
			}
		}
	}
}

func TestScrollingChangesWhichDigitsAreDrawn(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)

	before := drawTo(m).Row(m.Layout.GridTop)
	m.Layout.ScrollTo(20, 20)
	after := drawTo(m).Row(m.Layout.GridTop)

	if before == after {
		t.Error("scrolling did not change the rendered grid")
	}
}

func TestCursorCellIsStyledDifferentlyFromItsNeighbour(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	m.CursorX, m.CursorY = 3, 3

	buf := drawTo(m)

	cx, cy := m.Layout.ScreenOf(3, 3)
	nx, ny := m.Layout.ScreenOf(4, 3)
	if buf.AttrAt(cx, cy) == buf.AttrAt(nx, ny) {
		t.Error("the cursor cell is styled the same as its neighbour")
	}
}

func TestBadNumbersAreStyledDifferentlyFromGoodOnes(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	l := m.Layout

	buf := drawTo(m)

	var goodAttr, badAttr canvas.Attr
	var haveGood, haveBad bool
	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH && !(haveGood && haveBad); gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			if gx == m.CursorX && gy == m.CursorY {
				continue
			}
			sx, sy := l.ScreenOf(gx, gy)
			if m.Grid.At(gx, gy).BadGroupID >= 0 {
				badAttr, haveBad = buf.AttrAt(sx, sy), true
			} else {
				goodAttr, haveGood = buf.AttrAt(sx, sy), true
			}
		}
	}

	if !haveGood || !haveBad {
		t.Skip("viewport contains only one kind of number")
	}
	if goodAttr == badAttr {
		t.Error("bad numbers render identically to good ones")
	}
}

func TestBinsShowTheirNumberAndPercentage(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	l := m.Layout

	buf := drawTo(m)

	var bins string
	for y := l.BinTop; y < l.BinTop+BinsHeight; y++ {
		bins += buf.Row(y) + "\n"
	}
	for i := 1; i <= BinCount; i++ {
		if !strings.Contains(bins, string(rune('0'+i))) {
			t.Errorf("bin row does not show bin %d:\n%s", i, bins)
		}
	}
	if !strings.Contains(bins, "%") {
		t.Errorf("bin row shows no percentage:\n%s", bins)
	}
}

func TestSelectedBinIsStyledDifferently(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	m.SelectedBin = 0

	buf := drawTo(m)

	first := buf.AttrAt(m.Layout.BinCenterX[0], m.Layout.BinTop)
	second := buf.AttrAt(m.Layout.BinCenterX[1], m.Layout.BinTop)
	if first == second {
		t.Error("the selected bin is styled the same as an unselected one")
	}
}

func TestRenderingSurvivesATinyTerminal(t *testing.T) {
	m := NewModel(1)
	m.Resize(8, 5)

	out := m.View()

	if len(strings.Split(out, "\n")) != 5 {
		t.Errorf("got %d lines, want 5", len(strings.Split(out, "\n")))
	}
}

func TestResizeRecomputesTheLayout(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	wide := m.Layout.ViewportW

	m.Resize(60, 40)

	if m.Layout.ViewportW >= wide {
		t.Errorf("viewport did not shrink with the terminal: %d then %d", wide, m.Layout.ViewportW)
	}
}

func TestAgitatedDigitsAreDrawnAwayFromTheirCell(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	tick(m, 200)

	buf := drawTo(m)

	displaced := 0
	for _, id := range grp.NumberIDs {
		gx, gy := id%GridSize, id/GridSize
		sx, sy := m.Layout.ScreenOf(gx, gy)
		if buf.RuneAt(sx, sy) != rune('0'+m.Grid.Cells[id].Value) {
			displaced++
		}
	}
	if displaced == 0 {
		t.Error("no digit in the agitating cluster was drawn off its cell centre")
	}
}

func TestInFlightDigitsAreOmittedFromTheGrid(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	m.Update(key("enter"))
	// Move the cursor well away first. The fisheye enlarges whatever is near it,
	// and a neighbour's glyph painted over the vacated cells would hide whether the
	// grid pass skipped them.
	m.CursorX, m.CursorY = m.Layout.ViewportX+m.Layout.ViewportW-1, m.Layout.ViewportY+m.Layout.ViewportH-1
	// A digit launches from exactly where it was drawn, so at frame zero the flight
	// overlaps its own cell. Once it has moved, the grid must not be drawing it.
	tick(m, 60)

	buf := drawTo(m)
	airborne := map[int]bool{}
	for _, f := range m.Flights.Active() {
		airborne[f.ID] = true
	}
	if len(airborne) == 0 {
		t.Skip("every digit had already landed")
	}

	for id := range airborne {
		gx, gy := id%GridSize, id/GridSize
		sx, sy := m.Layout.ScreenOf(gx, gy)
		if got := buf.RuneAt(sx, sy); got != ' ' {
			t.Errorf("cell (%d,%d) still shows %q while its digit is airborne", gx, gy, got)
		}
	}
}

func TestFlightsAreDrawnInTheAir(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	m.Update(key("enter"))
	tick(m, 20)

	buf := drawTo(m)

	drawn := 0
	for _, f := range m.Flights.Active() {
		// A digit still large enough to be a glyph is a block of ramp characters; once
		// it has shrunk past the threshold it is the plain digit. Either way something
		// of it has to be on screen.
		x, y := int(f.X+0.5), int(f.Y+0.5)
		if f.Height >= MinGlyphHeight {
			if buf.RuneAt(x, y) != ' ' {
				drawn++
			}
			continue
		}
		if buf.RuneAt(x, y) == f.Digit {
			drawn++
		}
	}
	if drawn == 0 {
		t.Errorf("none of the %d airborne digits were drawn", len(m.Flights.Active()))
	}
}

func TestIdleScreenIsAllMark(t *testing.T) {
	m := ready()
	m.Update(key("i"))
	tick(m, 60)

	out := m.View()

	if strings.Contains(out, "LUMON INDUSTRIES") {
		t.Error("the idle screen still shows the main screen chrome")
	}
}

func TestInactiveNumbersAreDarkerThanActiveOnes(t *testing.T) {
	if theme.AttrDigit.FG == theme.AttrBadLive.FG {
		t.Fatal("inactive and active numbers share a colour")
	}
	if luminance(theme.AttrDigit.FG.String()) >= luminance(theme.AttrBadLive.FG.String()) {
		t.Errorf("inactive %s is not darker than active %s", theme.AttrDigit.FG, theme.AttrBadLive.FG)
	}
}

// The background grid recedes: about half brightness, as in the show.
func TestBackgroundNumbersAreGenuinelyDim(t *testing.T) {
	if l := luminance(theme.AttrDigit.FG.String()); l > 0.5 {
		t.Errorf("theme.AttrDigit luminance is %.2f, want a dim background colour", l)
	}
}

// As in the show, a resting digit can be read at a glance: well clear of the
// backdrop, heavier than chrome, and still below a waking cluster.
func TestRestingDigitsReadAgainstTheScreen(t *testing.T) {
	for _, th := range theme.Themes() {
		if th.Paper.Set() {
			continue
		}
		digit := luminance(th.Digit.FG.String())
		back := luminance(th.Backdrop.String())
		live := luminance(th.Live.String())
		if digit < back*1.5 {
			t.Errorf("%s: a resting digit (%.3f) is barely off the backdrop (%.3f)", th.Name, digit, back)
		}
		if digit >= live {
			t.Errorf("%s: a resting digit (%.3f) is as bright as a waking one (%.3f)", th.Name, digit, live)
		}
		if !th.Digit.Bold {
			t.Errorf("%s: resting digits are not bold", th.Name)
		}
	}
}

// Otherwise brightening the grid would leave the dormant corrupt digits at the
// old, darker shade, and give every cluster away before it wakes.
func TestADormantDigitStaysAShadeOffARestingOne(t *testing.T) {
	for _, th := range theme.Themes() {
		if th.Bad.FG == th.Digit.FG {
			t.Errorf("%s: a dormant corrupt digit is indistinguishable from a resting one", th.Name)
		}
		if gap := colourGap(th.Bad.FG, th.Digit.FG); gap > 40 {
			t.Errorf("%s: a dormant corrupt digit %v stands %d off a resting one %v",
				th.Name, th.Bad.FG, gap, th.Digit.FG)
		}
		if th.Bad.Bold != th.Digit.Bold {
			t.Errorf("%s: dormant and resting digits differ in weight", th.Name)
		}
	}
}

// Otherwise it gives itself away before its cluster wakes.
func TestDormantCorruptNumbersDoNotStandOut(t *testing.T) {
	dim, bad := luminance(theme.AttrDigit.FG.String()), luminance(theme.AttrBad.FG.String())
	if bad-dim > 0.12 {
		t.Errorf("dormant corrupt (%.2f) is much brighter than ordinary (%.2f)", bad, dim)
	}
}

func luminance(hex string) float64 {
	if len(hex) != 7 || hex[0] != '#' {
		return 0
	}
	v := func(i int) float64 {
		var n int
		for _, c := range hex[i : i+2] {
			n *= 16
			switch {
			case c >= '0' && c <= '9':
				n += int(c - '0')
			case c >= 'a' && c <= 'f':
				n += int(c-'a') + 10
			case c >= 'A' && c <= 'F':
				n += int(c-'A') + 10
			}
		}
		return float64(n) / 255
	}
	return 0.2126*v(1) + 0.7152*v(3) + 0.0722*v(5)
}

func TestBackgroundNumbersDriftOnScreen(t *testing.T) {
	m := ready()
	tick(m, 300)

	buf := drawTo(m)
	l := m.Layout

	displaced := 0
	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			if gx == m.CursorX && gy == m.CursorY {
				continue
			}
			sx, sy := l.ScreenOf(gx, gy)
			if buf.RuneAt(sx, sy) != rune('0'+m.Grid.At(gx, gy).Value) {
				displaced++
			}
		}
	}
	if displaced == 0 {
		t.Error("no background number was drawn off its cell centre")
	}
}

// activateClusterWithRoom wakes a cluster far enough from the viewport edges
// that a full-size glyph on it is not mostly clipped, and parks the cursor on
// one of its members.
func activateClusterWithRoom(t *testing.T, m *Model) (*BadGroup, int) {
	t.Helper()
	l := m.Layout
	for gy := l.ViewportY + 6; gy < l.ViewportY+l.ViewportH-6; gy++ {
		for gx := l.ViewportX + 3; gx < l.ViewportX+l.ViewportW-3; gx++ {
			n := m.Grid.At(gx, gy)
			if n.BadGroupID < 0 {
				continue
			}
			m.Activate(m.Grid.Group(n.BadGroupID))
			m.CursorX, m.CursorY = gx, gy
			return m.FirstActive(), n.ID
		}
	}
	t.Skip("no bad cluster sits clear of the viewport edges")
	return nil, 0
}

// The fisheye reaches every number, corrupt or not. Colour is the other half of
// the signal: only the woken cluster is bright.

// attrOf returns the attribute a digit is drawn with.
func attrOf(m *Model, gridID int) canvas.Attr {
	buf := drawTo(m)
	sx, sy := m.Layout.ScreenOf(gridID%GridSize, gridID/GridSize)
	return buf.AttrAt(sx, sy)
}

func TestWokenClusterIsDrawnScary(t *testing.T) {
	m := ready()
	grp, focus := activateClusterWithRoom(t, m)
	parkCursorAwayHelper(m)
	tick(m, 900)

	want := theme.ScaryColorAt(m.ScaryElapsed())
	for _, id := range grp.NumberIDs {
		if id == focus {
			continue
		}
		got, ok := glyphAttrOf(t, m, id)
		if !ok {
			continue
		}
		if got.FG != want {
			t.Errorf("member %d is drawn %s, want the urgency colour %s", id, got.FG, want)
		}
		if !got.Bold {
			t.Errorf("member %d is not bold", id)
		}
		return
	}
	t.Skip("no non-hovered cluster member is drawn large")
}

// Otherwise it gives itself away.
func TestDormantClustersAreNeverScaryColoured(t *testing.T) {
	m := ready()
	l := m.Layout
	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			n := m.Grid.At(gx, gy)
			if n.BadGroupID < 0 {
				continue
			}
			m.CursorX, m.CursorY = gx, gy
			tick(m, 600)
			if m.IsActive(n.BadGroupID) {
				t.Skip("the cluster woke on its own during the test")
			}
			if got := attrOf(m, n.ID); isScaryColour(got.FG) {
				t.Fatalf("a dormant corrupt digit is drawn %s, a scary colour", got.FG)
			}
			return
		}
	}
}

func TestOrdinaryNumbersAreNeverScaryColoured(t *testing.T) {
	m := ready()
	activateClusterWithRoom(t, m)
	tick(m, 600)

	buf := drawTo(m)
	l := m.Layout

	// An enlarged member's glyph paints over neighbouring cells, so only cells
	// outside every glyph box show what the grid pass drew.
	inGlyph := func(x, y int) bool {
		for _, b := range m.boxes {
			if x >= b.x && x < b.x+b.w && y >= b.y && y < b.y+b.h {
				return true
			}
		}
		return false
	}

	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			n := m.Grid.At(gx, gy)
			if m.IsActive(n.BadGroupID) {
				continue
			}
			sx, sy := l.ScreenOf(gx, gy)
			if inGlyph(sx, sy) {
				continue
			}
			if isScaryColour(buf.AttrAt(sx, sy).FG) {
				t.Fatalf("ordinary digit (%d,%d) is drawn in a scary colour", gx, gy)
			}
		}
	}
}

// isScaryColour reports whether a colour lies on the cyan-to-red urgency ramp.
// The scary colour is a ramp, not a constant, so it is asked of the ramp.
func isScaryColour(c canvas.Color) bool {
	for i := 0; i <= 64; i++ {
		if c == theme.ScaryColorAt(theme.ScaryRedSeconds*float64(i)/64) {
			return true
		}
	}
	return false
}

// Otherwise a cluster stops being visible partway through reddening.
func TestBothEndsOfTheScaryRampStandOut(t *testing.T) {
	bg := luminance(theme.AttrNumber.FG.String())
	for _, c := range []canvas.Color{theme.ScaryColorAt(0), theme.ScaryColorAt(theme.ScaryRedSeconds / 2), theme.ScaryColorAt(theme.ScaryRedSeconds)} {
		if luminance(c.String()) <= bg {
			t.Errorf("urgency colour %s is not brighter than the background %s", c, theme.AttrNumber.FG)
		}
	}
}

func TestFocusedDigitIsDrawnLarge(t *testing.T) {
	m := ready()
	_, focus := activateClusterWithRoom(t, m)

	tick(m, 700)
	h := m.Magnify.Height(focus%GridSize, focus/GridSize)
	if h < MinGlyphHeight {
		t.Fatalf("the focused digit is only %v rows tall", h)
	}

	buf := drawTo(m)
	cov := glyph.Rasterize(m.Grid.Cells[focus].Value, h*glyph.CellAspect, h)
	cx, cy := m.Layout.ScreenOf(focus%GridSize, focus/GridSize)

	// Solid, not the digit. An enlarged number is a shape spread over many cells,
	// and drawing it in copies of itself makes figure and ground the same character,
	// with no silhouette.
	found := 0
	for row := 0; row < cov.H; row++ {
		for col := 0; col < cov.W; col++ {
			if buf.RuneAt(cx-cov.W/2+col, cy-cov.H/2+row) == glyph.GlyphSolid {
				found++
			}
		}
	}
	if found < 10 {
		t.Errorf("only %d cells of the enlarged glyph were drawn solid", found)
	}
}

// plannedBox returns the glyph box the renderer placed for a digit. An enlarged
// digit is anchored on its cell plus its cluster's agitation that frame, so
// tests read the placement instead of recomputing it from Layout.
func plannedBox(m *Model, gridID int) (glyphBox, bool) {
	for _, b := range m.boxes {
		if b.id == gridID {
			return b, true
		}
	}
	return glyphBox{}, false
}

// ownsCell reports whether only this box covers a screen position. Glyph boxes
// may touch (only their centres are protected), so a neighbour may paint inside
// another's blank margin, and a test of what a glyph drew must skip shared
// cells.
func ownsCell(m *Model, box glyphBox, x, y int) bool {
	for _, other := range m.boxes {
		if other.id == box.id {
			continue
		}
		if x >= other.x && x < other.x+other.w && y >= other.y && y < other.y+other.h {
			return false
		}
	}
	return true
}

func TestEnlargedGlyphMatchesItsRasterization(t *testing.T) {
	m := ready()
	_, focus := activateClusterWithRoom(t, m)
	tick(m, 900)

	buf := drawTo(m)
	box, ok := plannedBox(m, focus)
	if !ok {
		t.Skip("the focused digit did not reach glyph size")
	}
	for row := 0; row < box.cov.H; row++ {
		for col := 0; col < box.cov.W; col++ {
			x, y := box.x+col, box.y+row
			if x < 0 || y < 0 || x >= m.Layout.Width || y >= m.Layout.Height {
				continue
			}
			if y < m.Layout.GridTop || y > m.Layout.GridTop+m.Layout.ViewportH-1 {
				continue
			}
			if !ownsCell(m, box, x, y) {
				continue
			}
			c := box.cov.At(col, row)
			want := ' '
			if c > 0 {
				want = glyph.RampRune(c)
			}
			if got := buf.RuneAt(x, y); got != want {
				t.Fatalf("glyph cell (%d,%d) coverage %.2f = %q, want %q", col, row, c, got, want)
			}
		}
	}
}

// The antialiased edge makes a fractional size look smooth, so ramp characters
// have to reach the screen.
func TestEnlargedGlyphShowsItsAntialiasedEdge(t *testing.T) {
	m := ready()
	_, focus := activateClusterWithRoom(t, m)

	sawRamp := false
	for f := 0; f < 900 && !sawRamp; f++ {
		tick(m, 1)
		buf := drawTo(m)
		h := m.Magnify.Height(focus%GridSize, focus/GridSize)
		if h < MinGlyphHeight {
			continue
		}
		cov := glyph.Rasterize(m.Grid.Cells[focus].Value, h*glyph.CellAspect, h)
		cx, cy := m.Layout.ScreenOf(focus%GridSize, focus/GridSize)
		for row := 0; row < cov.H && !sawRamp; row++ {
			for col := 0; col < cov.W; col++ {
				r := buf.RuneAt(cx-cov.W/2+col, cy-cov.H/2+row)
				if r != ' ' && r != glyph.GlyphSolid && glyph.IsGlyphRune(r) {
					sawRamp = true
					break
				}
			}
		}
	}

	if !sawRamp {
		t.Error("no ramp characters were drawn; the glyph has no soft edge")
	}
}

func TestEnlargedGlyphIsClippedToTheScreen(t *testing.T) {
	m := ready()
	l := m.Layout
	for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
		n := m.Grid.At(gx, l.ViewportY)
		if n.BadGroupID < 0 {
			continue
		}
		m.Activate(m.Grid.Group(n.BadGroupID))
		m.CursorX, m.CursorY = gx, l.ViewportY
		break
	}
	tick(m, 700)

	if got := len(strings.Split(m.View(), "\n")); got != m.Layout.Height {
		t.Errorf("frame is %d lines, want %d; the glyph broke the layout", got, m.Layout.Height)
	}
}

func TestNothingIsEnlargedInIdleMode(t *testing.T) {
	m := ready()
	activateClusterWithRoom(t, m)
	tick(m, 400)
	m.Update(key("i"))
	tick(m, 60)

	if strings.Contains(m.View(), "LUMON INDUSTRIES") {
		t.Error("main screen content leaked into idle mode")
	}
}

func TestEnlargedGlyphClearsTheGridBehindIt(t *testing.T) {
	m := ready()
	_, focus := activateClusterWithRoom(t, m)
	tick(m, 900)

	buf := drawTo(m)
	box, ok := plannedBox(m, focus)
	if !ok {
		t.Skip("the focused digit did not reach glyph size")
	}

	for row := 0; row < box.cov.H; row++ {
		for col := 0; col < box.cov.W; col++ {
			if box.cov.At(col, row) > 0 {
				continue
			}
			x, y := box.x+col, box.y+row
			if x < 0 || y < 0 || x >= m.Layout.Width || y >= m.Layout.Height {
				continue
			}
			if y < m.Layout.GridTop || y > m.Layout.GridTop+m.Layout.ViewportH-1 {
				continue
			}
			if !ownsCell(m, box, x, y) {
				continue
			}
			if got := buf.RuneAt(x, y); got != ' ' {
				t.Fatalf("uncovered glyph cell (%d,%d) shows %q; the grid is showing through", col, row, got)
			}
		}
	}
}

func TestEnlargedGlyphDoesNotEraseTheChrome(t *testing.T) {
	m := ready()
	l := m.Layout
	for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
		n := m.Grid.At(gx, l.ViewportY)
		if n.BadGroupID < 0 {
			continue
		}
		m.Activate(m.Grid.Group(n.BadGroupID))
		m.CursorX, m.CursorY = gx, l.ViewportY
		break
	}
	tick(m, 700)

	buf := drawTo(m)
	if row := buf.Row(l.GridTop - 1); !strings.Contains(row, "═") {
		t.Errorf("the rule above the grid was erased: %q", row)
	}
	// Asked as a band, which survives the bar changing how it draws: ink in the
	// columns the bar says it used.
	if m.header.name.empty() {
		t.Fatal("the bar drew no file name at all")
	}
	inked := 0
	for y := 0; y < m.Layout.HeaderH; y++ {
		row := []rune(buf.Row(y))
		for x := m.header.name[0]; x < m.header.name[1] && x < len(row); x++ {
			if row[x] != ' ' {
				inked++
			}
		}
	}
	if inked == 0 {
		t.Error("the header was erased by the enlarged glyph")
	}
}

// Cluster members sit close together and glyphs are several rows tall, so
// strict non-overlap would leave a cluster as one lonely digit. A member is
// dropped only when its centre falls inside a box already placed.
func TestNoGlyphCentreFallsInsideAnother(t *testing.T) {
	m := ready()
	activateClusterWithRoom(t, m)

	for f := 0; f < 900; f++ {
		tick(m, 1)
		m.planGlyphs()
		for i := range m.boxes {
			for j := range m.boxes {
				if i == j {
					continue
				}
				a, b := m.boxes[i], m.boxes[j]
				ccx, ccy := a.x+a.w/2, a.y+a.h/2
				if ccx >= b.x && ccx < b.x+b.w && ccy >= b.y && ccy < b.y+b.h {
					t.Fatalf("frame %d: glyph %d's centre is inside glyph %d", f, a.id, b.id)
				}
			}
		}
	}
}

func TestSeveralClusterMembersAreDrawnTogether(t *testing.T) {
	m := ready()
	var grp *BadGroup
	l := m.Layout
	for gy := l.ViewportY + 4; gy < l.ViewportY+l.ViewportH-4 && grp == nil; gy++ {
		for gx := l.ViewportX + 2; gx < l.ViewportX+l.ViewportW-2; gx++ {
			n := m.Grid.At(gx, gy)
			if n.BadGroupID >= 0 && len(m.Grid.Group(n.BadGroupID).NumberIDs) >= 3 {
				m.Activate(m.Grid.Group(n.BadGroupID))
				m.CursorX, m.CursorY = gx, gy
				grp = m.FirstActive()
				break
			}
		}
	}
	if grp == nil {
		t.Skip("no multi-digit cluster clear of the viewport edges")
	}

	best := 0
	for f := 0; f < int(60*EscalateSeconds); f++ {
		tick(m, 1)
		m.planGlyphs()
		if len(m.boxes) > best {
			best = len(m.boxes)
		}
	}

	if best < 2 {
		t.Errorf("at most %d glyph was drawn for a %d-digit cluster; want several", best, len(grp.NumberIDs))
	}
}

// glyphAttrOf returns the attribute a drawn glyph is painted with, or false if
// the digit is not currently drawn large.
func glyphAttrOf(t *testing.T, m *Model, gridID int) (canvas.Attr, bool) {
	t.Helper()
	buf := drawTo(m)
	box, ok := plannedBox(m, gridID)
	if !ok {
		return canvas.Attr{}, false
	}
	for row := 0; row < box.cov.H; row++ {
		for col := 0; col < box.cov.W; col++ {
			if box.cov.At(col, row) <= 0 {
				continue
			}
			x, y := box.x+col, box.y+row
			if a := buf.AttrAt(x, y); a != (canvas.Attr{}) && a != theme.AttrNumber && a != theme.AttrBad {
				return a, true
			}
		}
	}
	return canvas.Attr{}, false
}

// So it is obvious which one is selected.
func TestHoveredGlyphIsWhite(t *testing.T) {
	m := ready()
	_, focus := activateClusterWithRoom(t, m)
	tick(m, 900)

	got, ok := glyphAttrOf(t, m, focus)
	if !ok {
		t.Skip("the focused digit is not drawn large")
	}
	if got != theme.AttrHovered {
		t.Errorf("the hovered glyph is %+v, want %+v", got, theme.AttrHovered)
	}
}

func TestUnhoveredClusterMembersAreNotWhite(t *testing.T) {
	m := ready()
	grp, focus := activateClusterWithRoom(t, m)
	tick(m, 900)

	for _, id := range grp.NumberIDs {
		if id == focus {
			continue
		}
		got, ok := glyphAttrOf(t, m, id)
		if !ok {
			continue
		}
		if got == theme.AttrHovered {
			t.Errorf("member %d is white without being hovered", id)
		}
		return
	}
}

func TestWokenClusterRedensOverTime(t *testing.T) {
	m := ready()
	grp, _ := activateClusterWithRoom(t, m)
	parkCursorAwayHelper(m)

	// Find a member that is drawn large, and sample its colour over time.
	sample := func() (int, bool) {
		for _, id := range grp.NumberIDs {
			if a, ok := glyphAttrOf(t, m, id); ok {
				r, _, _, parsed := theme.ParseHex(a.FG.String())
				if parsed {
					return r, true
				}
			}
		}
		return 0, false
	}

	tick(m, 300)
	early, ok := sample()
	if !ok {
		t.Skip("no cluster member is drawn large")
	}

	tick(m, 1800) // several more seconds
	late, ok := sample()
	if !ok {
		t.Skip("no cluster member is drawn large")
	}

	if late <= early {
		t.Errorf("red channel went %d -> %d over time, want it to rise", early, late)
	}
}

func TestActivatingANewClusterResetsTheUrgency(t *testing.T) {
	m := ready()
	grp, _ := activateClusterWithRoom(t, m)
	parkCursorAwayHelper(m)
	tick(m, 1800)
	aged := m.ScaryElapsed()
	if aged <= 1 {
		t.Skip("the urgency clock did not advance")
	}

	// Wake a different cluster.
	for _, other := range m.Grid.Groups {
		if other.ID != grp.ID {
			m.Activate(other)
			break
		}
	}

	if got := m.ScaryElapsed(); got >= aged {
		t.Errorf("urgency is %v after waking a new cluster, want it reset from %v", got, aged)
	}
}

// The lean has to reach the screen.
func TestFlyingGlyphIsDrawnTilted(t *testing.T) {
	m := ready()
	grp, focus := activateClusterWithRoom(t, m)
	tick(m, 900)
	if m.Magnify.Height(focus%GridSize, focus/GridSize) < MinGlyphHeight+2 {
		t.Skip("the cluster did not grow enough to see a lean")
	}
	_ = grp

	m.Update(key("enter"))

	differed := false
	for i := 0; i < 90 && !differed; i++ {
		var f Flight
		found := false
		for _, a := range m.Flights.Active() {
			if a.ID == focus {
				f, found = a, true
				break
			}
		}
		if !found || f.Tilt == 0 || f.Height < MinGlyphHeight {
			tick(m, 1)
			continue
		}

		buf := drawTo(m)
		// The digit in the air is the one that left, not the cell's replacement: the
		// board commits the replacement at refine time so it does not depend on when
		// anybody's animation finishes.
		value := int(f.Digit - '0')
		upright := glyph.Rasterize(value, f.Height*glyph.CellAspect, f.Height)
		leaning := glyph.RasterizeRotated(value, f.Height*glyph.CellAspect, f.Height, f.Tilt)
		if upright.String() == leaning.String() {
			tick(m, 1)
			continue // this angle happens to round to the same glyph
		}

		// The screen should show the leaning form, not the upright one, except where a
		// cluster-mate has painted over it. Refined digits fan out from a common centre,
		// so only cells this digit alone covers can be asserted on.
		ox, oy := int(f.X+0.5)-leaning.W/2, int(f.Y+0.5)-leaning.H/2
		_, height := buf.Size()
		shared := func(x, y int) bool {
			for _, other := range m.Flights.Active() {
				if other.ID == f.ID || other.Height < MinGlyphHeight {
					continue
				}
				ow := int(other.Height*glyph.CellAspect) + 2
				oh := int(other.Height) + 2
				oxx, oyy := int(other.X+0.5)-ow/2, int(other.Y+0.5)-oh/2
				if x >= oxx && x < oxx+ow && y >= oyy && y < oyy+oh {
					return true
				}
			}
			return false
		}

		matchesLeaning := true
		checked := 0
		for row := 0; row < leaning.H && matchesLeaning; row++ {
			for col := 0; col < leaning.W; col++ {
				c := leaning.At(col, row)
				if c <= 0 {
					continue
				}
				x, y := ox+col, oy+row
				if y < 0 || y >= height || shared(x, y) {
					continue
				}
				checked++
				if buf.RuneAt(x, y) != glyph.RampRune(c) {
					matchesLeaning = false
					break
				}
			}
		}
		if checked == 0 {
			tick(m, 1)
			continue // entirely overlapped this frame
		}
		if !matchesLeaning {
			t.Fatal("the flying glyph on screen does not match its tilted rasterization")
		}
		differed = true
	}

	if !differed {
		t.Skip("the flight never produced a visibly distinct lean")
	}
}

// So the floor shows where the work has gone.
func TestABinFillsInProportionToWhatItHolds(t *testing.T) {
	m := ready()
	m.Resize(140, 40)

	m.Room.mu.Lock()
	m.Grid.Bins[0].Refined = 0
	m.Grid.Bins[4].Refined = m.Grid.Bins[4].Quota
	m.Room.mu.Unlock()
	tick(m, 600) // let the bars ease all the way

	if got := m.BinFill.Fill(0); got > 0.02 {
		t.Errorf("an empty bin is drawn %.0f%% full", got*100)
	}
	if got := m.BinFill.Fill(4); got < 0.98 {
		t.Errorf("a full bin is drawn %.0f%% full", got*100)
	}

	// And the fill is painted, not merely computed.
	buf := drawTo(m)
	l := m.Layout
	y := l.BinTop + 4
	filled := 0
	for x := l.BinOriginX(4) + 1; x < l.BinOriginX(4)+l.BinWidth(4)-1; x++ {
		if buf.AttrAt(x, y).BG == m.Theme.BinFull.BG {
			filled++
		}
	}
	if filled == 0 {
		t.Error("a full bin has no fill painted behind its readout")
	}
}

// Like the file bar above it, so a cluster landing registers as something that
// happened.
func TestABinsBarEasesRatherThanJumping(t *testing.T) {
	m := ready()
	m.Resize(140, 40)

	m.Room.mu.Lock()
	m.Grid.Bins[2].Refined = m.Grid.Bins[2].Quota
	m.Room.mu.Unlock()

	tick(m, 2)
	early := m.BinFill.Fill(2)
	if early > 0.5 {
		t.Errorf("the bar jumped to %.0f%% in two frames", early*100)
	}
	tick(m, 600)
	if late := m.BinFill.Fill(2); late < 0.98 {
		t.Errorf("the bar settled at %.0f%%, want it reaching the top", late*100)
	}
}

// Otherwise the bins are inert until a number appears in them.
func TestABinIsLitWhileDigitsAreOnTheirWayToIt(t *testing.T) {
	m := ready()
	grp := grownCluster(t, m)
	m.SelectedBin = 3
	id := grp.NumberIDs[0]
	m.CursorX, m.CursorY = id%GridSize, id/GridSize

	if m.BinFill.Receiving(3) {
		t.Fatal("a bin is lit before anything was sent to it")
	}

	m.Update(key("enter"))
	tick(m, 2)

	if len(m.Flights.Active()) == 0 {
		t.Skip("nothing took off")
	}
	if !m.BinFill.Receiving(3) {
		t.Error("the bin being flown to is not lit")
	}
	if m.BinFill.Receiving(0) {
		t.Error("a bin nothing was sent to is lit")
	}

	// And it goes out once the sky is empty.
	for i := 0; i < 60*10 && len(m.Flights.Active()) > 0; i++ {
		tick(m, 1)
	}
	if m.BinFill.Receiving(3) {
		t.Error("the bin is still lit after everything landed")
	}
}

// Easing down from the last file's contents would read as the work being
// undone.
func TestTheBinBarsStartTheNextFileEmpty(t *testing.T) {
	m := ready()
	m.Resize(140, 40)
	completeTheFile(m)
	tick(m, 60)
	if m.BinFill.Fill(0) < 0.5 {
		t.Skip("the bars never filled")
	}

	tick(m, int(FileHoldSeconds*60)+10)

	for i := 0; i < BinCount; i++ {
		if got := m.BinFill.Fill(i); got > 0.02 {
			t.Errorf("bin %d starts the new file %.0f%% full", i+1, got*100)
		}
	}
}

// launchInto refines the cluster under the cursor into a bin and returns once
// the digits are airborne.
func launchInto(t *testing.T, m *Model, bin int) {
	t.Helper()
	grp := grownCluster(t, m)
	m.SelectedBin = bin
	id := grp.NumberIDs[0]
	m.CursorX, m.CursorY = id%GridSize, id/GridSize
	m.Update(key("enter"))
	tick(m, 1)
	if len(m.Flights.Active()) == 0 {
		t.Skip("nothing took off")
	}
}

// Otherwise the numbers fly through the lid.
func TestTheLidIsOpenBeforeTheDigitsArrive(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	launchInto(t, m, 1)

	// Run to the moment the first digit lands and see how open the lid was then.
	// The angle at an arbitrary frame would only measure the spring's tuning.
	airborne := len(m.Flights.Active())
	atFirstLanding := -1.0
	for i := 0; i < 60*10; i++ {
		tick(m, 1)
		if n := len(m.Flights.Active()); n < airborne {
			atFirstLanding = m.BinFill.Lid(1)
			break
		}
	}
	if atFirstLanding < 0 {
		t.Skip("nothing landed")
	}
	if atFirstLanding < LidOpen*0.95 {
		t.Errorf("the first digit arrived with the lid only %.0f of %v degrees open",
			atFirstLanding, LidOpen)
	}
	if !m.BinFill.Ajar(1) {
		t.Error("the lid does not read as open while digits are on their way")
	}
	// And only that bin's.
	if m.BinFill.Ajar(3) {
		t.Error("a bin nothing was sent to opened its lid")
	}
}

func TestTheLidShutsOnceTheNumbersAreIn(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	launchInto(t, m, 2)

	for i := 0; i < 60*10 && len(m.Flights.Active()) > 0; i++ {
		tick(m, 1)
	}
	if len(m.Flights.Active()) != 0 {
		t.Skip("the digits never landed")
	}
	tick(m, 60*3) // the drawer's hold, its way down, and the lid's swing

	if got := m.BinFill.Lid(2); got > 1 {
		t.Errorf("the lid rests %.1f degrees open after everything landed", got)
	}
	if m.BinFill.Ajar(2) {
		t.Error("the lid still reads as open with nothing in the air")
	}
}

// It rings, as a hinged thing with mass does. A lid that reaches open without
// overshooting reads as a number being eased.
func TestTheLidSwingsPastVerticalAndSettlesBack(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	launchInto(t, m, 0)

	peak := 0.0
	for i := 0; i < 60; i++ {
		tick(m, 1)
		if got := m.BinFill.Lid(0); got > peak {
			peak = got
		}
	}
	if peak <= LidOpen {
		t.Errorf("the lid peaked at %.1f degrees; a sprung hinge should carry past %v", peak, LidOpen)
	}
}

// The two flaps are hinged at the bin's corners and splay outward into a V,
// overhanging the neighbours, but must not reach the bin beyond.
func TestTheFlapsSplayOutwardButNotOntoTheNextBinButOne(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	launchInto(t, m, 2)

	l := m.Layout
	// The far edges of the bins either side. A flap may cross into its neighbour;
	// it may not cross that neighbour entirely.
	tooFarLeft := l.BinOriginX(1)
	tooFarRight := l.BinOriginX(3) + l.BinWidth(3)

	overhung := false
	for i := 0; i < 90; i++ {
		tick(m, 1)
		buf := drawTo(m)
		for y := l.BinTop - int(lidRise) - 1; y <= l.BinTop; y++ {
			for x := 0; x < l.Width; x++ {
				r := buf.RuneAt(x, y)
				if r <= canvas.BrailleBlank || r > canvas.BrailleBlank+0xFF {
					continue
				}
				if x < tooFarLeft || x >= tooFarRight {
					t.Fatalf("at %.1f degrees a flap painted at column %d, past the bin but one",
						m.BinFill.Lid(2), x)
				}
				if x < l.BinOriginX(2) || x >= l.BinOriginX(2)+l.BinWidth(2) {
					overhung = true
				}
			}
		}
	}
	if !overhung {
		t.Error("neither flap ever overhung its bin; they are not splaying")
	}
}

// Both flaps, hinged at their own corners, mirrored, as in the reference.
func TestABinHasTwoFlapsNotOne(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	launchInto(t, m, 2)
	tick(m, 20)

	l := m.Layout
	buf := drawTo(m)
	x, w := l.BinOriginX(2), l.BinWidth(2)
	mid := x + w/2

	var leftHalf, rightHalf int
	for y := l.BinTop - int(lidRise) - 1; y < l.BinTop; y++ {
		for cx := 0; cx < l.Width; cx++ {
			r := buf.RuneAt(cx, y)
			if r <= canvas.BrailleBlank || r > canvas.BrailleBlank+0xFF {
				continue
			}
			if cx < mid {
				leftHalf++
			} else {
				rightHalf++
			}
		}
	}
	if leftHalf == 0 || rightHalf == 0 {
		t.Errorf("the lid has ink on only one side of the bin (%d left, %d right); it should be two flaps",
			leftHalf, rightHalf)
	}
}

// At rest the bin draws its own top edge and the lid draws nothing, so the two
// can never both show, or both be missing.
func TestAShutBinHasItsOwnTopEdgeBack(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	tick(m, 60)

	l := m.Layout
	buf := drawTo(m)
	x := l.BinOriginX(1)
	if got := buf.RuneAt(x, l.BinTop); got != '┌' {
		t.Errorf("a shut bin's top-left corner is %q, want the box's own ┌", got)
	}
	// And nothing of the lid is left hanging above it.
	for y := l.BinTop - int(lidRise) - 1; y < l.BinTop; y++ {
		for cx := x; cx < x+l.BinWidth(1); cx++ {
			r := buf.RuneAt(cx, y)
			if r > canvas.BrailleBlank && r <= canvas.BrailleBlank+0xFF {
				t.Errorf("a shut bin left braille at (%d,%d): %q", cx, y, r)
			}
		}
	}
}

// Clearing the full top row would take the corners and side walls with it, so
// the bin would read as damaged with a tick floating over it.
func TestAnOpenBinKeepsItsWalls(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	launchInto(t, m, 2)
	tick(m, 16)

	if !m.BinFill.Ajar(2) {
		t.Skip("the lid never opened")
	}

	l := m.Layout
	buf := drawTo(m)
	x, w := l.BinOriginX(2), l.BinWidth(2)

	if got := buf.RuneAt(x, l.BinTop); got != '│' {
		t.Errorf("an open bin's left wall is %q, want │", got)
	}
	if got := buf.RuneAt(x+w-1, l.BinTop); got != '│' {
		t.Errorf("an open bin's right wall is %q, want │", got)
	}
	// And nothing of the old top edge is left between them.
	for cx := x + 1; cx < x+w-1; cx++ {
		if got := buf.RuneAt(cx, l.BinTop); got == '─' {
			t.Errorf("an open bin still has its top edge at column %d", cx)
			break
		}
	}
}

// High enough to read as a lid: three rows keeps it under the rule, like a tick
// mark.
func TestAnOpenLidStandsClearOfTheBin(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	launchInto(t, m, 0)
	tick(m, 16)

	l := m.Layout
	buf := drawTo(m)
	x := l.BinOriginX(0)

	// Count the rows above the bin with lid ink anywhere across its reach. The two
	// flaps' ink moves sideways as they swing, so one column would measure the
	// angle, not the height.
	rows := 0
	for y := l.BinTop - int(lidRise); y < l.BinTop; y++ {
		for cx := 0; cx < l.Width; cx++ {
			if r := buf.RuneAt(cx, y); r > canvas.BrailleBlank && r <= canvas.BrailleBlank+0xFF {
				rows++
				break
			}
		}
	}
	_ = x
	if rows < int(lidRise)-1 {
		t.Errorf("an open lid stands %d rows clear, want about %v", rows, lidRise)
	}
}

// The show numbers its bins 01 through 05. Centred on the bin, not at a fixed
// inset: Layout gives the remainder width to the leftmost bin, so a fixed inset
// puts each number at a different distance from its box.
func TestBinsAreNumberedTheWayTheShowNumbersThem(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	buf := drawTo(m)
	l := m.Layout
	row := []rune(buf.Row(l.BinTop + 1))

	for i := 0; i < BinCount; i++ {
		want := binLabel(i + 1)
		x := binTextX(l.BinCenterX[i], len(want), l.BinOriginX(i), l.BinWidth(i))
		if got := string(row[x : x+len(want)]); got != want {
			t.Errorf("bin %d is labelled %q, want %q", i+1, got, want)
		}

		// And that position is the middle of the box, to within the rounding an
		// even-width label on an odd-width bin cannot avoid.
		lo, hi := l.BinOriginX(i)+1, l.BinOriginX(i)+l.BinWidth(i)-1
		gapL, gapR := x-lo, hi-(x+len(want))
		if d := gapL - gapR; d < -1 || d > 1 {
			t.Errorf("bin %d's number has %d columns to its left and %d to its right",
				i+1, gapL, gapR)
		}
	}
}

// Full in the terminal's own colour and empty by being hollow, asked of what was
// painted, not of what BinFill computed.
func TestAFullBinIsSolidAndAnEmptyOneIsHollow(t *testing.T) {
	m := ready()
	m.Resize(120, 34)

	m.Room.mu.Lock()
	m.Grid.Bins[0].Refined = 0
	m.Grid.Bins[4].Refined = m.Grid.Bins[4].Quota
	m.Room.mu.Unlock()
	tick(m, 600)

	buf := drawTo(m)
	l := m.Layout
	y := l.BinTop + 4

	count := func(i int) int {
		n := 0
		for x := l.BinOriginX(i) + 1; x < l.BinOriginX(i)+l.BinWidth(i)-1; x++ {
			if buf.AttrAt(x, y).BG == m.Theme.BinFull.BG {
				n++
			}
		}
		return n
	}

	if n := count(0); n != 0 {
		t.Errorf("an empty bin paints %d filled cells; hollow means nothing drawn", n)
	}
	// All but the last, which the easing spring may still be a fraction short of,
	// making it the partial-block leading edge instead of solid fill.
	if n, want := count(4), l.BinWidth(4)-3; n < want {
		t.Errorf("a full bin paints %d cells, want at least %d", n, want)
	}

	// The fill is the terminal's own cyan, not the file bar's shadow, which looks
	// like an empty bin.
	if m.Theme.BinFull.BG != m.Theme.Primary {
		t.Errorf("a full bin fills with %v, want the primary %v",
			m.Theme.BinFull.BG, m.Theme.Primary)
	}

	// And the readout on it is inverted, not one bright thing on another. Only the
	// characters over the fill: a cell past the leading edge is the bin's own
	// colour on nothing.
	row := []rune(buf.Row(y))
	seen := 0
	for x := l.BinOriginX(4) + 1; x < l.BinOriginX(4)+l.BinWidth(4)-1; x++ {
		a := buf.AttrAt(x, y)
		if row[x] == ' ' || a.BG != m.Theme.Primary {
			continue
		}
		seen++
		if a.FG != m.Theme.Ink {
			t.Errorf("the readout at column %d is %v on the fill, want the ink %v",
				x, a.FG, m.Theme.Ink)
			break
		}
	}
	if seen == 0 {
		t.Error("a full bin has no readout standing on its fill at all")
	}
}

// The mark is at the right of the file bar and gives way first: the percentage
// is information and the mark is a logo.
func TestTheLumonMarkGivesWayBeforeThePercentage(t *testing.T) {
	// The full bar, not one row of it: a tall terminal draws the mark as the
	// intro's globe across five rows of braille with the words centred against it.
	bar := func(m *Model) string {
		buf := drawTo(m)
		var s string
		for y := 0; y < m.Layout.HeaderH; y++ {
			s += buf.Row(y) + "\n"
		}
		return s
	}

	m := ready()
	m.Resize(140, 34)
	drawTo(m)
	if m.header.pct.empty() {
		t.Errorf("a wide header does not carry the percentage:\n%s", bar(m))
	}
	if m.header.mark.empty() {
		t.Errorf("a wide header carries no mark at all:\n%s", bar(m))
	}

	m.Resize(46, 24)
	drawTo(m)
	if !m.header.mark.empty() && m.header.pct.empty() {
		t.Errorf("a narrow header kept the mark and dropped the percentage:\n%s", bar(m))
	}
}

// The bar is lanes side by side, and a four-row completion takes most of the
// width, so fitting is asserted.
func TestTheBarsLanesNeverOverlap(t *testing.T) {
	m := ready()
	for _, w := range []int{160, 140, 120, 110, 100, 90, 80, 70, 60, 46} {
		m.Resize(w, 40)
		drawTo(m)
		h := m.header

		lanes := []struct {
			name string
			b    band
		}{{"name", h.name}, {"ticks", h.ticks}, {"percentage", h.pct}, {"mark", h.mark}}
		for i := range lanes {
			for j := i + 1; j < len(lanes); j++ {
				if lanes[i].b.overlaps(lanes[j].b) {
					t.Errorf("at %d columns the %s (%v) runs through the %s (%v)",
						w, lanes[i].name, lanes[i].b, lanes[j].name, lanes[j].b)
				}
			}
			if b := lanes[i].b; !b.empty() && (b[0] < 1 || b[1] > w-1) {
				t.Errorf("at %d columns the %s (%v) crosses the bar's own border",
					w, lanes[i].name, b)
			}
		}

		// And the percentage is there at every width wide enough to show anything.
		if h.pct.empty() {
			t.Errorf("a %d-column bar carries no completion at all", w)
		}
	}
}

// The line is a caption, glanced at: which file you are on and how far through.
// Set five rows tall it spends a third of the bar and most of the width saying
// that slowly. The show's own bar is one line.
func TestTheCompletionIsOneRowOfText(t *testing.T) {
	m := ready()
	for _, w := range []int{200, 160, 120, 100, 80, 64} {
		m.Resize(w, 40)
		buf := drawTo(m)

		want := strconv.Itoa(m.File.Percent(m.Grid)) + "% Complete"
		if m.header.pct.empty() {
			t.Fatalf("a %d-column bar carries no completion", w)
		}
		if got := m.header.pct[1] - m.header.pct[0]; got != len(want) {
			t.Errorf("at %d columns the completion took %d columns for %d characters",
				w, got, len(want))
		}

		// On the middle row and nowhere else: the rows either side of it belong to the
		// mark and the progress lane.
		mid := m.Layout.HeaderH / 2
		if row := buf.Row(mid); !strings.Contains(row, want) {
			t.Errorf("the %d-column bar does not carry %q on its middle row:\n%q",
				w, want, row)
		}
		for y := 1; y < m.Layout.HeaderH-1; y++ {
			if y == mid {
				continue
			}
			row := []rune(buf.Row(y))
			for x := m.header.pct[0]; x < m.header.pct[1] && x < len(row); x++ {
				if row[x] != ' ' {
					t.Fatalf("at %d columns something is drawn above or below the "+
						"completion at (%d,%d): %q", w, x, y, row[x])
				}
			}
		}
	}
}

// The globe is the mark the boot sequence draws, not a second drawing, so the
// terminal opens on a logo and keeps it in the corner.
func TestTheTallBarCarriesTheIntroMark(t *testing.T) {
	m := ready()
	m.Resize(140, 40)
	if m.Layout.HeaderH < HeaderTall {
		t.Fatalf("a 140x40 terminal took a %d-row header", m.Layout.HeaderH)
	}

	// Centred in the bar's height, not pinned under its top border: the mark is
	// five rows and the bar is seven or ten.
	buf := drawTo(m)
	top := (m.Layout.HeaderH - len(SmallLogo)) / 2
	for i, row := range SmallLogo {
		want := strings.TrimRight(strings.ReplaceAll(row, "⠀", " "), " ")
		if want == "" {
			continue
		}
		if got := buf.Row(top + i); !strings.Contains(got, want) {
			t.Errorf("row %d of the bar does not carry row %d of the mark:\n got %q\nwant %q",
				top+i, i, got, want)
		}
	}

	// And it has not eaten the bar's own border doing it.
	for _, y := range []int{0, m.Layout.HeaderH - 1} {
		if row := buf.Row(y); !strings.Contains(row, "─") {
			t.Errorf("the mark rubbed out the bar's border on row %d: %q", y, row)
		}
	}
}

// The bar is lanes side by side, so the completion is never under the
// progress.
func TestTheCompletionIsNeverUnderTheProgress(t *testing.T) {
	m := ready()
	m.Resize(120, 40)

	for _, at := range []float64{0, 0.13, 0.5, 0.87, 1} {
		m.File.fill.Set(at)
		buf := drawTo(m)

		if m.header.pct.empty() {
			t.Fatalf("at %.0f%% the bar carries no completion", at*100)
		}
		if m.header.ticks.overlaps(m.header.pct) {
			t.Errorf("at %.0f%% the progress lane %v runs through the completion %v",
				at*100, m.header.ticks, m.header.pct)
		}

		// And nothing of the lane is in those columns, on any row, since the setting
		// is four rows tall.
		for y := 1; y < m.Layout.HeaderH-1; y++ {
			row := []rune(buf.Row(y))
			for x := m.header.pct[0]; x < m.header.pct[1] && x < len(row); x++ {
				if row[x] == fileTickGap {
					t.Errorf("at %.0f%% a division sits in the completion at (%d,%d)",
						at*100, x, y)
				}
			}
		}
	}
}

// The progress moves a division at a time.
func TestTheProgressFillsInDivisions(t *testing.T) {
	m := ready()

	// Wide, because the lane is what is left after the completion is set large, and
	// at 120 columns it gets nothing (the bar then draws no divisions instead of a
	// broken few). This tests how they fill, so it runs where there are some.
	m.Resize(160, 40)
	mid := m.Layout.HeaderH / 2

	// Counted inside the lane the bar says it drew, so a █ elsewhere on the row
	// is not counted as progress.
	count := func() int {
		buf := drawTo(m)
		row := []rune(buf.Row(mid))
		n := 0
		for x := m.header.ticks[0]; x < m.header.ticks[1] && x < len(row); x++ {
			if row[x] == fileTick {
				n++
			}
		}
		return n
	}

	m.File.fill.Set(0)
	empty := count()
	m.File.fill.Set(0.5)
	half := count()
	m.File.fill.Set(1)
	full := count()

	if !(empty < half && half < full) {
		t.Errorf("the bar lit %d, %d and %d divisions at nought, half and full",
			empty, half, full)
	}
	if empty != 0 {
		t.Errorf("an untouched file already shows %d lit divisions", empty)
	}
	if full < fileTickMinRun {
		t.Errorf("a finished file shows only %d divisions", full)
	}
}

// binNumberX is where a bin's two-character number was drawn. Layout gives the
// remainder width to the leftmost bin, so this is the centred position, not a
// fixed inset from the wall.
func binNumberX(l *Layout, i int) int {
	return binTextX(l.BinCenterX[i], len(binLabel(i+1)), l.BinOriginX(i), l.BinWidth(i))
}

// indexRunes finds a run of runes inside another, or -1.
func indexRunes(hay, needle []rune) int {
	for i := 0; i+len(needle) <= len(hay); i++ {
		ok := true
		for j := range needle {
			if hay[i+j] != needle[j] {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

// The show's bins read a percentage and nothing else.
func TestABinsReadoutIsThePercentageAlone(t *testing.T) {
	m := ready()
	m.Resize(160, 40)

	m.Room.mu.Lock()
	bin := m.Grid.Bins[0]
	bin.Refined = bin.Quota / 2
	want := strconv.Itoa(bin.PercentInt()) + "%"
	m.Room.mu.Unlock()
	tick(m, 600)

	buf := drawTo(m)
	l := m.Layout
	x, w := l.BinOriginX(0), l.BinWidth(0)
	bar := string([]rune(buf.Row(l.BinTop + 4))[x+1 : x+w-1])
	if got := strings.Fields(bar); len(got) == 0 || got[0] != want || strings.Contains(bar, "/") {
		t.Errorf("bin 01 reads %q, want %q alone", bar, want)
	}
}

// A fill grows from the left, so a centred readout would spend the first half
// of a bin's life on bare box, the second on cyan, and cross the fill's moving
// edge where the eye is. Pinned left, it is on the fill from the first cluster.
func TestABinsReadoutSitsAtTheLeftOfItsBar(t *testing.T) {
	m := ready()
	m.Resize(120, 40)

	m.Room.mu.Lock()
	m.Grid.Bins[0].Refined = m.Grid.Bins[0].Quota / 2
	m.Room.mu.Unlock()
	tick(m, 600)

	buf := drawTo(m)
	l := m.Layout
	y := l.BinTop + 4

	// The first character of the readout is against the bin's own left wall.
	x, w := l.BinOriginX(0), l.BinWidth(0)
	if got := buf.RuneAt(x+1, y); got == ' ' {
		t.Errorf("the readout does not start at the bin's left wall; (%d,%d) is blank", x+1, y)
	}

	// And the far end of the bar is blank; centred would leave a gap at both ends.
	row := []rune(buf.Row(y))
	if got := row[x+w-2]; got != ' ' {
		t.Errorf("the last column of the bar carries %q, so the readout is not left-aligned:\n%q",
			got, string(row[x:x+w]))
	}

	// It stands on the fill: at half full the first cell of the readout has the
	// fill's own background.
	if got := buf.AttrAt(x+1, y).BG; got != m.Theme.Primary {
		t.Errorf("the readout's first cell is on %v, not on the fill's %v", got, m.Theme.Primary)
	}
}

// The hint is what tells a new refiner what the keys do. Sharing a row with
// the readout and banner it would be truncated to fit, and in Theme.Footer it
// would be lost among the muted text.
func TestTheHintIsOnItsOwnRowAndCanBeRead(t *testing.T) {
	m := ready()
	l := m.Layout
	buf := drawTo(m)

	readout, hint := buf.Row(l.FooterY()), buf.Row(l.HintY())

	if !strings.Contains(readout, "0x") {
		t.Errorf("the readout is not on the row above the hint:\n%q", readout)
	}
	if strings.Contains(hint, "0x") {
		t.Errorf("the hint row still carries the readout:\n%q", hint)
	}
	if !strings.Contains(hint, "move") {
		t.Fatalf("the hint is not on the last row:\n%q", hint)
	}

	// In the palette's most prominent colour, not the one that recedes. Read at the
	// first inked column, since the line is centred and column 1 is margin.
	at := buf.AttrAt(strings.IndexRune(hint, '↑'), l.HintY())
	if at.FG != m.Theme.Peak {
		t.Errorf("the hint is drawn %v, want the palette's Peak %v", at.FG, m.Theme.Peak)
	}
	if at.FG == m.Theme.Footer.FG {
		t.Errorf("the hint is still drawn in Footer, %v", at.FG)
	}

	// And it has the width for every binding, up to the last one in the table.
	if !strings.Contains(hint, "quit") {
		t.Errorf("the hint is still being truncated:\n%q", hint)
	}

	// Centred on the terminal, the axis drawAddress puts the readout on at
	// (Width-n)/2. Measured against the width, since the banner shares the row
	// above and sits right of the readout.
	if got, want := textCentre(hint), l.Width/2; got < want-1 || got > want+1 {
		t.Errorf("the hint is centred on column %d, want the terminal's own %d",
			got, want)
	}
}

// textCentre is the middle column of whatever is written in a row.
func textCentre(row string) int {
	first := strings.IndexFunc(row, func(r rune) bool { return r != ' ' })
	last := strings.LastIndexFunc(row, func(r rune) bool { return r != ' ' })
	if first < 0 {
		return -1
	}
	return (len([]rune(row[:first])) + len([]rune(row[:last+1]))) / 2
}
