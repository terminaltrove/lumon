package lumon

import (
	"math"
	"testing"

	"github.com/terminaltrove/lumon/internal/physics"
)

func vec(x, y float64) physics.Vec2 { return physics.Vec2{X: x, Y: y} }

func stepUntilLanded(f *Flights, maxSteps int) int {
	for i := 0; i < maxSteps; i++ {
		f.Step(physics.DT)
		if len(f.Active()) == 0 {
			return i
		}
	}
	return -1
}

func TestLaunchCreatesOneFlightPerDigit(t *testing.T) {
	f := NewFlights()

	f.Launch(Departure{GridID: 10, Digit: '7', From: vec(0, 0), To: vec(40, 20), BinIdx: 2})
	f.Launch(Departure{GridID: 11, Digit: '3', From: vec(5, 0), To: vec(40, 20), BinIdx: 2})

	if got := len(f.Active()); got != 2 {
		t.Errorf("active flights = %d, want 2", got)
	}
}

func TestFlightStartsWhereItWasLaunched(t *testing.T) {
	f := NewFlights()
	f.Launch(Departure{GridID: 10, Digit: '7', From: vec(3, 4), To: vec(40, 20), BinIdx: 1})

	got := f.Active()[0]

	if math.Abs(got.X-3) > 1e-9 || math.Abs(got.Y-4) > 1e-9 {
		t.Errorf("flight starts at (%v,%v), want (3,4)", got.X, got.Y)
	}
	if got.Digit != '7' {
		t.Errorf("digit = %q, want '7'", got.Digit)
	}
}

func TestFlightMovesTowardItsBin(t *testing.T) {
	f := NewFlights()
	target := vec(40, 20)
	f.Launch(Departure{GridID: 10, Digit: '7', From: vec(0, 0), To: target, BinIdx: 1})

	before := math.Hypot(target.X, target.Y)
	for i := 0; i < 40; i++ {
		f.Step(physics.DT)
	}
	fl := f.Active()[0]
	after := math.Hypot(target.X-fl.X, target.Y-fl.Y)

	if after >= before {
		t.Errorf("distance to bin went %v -> %v, want it to close", before, after)
	}
}

func TestFlightArrivesFromAcrossTheScreen(t *testing.T) {
	for _, start := range []physics.Vec2{vec(0, 0), vec(119, 0), vec(0, 39), vec(60, 12)} {
		f := NewFlights()
		f.Launch(Departure{GridID: 10, Digit: '7', From: start, To: vec(40, 30), BinIdx: 1})

		if steps := stepUntilLanded(f, 600); steps < 0 {
			t.Errorf("flight from %+v never landed within 10 seconds", start)
		}
	}
}

func TestLandedFlightsAreReportedOnce(t *testing.T) {
	f := NewFlights()
	f.Launch(Departure{GridID: 10, Digit: '7', From: vec(0, 0), To: vec(20, 10), BinIdx: 3})

	var landed []Flight
	for i := 0; i < 600 && len(landed) == 0; i++ {
		f.Step(physics.DT)
		landed = append(landed, f.TakeLanded()...)
	}

	if len(landed) != 1 {
		t.Fatalf("landed = %d, want 1", len(landed))
	}
	if landed[0].ID != 10 || landed[0].BinIdx != 3 {
		t.Errorf("landed %+v, want ID 10 into bin 3", landed[0])
	}
	if got := f.TakeLanded(); len(got) != 0 {
		t.Errorf("TakeLanded returned %d flights on the second call, want 0", len(got))
	}
}

// Otherwise the player sees an airborne digit in two places at once.
func TestInFlightTracksAirborneDigits(t *testing.T) {
	f := NewFlights()
	f.Launch(Departure{GridID: 10, Digit: '7', From: vec(0, 0), To: vec(20, 10), BinIdx: 0})

	if !f.InFlight(10) {
		t.Error("digit 10 should be in flight")
	}
	if f.InFlight(11) {
		t.Error("digit 11 was never launched")
	}

	stepUntilLanded(f, 600)
	f.TakeLanded()

	if f.InFlight(10) {
		t.Error("digit 10 is still marked in flight after landing")
	}
}

func TestFlightsFromDifferentClustersCoexist(t *testing.T) {
	f := NewFlights()
	f.Launch(Departure{GridID: 10, Digit: '1', From: vec(0, 0), To: vec(10, 30), BinIdx: 0})
	f.Launch(Departure{GridID: 20, Digit: '2', From: vec(60, 0), To: vec(90, 30), BinIdx: 4})

	for i := 0; i < 35; i++ {
		f.Step(physics.DT)
	}

	if got := len(f.Active()); got != 2 {
		t.Fatalf("active = %d, want both still flying or landed together", got)
	}
	a, b := f.Active()[0], f.Active()[1]
	if a.BinIdx == b.BinIdx {
		t.Error("flights lost their separate bin targets")
	}
}

func TestFlightIsDeterministic(t *testing.T) {
	run := func() (float64, float64) {
		f := NewFlights()
		f.Launch(Departure{GridID: 10, Digit: '7', From: vec(0, 0), To: vec(40, 20), BinIdx: 1})
		for i := 0; i < 45; i++ {
			f.Step(physics.DT)
		}
		fl := f.Active()[0]
		return fl.X, fl.Y
	}

	x1, y1 := run()
	x2, y2 := run()
	if x1 != x2 || y1 != y2 {
		t.Errorf("(%v,%v) vs (%v,%v)", x1, y1, x2, y2)
	}
}

// It eases into the bin and stops there; it never sails past.
func TestFlightDoesNotOvershootTheBin(t *testing.T) {
	f := NewFlights()
	target := vec(40, 0)
	f.Launch(Departure{GridID: 10, Digit: '7', From: vec(0, 0), To: target, BinIdx: 1})

	maxX := 0.0
	for i := 0; i < 600; i++ {
		f.Step(physics.DT)
		for _, fl := range f.Active() {
			if fl.X > maxX {
				maxX = fl.X
			}
		}
	}

	if maxX > target.X+1 {
		t.Errorf("flight reached X=%v, overshooting the bin at %v", maxX, target.X)
	}
}

func TestNoFlightsMeansNothingToStep(t *testing.T) {
	f := NewFlights()

	f.Step(physics.DT)

	if len(f.Active()) != 0 || len(f.TakeLanded()) != 0 {
		t.Error("an empty Flights produced work from nothing")
	}
}

// The shrink is tied to distance covered, so a digit released next to its bin
// is plain within those few cells and one released across the screen uses the
// whole journey. A spring at a fixed rate could not do both.
func TestShrinkingKeepsPaceWithTravelFromAnyDistance(t *testing.T) {
	for _, span := range []struct {
		name string
		to   physics.Vec2
	}{
		{"near", vec(6, 6)},
		{"far", vec(200, 60)},
	} {
		f := NewFlights()
		f.Launch(Departure{
			GridID: 1, Digit: '8', From: vec(0, 0), To: span.to,
			BinIdx: 0, Height: MaxGlyphHeight,
		})

		start := f.Active()[0].Height
		if start != MaxGlyphHeight {
			t.Errorf("%s flight left at %v rows, want the %v it had", span.name, start, MaxGlyphHeight)
		}

		last := start
		for i := 0; i < 3000 && len(f.Active()) > 0; i++ {
			f.Step(1.0 / 60)
			if a := f.Active(); len(a) > 0 {
				last = a[0].Height
			}
		}

		if len(f.Active()) > 0 {
			t.Errorf("%s flight never arrived", span.name)
			continue
		}
		if last > PlainHeight+1 {
			t.Errorf("%s flight was still %v rows tall at the end, want a plain character", span.name, last)
		}
	}
}

func TestAFlightIsHalfShrunkAboutHalfway(t *testing.T) {
	f := NewFlights()
	to := vec(120, 40)
	f.Launch(Departure{GridID: 1, Digit: '8', From: vec(0, 0), To: to, BinIdx: 0, Height: MaxGlyphHeight})
	span := to.Len()

	for i := 0; i < 3000 && len(f.Active()) > 0; i++ {
		f.Step(1.0 / 60)
		a := f.Active()
		if len(a) == 0 {
			break
		}
		remaining := math.Hypot(a[0].X-to.X, a[0].Y-to.Y)
		if math.Abs(remaining/span-0.5) > 0.02 {
			continue
		}
		// Halfway along, smoothstep puts the size halfway too.
		frac := (a[0].Height - PlainHeight) / (MaxGlyphHeight - PlainHeight)
		if frac < 0.3 || frac > 0.7 {
			t.Errorf("halfway to the bin the digit is at %.0f%% of its size, want roughly half", frac*100)
		}
		return
	}
	t.Skip("never sampled the midpoint")
}

// stepFor advances f by seconds of fixed steps.
func stepFor(f *Flights, seconds float64) {
	for range int(seconds/physics.DT + 0.5) {
		f.Step(physics.DT)
	}
}

// A refined cluster waits a moment where it was refined, still shaking as it
// did on the board, at its size and upright, then leaves.
func TestARefinedClusterShakesInPlaceBeforeItLeaves(t *testing.T) {
	f := NewFlights()
	f.Launch(
		Departure{GridID: 1, Digit: '3', From: vec(10, 5), To: vec(40, 30), BinIdx: 1, Height: MaxGlyphHeight, Tilt: 10},
		Departure{GridID: 2, Digit: '0', From: vec(14, 5), To: vec(40, 30), BinIdx: 1, Height: MaxGlyphHeight, Tilt: -10},
	)
	from := map[int]physics.Vec2{1: vec(10, 5), 2: vec(14, 5)}
	seen := map[int]map[physics.Vec2]bool{1: {}, 2: {}}
	for range int(flightHold / physics.DT) {
		f.Step(physics.DT)
		for _, fl := range f.Active() {
			at := vec(fl.X, fl.Y)
			seen[fl.ID][at] = true
			if d := at.Sub(from[fl.ID]); math.Abs(d.X) > shakeX*2 || math.Abs(d.Y) > shakeY*2 {
				t.Errorf("while it waits, digit %d has wandered to (%.2f,%.2f)", fl.ID, fl.X, fl.Y)
			}
			if fl.Height < MaxGlyphHeight-0.01 || math.Abs(fl.Tilt) > 0.01 {
				t.Errorf("while it waits, digit %d is %.2f rows, leaning %.2f", fl.ID, fl.Height, fl.Tilt)
			}
		}
	}
	for id, at := range seen {
		if flightHold > 0.05 && len(at) < 3 {
			t.Errorf("digit %d froze while it waited: %d places in %v", id, len(at), flightHold)
		}
	}
	stepFor(f, 0.5)
	if fl := f.Active()[0]; fl.Y-5 < shakeY*2+1 {
		t.Errorf("half a second after the wait the nearest digit is still at row %.2f", fl.Y)
	}
}

// The digit nearest its bin leaves first; the rest follow in order, all of
// them within flightSpread.
func TestTheNearestDigitLeavesFirstAndTheRestFollowClosely(t *testing.T) {
	f := NewFlights()
	bin := vec(40, 30)
	var cluster []Departure
	for i := range 9 {
		cluster = append(cluster, Departure{GridID: i, Digit: '1', From: vec(float64(i*3), 2), To: bin, BinIdx: 1})
	}
	f.Launch(cluster...)

	left := map[int]float64{}
	for step := 0; step < 120 && len(left) < len(cluster); step++ {
		f.Step(physics.DT)
		for _, fl := range f.Active() {
			from := cluster[fl.ID].From
			if _, ok := left[fl.ID]; !ok && (fl.X != from.X || fl.Y != from.Y) {
				left[fl.ID] = float64(step+1) * physics.DT
			}
		}
	}
	nearest := 8 // from (24, 2), the closest to (40, 30)
	for id, at := range left {
		if at < left[nearest] {
			t.Errorf("digit %d left at %.2fs, before the nearest at %.2fs", id, at, left[nearest])
		}
		if d := at - left[nearest]; d > flightSpread+physics.DT {
			t.Errorf("digit %d left %.2fs after the first", id, d)
		}
	}
	if len(left) != len(cluster) {
		t.Errorf("only %d of %d digits left", len(left), len(cluster))
	}
	for id := 1; id < nearest; id++ {
		if left[id] < left[id+1] {
			t.Errorf("digit %d, further from the bin, left before digit %d", id, id+1)
		}
	}
}

// Once it leaves it gathers speed from rest, then eases into the bin: most of
// the way in the first half of its time, the rest ever slower.
func TestAFlightStartsSmoothlyAndEasesIntoItsBin(t *testing.T) {
	f := NewFlights()
	from, to := vec(0, 0), vec(60, 0)
	f.Launch(Departure{GridID: 1, Digit: '5', From: from, To: to, BinIdx: 0})
	stepFor(f, flightHold)

	var xs []float64
	for len(f.Active()) > 0 {
		f.Step(physics.DT)
		if a := f.Active(); len(a) > 0 {
			xs = append(xs, a[0].X)
		}
	}
	if first := xs[0] - from.X; first > 0.5 {
		t.Errorf("it jumped %.2f cells on its first frame away", first)
	}
	half := int(flightTravel / 2 / physics.DT)
	if half < len(xs) && xs[half]/to.X < 0.6 {
		t.Errorf("halfway through its time it had come %.0f%% of the way", 100*xs[half]/to.X)
	}
	peak := 0
	for i := 1; i < len(xs); i++ {
		if xs[i]-xs[i-1] > xs[peak+1]-xs[peak] {
			peak = i - 1
		}
	}
	for i := peak + 1; i+1 < len(xs); i++ {
		if xs[i+1]-xs[i] > xs[i]-xs[i-1]+1e-9 {
			t.Fatalf("it sped up again at x=%.2f, after its fastest at x=%.2f", xs[i], xs[peak])
		}
	}
	if float64(peak) > flightStart*flightTravel/physics.DT+2 {
		t.Errorf("it was fastest %d frames in, after it should have been easing out", peak)
	}
}

// Leaving one after another, a cluster's digits still come into the bin
// together, from near and far alike.
func TestAClusterArrivesTogether(t *testing.T) {
	f := NewFlights()
	bin := vec(40, 30)
	var cluster []Departure
	for i := range 6 {
		cluster = append(cluster, Departure{GridID: i, Digit: '7', From: vec(float64(i*7), float64(i)), To: bin, BinIdx: 1})
	}
	f.Launch(cluster...)
	first, last := -1, -1
	for step := 0; step < 300 && len(f.Active()) > 0; step++ {
		f.Step(physics.DT)
		if len(f.TakeLanded()) > 0 {
			if first < 0 {
				first = step
			}
			last = step
		}
	}
	if first < 0 || last-first > 2 {
		t.Errorf("the cluster landed between frames %d and %d, not together", first, last)
	}
}

func TestAClusterIsHeardOnceHalfwayToItsBin(t *testing.T) {
	f := NewFlights()
	f.Launch(
		Departure{GridID: 1, Digit: '1', From: vec(0, 0), To: vec(40, 30), BinIdx: 1, Mine: true},
		Departure{GridID: 2, Digit: '2', From: vec(4, 0), To: vec(40, 30), BinIdx: 1, Mine: true},
	)
	stepFor(f, flightHold)
	if got := f.TakeHalfway(); len(got) != 0 {
		t.Errorf("heard %v while it held", got)
	}
	var heard []bool
	for range 120 {
		f.Step(physics.DT)
		heard = append(heard, f.TakeHalfway()...)
	}
	if len(heard) != 1 || !heard[0] {
		t.Errorf("the cluster was heard %v, want once, as this refiner's", heard)
	}
}
