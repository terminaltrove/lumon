package lumon

import (
	"math"
	"strings"
	"testing"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// bouncing is a terminal watching the mark.
func bouncing(t *testing.T, w, h int) *Model {
	t.Helper()
	m := ready()
	m.Resize(w, h)
	typeWord(m, theWord)
	found := false
	for i, d := range diversions {
		if d.Name == "PERPETUITY" {
			m.divertAt, found = i, true
		}
	}
	if !found {
		t.Fatal("PERPETUITY is not in the menu")
	}
	m.Update(key("enter"))
	arrive(t, m)
	tick(m, 30)
	if m.mark == nil {
		t.Fatal("the mark is not running")
	}
	return m
}

// SmallLogo is pre-rendered runes, and this is the one place they are turned
// back into dots. Ranging a string yields byte offsets and a braille code point
// is three bytes, so getting it wrong makes the mark three times its width.
func TestTheMarkIsTheRightShape(t *testing.T) {
	dots, w, h := markDots()
	if len(dots) < 100 {
		t.Fatalf("the mark decoded to %d dots; it is a logo, not a smudge", len(dots))
	}
	if want := len([]rune(SmallLogo[0])) * canvas.DotsX; w != want {
		t.Errorf("the mark decoded to %d dots wide, want %d", w, want)
	}
	if want := len(SmallLogo) * canvas.DotsY; h != want {
		t.Errorf("the mark decoded to %d dots tall, want %d", h, want)
	}
	for _, d := range dots {
		if d.x < 0 || d.x >= w || d.y < 0 || d.y >= h {
			t.Fatalf("a dot at (%d,%d) is outside a mark %dx%d", d.x, d.y, w, h)
		}
	}
}

// drawnDots reads the mark back off the screen as dots relative to its own
// top-left, so a drawn mark can be compared with the asset wherever it is.
func drawnDots(t *testing.T, m *Model) (dots map[[2]int]bool, top int) {
	t.Helper()
	buf := drawTo(m)
	panel := m.gridPanel()

	dots = map[[2]int]bool{}
	minX, minY := 1<<30, 1<<30
	for y := panel.Y; y < panel.Y+panel.H; y++ {
		for x, r := range []rune(buf.Row(y)) {
			if r < 0x2800 || r > 0x28FF {
				continue
			}
			mask := uint8(r - canvas.BrailleBlank)
			for dx := 0; dx < canvas.DotsX; dx++ {
				for dy := 0; dy < canvas.DotsY; dy++ {
					if mask&(1<<canvas.DotBit(dx, dy)) == 0 {
						continue
					}
					px := (x-panel.X)*canvas.DotsX + dx
					py := (y-panel.Y)*canvas.DotsY + dy
					dots[[2]int{px, py}] = true
					minX, minY = mini(minX, px), mini(minY, py)
				}
			}
		}
	}
	if len(dots) == 0 {
		t.Fatal("the mark drew nothing at all")
	}

	at := make(map[[2]int]bool, len(dots))
	for d := range dots {
		at[[2]int{d[0] - minX, d[1] - minY}] = true
	}
	return at, minY
}

// Translated by whole dots the mark is the same mark, so this compares it with
// markDots relative to its own corner, which is true at any sub-cell phase.
// Requiring more than one vertical phase stops a cell-grid snap coming back
// (see origin for why there is none).
func TestTheMarkIsDrawnAsItWasAuthored(t *testing.T) {
	m := bouncing(t, 110, 30)

	// Both sides relative to their own corner: the asset's leftmost dot is two in
	// from the edge of its box.
	asset, _, _ := markDots()
	assetX, assetY := 1<<30, 1<<30
	for _, d := range asset {
		assetX, assetY = mini(assetX, d.x), mini(assetY, d.y)
	}
	want := make(map[[2]int]bool, len(asset))
	for _, d := range asset {
		want[[2]int{d.x - assetX, d.y - assetY}] = true
	}

	// Seven frames apart, not thirty-seven. At a third of a dot a frame,
	// thirty-seven frames is nearly a whole number of dots, so samples that far
	// apart land on the same phase and the test asserts almost nothing.
	phases := map[int]bool{}
	for i := 0; i < 48; i++ {
		tick(m, 7)
		got, top := drawnDots(t, m)
		phase := top % canvas.DotsY
		phases[phase] = true

		if len(got) != len(want) {
			t.Fatalf("at phase %d the mark drew %d dots, the asset has %d",
				phase, len(got), len(want))
		}
		for d := range want {
			if !got[d] {
				t.Fatalf("at phase %d the dot at (%d,%d) is missing; the mark came "+
					"apart", phase, d[0], d[1])
			}
		}
	}

	if len(phases) < canvas.DotsY {
		t.Errorf("the mark was only drawn at %d of the %d sub-cell phases; the "+
			"vertical is free and should visit all of them, and a snap is what "+
			"makes it step down the screen instead of sliding", len(phases), canvas.DotsY)
	}
}

// A dot is as fine as braille goes, so a step larger than one is a snap. A step
// in one axis next to a slide in the other reads worse than snapping both.
func TestTheMarkGlidesInBothAxes(t *testing.T) {
	m := bouncing(t, 110, 30)
	p := m.mark

	lx, ly := p.origin()
	var still, wide, rows int
	for i := 0; i < 600; i++ {
		tick(m, 1)
		x, y := p.origin()
		dx, dy := x-lx, y-ly
		if dx > 1 || dx < -1 || dy > 1 || dy < -1 {
			wide++
		}
		if dx == 0 && dy == 0 {
			still++
		}
		if dy != 0 {
			rows++
		}
		lx, ly = x, y
	}

	if wide > 0 {
		t.Errorf("it moved more than a dot on %d frames; both axes are free and it "+
			"should never jump", wide)
	}
	// Free, it moves a quarter of a row at a time on far more frames than a
	// snapped mark's four-dot hops.
	if rows < 120 {
		t.Errorf("it moved vertically on only %d of 600 frames; snapped it managed "+
			"about fifty, and that hop is what read as stepping", rows)
	}
	// At bounceSpeed the mark covers well under a dot a frame, so it cannot land
	// on every one, but it must not stall for most of them.
	if still > 260 {
		t.Errorf("it held still on %d of 600 frames; that is the rigidity this is "+
			"here to catch", still)
	}
}

// Left alone it keeps the speed it started with, for ever.
func TestTheMarkNeverSettles(t *testing.T) {
	m := bouncing(t, 110, 30)
	p := m.mark
	want := math.Hypot(p.vx, p.vy)

	var minX, maxX, minY, maxY float64 = 1e9, -1e9, 1e9, -1e9
	for i := 0; i < 60*30; i++ {
		tick(m, 1)
		minX, maxX = math.Min(minX, p.x), math.Max(maxX, p.x)
		minY, maxY = math.Min(minY, p.y), math.Max(maxY, p.y)
		if got := math.Hypot(p.vx, p.vy); math.Abs(got-want) > 0.01 {
			t.Fatalf("after %d frames it is going %v, started at %v; a bouncing "+
				"mark loses nothing", i, got, want)
		}
	}

	// It got about the box, not pacing one edge. Measured against what the centre
	// can travel (the box less the mark); the mark takes half the box, so half is
	// all there is.
	rx, ry := p.reach()
	if maxX-minX < (p.w-2*rx)*0.8 {
		t.Errorf("it covered %v of the %v it can travel across", maxX-minX, p.w-2*rx)
	}
	if maxY-minY < (p.h-2*ry)*0.8 {
		t.Errorf("it covered %v of the %v it can travel down", maxY-minY, p.h-2*ry)
	}
}

func TestTheMarkBouncesOffEveryEdge(t *testing.T) {
	m := bouncing(t, 110, 30)
	p := m.mark

	var left, right, top, bottom bool
	lastX, lastY := p.vx, p.vy
	for i := 0; i < 60*40; i++ {
		tick(m, 1)
		switch {
		case p.vx > 0 && lastX < 0:
			left = true
		case p.vx < 0 && lastX > 0:
			right = true
		}
		switch {
		case p.vy > 0 && lastY < 0:
			top = true
		case p.vy < 0 && lastY > 0:
			bottom = true
		}
		lastX, lastY = p.vx, p.vy

		rx, ry := p.reach()
		if p.x < rx-0.01 || p.x > p.w-rx+0.01 || p.y < ry-0.01 || p.y > p.h-ry+0.01 {
			t.Fatalf("frame %d: the mark is at (%v,%v), outside a box %vx%v",
				i, p.x, p.y, p.w, p.h)
		}
	}
	if !left || !right || !top || !bottom {
		t.Errorf("it turned at left=%v right=%v top=%v bottom=%v; it should reach all four",
			left, right, top, bottom)
	}
}

// c takes the next colour, and on bounce the mark changes as it hits.
func TestTheColourChangesOnEachBounce(t *testing.T) {
	m := bouncing(t, 110, 30)
	p := m.mark

	if got := p.Attr(m.Theme).FG; got != m.Theme.Primary {
		// The fade is still coming up, so compare what it is heading for.
		if p.hue != hueOff {
			t.Errorf("it starts on hue %v, want off", p.hue)
		}
	}

	m.Update(key("c"))
	if p.hue != hueBounce {
		t.Fatalf("one c left the hue at %v", p.hue)
	}
	tick(m, 120) // let the fade finish, so the colour is the only thing moving

	seen := map[string]bool{}
	was := p.step
	for i := 0; i < 60*40 && len(seen) < 3; i++ {
		tick(m, 1)
		if p.step != was {
			seen[p.Attr(m.Theme).FG.String()] = true
			was = p.step
		}
	}
	if len(seen) < 3 {
		t.Errorf("the mark took %d colours over forty seconds of bouncing; the "+
			"cycle is cyan, dark, cyan, white", len(seen))
	}

	// And again for the roll, which moves whether or not anything is hit.
	m.Update(key("c"))
	if p.hue != hueRoll {
		t.Fatalf("two cs left the hue at %v", p.hue)
	}
	before := p.Attr(m.Theme).FG
	tick(m, 60)
	if p.Attr(m.Theme).FG == before {
		t.Error("the gradient did not roll")
	}
}

// p drops the mark, and p again picks it back up.
func TestPhysicsFallsAndTumblesAndComesBack(t *testing.T) {
	m := bouncing(t, 140, 40)
	p := m.mark

	m.Update(key("p"))
	if !p.physics {
		t.Fatal("p did not turn physics on")
	}

	// It accelerates downward and it turns.
	tick(m, 10)
	if p.vy <= 0 {
		t.Errorf("under gravity it is going %v vertically; down is positive", p.vy)
	}
	angle := p.angle
	tick(m, 30)
	if p.angle == angle {
		t.Error("it is falling without tumbling")
	}

	// It loses height to the floor instead of bouncing for ever.
	fast := math.Hypot(p.vx, p.vy)
	tick(m, 60*8)
	if slow := math.Hypot(p.vx, p.vy); slow >= fast {
		t.Errorf("it is going %v after eight seconds, was %v; a bounce below one "+
			"has to cost it something", slow, fast)
	}

	// And back: constant speed again, and it comes upright.
	m.Update(key("p"))
	if p.physics {
		t.Fatal("p again did not turn physics off")
	}
	tick(m, 120)
	if got := math.Hypot(p.vx, p.vy); math.Abs(got-bounceSpeed) > 0.01 {
		t.Errorf("back to bouncing it is going %v, want %v", got, bounceSpeed)
	}
	if math.Abs(p.angle) > 0.05 {
		t.Errorf("it settled at %v radians off level", p.angle)
	}
}

// f is the one diversion that may leave the panel. The tests that hold every
// other diversion inside it never press f; this checks the chrome is gone when
// it is asked for.
func TestFullScreenTakesTheWholeTerminal(t *testing.T) {
	m := bouncing(t, 110, 30)

	before := screenOf(m)
	for _, want := range []string{"% Complete", "═"} {
		if !strings.Contains(before, want) {
			t.Fatalf("the chrome is not up to begin with: no %q", want)
		}
	}

	m.Update(key("f"))
	if !m.mark.Fullscreen() {
		t.Fatal("f did not go full screen")
	}
	tick(m, 30)

	after := screenOf(m)
	for _, gone := range []string{"% Complete", "═", "01", "LUMON INDUSTRIES"} {
		if strings.Contains(after, gone) {
			t.Errorf("full screen still shows %q", gone)
		}
	}
	if strings.TrimSpace(after) == "" {
		t.Error("full screen shows nothing at all")
	}

	// The box grew with it, or the mark would go on bouncing inside the old one.
	if m.mark.h <= float64(m.Layout.BoardH*4) {
		t.Errorf("the box is %v dots tall against a board of %d rows; it did not "+
			"take the terminal", m.mark.h, m.Layout.BoardH)
	}

	// And f again gives it back.
	m.Update(key("f"))
	tick(m, 30)
	if !strings.Contains(screenOf(m), "% Complete") {
		t.Error("f again did not give the chrome back")
	}
}

// A one-row play area, as the resize suite walks every diversion through at
// 30x8. It must not divide by zero or draw outside itself.
func TestTheMarkSurvivesAOneRowPanel(t *testing.T) {
	m := bouncing(t, 110, 30)
	for _, wh := range [][2]int{{30, 8}, {200, 50}, {40, 10}} {
		m.Resize(wh[0], wh[1])
		tick(m, 60)
		buf := drawTo(m)
		w, h := buf.Size()
		for y := 0; y < h; y++ {
			if n := len([]rune(buf.Row(y))); n != w {
				t.Fatalf("at %dx%d the mark drew a row of %d, want %d", wh[0], wh[1], n, w)
			}
		}
	}
}

// The other colour states stay inside the palette so the refiner's theme owns
// the screen. This one goes outside it, since a four-entry ring repeats too
// soon for a mark bouncing round an empty screen.
func TestTheMarkTakesAFreshColourOnEveryEdge(t *testing.T) {
	m := bouncing(t, 120, 34)
	for i := 0; i < int(hueRandom); i++ {
		m.Update(key("c"))
	}
	if m.mark.hue != hueRandom {
		t.Fatalf("%d presses of c left it in state %d, want hueRandom", hueRandom, m.mark.hue)
	}

	seen := map[string]bool{}
	last := m.mark.picked
	changes := 0
	for i := 0; i < 2400; i++ {
		tick(m, 1)
		seen[m.mark.Attr(m.Theme).FG.String()] = true
		if m.mark.picked != last {
			changes++
			last = m.mark.picked
		}
	}
	if changes < 3 {
		t.Errorf("the colour changed %d times in 2400 frames, want one per edge", changes)
	}
	if len(seen) < 4 {
		t.Errorf("only %d colours in 2400 frames, want a fresh one off the wheel each edge",
			len(seen))
	}
}

func TestThePhysicsTumbleIsRandomised(t *testing.T) {
	m := bouncing(t, 140, 40)

	spins := map[float64]bool{}
	for i := 0; i < 12; i++ {
		m.Update(key("p")) // on
		spins[m.mark.spin] = true
		m.Update(key("p")) // off again
	}
	if len(spins) < 6 {
		t.Errorf("twelve presses gave %d different tumbles, want a fresh one each time",
			len(spins))
	}

	// Both ways round, and never neither: a mark that does not turn under gravity
	// is not falling.
	pos, neg := false, false
	for s := range spins {
		if s == 0 {
			t.Error("one press left it with no tumble at all")
		}
		if s > 0 {
			pos = true
		} else {
			neg = true
		}
	}
	if !pos || !neg {
		t.Error("every tumble went the same way round")
	}
}

// Outside physics it crosses at a constant speed, which TestTheMarkNeverSettles
// pins to within a hundredth over eighteen hundred frames. A drag would break
// that, and a press must still leave.
func TestTheMarkIsOnlyDraggableUnderPhysics(t *testing.T) {
	m := bouncing(t, 140, 40)
	panel := m.markPanel()

	// Where the mark is, in screen cells.
	cellAt := func() (int, int) {
		return panel.X + int(m.mark.x)/canvas.DotsX, panel.Y + int(m.mark.y)/canvas.DotsY
	}

	// Outside physics: a press on it leaves.
	x, y := cellAt()
	m.Update(press(x, y))
	if m.mark != nil && m.mark.Held() {
		t.Error("the mark was picked up with no gravity to throw it against")
	}
	// It fades before it flies: Perpetuity embeds Fade, so endDiversion asks it to
	// fade and sends the digits home only once it has. Leaving is the signal at
	// this instant; the transit comes later.
	if !m.mark.Leaving() {
		t.Error("a press outside physics mode did not start it leaving")
	}

	// Under physics: it comes with the pointer.
	m = bouncing(t, 140, 40)
	m.Update(key("p"))
	tick(m, 2)
	x, y = cellAt()
	m.Update(press(x, y))
	if !m.mark.Held() {
		t.Fatalf("a press at (%d,%d), where the mark is, did not pick it up", x, y)
	}

	m.Update(drag(x+8, y+3))
	tick(m, 2)
	gotX, gotY := cellAt()
	if gotX == x && gotY == y {
		t.Error("the mark did not follow the pointer")
	}

	// And it is thrown, not dropped dead.
	m.Update(drag(x+16, y+5))
	tick(m, 1)
	m.Update(release(x+16, y+5))
	if m.mark.Held() {
		t.Error("releasing did not let go")
	}
	if math.Hypot(m.mark.vx, m.mark.vy) < 1 {
		t.Error("the mark was released stationary, so the throw was lost")
	}
}

// Otherwise the diversion is a trap.
func TestAPressBesideTheMarkStillLeaves(t *testing.T) {
	m := bouncing(t, 140, 40)
	m.Update(key("p"))
	tick(m, 2)

	panel := m.markPanel()
	m.Update(press(panel.X, panel.Y+panel.H-1)) // a corner the mark is not in
	if m.mark.Held() {
		t.Fatal("a press in the corner picked the mark up")
	}
	if !m.mark.Leaving() {
		t.Error("a press that missed the mark did not start it leaving")
	}
}

// Snapping the centre to the pointer would read as the mark jumping out of your
// hand, and the jump would register as a throw.
func TestTakingTheMarkByItsEdgeDoesNotSnapIt(t *testing.T) {
	m := bouncing(t, 140, 40)
	m.Update(key("p"))
	tick(m, 2)

	p := m.mark
	rx, _ := p.radius()
	wasX, wasY := p.x, p.y

	// Take it near its left edge.
	if !p.Take(p.x-rx*0.8, p.y) {
		t.Fatal("the mark refused a grab well inside its own extent")
	}
	if p.x != wasX || p.y != wasY {
		t.Errorf("taking hold moved the mark from (%.1f,%.1f) to (%.1f,%.1f)",
			wasX, wasY, p.x, p.y)
	}

	// Dragging by a known amount moves it by the same amount.
	p.DragTo(p.x-rx*0.8+10, p.y+4)
	p.Step(1.0 / 60)
	if got := p.x - wasX; math.Abs(got-10) > 0.001 {
		t.Errorf("the mark moved %.2f dots for a 10 dot drag", got)
	}
	if got := p.y - wasY; math.Abs(got-4) > 0.001 {
		t.Errorf("the mark moved %.2f dots for a 4 dot drag", got)
	}
}
