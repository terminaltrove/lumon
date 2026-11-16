package lumon

import (
	"github.com/terminaltrove/lumon/internal/theme"
	"math"
	"math/rand"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// PERPETUITY: the Lumon mark bouncing edge to edge, the one diversion with no
// numbers in it.
//
// Everything here is in braille dots. A cell is two dots across and four down,
// and a row is about two columns tall, so dots are square: no RowAspect
// correction is needed, and a rotation in dot space is a true rotation on
// screen.

const (
	// bounceSpeed is how fast the mark crosses the box, in dots a second.
	bounceSpeed = 34.0

	// bounceGravity is the pull in physics mode, in dots a second squared, and
	// bounceBounce how much speed survives a wall: below one so it settles,
	// high enough that it bounces several times first.
	bounceGravity = 260.0
	bounceBounce  = 0.72

	// bounceSpin is how fast the mark turns in physics mode, in radians a
	// second, and bounceSpinLoss the fraction of spin a floor bounce keeps.
	// Much lower and the mark stops tumbling and slides.
	bounceSpin     = 2.6
	bounceSpinLoss = 0.94

	// bounceUpright is how fast the mark comes back level when physics is
	// switched off. Critically damped, so it settles without swinging past.
	bounceUpright = 7.0

	// bounceRoll is how long the gradient takes to go round, in seconds.
	bounceRoll = 6.0
)

// The colour states c cycles through.
type bounceHue int

const (
	// hueOff is the terminal's own cyan and nothing else.
	hueOff bounceHue = iota
	// hueBounce steps a colour on every edge the mark touches.
	hueBounce
	// hueRoll rolls continuously, whether or not anything is hit.
	hueRoll
	// hueRandom takes a fresh bright colour on every edge, from anywhere on the
	// wheel. Unlike the other states it ignores the theme's palette.
	hueRandom
	bounceHues
)

// dot is one lit dot of the mark, in whole dots from its top-left. Offsets from
// the centre would be half-dots, and rounding each against a fractional
// position would tear neighbours into different cells.
type dot struct{ x, y int }

// Perpetuity is the mark bouncing in a box.
type Perpetuity struct {
	// Fade brings the mark up and down; the digits gather into its shape
	// first (see Digits).
	Fade

	// The mark, decoded from the asset once, and its size in dots.
	dots   []dot
	mw, mh int

	// digits are the board's visible numbers, which form the mark's shape on
	// the way in and out.
	digits []rune

	// The box, in dots, and where the mark is in it.
	w, h   float64
	x, y   float64
	vx, vy float64

	// angle and spin are only ever non-zero in physics mode.
	angle, spin float64
	physics     bool

	// full is whether the mark has the whole terminal. It is the one diversion
	// that may leave the panel; see Model.draw.
	full bool

	// held is whether the pointer has hold of it, hand where the pointer is in
	// dots, grip the offset from the hand to the centre, and grabbed the
	// smoothed hand speed, which becomes the throw on release (as in
	// physics.Grab).
	held    bool
	hand    dot2
	grip    dot2
	grabbed dot2

	// wallX and wallY are when (on rolled's clock) the mark last left a side
	// wall and a top or bottom one, -1 for never; cornered is set when they
	// are close enough to call a corner, flash how long it flashes for.
	// Only the screensaver's mark watches for corners.
	wallX, wallY float64
	corners      bool
	cornered     bool
	flash        float64

	hue    bounceHue
	step   int     // which of the four colours, while hue is hueBounce
	rolled float64 // how far round the gradient has got, while hue is hueRoll
	picked canvas.Color

	// rng is this session's own, so two terminals do not share colours or
	// tumbles.
	rng *rand.Rand
}

// NewPerpetuity sets the mark going.
func NewPerpetuity(m *Model, panel Panel, rng *rand.Rand) *Perpetuity {
	p := &Perpetuity{Fade: NewFade(), digits: boardDigits(m), rng: rng}
	p.launch(panel)
	return p
}

// newIdleMark is the mark alone on the terminal, which is the screensaver. It
// needs no board, so it has no digits to gather from and only fades up.
func newIdleMark(w, h int, rng *rand.Rand) *Perpetuity {
	p := &Perpetuity{Fade: NewFade(), rng: rng, full: true, corners: true}
	p.launch(Panel{W: w, H: h})
	return p
}

// launch decodes the mark and sets it moving in a panel.
func (p *Perpetuity) launch(panel Panel) {
	p.wallX, p.wallY = -1, -1
	p.dots, p.mw, p.mh = markDots()
	p.Resize(panel)

	// Start in the middle, at an angle that is not a neat fraction of the box,
	// so the path does not soon repeat. Bounces are lossless, so |vx| and |vy|
	// keep these values for the session.
	p.x, p.y = p.w/2, p.h/2
	const lean = 0.62
	p.vx = bounceSpeed * math.Cos(lean)
	p.vy = bounceSpeed * math.Sin(lean)
	p.picked = canvas.Bright(p.roll())
}

// markDots decodes SmallLogo's braille runes into dots, by inverting canvas's
// dot-bit table, so the mark can be rotated. Scaling these dots up turns the
// globe into solid blocks; a bigger mark needs a bigger asset (the recipe is
// at SmallLogo).
func markDots() (dots []dot, w, h int) {
	for row, line := range SmallLogo {
		// By rune index: ranging the string gives byte offsets, and a braille
		// code point is three bytes.
		for col, r := range []rune(line) {
			mask := uint8(r - canvas.BrailleBlank)
			for dx := 0; dx < canvas.DotsX; dx++ {
				for dy := 0; dy < canvas.DotsY; dy++ {
					if mask&(1<<canvas.DotBit(dx, dy)) == 0 {
						continue
					}
					dots = append(dots, dot{col*canvas.DotsX + dx, row*canvas.DotsY + dy})
				}
			}
		}
	}
	return dots, len([]rune(SmallLogo[0])) * canvas.DotsX, len(SmallLogo) * canvas.DotsY
}

// boardDigits is the visible numbers, in reading order, for the mark to be
// spelled out in on the way in and out (as PRAISE and COMPUNCTION do).
func boardDigits(m *Model) []rune {
	l := m.Layout
	out := make([]rune, 0, l.ViewportW*l.ViewportH)
	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			if n := m.Grid.At(gx, gy); n != nil {
				out = append(out, rune('0'+n.Value))
			}
		}
	}
	if len(out) == 0 {
		out = append(out, '0')
	}
	return out
}

// radius is half the mark, in dots.
func (p *Perpetuity) radius() (rx, ry float64) {
	return float64(p.mw-1) / 2, float64(p.mh-1) / 2
}

// origin is the dot the mark's top-left corner is drawn at, its position
// rounded to the nearest whole dot in both axes.
//
// Do not snap it to the cell grid. The mark's dot pattern is the same at every
// sub-cell phase (it is a thresholded image, with no alignment to cells), and
// a snap makes the mark hop a whole row at a time instead of sliding. The only
// cost of an off-boundary phase is one extra inter-cell gap in the font, well
// under a dot.
func (p *Perpetuity) origin() (x, y int) {
	rx, ry := p.radius()
	return int(math.Round(p.x - rx)), int(math.Round(p.y - ry))
}

// Resize re-measures the box and puts the mark back inside it. The mark is a
// fixed size.
func (p *Perpetuity) Resize(panel Panel) {
	w, h := maxi(panel.W, 1), maxi(panel.H, 1)
	p.w, p.h = float64(w*canvas.DotsX), float64(h*canvas.DotsY)
	p.clampIn()
}

// travel is how far the mark's centre may go along one axis of a box. The far
// limit is box-1-reach, not box-reach: the last lit dot is at box-1, and one
// more would be clipped by Braille.Set.
func travel(reach, box float64) (lo, hi float64) {
	return reach, math.Max(reach, box-1-reach)
}

// clampIn pulls the mark back inside the box, for a resize that shrank it.
func (p *Perpetuity) clampIn() {
	rx, ry := p.reach()
	loX, hiX := travel(rx, p.w)
	loY, hiY := travel(ry, p.h)
	p.x, p.y = clampF(p.x, loX, hiX), clampF(p.y, loY, hiY)
}

// reach is the half-extent that has to clear the walls: the mark's own
// half-size, ignoring rotation. Counting the rotated corners would leave a
// small panel too little travel and wedge the mark between floor and ceiling;
// a turned corner past the edge is just clipped by Braille.Set.
//
// Clamped to half the box, so a panel smaller than the mark leaves it in the
// middle.
func (p *Perpetuity) reach() (rx, ry float64) {
	hx, hy := p.radius()
	return math.Min(hx, p.w/2), math.Min(hy, p.h/2)
}

// Step moves the mark.
func (p *Perpetuity) Step(dt float64) {
	p.StepFade(dt)
	p.rolled += dt
	p.flash = math.Max(0, p.flash-dt)

	// Held, it follows the hand and skips integration, or gravity would build
	// up a velocity that fires it at the floor on release.
	if p.held {
		p.velFrom(dt)
		p.x, p.y = p.hand.x, p.hand.y
		return
	}

	if p.physics {
		p.vy += bounceGravity * dt
		p.angle += p.spin * dt
	} else if p.angle != 0 {
		// Coming back level after physics is switched off, critically damped.
		p.spin += (-p.angle*bounceUpright*bounceUpright - p.spin*2*bounceUpright) * dt
		p.angle += p.spin * dt
		if math.Abs(p.angle) < 0.01 && math.Abs(p.spin) < 0.05 {
			p.angle, p.spin = 0, 0
		}
	}

	p.x += p.vx * dt
	p.y += p.vy * dt
	p.walls()
}

// walls reflects the mark off the box, and is where a bounce colour is taken.
func (p *Perpetuity) walls() {
	rx, ry := p.reach()
	lo, hi := travel(rx, p.w)
	if p.x < lo && p.vx < 0 {
		p.x, p.vx = lo, -p.vx*p.restitution()
		p.hitEdge()
		p.wallX = p.rolled
	} else if p.x > hi && p.vx > 0 {
		p.x, p.vx = hi, -p.vx*p.restitution()
		p.hitEdge()
		p.wallX = p.rolled
	}

	lo, hi = travel(ry, p.h)
	if p.y < lo && p.vy < 0 {
		p.y, p.vy = lo, -p.vy*p.restitution()
		p.hitEdge()
		p.wallY = p.rolled
	} else if p.y > hi && p.vy > 0 {
		p.y, p.vy = hi, -p.vy*p.restitution()
		p.spin *= bounceSpinLoss // a floor takes the turn off it
		p.hitEdge()
		p.wallY = p.rolled
	}

	// The corner everybody waits for: both walls within a moment of each
	// other. It moves under half a dot a step, so the same step is too rare.
	if p.corners && p.wallX >= 0 && p.wallY >= 0 && math.Abs(p.wallX-p.wallY) <= cornerWindow && !p.physics {
		p.cornered, p.flash = true, cornerFlash
		p.wallX, p.wallY = -1, -1
	}
}

// cornerWindow is how close together, in seconds, the two walls must be hit
// to count as the corner, and cornerFlash how long the mark flashes for it.
const (
	cornerWindow = 2.0 / 60
	cornerFlash  = 1.5
)

// Cornered reports whether the mark has hit a corner since it was last asked.
func (p *Perpetuity) Cornered() bool {
	c := p.cornered
	p.cornered = false
	return c
}

// restitution is how much speed survives a wall: exactly one outside physics
// mode, so the mark never slows down.
func (p *Perpetuity) restitution() float64 {
	if p.physics {
		return bounceBounce
	}
	return 1
}

// hitEdge takes the next colour, if that is what c asked for.
func (p *Perpetuity) hitEdge() {
	switch p.hue {
	case hueBounce:
		p.step++
	case hueRandom:
		p.picked = canvas.Bright(p.roll())
	}
}

// Physics turns gravity and tumbling on, or off again.
func (p *Perpetuity) Physics() {
	p.physics = !p.physics
	if p.physics {
		// A different tumble each time, in either direction, between half and
		// one and a half times bounceSpin. Never zero.
		p.spin = bounceSpin * (0.5 + p.roll())
		if p.roll() < 0.5 {
			p.spin = -p.spin
		}
		return
	}
	// Back to a constant crossing speed, keeping the direction.
	p.spin = 0
	if v := math.Hypot(p.vx, p.vy); v > 0.01 {
		p.vx, p.vy = p.vx/v*bounceSpeed, p.vy/v*bounceSpeed
		return
	}
	p.vx, p.vy = bounceSpeed, -bounceSpeed
}

// dot2 is a point in the mark's own dot space.
type dot2 struct{ x, y float64 }

// grabSmooth is how much of the hand's history is kept each frame, the same
// weighting physics.Grab uses.
const grabSmooth = 0.6

// Take picks the mark up, if the pointer is on it. Only in physics mode:
// outside it the mark keeps a constant speed (TestTheMarkNeverSettles), which
// a drag would break.
func (p *Perpetuity) Take(dotX, dotY float64) bool {
	if !p.physics {
		return false
	}
	rx, ry := p.radius()
	if math.Abs(dotX-p.x) > rx || math.Abs(dotY-p.y) > ry {
		return false
	}
	// Keep the point taken hold of, or the centre snaps to the pointer and the
	// jump registers as a throw.
	p.held = true
	p.grip = dot2{x: p.x - dotX, y: p.y - dotY}
	p.hand = dot2{x: p.x, y: p.y}
	p.grabbed = dot2{}
	return true
}

// DragTo moves the hand.
func (p *Perpetuity) DragTo(dotX, dotY float64) {
	if !p.held {
		return
	}
	p.hand = dot2{x: dotX + p.grip.x, y: dotY + p.grip.y}
}

// Drop lets go, giving the mark the speed the hand was moving at.
func (p *Perpetuity) Drop() {
	if !p.held {
		return
	}
	p.held = false
	p.vx, p.vy = p.grabbed.x, p.grabbed.y
}

// Held reports whether the pointer has hold of it.
func (p *Perpetuity) Held() bool { return p.held }

// velFrom folds this frame's hand movement into the smoothed throw speed.
func (p *Perpetuity) velFrom(dt float64) {
	if dt <= 0 {
		return
	}
	vx := (p.hand.x - p.x) / dt
	vy := (p.hand.y - p.y) / dt
	p.grabbed.x = p.grabbed.x*grabSmooth + vx*(1-grabSmooth)
	p.grabbed.y = p.grabbed.y*grabSmooth + vy*(1-grabSmooth)
}

// roll is a random place on the colour wheel.
func (p *Perpetuity) roll() float64 {
	if p.rng == nil {
		return 0
	}
	return p.rng.Float64()
}

// Colour takes the next colour state.
func (p *Perpetuity) Colour() { p.hue = (p.hue + 1) % bounceHues }

// Full takes the whole terminal, or gives it back.
func (p *Perpetuity) Full() bool {
	p.full = !p.full
	return p.full
}

// Fullscreen reports whether the mark has the terminal.
func (p *Perpetuity) Fullscreen() bool { return p.full }

// Digits is the mark's shape in the board's numbers, one per cell, so the
// digits gather into the logo on the way in and scatter out of it on the way
// back. An empty list would make Transit arrive on its first frame and the
// entrance a cut (TestEnteringAndLeavingAreJourneys).
func (p *Perpetuity) Digits(panel Panel) []Placed {
	if len(p.digits) == 0 {
		return nil
	}
	seen := make(map[[2]int]bool, len(p.dots))
	out := make([]Placed, 0, len(p.dots)/4)

	ox, oy := p.origin()
	for _, d := range p.dots {
		cx := panel.X + (ox+d.x)/canvas.DotsX
		cy := panel.Y + (oy+d.y)/canvas.DotsY
		if seen[[2]int{cx, cy}] {
			continue
		}
		seen[[2]int{cx, cy}] = true
		out = append(out, Placed{
			X:     float64(cx),
			Y:     float64(cy),
			Digit: p.digits[len(out)%len(p.digits)],
		})
	}
	return out
}

// Attr is what the mark is drawn in. hueBounce steps through primary,
// backdrop, primary, peak (the dark beat makes it read as a cycle); hueRoll
// blends through the same four continuously.
func (p *Perpetuity) Attr(t *theme.Theme) canvas.Attr {
	ring := [4]canvas.Color{t.Primary, t.Backdrop, t.Primary, t.Peak}

	var fg canvas.Color
	switch p.hue {
	case hueBounce:
		fg = ring[p.step%len(ring)]
	case hueRandom:
		fg = p.picked
	case hueRoll:
		at := math.Mod(p.rolled/bounceRoll, 1) * float64(len(ring))
		i := int(at)
		fg = theme.BlendColors(ring[i%len(ring)], ring[(i+1)%len(ring)], at-float64(i))
	default:
		fg = t.Primary
	}
	// Having hit the corner, it flashes between the brightest colour and its
	// own, a few times a second.
	if p.flash > 0 && int(p.flash*6)%2 == 0 {
		fg = t.Peak
	}
	return t.On(theme.BlendColors(t.Backdrop, fg, p.Level()), false)
}

// Draw paints the mark into a region.
func (p *Perpetuity) Draw(buf *canvas.Buffer, t *theme.Theme, panel Panel) {
	if panel.W < 1 || panel.H < 1 {
		return
	}
	br := canvas.NewBraille(panel.W, panel.H)
	if p.angle == 0 {
		ox, oy := p.origin()
		// Upright, the dots go down exactly as authored. See origin.
		for _, d := range p.dots {
			// Set, not Plot: Plot drops any point with a negative coordinate,
			// which would cut the mark off at the left edge.
			br.Set(ox+d.x, oy+d.y)
		}
		buf.BlitBrailleOver(panel.X, panel.Y, br, p.Attr(t))
		return
	}

	rx, ry := p.radius()
	sin, cos := math.Sin(p.angle), math.Cos(p.angle)
	for _, d := range p.dots {
		dx, dy := float64(d.x)-rx, float64(d.y)-ry
		br.Set(
			int(math.Round(p.x+dx*cos-dy*sin)),
			int(math.Round(p.y+dx*sin+dy*cos)),
		)
	}

	// Over, not wholesale: a wholesale blit colours every cell, blanks
	// included, which paints a rectangle of background and costs an escape
	// sequence per cell.
	buf.BlitBrailleOver(panel.X, panel.Y, br, p.Attr(t))
}
