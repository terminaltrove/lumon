package lumon

import (
	"math"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/terminaltrove/lumon/internal/physics"
)

// startDiversionNamed opens the menu and picks one by name.
func startDiversionNamed(t *testing.T, m *Model, name string) {
	t.Helper()
	typeWord(m, theWord)
	for i, d := range diversions {
		if d.Name == name {
			m.divertAt = i
		}
	}
	m.Update(key("enter"))
	arrive(t, m)
}

// Loosening what was visible makes it read as the screen coming apart.
func TestTheDigitsThatComeLooseAreTheOnesOnScreen(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	l := m.Layout
	want := l.ViewportW * l.ViewportH

	startDiversionNamed(t, m, "SCATTER")
	if m.sandbox == nil {
		t.Fatalf("SCATTER did not start; screen is %v", m.Screen)
	}
	if got := m.sandbox.Len(); got != want {
		t.Errorf("%d digits came loose, want the %d on screen", got, want)
	}
}

// Throwing every digit outward from the centre empties the middle within a
// second and packs everything against the walls, so each digit gets its own
// direction. Measured early, while still moving: the drift is damped and comes
// to rest within a few seconds.
func TestScatteringFillsTheScreenRatherThanTheWalls(t *testing.T) {
	m := ready()
	m.Resize(100, 28)
	startDiversionNamed(t, m, "SCATTER")
	tick(m, 45)

	l := m.Layout
	var middle, edges int
	for i := 0; i < m.sandbox.Len(); i++ {
		x := m.sandbox.world.Pos(i).X
		switch {
		case x < float64(l.Width)*0.25 || x > float64(l.Width)*0.75:
			edges++
		default:
			middle++
		}
	}
	if middle == 0 {
		t.Fatal("the middle of the screen is empty; this is an explosion, not a scatter")
	}
	// The middle is half the width, so it should hold roughly half of them.
	if float64(middle) < float64(edges)*0.4 {
		t.Errorf("%d digits in the middle against %d at the edges; they are piling against the walls",
			middle, edges)
	}
}

// They land, and they land on each other.
func TestDescendingPilesUpRatherThanLandingInARow(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "DESCENT")

	n := m.sandbox.Len()
	if n < 100 {
		t.Skipf("only %d digits; too few to need stacking", n)
	}
	tick(m, 600)

	floor := m.sandbox.bounds.Max.Y
	rows := map[int]int{}
	for i := 0; i < n; i++ {
		p := m.sandbox.world.Pos(i)
		if p.Y > floor+0.01 {
			t.Fatalf("digit %d ended at y=%v, below the floor at %v", i, p.Y, floor)
		}
		rows[int(math.Round(p.Y/RowAspect))]++
	}
	if len(rows) < 3 {
		t.Errorf("everything landed in %d row(s); a hundred and thirty digits should stack", len(rows))
	}

	// And nothing may have ended up inside anything else.
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if d := m.sandbox.world.Pos(i).Sub(m.sandbox.world.Pos(j)).Len(); d < 1.4 {
				t.Fatalf("digits %d and %d ended %v apart, inside each other", i, j, d)
			}
		}
	}
}

// Without a sideways nudge they fall in their grid columns, six columns apart
// (three contact diameters), and land as the grid again, one row lower.
func TestTheFallSpreadsRatherThanDroppingTheGrid(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "DESCENT")

	before := map[int]bool{}
	for i := 0; i < m.sandbox.Len(); i++ {
		before[int(math.Round(m.sandbox.world.Pos(i).X))] = true
	}
	tick(m, 600)
	after := map[int]bool{}
	for i := 0; i < m.sandbox.Len(); i++ {
		after[int(math.Round(m.sandbox.world.Pos(i).X))] = true
	}

	if len(after) <= len(before) {
		t.Errorf("the digits occupied %d columns and now occupy %d; they fell as rigid columns",
			len(before), len(after))
	}
}

// This is what Grab does that a Pin cannot.
func TestADraggedDigitFollowsThePointerAndKeepsItsMomentum(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "DESCENT")
	tick(m, 400) // let it settle

	// Pick up whatever is nearest the middle of the heap.
	sb := m.sandbox
	target := physics.Vec2{X: sb.bounds.Max.X / 2, Y: sb.bounds.Max.Y}
	i := physics.Nearest(sb.world, target, 1e9)
	p := sb.world.Pos(i)

	col, row := int(math.Round(p.X)), int(math.Round(p.Y/RowAspect))
	m.Update(tea.MouseMsg{X: col, Y: row, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if !sb.Holding() {
		t.Fatal("clicking on a digit picked nothing up")
	}

	// Drag it up and to the right.
	for k := 1; k <= 8; k++ {
		m.Update(tea.MouseMsg{X: col + k, Y: row - 4, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft})
		tick(m, 1)
	}
	held := sb.world.Pos(i)
	if math.Abs(held.X-float64(col+8)) > 1.5 {
		t.Errorf("the held digit is at x=%v, not under the pointer at %d", held.X, col+8)
	}

	// Let go mid-flick: it should keep moving.
	m.Update(tea.MouseMsg{X: col + 8, Y: row - 4, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft})
	if sb.Holding() {
		t.Fatal("releasing did not let go")
	}
	x0 := sb.world.Pos(i).X
	tick(m, 6)
	if sb.world.Pos(i).X <= x0+0.2 {
		t.Errorf("a thrown digit went from x=%v to %v; it was released dead", x0, sb.world.Pos(i).X)
	}
}

// r starts it again from wherever the board is now; anything else goes back to
// work and lets the simulation go.
func TestADiversionRestartsAndEnds(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "SCATTER")
	tick(m, 120)

	first := m.sandbox
	m.Update(key("r"))
	arrive(t, m)
	if m.sandbox == first {
		t.Error("r did not start the diversion again")
	}
	if m.sandbox == nil {
		t.Errorf("r left the diversion for %v", m.Screen)
	}

	m.Update(key("x"))
	arrive(t, m)
	if m.Diverting() {
		t.Error("a stray key did not end the diversion")
	}
	if m.sandbox != nil {
		t.Error("leaving a diversion kept its simulation running")
	}
}

// A resize shuffles the heap against its new walls instead of sweeping it off
// the screen.
func TestADiversionSurvivesAResize(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	startDiversionNamed(t, m, "DESCENT")
	tick(m, 400)
	n := m.sandbox.Len()

	m.Resize(70, 20)
	tick(m, 200)

	if m.sandbox == nil {
		t.Fatal("a resize ended the diversion")
	}
	if got := m.sandbox.Len(); got != n {
		t.Errorf("%d digits survived a resize, want all %d", got, n)
	}
	for i := 0; i < m.sandbox.Len(); i++ {
		p := m.sandbox.world.Pos(i)
		if p.X < -0.01 || p.X > m.sandbox.bounds.Max.X+0.01 ||
			p.Y < -0.01 || p.Y > m.sandbox.bounds.Max.Y+0.01 {
			t.Fatalf("digit %d is at %v, outside the resized box", i, p)
		}
	}
}

// Nothing piles up over the line telling you how to stop.
func TestTheHeapStaysOffTheHintRow(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "DESCENT")
	tick(m, 600)

	// The hint is on the bottom row of the panel, not the terminal.
	panel := m.gridPanel()
	buf := drawTo(m)
	row := buf.Row(panel.Y + panel.H - 1)
	for i := 0; i < m.sandbox.Len(); i++ {
		p := m.sandbox.world.Pos(i)
		if int(p.Y/RowAspect) >= panel.Y+panel.H-1 {
			t.Errorf("a digit reached the hint row:\n%q", row)
			break
		}
	}
}

// World.Step takes one shared gravity vector, so everything falls at the same
// rate, and a screenful falling in lockstep reads as a sheet coming down. Each
// digit needs its own pull.
func TestFallingDigitsDoNotFallInLockstep(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "DESCENT")

	// They start on the grid's own rows, so measure the change in depth.
	start := make([]float64, m.sandbox.world.Len())
	for i := range start {
		start[i] = m.sandbox.world.Pos(i).Y
	}
	tick(m, 30)

	lo, hi := math.Inf(1), math.Inf(-1)
	for i := range start {
		d := m.sandbox.world.Pos(i).Y - start[i]
		lo, hi = math.Min(lo, d), math.Max(hi, d)
	}
	if hi-lo < 2 {
		t.Errorf("after half a second the fastest and slowest have fallen %v apart; "+
			"they are coming down as a sheet", hi-lo)
	}
}

// Each digit's pull is its own, and is not noise added per frame.
func TestEachDigitKeepsItsOwnGravity(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "DESCENT")

	seen := map[float64]bool{}
	for _, g := range m.sandbox.gravity {
		seen[g.Y] = true
		if g.Y <= 0 {
			t.Fatalf("a digit was given a pull of %v", g.Y)
		}
	}
	if len(seen) < len(m.sandbox.gravity)/2 {
		t.Errorf("%d distinct pulls across %d digits", len(seen), len(m.sandbox.gravity))
	}

	// And it does not change under it.
	was := append([]physics.Vec2(nil), m.sandbox.gravity...)
	tick(m, 120)
	for i := range was {
		if m.sandbox.gravity[i] != was[i] {
			t.Fatalf("digit %d's pull changed from %v to %v", i, was[i], m.sandbox.gravity[i])
		}
	}
}

func TestADoubleTapScattersWhatIsNearIt(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "DESCENT")
	tick(m, 400) // let it settle into a heap

	p := m.gridPanel()
	bx, by := p.X+p.W/2, p.Y+p.H-2
	at := physics.Vec2{X: float64(bx), Y: float64(by) * RowAspect}

	before := make([]physics.Vec2, m.sandbox.world.Len())
	for i := range before {
		before[i] = m.sandbox.world.Pos(i)
	}

	m.sandbox.Blast(bx, by)
	tick(m, 25)

	var near, far float64
	var nn, nf int
	for i := range before {
		d := before[i].Sub(at).Len()
		moved := m.sandbox.world.Pos(i).Sub(before[i]).Len()
		switch {
		case d <= blastReach/2:
			near, nn = near+moved, nn+1
		case d > blastReach*1.5:
			far, nf = far+moved, nf+1
		}
	}
	if nn == 0 || nf == 0 {
		t.Skip("the heap is not spread enough to have both near and far digits")
	}
	if near/float64(nn) < 3 {
		t.Errorf("digits near the tap moved %v on average; the shove did nothing", near/float64(nn))
	}
	if far/float64(nf) > near/float64(nn)/4 {
		t.Errorf("digits far from the tap moved %v against %v near it; the falloff is not working",
			far/float64(nf), near/float64(nn))
	}
}

// Two presses close together in time and place are a double tap; spread out in
// either, they are two ordinary presses.
func TestWhatCountsAsADoubleTap(t *testing.T) {
	at := time.Now()
	fresh := func() *Sandbox {
		m := ready()
		m.Resize(110, 30)
		startDiversionNamed(t, m, "DESCENT")
		s := m.sandbox
		s.now = func() time.Time { return at }
		return s
	}

	s := fresh()
	if s.Tap(20, 10) {
		t.Error("a first tap counted as a double")
	}
	if !s.Tap(20, 10) {
		t.Error("a second tap in the same place was not a double")
	}
	if s.Tap(20, 10) {
		t.Error("a third tap fired a second blast; a triple is not two doubles")
	}

	s = fresh()
	s.Tap(20, 10)
	s.now = func() time.Time { return at.Add(2 * blastWindow) }
	if s.Tap(20, 10) {
		t.Error("two taps a long way apart in time counted as a double")
	}

	s = fresh()
	s.Tap(20, 10)
	if s.Tap(60, 10) {
		t.Error("two taps a long way apart on screen counted as a double")
	}
}

// A lossless drift (damping one, elastic walls, no contacts) conserves energy
// and stays frantic for ever. It should open up and then come to rest.
func TestAScatteredBoardSettles(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "SCATTER")

	speed := func() float64 {
		v := 0.0
		for i := 0; i < m.sandbox.world.Len(); i++ {
			v += m.sandbox.world.Velocity(i, physics.DT).Len()
		}
		return v / float64(m.sandbox.world.Len())
	}

	start := speed()
	if start < 5 {
		t.Fatalf("the board set off at %v; it is meant to open with some force", start)
	}
	tick(m, 60*8)
	if got := speed(); got > start/10 {
		t.Errorf("after eight seconds the mean speed is %v against %v at the start; "+
			"it should have settled", got, start)
	}
}

// Collide uses a single radius, so a larger glyph's ink would sit on neighbours
// its centre cleared.
func TestScatteredDigitsAreAllPlain(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	startDiversionNamed(t, m, "SCATTER")
	for i, h := range m.sandbox.height {
		if h != PlainHeight {
			t.Fatalf("digit %d is %v tall; the drift is plain characters", i, h)
		}
	}
}
