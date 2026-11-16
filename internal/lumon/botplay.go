package lumon

import (
	"math"
	"math/rand"
	"sort"
)

// How a computer refiner plays. Each has a temperament drawn once: its pace,
// how long it works before a break, how often it gathers several clusters or
// looks one over and moves on. Every wait is drawn too, mostly short with now
// and then a long hesitation, so no two refiners keep the same rhythm and
// none keeps a rhythm at all.

// bot is one computer refiner.
type bot struct {
	desk *Desk

	// rng is the bot's own, so its choices never draw from the board's stream
	// and shift a seeded board.
	rng *rand.Rand

	// The temperament: seconds per cell walked, the mean length of a working
	// spell and of a break, and the chances that a pick gathers more clusters
	// and that a cluster is looked over and left.
	pace, focus, rest float64
	gatherer, browse  float64

	// plan is the clusters still to reach, the first the one walked to; held
	// is those reached, refined together once the plan is done.
	plan, held []int

	// goalX and goalY are the cell walked to, and walk the time saved toward
	// the next step.
	goalX, goalY int
	walk         float64

	// wait is a pause before acting; hover how long the bot has sat on its
	// cluster, and hoverFor how long it will.
	wait, hover, hoverFor float64

	// working is what is left of this working spell, resting of this break,
	// and fidget the time to the next idle nudge of the cursor on a break.
	working, resting, fidget float64

	// passed is clusters looked over and left, by the board time until which
	// they are not picked again.
	passed map[int]float64

	// wandering is a walk toward a sleeping cluster with nothing awake to
	// refine. Clusters wake only where somebody looks, so a bot that stood
	// still would leave the far corners of a file asleep for good.
	wandering bool

	// view is its window on the board, for waking clusters where it looks.
	view [1]Viewport
}

func newBot(d *Desk, rng *rand.Rand) *bot {
	b := &bot{
		desk:     d,
		rng:      rng,
		pace:     1 / (7 + 5*rng.Float64()),
		focus:    25 + 35*rng.Float64(),
		rest:     5 + 8*rng.Float64(),
		gatherer: 0.12 + 0.18*rng.Float64(),
		browse:   0.08 + 0.12*rng.Float64(),
		passed:   map[int]float64{},
	}
	b.working = b.delay(b.focus) + 8
	return b
}

// delay is a wait of about mean seconds: exponential, so mostly short, and now
// and then a long hesitation on top.
func (b *bot) delay(mean float64) float64 {
	d := -math.Log(1-b.rng.Float64()) * mean
	if b.rng.Float64() < 0.07 {
		d += 1.5 + 3*b.rng.Float64()
	}
	return d
}

// Board times for a cluster's age, in seconds: past urgentAfter one breaks a
// rest, and past keepAfter one is never left once looked at. A cluster
// detonates at about 120.
const (
	urgentAfter = 70.0
	keepAfter   = 60.0
)

// stepBots moves every computer refiner on by dt seconds, in the order they
// sat down. The caller holds the lock, which Step's callers do.
func (r *Room) stepBots(dt float64) {
	for _, b := range r.bots {
		r.stepBot(b, dt)
	}
}

func (r *Room) stepBot(b *bot, dt float64) {
	b.look()
	r.MaybeActivate(b.view[:])
	r.publishReach(b)

	if r.Complete() {
		b.plan, b.held = b.plan[:0], b.held[:0]
		return
	}
	if b.wait > 0 {
		b.wait -= dt
		return
	}

	if b.resting > 0 {
		b.resting -= dt
		if !r.urgent(b) {
			b.fidgetOn(dt)
			return
		}
		b.resting = 0 // something is about to go off
	}
	if b.working -= dt; b.working <= 0 && len(b.held) == 0 {
		b.plan, b.wandering = b.plan[:0], false
		b.resting = b.delay(b.rest) + 2
		b.working = b.delay(b.focus) + 8
		b.fidget = 0.5
		return
	}

	r.forgetGone(b)
	if len(b.plan) == 0 && len(b.held) > 0 {
		r.refineHeld(b)
		return
	}
	if len(b.plan) == 0 {
		if r.choose(b) {
			b.wandering = false
			b.wait = b.delay(0.6) + 0.25 // noticing it
			return
		}
		if !b.wandering {
			b.wandering = r.wander(b)
		}
	}

	if b.walkOn(dt) {
		return
	}
	if len(b.plan) == 0 {
		b.wandering, b.wait = false, b.delay(0.8)
		return
	}

	if b.hover += dt; b.hover < b.hoverFor {
		return
	}
	b.hover = 0
	target := b.plan[0]
	if len(b.held) == 0 && len(b.plan) == 1 && b.rng.Float64() < b.browse && r.Awake(target) < keepAfter {
		b.passed[target] = r.stepped + 8
		b.plan = b.plan[:0]
		b.wait = b.delay(0.5)
		return
	}
	b.held = append(b.held, target)
	b.plan = b.plan[1:]
	if len(b.plan) > 0 {
		b.aim(r)
		b.hoverFor = b.delay(0.2) + 0.1
	}
}

// walkOn steps the cursor toward the goal at the bot's pace, and reports
// whether it is still on the way.
func (b *bot) walkOn(dt float64) bool {
	d := b.desk
	if d.CursorX == b.goalX && d.CursorY == b.goalY {
		return false
	}
	b.walk += dt
	for b.walk >= b.pace && (d.CursorX != b.goalX || d.CursorY != b.goalY) {
		b.walk -= b.pace
		b.stepToward()
	}
	return true
}

// refineHeld refines what the bot gathered, each cluster into the bin
// furthest behind, and starts it looking again.
func (r *Room) refineHeld(b *bot) {
	d := b.desk
	for _, id := range b.held {
		if !r.IsActive(id) || r.reachedByOther(id, d) {
			continue
		}
		bin := r.openBinNear(d.Bin)
		if bin < 0 {
			break
		}
		d.Bin = bin
		if grp := r.Grid.Group(id); grp != nil {
			r.refineInto(grp, bin, d)
			r.lastBot = d.Seat
		}
	}
	b.held = b.held[:0]
	if d.CalloutUntil <= r.stepped && b.rng.Intn(14) == 0 {
		d.Callout = callouts[b.rng.Intn(len(callouts))]
		d.CalloutUntil = r.stepped + calloutHold
	}
	b.wait = b.delay(0.5) + 0.15
}

// choose plans the bot's next clusters: the awake one most worth its time
// (the longest ignored, less a little for distance), and sometimes one or two
// near it, or rarely every one free. It reports whether there was any.
func (r *Room) choose(b *bot) bool {
	d := b.desk
	var free []int
	best, bestScore := -1, 0.0
	for _, c := range r.actives {
		id := c.grp.ID
		if r.reachedByOther(id, d) || r.botAfter(id, b) || b.passed[id] > r.stepped {
			continue
		}
		free = append(free, id)
		cx, cy := nearestCell(c.grp.NumberIDs, d.CursorX, d.CursorY)
		score := c.elapsed - float64(abs(cx-d.CursorX)+abs(cy-d.CursorY))*b.pace*2
		if best < 0 || score > bestScore {
			best, bestScore = id, score
		}
	}
	for id, until := range b.passed {
		if until <= r.stepped {
			delete(b.passed, id)
		}
	}
	if best < 0 {
		return false
	}
	b.plan = append(b.plan[:0], best)

	switch roll := b.rng.Float64(); {
	case roll < 0.03 && len(free) > 2:
		for _, id := range free {
			if id != best {
				b.plan = append(b.plan, id)
			}
		}
	case roll < b.gatherer:
		b.plan = append(b.plan, r.near(best, free, 1+b.rng.Intn(2))...)
	}
	b.orderPlan(r)
	b.aim(r)
	b.hoverFor = b.delay(0.35) + 0.2
	return true
}

// near is up to n of free, other than id, within reach of it, nearest first.
func (r *Room) near(id int, free []int, n int) []int {
	const reach = 18 // cells
	ax, ay := nearestCell(r.Grid.Group(id).NumberIDs, 0, 0)
	type dist struct{ id, d int }
	var close []dist
	for _, other := range free {
		if other == id {
			continue
		}
		x, y := nearestCell(r.Grid.Group(other).NumberIDs, ax, ay)
		if d := abs(x-ax) + abs(y-ay); d <= reach {
			close = append(close, dist{other, d})
		}
	}
	sort.Slice(close, func(i, j int) bool {
		return close[i].d < close[j].d || close[i].d == close[j].d && close[i].id < close[j].id
	})
	var out []int
	for i := 0; i < len(close) && i < n; i++ {
		out = append(out, close[i].id)
	}
	return out
}

// orderPlan visits the plan's clusters nearest-first from the cursor, keeping
// its first.
func (b *bot) orderPlan(r *Room) {
	if len(b.plan) < 3 {
		return
	}
	x, y := nearestCell(r.Grid.Group(b.plan[0]).NumberIDs, b.desk.CursorX, b.desk.CursorY)
	rest := b.plan[1:]
	for i := range rest {
		nearest := i
		bestD := -1
		for j := i; j < len(rest); j++ {
			cx, cy := nearestCell(r.Grid.Group(rest[j]).NumberIDs, x, y)
			if d := abs(cx-x) + abs(cy-y); bestD < 0 || d < bestD {
				nearest, bestD = j, d
			}
		}
		rest[i], rest[nearest] = rest[nearest], rest[i]
		x, y = nearestCell(r.Grid.Group(rest[i]).NumberIDs, x, y)
	}
}

// aim points the walk at the nearest cell of the plan's first cluster.
func (b *bot) aim(r *Room) {
	if grp := r.Grid.Group(b.plan[0]); grp != nil {
		b.goalX, b.goalY = nearestCell(grp.NumberIDs, b.desk.CursorX, b.desk.CursorY)
	}
}

// forgetGone drops clusters from the plan and the hold that have been refined
// or detonated, or that somebody else is reaching for.
func (r *Room) forgetGone(b *bot) {
	keep := func(ids []int) []int {
		out := ids[:0]
		for _, id := range ids {
			if r.IsActive(id) && !r.reachedByOther(id, b.desk) {
				out = append(out, id)
			}
		}
		return out
	}
	first := -1
	if len(b.plan) > 0 {
		first = b.plan[0]
	}
	b.plan, b.held = keep(b.plan), keep(b.held)
	if len(b.plan) > 0 && b.plan[0] != first {
		b.aim(r)
		b.hover = 0
	}
}

// publishReach is what the bot is reaching for, as a person's ringfence and
// hover are: what it holds, and the cluster under its cursor. Reaching lifts a
// cluster on everyone's screen and puts it on the map.
func (r *Room) publishReach(b *bot) {
	d := b.desk
	d.Reach = append(d.Reach[:0], b.held...)
	if len(b.plan) > 0 && b.on(r, b.plan[0]) {
		d.Reach = append(d.Reach, b.plan[0])
	}
	for _, id := range d.Reach {
		r.Found(id)
	}
}

// urgent reports whether an awake cluster nobody else is after is close to
// going off, which ends a break.
func (r *Room) urgent(b *bot) bool {
	for _, c := range r.actives {
		if c.elapsed > urgentAfter && !r.reachedByOther(c.grp.ID, b.desk) && !r.botAfter(c.grp.ID, b) {
			return true
		}
	}
	return false
}

// fidgetOn nudges the cursor a cell now and then on a break, as a hand on the
// keys does.
func (b *bot) fidgetOn(dt float64) {
	if b.fidget -= dt; b.fidget > 0 {
		return
	}
	b.fidget = 0.6 + 2.4*b.rng.Float64()
	d := b.desk
	switch b.rng.Intn(4) {
	case 0:
		d.CursorX = clampInt(d.CursorX+1, 0, GridSize-1)
	case 1:
		d.CursorX = clampInt(d.CursorX-1, 0, GridSize-1)
	case 2:
		d.CursorY = clampInt(d.CursorY+1, 0, GridSize-1)
	default:
		d.CursorY = clampInt(d.CursorY-1, 0, GridSize-1)
	}
	b.goalX, b.goalY = d.CursorX, d.CursorY
}

// wander sends the bot toward a sleeping cluster chosen at random, and reports
// whether there was one left.
func (r *Room) wander(b *bot) bool {
	n := 0
	pick := -1
	for i, grp := range r.Grid.Groups {
		if r.IsActive(grp.ID) || len(grp.NumberIDs) == 0 || r.Grid.Cells[grp.NumberIDs[0]].BadGroupID != grp.ID {
			continue
		}
		// One pass, each sleeping cluster equally likely.
		n++
		if b.rng.Intn(n) == 0 {
			pick = i
		}
	}
	if pick < 0 {
		return false
	}
	grp := r.Grid.Groups[pick]
	b.goalX, b.goalY = nearestCell(grp.NumberIDs, b.desk.CursorX, b.desk.CursorY)
	return true
}

// reachedByOther reports whether anyone but desk is reaching for the cluster.
func (r *Room) reachedByOther(id int, desk *Desk) bool {
	for _, d := range r.desk {
		if d == nil || d == desk {
			continue
		}
		for _, reach := range d.Reach {
			if reach == id {
				return true
			}
		}
	}
	return false
}

// botAfter reports whether another computer refiner plans or holds the
// cluster.
func (r *Room) botAfter(id int, b *bot) bool {
	for _, other := range r.bots {
		if other == b {
			continue
		}
		for _, ids := range [2][]int{other.plan, other.held} {
			for _, x := range ids {
				if x == id {
					return true
				}
			}
		}
	}
	return false
}

// openBinNear is the bin furthest behind, once the credit still flying lands,
// the nearest to want of any as far behind, or -1 if none has room. Filling
// the emptiest first keeps all five going, though there are only four desks
// to call bins their own. Bin.Full lags creditDelay behind a refine, so it
// cannot be trusted alone.
func (r *Room) openBinNear(want int) int {
	var flying [BinCount]int
	for _, c := range r.creditIn {
		if c.bin >= 0 && c.bin < BinCount {
			flying[c.bin]++
		}
	}
	best, behind := -1, 2.0
	for step := 0; step < BinCount; step++ {
		for _, i := range [2]int{want - step, want + step} {
			if i < 0 || i >= BinCount {
				continue
			}
			b := r.Grid.Bins[i]
			if b.Quota <= 0 || b.Refined+flying[i] >= b.Quota {
				continue
			}
			// Strictly less, so a tie goes to the bin nearer want.
			if f := float64(b.Refined+flying[i]) / float64(b.Quota); f < behind-1e-9 {
				best, behind = i, f
			}
		}
	}
	return best
}

// on reports whether the bot's cursor is on the cluster.
func (b *bot) on(r *Room, id int) bool {
	n := r.Grid.At(b.desk.CursorX, b.desk.CursorY)
	return n != nil && n.BadGroupID == id
}

// stepToward moves the cursor one cell toward the goal, along the longer way
// first, as a person on the arrow keys does.
func (b *bot) stepToward() {
	d := b.desk
	dx, dy := b.goalX-d.CursorX, b.goalY-d.CursorY
	if abs(dx) >= abs(dy) {
		d.CursorX += sign(dx)
	} else {
		d.CursorY += sign(dy)
	}
}

// look centres the bot's window on its cursor and publishes it for the map.
func (b *bot) look() {
	d := b.desk
	x := clampInt(d.CursorX-botViewW/2, 0, GridSize-botViewW)
	y := clampInt(d.CursorY-botViewH/2, 0, GridSize-botViewH)
	b.view[0] = Viewport{X: x, Y: y, W: botViewW, H: botViewH}
	d.ViewX, d.ViewY, d.ViewW, d.ViewH = x, y, botViewW, botViewH
}

// nearestCell is the member of ids nearest (x, y), in grid cells.
func nearestCell(ids []int, x, y int) (int, int) {
	bx, by, best := x, y, -1
	for _, id := range ids {
		cx, cy := id%GridSize, id/GridSize
		if dist := abs(cx-x) + abs(cy-y); best < 0 || dist < best {
			bx, by, best = cx, cy, dist
		}
	}
	return bx, by
}
