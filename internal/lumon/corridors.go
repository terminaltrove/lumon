package lumon

import (
	"math"
	"math/rand"

	"github.com/terminaltrove/lumon/internal/canvas"
	"github.com/terminaltrove/lumon/internal/ease"
	"github.com/terminaltrove/lumon/internal/glyph"
	"github.com/terminaltrove/lumon/internal/physics"
	"github.com/terminaltrove/lumon/internal/theme"
)

// The severed floor, seen from inside it: a maze carved fresh every time, with
// walls made of this file's own numbers, walked for ever. Doors open as you
// reach them and close behind you.
//
// It is a raycaster. For each panel column a ray is marched through the maze
// until it meets a wall, and the wall slice is drawn at a height of one over
// the distance. Camera is not involved; it projects points.
//
// It draws characters, not braille, because the braille layer takes one
// attribute per canvas and the walls need a colour and a digit per
// cell. Shading is glyph.EmergeRune fed one minus the distance: a near wall
// shows its digits and a far one fades into a ramp with no digit-like
// characters.
//
// The maze is carved by recursive backtracking, giving a perfect maze: one
// path between any two cells, no loops, nothing walled off
// (TestEveryCellOfTheMazeIsReachable). A broken carve still looks like a maze.

const (
	// corridorSpeed is how fast the cruise moves, in maze cells a second.
	corridorSpeed = 1.9

	// corridorTurn is the frequency of the critically damped spring that turns
	// the heading, in radians a second, so corners are curves.
	corridorTurn = 3.4

	// corridorReach is how near a cell's centre counts as arrived, in cells.
	// It must be above zero: a critically damped spring never quite arrives.
	corridorReach = 0.28

	// cornerSlow is the fraction of full speed the walk is down to when it is
	// heading exactly the wrong way. See the note in Step.
	cornerSlow = 0.35

	// corridorStall is how long the walk may get nowhere before it gives up on
	// its route and takes another, in seconds. See reroute. It must be longer
	// than a slow tight turn.
	corridorStall = 1.0

	// corridorRest is the entry in corridorPaces meaning ordinary speed, which is
	// what a pace offset of zero lands on.
	corridorRest = 2

	// The white lab is one luminance ramp and these are where each surface sits
	// on it, before distance is taken off. See labGrey. Only the ceiling reaches
	// Peak; walls stop just short, and floor and structure are light grey so
	// the corridor has edges.
	labCeiling   = 1.0
	labWall      = 0.88
	labStructure = 0.7
	labFloor     = 0.7

	// labBold is the tone above which a lab surface is drawn bold: the near
	// ceiling and walls.
	labBold = 0.75

	// silhouetteLeast is the fraction of columns that must have a wall edge on
	// screen for the arrival to follow the edges; below it the digits are laid
	// over the wall's face. See silhouette.
	silhouetteLeast = 0.25

	// corridorSkin is how near a wall the walk may come, in cells. See
	// slideOffWalls. It must stay larger than Draw's near clip, or a close wall
	// is clipped away.
	corridorSkin = 0.16

	// corridorFOV is the horizontal field of view, in radians (sixty degrees).
	// Wider distorts badly at terminal resolution.
	corridorFOV = math.Pi / 3

	// corridorDepth is how far a ray is followed, in cells. Past it the corridor
	// is dark and there is nothing to draw.
	corridorDepth = 14.0

	// corridorWall is the height of a wall at one cell's distance, as a fraction
	// of the panel; one over the distance scales it from there. It must be near
	// one for a wall at arm's length to fill the panel, leaving floor and
	// ceiling as margins.
	corridorWall = 0.95

	// corridorLit is how far the floor and the ceiling are lit, in cells. It is
	// well short of corridorDepth because a row's distance is
	// corridorWall*H/(2p), so all visible floor is within a few cells; shading
	// it out to corridorDepth would make every row the same.
	corridorLit = 3.2

	// doorEvery is roughly how many walls in one is a door, and doorSpeed how
	// fast one opens.
	doorEvery = 7
	doorSpeed = 2.2

	// doorJamb is how much of a door's face on each side is frame, and doorRail
	// how much of its height is lintel and threshold, as fractions of the door's
	// face so they scale with distance. See doorCell.
	doorJamb = 0.13
	doorRail = 0.11

	// The endless corridor from the end of the second series: no maze, no
	// turns, no end.
	//
	// hallWidth is the distance between the walls in cells, hallSpeed the run
	// speed in cells a second, and hallRib the spacing of the frame's ribs in
	// cells. Without ribs, parallel walls of tiling digits look identical every
	// frame. Much wider than one cell and the walls shrink to a window in the
	// middle of the view.
	hallWidth = 1.0
	hallSpeed = 4.6
	hallRib   = 3.0

	// hallBob is how far the view rises and falls, in rows, and hallBobRate how
	// often, in radians a second.
	hallBob     = 0.55
	hallBobRate = 7.0

	// roomCount is how many open rooms are knocked into the carve, and roomMin
	// and roomMax how big one is in cells each way. Rooms give the walk places
	// that open out; they stay smaller than the far clip.
	roomCount = 7
	roomMin   = 2
	roomMax   = 4
)

// Corridors is a maze and a walk through it.
type Corridors struct {
	// cols and rows are the maze in cells, and openRight/openDown which sides of
	// each are carved through. The other two sides are read off the neighbour.
	cols, rows int
	openRight  []bool
	openDown   []bool

	// door is how far open the wall between two cells is, keyed the same way
	// openRight and openDown are: right doors first, then down. Zero is shut.
	doorRight []float64
	doorDown  []float64
	isDoor    []bool // right, then down, in one slice of 2*n

	// where the walker is, in cells, and which way it faces.
	at      physics.Vec2
	heading *physics.Scalar
	aim     float64

	// path is the route being followed and step how far along it is, and
	// stalled how long it has been getting nowhere. See reroute.
	path    []int
	step    int
	stalled float64

	digits []rune
	next   int

	// fx is which effects are on, and age the clock they run off. See Key.
	fx  effects
	age float64

	// pace is the speed dial: steps away from ordinary speed, not an index into
	// corridorPaces, so the zero value is ordinary speed. Tests build this
	// struct literally and rely on that.
	pace int

	panel Panel
	rng   *rand.Rand
}

// NewCorridors carves a maze and puts you in it.
func NewCorridors(m *Model, panel Panel, rng *rand.Rand) *Corridors {
	// The white lab is the default; l turns it off.
	c := &Corridors{rng: rng, fx: effects{lumon: true}}

	l := m.Layout
	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			if n := m.Grid.At(gx, gy); n != nil {
				c.digits = append(c.digits, rune('0'+n.Value))
			}
		}
	}
	if len(c.digits) == 0 {
		c.digits = []rune("0")
	}

	c.Resize(panel)
	return c
}

// digitAt is the next board digit, round-robin. Walls must not use it; see
// wallDigit.
func (c *Corridors) digitAt() rune {
	d := c.digits[c.next%len(c.digits)]
	c.next++
	return d
}

// dropDigit is the digit one cell of the rain shows, hashed from the drop and
// the row so it is stable across frames.
func (c *Corridors) dropDigit(drop, row int) rune {
	h := uint64(int64(drop))*0x9E3779B97F4A7C15 + uint64(int64(row))*0x94D049BB133111EB
	h ^= h >> 33
	h *= 0xFF51AFD7ED558CCD
	h ^= h >> 29
	return c.digits[h%uint64(len(c.digits))]
}

// wallDigit is the digit at one point on a wall's face. It is a pure function
// of the cell, the side, and the position on the face, so the texture is
// stable across frames and slides with the wall. A shared counter would
// re-letter the wall every frame.
//
// It uses an integer hash like solidDigit, since smooth Perlin noise would
// streak.
func (c *Corridors) wallDigit(got hit, down, band float64, scroll int) rune {
	// Texture density follows the drawn size, about one digit per cell, in
	// whole doublings. The count is part of the hash, so a continuous count
	// would change every frame and shimmer.
	level := int(math.Round(math.Log2(math.Max(band, 1)))) + wallTexStep
	if level > wallTexFine {
		level = wallTexFine
	} else if level < wallTexCoarse {
		level = wallTexCoarse
	}
	rows := 1 << level

	u := int(ease.Clamp01(got.face) * float64(rows*wallTexWide))
	v := int(ease.Clamp01(down)*float64(rows)) + scroll

	h := uint64(int64(got.cell))*0x9E3779B97F4A7C15 +
		uint64(int64(got.side))*0xBF58476D1CE4E5B9 +
		uint64(int64(u))*0x94D049BB133111EB +
		uint64(int64(v))*0xD6E8FEB86659FD93
	h ^= h >> 33
	h *= 0xFF51AFD7ED558CCD
	h ^= h >> 29
	return c.digits[h%uint64(len(c.digits))]
}

// carve knocks the walls out with a recursive backtracker. It uses an explicit
// stack because the path can visit thousands of cells before backtracking.
func (c *Corridors) carve() {
	n := c.cols * c.rows
	c.openRight = make([]bool, n)
	c.openDown = make([]bool, n)

	seen := make([]bool, n)
	stack := []int{0}
	seen[0] = true

	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		cx, cy := cur%c.cols, cur/c.cols

		// The unvisited neighbours.
		var next []int
		var dir []int
		for d, off := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx, ny := cx+off[0], cy+off[1]
			if nx < 0 || ny < 0 || nx >= c.cols || ny >= c.rows {
				continue
			}
			if i := ny*c.cols + nx; !seen[i] {
				next = append(next, i)
				dir = append(dir, d)
			}
		}
		if len(next) == 0 {
			stack = stack[:len(stack)-1]
			continue
		}

		pick := c.rng.Intn(len(next))
		to, d := next[pick], dir[pick]

		// A wall is recorded on the cell to its left or above.
		switch d {
		case 0: // right
			c.openRight[cur] = true
		case 1: // left
			c.openRight[to] = true
		case 2: // down
			c.openDown[cur] = true
		case 3: // up
			c.openDown[to] = true
		}

		seen[to] = true
		stack = append(stack, to)
	}
}

// open reports whether you can walk from one maze cell to a neighbour.
func (c *Corridors) open(from, to int) bool {
	if from < 0 || to < 0 || from >= c.cols*c.rows || to >= c.cols*c.rows {
		return false
	}
	switch to - from {
	case 1:
		return from%c.cols != c.cols-1 && c.openRight[from]
	case -1:
		return to%c.cols != c.cols-1 && c.openRight[to]
	case c.cols:
		return c.openDown[from]
	case -c.cols:
		return c.openDown[to]
	}
	return false
}

// pathFrom is the shortest walk from start to the furthest reachable cell, as
// a list of cells, found breadth-first.
func (c *Corridors) pathFrom(start int) []int {
	n := c.cols * c.rows
	from := make([]int, n)
	for i := range from {
		from[i] = -1
	}
	from[start] = start

	queue := []int{start}
	last := start
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		last = cur
		for _, to := range [4]int{cur + 1, cur - 1, cur + c.cols, cur - c.cols} {
			if to < 0 || to >= n || from[to] != -1 || !c.open(cur, to) {
				continue
			}
			from[to] = cur
			queue = append(queue, to)
		}
	}

	var back []int
	for i := last; i != start; i = from[i] {
		back = append(back, i)
	}
	back = append(back, start)
	for l, r := 0, len(back)-1; l < r; l, r = l+1, r-1 {
		back[l], back[r] = back[r], back[l]
	}
	return back
}

// Resize re-boxes the view. The maze does not depend on the panel size.
func (c *Corridors) Resize(panel Panel) {
	c.panel = panel
	if c.cols < 1 || c.rows < 1 {
		c.build()
	}
}

// mazeCols and mazeRows are the maze's size in cells, large enough that the
// walk does not double back within sight.
const (
	mazeCols = 24
	mazeRows = 18
)

// build carves a maze, hangs doors in it, and puts the walker at one end.
func (c *Corridors) build() {
	c.cols, c.rows = mazeCols, mazeRows
	c.carve()
	c.rooms()
	c.hang()
	c.route()
}

// rooms knocks open floor into the finished carve. Opening walls after the
// carve keeps every cell reachable, though rooms add loops, so the maze is no
// longer perfect. It runs before hang so doors only go in walls that remain.
func (c *Corridors) rooms() {
	for n := 0; n < roomCount; n++ {
		w := roomMin + c.rng.Intn(roomMax-roomMin+1)
		h := roomMin + c.rng.Intn(roomMax-roomMin+1)
		if w >= c.cols || h >= c.rows {
			continue
		}
		x0, y0 := c.rng.Intn(c.cols-w), c.rng.Intn(c.rows-h)
		for y := y0; y < y0+h; y++ {
			for x := x0; x < x0+w; x++ {
				i := y*c.cols + x
				if x < x0+w-1 {
					c.openRight[i] = true
				}
				if y < y0+h-1 {
					c.openDown[i] = true
				}
			}
		}
	}
}

// hang turns roughly one shut wall in doorEvery into a door, so an open door
// is a shortcut.
func (c *Corridors) hang() {
	n := c.cols * c.rows
	c.doorRight = make([]float64, n)
	c.doorDown = make([]float64, n)
	c.isDoor = make([]bool, 2*n)

	for i := 0; i < n; i++ {
		if i%c.cols != c.cols-1 && !c.openRight[i] && c.rng.Intn(doorEvery) == 0 {
			c.isDoor[i] = true
		}
		if i/c.cols != c.rows-1 && !c.openDown[i] && c.rng.Intn(doorEvery) == 0 {
			c.isDoor[n+i] = true
		}
	}
}

// route picks somewhere to walk and puts the walker at the start of it.
func (c *Corridors) route() {
	c.path = c.pathFrom(c.rng.Intn(c.cols * c.rows))
	c.step = 0
	c.at = c.centreOf(c.path[0])
	c.aim = c.bearing()
	c.heading = physics.NewScalar(c.aim, corridorTurn)
}

// centreOf is the middle of a maze cell, in cells.
func (c *Corridors) centreOf(i int) physics.Vec2 {
	return physics.Vec2{X: float64(i%c.cols) + 0.5, Y: float64(i/c.cols) + 0.5}
}

// bearing is the direction the walk is steering in: the next cell on the
// route, or the one after once the walk is inside the next cell. The
// look-ahead turns corners into curves.
//
// Only look ahead from inside the route's cell. From anywhere else the line to
// the cell after can cut through a wall, the walk drifts off the route, and it
// wedges for ever (TestTheWalkKeepsGoing).
func (c *Corridors) bearing() float64 {
	if len(c.path) == 0 {
		return 0
	}
	at := c.step
	if at+1 < len(c.path) && c.cellAt(c.at) == c.path[at] {
		at++
	}
	to := c.centreOf(c.path[at])
	d := to.Sub(c.at)
	if d.LenSq() < 1e-9 {
		return c.aim
	}
	return math.Atan2(d.Y, d.X)
}

// blocked reports whether the wall between two neighbouring cells stops you. A
// door blocks until it is mostly open.
func (c *Corridors) blocked(from, to int) bool {
	if c.open(from, to) {
		return false
	}
	if d, ok := c.doorAt(from, to); ok && d >= 0.85 {
		return false
	}
	return true
}

// doorAt is how far open the door between two cells is, and whether there is one.
func (c *Corridors) doorAt(from, to int) (float64, bool) {
	n := c.cols * c.rows
	if from < 0 || to < 0 || from >= n || to >= n {
		return 0, false
	}
	switch to - from {
	case 1:
		return c.doorRight[from], c.isDoor[from]
	case -1:
		return c.doorRight[to], c.isDoor[to]
	case c.cols:
		return c.doorDown[from], c.isDoor[n+from]
	case -c.cols:
		return c.doorDown[to], c.isDoor[n+to]
	}
	return 0, false
}

// corridorPaces is the speed dial, as multiples of ordinary speed.
var corridorPaces = []float64{0.25, 0.5, 1, 2, 4}

// Step walks, turns, and works the doors.
//
// The pace dial scales dt and nothing else. Everything that moves here is
// driven by dt or c.age, so every effect slows together; the walk alone
// slowing while the rest runs at full speed makes people ill. corridorStall is
// measured in the same scaled time, so it is one second of walk time at every
// pace.
func (c *Corridors) Step(dt float64) {
	dt *= c.Pace()
	c.age += dt
	if c.fx.endless {
		c.runHall(dt)
		return
	}
	if c.cols < 1 || len(c.path) == 0 {
		return
	}

	c.doors(dt)

	c.aim = c.bearing()
	c.heading.Step(dt, nearestAngle(c.heading.Value(), c.aim))

	// Slow into a corner. At full speed the turn radius (corridorSpeed over
	// corridorTurn) is about 0.56 of a cell, too wide to turn inside a corner
	// cell, so the walk overshoots off the route. Scaling speed by the cosine
	// of the heading error brings it down to cornerSlow at a right angle,
	// turning inside a fifth of a cell.
	h := c.heading.Value()
	off := math.Cos(nearestAngle(h, c.aim) - h)
	pace := corridorSpeed * (cornerSlow + (1-cornerSlow)*math.Max(0, off))

	was := c.at
	c.at = c.at.Add(physics.Vec2{X: math.Cos(h), Y: math.Sin(h)}.Scale(pace * dt))
	c.slideOffWalls()

	// Two backstops against steering bugs, both meant never to fire.
	//
	// First: if the target cell is not reachable from the current cell, the
	// walk is off the route, so reroute at once. This checks open and doorAt,
	// not blocked, because routes go through shut doors.
	//
	// Second: reroute after corridorStall seconds without progress. A walk can
	// slide along a wall while wedged, so this alone does not catch the first
	// case.
	if here := c.cellAt(c.at); here != c.path[c.step] {
		if _, isDoor := c.doorAt(here, c.path[c.step]); !c.open(here, c.path[c.step]) && !isDoor {
			c.reroute()
			return
		}
	}
	if c.at.Sub(was).Len() < pace*dt*0.25 {
		c.stalled += dt
		if c.stalled > corridorStall {
			c.reroute()
			return
		}
	} else {
		c.stalled = 0
	}

	if c.reached(c.path[c.step]) {
		c.step++
		if c.step >= len(c.path) {
			// Arriving means choosing somewhere else.
			c.path = c.pathFrom(c.path[len(c.path)-1])
			c.step = 0
		}
	}
}

// reroute takes a fresh route from the cell the walk is standing in. It moves
// the walk to the cell's middle and sets the heading directly: letting the
// spring turn it from a wedged heading would aim back into the wall long
// enough to wedge again.
func (c *Corridors) reroute() {
	c.stalled = 0
	if c.cols < 1 {
		return
	}
	here := c.cellAt(c.at)
	if here < 0 {
		here = 0
	}
	c.path = c.pathFrom(here)
	c.step = 0
	c.at = c.centreOf(here)
	c.aim = c.bearing()
	c.heading.Set(c.aim)
}

// walled reports whether a wall stops you leaving a cell in one direction. The
// edge of the maze is one, since there is no way off this floor.
func (c *Corridors) walled(cx, cy, dx, dy int) bool {
	nx, ny := cx+dx, cy+dy
	if nx < 0 || ny < 0 || nx >= c.cols || ny >= c.rows {
		return true
	}
	return c.blocked(cy*c.cols+cx, ny*c.cols+nx)
}

// slideOffWalls keeps the walk at least corridorSkin from any wall of its
// cell, so cutting a corner never takes the camera through a wall
// (TestTheWalkStaysOnTheFloor). Walls are sides of axis-aligned unit cells, so
// it clamps each axis separately and the walk slides along a wall. A shut door
// holds the walk until it opens.
func (c *Corridors) slideOffWalls() {
	cx, cy := int(c.at.X), int(c.at.Y)
	if cx < 0 || cy < 0 || cx >= c.cols || cy >= c.rows {
		return
	}
	if c.walled(cx, cy, 1, 0) && c.at.X > float64(cx+1)-corridorSkin {
		c.at.X = float64(cx+1) - corridorSkin
	}
	if c.walled(cx, cy, -1, 0) && c.at.X < float64(cx)+corridorSkin {
		c.at.X = float64(cx) + corridorSkin
	}
	if c.walled(cx, cy, 0, 1) && c.at.Y > float64(cy+1)-corridorSkin {
		c.at.Y = float64(cy+1) - corridorSkin
	}
	if c.walled(cx, cy, 0, -1) && c.at.Y < float64(cy)+corridorSkin {
		c.at.Y = float64(cy) + corridorSkin
	}
}

// reached reports whether the walk is done with the cell it is heading for:
// standing in it, and either near its middle or past it.
//
// "Near the middle" alone lets the walk orbit a missed cell for ever, since
// the turn radius exceeds corridorReach. "Past it" without being inside lets a
// cornering walk advance to a cell diagonal to where it stands, and wedge.
// Requiring the walk to be inside the cell prevents both, because bearing aims
// at this cell until the walk enters it.
func (c *Corridors) reached(i int) bool {
	if c.cellAt(c.at) != i {
		return false
	}
	back := c.at.Sub(c.centreOf(i))
	if back.Len() < corridorReach {
		return true
	}
	h := c.heading.Value()
	return back.Dot(physics.Vec2{X: math.Cos(h), Y: math.Sin(h)}) > 0
}

// doors opens the one ahead and lets the rest fall shut.
func (c *Corridors) doors(dt float64) {
	here := c.cellAt(c.at)
	for i := range c.doorRight {
		want := 0.0
		if i == here || i+1 == here {
			want = 1
		}
		c.doorRight[i] = slide(c.doorRight[i], want, dt)
	}
	for i := range c.doorDown {
		want := 0.0
		if i == here || i+c.cols == here {
			want = 1
		}
		c.doorDown[i] = slide(c.doorDown[i], want, dt)
	}
}

// slide moves a value toward a target at doorSpeed.
func slide(v, want, dt float64) float64 {
	if v < want {
		return math.Min(want, v+doorSpeed*dt)
	}
	return math.Max(want, v-doorSpeed*dt)
}

// cellAt is which maze cell a position is in.
func (c *Corridors) cellAt(p physics.Vec2) int {
	x, y := int(p.X), int(p.Y)
	if x < 0 || y < 0 || x >= c.cols || y >= c.rows {
		return -1
	}
	return y*c.cols + x
}

// nearestAngle is b expressed as the value nearest a, so a heading crossing pi
// turns the short way.
func nearestAngle(a, b float64) float64 {
	for b-a > math.Pi {
		b -= 2 * math.Pi
	}
	for a-b > math.Pi {
		b += 2 * math.Pi
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// hit is what a ray found.
type hit struct {
	dist   float64
	cell   int     // the cell it stopped in
	side   int     // 0 crossing a vertical wall, 1 a horizontal one
	face   float64 // where along that wall's own face it struck, 0 to 1
	door   float64
	isDoor bool
	rib    bool // a frame rib of the endless corridor
}

// castHall is cast down the endless corridor: two parallel planes, so the hit
// distance is a division. A ray along the corridor is clipped at
// corridorDepth. The maze is left untouched while the hall runs.
func (c *Corridors) castHall(angle float64) hit {
	dx, dy := math.Cos(angle), math.Sin(angle)
	if math.Abs(dy) < 1e-9 {
		return hit{dist: corridorDepth}
	}

	wall := hallWidth
	side := 1
	if dy < 0 {
		wall, side = 0, 1
	}
	dist := (wall - c.at.Y) / dy
	if dist <= 0 || dist > corridorDepth {
		return hit{dist: corridorDepth}
	}

	// How far along the corridor it struck, for the texture and the ribs.
	along := c.at.X + dist*dx
	return hit{
		dist: dist,
		cell: int(math.Abs(along)) + side,
		side: side,
		face: along - math.Floor(along),
		rib:  math.Mod(math.Abs(along), hallRib) < 1,
	}
}

// runHall goes forward, for ever, with the heading pinned straight ahead.
func (c *Corridors) runHall(dt float64) {
	c.at.X += hallSpeed * dt
	c.at.Y = hallWidth / 2
	c.heading.Set(0)
}

// enterHall and leaveHall put the walker in the corridor and take it out
// again. Leaving picks a fresh route, since the walker is nowhere near the old
// one.
func (c *Corridors) enterHall() {
	c.at = physics.Vec2{X: 0, Y: hallWidth / 2}
	c.heading = physics.NewScalar(0, corridorTurn)
}

func (c *Corridors) leaveHall() {
	if c.cols > 0 {
		c.route()
	}
}

// cast marches one ray until it meets something. It steps from cell boundary
// to cell boundary (DDA), so walls are hit exactly and cost scales with cells
// crossed.
func (c *Corridors) cast(angle float64) hit {
	if c.fx.endless {
		return c.castHall(angle)
	}
	dx, dy := math.Cos(angle), math.Sin(angle)
	cx, cy := int(c.at.X), int(c.at.Y)

	stepX, stepY := 1, 1
	if dx < 0 {
		stepX = -1
	}
	if dy < 0 {
		stepY = -1
	}

	// Distance along the ray to the first boundary in each axis, and between
	// boundaries after that.
	var nextX, nextY, deltaX, deltaY float64
	if dx == 0 {
		nextX, deltaX = math.Inf(1), math.Inf(1)
	} else {
		deltaX = math.Abs(1 / dx)
		if dx > 0 {
			nextX = (float64(cx) + 1 - c.at.X) / dx
		} else {
			nextX = (c.at.X - float64(cx)) / -dx
		}
	}
	if dy == 0 {
		nextY, deltaY = math.Inf(1), math.Inf(1)
	} else {
		deltaY = math.Abs(1 / dy)
		if dy > 0 {
			nextY = (float64(cy) + 1 - c.at.Y) / dy
		} else {
			nextY = (c.at.Y - float64(cy)) / -dy
		}
	}

	from := cy*c.cols + cx
	for dist := 0.0; dist < corridorDepth; {
		side := 0
		if nextY < nextX {
			side = 1
		}
		if side == 0 {
			dist, cx = nextX, cx+stepX
			nextX += deltaX
		} else {
			dist, cy = nextY, cy+stepY
			nextY += deltaY
		}

		// Where along the struck wall's face the ray landed, which doors need
		// for their jambs and leaf.
		var face float64
		if side == 0 {
			face = c.at.Y + dist*dy
		} else {
			face = c.at.X + dist*dx
		}
		face -= math.Floor(face)

		to := -1
		if cx >= 0 && cy >= 0 && cx < c.cols && cy < c.rows {
			to = cy*c.cols + cx
		}
		if to < 0 {
			return hit{dist: dist, cell: from, side: side, face: face}
		}
		if c.blocked(from, to) {
			d, isDoor := c.doorAt(from, to)
			return hit{dist: dist, cell: from, side: side, face: face, door: d, isDoor: isDoor}
		}
		from = to
	}
	return hit{dist: corridorDepth, cell: from}
}

// Digits is the corridor's silhouette, one place per digit, so the board flies
// into the corridor it is about to be inside. It uses horizon and wallAt, as Draw
// does, and places digits along the wall band's two edges.
//
// The silhouette is sampled by arc length to exactly len(c.digits) points, as
// coil does, because the digit count depends on the grid and the column count
// on the screen. No Fade: the flight is the transition.
func (c *Corridors) Digits(panel Panel) []Placed {
	if c.cols < 1 || panel.W < 2 || panel.H < 1 || len(c.digits) == 0 {
		return nil
	}

	pts := c.silhouette(panel)
	if len(pts) < 2 {
		return nil
	}

	// Path length in visual columns, so steep parts are not bunched.
	run := make([]float64, len(pts))
	for i := 1; i < len(pts); i++ {
		d := pts[i].Sub(pts[i-1])
		run[i] = run[i-1] + math.Hypot(d.X, d.Y*RowAspect)
	}

	out := make([]Placed, 0, len(c.digits))
	total, at := run[len(run)-1], 0
	for i, d := range c.digits {
		want := 0.0
		if total > 0 {
			want = total * (float64(i) + 0.5) / float64(len(c.digits))
		}
		for at < len(pts)-2 && run[at+1] < want {
			at++
		}
		out = append(out, Placed{X: pts[at].X, Y: pts[at].Y, Digit: d})
	}
	return out
}

// silhouette is the path the arrival is sampled along: out along the ceiling
// edge of the wall band and back along its floor edge, as one continuous curve.
//
// Facing a wall close up, the edges are off the panel, and clamping them would
// stack every digit on the top and bottom rows. When fewer than
// silhouetteLeast of the columns have an edge in view, the wall's face is
// sampled in rows instead.
func (c *Corridors) silhouette(panel Panel) []physics.Vec2 {
	mid := c.horizon(panel)

	lo := float64(panel.Y)
	hi := float64(panel.Y + panel.H - 1)
	clamp := func(v float64) float64 { return math.Max(lo, math.Min(hi, v)) }

	tops := make([]float64, panel.W)
	bots := make([]float64, panel.W)
	seen := 0
	for col := 0; col < panel.W; col++ {
		top, bottom, _, _ := c.wallAt(col, panel, mid)
		tops[col] = float64(panel.Y) + top
		bots[col] = float64(panel.Y) + bottom
		if tops[col] >= lo && tops[col] <= hi {
			seen++
		}
	}

	// Nothing but wall: lay the digits over its face in rows.
	if float64(seen) < float64(panel.W)*silhouetteLeast {
		out := make([]physics.Vec2, 0, panel.W*4)
		for r := 0; r < 4; r++ {
			y := lo + (hi-lo)*(float64(r)+0.5)/4
			for k := 0; k < panel.W; k++ {
				col := k
				if r%2 == 1 {
					col = panel.W - 1 - k // boustrophedon, so the path is continuous
				}
				out = append(out, physics.Vec2{X: float64(panel.X + col), Y: y})
			}
		}
		return out
	}

	out := make([]physics.Vec2, 0, 2*panel.W)
	for col := 0; col < panel.W; col++ {
		out = append(out, physics.Vec2{X: float64(panel.X + col), Y: clamp(tops[col])})
	}
	for col := panel.W - 1; col >= 0; col-- {
		out = append(out, physics.Vec2{X: float64(panel.X + col), Y: clamp(bots[col])})
	}
	return out
}

// horizon is the row the middle of the view sits on. Draw and Digits share it.
func (c *Corridors) horizon(panel Panel) (mid float64) {
	mid = float64(panel.H) / 2

	// The run bobs the horizon. Moving the walker instead would show nothing,
	// since the corridor is symmetric about its axis.
	if c.fx.endless {
		mid += math.Sin(c.age*hallBobRate) * hallBob
	}
	return mid
}

// wallAt is where one column's slice of wall begins and ends, and what the ray
// found. It mutates nothing, so Digits and Draw can both call it.
func (c *Corridors) wallAt(col int, panel Panel, mid float64) (top, bottom, dist float64, got hit) {
	// The ray for this column. The cosine undoes the fish-eye, which would
	// bulge a flat wall toward the middle of the screen.
	off := (float64(col)/float64(panel.W) - 0.5) * corridorFOV
	got = c.cast(c.heading.Value() + off)
	dist = got.dist * math.Cos(off)
	if dist < 0.05 {
		dist = 0.05
	}

	height := corridorWall / dist * float64(panel.H)
	return mid - height/2, mid + height/2, dist, got
}

// column is one cast column's geometry, kept so the screen pass can look it up.
type column struct {
	top, bottom, dist float64
	got               hit
}

// sample is what the unrotated picture holds at one virtual cell, so Draw's
// rotated screen pass can look cells up by source position.
func (c *Corridors) sample(t *theme.Theme, panel Panel, cols []column, vcol int, y, centre float64, scroll int) (rune, canvas.Attr, bool) {
	if vcol < 0 || vcol >= len(cols) {
		return 0, canvas.Attr{}, false
	}
	k := cols[vcol]
	top, bottom, dist, got := k.top, k.bottom, k.dist, k.got

	if y < top || y > bottom {
		// Floor and ceiling: the wall's arithmetic run backwards. A wall at
		// distance d has its foot corridorWall*H/(2d) below the horizon, so a
		// row p from the horizon is floor (or ceiling) at that distance.
		p := math.Abs(y - centre)
		if p < 0.5 {
			return 0, canvas.Attr{}, false // the horizon itself, infinitely far off
		}
		d := corridorWall * float64(panel.H) / (2 * p)
		if d >= corridorLit {
			return 0, canvas.Attr{}, false // out past the light, where the floor is dark
		}
		near := 1 - d/corridorLit
		return surfaceRune(near), c.surfaceAttr(t, y < centre, near), true
	}

	shade := 1 - dist/corridorDepth
	if shade < 0 {
		shade = 0
	}
	// Horizontal walls are a shade darker, so corners show.
	if got.side == 1 {
		shade *= 0.72
	}

	down := 0.0
	if bottom > top {
		down = (y - top) / (bottom - top)
	}
	r := glyph.EmergeRune(c.wallDigit(got, down, bottom-top, scroll), shade)
	attr := c.wallAttr(t, got, shade)

	if got.rib {
		// A rib is drawn like a door's frame.
		r = glyph.GlyphSolid
		if !c.fx.alarm && !c.fx.colour {
			attr = c.frameAttr(t, shade)
		}
	}
	if got.isDoor {
		r, attr = c.doorCell(t, got, down, shade, r, attr)
	}
	return r, attr, true
}

// Draw paints the corridor.
//
// It casts one ray per column, then walks the screen cell by cell, rotating
// each cell's offset back by the roll angle to find the source column and row.
// Tilting columns would shear, not rotate. The rotation is in visual columns
// (rows scaled by RowAspect) so it is circular on screen. The screen pass
// visits source cells out of order, which is why the wall texture must not
// use a counter.
func (c *Corridors) Draw(buf *canvas.Buffer, t *theme.Theme, panel Panel) {
	if c.cols < 1 || panel.W < 1 || panel.H < 1 {
		return
	}
	mid := c.horizon(panel)

	cols := make([]column, panel.W)
	for col := 0; col < panel.W; col++ {
		top, bottom, dist, got := c.wallAt(col, panel, mid)
		cols[col] = column{top: top, bottom: bottom, dist: dist, got: got}
	}

	scroll := 0
	if c.fx.scroll {
		scroll = int(c.age * scrollSpeed)
	}

	sin, cos := 0.0, 1.0
	if c.fx.roll {
		a := c.age * rollRate
		sin, cos = math.Sin(a), math.Cos(a)
	}
	// Roll about the vanishing point (W/2, mid), not the panel's centre, or the
	// corridor lurches sideways as it turns.
	cx := float64(panel.W) / 2

	for row := 0; row < panel.H; row++ {
		for col := 0; col < panel.W; col++ {
			vcol, y := col, float64(row)
			if c.fx.roll {
				dx := float64(col) - cx
				dy := (float64(row) - mid) * RowAspect
				vcol = int(math.Round(cx + dx*cos + dy*sin))
				y = mid + (-dx*sin+dy*cos)/RowAspect
			}
			// No row bound: rotating a wide panel sends most cells to source
			// rows outside it, and the floor and ceiling arithmetic answers
			// those correctly.
			r, attr, ok := c.sample(t, panel, cols, vcol, y, mid, scroll)
			if !ok {
				continue
			}
			buf.Set(panel.X+col, panel.Y+row, r, attr)
		}
	}

	// Last, so it is in front of the corridor.
	if c.fx.rain {
		c.drawRain(buf, t, panel)
	}
}

// The corridor's effects, and the keys that turn them on. They are reachable
// only inside the maze. handleDivertingKey is not scanned by keys_test.go, so
// they need no bindingTable row, and they must not appear in Bindings()
// (TestTheDiversionsAreNotDocumented).
//
// r is taken by handleDivertingKey ("again") and a by handleKey (about), which
// runs first, so rotation is t and the alarm is w.

const (
	// alarmRate is how fast the alarm breathes, in cycles a second.
	alarmRate = 0.55

	// rainDrops is how many columns of matrix rain fall in front of the corridor,
	// and rainSpeed how fast, in rows a second.
	rainDrops = 26
	rainSpeed = 14.0

	// scrollSpeed is how fast the wall texture runs down, in characters a second.
	scrollSpeed = 9.0

	// wallTexWide is how much wider a wall's face is than tall, on screen. Both
	// scale as one over distance, so the ratio is fixed at about four columns
	// per row, and one level of detail serves both axes. See wallDigit.
	wallTexWide = 4

	// wallTexFine and wallTexCoarse bound how many digits a face has down its
	// height, as powers of two, and wallTexStep how many doublings finer than
	// the drawn height the texture is. At the drawn height exactly, a near face
	// shows runs of the same digit, which read as stripes.
	wallTexFine   = 7
	wallTexCoarse = 1
	wallTexStep   = 1

	// rollRate is how fast the view rolls, in radians a second.
	rollRate = 0.5
)

// effects is which of the seven are on.
type effects struct {
	colour  bool
	rain    bool
	scroll  bool
	roll    bool
	alarm   bool
	lumon   bool
	endless bool
}

// Key takes one of the maze's own keystrokes, and reports whether it was one.
func (c *Corridors) Key(key string) bool {
	switch key {
	case ",":
		if c.pace > -corridorRest {
			c.pace--
		}
	case ".":
		if c.pace < len(corridorPaces)-1-corridorRest {
			c.pace++
		}
	case "c":
		c.fx.colour = !c.fx.colour
	case "m":
		c.fx.rain = !c.fx.rain
	case "n":
		c.fx.scroll = !c.fx.scroll
	case "t":
		c.fx.roll = !c.fx.roll
	case "w":
		c.fx.alarm = !c.fx.alarm
	case "l":
		c.fx.lumon = !c.fx.lumon
	case "i":
		// The endless corridor replaces the maze until toggled off.
		c.fx.endless = !c.fx.endless
		if c.fx.endless {
			c.enterHall()
		} else {
			c.leaveHall()
		}
	default:
		return false
	}
	return true
}

// corridorPaceNames is how each entry in corridorPaces is written in the hint.
var corridorPaceNames = []string{"1/4x", "1/2x", "1x", "2x", "4x"}

// Pace is the speed dial's current multiple, for the tests.
func (c *Corridors) Pace() float64 { return corridorPaces[c.pace+corridorRest] }

// PaceLabel is how to write it in the hint line.
func (c *Corridors) PaceLabel() string { return corridorPaceNames[c.pace+corridorRest] }

// Effects is which are on, for the hint line and the tests.
func (c *Corridors) Effects() (colour, rain, scroll, roll, alarm, lumon, endless bool) {
	return c.fx.colour, c.fx.rain, c.fx.scroll, c.fx.roll,
		c.fx.alarm, c.fx.lumon, c.fx.endless
}

// alarmAttr is the wash the alarm puts over everything, pulsing between the
// theme's Urgent and Peak, as a fusing cluster does. It goes through Theme.On,
// or the pale theme shows the terminal's background through it.
//
// The phase is rate*time, which is only correct for a constant rate. To make
// it accelerate, integrate the rate as FuseFlash does.
func (c *Corridors) alarmAttr(t *theme.Theme) canvas.Attr {
	phase := 0.5 - 0.5*math.Cos(2*math.Pi*alarmRate*c.age)
	return t.On(theme.BlendColors(t.Urgent, t.Peak, phase), true)
}

// surfaceRune is the floor or the ceiling at a given nearness, from
// glyph.GlyphRamp. EmergeRune would draw digits competing with the walls, and
// RampRune snaps to GlyphSolid near the top, making a solid slab at your feet.
// The ramp's few characters give the floor visible bands.
func surfaceRune(near float64) rune {
	ramp := []rune(glyph.GlyphRamp)
	i := int((1 - ease.Clamp01(near)) * float64(len(ramp)))
	if i < 0 {
		i = 0
	}
	if i >= len(ramp) {
		i = len(ramp) - 1
	}
	return ramp[i]
}

// labGrey is the white lab's one luminance ramp, from the theme's Ink to its
// Peak, so it is white on every theme. Each surface has a base tone, and
// nearness multiplies it, so every surface fades toward black in the same
// order.
func (c *Corridors) labGrey(t *theme.Theme, base, near float64) canvas.Attr {
	tone := base * ease.Clamp01(near)
	return t.On(theme.BlendColors(t.Ink, t.Peak, tone), tone > labBold)
}

// surfaceAttr is what the floor and the ceiling are drawn in: the alarm wash,
// the lab greys, or else Dim so they sit behind the walls.
func (c *Corridors) surfaceAttr(t *theme.Theme, up bool, near float64) canvas.Attr {
	if c.fx.alarm {
		return c.alarmAttr(t)
	}
	if c.fx.lumon {
		if up {
			return c.labGrey(t, labCeiling, near)
		}
		return c.labGrey(t, labFloor, near)
	}
	return t.On(t.Dim, false)
}

// frameAttr is what the structure is drawn in: a door's frame, and the ribs of
// the endless corridor. In the lab it is light grey, faded with distance;
// otherwise flat Primary like the chrome.
func (c *Corridors) frameAttr(t *theme.Theme, near float64) canvas.Attr {
	if c.fx.lumon {
		return c.labGrey(t, labStructure, near)
	}
	return t.On(t.Primary, true)
}

// wallAttr is what a wall is drawn in.
func (c *Corridors) wallAttr(t *theme.Theme, got hit, shade float64) canvas.Attr {
	if c.fx.alarm {
		return c.alarmAttr(t)
	}
	if c.fx.colour {
		// Cycle the desk colours. Colour wins over the lab look; with both on,
		// walls are coloured under a white ceiling.
		return t.On(t.DeskColour(got.cell+got.side), false)
	}
	if c.fx.lumon {
		// A door is light grey like its frame, so a closed door is visible
		// down the corridor.
		if got.isDoor {
			return c.labGrey(t, labStructure, shade)
		}
		// Just under the ceiling's tone.
		return c.labGrey(t, labWall, shade)
	}
	// After the lab branch, or doors would be cyan in the white corridor.
	if got.isDoor {
		return t.BadLive
	}
	if shade > 0.72 {
		return t.Hovered
	}
	return t.Digit
}

// doorCell is one cell of a door: its frame, its leaf, or the gap the leaf has
// slid out of. The pieces are read off fractions across (got.face) and down
// the door's face, so they scale with distance.
//
// The gap is drawn dark; showing the corridor beyond would need a second cast.
// A door is only drawn up to 85% open, since past that blocked lets the ray
// through.
func (c *Corridors) doorCell(t *theme.Theme, got hit, down, shade float64, r rune, attr canvas.Attr) (rune, canvas.Attr) {
	frame := c.wallAttr(t, got, shade)
	if !c.fx.alarm && !c.fx.colour {
		frame = c.frameAttr(t, shade)
	}
	if got.face < doorJamb || got.face > 1-doorJamb || down < doorRail || down > 1-doorRail {
		return glyph.GlyphSolid, frame
	}

	// Position across the opening inside the frame; the leaf covers the part
	// not yet slid open.
	across := (got.face - doorJamb) / (1 - 2*doorJamb)
	if across > 1-ease.Clamp01(got.door) {
		return surfaceRune(0), t.On(t.Lurk, false)
	}
	return r, attr
}

// drawRain lays a fall of digits in front of the corridor. The columns are
// fixed and only the heads move, as in MATRIX.
func (c *Corridors) drawRain(buf *canvas.Buffer, t *theme.Theme, panel Panel) {
	attr := t.Hovered
	if c.fx.alarm {
		attr = c.alarmAttr(t)
	}
	for i := 0; i < rainDrops; i++ {
		// Columns come from the drop's index, so a resize does not reshuffle
		// the fall.
		x := panel.X + (i*7919)%maxInt(panel.W, 1)
		head := math.Mod(c.age*rainSpeed+float64(i)*3.7, float64(panel.H)+8) - 4
		for k := 0; k < 5; k++ {
			y := int(head) - k
			if y < 0 || y >= panel.H {
				continue
			}
			a := attr
			if k > 1 {
				a = t.Digit
				if c.fx.alarm {
					a = attr
				}
			}
			// Hashed by drop and row, so the fall moves down a fixed column of
			// digits.
			buf.Set(x, panel.Y+y, c.dropDigit(i, y), a)
		}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
