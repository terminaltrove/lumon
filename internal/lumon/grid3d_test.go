package lumon

import (
	"math"
	"strings"
	"testing"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// enter3D types the word and waits for the digits to land.
func enter3D(t *testing.T, m *Model) {
	t.Helper()
	m.Update(key("3"))
	m.Update(key("d"))
	for i := 0; i < 60*4 && m.transit != nil; i++ {
		tick(m, 1)
	}
	if m.transit != nil {
		t.Fatal("the trip into the third dimension never finished")
	}
	if !m.Solid() {
		t.Fatal("typing the word did not open the view")
	}
	tick(m, 30)
}

// A lone 3 still picks bin three: the first letter of a word is never
// swallowed, the same rule that lets a lone K move the cursor.
func TestTypingItOpensTheViewWithoutCostingTheBinKey(t *testing.T) {
	m := ready()
	m.Resize(110, 30)

	m.Update(key("3"))
	if m.SelectedBin != 2 {
		t.Errorf("a lone 3 left bin %d selected, want bin three", m.SelectedBin+1)
	}
	if m.Solid() {
		t.Fatal("one letter opened the view")
	}
	m.Update(key("d"))
	if !m.Solid() {
		t.Fatal("the word did not open the view")
	}

	// And it is a journey, not a cut.
	if m.transit == nil {
		t.Error("entering was instant")
	}
}

// It is not a diversion, so the ordinary keys still play the game.
func TestTheBoardIsStillPlayedInThreeDimensions(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	enter3D(t, m)

	if m.Diverting() {
		t.Fatal("the view reports itself as a diversion")
	}

	x, y := m.CursorX, m.CursorY
	m.Update(key("right"))
	tick(m, 2)
	if m.CursorX == x && m.CursorY == y {
		t.Error("the cursor did not move")
	}
	if !m.Solid() {
		t.Error("moving the cursor left the view")
	}

	m.Update(key("4"))
	if m.SelectedBin != 3 {
		t.Errorf("the bin keys stopped working; bin %d is selected", m.SelectedBin+1)
	}
	if !m.Solid() {
		t.Error("picking a bin left the view")
	}
}

// Same board, same code; only the picture differs.
func TestARefinementFromAnAngleIsTheSameRefinement(t *testing.T) {
	flat := ready()
	flat.Resize(110, 30)
	solid := ready()
	solid.Resize(110, 30)

	// Both models are the same seed, so they wake the same cluster.
	for i := 0; i < 60*30; i++ {
		tick(flat, 1)
		tick(solid, 1)
		if flat.FirstActive() != nil && solid.FirstActive() != nil {
			break
		}
	}
	a, b := flat.FirstActive(), solid.FirstActive()
	if a == nil || b == nil {
		t.Skip("nothing woke up")
	}
	if a.ID != b.ID {
		t.Skip("the two boards diverged")
	}

	enter3D(t, solid)
	if solid.FirstActive() == nil {
		t.Skip("the cluster was refined on the way in")
	}
	// Typing the word picks a bin on the way in (the first letter is never
	// swallowed), so both boards have to be aiming at the same one.
	flat.SelectedBin = solid.SelectedBin

	flat.refine(a)
	solid.refine(solid.FirstActive())
	tick(flat, 120)
	tick(solid, 120)

	for i := range flat.Grid.Cells {
		if flat.Grid.Cells[i].Value != solid.Grid.Cells[i].Value {
			t.Fatalf("cell %d is %d flat and %d from an angle",
				i, flat.Grid.Cells[i].Value, solid.Grid.Cells[i].Value)
		}
		if flat.Grid.Cells[i].BadGroupID != solid.Grid.Cells[i].BadGroupID {
			t.Fatalf("cell %d belongs to group %d flat and %d from an angle",
				i, flat.Grid.Cells[i].BadGroupID, solid.Grid.Cells[i].BadGroupID)
		}
	}
	for i := 0; i < BinCount; i++ {
		if flat.Grid.Bins[i].Refined != solid.Grid.Bins[i].Refined {
			t.Errorf("bin %d holds %d flat and %d from an angle", i+1,
				flat.Grid.Bins[i].Refined, solid.Grid.Bins[i].Refined)
		}
	}
}

// HitTest's second tier is the index the renderer builds while drawing, and
// recording where a digit was put works over a camera as well as over a lens.
func TestAClickLandsOnTheDigitThatWasDrawnThere(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	enter3D(t, m)
	drawTo(m)

	// Asked of the screen, so the test replays none of the renderer's geometry:
	// wherever a board digit is legible, clicking it must find a cell holding that
	// digit. The mass is excluded: it has no cells and answers nothing
	// (TestTheMassAnswersNoClick). The two are told apart by attribute, since the
	// mass is drawn in shades no live digit is given.
	buf := drawTo(m)
	panel := m.gridPanel()
	mass := map[canvas.Attr]bool{}
	for _, a := range m.solid.shades(m.Theme) {
		mass[a] = true
	}

	checked := 0
	for y := panel.Y; y < panel.Y+panel.H; y++ {
		for x := panel.X; x < panel.X+panel.W; x++ {
			r := buf.RuneAt(x, y)
			if r < '0' || r > '9' || mass[buf.AttrAt(x, y)] {
				continue
			}
			checked++
			gx, gy, ok := m.HitTest(x, y)
			if !ok {
				t.Errorf("nothing was found at (%d,%d), where %q is drawn", x, y, r)
				continue
			}
			if got := m.Grid.At(gx, gy); got == nil || rune('0'+got.Value) != r {
				t.Errorf("(%d,%d) shows %q but found cell (%d,%d)", x, y, r, gx, gy)
			}
		}
	}
	if checked < 40 {
		t.Errorf("only %d legible digits were on screen", checked)
	}
}

// On the flat grid the space between digits belongs to a cell. From an angle
// the gaps are the projection's, and the flat mapping would pick a cell nowhere
// near the pointer.
func TestAClickOnNothingFindsNothing(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	enter3D(t, m)
	drawTo(m)

	// Again off the screen: a cell the renderer left blank must answer nothing.
	buf := drawTo(m)
	panel := m.gridPanel()
	empty, wrong := 0, 0
	for y := panel.Y; y < panel.Y+panel.H; y++ {
		for x := panel.X; x < panel.X+panel.W; x++ {
			if buf.RuneAt(x, y) != ' ' {
				continue
			}
			empty++
			if _, _, ok := m.HitTest(x, y); ok {
				wrong++
			}
		}
	}
	if empty == 0 {
		t.Skip("the projection filled the panel")
	}
	if wrong != 0 {
		t.Errorf("%d of %d empty cells named a digit anyway", wrong, empty)
	}
}

// The mass is the file continuing past the viewport and behind it, drawn faint,
// with no cell behind it. Corruption you could see but not reach would be worse
// than none, so there is none out there.
func TestTheMassAnswersNoClick(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	enter3D(t, m)
	buf := drawTo(m)

	shade := map[canvas.Attr]bool{}
	for _, a := range m.solid.shades(m.Theme) {
		shade[a] = true
	}

	panel := m.gridPanel()
	seen, wrong := 0, 0
	for y := panel.Y; y < panel.Y+panel.H; y++ {
		for x := panel.X; x < panel.X+panel.W; x++ {
			if r := buf.RuneAt(x, y); r < '0' || r > '9' {
				continue
			}
			if !shade[buf.AttrAt(x, y)] {
				continue
			}
			seen++
			if _, _, ok := m.HitTest(x, y); ok {
				wrong++
			}
		}
	}
	if seen < 100 {
		t.Fatalf("only %d digits of the mass were on screen; it is not being drawn", seen)
	}
	if wrong != 0 {
		t.Errorf("%d of %d digits of the mass named a cell", wrong, seen)
	}
}

// The file bar, the bins and the footer are the terminal around the grid; this
// replaces the grid alone.
func TestTheChromeSurvivesTheThirdDimension(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	enter3D(t, m)

	buf := drawTo(m)
	var all string
	for y := 0; y < m.Layout.Height; y++ {
		all += buf.Row(y) + "\n"
	}
	if !strings.Contains(all, m.FileName) {
		t.Errorf("the chrome lost %q", m.FileName)
	}
	if m.header.pct.empty() {
		t.Error("the chrome lost the completion")
	}
}

func TestTypingItAgainGoesBackToTheFlatBoard(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	enter3D(t, m)

	m.Update(key("3"))
	m.Update(key("d"))
	if m.Solid() {
		t.Fatal("the view did not close")
	}
	if m.transit == nil {
		t.Fatal("leaving was instant")
	}
	if !m.transit.Back() {
		t.Error("leaving set off in the wrong direction")
	}

	frames := 0
	for ; frames < 60*4 && m.transit != nil; frames++ {
		tick(m, 1)
	}
	if frames < 8 {
		t.Errorf("the trip home took %d frames; that is a cut", frames)
	}
	drawTo(m)

	// And the flat board answers clicks by the layout again.
	l := m.Layout
	sx, sy := l.ScreenOf(l.ViewportX+1, l.ViewportY+1)
	gx, gy, ok := m.HitTest(sx, sy)
	if !ok || gx != l.ViewportX+1 || gy != l.ViewportY+1 {
		t.Errorf("back on the flat board a click on (%d,%d) found (%d,%d) ok=%v",
			l.ViewportX+1, l.ViewportY+1, gx, gy, ok)
	}
}

// They put the same cells in different places.
func TestTheClickIndexIsNotSharedBetweenTheTwoViews(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	enter3D(t, m)
	drawTo(m)

	stamped := m.anchorAt
	if !stamped.solid {
		t.Error("the index does not record which view built it")
	}

	m.solid = nil
	if m.anchorAt == m.stampNow() {
		t.Error("an index built from an angle is still trusted on the flat board")
	}
}

// A block is read from its faces, and a face with a third of its sites occupied
// has a ragged silhouette and reads as a haze, so every site holds a digit.
func TestEverySiteOfTheLatticeIsOccupied(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	enter3D(t, m)
	tick(m, 90)

	l := m.Layout
	seen := map[[3]int]int{}
	for plane := 0; plane < solidLayers; plane++ {
		for gy := l.ViewportY - solidMargin; gy < l.ViewportY+l.ViewportH+solidMargin; gy++ {
			for gx := l.ViewportX - solidMargin; gx < l.ViewportX+l.ViewportW+solidMargin; gx++ {
				seen[[3]int{gx, gy, plane}]++
			}
		}
	}

	// Asked of the two builders, not the screen, because the projection drops
	// whatever leaves the panel, which is what running off the edges means.
	m.solid.aim(m, m.gridPanel())
	buf := canvas.NewBuffer(l.Width, l.Height)
	m.drawSolidMass(buf, m.gridPanel())

	want := len(seen)
	// The frontmost plane leaves the viewport to the board, which draws it.
	want -= l.ViewportW * l.ViewportH
	if got := len(m.solid.mass); got != want {
		t.Errorf("the mass holds %d digits; the lattice has %d sites", got, want)
	}

	face := 0
	for range m.solid.shots(m, m.gridPanel()) {
		face++
	}
	if face == 0 {
		t.Fatal("the live face is empty")
	}
}

// Every visible cell is on the one front plane, so the four edges are straight
// lines of digits.
func TestTheLiveFaceIsOneCompleteRectangle(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	enter3D(t, m)
	tick(m, 90)

	m.solid.aim(m, m.gridPanel())

	// Nothing may be behind anything else: a cell at a depth of its own makes the
	// face ragged. A woken cluster is the exception: it stands proud, which a flat
	// board cannot show.
	depths := map[float64]int{}
	for _, pt := range latticeFace(m) {
		depths[pt]++
	}
	if len(depths) > 2 {
		t.Errorf("the live face is spread over %d depths; it is a sheet, not a face", len(depths))
	}
	if _, ok := depths[solidFront]; !ok {
		t.Error("no cell is on the front plane")
	}
}

// latticeFace is the depth of every cell of the live face, with a cluster's
// shake taken off so a woken one reads as a single lifted depth.
func latticeFace(m *Model) []float64 {
	l := m.Layout
	out := make([]float64, 0, l.ViewportW*l.ViewportH)
	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			n := m.Grid.At(gx, gy)
			if n == nil || m.Flights.InFlight(n.ID) {
				continue
			}
			if m.IsActive(n.BadGroupID) {
				out = append(out, solidFront+solidLift)
				continue
			}
			out = append(out, solidFront)
		}
	}
	return out
}

// The mass is the board's, not this session's: a pure function of the seed, the
// file and the site, so two refiners agree with nothing exchanged, as they do
// about the drift.
func TestTwoRefinersSeeTheSameVolume(t *testing.T) {
	for _, at := range [][3]int{{0, 0, 0}, {3, 7, 2}, {41, 19, 5}, {-4, 103, 7}} {
		a := solidDigit(1234, 2, at[0], at[1], at[2])
		b := solidDigit(1234, 2, at[0], at[1], at[2])
		if a != b {
			t.Errorf("site (%d,%d,%d) is %q and then %q", at[0], at[1], at[2], a, b)
		}
		if a < '0' || a > '9' {
			t.Errorf("site (%d,%d,%d) is %q, which is not a digit", at[0], at[1], at[2], a)
		}
	}

	// Otherwise the volume is one picture repeated backwards into the screen.
	// Counted, since two planes agreeing at one site means nothing.
	agree := 0
	for gy := 0; gy < 50; gy++ {
		for gx := 0; gx < 50; gx++ {
			if solidDigit(11, 0, gx, gy, 0) == solidDigit(11, 0, gx, gy, 1) {
				agree++
			}
		}
	}
	if agree > 2500/5 {
		t.Errorf("two planes agree at %d of 2,500 sites; they are the same picture", agree)
	}

	spread := map[rune]int{}
	sites := 0
	for plane := 0; plane < solidLayers; plane++ {
		for gy := 0; gy < 40; gy++ {
			for gx := 0; gx < 40; gx++ {
				spread[solidDigit(99, 0, gx, gy, plane)]++
				sites++
			}
		}
	}
	if len(spread) != 10 {
		t.Errorf("the mass uses %d of the ten digits", len(spread))
	}
	// Roughly even, or the mass has a texture. Counted off the sites walked, since
	// the number of planes gets tuned.
	even := float64(sites) / 10
	for d, n := range spread {
		if float64(n) < even*0.75 || float64(n) > even*1.25 {
			t.Errorf("%q appears %d times in %d sites; an even tenth is %.0f",
				d, n, sites, even)
		}
	}
}

// So this is a hash, not the board's noise field: Perlin is smooth, and would
// lay the mass down in visible streaks of fours and fives.
func TestTheMassIsDataAndNotATexture(t *testing.T) {
	same := 0
	const n = 60
	for gy := 0; gy < n; gy++ {
		for gx := 0; gx < n-1; gx++ {
			if solidDigit(7, 0, gx, gy, 3) == solidDigit(7, 0, gx+1, gy, 3) {
				same++
			}
		}
	}
	// A tenth of neighbouring pairs agree by chance. Twice that is a field with
	// structure in it.
	if total := n * (n - 1); same > total/5 {
		t.Errorf("%d of %d neighbouring sites hold the same digit", same, total)
	}
}

// A prism seen from outside keeps its back face inside its front face's
// outline, so depth alone makes an object sitting in a frame. The margin makes
// it part of something larger.
func TestTheMassReachesPastTheBoard(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	enter3D(t, m)
	tick(m, 90)

	l := m.Layout
	m.solid.aim(m, m.gridPanel())
	buf := canvas.NewBuffer(l.Width, l.Height)
	m.drawSolidMass(buf, m.gridPanel())

	lo, hi := 1<<30, -1
	for _, pt := range m.solid.mass {
		gx := int(math.Round(pt.X/CellWidth + m.solid.atX.Value()))
		lo, hi = mini(lo, gx), maxi(hi, gx)
	}
	if lo > l.ViewportX-solidMargin || hi < l.ViewportX+l.ViewportW+solidMargin-1 {
		t.Errorf("the mass spans columns %d..%d; the board is %d..%d",
			lo, hi, l.ViewportX, l.ViewportX+l.ViewportW-1)
	}
}

// Size says it on the flat board; depth is what a flat board cannot say.
func TestAWokenClusterStandsOutOfTheFile(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	for i := 0; i < 60*40 && m.FirstActive() == nil; i++ {
		tick(m, 1)
	}
	grp := m.FirstActive()
	if grp == nil {
		t.Skip("nothing woke up")
	}
	enter3D(t, m)

	member := map[int]bool{}
	for _, id := range grp.NumberIDs {
		member[id] = true
	}

	// Column by column. Depth on a Shot is measured after the camera turns, and yaw
	// mixes x into it: across this face at twenty degrees that is thirty-odd units
	// of spread against a lift of fifteen, so an ordinary digit at the near edge is
	// closer to the eye than a cluster member at the far one. Within one column the
	// yaw adds the same to every cell, leaving the lift.
	plain := map[int]float64{}
	worst := map[int]float64{}
	for _, s := range m.solid.shots(m, m.gridPanel()) {
		gx := s.Tag % GridSize
		if member[s.Tag] {
			if d, ok := worst[gx]; !ok || s.Depth < d {
				worst[gx] = s.Depth
			}
			continue
		}
		if d, ok := plain[gx]; !ok || s.Depth > d {
			plain[gx] = s.Depth
		}
	}
	if len(worst) == 0 {
		t.Skip("no member of the cluster is on screen")
	}
	for gx, deep := range worst {
		near, ok := plain[gx]
		if !ok {
			continue
		}
		if deep <= near {
			t.Errorf("in column %d the furthest cluster member is at %.1f and the nearest"+
				" ordinary digit at %.1f; corruption is not standing out", gx, deep, near)
		}
	}
}

// A cluster that trembles only sideways inside a volume is flat.
func TestAShakingClusterShakesInDepthToo(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	for i := 0; i < 60*40 && m.FirstActive() == nil; i++ {
		tick(m, 1)
	}
	grp := m.FirstActive()
	if grp == nil {
		t.Skip("nothing woke up")
	}
	enter3D(t, m)

	member := map[int]bool{}
	for _, id := range grp.NumberIDs {
		member[id] = true
	}
	depthOf := func() (float64, bool) {
		for _, s := range m.solid.shots(m, m.gridPanel()) {
			if member[s.Tag] {
				return s.Depth, true
			}
		}
		return 0, false
	}

	first, ok := depthOf()
	if !ok {
		t.Skip("no member on screen")
	}
	moved := false
	for i := 0; i < 60*4 && !moved; i++ {
		tick(m, 1)
		if z, ok := depthOf(); ok && math.Abs(z-first) > 0.5 {
			moved = true
		}
	}
	if !moved {
		t.Error("a woken cluster held exactly still in depth for four seconds")
	}
}

// Smaller the further into the file, which is what a Shot's perspective factor
// is for.
func TestACorruptDigitIsDrawnLargerNearerAndSmallerFurther(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	for i := 0; i < 60*40 && m.FirstActive() == nil; i++ {
		tick(m, 1)
	}
	if m.FirstActive() == nil {
		t.Skip("nothing woke up")
	}
	enter3D(t, m)
	tick(m, 120)
	drawTo(m)

	// A shot's near factor sizes it, and has to vary or depth says nothing. Asked
	// of the full volume: the live face sits at the front, in front of the world
	// origin, and the mass behind brackets it.
	panel := m.gridPanel()
	m.solid.aim(m, panel)
	buf := canvas.NewBuffer(m.Layout.Width, m.Layout.Height)
	m.drawSolidMass(buf, panel)

	near, far := 0.0, math.Inf(1)
	for _, s := range m.solid.cam.Cast(m.solid.mass, panel) {
		near, far = math.Max(near, s.Near), math.Min(far, s.Near)
	}
	if near < 1 || far > 1 {
		t.Errorf("the volume's near factor spans %.3f to %.3f; nothing is in front of"+
			" the eye's plane or nothing behind it", far, near)
	}

	// The live face is sized by distance too, so a cluster at the far side of the
	// file is drawn smaller than one under your nose.
	lo, hi := math.Inf(1), 0.0
	for _, s := range m.solid.shots(m, panel) {
		lo, hi = math.Min(lo, s.Near), math.Max(hi, s.Near)
	}
	if hi <= lo {
		t.Fatalf("every digit of the face came back at the same perspective factor, %.3f", hi)
	}
}

// The fit is centred on the middle of the block, so aiming at the cursor would
// drop whichever edge it leans away from, and hide woken clusters there. The
// camera keys move the view instead.
func TestTheViewFollowsTheBlockAndGlidesAfterIt(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	enter3D(t, m)
	tick(m, 60)

	// Moving the cursor inside the viewport does not move the aim: the block has
	// not moved.
	held := [2]float64{m.solid.atX.Value(), m.solid.atY.Value()}
	m.Update(key("right"))
	tick(m, 30)
	if m.solid.atX.Value() != held[0] || m.solid.atY.Value() != held[1] {
		t.Error("the view moved for a cursor step that did not scroll the block")
	}

	// Scrolling the viewport does move it, and it takes a while to get there.
	before := m.solid.atY.Value()
	was := m.Layout.ViewportY
	for i := 0; i < 40 && m.Layout.ViewportY == was; i++ {
		m.Update(key("down"))
		tick(m, 1)
	}
	if m.Layout.ViewportY == was {
		t.Skip("the viewport never scrolled")
	}
	tick(m, 1)
	moved := m.solid.atY.Value()
	want, _ := m.solidCentre()
	_ = want
	if moved == before {
		t.Error("the view had not begun to follow a frame after the block moved")
	}
	_, target := m.solidCentre()
	if math.Abs(moved-target) < 1e-9 {
		t.Error("the view arrived instantly; it is meant to glide")
	}
	tick(m, 120)
	_, target = m.solidCentre()
	if math.Abs(m.solid.atY.Value()-target) > 1e-2 {
		t.Errorf("two seconds on, the view is at %.3f and the block at %.3f",
			m.solid.atY.Value(), target)
	}
}

// solidLift must stay well short of the eye distance: at 90 against 108 the
// perspective factor is six, and most of a woken cluster lands outside the
// panel.
func TestAWokenClusterIsActuallyOnTheScreen(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	for i := 0; i < 60*40 && m.FirstActive() == nil; i++ {
		tick(m, 1)
	}
	grp := m.FirstActive()
	if grp == nil {
		t.Skip("nothing woke up")
	}
	enter3D(t, m)
	tick(m, 120)

	member := map[int]bool{}
	for _, id := range grp.NumberIDs {
		member[id] = true
	}

	// The camera drops members that are off the panel, so they must not all be
	// dropped.
	panel := m.gridPanel()
	seen, inside := 0, 0
	nearest := 0.0
	for _, s := range m.solid.shots(m, panel) {
		if !member[s.Tag] {
			continue
		}
		seen++
		if panel.Contains(s.Col, s.Row) {
			inside++
		}
		if s.Near > nearest {
			nearest = s.Near
		}
	}
	if seen == 0 {
		t.Skip("no member of the cluster is in the visible block")
	}
	if inside != seen {
		t.Errorf("%d of %d cluster members landed outside the panel", seen-inside, seen)
	}

	// Standing proud of the file, not most of the way to the eye.
	if nearest <= 1 {
		t.Errorf("the nearest cluster member is at a factor of %.2f; it is not standing out", nearest)
	}
	if nearest > solidNearest+0.4 {
		t.Errorf("the nearest cluster member is magnified %.1f times; it is being flung at the lens",
			nearest)
	}
}

// A cluster on the back plane lifted by one plane would still be behind every
// ordinary digit at the front.
func TestCorruptionIsInFrontOfEveryPlane(t *testing.T) {
	if solidFront+solidLift <= solidFront {
		t.Fatal("the lift does not put a cluster in front of the frontmost plane")
	}
	for at := 0; at < solidLayers; at++ {
		depth := (float64(at) - float64(solidLayers-1)/2) * solidLayerGap
		if depth >= solidFront+solidLift {
			t.Errorf("plane %d is at %.1f, level with or in front of a cluster at %.1f",
				at, depth, solidFront+solidLift)
		}
	}
}

// Fitting the block to the panel is what a picture does; this is a place, and
// you cannot fly into something the renderer keeps pulling back.
func TestFlyingChangesHowMuchOfTheFileIsOnScreen(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	enter3D(t, m)
	tick(m, 60)

	// Counted, not measured across: the face fills most of the panel at rest, so a
	// screen span saturates as soon as you fly in, since Shots drops whatever leaves
	// the panel and both edges leave together.
	showing := func() int { return len(m.solid.shots(m, m.gridPanel())) }

	at := showing()
	for i := 0; i < 5; i++ {
		m.Update(key("+"))
	}
	tick(m, 120)
	in := showing()
	if in >= at {
		t.Errorf("flying in left %d cells on screen, from %d", in, at)
	}

	for i := 0; i < 12; i++ {
		m.Update(key("-"))
	}
	tick(m, 180)
	if out := showing(); out <= in {
		t.Errorf("flying out left %d cells on screen, from %d", out, in)
	}

	// And nothing is turned inside out by passing through the eye.
	for i := 0; i < 60; i++ {
		m.Update(key("+"))
		tick(m, 4)
		for _, s := range m.solid.shots(m, m.gridPanel()) {
			if s.Near <= 0 {
				t.Fatalf("a digit came back at a perspective factor of %.3f", s.Near)
			}
		}
	}
}

// The camera keys steer without disturbing the game, and the game's own keys
// hold the tour down while the board is being worked.
func TestTheCameraAndTheGameShareTheKeyboard(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	enter3D(t, m)

	x, y, bin := m.CursorX, m.CursorY, m.SelectedBin
	for _, k := range []string{"+", "-", "[", "]", ",", "."} {
		m.Update(key(k))
	}
	tick(m, 30)
	if m.CursorX != x || m.CursorY != y || m.SelectedBin != bin {
		t.Error("steering the camera moved the cursor or the bin")
	}
	if !m.Solid() {
		t.Fatal("steering the camera left the view")
	}

	// Left alone it tours; the cursor stops it.
	for i := 0; i < int(60*(rigRest+1)); i++ {
		tick(m, 1)
	}
	if !m.solid.Rig.Spinning() {
		t.Fatal("the view never began to turn")
	}
	m.Update(key("right"))
	tick(m, 1)
	if m.solid.Rig.Spinning() {
		t.Error("moving the cursor did not hold the view still")
	}
}

// The Solid is built when the key is pressed and the journey takes most of a
// second, so the spring is held down until the trip in is over; started at
// construction, it would settle before the volume is first drawn.
func TestTheFileOpensOutBehindTheBoardOnArrival(t *testing.T) {
	m := ready()
	m.Resize(120, 34)

	m.Update(key("3"))
	m.Update(key("d"))
	if m.solid == nil {
		t.Fatal("the view did not open")
	}

	// Through the journey the mass stays flat against the board, however long it
	// takes.
	for i := 0; i < 60*4 && m.transit != nil; i++ {
		tick(m, 1)
		if got := m.solid.spread.Value(); got != 0 {
			t.Fatalf("the mass had spread to %.3f before the digits landed", got)
		}
	}
	if m.transit != nil {
		t.Fatal("the trip never finished")
	}

	// Then it opens, slowly enough to be seen doing it.
	frames := 0
	for ; frames < 60*4 && m.solid.spread.Value() < 0.9; frames++ {
		tick(m, 1)
	}
	if frames < 6 {
		t.Errorf("the file opened out in %d frames; that is a cut", frames)
	}
	if m.solid.spread.Value() < 0.9 {
		t.Error("the file never opened out")
	}

	// Flat against the board means every plane on the front.
	if z := solidPlaneZ(0, 0); z != solidFront {
		t.Errorf("at rest the deepest plane is at %.1f, not on the board at %.1f",
			z, solidFront)
	}
	if z := solidPlaneZ(0, 1); z >= solidFront {
		t.Errorf("opened out, the deepest plane is at %.1f and not behind %.1f",
			z, solidFront)
	}
}

// Flights must be drawn in the volume too, or the cluster vanishes and the bin
// lights up as receiving with nothing crossing the screen.
func TestRefiningFromAnAngleSendsTheDigitsSomewhereVisible(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	enter3D(t, m)
	if m.FirstActive() == nil {
		t.Skip("the cluster was refined on the way in")
	}

	m.refine(m.FirstActive())
	tick(m, 20)
	if len(m.Flights.Active()) == 0 {
		t.Fatal("nothing was launched")
	}

	// They are drawn, and inside the terminal.
	buf := drawTo(m)
	w, h := buf.Size()
	drawn := 0
	for _, f := range m.Flights.Active() {
		x, y := int(f.X+0.5), int(f.Y+0.5)
		if x < 0 || y < 0 || x >= w || y >= h {
			t.Errorf("a digit is flying off the terminal at (%d,%d)", x, y)
			continue
		}
		if buf.RuneAt(x, y) >= '0' && buf.RuneAt(x, y) <= '9' {
			drawn++
		}
	}
	if drawn == 0 {
		t.Error("no airborne digit was painted onto the buffer")
	}
}

// The flat layout's position is nowhere near the projection.
func TestAFlightFromAnAngleLeavesFromTheProjectedDigit(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	enter3D(t, m)
	if m.FirstActive() == nil {
		t.Skip("the cluster was refined on the way in")
	}
	grp = m.FirstActive()

	at := m.solidAnchors()
	if len(at) == 0 {
		t.Fatal("the volume has no digits")
	}

	m.refine(grp)
	live := m.Flights.Active()
	if len(live) == 0 {
		t.Fatal("nothing was launched")
	}

	// Every launch starts within a cell or two of where the volume drew it.
	// Compared before the flight has travelled, so this is about the origin.
	checked := 0
	for _, f := range live {
		want, ok := at[f.ID]
		if !ok {
			continue
		}
		checked++
		if d := math.Hypot(f.X-want.X, f.Y-want.Y); d > 3*CellWidth {
			t.Errorf("digit %d left from (%.0f,%.0f); the volume drew it at (%.0f,%.0f)",
				f.ID, f.X, f.Y, want.X, want.Y)
		}
	}
	if checked == 0 {
		t.Skip("no member of the cluster was on screen")
	}
}
