package lumon

import (
	"github.com/terminaltrove/lumon/internal/canvas"
	"github.com/terminaltrove/lumon/internal/theme"
	"math"
	"strings"
	"testing"
)

// twoInARoom seats two refiners at one board, both on usable terminals.
func twoInARoom(t *testing.T) (*Room, *Model, *Model) {
	t.Helper()
	room := NewRoom(3)
	a, b := JoinRoom(room, 1), JoinRoom(room, 2)
	if a == nil || b == nil {
		t.Fatal("could not seat two refiners")
	}
	a.Resize(120, 40)
	b.Resize(120, 40)
	return room, a, b
}

func TestEachRefinerGetsTheirOwnDesk(t *testing.T) {
	_, a, b := twoInARoom(t)

	if a.Desk == b.Desk {
		t.Fatal("both refiners were given the same desk")
	}
	if a.Desk.Seat == b.Desk.Seat {
		t.Errorf("both refiners are at seat %d", a.Desk.Seat)
	}
	if a.deskColour(a.Desk) == b.deskColour(b.Desk) {
		t.Error("both refiners are drawn in the same colour")
	}
}

// Otherwise nameplates shuffle along when somebody leaves the middle of the
// room.
func TestASeatKeepsItsPlace(t *testing.T) {
	room := NewRoom(3)
	a, b, c := JoinRoom(room, 1), JoinRoom(room, 2), JoinRoom(room, 3)
	if a == nil || b == nil || c == nil {
		t.Fatal("could not seat three")
	}
	seatC := c.Desk.Seat

	b.Leave()

	if got := c.Desk.Seat; got != seatC {
		t.Errorf("a refiner moved from seat %d to %d when somebody else left", seatC, got)
	}
	// The vacated seat is the one reused.
	d := JoinRoom(room, 4)
	if d == nil {
		t.Fatal("could not take the freed seat")
	}
	if got := d.Desk.Seat; got != 1 {
		t.Errorf("the new arrival took seat %d, want the vacated 1", got)
	}
}

func TestYouSeeTheOtherRefinersCursor(t *testing.T) {
	room, a, b := twoInARoom(t)

	// Put B somewhere A can see, well away from A's own cursor.
	l := a.Layout
	bx, by := l.ViewportX+l.ViewportW-2, l.ViewportY+1
	a.CursorX, a.CursorY = l.ViewportX+1, l.ViewportY+3
	b.CursorX, b.CursorY = bx, by

	tick(b, 1) // B publishes to their desk
	tick(a, 1)

	if got := room.Roster(); len(got) != 2 {
		t.Fatalf("%d desks in the roster, want 2", len(got))
	}
	if d := a.deskAt(bx, by); d != b.Desk {
		t.Fatalf("A does not see B's cursor at (%d,%d)", bx, by)
	}

	buf := drawTo(a)
	sx, sy := l.ScreenOf(bx, by)
	if got := buf.AttrAt(sx, sy).BG; got != a.deskColour(b.Desk) {
		t.Errorf("B's cell is drawn %v, want B's colour %v", got, a.deskColour(b.Desk))
	}
}

// With four people the question is which cursor is mine, so in company every
// cursor takes its seat's colour and no two look alike. Alone it is still the
// plate theme's cursorPlate describes (TestAloneTheCursorIsStillTheShowsPlate).
func TestYourOwnCursorIsDrawnAsYourOwnSeat(t *testing.T) {
	_, a, b := twoInARoom(t)
	tick(b, 1)
	tick(a, 1)

	buf := drawTo(a)
	sx, sy := a.Layout.ScreenOf(a.CursorX, a.CursorY)

	want := a.Theme.DeskCursor(a.Desk.Seat)
	if got := buf.AttrAt(sx, sy); got != want {
		t.Errorf("own cursor is drawn %v, want this refiner's own seat %v", got, want)
	}
	if other := a.Theme.DeskCursor(b.Desk.Seat); want == other {
		t.Errorf("both refiners' cursors are drawn %v; there is nothing to tell "+
			"them apart", want)
	}
}

// The show's cursor is a faint plate with the digit over it brightening (see
// theme's cursorPlate); a bright block would outshout corruption.
func TestAloneTheCursorIsStillTheShowsPlate(t *testing.T) {
	m := ready()
	tick(m, 1)

	buf := drawTo(m)
	sx, sy := m.Layout.ScreenOf(m.CursorX, m.CursorY)
	if got := buf.AttrAt(sx, sy); got != theme.AttrCursor {
		t.Errorf("alone, the cursor is drawn %v, want the terminal's own plate %v",
			got, theme.AttrCursor)
	}
}

func TestAloneTheFooterKeepsTheBanner(t *testing.T) {
	m := ready()
	tick(m, 1)

	buf := drawTo(m)
	footer := buf.Row(m.Layout.FooterY())

	if !strings.Contains(footer, "LUMON") {
		t.Errorf("alone, the footer lost the banner: %q", footer)
	}
}

func TestTheRosterNamesWhoIsInTheRoom(t *testing.T) {
	_, a, b := twoInARoom(t)
	a.Desk.Name = "MARK S."
	b.Desk.Name = "HELLY R."
	tick(a, 1)

	buf := drawTo(a)
	footer := buf.Row(a.Layout.FooterY())

	for _, want := range []string{"MARK S.", "HELLY R."} {
		if !strings.Contains(footer, want) {
			t.Errorf("the roster does not mention %q: %q", want, footer)
		}
	}
}

func TestRefiningCreditsTheRefiner(t *testing.T) {
	_, a, _ := twoInARoom(t)

	grp := a.FirstActive()
	for i := 0; grp == nil && i < 900; i++ {
		tick(a, 1)
		grp = a.FirstActive()
	}
	if grp == nil {
		t.Skip("no cluster woke")
	}
	before := a.Desk.Credit

	id := grp.NumberIDs[0]
	a.CursorX, a.CursorY = id%GridSize, id/GridSize
	a.Update(key("enter"))

	if got := a.Desk.Credit; got != before+1 {
		t.Errorf("the refiner's tally is %d, want %d", got, before+1)
	}
}

// The board still counts the cluster once. "At the same time" is a grace
// period, because the lock means one of them is always strictly second.
func TestBothRefinersAreCreditedForOneCluster(t *testing.T) {
	room, a, b := twoInARoom(t)

	grp := a.FirstActive()
	for i := 0; grp == nil && i < 900; i++ {
		tick(a, 1)
		grp = a.FirstActive()
	}
	if grp == nil {
		t.Skip("no cluster woke")
	}
	id := grp.NumberIDs[0]
	gx, gy := id%GridSize, id/GridSize
	binBefore := room.Grid.Bins[a.SelectedBin].Refined

	a.CursorX, a.CursorY = gx, gy
	b.CursorX, b.CursorY = gx, gy
	a.Update(key("enter"))
	b.Update(key("enter")) // a moment later, on a cluster already gone

	if got := a.Desk.Credit; got != 1 {
		t.Errorf("the first refiner has %d, want 1", got)
	}
	if got := b.Desk.Credit; got != 1 {
		t.Errorf("the second refiner has %d, want 1, the reach should still count", got)
	}

	// But the board counts the cluster once.
	for i := 0; i < 240; i++ {
		tick(a, 1)
	}
	if got := room.Grid.Bins[a.SelectedBin].Refined; got != binBefore+1 {
		t.Errorf("the bin took %d clusters, want 1", got-binBefore)
	}
}

// Otherwise a cluster refined long ago would keep paying out.
func TestTheCoRefineWindowCloses(t *testing.T) {
	_, a, b := twoInARoom(t)

	grp := a.FirstActive()
	for i := 0; grp == nil && i < 900; i++ {
		tick(a, 1)
		grp = a.FirstActive()
	}
	if grp == nil {
		t.Skip("no cluster woke")
	}
	id := grp.NumberIDs[0]
	gx, gy := id%GridSize, id/GridSize

	a.CursorX, a.CursorY = gx, gy
	a.Update(key("enter"))

	tick(a, 60) // a full second: well past the window

	b.CursorX, b.CursorY = gx, gy
	b.Update(key("enter"))

	if got := b.Desk.Credit; got != 0 {
		t.Errorf("a refiner was credited %d for a cluster taken a second earlier", got)
	}
}

// Four nameplates and the controls hint do not both fit on a narrow terminal,
// so the plates shorten and the hint gives way.
func TestTheRosterSurvivesANarrowTerminal(t *testing.T) {
	room := NewRoom(3)
	var views []*Model
	for i := 0; i < MaxDesks; i++ {
		m := JoinRoom(room, int64(i))
		if m == nil {
			t.Fatalf("could not seat refiner %d", i+1)
		}
		m.Desk.Name = []string{"MARK S.", "HELLY R.", "IRVING B.", "DYLAN G."}[i]
		views = append(views, m)
	}

	for _, width := range []int{160, 120, 100, 80} {
		m := views[0]
		m.Resize(width, 30)
		tick(m, 1)

		footer := drawTo(m).Row(m.Layout.FooterY())
		if len([]rune(strings.TrimRight(footer, " "))) > width {
			t.Errorf("at %d columns the footer overflows", width)
		}
		// Every refiner is represented, in full or by initial.
		for _, d := range room.Roster() {
			if strings.Contains(footer, d.Name) || strings.Contains(footer, initials(d.Name)) {
				continue
			}
			t.Errorf("at %d columns %q is missing from the footer: %q", width, d.Name, footer)
		}
	}
}

func TestOneBoardTwoTerminalSizes(t *testing.T) {
	room := NewRoom(3)
	small, wide := JoinRoom(room, 1), JoinRoom(room, 2)
	small.Resize(80, 24)
	wide.Resize(200, 50)

	tick(small, 300)
	tick(wide, 300)

	sBuf, wBuf := drawTo(small), drawTo(wide)
	sw, _ := sBuf.Size()
	ww, _ := wBuf.Size()
	if sw != 80 || ww != 200 {
		t.Fatalf("buffers are %d and %d columns", sw, ww)
	}

	// Same file, same bins: the things the board owns.
	if small.FileName != wide.FileName {
		t.Error("different file names")
	}
	for i := range room.Grid.Bins {
		if small.Grid.Bins[i].Refined != wide.Grid.Bins[i].Refined {
			t.Errorf("bin %d disagrees between the two terminals", i+1)
		}
	}
}

func TestYouSeeTheOtherRefinersBulge(t *testing.T) {
	room := NewRoom(3)
	room.SetDesks(2)
	a, b := JoinRoom(room, 1), JoinRoom(room, 2)
	a.Resize(140, 40)
	b.Resize(140, 40)

	l := a.Layout
	// B sits well away from A, so any movement near B is B's lens alone.
	a.CursorX, a.CursorY = l.ViewportX+1, l.ViewportY+1
	b.CursorX, b.CursorY = l.ViewportX+l.ViewportW-3, l.ViewportY+l.ViewportH-2
	a.focusLens()
	b.focusLens()
	tick(b, 1)
	tick(a, 30)

	// A cell one to the side of B's cursor must be pushed away from it.
	near := GridIndex(b.CursorX-1, b.CursorY)
	sx, _ := l.ScreenOf(b.CursorX-1, b.CursorY)
	ax, _ := a.PlainAnchor(near)

	if ax == sx {
		t.Errorf("the cell beside another refiner's cursor is drawn at %d, exactly where "+
			"layout puts it, their lens is not bulging this view", ax)
	}
	// Pushed outward, further from their focus, which is to the right.
	if ax > sx {
		t.Errorf("the cell was pulled toward the other refiner rather than pushed away")
	}
}

func TestTheBulgeIsBoundedHoweverManyLenses(t *testing.T) {
	room := NewRoom(3)
	room.SetDesks(4)
	var views []*Model
	for i := 0; i < 4; i++ {
		m := JoinRoom(room, int64(i+1))
		m.Resize(140, 40)
		views = append(views, m)
	}

	l := views[0].Layout
	gx, gy := l.ViewportX+l.ViewportW/2, l.ViewportY+l.ViewportH/2
	for _, m := range views {
		m.CursorX, m.CursorY = gx, gy
		m.focusLens()
	}
	tick(views[0], 30)

	// The lenses are all on one cell, the worst pile-up there is.
	worst := 0.0
	for y := 0; y < l.Height; y++ {
		for x := 0; x < l.Width; x++ {
			bx, by := views[0].Bulge(float64(x), float64(y))
			if d := math.Hypot(bx-float64(x), (by-float64(y))*RowAspect); d > worst {
				worst = d
			}
		}
	}
	if worst > l.MaxBulge()+0.01 {
		t.Errorf("four stacked lenses moved a cell %v visual columns, past the %v ceiling",
			worst, l.MaxBulge())
	}
	// It must be doing something, or the test passes for the wrong reason: four
	// lenses on one cell push further than one lens does.
	if worst < l.LensPush()+0.01 {
		t.Errorf("four stacked lenses only managed %v visual columns, no more than one lens's %v",
			worst, l.LensPush())
	}
}

// The cap is for crowds; a lone refiner is pushed exactly as hard as one lens
// says. The lens tests call Lens.Displace, which the renderer does not use, so
// this compares the two directly. An unconditional tanh would fail it.
func TestOneLensPushesExactlyAsHardAsItAlwaysDid(t *testing.T) {
	m := ready()
	m.Resize(140, 40)
	l := m.Layout
	fx, fy := float64(l.Width/2), float64(l.Height/2)
	m.Lens.Warp(fx, fy)

	peak := 0.0
	for y := 0; y < l.Height; y++ {
		for x := 0; x < l.Width; x++ {
			// What the lens alone says, and what the renderer applies.
			wx, wy := m.Lens.Displace(float64(x), float64(y))
			bx, by := m.Bulge(float64(x), float64(y))

			if math.Hypot(bx-wx, (by-wy)*RowAspect) > 1e-9 {
				t.Fatalf("at (%d,%d) the renderer put the cell at (%v,%v) where the lens says (%v,%v)",
					x, y, bx, by, wx, wy)
			}
			if d := math.Hypot(bx-float64(x), (by-float64(y))*RowAspect); d > peak {
				peak = d
			}
		}
	}
	// bulge peaks at l.LensPush() by construction, half a radius out.
	if peak < l.LensPush()-0.05 {
		t.Errorf("the strongest push one lens managed was %v visual columns, want the full %v",
			peak, l.LensPush())
	}
}

func TestABinIsColouredForItsRefiner(t *testing.T) {
	_, a, b := twoInARoom(t)
	a.SelectedBin = 0
	b.SelectedBin = 2
	tick(a, 1)
	tick(b, 1)

	buf := drawTo(a)
	l := a.Layout

	// B's bin has B's colour on A's screen.
	got := buf.AttrAt(binNumberX(l, 2), l.BinTop+1).FG
	if got != a.deskColour(b.Desk) {
		t.Errorf("bin 3 is drawn %v, want the refiner feeding it %v", got, a.deskColour(b.Desk))
	}

	// A bin nobody is on stays plain.
	if got := buf.AttrAt(binNumberX(l, 4), l.BinTop+1); got != theme.AttrBinOff {
		t.Errorf("an unattended bin is drawn %v, want %v", got, theme.AttrBinOff)
	}

	// Your own has your colour too, by the same rule as everybody else's, and is
	// it alone is bold so you can pick it out at a glance.
	own := buf.AttrAt(binNumberX(l, 0), l.BinTop+1)
	if own.FG != a.deskColour(a.Desk) {
		t.Errorf("your own bin is drawn %v, want your colour %v", own.FG, a.deskColour(a.Desk))
	}
	if !own.Bold {
		t.Error("your own bin is not bold, so there is nothing marking it as yours")
	}
	if buf.AttrAt(binNumberX(l, 2), l.BinTop+1).Bold {
		t.Error("somebody else's bin is bold, which is what marks your own")
	}
}

func TestASharedBinShowsBothRefiners(t *testing.T) {
	_, a, b := twoInARoom(t)
	a.SelectedBin = 1
	b.SelectedBin = 1
	tick(a, 1)
	tick(b, 1)

	buf := drawTo(a)
	l := a.Layout

	seen := map[canvas.Color]bool{}
	for x := l.BinOriginX(1); x < l.BinOriginX(1)+l.BinWidth(1); x++ {
		seen[buf.AttrAt(x, l.BinTop).FG] = true
	}
	if !seen[a.deskColour(b.Desk)] {
		t.Errorf("a shared bin does not show the other refiner's colour")
	}
	if !seen[a.deskColour(a.Desk)] {
		t.Errorf("a shared bin does not show your own colour")
	}
}

func TestAloneTheBinsAreUnchanged(t *testing.T) {
	m := ready()
	m.SelectedBin = 3
	tick(m, 1)

	buf := drawTo(m)
	l := m.Layout
	if got := buf.AttrAt(binNumberX(l, 3), l.BinTop+1); got != theme.AttrBinOn {
		t.Errorf("alone, the selected bin is %v, want %v", got, theme.AttrBinOn)
	}
	if got := buf.AttrAt(binNumberX(l, 0), l.BinTop+1); got != theme.AttrBinOff {
		t.Errorf("alone, an unselected bin is %v, want %v", got, theme.AttrBinOff)
	}
}

// HitTest consults three tiers: the glyph boxes the renderer placed, the index
// it built while drawing, and finally Layout. The first two were computed
// against the previous frame, so both go stale when the board moves and both
// must be guarded. ready() has no woken cluster, so this wakes one to get boxes.
func TestAStaleGlyphBoxDoesNotAnswerAClick(t *testing.T) {
	m := ready()
	grownCluster(t, m)
	drawTo(m)
	if len(m.boxes) == 0 {
		t.Skip("nothing was drawn large")
	}

	// A point on the ink of an enlarged digit.
	b := m.boxes[0]
	px, py, found := 0, 0, false
	for row := 0; row < b.cov.H && !found; row++ {
		for col := 0; col < b.cov.W; col++ {
			if b.cov.At(col, row) > 0 {
				px, py, found = b.x+col, b.y+row, true
				break
			}
		}
	}
	if !found {
		t.Skip("the box has no ink")
	}

	// Scroll without redrawing, as Update does between frames.
	l := m.Layout
	m.Layout.ScrollTo(l.ViewportX+10, l.ViewportY+5)

	hx, hy, _ := m.HitTest(px, py)
	wx, wy, _ := m.Layout.CellAt(px, py)
	if hx != wx || hy != wy {
		t.Errorf("after a scroll a stale glyph box answered (%d,%d); the live layout says (%d,%d)",
			hx, hy, wx, wy)
	}
}

// The room can swap the grid with the viewport and terminal size unchanged, so
// the stamp has the board's epoch as well as the screen's dimensions. The grid
// is drawn under the finished-file modal, so the index is rebuilt throughout
// and the epoch is all that stops a click landing on the wrong cell.
func TestTheIndexDoesNotSurviveANewFile(t *testing.T) {
	m := ready()
	drawTo(m)

	x, y := overTheGrid(m)
	if _, _, ok := m.HitTest(x, y); !ok {
		t.Skip("nothing under the pointer to begin with")
	}

	completeTheFile(m)
	tick(m, int(FileHoldSeconds*60)+10)

	if m.anchorAt == m.stampNow() && len(m.anchors) > 0 {
		t.Error("the index from the last file still claims to describe this one")
	}
	// It answers from the live layout instead.
	hx, hy, _ := m.HitTest(x, y)
	wx, wy, _ := m.Layout.CellAt(x, y)
	if hx != wx || hy != wy {
		t.Errorf("a stale index answered (%d,%d); the live layout says (%d,%d)", hx, hy, wx, wy)
	}
}

// Everybody else's lens is aimed at a grid cell, so a scroll has to re-project
// it. Otherwise their bulge springs a whole column on every arrow key at the
// viewport edge.
func TestScrollingDoesNotFlingSomebodyElsesLens(t *testing.T) {
	_, a, b := twoInARoom(t)

	b.CursorX, b.CursorY = 20, 12
	tick(b, 1)
	tick(a, 30)

	l := a.Layout
	before := a.remoteLens[b.Desk.Seat].Focus()

	// Scroll A's viewport one column, as walking off the edge does.
	a.CursorX = l.ViewportX + l.ViewportW
	a.scrollToCursor()

	after := a.remoteLens[b.Desk.Seat].Focus()

	// B has not moved, so their lens sits on the same cell, now one column further
	// left on A's screen.
	if got := before.X - after.X; got < float64(CellWidth)-0.5 || got > float64(CellWidth)+0.5 {
		t.Errorf("after a one-cell scroll the other refiner's lens moved %v columns, want %v",
			got, CellWidth)
	}
}

// The lens list is read once per visible cell, so it must not allocate or
// reorder: floating-point addition is not associative, and a map's order
// changes every frame, which would jitter a still digit between two positions.
func TestTheLensListIsStableAndDoesNotAllocate(t *testing.T) {
	_, a, _ := twoInARoom(t)
	tick(a, 2)

	first := append([]*Lens(nil), a.lenses()...)
	for i := 0; i < 50; i++ {
		got := a.lenses()
		if len(got) != len(first) {
			t.Fatalf("the lens list changed length from %d to %d", len(first), len(got))
		}
		for j := range got {
			if got[j] != first[j] {
				t.Fatalf("the lens list reordered itself at position %d", j)
			}
		}
	}

	if n := testing.AllocsPerRun(100, func() { _ = a.lenses() }); n > 0 {
		t.Errorf("lenses() allocated %v times per call", n)
	}
}

// A coloured block does not say who; the footer roster is a whole panel away.
// Asserted against the drawn buffer, since the claim is that a name can be read
// off the board; m.plates only says where to look.
func TestEveryRefinerWearsTheirName(t *testing.T) {
	_, a, b := twoInARoom(t)
	l := a.Layout
	a.CursorX, a.CursorY = l.ViewportX+4, l.ViewportY+4
	b.CursorX, b.CursorY = l.ViewportX+11, l.ViewportY+7
	for i := 0; i < 120; i++ {
		tick(b, 1)
		tick(a, 1)
	}

	buf := drawTo(a)
	if len(a.plates) != 2 {
		t.Fatalf("%d nameplates drawn, want one for each of the two refiners: %+v",
			len(a.plates), a.plates)
	}

	seen := map[int]string{}
	for _, p := range a.plates {
		row := []rune(buf.Row(p.Y))
		seen[p.Seat] = strings.TrimSpace(string(row[p.X : p.X+p.W]))

		// In that seat's own colour, the attribute its cursor block uses, so the tag
		// and the marker read as one thing.
		want := a.Theme.DeskCursor(p.Seat)
		if got := buf.AttrAt(p.X, p.Y); got != want {
			t.Errorf("seat %d's tag is drawn %v, want their own seat %v",
				p.Seat, got, want)
		}
	}

	if got := seen[b.Desk.Seat]; got != strings.ToUpper(b.Desk.Name) {
		t.Errorf("the other refiner's tag reads %q, want %q",
			got, strings.ToUpper(b.Desk.Name))
	}
	if got := seen[a.Desk.Seat]; got != "YOU" {
		t.Errorf("your own tag reads %q, want YOU", got)
	}

	// Each hangs over its own cursor.
	for _, p := range a.plates {
		d := a.Desk
		if p.Seat != a.Desk.Seat {
			d = b.Desk
		}
		sx, sy := l.ScreenOf(d.CursorX, d.CursorY)
		if p.Y != sy-1 {
			t.Errorf("seat %d's tag is on row %d, want the row above their cursor, %d",
				p.Seat, p.Y, sy-1)
		}
		if mid := p.X + p.W/2; mid < sx-1 || mid > sx+1 {
			t.Errorf("seat %d's tag is centred on column %d, want their cursor's %d",
				p.Seat, mid, sx)
		}
	}
}

// A tag hangs from the sprung focus of that refiner's lens, a physics.Seek at
// lensFrequency (critically damped, no overshoot), which keeps it on that
// lens's fisheye. After the cursor moves the tag is strictly between where it
// was and where it is going for several frames, then arrives exactly.
func TestTheNameplateEasesAfterTheCursor(t *testing.T) {
	_, a, b := twoInARoom(t)
	l := a.Layout
	b.CursorX, b.CursorY = l.ViewportX+2, l.ViewportY+4
	for i := 0; i < 120; i++ {
		tick(b, 1)
		tick(a, 1)
	}

	plateX := func() int {
		drawTo(a)
		for _, p := range a.plates {
			if p.Seat == b.Desk.Seat {
				return p.X + p.W/2
			}
		}
		t.Fatal("the other refiner has no nameplate")
		return 0
	}

	from := plateX()
	b.CursorX = l.ViewportX + 14
	tick(b, 1)
	want, _ := l.ScreenOf(b.CursorX, b.CursorY)
	if want <= from+8 {
		t.Fatalf("the cursor moved from column %d to %d, which is not far enough "+
			"to tell a glide from a jump", from, want)
	}

	between := 0
	for i := 0; i < 60; i++ {
		tick(a, 1)
		switch got := plateX(); {
		case got > from && got < want:
			between++
		case got == want:
			if between < 4 {
				t.Fatalf("the tag reached its new place after %d frames in between; "+
					"it is jumping rather than easing", between)
			}
			return
		}
	}
	t.Errorf("a second after the cursor moved the tag is at %d, want %d; it is not "+
		"arriving", plateX(), want)
}

// The board moving under somebody's cursor is not that refiner moving.
// reprojectLenses handles this, and it happens on every cursor move at the
// viewport edge.
func TestTheNameplateDoesNotTravelOnAScroll(t *testing.T) {
	_, a, b := twoInARoom(t)
	l := a.Layout
	b.CursorX, b.CursorY = l.ViewportX+6, l.ViewportY+6
	for i := 0; i < 120; i++ {
		tick(b, 1)
		tick(a, 1)
	}

	plateAt := func() (int, int) {
		drawTo(a)
		for _, p := range a.plates {
			if p.Seat == b.Desk.Seat {
				return p.X + p.W/2, p.Y
			}
		}
		t.Fatal("the other refiner has no nameplate")
		return 0, 0
	}

	was, _ := plateAt()
	a.Layout.ScrollTo(l.ViewportX+3, l.ViewportY)
	a.reprojectLenses()

	sx, _ := a.Layout.ScreenOf(b.CursorX, b.CursorY)
	if got, _ := plateAt(); got < sx-1 || got > sx+1 {
		t.Errorf("straight after a scroll the tag is at column %d, want %d: it "+
			"was at %d and is gliding across the board while its owner stood still",
			got, sx, was)
	}
}
