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

// The bins fire the file into the sky: shells go up out of the boxes, burst at
// the top of their climb, and the sparks fall and go out.
//
// A volley says which bins are open (BinFill.SetHold), and a bin fires only
// once its own lid is clear.
//
// A shell is a spark whose life is its fuse, in the same arrays. When a
// particle expires or leaves the panel, a shell (from >= 0) bursts and a spark
// goes home to a bin.
//
// Every digit is in exactly one place, queued at a bin or carried by one
// particle, so the count never changes and the file caps the particle count.
//
// Positions are in visual columns: x a column, y a row times RowAspect, so a
// burst comes out round.

const (
	// skyGravity is the pull, in visual columns a second squared (the same as
	// the fluid's).
	skyGravity = 26.0

	// skyRise is the fraction of the panel a shell climbs before it bursts.
	// Shells have no drag, so the launch speed is sqrt(2gh) and the fuse v/g is
	// exactly the time to the top, at any panel height. There is room left
	// above the burst so the sparks are not cut off by the ceiling.
	skyRise = 0.82

	// shellGap is the least time between two shells out of the same bin, in
	// seconds.
	shellGap = 0.55

	// skyCrowd is the fraction of the file that may be in the air before a
	// bin holds fire, so a new shell is not lost in a full sky. (The digit
	// count is the hard cap.)
	skyCrowd = 0.55

	// lidClear is how far open a bin has to be before it will fire, in
	// degrees: half open, so the box is seen to open first (Ajar is true
	// almost at once). The lid takes about a fifth of a second to get here, so
	// a volley window shorter than that never fires.
	lidClear = LidOpen * 0.5

	// skyTrail is how many cells of tail a spark is drawn with.
	skyTrail = 3

	// mineSpread is how much of the circle a mine's cone leaves out, in
	// radians. It throws upward over the rest, a curtain 120 degrees wide.
	mineSpread = 2 * math.Pi / 3

	// cometApex is how far up the panel a comet gets at the top of its arc, as
	// a fraction. The crossing time follows from it; see cometThrow.
	cometApex = 0.45

	// skySparkRef is the panel height the burst speeds in burstShapes were
	// tuned against, in visual columns (21 rows, at 120x40). f.spark scales
	// them by the actual panel height.
	skySparkRef = 21 * RowAspect

	// burstLeast is the fewest digits a bin needs to burst. The shapes want 16
	// to 20, but at 80x24 a bin holds only about twelve, so requiring the full
	// count would stop most volleys firing at all. burst clamps to what the
	// queue holds.
	burstLeast = 6

	// fanSweep is how far a fan's angle turns between one spark and the next,
	// in radians, fanSpread how far either side of vertical it reaches, and
	// fanLift how much harder it throws than the shape's own burst, since it
	// starts on the bottom row under its lid.
	//
	// A fan cannot use a high-drag shape: the willow's drag makes a burst hang
	// in the air, but from the floor its sparks never leave the box. SWEEP fans
	// peonies for that reason.
	fanSweep  = 0.16
	fanSpread = 0.55
	fanLift   = 1.5

	// mineLift is how much harder a mine throws than the shape's own burst. A
	// mine starts on the bottom row under the lid, and at the shape's speed a
	// ring rises only about three rows.
	mineLift = 1.8

	// fanReserve is the fraction of a bin's fair share of the file a fan leaves
	// behind, so the next volley has something to fire. Spent sparks mostly
	// come back to the bin that threw them (the nearest mouth).
	fanReserve = 0.25
)

// burstKind is what a shell turns into.
type burstKind uint8

const (
	peony burstKind = iota
	willow
	ringBurst
	crackle
	burstKinds
)

// burstSpec is one shape. Drag (the fraction of speed kept per second, per
// particle) is what distinguishes them: a willow's low value makes it slow at
// once, then hang and droop.
type burstSpec struct {
	name  string
	count int
	speed float64
	drag  float64
	life  float64

	// even is whether every spark leaves at the same speed, making a ring.
	even bool
}

var burstShapes = [burstKinds]burstSpec{
	peony:     {name: "PEONY", count: 16, speed: 21, drag: 0.55, life: 1.7},
	willow:    {name: "WILLOW", count: 14, speed: 15, drag: 0.10, life: 2.4},
	ringBurst: {name: "RING", count: 18, speed: 18, drag: 0.50, life: 1.5, even: true},
	crackle:   {name: "CRACKLE", count: 20, speed: 25, drag: 0.35, life: 0.9},
}

// figure is what a bin does when it fires; burstKind is what a shell turns
// into. The figure owns the launch and the shape owns the spark, so they
// combine freely: fire has one arm per figure, and burst and settle do not
// care which figure launched a shell.
type figure uint8

const (
	// shellUp goes up out of the box and bursts at the top.
	shellUp figure = iota

	// comet is thrown across the panel and bursts at the far side.
	comet

	// mine is the box itself going off, a wide low curtain with no shell at all.
	mine

	// fan is a sustained spray whose angle sweeps from side to side while the
	// lid is open. It never bursts.
	fan
)

// volley is one figure of the programme: which bins are open, for how long, and
// what they are firing. Each has its own shape, which the b key overrides.
type volley struct {
	name  string
	secs  float64
	shape burstKind
	how   figure

	// mixed takes a fresh shape per shell instead of the one above.
	mixed bool

	// open says whether bin i of n is open, t seconds in.
	open func(t float64, i, n int) bool
}

var volleys = []volley{
	{name: "ROLL", secs: 6, shape: peony, how: shellUp, open: func(t float64, i, n int) bool {
		// One at a time, left to right. Each window (secs/n) is well over
		// the lid's rise time.
		return i == int(t/6*float64(n))%n
	}},
	{name: "SWEEP", secs: 8, shape: peony, how: fan, open: func(t float64, i, n int) bool {
		// There and back.
		u := t / 8 * 2
		if u > 1 {
			u = 2 - u
		}
		return i == int(u*float64(n-1)+0.5)
	}},
	{name: "OUTWARD", secs: 6, shape: ringBurst, how: mine, open: func(t float64, i, n int) bool {
		// The middle, then its neighbours, then the ends: three windows of two
		// seconds, by how far off centre a bin is.
		mid := float64(n-1) / 2
		return int(math.Abs(float64(i)-mid)+0.001) == int(t/6*3)
	}},
	{name: "ALTERNATE", secs: 6, shape: crackle, how: comet, open: func(t float64, i, n int) bool {
		return i%2 == int(t/1.5)%2
	}},
	{name: "FINALE", secs: 5, mixed: true, how: shellUp, open: func(t float64, i, n int) bool {
		return true
	}},
}

// Fireworks is the file going up.
type Fireworks struct {
	sim *physics.Sparks

	// digits, from and shape run parallel to sim; release is the only place
	// any of the four is shortened.
	//
	// from is the bin a rising shell came out of, or -1 once it is a spark.
	// shape is what it will burst into, fixed at launch, or a shell fired at
	// the end of one volley would burst as the next volley's shape.
	digits []rune
	from   []int8
	shape  []burstKind

	// queued is what is waiting in each bin, indexed by bin.
	queued [BinCount][]rune

	// mouths is indexed by bin, with live saying which are on the panel. A
	// packed slice of only the live mouths would misalign with queued when a
	// bin is off the panel.
	mouths [BinCount]physics.Vec2
	live   [BinCount]bool

	// speed and fuse are read off the panel by fit.
	speed, fuse float64

	// spark scales burst speeds to the panel height; 1 at skySparkRef.
	spark float64

	// since is how long each bin has been waiting to fire again, and phase how
	// far round a fan's sweep it has got, as an angle (unaffected by resize).
	since [BinCount]float64
	phase [BinCount]float64

	// at and atSecs are where the programme has got to, pick and forced the
	// shape the b key has insisted on, and trails whether t is on.
	at     int
	atSecs float64
	pick   burstKind
	forced bool
	trails bool
	now    bool // f has asked for one this frame

	// total is every digit this display holds, which never changes.
	total int

	min, max physics.Vec2
	panel    Panel
	rng      *rand.Rand
}

// NewFireworks queues the digits that were on screen at the five bins.
func NewFireworks(m *Model, panel Panel, rng *rand.Rand) *Fireworks {
	f := &Fireworks{sim: physics.NewSparks(), rng: rng}
	f.sim.Gravity = physics.Vec2{Y: skyGravity}
	f.Resize(panel)
	f.Aim(m.Layout)

	// Seeded from the frame that was drawn, so what goes up is what was on
	// screen.
	for i, h := range m.drawnFrame() {
		f.queued[i%BinCount] = append(f.queued[i%BinCount], h.digit)
		f.total++
	}
	return f
}

// Resize re-boxes the sky and re-reads the numbers that come off the panel.
func (f *Fireworks) Resize(panel Panel) {
	f.panel = panel
	f.min = physics.Vec2{X: float64(panel.X), Y: float64(panel.Y) * RowAspect}
	f.max = physics.Vec2{
		X: float64(panel.X + panel.W - 1),
		Y: float64(panel.Y+panel.H-1) * RowAspect,
	}

	rise := math.Max(f.max.Y-f.min.Y, 1) * skyRise
	f.speed = math.Sqrt(2 * skyGravity * rise)
	f.fuse = f.speed / skyGravity

	f.spark = math.Max(f.max.Y-f.min.Y, 1) / skySparkRef
}

// Aim puts a mouth over each bin's centre column, on the panel's bottom edge.
// It is separate from Resize, which like every diversion's takes only a panel.
func (f *Fireworks) Aim(l *Layout) {
	if l == nil {
		return
	}
	y := f.max.Y
	for i := 0; i < BinCount; i++ {
		x := float64(l.BinCenterX[i])
		f.mouths[i] = physics.Vec2{X: x, Y: y}
		f.live[i] = x >= f.min.X && x <= f.max.X
	}
}

// mouthOf is where a bin's queue waits, with a fallback for a bin that is not on
// the panel at all, so Digits always has somewhere to put its digits.
func (f *Fireworks) mouthOf(i int) physics.Vec2 {
	if i >= 0 && i < BinCount && f.live[i] {
		return f.mouths[i]
	}
	return physics.Vec2{X: (f.min.X + f.max.X) / 2, Y: f.max.Y}
}

// Volley is which figure is running, for the hint line and the tests.
func (f *Fireworks) Volley() string { return volleys[f.at%len(volleys)].name }

// Shape is what is being fired, for the tests.
func (f *Fireworks) Shape() string { return burstShapes[f.shapeNow()].name }

// shapeNow is the shape the next shell will carry.
func (f *Fireworks) shapeNow() burstKind {
	if f.forced {
		return f.pick
	}
	v := volleys[f.at%len(volleys)]
	if v.mixed {
		return burstKind(f.rng.Intn(int(burstKinds)))
	}
	return v.shape
}

// Key takes one of the sky's own keystrokes and reports whether it was one.
// These are hidden keys: handleDivertingKey calls this and is outside the
// handlers keys_test.go scans, so they have no Binding row.
func (f *Fireworks) Key(key string) bool {
	switch key {
	case "f":
		f.now = true
	case "b":
		// The first press locks the shape already firing; later presses
		// walk the four, and passing the last returns to the programme.
		if !f.forced {
			f.pick, f.forced = f.shapeNow(), true
			return true
		}
		if f.pick = f.pick + 1; f.pick >= burstKinds {
			f.forced = false
		}
	case "t":
		f.trails = !f.trails
	default:
		return false
	}
	return true
}

// Effects is what is on, for the tests.
func (f *Fireworks) Effects() (forced, trails bool) { return f.forced, f.trails }

// Step advances the programme, works the lids, fires, and moves everything.
// It takes the bins (FOUNTAIN does not) because the lids gate firing.
//
// Order matters: advance, fire, integrate, then cull. Culling first would burst
// a shell at last frame's position and give its sparks a whole step at once,
// leaving a hole where the burst should be.
func (f *Fireworks) Step(dt float64, bins *BinFill) {
	f.advance(dt)

	for b := 0; b < BinCount; b++ {
		f.since[b] += dt

		// A lid opens only if the volley wants it and the bin has the digits
		// to fire. Timing gates (shellGap, skyCrowd) stay in fire, or the lid
		// would flap between shots.
		open := (f.wants(b) && f.ready(b)) || (f.now && f.live[b])
		if bins != nil {
			bins.SetHold(b, open)
		}
		// A nil BinFill (the physics tests) has no lid to wait for.
		if open && (bins == nil || bins.Lid(b) >= lidClear) {
			f.fire(b)
		}
	}
	f.now = false

	f.sim.Step(dt)
	f.settle()
}

// wants reports whether the running volley has bin i open. A bin off the
// panel is never wanted.
func (f *Fireworks) wants(i int) bool {
	if i < 0 || i >= BinCount || !f.live[i] {
		return false
	}
	return volleys[f.at%len(volleys)].open(f.atSecs, i, BinCount)
}

// ready reports whether bin b has the digits to fire what the volley is asking
// of it. The lid and fire both ask it; fire's other checks are timing.
func (f *Fireworks) ready(b int) bool {
	if b < 0 || b >= BinCount {
		return false
	}
	have := len(f.queued[b])
	switch volleys[f.at%len(volleys)].how {
	case fan:
		// See fanReserve.
		return float64(have) > float64(f.total)/float64(BinCount)*fanReserve
	case mine:
		return have >= burstLeast
	case comet:
		return have >= 1
	}
	// One for the shell, plus enough for its burst, so no shell is a dud.
	return have >= burstLeast+1
}

// advance moves the programme on.
func (f *Fireworks) advance(dt float64) {
	f.atSecs += dt
	for f.atSecs >= volleys[f.at%len(volleys)].secs {
		f.atSecs -= volleys[f.at%len(volleys)].secs
		f.at = (f.at + 1) % len(volleys)
	}
}

// fire is one bin doing whatever the running volley has it doing. Everything
// that leaves a mouth goes through add with a digit taken from that bin's
// queue, which keeps the digit count constant.
func (f *Fireworks) fire(b int) {
	// Checked here as well as by the lid: the f key and the tests reach fire
	// without it.
	if !f.ready(b) {
		return
	}

	sh := f.shapeNow()
	spec := burstShapes[sh]
	how := volleys[f.at%len(volleys)].how

	switch how {
	case fan:
		f.spray(b, sh, spec)
		return
	case mine:
		// A mine is one large event, so it waits twice shellGap.
		if f.since[b] < shellGap*2 && !f.now {
			return
		}
		if !f.now && float64(f.sim.Len()) > float64(f.total)*skyCrowd {
			return
		}
		f.since[b] = 0
		// Up and out, over a cone (see mineSpread).
		f.burst(f.mouths[b], b, sh, math.Pi+mineSpread/2, math.Pi*2-mineSpread/2, mineLift)
		return
	}

	if f.since[b] < shellGap && !f.now {
		return
	}
	if !f.now && float64(f.sim.Len()) > float64(f.total)*skyCrowd {
		return
	}
	f.since[b] = 0

	if how == comet {
		if d, ok := f.take(b); ok {
			vel, fuse := f.cometThrow(b)
			f.add(f.mouths[b], vel, fuse, 1, int8(b), sh, d)
		}
		return
	}

	// A little random lean and throw, so the bins do not fire parallel lines.
	lean := (f.rng.Float64() - 0.5) * f.speed * 0.16
	up := -f.speed * (0.92 + 0.16*f.rng.Float64())
	if d, ok := f.take(b); ok {
		f.add(f.mouths[b], physics.Vec2{X: lean, Y: up},
			f.fuse*(0.92+0.16*f.rng.Float64()), 1, int8(b), sh, d)
	}
}

// cometThrow is the velocity and fuse for a shell that crosses the panel.
//
// The mouths are on the bottom row and gravity is +Y, so a flat throw would
// leave the panel at once and burst at its own mouth; a comet must arc. To land
// back at its starting height after crossing time T, the upward speed is gT/2
// and the apex h = gT²/8, so T = sqrt(8h/g) with h = cometApex of the panel.
// The sideways speed is the panel width over T, and the fuse is T, so it bursts
// as it arrives. It is thrown away from the nearer wall.
func (f *Fireworks) cometThrow(b int) (physics.Vec2, float64) {
	apex := math.Max(f.max.Y-f.min.Y, 1) * cometApex
	cross := math.Sqrt(8 * apex / skyGravity)

	span := f.max.X - f.min.X
	across := span / cross
	if f.mouths[b].X > (f.min.X+f.max.X)/2 {
		across = -across
	}
	return physics.Vec2{X: across, Y: -skyGravity * cross / 2}, cross
}

// spray is a fan: sparks straight out of the mouth at a rate, sweeping. It has
// no shell, so it does its own pacing (shellGap would allow only two a second).
//
// In a steady state a fan keeps rate x life sparks in the air, so the rate is
// set to put skyCrowd of the file up.
func (f *Fireworks) spray(b int, sh burstKind, spec burstSpec) {
	// The whole allowance, not a fifth: a fanning volley opens one bin at a
	// time. The crowd check below shares it if more are open.
	rate := float64(f.total) * skyCrowd / math.Max(spec.life, 0.1)
	if f.since[b] < 1/math.Max(rate, 1) && !f.now {
		return
	}
	if !f.now && float64(f.sim.Len()) > float64(f.total)*skyCrowd {
		return
	}
	f.since[b] = 0

	// Straight up, swept side to side, with a phase per bin.
	f.phase[b] += fanSweep
	a := -math.Pi/2 + math.Sin(f.phase[b])*fanSpread
	sp := spec.speed * f.spark * fanLift * (0.8 + 0.4*f.rng.Float64())
	if d, ok := f.take(b); ok {
		f.add(f.mouths[b], physics.Vec2{X: math.Cos(a) * sp, Y: math.Sin(a) * sp},
			spec.life*(0.7+0.6*f.rng.Float64()), spec.drag, -1, sh, d)
	}
}

// burst turns a shell into its sparks, drawing digits from the bin it came
// from. lo and hi are the angle range in radians (the full circle for a shell,
// an upward cone for a mine). The spacing divides by the shape's own count, not
// the clamped n, so a burst short of digits leaves a gap instead of closing up.
func (f *Fireworks) burst(at physics.Vec2, bin int, sh burstKind, lo, hi, lift float64) {
	spec := burstShapes[sh]
	n := spec.count
	if q := len(f.queued[bin]); n > q {
		n = q // ready means this rarely applies
	}
	for k := 0; k < n; k++ {
		// Evenly spread with a little jitter; random angles clump.
		a := lo + (hi-lo)*(float64(k)+0.6*f.rng.Float64())/float64(spec.count)
		sp := spec.speed * f.spark * lift
		if !spec.even {
			sp *= 0.45 + 0.55*f.rng.Float64()
		}
		d, ok := f.take(bin)
		if !ok {
			break
		}
		f.add(at, physics.Vec2{X: math.Cos(a) * sp, Y: math.Sin(a) * sp},
			spec.life*(0.7+0.6*f.rng.Float64()), spec.drag, -1, sh, d)
	}
}

// settle culls what is spent and bursts what has reached the top.
//
// It walks backwards because Remove swaps the tail into the hole: walking
// forwards would skip the swapped-in element, so of two adjacent spent sparks
// one would survive a frame. Walking backwards also means sparks appended
// during the walk are not visited this frame.
//
// A slot must not be read after release, so the burst's inputs are copied out
// first. The digit goes home before the burst draws from the queue, so a
// nearly empty bin gets one more spark.
func (f *Fireworks) settle() {
	for i := f.sim.Len() - 1; i >= 0; i-- {
		if f.sim.Life[i] > 0 && f.inPlay(f.sim.Pos[i]) {
			continue
		}
		at, bin, sh, d := f.sim.Pos[i], f.from[i], f.shape[i], f.digits[i]
		f.release(i)

		home := f.homeBin(at)
		f.queued[home] = append(f.queued[home], d)

		// A shell always bursts, even one that left the panel (on a short
		// terminal it can leave the top); hold pulls the burst back inside.
		if bin >= 0 {
			f.burst(f.hold(at), int(bin), sh, 0, 2*math.Pi, 1)
		}
	}
}

// inPlay reports whether a position is still inside the panel.
func (f *Fireworks) inPlay(p physics.Vec2) bool {
	return p.X >= f.min.X && p.X <= f.max.X && p.Y >= f.min.Y && p.Y <= f.max.Y
}

// hold pulls a point back inside the panel, so a burst that happened just off it
// is still seen.
func (f *Fireworks) hold(p physics.Vec2) physics.Vec2 {
	return physics.Vec2{
		X: math.Max(f.min.X, math.Min(f.max.X, p.X)),
		Y: math.Max(f.min.Y, math.Min(f.max.Y, p.Y)),
	}
}

// homeBin is the bin a spent digit goes back to: the nearest live mouth, which
// keeps a bin under a busy patch of sky supplied.
func (f *Fireworks) homeBin(at physics.Vec2) int {
	best, near := 0, math.Inf(1)
	found := false
	for b := 0; b < BinCount; b++ {
		if !f.live[b] {
			continue
		}
		if d := math.Abs(at.X - f.mouths[b].X); d < near {
			best, near, found = b, d, true
		}
	}
	if !found {
		return 0 // no mouths on the panel at all
	}
	return best
}

// add puts one particle in, appending to sim and the parallel slices together.
func (f *Fireworks) add(at, vel physics.Vec2, life, drag float64, bin int8, sh burstKind, d rune) {
	f.sim.Add(at, vel, life, drag)
	f.digits = append(f.digits, d)
	f.from = append(f.from, bin)
	f.shape = append(f.shape, sh)
}

// release takes one out. physics.Sparks moves its last element into the hole,
// and the parallel slices must make exactly the same move, or digits are
// silently mismatched; this is the only place any of them is shortened.
func (f *Fireworks) release(i int) {
	last := f.sim.Len() - 1
	if i < 0 || i > last {
		return
	}
	f.sim.Remove(i)
	f.digits[i], f.from[i], f.shape[i] = f.digits[last], f.from[last], f.shape[last]
	f.digits, f.from, f.shape = f.digits[:last], f.from[:last], f.shape[:last]
}

// take pops a digit out of a bin's queue, and reports whether there was one.
// On false the caller adds nothing, so the digit count stays constant.
func (f *Fireworks) take(b int) (rune, bool) {
	if b < 0 || b >= BinCount || len(f.queued[b]) == 0 {
		return 0, false
	}
	d := f.queued[b][0]
	f.queued[b] = f.queued[b][1:]
	return d, true
}

// Airborne is how many digits are up against how many are waiting, for the tests.
func (f *Fireworks) Airborne() (flying, waiting int) {
	flying = f.sim.Len()
	for b := range f.queued {
		waiting += len(f.queued[b])
	}
	return
}

// Digits is every digit, waiting or in the air, for the transitions.
func (f *Fireworks) Digits() []Placed {
	out := make([]Placed, 0, f.total)
	for i, p := range f.sim.Pos {
		out = append(out, Placed{X: p.X, Y: p.Y / RowAspect, Digit: f.digits[i]})
	}
	for b := 0; b < BinCount; b++ {
		at := f.mouthOf(b)
		for j, d := range f.queued[b] {
			// Stacked a little above the mouth, three across.
			out = append(out, Placed{
				X:     at.X + float64(j%3) - 1,
				Y:     at.Y/RowAspect - float64(j/3)*0.34,
				Digit: d,
			})
		}
	}
	return out
}

// skyAttr is what a spark is drawn in, by how much of its life is left: Peak
// through Glow to Dim. These are theme roles, so the ramp reads as cooling on
// every palette, including the pale one where Peak is darkest.
func skyAttr(t *theme.Theme, u float64) canvas.Attr {
	if u > 0.5 {
		return t.On(theme.BlendColors(t.Glow, t.Peak, (u-0.5)*2), true)
	}
	return t.On(theme.BlendColors(t.Dim, t.Glow, u*2), false)
}

// skyRune is the character a spark shows at that point in its life. The life
// is bent (u^0.4) before EmergeRune, which would otherwise drop the digit at
// about 0.63 of a life; this way it stays a digit for most of the fall, then
// steps down "%+=-;:,." to a dot.
func skyRune(d rune, u float64) rune {
	return glyph.EmergeRune(d, math.Pow(ease.Clamp01(u), 0.4))
}

// Draw paints the sky in two passes, tails and then heads. Buffer.Set
// overwrites, so in one pass a tail could rub out another spark's head, which
// shows as flicker.
func (f *Fireworks) Draw(buf *canvas.Buffer, t *theme.Theme, panel Panel) {
	for i, p := range f.sim.Pos {
		// A shell always has a tail, so a lone fast digit reads as travelling.
		if f.trails || f.from[i] >= 0 {
			// Step back along the direction of travel in whole cells; one
			// frame's movement is usually under a cell.
			step := f.sim.Vel[i].Normalize()
			if step.X == 0 && step.Y == 0 {
				continue
			}
			u := f.sim.Fade(i)
			if f.from[i] >= 0 {
				u = 1
			}
			for k := 1; k <= skyTrail; k++ {
				back := p.Sub(step.Scale(float64(k)))
				dim := u * (1 - float64(k)/(skyTrail+1))
				f.put(buf, t, panel, back, f.digits[i], dim)
			}
		}
	}

	for i, p := range f.sim.Pos {
		u := f.sim.Fade(i)
		if f.from[i] >= 0 {
			// A rising shell is one bright point whatever its fuse has left.
			u = 1
		}
		f.put(buf, t, panel, p, f.digits[i], u)
	}
}

// put draws one cell of the sky, clipped to the panel.
func (f *Fireworks) put(buf *canvas.Buffer, t *theme.Theme, panel Panel, at physics.Vec2, d rune, u float64) {
	x := int(math.Round(at.X))
	y := int(math.Round(at.Y / RowAspect))
	if !panel.Contains(x, y) {
		return
	}
	buf.Set(x, y, skyRune(d, u), skyAttr(t, u))
}
