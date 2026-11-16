package lumon

import "github.com/terminaltrove/lumon/internal/physics"

// A camera you can fly, used by the volume view and the wave in depth. Every
// axis is a physics.Scalar, so a keystroke glides.
//
// Callers hook Rig.Key with an `if`, never a `case` in a key handler:
// TestEveryBoundKeyIsDocumented scans handleKey's case clauses, and these
// hidden modes must stay out of the controls overlay. The footer shows RigKeys
// instead.

const (
	// rigZoom is the fraction of the eye distance one press of + or - removes
	// or adds, so flying in slows as it closes. rigNear and rigFar bound it.
	rigZoom = 0.18
	rigNear = 0.35
	rigFar  = 3.2

	// rigTurn is how far one press of [ or ] turns the view and rigTip how far
	// , or . tips it, both in degrees. rigTipLimit stops it going over the top.
	rigTurn     = 12.0
	rigTip      = 7.0
	rigTipLimit = 78.0

	// rigFrequency is how fast the camera settles after a press, in radians a
	// second.
	rigFrequency = 7.0

	// rigSpin is how fast the view turns on its own, in degrees a second, and
	// rigRest how long the board must be left alone, in seconds, before it
	// starts. Slow enough to read a digit while it turns.
	rigSpin = 5.0
	rigRest = 4.0
)

// Rig is a steerable viewpoint: where the eye is, which way it faces, and
// whether it is turning on its own.
type Rig struct {
	// dolly multiplies the caller's own eye distance, so one rig serves clouds
	// of very different sizes.
	dolly *physics.Scalar
	yaw   *physics.Scalar
	pitch *physics.Scalar

	// wantDolly, wantYaw and wantPitch are where each is heading.
	// physics.Scalar takes its target at Step and keeps none.
	wantDolly, wantYaw, wantPitch float64

	// homeYaw and homePitch are where 0 puts it back to.
	homeYaw, homePitch float64

	// spin is the yaw the idle turn has accumulated, kept apart from the
	// refiner's yaw so a manual turn adds to it.
	spin float64

	// quiet is how long since the board was last touched. The spin waits on it.
	quiet float64
}

// NewRig returns a camera aimed where the caller wants it.
func NewRig(yaw, pitch float64) *Rig {
	return &Rig{
		dolly:     physics.NewScalar(1, rigFrequency),
		yaw:       physics.NewScalar(yaw, rigFrequency),
		pitch:     physics.NewScalar(pitch, rigFrequency),
		wantDolly: 1,
		wantYaw:   yaw,
		wantPitch: pitch,
		homeYaw:   yaw,
		homePitch: pitch,
	}
}

// Step settles the camera, and turns it if the board has been left alone.
func (r *Rig) Step(dt float64) {
	r.quiet += dt
	if r.Spinning() {
		r.spin += rigSpin * dt
		for r.spin >= 360 {
			r.spin -= 360
		}
	}
	r.dolly.Step(dt, r.wantDolly)
	r.yaw.Step(dt, r.wantYaw)
	r.pitch.Step(dt, r.wantPitch)
}

// Touch reports that the refiner is working, which stops the idle turn. It is
// called from the cursor, the bins and refining, not the key handler, so
// steering the camera does not stop the turn.
func (r *Rig) Touch() { r.quiet = 0 }

// Spinning reports whether the idle turn is running.
func (r *Rig) Spinning() bool { return r.quiet >= rigRest }

// Dolly is the multiplier on the caller's eye distance, Yaw the total turn
// including whatever the idle spin has added, and Pitch the tip.
func (r *Rig) Dolly() float64 { return r.dolly.Value() }
func (r *Rig) Yaw() float64   { return r.yaw.Value() + r.spin }
func (r *Rig) Pitch() float64 { return r.pitch.Value() }

// Aim points a camera the way the rig is facing. base is the eye distance the
// caller would use standing still.
func (r *Rig) Aim(c *Camera, base float64) {
	c.Dolly = base * r.Dolly()
	c.Yaw = r.Yaw()
	c.Pitch = r.Pitch()
}

// Key takes a keystroke and reports whether the camera used it. Callers pass
// unused keys on to the screen's own handler.
func (r *Rig) Key(key string) bool {
	switch key {
	case "+", "=":
		r.wantDolly = clampF(r.wantDolly*(1-rigZoom), rigNear, rigFar)
	case "-", "_":
		r.wantDolly = clampF(r.wantDolly*(1+rigZoom), rigNear, rigFar)
	case "[":
		r.wantYaw -= rigTurn
	case "]":
		r.wantYaw += rigTurn
	case ",":
		r.wantPitch = clampF(r.wantPitch-rigTip, -rigTipLimit, rigTipLimit)
	case ".":
		r.wantPitch = clampF(r.wantPitch+rigTip, -rigTipLimit, rigTipLimit)
	case "0":
		r.wantDolly, r.wantYaw, r.wantPitch = 1, r.homeYaw, r.homePitch
		r.spin = 0
		// Reset stops the tour too, or it would resume on the next frame and
		// turn away from the view just restored.
		r.Touch()
	default:
		return false
	}
	return true
}

// RigKeys is the line the footer shows while a camera is up. The controls
// overlay must not carry it, see the note at the top of this file.
const RigKeys = "+ - fly · [ ] turn · , . tip · 0 reset"
