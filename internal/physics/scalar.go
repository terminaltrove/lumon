package physics

// Scalar is a damped spring over a single number, the 1-D counterpart of Seek:
// a digit's drawn size, its brightness, how far a panel has slid open.
type Scalar struct {
	value     float64
	velocity  float64
	Frequency float64 // radians per second

	// Damping is the damping ratio. One is critical: the value arrives without
	// overshoot. Below one it overshoots and rings, as a bin's lid knocks shut.
	// Zero also means critical.
	Damping float64
}

// NewScalar returns a Scalar at value, converging at the given angular
// frequency. Around 3 rad/s takes roughly a second to arrive at 60Hz.
func NewScalar(value, frequency float64) *Scalar {
	return &Scalar{value: value, Frequency: frequency, Damping: 1}
}

// NewScalarDamped returns a Scalar with an explicit damping ratio: 1 is
// critical, below 1 overshoots and rings, above 1 crawls in.
func NewScalarDamped(value, frequency, damping float64) *Scalar {
	return &Scalar{value: value, Frequency: frequency, Damping: damping}
}

// Value returns the current value.
func (s *Scalar) Value() float64 { return s.value }

// Velocity returns the current rate of change, per second.
func (s *Scalar) Velocity() float64 { return s.velocity }

// Set jumps to a value and cancels any motion.
func (s *Scalar) Set(v float64) {
	s.value = v
	s.velocity = 0
}

// Step advances one step of size dt toward target.
func (s *Scalar) Step(dt, target float64) {
	omega := s.Frequency
	stiffness := omega * omega

	ratio := s.Damping
	if ratio <= 0 {
		ratio = 1
	}
	damping := 2 * ratio * omega

	accel := (target-s.value)*stiffness - s.velocity*damping
	s.velocity += accel * dt
	s.value += s.velocity * dt
}
