package lumon

import "github.com/terminaltrove/lumon/internal/physics"

// The transition for a diversion that has nowhere to fly.
//
// Most diversions enter and leave by flying the digits between the grid and
// the diversion. Some, such as MATRIX and DEPTH, move no digit, so their flight
// takes a frame and the screen would change abruptly. They fade instead: a
// level rises as the digits land and falls before they set off home, and each
// diversion applies it in its own way (MATRIX mixes its wave toward the resting
// board, DEPTH its wave once the camera is back, PERPETUITY the mark itself).

const (
	// fadeFrequency is how fast the level rises and falls, in radians a
	// second. Critically damped, because overshooting full reads as a flash.
	fadeFrequency = 4.5

	// fadeGone is how faint counts as finished on the way out.
	fadeGone = 0.02
)

// Fade is how much of a static diversion is showing, and which way it is going.
// Diversions that need one embed it.
type Fade struct {
	level   *physics.Scalar
	leaving bool
}

// NewFade starts dark, so on the frame the digits land the screen still shows
// the board they came from.
func NewFade() Fade { return Fade{level: physics.NewScalar(0, fadeFrequency)} }

// StepFade advances the level. Diversions call it from their own Step.
func (f *Fade) StepFade(dt float64) {
	target := 1.0
	if f.leaving {
		target = 0
	}
	f.level.Step(dt, target)
}

// Leave asks the diversion to fall. It keeps running and keeps being drawn until
// Gone reports it has.
func (f *Fade) Leave() { f.leaving = true }

// Leaving reports whether it is on its way out, and Gone whether it has finished
// going.
func (f *Fade) Leaving() bool { return f.leaving }
func (f *Fade) Gone() bool    { return f.leaving && f.level.Value() < fadeGone }

// Level is how much is showing, 0 to 1.
func (f *Fade) Level() float64 { return f.level.Value() }

// SetLevel sets the level directly, for tests.
func (f *Fade) SetLevel(v float64) { f.level.Set(v) }

// fading is a diversion with nowhere to fly, which diversions.go asks to fall
// before sending its digits home.
type fading interface {
	Leave()
	Leaving() bool
	Gone() bool
}
