package lumon

import (
	"github.com/terminaltrove/lumon/internal/theme"
	"math"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// frameInterval is one physics step as a Duration.
var frameInterval = time.Second / 60

func ready() *Model {
	m := NewModel(1)
	m.Resize(120, 40)
	return m
}

// tick advances the model by n frames of simulated time.
func tick(m *Model, n int) {
	for i := 0; i < n; i++ {
		m.Update(FrameMsg(frameInterval))
	}
}

func TestArrowKeysMoveTheCursor(t *testing.T) {
	m := ready()
	m.CursorX, m.CursorY = 5, 5

	m.Update(key("right"))
	m.Update(key("down"))

	if m.CursorX != 6 || m.CursorY != 6 {
		t.Errorf("cursor at (%d,%d), want (6,6)", m.CursorX, m.CursorY)
	}
}

func TestCursorCannotLeaveTheGrid(t *testing.T) {
	m := ready()
	m.CursorX, m.CursorY = 0, 0

	m.Update(key("left"))
	m.Update(key("up"))

	if m.CursorX != 0 || m.CursorY != 0 {
		t.Errorf("cursor at (%d,%d), want it clamped to (0,0)", m.CursorX, m.CursorY)
	}
}

func TestMovingPastTheViewportEdgeScrolls(t *testing.T) {
	m := ready()
	m.CursorX = m.Layout.ViewportX + m.Layout.ViewportW - 1
	before := m.Layout.ViewportX

	m.Update(key("right"))

	if m.Layout.ViewportX <= before {
		t.Errorf("viewport did not scroll: %d then %d", before, m.Layout.ViewportX)
	}
	if m.CursorX < m.Layout.ViewportX || m.CursorX >= m.Layout.ViewportX+m.Layout.ViewportW {
		t.Error("cursor scrolled out of its own viewport")
	}
}

func TestNumberKeysSelectABin(t *testing.T) {
	m := ready()

	m.Update(key("4"))

	if m.SelectedBin != 3 {
		t.Errorf("SelectedBin = %d, want 3", m.SelectedBin)
	}
}

func TestQuitKeysSetQuitting(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		m := ready()
		m.Update(key(k))
		if !m.Quitting {
			t.Errorf("%q did not quit", k)
		}
	}
}

func TestIToggleIdleMode(t *testing.T) {
	m := ready()

	m.Update(key("i"))
	if m.Screen != ScreenIdle {
		t.Fatal("i did not enter idle mode")
	}

	m.Update(key("x"))
	if m.Screen != ScreenMain {
		t.Error("any key should leave idle mode")
	}
}

// The refine flow, end to end.

func activateFirstVisibleGroup(m *Model) *BadGroup {
	l := m.Layout
	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			n := m.Grid.At(gx, gy)
			if n.BadGroupID < 0 {
				continue
			}
			grp := m.Grid.Group(n.BadGroupID)
			m.Activate(grp)
			m.CursorX, m.CursorY = gx, gy
			return grp
		}
	}
	return nil
}

func TestRefiningAnActiveClusterLaunchesItsDigits(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible in the default viewport")
	}
	want := len(grp.NumberIDs)

	m.Update(key("enter"))
	tick(m, 1)

	if launched := len(m.Flights.Active()); launched != want {
		t.Errorf("%d digits launched after collapsing, want %d", launched, want)
	}
}

func TestRefinedDigitsAreNotDrawnInTheGrid(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}

	m.Update(key("enter"))
	tick(m, 120) // let them launch

	inFlightOrGone := 0
	for _, id := range grp.NumberIDs {
		if m.Flights.InFlight(id) || !m.Regen.Active(id) == false {
			inFlightOrGone++
		}
	}
	if inFlightOrGone == 0 {
		t.Error("no digit left its cell after collapsing")
	}
}

func TestRefinedGroupCreditsItsBinOnArrival(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	m.SelectedBin = 2
	before := m.Grid.Bins[2].Refined

	m.Update(key("enter"))
	tick(m, 900)

	if got := m.Grid.Bins[2].Refined; got != before+1 {
		t.Errorf("bin 2 refined count = %d, want %d", got, before+1)
	}
	if len(m.Flights.Active()) != 0 {
		t.Errorf("%d digits still airborne after 15 seconds", len(m.Flights.Active()))
	}
}

func TestBinIsCreditedOncePerCluster(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil || len(grp.NumberIDs) < 2 {
		t.Skip("need a multi-digit group")
	}
	m.SelectedBin = 1

	m.Update(key("enter"))
	tick(m, 900)

	if got := m.Grid.Bins[1].Refined; got != 1 {
		t.Errorf("bin credited %d times for one cluster of %d digits", got, len(grp.NumberIDs))
	}
}

func TestRefiningAnInactiveClusterDoesNothing(t *testing.T) {
	m := ready()
	// Find a bad digit but do not activate its group.
	l := m.Layout
	found := false
	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH && !found; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			if m.Grid.At(gx, gy).BadGroupID >= 0 {
				m.CursorX, m.CursorY = gx, gy
				found = true
				break
			}
		}
	}
	if !found {
		t.Skip("no bad group visible")
	}

	m.Update(key("enter"))

	if got := len(m.Flights.Active()); got != 0 {
		t.Errorf("%d digits launched from a dormant cluster, want 0", got)
	}
}

func TestRefiningAnOrdinaryNumberDoesNothing(t *testing.T) {
	m := ready()
	for gy := 0; gy < 20; gy++ {
		for gx := 0; gx < 20; gx++ {
			if m.Grid.At(gx, gy).BadGroupID < 0 {
				m.CursorX, m.CursorY = gx, gy
				m.Update(key("enter"))
				if got := len(m.Flights.Active()); got != 0 {
					t.Fatalf("refining a normal digit launched %d flights", got)
				}
				return
			}
		}
	}
}

// Mouse handling must go through Layout.
func TestMouseClickSelectsTheCellUnderTheCursor(t *testing.T) {
	m := ready()
	wantX, wantY := m.Layout.ViewportX+4, m.Layout.ViewportY+3
	sx, sy := m.Layout.ScreenOf(wantX, wantY)

	m.Update(tea.MouseMsg{X: sx, Y: sy, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})

	if m.CursorX != wantX || m.CursorY != wantY {
		t.Errorf("click at screen (%d,%d) selected (%d,%d), want (%d,%d)",
			sx, sy, m.CursorX, m.CursorY, wantX, wantY)
	}
}

func TestMouseClickOnABinSelectsIt(t *testing.T) {
	m := ready()

	m.Update(tea.MouseMsg{
		X: m.Layout.BinCenterX[3], Y: m.Layout.BinTop + 1,
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	})

	if m.SelectedBin != 3 {
		t.Errorf("SelectedBin = %d, want 3", m.SelectedBin)
	}
}

func TestWindowResizeUpdatesTheLayout(t *testing.T) {
	m := ready()

	m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})

	if m.Layout.Width != 60 || m.Layout.Height != 20 {
		t.Errorf("layout is %dx%d, want 60x20", m.Layout.Width, m.Layout.Height)
	}
}

func TestActiveClusterAgitatesWhileDormantOnesDoNot(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}

	tick(m, 120)

	moved := false
	for _, id := range grp.NumberIDs {
		if dx, dy := m.Agitation.Offset(id); dx != 0 || dy != 0 {
			moved = true
		}
	}
	if !moved {
		t.Error("the active cluster is not agitating")
	}
}

func TestIdleScreenRendersTheMark(t *testing.T) {
	m := ready()
	m.Update(key("i"))

	tick(m, 60)
	out := m.View()

	if out == "" {
		t.Fatal("idle screen rendered nothing")
	}
	found := false
	for _, r := range out {
		if r >= '⠀' && r <= '⣿' && r != '⠀' {
			found = true
			break
		}
	}
	if !found {
		t.Error("no braille dots in the idle screen")
	}
}

// A vacated cell spins, so it does not simply show a new number the next frame.
func TestVacatedCellSpinsBeforeSettling(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	id := grp.NumberIDs[0]

	m.Update(key("enter"))

	seen := map[int]bool{}
	spun := false
	for i := 0; i < 1200; i++ {
		tick(m, 1)
		if v, ok := m.Regen.Display(id); ok {
			spun = true
			seen[v] = true
		}
	}

	if !spun {
		t.Fatal("the vacated cell never entered a regeneration")
	}
	if len(seen) < 3 {
		t.Errorf("the cell showed %d distinct digits, want a visible scramble", len(seen))
	}
	if m.Regen.Active(id) {
		t.Error("the cell never settled")
	}
}

// A refined cluster leaves from where it was drawn, agitation and all.
func TestRefinedDigitsLeaveFromWhereTheyWereDrawn(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	tick(m, 240) // let the cluster build up real motion
	l := m.Layout
	drawn := map[int][2]float64{}
	for _, id := range grp.NumberIDs {
		sx, sy := l.ScreenOf(id%GridSize, id/GridSize)
		ox, oy := l.Screen(m.Agitation.Offset(id))
		drawn[id] = [2]float64{float64(sx) + ox, float64(sy) + oy}
	}

	m.Update(key("enter"))
	if len(m.Flights.Active()) == 0 {
		t.Fatal("no digit launched")
	}
	for _, f := range m.Flights.Active() {
		if p, ok := drawn[f.ID]; ok && (f.X != p[0] || f.Y != p[1]) {
			t.Errorf("digit %d left from (%v,%v), drawn at (%v,%v)", f.ID, f.X, f.Y, p[0], p[1])
		}
	}
	tick(m, int(flightHold*60)+30)
	moved := false
	for _, f := range m.Flights.Active() {
		moved = moved || f.X != drawn[f.ID][0] || f.Y != drawn[f.ID][1]
	}
	if !moved && len(m.Flights.Active()) > 0 {
		t.Error("the cluster never left")
	}
}

// Digits from one cluster fan out instead of following a single path.
func TestRefinedDigitsFanOut(t *testing.T) {
	m := ready()
	var grp *BadGroup
	for _, g := range m.Grid.Groups {
		if len(g.NumberIDs) >= 3 && g.NumberIDs[0]/GridSize < 15 {
			grp = g
			break
		}
	}
	if grp == nil {
		t.Skip("no multi-digit cluster near the top of the grid")
	}
	m.Activate(grp)
	id := grp.NumberIDs[0]
	m.CursorX, m.CursorY = id%GridSize, id/GridSize
	tick(m, 120)

	m.Update(key("enter"))
	tick(m, 15)

	positions := map[[2]int]bool{}
	for _, f := range m.Flights.Active() {
		positions[[2]int{int(f.X), int(f.Y)}] = true
	}
	if len(m.Flights.Active()) > 1 && len(positions) < 2 {
		t.Error("every refined digit is at the same place; they are not fanning out")
	}
}

// A refined digit must shrink promptly and keep showing its own value, not the
// replacement number, until it leaves.
func TestRefinedDigitsCollapsePromptlyAndKeepTheirValue(t *testing.T) {
	m := ready()
	grp, focus := activateClusterWithRoom(t, m)
	tick(m, 900)
	if m.Magnify.Height(focus%GridSize, focus/GridSize) < MinGlyphHeight {
		t.Skip("the cluster did not reach glyph size")
	}
	was := m.Grid.Cells[focus].Value

	m.Update(key("enter"))

	for i := 0; i < 90; i++ { // a second and a half
		tick(m, 1)
		if m.Flights.InFlight(focus) || m.Regen.Active(focus) {
			break
		}
		if got := m.Grid.Cells[focus].Value; got != was {
			t.Fatalf("the digit changed from %d to %d while collapsing", was, got)
		}
	}

	for _, id := range grp.NumberIDs {
		if h := m.Magnify.Height(id%GridSize, id/GridSize); h > MinGlyphHeight {
			t.Errorf("cell %d is still %v rows tall a second and a half after refining", id, h)
		}
	}
}

// parkCursorAwayHelper moves the cursor to the far corner of the viewport, to
// show a cluster grows on its own.
func parkCursorAwayHelper(m *Model) {
	l := m.Layout
	m.CursorX = l.ViewportX + l.ViewportW - 1
	m.CursorY = l.ViewportY + l.ViewportH - 1
}

func clusterPeak(m *Model, grp *BadGroup) float64 {
	peak := 0.0
	for _, id := range grp.NumberIDs {
		if h := m.Magnify.Height(id%GridSize, id/GridSize); h > peak {
			peak = h
		}
	}
	return peak
}

func TestWokenClusterGrowsWithoutTheCursor(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	parkCursorAwayHelper(m)

	// Half the escalation, by which point the swell is well clear of the threshold
	// however slowly it leaves the ground.
	tick(m, int(60*EscalateSeconds/2))

	if got := clusterPeak(m, grp); got < MinGlyphHeight {
		t.Errorf("the cluster peaked at %v rows, want it past %v", got, MinGlyphHeight)
	}
}

func TestWokenClusterApproachesFullSize(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	parkCursorAwayHelper(m)

	// A cluster's life less a moment, derived from the clock, so the window covers
	// the crest.
	peak := 0.0
	for i := 0; i < clusterLifeFrames()-120; i++ {
		tick(m, 1)
		if h := clusterPeak(m, grp); h > peak {
			peak = h
		}
	}

	if peak < BreatheHigh-0.5 {
		t.Errorf("the cluster only reached %v rows, want near %v", peak, BreatheHigh)
	}
}

// Size is the hint: it says "something is over here" and gets there in
// HintSeconds. The urgency that keeps climbing is in the colour and the shake.
func TestAWokenClusterSwellsToItsHintAndBreathes(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	parkCursorAwayHelper(m)

	var series []float64
	for i := 0; i < clusterLifeFrames()-120; i++ {
		tick(m, 1)
		series = append(series, clusterPeak(m, grp))
	}

	crest, at := 0.0, 0
	for i, h := range series {
		if h > crest {
			crest, at = h, i
		}
	}

	// It gets up to the hint's ceiling...
	if crest < BreatheLow {
		t.Errorf("the cluster crested at %v rows, want it up to at least %v",
			crest, BreatheLow)
	}
	// ...and never past it, which leaves the reveal to the gesture.
	if crest > BreatheHigh+0.1 {
		t.Errorf("the cluster crested at %v rows, past the %v ceiling", crest, BreatheHigh)
	}
	// ...having taken the swell's own clock to get there...
	if at < int(60*HintSeconds)/3 {
		t.Errorf("the cluster crested at frame %d, which is inside the first "+
			"third of its swell", at)
	}
	// ...and then it breathes instead of sitting on the number.
	turns := 0
	rising := true
	for i := 1; i < len(series); i++ {
		if d := series[i] - series[i-1]; d > 1e-6 && !rising {
			rising, turns = true, turns+1
		} else if d < -1e-6 && rising {
			rising, turns = false, turns+1
		}
	}
	if turns < 2 {
		t.Errorf("the cluster turned %d times in its whole life; it is meant to "+
			"breathe once it is up", turns)
	}
}

// clusterLifeFrames is how long a woken cluster lasts before it detonates, in
// frames. Derived, so windows that watch one cluster from waking to blast move
// with the clock.
func clusterLifeFrames() int {
	secs := theme.ScaryRedSeconds + FuseWait + FuseSeconds
	return int(60 * secs)
}

// Otherwise the cluster keeps vanishing.
func TestBreathingTroughStaysLarge(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	parkCursorAwayHelper(m)

	// Far enough up the ramp to be breathing. The band opens at PlainHeight, so a
	// young cluster's trough is an ordinary character; an escalated one must not
	// deflate out of being a glyph between breaths.
	tick(m, int(60*EscalateSeconds))

	// Only while it is still there: a loop long enough to see several breaths can
	// outlast the fuse.
	trough := 999.0
	for i := 0; i < 480 && m.IsActive(grp.ID); i++ {
		tick(m, 1)
		if h := clusterPeak(m, grp); h < trough {
			trough = h
		}
	}

	if trough < MinGlyphHeight {
		t.Errorf("the cluster deflated to %v rows between breaths, want it to stay a glyph", trough)
	}
}

// TestOnlyCorruptionMakesADigitBig generalises this.
func TestOrdinaryNumbersStayPlain(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	member := map[int]bool{}
	for _, id := range grp.NumberIDs {
		member[id] = true
	}
	parkCursorAwayHelper(m)
	tick(m, 900)

	l := m.Layout
	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			if awakeAt(m, gx, gy) {
				continue
			}
			if h := m.Magnify.Height(gx, gy); h != PlainHeight {
				t.Fatalf("ordinary cell (%d,%d) is %v rows", gx, gy, h)
			}
		}
	}
}

func TestClusterSizeSurvivesScrolling(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	parkCursorAwayHelper(m)

	// Waited on, not counted: the swell takes most of two minutes.
	for i := 0; i < int(60*EscalateSeconds) && clusterPeak(m, grp) < MinGlyphHeight; i++ {
		tick(m, 1)
	}
	before := clusterPeak(m, grp)
	if before < MinGlyphHeight {
		t.Skip("the cluster had not grown yet")
	}

	m.Layout.ScrollTo(m.Layout.ViewportX+6, m.Layout.ViewportY+4)

	if got := clusterPeak(m, grp); got != before {
		t.Errorf("cluster peak went %v -> %v across a scroll", before, got)
	}
}

// A refined digit launches at once, at the size it had, and shrinks on the way
// to its bin.

func grownCluster(t *testing.T, m *Model) *BadGroup {
	t.Helper()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	// Past the Schmitt band, beyond the threshold: glyphOn needs
	// MinGlyphHeight+glyphBand to turn a glyph on, so a cluster stopped at
	// MinGlyphHeight is drawn as plain characters and has no glyph box.
	for i := 0; i < 60*50 && clusterPeak(m, grp) < MinGlyphHeight+glyphBand; i++ {
		tick(m, 1)
	}
	if clusterPeak(m, grp) < MinGlyphHeight+glyphBand {
		t.Skip("the cluster never grew")
	}
	return grp
}

func TestRefiningLaunchesImmediately(t *testing.T) {
	m := ready()
	grp := grownCluster(t, m)

	m.Update(key("enter"))

	if got := len(m.Flights.Active()); got != len(grp.NumberIDs) {
		t.Errorf("%d digits airborne on the refine frame, want all %d", got, len(grp.NumberIDs))
	}
}

// Shrinking in its own cell before setting off would read as a deflate, a
// pause, and then a departure.
func TestRefinedDigitsLeaveAtTheSizeTheyHad(t *testing.T) {
	m := ready()
	grp := grownCluster(t, m)
	before := clusterPeak(m, grp)

	m.Update(key("enter"))

	peak := 0.0
	for _, f := range m.Flights.Active() {
		if f.Height > peak {
			peak = f.Height
		}
	}
	if peak < before-1e-9 {
		t.Errorf("largest digit left at %v rows, want the %v it had grown to", peak, before)
	}
	if peak <= MinGlyphHeight {
		t.Errorf("digits left at %v rows, small enough to be plain characters already", peak)
	}
}

func TestRefinedDigitsShrinkWhileTheyTravel(t *testing.T) {
	m := ready()
	grownCluster(t, m)
	m.Update(key("enter"))

	type sample struct{ dist, height float64 }
	prev := map[int]sample{}
	for _, f := range m.Flights.Active() {
		prev[f.ID] = sample{binDistance(m, f), f.Height}
	}

	shrankWhileMoving := false
	for i := 0; i < 600; i++ {
		tick(m, 1)
		for _, f := range m.Flights.Active() {
			was, ok := prev[f.ID]
			if !ok {
				continue
			}
			now := sample{binDistance(m, f), f.Height}
			if now.dist < was.dist-1e-9 && now.height < was.height-1e-9 {
				shrankWhileMoving = true
			}
			if now.height > was.height+1e-9 {
				t.Fatalf("digit %d grew back from %v to %v in flight", f.ID, was.height, now.height)
			}
			prev[f.ID] = now
		}
	}

	if !shrankWhileMoving {
		t.Error("no digit ever got smaller and closer to its bin in the same frame")
	}
}

// That ties the shrink to the journey, not to a clock.
func TestARefinedDigitIsPlainWhenItLands(t *testing.T) {
	m := ready()
	grp := grownCluster(t, m)
	m.Update(key("enter"))

	last := map[int]float64{}
	for i := 0; i < 900; i++ {
		tick(m, 1)
		for _, f := range m.Flights.Active() {
			last[f.ID] = f.Height
		}
		if len(m.Flights.Active()) == 0 && len(last) > 0 {
			break
		}
	}

	if len(last) == 0 {
		t.Fatal("no digit was ever seen in flight")
	}
	for _, id := range grp.NumberIDs {
		h, ok := last[id]
		if !ok {
			continue
		}
		if h > PlainHeight+0.5 {
			t.Errorf("digit %d was still %v rows tall on arrival, want a plain character", id, h)
		}
	}
}

func TestFlightTiltEasesBackToUpright(t *testing.T) {
	m := ready()
	grownCluster(t, m)
	m.Update(key("enter"))

	sawTilt := false
	last := map[int]float64{}
	for i := 0; i < 900; i++ {
		for _, f := range m.Flights.Active() {
			if f.Tilt != 0 {
				sawTilt = true
			}
			if f.Tilt > MaxCollapseTilt+1e-6 || f.Tilt < -MaxCollapseTilt-1e-6 {
				t.Fatalf("digit %d tilted %v degrees, beyond %v", f.ID, f.Tilt, MaxCollapseTilt)
			}
			last[f.ID] = f.Tilt
		}
		tick(m, 1)
	}

	if !sawTilt {
		t.Error("no digit ever tilted on its way to a bin")
	}
	for id, tilt := range last {
		if tilt > 1 || tilt < -1 {
			t.Errorf("digit %d was still leaning %v degrees on arrival", id, tilt)
		}
	}
}

func TestClusterLeavesRaggedly(t *testing.T) {
	m := ready()
	grp := grownCluster(t, m)
	if len(grp.NumberIDs) < 2 {
		t.Skip("need a multi-digit cluster")
	}
	m.Update(key("enter"))
	tick(m, 56) // past the hold, the last departure and its lean

	tilts := map[float64]bool{}
	for _, f := range m.Flights.Active() {
		tilts[f.Tilt] = true
	}
	if len(tilts) < 2 {
		t.Error("every member is leaning identically")
	}
}

// binDistance is how far a flight still has to go, in screen cells.
func binDistance(m *Model, f Flight) float64 {
	dx := f.X - float64(m.Layout.BinCenterX[f.BinIdx])
	dy := f.Y - float64(m.Layout.BinTop)
	return math.Hypot(dx, dy)
}

// The bin accounting must survive digits leaving at different sizes.
func TestRefiningStillCreditsTheBinExactlyOnce(t *testing.T) {
	m := ready()
	grownCluster(t, m)
	m.SelectedBin = 2

	m.Update(key("enter"))

	// Count the increments, not the final count: a detonation elsewhere docks a
	// credit from the fullest bin, so over forty seconds the count no longer
	// records what went in, but how many times it went up still does.
	credits, was := 0, m.Grid.Bins[2].Refined
	for i := 0; i < 2400; i++ {
		tick(m, 1)
		if now := m.Grid.Bins[2].Refined; now > was {
			credits += now - was
		}
		was = m.Grid.Bins[2].Refined
	}

	if credits != 1 {
		t.Errorf("bin 2 credited %d times, want exactly once", credits)
	}
	if len(m.Flights.Active()) != 0 {
		t.Errorf("%d digits still airborne", len(m.Flights.Active()))
	}
}

// Bin credit and the replacement digit run on the clock. Clocked by a flight
// reaching its bin, a screen-space distance, a tall terminal would write
// different numbers into the grid than a short one.
func TestTheBoardIsTheSameWhateverTheTerminalSize(t *testing.T) {
	play := func(w, h int) ([]int, [BinCount]int) {
		m := NewModel(9)
		m.Resize(w, h)
		grp := activateFirstVisibleGroup(m)
		if grp == nil {
			t.Skip("no bad group visible")
		}
		id := grp.NumberIDs[0]
		m.CursorX, m.CursorY = id%GridSize, id/GridSize
		m.SelectedBin = 2
		m.Update(key("enter"))
		tick(m, 600)

		vacated := make([]int, 0, len(grp.NumberIDs))
		for _, id := range grp.NumberIDs {
			vacated = append(vacated, m.Grid.Cells[id].Value)
		}
		var bins [BinCount]int
		for i := range m.Grid.Bins {
			bins[i] = m.Grid.Bins[i].Refined
		}
		return vacated, bins
	}

	smallCells, smallBins := play(80, 24)
	wideCells, wideBins := play(200, 60)

	if len(smallCells) != len(wideCells) {
		t.Fatalf("different cluster sizes: %d vs %d", len(smallCells), len(wideCells))
	}
	for i := range smallCells {
		if smallCells[i] != wideCells[i] {
			t.Errorf("vacated cell %d became %d on a small terminal and %d on a wide one",
				i, smallCells[i], wideCells[i])
		}
	}
	if smallBins != wideBins {
		t.Errorf("bin credit differs by terminal size: %v vs %v", smallBins, wideBins)
	}
}

// Credit arrives on the clock, not on arrival, but it still has to arrive.
func TestTheBinIsCreditedAfterARefine(t *testing.T) {
	m := ready()
	m.SelectedBin = 3
	before := m.Grid.Bins[3].Refined

	grownCluster(t, m)
	m.Update(key("enter"))

	if got := m.Grid.Bins[3].Refined; got != before {
		t.Errorf("bin credited instantly (%d -> %d); it should wait out the flight", before, got)
	}

	tick(m, 240) // comfortably past creditDelay
	if got := m.Grid.Bins[3].Refined; got != before+1 {
		t.Errorf("bin is at %d after the delay, want %d", got, before+1)
	}
}

// Harmless alone at a terminal, wrong the moment a board is shared.
func TestTheScreensaverDoesNotFreezeTheBoard(t *testing.T) {
	m := ready()
	if activateFirstVisibleGroup(m) == nil {
		t.Skip("no bad group visible")
	}
	tick(m, 60)

	before := m.ScaryElapsed()
	m.Update(key("i")) // into the screensaver
	if m.Screen != ScreenIdle {
		t.Fatal("did not enter the screensaver")
	}
	tick(m, 300)

	if got := m.ScaryElapsed(); got <= before {
		t.Errorf("the cluster's urgency is still %v after five seconds in the screensaver", got)
	}
}

func TestCreditArrivesWhileInTheScreensaver(t *testing.T) {
	m := ready()
	m.SelectedBin = 1
	grownCluster(t, m)
	before := m.Grid.Bins[1].Refined

	m.Update(key("enter"))
	m.Update(key("i"))
	tick(m, 240)

	if got := m.Grid.Bins[1].Refined; got != before+1 {
		t.Errorf("bin is at %d after refining then idling, want %d", got, before+1)
	}
}

// While a finished file is on its notice the grid is not drawn, so its keys
// must not move a cursor nobody can see or open the screensaver over the notice.
func TestTheFinishedFileNoticeSwallowsTheGridsKeys(t *testing.T) {
	m := ready()
	completeTheFile(m)
	tick(m, 1)

	beforeX, beforeY, beforeBin := m.CursorX, m.CursorY, m.SelectedBin

	m.Update(key("down"))
	m.Update(key("right"))
	m.Update(key("4"))
	m.Update(key("i"))

	if m.CursorX != beforeX || m.CursorY != beforeY {
		t.Errorf("the cursor moved to (%d,%d) behind the finished-file notice", m.CursorX, m.CursorY)
	}
	if m.SelectedBin != beforeBin {
		t.Errorf("the bin changed to %d behind the finished-file notice", m.SelectedBin)
	}
	if m.Screen == ScreenIdle {
		t.Error("i opened the screensaver over the finished-file notice")
	}
}

// The program asks for mouse reporting, so wheel events arrive whenever anyone
// scrolls.
func TestTheWheelScrollsTheGrid(t *testing.T) {
	m := ready()
	before := m.CursorY

	m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if m.CursorY <= before {
		t.Errorf("the wheel down left the cursor at row %d, want it below %d", m.CursorY, before)
	}

	down := m.CursorY
	m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	if m.CursorY >= down {
		t.Errorf("the wheel up left the cursor at row %d, want it above %d", m.CursorY, down)
	}
}

func TestTheWheelIsSafeBeforeTakingADesk(t *testing.T) {
	m := named(NewSession(NewRooms(1), 1), "mark")
	m.Resize(120, 40)
	m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	_ = drawTo(m)
}

// Retyping the word that opened it is no way out: its first keystroke is not
// swallowed and picks a bin on the way past.
func TestBackingOutLeavesTheThirdDimensionRatherThanQuitting(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	enter3D(t, m)

	m.Update(key("q"))
	if m.Quitting {
		t.Fatal("q quit the session from inside 3D")
	}
	if m.Solid() {
		t.Error("q did not leave 3D")
	}
}

// Out of a diversion, an overlay and the drawer, one rung at a time, quitting
// only once there is nothing left to be inside.
func TestBackingOutTakesOneThingAtATime(t *testing.T) {
	m := ready()
	m.Resize(120, 40)

	m.Helping = true
	m.About = true
	m.divertOpen = true

	for _, want := range []string{"help", "about", "drawer"} {
		m.Update(key("q"))
		if m.Quitting {
			t.Fatalf("q quit while the %s was still up", want)
		}
	}
	if m.Helping || m.About || m.divertOpen {
		t.Fatalf("something is still up: help=%v about=%v drawer=%v",
			m.Helping, m.About, m.divertOpen)
	}

	// Nothing left to back out of, so now it quits.
	m.Update(key("q"))
	if !m.Quitting {
		t.Error("q did not quit from the bare board")
	}
}

// Otherwise m.solid stays non-nil under a live diversion: the diversion draws
// over it, the footer shows the camera's keys, and ending the diversion drops
// you back into 3D.
func TestADiversionOpenedFromTheThirdDimensionLeavesIt(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	enter3D(t, m)

	startDiversionNamed(t, m, "MATRIX")
	if m.Solid() {
		t.Error("the diversion is running on top of a live 3D view")
	}
	// The footer is the board's own again: a diversion draws its hint inside the
	// panel, so the camera's keys there mean the view is still dangling.
	if hint := m.hintLine(120); strings.Contains(hint, "fly") {
		t.Errorf("the footer still shows the camera's keys: %q", hint)
	}
}

// The footer shows the short way out.
func TestTheThirdDimensionSaysHowToLeave(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	enter3D(t, m)
	hint := m.hintLine(120)
	if !strings.Contains(hint, "q back") {
		t.Errorf("the 3D footer does not say how to get out: %q", hint)
	}
	// And it does not offer the key that does nothing.
	if strings.Contains(hint, "esc") {
		t.Errorf("the 3D footer still offers esc: %q", hint)
	}
}

// wheel is one notch of the mouse wheel.
func wheel(up bool) tea.MouseMsg {
	b := tea.MouseButtonWheelDown
	if up {
		b = tea.MouseButtonWheelUp
	}
	return tea.MouseMsg{Button: b, Action: tea.MouseActionPress}
}

func TestTheWheelScrollsTheScoreboard(t *testing.T) {
	rs := NewRooms(3)
	room := rs.Open()
	m := JoinRoom(room, 1)
	if m == nil {
		t.Fatal("could not take a desk")
	}
	m.Resize(120, 34)

	l := m.ledger()
	if l == nil {
		t.Skip("no ledger")
	}
	for _, who := range []string{"MARK", "HELLY", "IRVING", "DYLAN"} {
		l.Refined(who, 1)
	}

	m.Screen, m.scoresAt = ScreenScores, 0
	m.Update(wheel(false))
	if m.scoresAt != 1 {
		t.Errorf("a wheel notch down left the marker at %d, want 1", m.scoresAt)
	}
	m.Update(wheel(true))
	if m.scoresAt != 0 {
		t.Errorf("a wheel notch up left the marker at %d, want 0", m.scoresAt)
	}
}

// The program asks for motion reporting, so moving the pointer across the
// notice would aim a lens at a grid that is not there.
func TestTheMouseDoesNotReachTheGridUnderTheFinishedNotice(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	tick(m, 4)

	// Finish the file outright, then let the room notice: Complete reads the hold
	// clock, not the bins, and advanceFile starts it.
	for i := range m.Grid.Bins {
		m.Grid.Bins[i].Refined = m.Grid.Bins[i].Quota
	}
	tick(m, 1)
	if !m.Room.Complete() {
		t.Fatal("the file did not read as finished")
	}

	before := m.Lens.Focus()
	m.CursorX, m.CursorY = 5, 5

	m.Update(tea.MouseMsg{X: 40, Y: 12, Action: tea.MouseActionMotion, Button: tea.MouseButtonNone})
	m.Update(tea.MouseMsg{X: 40, Y: 12, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})

	if got := m.Lens.Focus(); got != before {
		t.Errorf("the lens was aimed at a grid that is not drawn: %v then %v", before, got)
	}
	if m.CursorX != 5 || m.CursorY != 5 {
		t.Errorf("a click under the notice moved the cursor to (%d,%d)", m.CursorX, m.CursorY)
	}
}
