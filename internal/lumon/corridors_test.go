package lumon

import (
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/terminaltrove/lumon/internal/canvas"
	"github.com/terminaltrove/lumon/internal/glyph"
	"github.com/terminaltrove/lumon/internal/physics"
	"github.com/terminaltrove/lumon/internal/theme"
)

// mazeOf carves one at a given size, without a board behind it.
func mazeOf(t *testing.T, w, h int, seed int64) *Corridors {
	t.Helper()
	// Mirrors NewCorridors, defaults included (the white lab is on), so the tests
	// build the same object production does.
	c := &Corridors{
		rng:    rand.New(rand.NewSource(seed)),
		digits: []rune("0123456789"),
		fx:     effects{lumon: true},
	}
	c.Resize(Panel{X: 0, Y: 0, W: w, H: h})
	if c.cols < 1 || c.rows < 1 {
		t.Fatalf("%dx%d carved nothing", w, h)
	}
	return c
}

// A recursive backtracker guarantees this. A wall recorded against the wrong
// cell walls a region off while still looking like a maze. Flood fill from a
// corner and count.
func TestEveryCellOfTheMazeIsReachable(t *testing.T) {
	for _, seed := range []int64{1, 2, 3, 17, 99} {
		c := mazeOf(t, 120, 30, seed)
		n := c.cols * c.rows

		seen := make([]bool, n)
		seen[0] = true
		stack := []int{0}
		count := 1
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, to := range [4]int{cur + 1, cur - 1, cur + c.cols, cur - c.cols} {
				if to < 0 || to >= n || seen[to] || !c.open(cur, to) {
					continue
				}
				seen[to] = true
				count++
				stack = append(stack, to)
			}
		}
		if count != n {
			t.Errorf("seed %d: %d of %d cells reachable, the maze has a region walled off",
				seed, count, n)
		}
	}
}

// A carve that opened every wall would pass reachability and be an empty
// rectangle. A perfect maze on n cells is a spanning tree with n-1 openings,
// and rooms add loops, so the extra openings are measured against the loops
// available. A grid this size opens half its walls just to be a maze, so
// "under half of all walls" would pass on an empty hall.
func TestTheMazeIsNotJustAnEmptyRoom(t *testing.T) {
	for _, seed := range []int64{3, 7, 41} {
		c := mazeOf(t, 120, 30, seed)
		n := c.cols * c.rows

		open, walls := 0, 0
		for i := 0; i < n; i++ {
			if i%c.cols != c.cols-1 {
				walls++
				if c.openRight[i] {
					open++
				}
			}
			if i/c.cols != c.rows-1 {
				walls++
				if c.openDown[i] {
					open++
				}
			}
		}

		tree := n - 1        // what a perfect maze opens
		loops := open - tree // what the rooms added on top
		if loops < 1 {
			t.Errorf("seed %d: %d openings against a spanning tree's %d; there are no "+
				"rooms and the walk has nowhere that opens out", seed, open, tree)
		}
		if spare := walls - tree; loops > spare/2 {
			t.Errorf("seed %d: the rooms opened %d of the %d walls a spanning tree "+
				"leaves standing; that is a hall rather than a maze", seed, loops, spare)
		}
	}
}

// Nothing here collides: the walker is a point moving along a heading, and it
// stays in the corridors only because the route is followed cell by cell. A
// route that skips a step walks through a wall, which shows on screen only as
// a wall passing over the camera. The look-ahead in bearing cuts corners in
// open floor, which is where this breaks.
//
// Rooms turned this up twice in one change. The look-ahead in bearing cuts a
// corner wide enough to miss the arrival cell in open floor, which left the walk
// orbiting a cell for ever; and the fix for that, taken alone, counted every cell
// of the route lying behind the walker as visited, which ate thirty steps in the
// time it takes to walk four.
func TestTheWalkStaysOnTheFloor(t *testing.T) {
	for _, seed := range []int64{1, 4, 7, 23} {
		c := mazeOf(t, 120, 30, seed)
		was := c.cellAt(c.at)
		for i := 0; i < 3000; i++ {
			c.Step(1.0 / 60)
			now := c.cellAt(c.at)
			if now == was {
				continue
			}
			if now < 0 {
				t.Fatalf("seed %d frame %d: the walk left the maze at %v", seed, i, c.at)
			}
			// Every cell it enters must be a neighbour it could walk into.
			if !c.open(was, now) {
				if _, isDoor := c.doorAt(was, now); !isDoor {
					t.Fatalf("seed %d frame %d: the walk went from cell %d to %d at %v, "+
						"which is through a wall", seed, i, was, now, c.at)
				}
			}
			was = now
		}
	}
}

func TestTheWallsAreMadeOfTheBoardsOwnDigits(t *testing.T) {
	m := diverting(t, "CORRIDORS", 120, 40)
	if m.halls == nil {
		t.Fatal("the corridors are not running")
	}

	places := m.halls.Digits(m.gridPanel().Play())
	if len(places) < 50 {
		t.Fatalf("the maze offered %d wall cells, want the whole outline", len(places))
	}
	for _, p := range places {
		if p.Digit < '0' || p.Digit > '9' {
			t.Errorf("a wall cell carries %q, want one of the board's digits", p.Digit)
			break
		}
	}
}

func TestTheWalkKeepsGoing(t *testing.T) {
	c := mazeOf(t, 120, 30, 4)

	start := c.at
	moved, far := 0, 0.0
	for i := 0; i < 900; i++ {
		before := c.at
		c.Step(1.0 / 60)
		if c.at.Sub(before).Len() > 0.001 {
			moved++
		}
		if d := c.at.Sub(start).Len(); d > far {
			far = d
		}
		if c.at.X < 0 || c.at.Y < 0 || c.at.X > float64(c.cols) || c.at.Y > float64(c.rows) {
			t.Fatalf("frame %d: the walk left the maze at %v", i, c.at)
		}
	}
	if moved < 850 {
		t.Errorf("it moved on %d of 900 frames; it is stopping", moved)
	}
	if far < 3 {
		t.Errorf("it got %.1f cells from where it started; it is not walking", far)
	}
}

// There is no way off this floor, so the end of a route means picking another.
func TestTheWalkAlwaysHasSomewhereToBe(t *testing.T) {
	c := mazeOf(t, 120, 30, 5)
	for i := 0; i < 6000; i++ {
		c.Step(1.0 / 60)
		if len(c.path) == 0 {
			t.Fatalf("frame %d: no route left", i)
		}
		if c.step >= len(c.path) {
			t.Fatalf("frame %d: past the end of the route", i)
		}
	}
}

// A bug in the distance or in the fish-eye correction still draws something
// that looks like a corridor.
func TestTheCorridorHasDepth(t *testing.T) {
	c := mazeOf(t, 120, 30, 9)
	panel := Panel{X: 0, Y: 0, W: 80, H: 20}
	c.Resize(panel)

	// March the same ray from further and further back and watch the wall shrink.
	c.at = physics.Vec2{X: 1.5, Y: 1.5}
	near := c.cast(0)

	var taller, shorter float64
	for _, d := range []float64{0.1, 4.0} {
		c.at = physics.Vec2{X: 1.5 - d, Y: 1.5}
		got := c.cast(0)
		h := corridorWall / math.Max(got.dist, 0.05) * float64(panel.H)
		if d < 1 {
			taller = h
		} else {
			shorter = h
		}
	}
	if !(taller > shorter) {
		t.Errorf("a wall %.1f away is drawn %.1f tall and one further back %.1f; there "+
			"is no perspective", near.dist, taller, shorter)
	}
}

func TestAClosedDoorBlocksTheWayAndAnOpenOneDoesNot(t *testing.T) {
	c := mazeOf(t, 120, 30, 3)

	// Find a door.
	n := c.cols * c.rows
	from, to := -1, -1
	for i := 0; i < n && from < 0; i++ {
		if c.isDoor[i] {
			from, to = i, i+1
		} else if c.isDoor[n+i] {
			from, to = i, i+c.cols
		}
	}
	if from < 0 {
		t.Skip("this maze got no doors")
	}

	if !c.blocked(from, to) {
		t.Error("a shut door did not block the way")
	}
	if _, isDoor := c.doorAt(from, to); !isDoor {
		t.Fatal("doorAt does not know about a door hang put in")
	}

	// Open it.
	if to-from == 1 {
		c.doorRight[from] = 1
	} else {
		c.doorDown[from] = 1
	}
	if c.blocked(from, to) {
		t.Error("an open door still blocked the way")
	}
}

func TestATinyPanelHasNoCorridors(t *testing.T) {
	c := &Corridors{rng: rand.New(rand.NewSource(1)), digits: []rune("0")}
	for _, size := range [][2]int{{0, 0}, {1, 1}, {3, 1}, {4, 2}} {
		c.Resize(Panel{X: 0, Y: 0, W: size[0], H: size[1]})
		c.Step(1.0 / 60)
		buf := canvas.NewBuffer(10, 5)
		c.Draw(buf, theme.DefaultTheme, Panel{X: 0, Y: 0, W: size[0], H: size[1]})
	}
}

// The keys are the ones the hint line promises.
func TestTheMazesEffectsAnswerTheirKeys(t *testing.T) {
	m := diverting(t, "CORRIDORS", 110, 32)

	// Each row has its starting state, so flipping a default costs one bool here.
	// The white lab starts on.
	for _, c := range []struct {
		key   string
		start bool
		read  func() bool
	}{
		{"c", false, func() bool { a, _, _, _, _, _, _ := m.halls.Effects(); return a }},
		{"m", false, func() bool { _, a, _, _, _, _, _ := m.halls.Effects(); return a }},
		{"n", false, func() bool { _, _, a, _, _, _, _ := m.halls.Effects(); return a }},
		{"t", false, func() bool { _, _, _, a, _, _, _ := m.halls.Effects(); return a }},
		{"w", false, func() bool { _, _, _, _, a, _, _ := m.halls.Effects(); return a }},
		{"l", true, func() bool { _, _, _, _, _, a, _ := m.halls.Effects(); return a }},
		{"i", false, func() bool { _, _, _, _, _, _, a := m.halls.Effects(); return a }},
	} {
		if c.read() != c.start {
			t.Fatalf("%q starts %v, want %v", c.key, c.read(), c.start)
		}
		m.Update(key(c.key))
		if c.read() == c.start {
			t.Errorf("%q did not change its effect", c.key)
		}
		m.Update(key(c.key))
		if c.read() != c.start {
			t.Errorf("%q did not change it back again", c.key)
		}
		if m.halls == nil {
			t.Fatalf("%q left the maze", c.key)
		}
	}

	// The hint lists all seven, so they can be found.
	hint := m.divertHint()
	for _, k := range []string{"c ", "i ", "l ", "m ", "n ", "t ", "w "} {
		if !strings.Contains(hint, k) {
			t.Errorf("the hint does not mention %q: %q", k, hint)
		}
	}
}

// A dial on the walk alone would leave the alarm pulsing and the walls running
// at full speed while the walker crept. The dial is one multiplier on the
// diversion's dt, asserted on the ground covered and on the alarm (a pure
// function of c.age). Neither is exact in floating point: 960 quarter-frame
// additions do not land on the same bits as 240 whole ones, and a spring
// integrated at dt/4 lands slightly elsewhere. So the clock may differ in its
// last bits and the ground by a fifth. The alarm is compared exactly, since it
// is quantised to a byte a channel.
func TestTheSpeedDialSlowsEverything(t *testing.T) {
	fast := diverting(t, "CORRIDORS", 110, 32)
	slow := diverting(t, "CORRIDORS", 110, 32)
	th := fast.Theme

	if fast.halls.at != slow.halls.at {
		t.Fatal("the two mazes did not start in the same place; nothing below compares anything")
	}
	for _, m := range []*Model{fast, slow} {
		m.halls.fx.alarm = true
	}
	slow.Update(key(","))
	slow.Update(key(","))
	if got := slow.halls.Pace(); got != 0.25 {
		t.Fatalf("two steps down the dial reached %vx, want 0.25x", got)
	}
	if got := fast.halls.Pace(); got != 1 {
		t.Fatalf("an untouched maze is at %vx, want 1x", got)
	}

	walked := func(m *Model) float64 {
		was, sum := m.halls.at, 0.0
		for n := 0; n < 240; n++ {
			tick(m, 1)
			sum += m.halls.at.Sub(was).Len()
			was = m.halls.at
		}
		return sum
	}

	// The same frames at a quarter speed, so the slow one should be a quarter of
	// the way along. Then three more helpings, and the two clocks should agree.
	quick := walked(fast)
	crept := walked(slow)
	if crept > quick*0.4 {
		t.Errorf("a quarter speed covered %.1f cells against %.1f at full speed; the dial is not reaching the walk",
			crept, quick)
	}
	for n := 0; n < 3; n++ {
		crept += walked(slow)
	}

	if math.Abs(fast.halls.age-slow.halls.age) > 1e-9 {
		t.Fatalf("four times the frames at a quarter speed reached age %v, want %v",
			slow.halls.age, fast.halls.age)
	}
	if a, b := fast.halls.alarmAttr(th), slow.halls.alarmAttr(th); a != b {
		t.Error("the alarm is at a different phase than the same simulated time at full speed; " +
			"the dial is wired to the walk rather than to the clock")
	}
	if crept < quick*0.8 || crept > quick*1.2 {
		t.Errorf("four times the frames at a quarter speed covered %.1f cells against %.1f; "+
			"the dial is not a clock scale", crept, quick)
	}

	// It stops at both ends of the slice.
	for n := 0; n < 8; n++ {
		slow.Update(key(","))
	}
	if got := slow.halls.Pace(); got != 0.25 {
		t.Errorf("the dial went past its slow end to %vx", got)
	}
	for n := 0; n < 8; n++ {
		slow.Update(key("."))
	}
	if got := slow.halls.Pace(); got != 4 {
		t.Errorf("the dial went past its fast end to %vx", got)
	}
}

func TestRStillMeansAgainInTheMaze(t *testing.T) {
	m := diverting(t, "CORRIDORS", 110, 32)
	was := m.halls
	m.Update(key("r"))
	if m.halls == nil {
		t.Fatal("r left the maze rather than restarting it")
	}
	if m.halls == was {
		t.Error("r did not build a new maze")
	}
}

// The theme's roles, so it reddens in each palette's own red: the pair a
// cluster on its fuse flashes between.
func TestTheAlarmPulsesBetweenUrgentAndPeak(t *testing.T) {
	m := diverting(t, "CORRIDORS", 110, 32)
	m.Update(key("w"))
	th := m.Theme

	seen := map[string]bool{}
	lo, hi := false, false
	for i := 0; i < 240; i++ {
		tick(m, 1)
		fg := m.halls.alarmAttr(th).FG
		seen[fg.String()] = true
		if fg == th.Urgent {
			lo = true
		}
		if fg == th.Peak {
			hi = true
		}
	}
	if !lo || !hi {
		t.Errorf("the alarm reached urgent=%v peak=%v over four seconds; it is not "+
			"sweeping the whole way", lo, hi)
	}
	if len(seen) < 20 {
		t.Errorf("only %d colours across the pulse; it is stepping rather than "+
			"gradating", len(seen))
	}

	// It has the theme's paper, or it punches the terminal's background through on
	// a pale one.
	if got := m.halls.alarmAttr(th).BG; got != th.Paper {
		t.Errorf("the alarm's background is %v, want the theme's paper %v", got, th.Paper)
	}
}

func TestAllSevenEffectsRunTogether(t *testing.T) {
	m := diverting(t, "CORRIDORS", 110, 32)
	for _, k := range []string{"c", "i", "l", "m", "n", "t", "w"} {
		m.Update(key(k))
	}
	panel := m.gridPanel().Play()
	for i := 0; i < 300; i++ {
		tick(m, 1)
		buf := drawTo(m)
		if buf == nil {
			t.Fatalf("frame %d drew nothing", i)
		}
		_ = panel
	}
	if m.halls == nil {
		t.Fatal("the maze is gone")
	}
}

// Both are shaded by distance out of glyph.GlyphRamp, so they read as surfaces
// and not more numbers; four characters is four bands. Asserted against the
// rendered buffer.
func TestTheCorridorHasAFloorAndACeiling(t *testing.T) {
	m := diverting(t, "CORRIDORS", 110, 34)
	panel := m.gridPanel().Play()

	// A frame in which some of the corridor is far enough off to leave floor and
	// ceiling on screen. A wall against your face fills the panel.
	var floor, ceiling map[rune]int
	for n := 0; n < 240 && (len(floor) < 2 || len(ceiling) < 2); n++ {
		tick(m, 1)
		buf := drawTo(m)
		floor, ceiling = map[rune]int{}, map[rune]int{}
		for y := panel.Y; y < panel.Y+panel.H; y++ {
			for _, r := range []rune(buf.Row(y))[panel.X : panel.X+panel.W] {
				if !strings.ContainsRune(glyph.GlyphRamp, r) {
					continue
				}
				if y < panel.Y+panel.H/2 {
					ceiling[r]++
				} else {
					floor[r]++
				}
			}
		}
	}

	if len(ceiling) < 2 {
		t.Errorf("the ceiling is drawn in %d shades over four seconds, want the ramp "+
			"banding with distance: %v", len(ceiling), ceiling)
	}
	if len(floor) < 2 {
		t.Errorf("the floor is drawn in %d shades: %v", len(floor), floor)
	}

	// They are surfaces: nothing digit-shaped in either.
	for _, r := range glyph.GlyphRamp {
		if r >= '0' && r <= '9' {
			t.Fatalf("the surface ramp contains %q", r)
		}
	}
}

// Peak is #FFFFFF in the lumon palette and means "the brightest single thing on
// screen", so the ceiling comes out right on all nine palettes, including the
// pale one where white would be invisible.
func TestTheLumonLookPutsPeakOverhead(t *testing.T) {
	for _, name := range []string{"lumon", "inverted"} {
		th, ok := theme.FindTheme(name)
		if !ok {
			t.Fatalf("no theme %q", name)
		}
		c := mazeOf(t, 120, 30, 11)

		// The nearest strip of ceiling, the brightest it gets. The look starts on, so
		// l takes it away.
		on := c.surfaceAttr(th, true, 1)
		c.Key("l")
		off := c.surfaceAttr(th, true, 1)
		c.Key("l")

		if on.FG != th.Peak {
			t.Errorf("%s: the ceiling nearest the eye is %v, want the palette's Peak %v",
				name, on.FG, th.Peak)
		}
		if on == off {
			t.Errorf("%s: l changed nothing overhead", name)
		}
		if on.BG != th.Paper {
			t.Errorf("%s: the ceiling's background is %v, want the theme's paper %v",
				name, on.BG, th.Paper)
		}

		// It falls off with distance, or a white ceiling has no depth.
		if far := c.surfaceAttr(th, true, 0); far.FG == on.FG {
			t.Errorf("%s: the ceiling is %v at the eye and at the far light too; it "+
				"does not recede", name, far.FG)
		}

		// The floor goes the other way: light above and shadow below.
		if c.surfaceAttr(th, false, 1).FG == on.FG {
			t.Errorf("%s: the floor is drawn the same as the ceiling", name)
		}
	}
}

// Asserted through doorCell, since whether a door is in view at a given moment
// depends on the carve and the walk.
func TestADoorIsDrawnAsAFrameAndASlidingLeaf(t *testing.T) {
	c := mazeOf(t, 120, 30, 3)
	th := theme.DefaultTheme
	got := hit{isDoor: true, dist: 2}

	// The frame runs all the way round: both jambs and both rails.
	for _, at := range []struct {
		face, down float64
		what       string
	}{
		{0.02, 0.5, "the near jamb"},
		{0.98, 0.5, "the far jamb"},
		{0.5, 0.02, "the lintel"},
		{0.5, 0.98, "the threshold"},
	} {
		got.face = at.face
		r, attr := c.doorCell(th, got, at.down, 1, '7', th.Number)
		if r != glyph.GlyphSolid {
			t.Errorf("%s is drawn %q, want the frame", at.what, r)
		}
		if attr.FG != c.frameAttr(th, 1).FG {
			t.Errorf("%s is %v, want the structure's own colour %v",
				at.what, attr.FG, c.frameAttr(th, 1).FG)
		}
	}

	// Shut, the middle of the face is leaf.
	got.face, got.door = 0.5, 0
	if r, _ := c.doorCell(th, got, 0.5, 1, '7', th.Number); r != '7' {
		t.Errorf("a shut door's middle is drawn %q, want the wall's own digit", r)
	}

	// Half open, the leaf covers the near half and the far half is the gap.
	got.door = 0.5
	leaf, _ := c.doorCell(th, got, 0.5, 1, '7', th.Number)
	got.face = 0.85
	gap, gapAttr := c.doorCell(th, got, 0.5, 1, '7', th.Number)
	if leaf != '7' {
		t.Errorf("half open, the near half of the face is %q, want the leaf", leaf)
	}
	if gap == leaf {
		t.Error("half open, the far half of the face is still the leaf; nothing slid")
	}
	if gapAttr.FG != th.Lurk {
		t.Errorf("the gap is %v, want it darker than the leaf", gapAttr.FG)
	}
}

// doorCell passing says nothing about whether Draw calls it. Stand in front of
// a shut door, draw, and look for the frame.
func TestADoorsFrameIsActuallyDrawn(t *testing.T) {
	c := mazeOf(t, 120, 30, 3)
	panel := Panel{X: 0, Y: 0, W: 100, H: 24}
	c.Resize(panel)
	th := theme.DefaultTheme

	// A door to look at, with floor behind to stand on. From inside the doorway the
	// door is drawn at twice the panel height and its frame is off screen, so this
	// stands back far enough to see the whole frame.
	n := c.cols * c.rows
	from, angle := -1, 0.0
	for i := 0; i < n && from < 0; i++ {
		switch {
		case c.isDoor[i] && c.open(i, i-1):
			from, angle = i, 0 // the wall to its right
		case c.isDoor[n+i] && c.open(i, i-c.cols):
			from, angle = i, math.Pi/2 // the wall below it
		}
	}
	if from < 0 {
		t.Skip("this maze got no doors with room to stand back from")
	}

	back := physics.Vec2{X: math.Cos(angle), Y: math.Sin(angle)}.Scale(0.9)
	c.at = c.centreOf(from).Sub(back)
	c.heading = physics.NewScalar(angle, corridorTurn)
	buf := canvas.NewBuffer(panel.W, panel.H)
	c.Draw(buf, th, panel)

	frame := 0
	for y := 0; y < panel.H; y++ {
		for _, r := range []rune(buf.Row(y)) {
			// The rune alone: the structure runs down the white lab's ramp, so its colour
			// varies with distance. Draw sets GlyphSolid only for ribs and door frames, and
			// neither ramp used for walls or surfaces contains it.
			if r == glyph.GlyphSolid {
				frame++
			}
		}
	}
	if frame == 0 {
		t.Error("standing in front of a shut door, none of its frame was drawn")
	}
}

// i is the endless corridor from the end of the second series. No maze, no
// turns, no end.
func TestTheEndlessCorridorRunsForEver(t *testing.T) {
	c := mazeOf(t, 120, 30, 12)
	c.Key("i")

	if _, _, _, _, _, _, on := c.Effects(); !on {
		t.Fatal("i did not open the endless corridor")
	}

	// It runs straight, and it never stops.
	start := c.at
	for i := 0; i < 60*60; i++ {
		c.Step(1.0 / 60)
		if c.at.Y != hallWidth/2 {
			t.Fatalf("frame %d: it wandered off the corridor's axis to y=%v", i, c.at.Y)
		}
	}
	want := hallSpeed * 60
	if got := c.at.X - start.X; math.Abs(got-want) > 1 {
		t.Errorf("a minute of running covered %.0f cells, want about %.0f", got, want)
	}

	// The corridor is still there at the far end: nothing about the cast depends
	// on how far it has come.
	panel := Panel{X: 0, Y: 0, W: 80, H: 20}
	c.Resize(panel)
	ahead := c.cast(0)
	if ahead.dist < corridorDepth {
		t.Errorf("looking straight down it, something is %.1f cells away; there is "+
			"meant to be nothing to hit", ahead.dist)
	}
	for _, a := range []float64{-corridorFOV / 2, corridorFOV / 2} {
		if got := c.cast(a); got.dist >= corridorDepth {
			t.Errorf("at %.2f radians off the axis the ray met no wall; the corridor "+
				"has no sides", a)
		}
	}
}

// Two parallel walls of tiling digits have nothing in them that arrives. The
// ribs make the motion visible.
func TestTheEndlessCorridorHasSomethingPassingYou(t *testing.T) {
	m := diverting(t, "CORRIDORS", 110, 32)
	m.Update(key("i"))
	panel := m.gridPanel().Play()

	ribsIn := func() int {
		buf := drawTo(m)
		n := 0
		for y := panel.Y; y < panel.Y+panel.H; y++ {
			row := []rune(buf.Row(y))
			for x := panel.X; x < panel.X+panel.W; x++ {
				// The rune alone, as for the door frame. There are no doors in the endless
				// corridor, so a solid block in it is a rib.
				if row[x] == glyph.GlyphSolid {
					n++
				}
			}
		}
		return n
	}

	tick(m, 20)
	first := ribsIn()
	if first == 0 {
		t.Fatal("no ribs are drawn in the endless corridor; nothing passes you")
	}

	// The picture changes as you run, which a fixed pattern would not.
	same := 0
	for i := 0; i < 40; i++ {
		tick(m, 3)
		if ribsIn() == first {
			same++
		}
	}
	if same > 30 {
		t.Errorf("the ribs came out the same on %d of 40 samples; the corridor is "+
			"not going past", same)
	}
}

func TestLeavingTheEndlessCorridorGoesBackToTheMaze(t *testing.T) {
	c := mazeOf(t, 120, 30, 15)
	c.Key("i")
	for i := 0; i < 600; i++ {
		c.Step(1.0 / 60)
	}
	if c.at.X < 5 {
		t.Fatal("it did not run anywhere")
	}

	c.Key("i")
	if _, _, _, _, _, _, on := c.Effects(); on {
		t.Fatal("i did not close the endless corridor")
	}

	// Back inside the maze, on a new route, walking it without going through walls.
	// The old route would aim at a cell it is nowhere near.
	if cell := c.cellAt(c.at); cell < 0 {
		t.Fatalf("the walk came back at %v, which is not in the maze", c.at)
	}
	was := c.cellAt(c.at)
	for i := 0; i < 1200; i++ {
		c.Step(1.0 / 60)
		now := c.cellAt(c.at)
		if now == was {
			continue
		}
		if !c.open(was, now) {
			if _, isDoor := c.doorAt(was, now); !isDoor {
				t.Fatalf("frame %d: back in the maze it went from %d to %d through a wall",
					i, was, now)
			}
		}
		was = now
	}
}

// A walk that drifts into a cell its route never named has its target as a
// diagonal neighbour: the aim points at a wall, slideOffWalls clamps the moving
// axis, and nothing moves again. Measured as the time without reaching a new
// cell; distance covered misses it, since a walk pinned against a wall slides
// up and down for ever.
func TestTheWalkNeverGetsStuckInAWall(t *testing.T) {
	for _, seed := range []int64{1, 3, 8, 13, 21, 34} {
		c := mazeOf(t, 120, 30, seed)
		was, since, worst, cells := c.cellAt(c.at), 0.0, 0.0, map[int]bool{}
		for i := 0; i < 60*90; i++ {
			c.Step(1.0 / 60)
			since += 1.0 / 60
			if now := c.cellAt(c.at); now != was {
				if since > worst {
					worst = since
				}
				was, since, cells[now] = now, 0, true
			}
		}
		if since > worst {
			worst = since
		}

		// A cell takes about half a second at corridorSpeed, and a shut door on the way
		// adds the half second it takes to slide. Four is generous.
		if worst > 4 {
			t.Errorf("seed %d: the walk spent %.1fs in one cell; it is stuck", seed, worst)
		}
		// It is going somewhere, not shuffling between two cells.
		if len(cells) < 60 {
			t.Errorf("seed %d: ninety seconds of walking reached %d distinct cells",
				seed, len(cells))
		}
	}
}

// The cell the walk heads for is either the one it stands in or a neighbour it
// can reach through a carved opening or a door; a diagonal is neither. Routes
// go through doors, so this asks open and doorAt and not blocked,
// as the check in Step does.
func TestTheWalkAlwaysHasARouteItCanActuallyWalk(t *testing.T) {
	for _, seed := range []int64{2, 5, 11, 19, 40} {
		c := mazeOf(t, 120, 30, seed)
		for i := 0; i < 60*90; i++ {
			c.Step(1.0 / 60)

			here, want := c.cellAt(c.at), c.path[c.step]
			if here == want {
				continue
			}
			if c.open(here, want) {
				continue
			}
			if _, isDoor := c.doorAt(here, want); isDoor {
				continue
			}
			t.Fatalf("seed %d frame %d: standing in cell %d (%d,%d) heading for %d "+
				"(%d,%d), which is not a neighbour it can reach; the walk is off its "+
				"route and will wedge against the wall between them",
				seed, i, here, here%c.cols, here/c.cols,
				want, want%c.cols, want/c.cols)
		}
	}
}

// The places are derived from what the corridor draws. On the endless hall the
// two walls converge on a point, so the vertical spread of the places is wide at
// the panel's edges and narrow in the middle. A ring is the other way round and
// a spiral is neither.
func TestTheArrivalIsTheShapeOfTheCorridor(t *testing.T) {
	m := diverting(t, "CORRIDORS", 120, 40)
	m.Update(key("i")) // the endless hall
	tick(m, 30)
	panel := m.gridPanel().Play()

	spread := func(from, to int) float64 {
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, p := range m.halls.Digits(panel) {
			if int(p.X) < from || int(p.X) >= to {
				continue
			}
			lo, hi = math.Min(lo, p.Y), math.Max(hi, p.Y)
		}
		if math.IsInf(lo, 1) {
			return 0
		}
		return hi - lo
	}

	edge := math.Max(spread(panel.X, panel.X+panel.W/6),
		spread(panel.X+panel.W*5/6, panel.X+panel.W))
	middle := spread(panel.X+panel.W*2/5, panel.X+panel.W*3/5)

	if edge < middle*2 {
		t.Errorf("the places span %.1f rows at the panel's edges and %.1f in its "+
			"middle; a corridor converges and this does not", edge, middle)
	}
}

// Digits are ViewportW times ViewportH, a grid fact, and columns are a screen
// fact; two edges a column (240 at 120 columns) is near 209 digits at that size
// only by chance. NewTransit walks the drawn frame and cycles the places, so too
// many drops the far end of the corridor and too few repeat its near end.
func TestTheArrivalFitsAnyNumberOfDigits(t *testing.T) {
	for _, wh := range [][2]int{{60, 18}, {80, 24}, {110, 32}, {120, 40}, {200, 50}} {
		m := diverting(t, "CORRIDORS", wh[0], wh[1])
		panel := m.gridPanel().Play()
		places := m.halls.Digits(panel)

		if got, want := len(places), len(m.halls.digits); got != want {
			t.Errorf("%dx%d: %d places for %d digits", wh[0], wh[1], got, want)
		}
		for i, p := range places {
			if !panel.Contains(int(p.X), int(p.Y)) {
				t.Errorf("%dx%d: place %d at %.1f,%.1f is outside the panel %+v, so "+
					"clampPlaced has to rescue it", wh[0], wh[1], i, p.X, p.Y, panel)
				break
			}
		}
	}
}

// Facing a wall a fraction of a cell away, the band is up to nineteen times the
// panel and every column's edges are off it. Clamping them would put every
// digit on the top and bottom rows.
func TestADeadEndSpreadsTheDigitsOverTheWall(t *testing.T) {
	m := diverting(t, "CORRIDORS", 120, 40)
	c := m.halls
	panel := m.gridPanel().Play()

	// Nose against a wall: find a blocked direction and stand right at it.
	here := c.cellAt(c.at)
	for _, d := range [4][3]float64{{1, 0, 0}, {-1, 0, math.Pi}, {0, 1, math.Pi / 2}, {0, -1, -math.Pi / 2}} {
		if c.walled(here%c.cols, here/c.cols, int(d[0]), int(d[1])) {
			c.at = c.centreOf(here).Add(physics.Vec2{X: d[0], Y: d[1]}.Scale(0.5 - corridorSkin))
			c.heading = physics.NewScalar(d[2], corridorTurn)
			break
		}
	}

	rows := map[int]bool{}
	for _, p := range c.Digits(panel) {
		rows[int(p.Y)] = true
	}
	if len(rows) < 3 {
		t.Errorf("against a wall the digits landed on %d rows; that is bars rather "+
			"than a surface", len(rows))
	}
}

// Asked of the buffer, not the geometry both sides call, or the test only shows
// a function agrees with itself. The surfaces are drawn out of GlyphRamp and the
// wall out of digits and EmergeRamp, so a place must be on the wall or within a
// row of it.
func TestTheCorridorArrivesWhereItIsDrawn(t *testing.T) {
	m := diverting(t, "CORRIDORS", 120, 40)
	m.Update(key("i")) // the hall, whose silhouette is unambiguous
	tick(m, 30)

	panel := m.gridPanel().Play()
	places := m.halls.Digits(panel)
	buf := drawTo(m)

	wall := func(x, y int) bool {
		if !panel.Contains(x, y) {
			return false
		}
		r := []rune(buf.Row(y))[x]
		return !strings.ContainsRune(glyph.GlyphRamp, r) && r != ' '
	}

	on := 0
	for _, p := range places {
		x, y := int(p.X), int(p.Y)
		if wall(x, y) || wall(x, y-1) || wall(x, y+1) {
			on++
		}
	}
	if want := len(places) * 3 / 4; on < want {
		t.Errorf("%d of %d places are on or beside a cell the corridor drew as wall, "+
			"want at least %d", on, len(places), want)
	}
}

// l turns the look off. The ribs and door frames use the theme's roles too, not
// the chrome's cyan, so they are not the one cyan thing under a Peak ceiling.
// Asserted through roles, so it holds on all nine palettes.
func TestTheLabIsWhiteWithoutBeingAskedAndSoIsItsStructure(t *testing.T) {
	for _, name := range []string{"lumon", "inverted"} {
		th, ok := theme.FindTheme(name)
		if !ok {
			t.Fatalf("no theme %q", name)
		}
		// Through the actual constructor, since the default is its to set.
		c := diverting(t, "CORRIDORS", 120, 40).halls
		if _, _, _, _, _, lumon, _ := c.Effects(); !lumon {
			t.Errorf("%s: the corridor did not open on the white lab", name)
		}
		if got := c.frameAttr(th, 1).FG; got == th.Peak {
			t.Errorf("%s: the structure reaches Peak; the ceiling is meant to be the "+
				"only thing that does", name)
		}

		// The walls must not also reach Peak: the ceiling is the brightest single thing
		// on screen, and "light above, shadow below" depends on it.
		if got := c.wallAttr(th, hit{}, 1).FG; got == th.Peak {
			t.Errorf("%s: the walls reach Peak as well as the ceiling", name)
		}
		if got := c.surfaceAttr(th, true, 1).FG; got != th.Peak {
			t.Errorf("%s: the ceiling nearest the eye is %v, want Peak", name, got)
		}

		// l gives the terminal its own colour back.
		c.Key("l")
		if got := c.frameAttr(th, 1).FG; got != th.Primary {
			t.Errorf("%s: with the look off the structure is %v, want the chrome's "+
				"own Primary %v", name, got, th.Primary)
		}
	}
}

// The building is white roof and walls, light grey doors and floor, grey
// further off and black further still: one luminance ramp, with every surface
// on it neutral. Palette roles each have their own hue, so mixing several would
// turn the corridor cyan.
// Positions are measured along the palette's own ramp from Ink to Peak, not as
// brightness: on the pale theme Peak is dark navy on white paper, so the
// corridor recedes into the page. Neutrality is measured against the palette's
// own endpoints (lumon's black is #1A1A2E, a slight blue): nothing on the ramp
// may be further off neutral than the two colours it is built from.
func TestTheWhiteLabIsGreyAndGoesToBlack(t *testing.T) {
	for _, name := range []string{"lumon", "inverted"} {
		th, ok := theme.FindTheme(name)
		if !ok {
			t.Fatalf("no theme %q", name)
		}
		c := diverting(t, "CORRIDORS", 120, 40).halls

		// Brightness, and how far off neutral, of one colour.
		read := func(col canvas.Color) (lum, cast float64) {
			r, g, b, ok := col.Parts()
			if !ok {
				t.Fatalf("%s: %v is not an RGB colour", name, col)
			}
			lo, hi := math.Min(float64(r), math.Min(float64(g), float64(b))),
				math.Max(float64(r), math.Max(float64(g), float64(b)))
			return (float64(r) + float64(g) + float64(b)) / 3, hi - lo
		}

		// How far along the ramp a colour sits: nought at the palette's Ink and one at
		// its Peak, whichever way round those are in brightness.
		black, _ := read(th.Ink)
		white, _ := read(th.Peak)
		along := func(col canvas.Color) float64 {
			lum, _ := read(col)
			return (lum - black) / (white - black)
		}

		// The palette's own cast is the budget: this code may not add to it.
		_, inkCast := read(th.Ink)
		_, peakCast := read(th.Peak)
		budget := math.Max(inkCast, peakCast) + 1

		near := []struct {
			what string
			attr canvas.Attr
		}{
			{"the ceiling", c.surfaceAttr(th, true, 1)},
			{"a wall", c.wallAttr(th, hit{}, 1)},
			{"the floor", c.surfaceAttr(th, false, 1)},
			{"a door", c.wallAttr(th, hit{isDoor: true}, 1)},
			{"the structure", c.frameAttr(th, 1)},
		}

		// Neutral, all of them.
		for _, s := range near {
			if _, cast := read(s.attr.FG); cast > budget {
				t.Errorf("%s: %s is %v, %.0f off neutral against the palette's own "+
					"%.0f; the lab has a colour of its own", name, s.what, s.attr.FG, cast, budget)
			}
		}

		roof := along(near[0].attr.FG)
		wall := along(near[1].attr.FG)
		floor := along(near[2].attr.FG)
		door := along(near[3].attr.FG)
		frame := along(near[4].attr.FG)

		// White roof and walls, light grey doors and floor, and the roof alone at the
		// top of the ramp.
		if roof <= wall {
			t.Errorf("%s: the ceiling is at %.2f of the ramp and a wall at %.2f; the "+
				"roof is not the brightest thing in the corridor", name, roof, wall)
		}
		if wall <= floor {
			t.Errorf("%s: a wall is at %.2f and the floor at %.2f; there is no light "+
				"above and shadow below", name, wall, floor)
		}
		if math.Abs(floor-door) > 0.01 || math.Abs(floor-frame) > 0.01 {
			t.Errorf("%s: floor %.2f, door %.2f, frame %.2f; the three light grey "+
				"surfaces are not the same grey", name, floor, door, frame)
		}

		// The far end is black on every surface, or the corridor recedes into a colour
		// and not into the dark.
		for _, s := range []struct {
			what string
			attr canvas.Attr
		}{
			{"the ceiling", c.surfaceAttr(th, true, 0)},
			{"a wall", c.wallAttr(th, hit{}, 0)},
			{"the floor", c.surfaceAttr(th, false, 0)},
			{"the structure", c.frameAttr(th, 0)},
		} {
			if s.attr.FG != th.Ink {
				t.Errorf("%s: %s at the far end is %v, want the palette's own black %v",
					name, s.what, s.attr.FG, th.Ink)
			}
		}

		// Half way is between the two, so it is a ramp and not two states. Measured on
		// the wall, the surface with the most of it on screen.
		if mid := along(c.wallAttr(th, hit{}, 0.5).FG); mid <= 0.01 || mid >= wall-0.01 {
			t.Errorf("%s: a wall half way down is at %.2f of the ramp, against %.2f "+
				"near and nought at the far end; it is not fading", name, mid, wall)
		}
	}
}

// digitAt must not advance a counter per wall cell: unless the cells drawn are a
// multiple of the board's digit count, the wall would take a fresh set of
// digits every frame. Asserted against the rendered buffer.
func TestTheWallHoldsStillWhileTheWalkerDoes(t *testing.T) {
	m := diverting(t, "CORRIDORS", 110, 32)
	c := m.halls
	panel := m.gridPanel().Play()

	at, head := c.at, c.heading.Value()
	freeze := func() {
		c.at = at
		c.heading.Set(head)
	}
	rows := func() []string {
		freeze()
		buf := drawTo(m)
		out := make([]string, 0, panel.H)
		for y := panel.Y; y < panel.Y+panel.H; y++ {
			out = append(out, buf.Row(y))
		}
		return out
	}

	// The doors beside the walker are still opening, which is meant to show.
	// Let them finish, so that only the walls are left to watch.
	c.doors(10)

	was := rows()
	for n := 0; n < 30; n++ {
		freeze()
		tick(m, 1)
		now := rows()
		for i := range was {
			if was[i] != now[i] {
				t.Fatalf("frame %d: row %d changed with the walker frozen:\n%q\n%q",
					n, i, was[i], now[i])
			}
		}
	}
}

// The digit is a pure function of the cell, the side, and the position across
// and down the face, so it slides with the surface and does not shimmer. The
// level of detail follows the drawn height in whole doublings, so a point gives
// the same digit at one distance and changes only at a doubling; continuous
// detail would re-letter every frame as the wall approaches.
func TestTheWallTextureBelongsToTheWall(t *testing.T) {
	c := mazeOf(t, 120, 30, 5)

	got := hit{cell: 42, side: 0, face: 0.37}
	first := c.wallDigit(got, 0.5, 20, 0)

	// The same place, asked again and again.
	for i := 0; i < 100; i++ {
		if again := c.wallDigit(got, 0.5, 20, 0); again != first {
			t.Fatalf("the same point on the same wall gave %q then %q", first, again)
		}
	}

	// A different wall, or a different place on this one, generally gives
	// something else: a texture, not one digit painted everywhere.
	seen := map[rune]int{}
	for cell := 0; cell < 40; cell++ {
		for u := 0; u < 20; u++ {
			for v := 0; v < 20; v++ {
				d := c.wallDigit(hit{cell: cell, face: float64(u) / 20}, float64(v)/20, 20, 0)
				if d < '0' || d > '9' {
					t.Fatalf("a wall carries %q, want one of the board's digits", d)
				}
				seen[d]++
			}
		}
	}
	if len(seen) < 8 {
		t.Errorf("the whole wall is drawn from %d different digits; it is not a texture",
			len(seen))
	}

	// The scroll moves it without reshuffling: the same point one scroll step on is
	// what was one step above it.
	up := c.wallDigit(got, 0.5, 20, 0)
	for s := 1; s < 5; s++ {
		if c.wallDigit(got, 0.5, 20, s) == up {
			continue // a coincidence at one step in ten, not a failure
		}
		return
	}
	t.Error("scrolling four steps changed nothing; the offset is not reaching the texture")
}

// A shear or a rock between limits fails at one of two angles:
//
//   - At half a turn a rotation is the picture upside down and back to front.
//     A shear leaves the ceiling on the ceiling at every phase.
//   - At a whole turn a rotation is the picture again, exactly. A rock lands
//     wherever its sine has got to.
//
// Both are compared with a frame drawn at the same c.age with the roll off, so
// they differ only by the turn. At these angles the sine is zero and the
// cosine plus or minus one, so the resample is exact.
func TestTheRollTurnsTheViewAllTheWayRound(t *testing.T) {
	m := diverting(t, "CORRIDORS", 110, 32)
	c := m.halls
	panel := m.gridPanel().Play()
	th := m.Theme

	// The bob is off: it moves the horizon the view turns about, and the half turn
	// would then be an inversion about a fractional row.
	c.fx.endless = false

	frameAt := func(age float64, roll bool) []string {
		c.age, c.fx.roll = age, roll
		buf := canvas.NewBuffer(panel.W, panel.H)
		c.Draw(buf, th, Panel{X: 0, Y: 0, W: panel.W, H: panel.H})
		out := make([]string, panel.H)
		for y := 0; y < panel.H; y++ {
			out[y] = buf.Row(y)
		}
		return out
	}

	// A whole turn takes 2pi/rollRate, and half of that is the half turn.
	whole := 2 * math.Pi / rollRate
	rest := frameAt(whole, false)
	round := frameAt(whole, true)
	half := frameAt(whole/2, true)
	flat := frameAt(whole/2, false)

	// Not cell for cell: math.Sin(2*math.Pi) is -2.4e-16, not nought, and on the
	// horizon row sample's test of |y - centre| < 0.5 flips across the axis, so
	// half that row may be drawn and half not, depending on the panel's parity.
	// The bound is one row of cells, far too few for the picture to have
	// turned.
	off := 0
	for y := range rest {
		a, b := []rune(rest[y]), []rune(round[y])
		for x := range a {
			if a[x] != b[x] {
				off++
			}
		}
	}
	if off > panel.W {
		t.Errorf("a whole turn left %d cells of %d different; it is not the picture "+
			"again", off, panel.W*panel.H)
	}

	// Upside down and back to front about the vanishing point: a cell at (col, row)
	// comes from (W-col, H-row), which is off the panel along the top row and the
	// left column. That is the same one column of slack the corridor's ray angles
	// have.
	rows := make([][]rune, len(flat))
	for y := range flat {
		rows[y] = []rune(flat[y])
	}
	turned, wrong := 0, 0
	for y, line := range half {
		got := []rune(line)
		for x := range got {
			sx, sy := panel.W-x, panel.H-y
			if sy >= len(rows) {
				// Row H, which the rolled frame may draw and the flat one never did (dropping
				// the row bound fills the screen at an angle). It has nothing to compare with.
				continue
			}
			want := ' '
			if sx < panel.W {
				want = rows[sy][sx]
			}
			if got[x] != want {
				// Counted, not fatal, and bounded by a row, as at the whole turn:
				// math.Sin(math.Pi) is 1.2e-16 and the horizon row is a knife-edge under it.
				wrong++
			}
			if got[x] != ' ' {
				turned++
			}
		}
	}
	if wrong > panel.W {
		t.Errorf("a half turn left %d cells of %d different from the picture "+
			"inverted", wrong, panel.W*panel.H)
	}
	if turned < panel.W {
		t.Fatalf("only %d cells of the half turn had anything in them; the roll is drawing nothing", turned)
	}

	// A quarter turn is a different picture; both exact angles are symmetries and
	// cannot show that alone.
	quarter := frameAt(whole/4, true)
	same := 0
	for y := range quarter {
		if quarter[y] == frameAt(whole/4, false)[y] {
			same++
		}
	}
	if same == len(quarter) {
		t.Error("a quarter turn drew the unrolled picture; the angle is not reaching the resample")
	}
}
