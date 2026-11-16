package lumon

import (
	"math"
	"testing"
)

// A terminal delivers input in discrete lumps; the spring turns them back into
// motion.
func TestTheCameraGlidesToWhereItIsSent(t *testing.T) {
	r := NewRig(0, 0)
	at := r.Dolly()
	if !r.Key("+") {
		t.Fatal("the camera did not take +")
	}
	r.Step(1.0 / 60)
	if moved := r.Dolly(); moved == at {
		t.Fatal("a frame after the key the camera had not begun to move")
	}
	if math.Abs(r.Dolly()-r.wantDolly) < 1e-9 {
		t.Error("the camera arrived instantly; it is meant to glide")
	}

	for i := 0; i < 240; i++ {
		r.Step(1.0 / 60)
	}
	if math.Abs(r.Dolly()-r.wantDolly) > 1e-3 {
		t.Errorf("after four seconds it is at %.4f, heading for %.4f", r.Dolly(), r.wantDolly)
	}
}

// Neither direction runs away, or passes through the eye and turns the view
// inside out.
func TestFlyingInAndOutIsBounded(t *testing.T) {
	r := NewRig(0, 0)
	for i := 0; i < 200; i++ {
		r.Key("+")
	}
	if r.wantDolly < rigNear-1e-9 {
		t.Errorf("flying in reached %.4f, past the near limit %v", r.wantDolly, rigNear)
	}
	if r.wantDolly <= 0 {
		t.Fatal("flying in put the eye at or behind the file")
	}

	for i := 0; i < 400; i++ {
		r.Key("-")
	}
	if r.wantDolly > rigFar+1e-9 {
		t.Errorf("flying out reached %.4f, past the far limit %v", r.wantDolly, rigFar)
	}

	// In is nearer than out.
	a := NewRig(0, 0)
	b := NewRig(0, 0)
	a.Key("+")
	b.Key("-")
	if a.wantDolly >= b.wantDolly {
		t.Errorf("+ went to %.3f and - to %.3f", a.wantDolly, b.wantDolly)
	}
}

func TestTippingStopsShortOfOverTheTop(t *testing.T) {
	r := NewRig(0, 0)
	for i := 0; i < 200; i++ {
		r.Key(".")
	}
	if r.wantPitch > rigTipLimit+1e-9 {
		t.Errorf("tipping reached %.1f degrees, past %v", r.wantPitch, rigTipLimit)
	}
	for i := 0; i < 400; i++ {
		r.Key(",")
	}
	if r.wantPitch < -rigTipLimit-1e-9 {
		t.Errorf("tipping the other way reached %.1f degrees", r.wantPitch)
	}
}

// The tour runs while you look and stops while you work, with no mode to choose.
func TestTheCameraToursWhileTheBoardIsLeftAlone(t *testing.T) {
	r := NewRig(0, 0)
	if r.Spinning() {
		t.Error("it began turning before it had been left alone at all")
	}

	for i := 0; i < int(60*(rigRest+1)); i++ {
		r.Step(1.0 / 60)
	}
	if !r.Spinning() {
		t.Fatalf("it is still not turning %v seconds after being left alone", rigRest+1)
	}
	before := r.Yaw()
	r.Step(1.0 / 60)
	if r.Yaw() == before {
		t.Error("it reports itself turning but the view is not moving")
	}

	// Touched, it holds within one frame.
	r.Touch()
	r.Step(1.0 / 60)
	if r.Spinning() {
		t.Error("it went on turning after the refiner touched the board")
	}
	held := r.Yaw()
	for i := 0; i < 30; i++ {
		r.Step(1.0 / 60)
	}
	if math.Abs(r.Yaw()-held) > 1e-9 {
		t.Errorf("it drifted from %.4f to %.4f while being worked", held, r.Yaw())
	}

	// It picks up again once the board is quiet.
	for i := 0; i < int(60*(rigRest+1)); i++ {
		r.Step(1.0 / 60)
	}
	if !r.Spinning() {
		t.Error("it never picked up again after the refiner stopped")
	}
}

// Otherwise pointing the camera somewhere and watching would stop the tour you
// pointed.
func TestSteeringDoesNotCountAsWorking(t *testing.T) {
	r := NewRig(0, 0)
	for i := 0; i < int(60*(rigRest+1)); i++ {
		r.Step(1.0 / 60)
	}
	if !r.Spinning() {
		t.Fatal("it never began to turn")
	}
	for _, key := range []string{"+", "-", "[", "]", ",", "."} {
		r.Key(key)
		r.Step(1.0 / 60)
		if !r.Spinning() {
			t.Errorf("pressing %q stood the tour down", key)
		}
	}

	// Reset is the exception: putting the view back means wanting to look at it.
	// Without the hold the tour resumes on the next frame and the reset view lasts
	// one frame.
	r.Key("0")
	r.Step(1.0 / 60)
	if r.Spinning() {
		t.Error("reset did not hold the view still to be looked at")
	}
}

// A manual turn adds to the tour, and a reset puts the view back without
// undoing the tour's progress.
func TestSteeringAndTheTourAddUp(t *testing.T) {
	r := NewRig(30, 10)
	for i := 0; i < int(60*(rigRest+2)); i++ {
		r.Step(1.0 / 60)
	}
	spun := r.spin
	if spun <= 0 {
		t.Fatal("the tour never moved")
	}

	r.Key("]")
	for i := 0; i < 120; i++ {
		r.Step(1.0 / 60)
	}
	if r.yaw.Value() < 30+rigTurn-1e-3 {
		t.Errorf("a manual turn left the steered yaw at %.2f, want %v", r.yaw.Value(), 30+rigTurn)
	}

	// Reset holds long enough for the springs to settle before the tour resumes.
	r.Key("0")
	for i := 0; i < int(60*rigRest)-2; i++ {
		r.Step(1.0 / 60)
	}
	if r.spin != 0 {
		t.Errorf("reset left %.2f degrees of tour on the view", r.spin)
	}
	if math.Abs(r.Yaw()-30) > 1e-2 || math.Abs(r.Pitch()-10) > 1e-2 {
		t.Errorf("reset left the view at yaw %.2f pitch %.2f, want 30 and 10", r.Yaw(), r.Pitch())
	}
}

func TestTheCameraTakesOnlyItsOwnKeys(t *testing.T) {
	r := NewRig(0, 0)
	for _, key := range []string{"h", "j", "k", "l", "up", "down", "left", "right",
		"1", "2", "3", "4", "5", "enter", " ", "s", "t", "i", "q", "esc", "d", "?"} {
		if r.Key(key) {
			t.Errorf("the camera swallowed %q, which the game needs", key)
		}
	}
	for _, key := range []string{"+", "=", "-", "_", "[", "]", ",", ".", "0"} {
		if !r.Key(key) {
			t.Errorf("the camera did not take %q", key)
		}
	}
}

// The camera mode is meant to be found, so the controls must not list its keys;
// the footer shows them once you are in it. `+` and `-` are shared: on the flat
// board they zoom, and handleKey gives them to the rig only when there is a
// volume to fly, so their documented meaning is the board's.
func TestTheCameraKeysAreNotDocumented(t *testing.T) {
	shared := map[string]bool{"+": true, "=": true, "-": true, "_": true}
	for _, g := range Bindings() {
		for _, b := range g.Bindings {
			for _, bound := range b.Bound {
				if shared[bound] {
					continue
				}
				if NewRig(0, 0).Key(bound) {
					t.Errorf("the controls bind %q, which the camera also takes", bound)
				}
			}
			all := b.Keys + " " + b.What + " " + b.Short
			for _, key := range []string{"[", "]", RigKeys} {
				if len(key) > 1 && contains(all, key) {
					t.Errorf("the controls mention the camera: %q", all)
				}
			}
		}
	}
}

func contains(hay, needle string) bool {
	if len(needle) > len(hay) {
		return false
	}
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
