package lumon

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/terminaltrove/lumon/internal/canvas"
	"github.com/terminaltrove/lumon/internal/glyph"
	"github.com/terminaltrove/lumon/internal/theme"
)

// arriving is a terminal that has just taken a file out of the drawer.
func arriving(t *testing.T, w, h int) *Model {
	t.Helper()
	rs := NewRooms(29)
	rs.EnsureOpen(DefaultRooms)
	m := NewLocalSession(rs, 1, "mark")
	m.Resize(w, h)
	tick(m, 120) // let the drawer come up
	m.Update(key("enter"))
	if m.Room == nil {
		t.Fatal("taking a card did not seat anybody")
	}
	if m.arrival == nil {
		t.Fatal("the board did not arrive")
	}
	return m
}

// digitsIn counts the plain digits drawn across a band of rows. RowPitch is 2,
// so every other board row is blank and a single row would measure the spacing.
func digitsIn(buf *canvas.Buffer, top, rows int) int {
	n := 0
	for y := top; y < top+rows; y++ {
		for _, r := range buf.Row(y) {
			if r >= '0' && r <= '9' {
				n++
			}
		}
	}
	return n
}

func TestTheBoardArrivesFromTheMiddle(t *testing.T) {
	m := arriving(t, 120, 40)
	l := m.Layout

	const band = 4
	mid := l.GridTop + l.BoardH/2 - band/2
	edge := l.GridTop

	tick(m, 12)
	buf := drawTo(m)
	if digitsIn(buf, mid, band) == 0 {
		t.Error("nothing has arrived in the middle of the board yet")
	}
	if got := digitsIn(buf, edge, band); got != 0 {
		t.Errorf("%d digits are already drawn at the top of the board; the "+
			"wavefront has not reached it", got)
	}

	// A moment later it has crossed the board.
	tick(m, 90)
	buf = drawTo(m)
	for _, y := range []int{edge, mid} {
		if digitsIn(buf, y, band) == 0 {
			t.Errorf("rows %d..%d are still empty once the board has arrived",
				y, y+band-1)
		}
	}
	if m.arrival != nil {
		t.Error("the arrival is still held after it finished; a settled board " +
			"must pay nothing but a nil check")
	}
}

// A terminal row is about two columns tall, so a reveal measured in cells would
// be an ellipse three times wider than high. Reach converts through CellW, RowH
// and RowAspect.
func TestTheArrivalIsRoundOnTheScreen(t *testing.T) {
	// 42 rows, so the viewport is an odd number of cells tall and its centre,
	// which Arrival.Sync puts at (h-1)/2, lands on a cell. With an even h this
	// test's integer cy is half a cell off and its two samples are at different
	// radii.
	m := arriving(t, 120, 42)
	a := m.arrival
	l := m.Layout
	if l.ViewportH%2 == 0 {
		t.Fatalf("the viewport is %d cells tall; this needs an odd one so its "+
			"centre is on a cell", l.ViewportH)
	}

	cx, cy := l.ViewportW/2, l.ViewportH/2

	// A cell three grid rows up is 3 × RowPitch × RowAspect visual columns away;
	// the same distance sideways is that many / CellW cells. Both should be at the
	// same point of the wavefront.
	up := 3
	across := int(math.Round(float64(up) * float64(l.RowH) * RowAspect / float64(l.CellW)))

	tick(m, 25)
	vert := a.Reach(cx, cy-up)
	horiz := a.Reach(cx+across, cy)
	if math.Abs(vert-horiz) > 0.15 {
		t.Errorf("%d rows up is %.2f arrived and the same screen distance across "+
			"is %.2f; the wavefront is an ellipse", up, vert, horiz)
	}

	// The same cell count either way is not the same distance, so the conversion
	// is needed.
	if naive := a.Reach(cx+up, cy); math.Abs(naive-vert) < 0.15 {
		t.Errorf("%d cells across reads %.2f and %d rows up reads %.2f; if those "+
			"agree the test is not measuring what it thinks", up, naive, up, vert)
	}
}

// The palette is swapped for a dimmed one and every drawing function is left
// alone.
func TestTheChromeComesUpOutOfTheDark(t *testing.T) {
	m := arriving(t, 120, 40)
	rule := m.Layout.GridTop - TopRuleHeight

	tick(m, 2)
	early := drawTo(m).AttrAt(0, rule).FG
	tick(m, 90)
	late := drawTo(m).AttrAt(0, rule).FG

	if early == late {
		t.Errorf("the rule is %v on the first frame and %v a second later; the "+
			"chrome is not fading in at all", early, late)
	}
	if late != m.Theme.Primary {
		t.Errorf("the rule settled at %v, not the theme's %v", late, m.Theme.Primary)
	}
	if early != m.Theme.Backdrop && early.String() == m.Theme.Primary.String() {
		t.Error("the chrome starts at full brightness")
	}
}

func TestARotationArrivesToo(t *testing.T) {
	rs := NewRooms(31)
	m := JoinRoom(rs.OpenNamed("Minsk"), 1)
	m.Resize(120, 40)
	if m.arrival != nil {
		t.Fatal("a board handed to a Model directly should not arrive")
	}

	completeTheFile(m)
	tick(m, int(FileHoldSeconds*60)+10)

	if m.Room.Epoch() == 0 {
		t.Fatal("the file never rotated")
	}
	if m.arrival == nil {
		t.Error("the next file did not arrive; followFile is the second hook")
	}
}

// afterLayout re-bases everything else the same way.
func TestAResizeDoesNotReplayTheArrival(t *testing.T) {
	m := arriving(t, 120, 40)
	tick(m, 30)

	was := m.arrival.elapsed
	m.Resize(96, 30)
	if m.arrival == nil {
		t.Fatal("the resize threw the arrival away")
	}
	if got := m.arrival.elapsed; got != was {
		t.Errorf("the clock went from %v to %v across a resize", was, got)
	}
	// Re-measured against the new viewport, on the middle cell and not the middle
	// of the block: Reach is asked about whole cells.
	if want := float64(m.Layout.ViewportW-1) / 2; m.arrival.cx != want {
		t.Errorf("the centre is %v against a viewport %d wide, want %v",
			m.arrival.cx, m.Layout.ViewportW, want)
	}
}

// Otherwise finishing a file blanks the terminal for seven seconds.
func TestTheFinishedFileIsAModalOverTheBoard(t *testing.T) {
	rs := NewRooms(31)
	m := JoinRoom(rs.OpenNamed("Minsk"), 1)
	m.Resize(120, 40)
	m.Name = "MARK S."
	m.Desk.Name = "MARK S."
	completeTheFile(m)
	openNotice(m)

	buf := drawTo(m)
	l := m.Layout

	// The board is still under it: rows well clear of the modal have digits.
	if got := digitsIn(buf, l.GridTop, 4); got == 0 {
		t.Error("the board is not drawn under the notice")
	}
	// So are the bins and the file bar.
	if !onScreen(buf, "% Complete") {
		t.Error("the file bar is gone while the file is being held")
	}

	// The hundred is set large, several rows of glyph runes.
	tall := 0
	for y := 0; y < l.Height; y++ {
		if strings.ContainsAny(string(buf.Row(y)), string(glyph.GlyphSolid)+"▓▒░·") {
			tall++
		}
	}
	if tall < int(completeGlyphHeights[len(completeGlyphHeights)-1])-1 {
		t.Errorf("the hundred covers %d rows; it is meant to be set large", tall)
	}
	if !onScreen(buf, "COMPLETE") {
		t.Error("the notice no longer says what it is")
	}
}

// Ink on Primary is the loudest thing on the board, and corruption must be.
// The show's cursor is a rectangle barely lifted off the page with the digit
// over it going brighter. Asserted as a comparison, so no palette change can
// bring the block back unnoticed.
func TestTheCursorIsAPlateAndNotABlock(t *testing.T) {
	for _, th := range theme.Themes() {
		toPage := colourGap(th.Cursor.BG, th.Ink)
		toVoice := colourGap(th.Cursor.BG, th.Primary)
		if toPage >= toVoice {
			t.Errorf("%s: the cursor's plate %v is nearer the terminal's voice %v "+
				"than its dark %v; it is a block again",
				th.Name, th.Cursor.BG, th.Primary, th.Ink)
		}
		if th.Cursor.FG != th.Peak {
			t.Errorf("%s: the cursor's digit is %v, want the peak %v, Ink only "+
				"works on a bright block and is invisible on a dark one",
				th.Name, th.Cursor.FG, th.Peak)
		}
		if th.Cursor.BG.Set() == false {
			t.Errorf("%s: the cursor has no plate at all", th.Name)
		}
	}
}

// colourGap is how far apart two colours are, as a rough sum of channels.
func colourGap(a, b canvas.Color) int {
	ar, ag, ab := hexOf(a)
	br, bg, bb := hexOf(b)
	return abs(ar-br) + abs(ag-bg) + abs(ab-bb)
}

func hexOf(c canvas.Color) (r, g, b int) {
	var rr, gg, bb int
	if _, err := fmt.Sscanf(c.String(), "#%02x%02x%02x", &rr, &gg, &bb); err != nil {
		return 0, 0, 0
	}
	return rr, gg, bb
}

// Arrival.dim must dim backgrounds as well as foregrounds, or every attribute
// with one (the cursor's plate, a held cluster's, another refiner's) is at full
// strength on the first frame. 42 rows for the same reason as the roundness
// test: with an even viewport the cursor's cell is half a cell off the centre
// and the wave passes it before this samples.
func TestTheCursorFadesInWithTheBoard(t *testing.T) {
	m := arriving(t, 120, 42)
	at := func() canvas.Color {
		x, y := m.Layout.ScreenOf(m.CursorX, m.CursorY)
		return drawTo(m).AttrAt(x, y).BG
	}

	tick(m, 2)
	early := at()
	tick(m, 90)
	late := at()

	if early == late {
		t.Errorf("the cursor's plate is %v on the first frame and %v a second "+
			"later; it is not fading in with the board", early, late)
	}
	if late != m.Theme.Cursor.BG {
		t.Errorf("the plate settled at %v, not the theme's %v", late, m.Theme.Cursor.BG)
	}
	if colourGap(early, m.Theme.Backdrop) > colourGap(late, m.Theme.Backdrop) {
		t.Error("the plate starts further from the backdrop than it ends; it is " +
			"arriving lit and dimming rather than coming up")
	}
}

// That is where the arrival's wavefront starts.
func TestTheCursorStartsInTheMiddle(t *testing.T) {
	m := arriving(t, 120, 40)
	l := m.Layout

	wantX := l.ViewportX + l.ViewportW/2
	wantY := l.ViewportY + l.ViewportH/2
	if m.CursorX != wantX || m.CursorY != wantY {
		t.Errorf("a fresh board puts the cursor at (%d,%d), want the middle of "+
			"the viewport (%d,%d)", m.CursorX, m.CursorY, wantX, wantY)
	}
	if m.CursorX < l.ViewportX || m.CursorX >= l.ViewportX+l.ViewportW ||
		m.CursorY < l.ViewportY || m.CursorY >= l.ViewportY+l.ViewportH {
		t.Error("the cursor is not on screen at all")
	}
}

// It goes through refine, so the bins are credited on the same clock, the
// deposit is recorded for other terminals, and the notice fires because
// percent() reached a hundred.
func TestTheCompletionDebugKeyFinishesTheFile(t *testing.T) {
	CompletionDebug = true
	defer func() { CompletionDebug = false }()

	m := ready()
	if m.Room.Complete() {
		t.Fatal("the file was already finished")
	}
	m.Update(key(completionDebugKey))
	tick(m, int(creditDelay*60)+120)

	if !m.Room.Complete() {
		t.Fatalf("the file is at %d%% with %d clusters left; the key did not finish it",
			m.Room.percent(), len(m.Grid.Groups))
	}
	if n := len(m.Grid.Groups); n != 0 {
		t.Errorf("%d clusters are still on the board", n)
	}

	// No bin is past its quota: refine credits SelectedBin, and credit lands on a
	// timer, so choosing by Bin.Refined would put every cluster in bin one.
	for _, b := range m.Grid.Bins {
		if b.Refined > b.Quota {
			t.Errorf("bin %d holds %d of a quota of %d", b.ID, b.Refined, b.Quota)
		}
	}
	if !onScreen(drawTo(m), "COMPLETE") {
		t.Error("the notice is not up")
	}
}

func TestTheCompletionDebugKeyIsOffByDefault(t *testing.T) {
	m := ready()
	was := len(m.Grid.Groups)
	m.Update(key(completionDebugKey))
	tick(m, 60)

	if len(m.Grid.Groups) != was {
		t.Errorf("%d clusters went without the flag; the key is live in an ordinary "+
			"session", was-len(m.Grid.Groups))
	}
	if m.Room.Complete() {
		t.Error("the file finished without the flag")
	}
}

// It is hooked as an `if`, never a `case`: keys_test.go finds a handler's keys
// by walking its case clauses and requires each to be documented, and a
// flag-only shortcut must not be promised.
func TestTheDebugKeyIsNotInTheControls(t *testing.T) {
	for _, grp := range Bindings() {
		for _, b := range grp.Bindings {
			if strings.Contains(b.Keys, completionDebugKey) ||
				slices.Contains(b.Bound, completionDebugKey) {
				t.Errorf("%q is in the controls under %q", completionDebugKey, grp.Title)
			}
		}
	}

	CompletionDebug = true
	defer func() { CompletionDebug = false }()
	m := ready()
	m.Update(key("?"))
	if onScreen(drawTo(m), completionDebugKey) {
		t.Errorf("%q is named in the controls overlay", completionDebugKey)
	}
}

// Drawn through glyph.RampRune, every antialiased edge comes out grey and each
// zero's counter gets a dot. The coverage is thresholded and only GlyphSolid is
// drawn, so any other ramp character in the box means shading.
func TestTheHundredIsSolidAndNotShaded(t *testing.T) {
	CompletionDebug = true
	defer func() { CompletionDebug = false }()

	m := ready()
	m.Update(key(completionDebugKey))
	tick(m, int(creditDelay*60)+120)
	buf := drawTo(m)

	// The number's own rows, just above its caption, and its own extent in
	// each: GlyphRamp has the middle dot, which the footer uses to separate its
	// hints, and the bins' fill bars are solid blocks too.
	caption := -1
	for y := 0; y < m.Layout.Height && caption < 0; y++ {
		if strings.Contains(buf.Row(y), "COMPLETE") {
			caption = y
		}
	}
	if caption < 0 {
		t.Fatal("no COMPLETE caption on screen")
	}
	top := caption
	for top > 0 && strings.ContainsRune(buf.Row(top-1), glyph.GlyphSolid) {
		top--
	}
	solid, shaded, rows := 0, 0, 0
	for y := top; y < caption; y++ {
		row := []rune(buf.Row(y))
		first, last := -1, -1
		for x, r := range row {
			if r == glyph.GlyphSolid {
				if first < 0 {
					first = x
				}
				last = x
			}
		}
		if first < 0 {
			continue
		}
		rows++
		for _, r := range row[first : last+1] {
			switch {
			case r == glyph.GlyphSolid:
				solid++
			case strings.ContainsRune(glyph.GlyphRamp, r):
				shaded++
			}
		}
	}
	if shaded > 0 {
		t.Errorf("%d cells inside the hundred are drawn in the ramp; it is shaded "+
			"rather than solid", shaded)
	}

	// It is big: at least completeGlyphHeights[0] rows of a twenty-six row modal.
	if want := int(completeGlyphHeights[0]); rows < want {
		t.Errorf("the hundred covers %d rows, want %d", rows, want)
	}
	if solid < 60 {
		t.Errorf("only %d cells of the hundred were drawn; it is not being set large", solid)
	}
}

// A box that grows out of the middle of the board is handed to you; one that
// appears whole is an announcement. Measured on the drawn box, since a spring
// that ran while the box was painted at full height would pass. Counted between
// GridTop and BinTop, where nothing else has a border: the header and the bins
// have boxes of their own.
func TestTheNoticeOpensRatherThanCuttingIn(t *testing.T) {
	CompletionDebug = true
	defer func() { CompletionDebug = false }()

	m := ready()
	m.Update(key(completionDebugKey))

	noticeRows := func() int {
		buf := drawTo(m)
		n := 0
		for y := m.Layout.GridTop; y < m.Layout.BinTop; y++ {
			if strings.ContainsAny(buf.Row(y), "\u250c\u2514\u2502") {
				n++
			}
		}
		return n
	}

	// Credit lands on a timer, so the file is not finished on the frame the key is
	// pressed.
	for i := 0; i < 600 && !m.Room.Complete(); i++ {
		tick(m, 1)
	}
	if !m.Room.Complete() {
		t.Fatal("the file never finished")
	}

	first := noticeRows()
	if onScreen(drawTo(m), "COMPLETE") {
		t.Error("the contents are drawn into a box that has not finished opening")
	}

	grew, was := 0, first
	for i := 0; i < 90; i++ {
		tick(m, 1)
		if now := noticeRows(); now > was {
			grew++
			was = now
		}
	}
	if grew < 3 {
		t.Errorf("the box grew on %d frames, from %d rows to %d; it is cutting in "+
			"rather than opening", grew, first, was)
	}
	if was < 12 {
		t.Errorf("the box reached %d rows; it never opened", was)
	}
	if !onScreen(drawTo(m), "COMPLETE") {
		t.Error("the notice opened and never said anything")
	}
}

// The bins are the fireworks, so this asserts on the lids too: Step is given
// this terminal's own BinFill, and the same SetHold the FIREWORKS diversion
// drives opens the boxes.
func TestFinishingAFileLaunchesFireworks(t *testing.T) {
	CompletionDebug = true
	defer func() { CompletionDebug = false }()

	m := ready()
	m.Update(key(completionDebugKey))

	opened, flying := 0, 0
	for i := 0; i < 240; i++ {
		tick(m, 1)
		for b := 0; b < BinCount; b++ {
			if m.BinFill.Lid(b) >= lidClear {
				opened++
				break
			}
		}
		if m.finale != nil {
			if air, _ := m.finale.Airborne(); air > 0 {
				flying++
			}
		}
	}
	if opened == 0 {
		t.Error("no bin ever opened its lid; the boxes are not firing")
	}
	if flying < 60 {
		t.Errorf("digits were in the air on %d of 240 frames; nothing is being "+
			"launched", flying)
	}

	// It is gone when the file is. A Fireworks holds a particle per digit on the
	// board and would otherwise be integrated for the rest of the session.
	tick(m, int(FileHoldSeconds*60)+30)
	if m.Room.Complete() {
		t.Fatal("the file never rotated")
	}
	if m.finale != nil {
		t.Error("the display outlived its file")
	}
	if m.noticeOpen != nil {
		t.Error("the notice did not close")
	}
	for b := 0; b < BinCount; b++ {
		if m.BinFill.Holding(b) {
			t.Errorf("bin %d is still held open on the next file", b+1)
		}
	}
}

// stepView is not called while a diversion runs, so the celebration is stepped
// from stepChrome, which runs on both paths, or five boxes would stay open.
func TestADiversionDuringTheHoldGivesTheLidsBack(t *testing.T) {
	CompletionDebug = true
	defer func() { CompletionDebug = false }()

	m := ready()
	m.Update(key(completionDebugKey))

	// Credit lands on a creditDelay timer, so the file is not complete on the frame
	// the key is pressed.
	for i := 0; i < 600 && m.finale == nil; i++ {
		tick(m, 1)
	}
	if m.finale == nil {
		t.Fatal("the celebration never started")
	}

	typeWord(m, theWord)
	m.Update(key("enter"))
	tick(m, 60)

	if m.finale != nil {
		t.Error("the celebration is still running under a diversion")
	}
	for b := 0; b < BinCount; b++ {
		if m.BinFill.Holding(b) {
			t.Errorf("bin %d is still held open by the celebration", b+1)
		}
	}
}

func TestTheHundredSaysItsOwnPercent(t *testing.T) {
	CompletionDebug = true
	defer func() { CompletionDebug = false }()

	m := ready()
	m.Update(key(completionDebugKey))
	for i := 0; i < 600 && !m.Room.Complete(); i++ {
		tick(m, 1)
	}
	openNotice(m)
	buf := drawTo(m)

	// The caption is the word alone, so the number is not on screen twice.
	if !onScreen(buf, "COMPLETE") {
		t.Error("the notice does not say what it is")
	}
	if onScreen(buf, "100% COMPLETE") {
		t.Error("the caption still repeats the number set above it")
	}

	// The mark is drawn. A glyph row with ink, a gap, then ink again is a ring,
	// and the percent has two.
	cov := glyph.RasterizeRuneFor('%', completeGlyphHeights[0]*glyph.CellAspect,
		completeGlyphHeights[0], 0, true)
	if cov == nil {
		t.Fatal("there is no outline for the percent")
	}
	// A counter needs about four cells across to survive the pen, and the box is
	// nine columns for a diagonal and two rings, so at seven rows the rings are
	// solid. What is asserted is the arrangement: the diagonal runs bottom left to
	// top right, so ink in the other two corners can only be the rings. A plain
	// slash leaves those quadrants empty.
	lit := func(x, y int) bool { return cov.At(x, y) >= completeSolidAt }
	topLeft, bottomRight := 0, 0
	for y := 0; y < cov.H; y++ {
		for x := 0; x < cov.W; x++ {
			if !lit(x, y) {
				continue
			}
			switch {
			case x*2 < cov.W && y*2 < cov.H:
				topLeft++
			case x*2 > cov.W && y*2 > cov.H:
				bottomRight++
			}
		}
	}
	if topLeft < 2 || bottomRight < 2 {
		t.Errorf("the percent inks %d cells top left and %d bottom right at %.0f "+
			"rows; its rings are gone and it is a plain slash",
			topLeft, bottomRight, completeGlyphHeights[0])
	}
}
