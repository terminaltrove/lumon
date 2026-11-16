package physics

import (
	"math"
	"testing"
)

// pool fills a box with a block of fluid and lets it settle.
func pool(cols, rows int) *Fluid {
	f := NewFluid()
	sp := f.Spacing()
	f.Gravity = Vec2{Y: 30}
	for x := 0; x < cols; x++ {
		for y := 0; y < rows; y++ {
			f.Add(Vec2{
				X: 4 + float64(x)*sp,
				Y: 2 + float64(y)*sp,
			})
		}
	}
	return f
}

// The rest density follows from the kernel: what a particle in the middle of a
// full lattice measures. If it is wrong, the fluid collapses to a point or
// blows apart, which looks like a bug in the forces.
func TestTheRestDensityIsWhatAFullLatticeMeasures(t *testing.T) {
	f := NewFluid()
	if f.RestDensity() <= 0 {
		t.Fatalf("the rest density is %v", f.RestDensity())
	}

	// A lone particle measures only itself.
	lone := f.poly6 * f.h2 * f.h2 * f.h2
	if lone >= f.RestDensity() {
		t.Errorf("a lone particle measures %v against a rest density of %v; nothing "+
			"would ever be under pressure", lone, f.RestDensity())
	}
}

// A solver with the pressure sign wrong keeps shrinking while still looking
// busy.
func TestTheFluidSettlesAndKeepsItsVolume(t *testing.T) {
	f := pool(12, 12)
	min := Vec2{X: 0, Y: 0}
	max := Vec2{X: 24, Y: 18}

	spread := func() float64 {
		lo, hi := f.Pos[0], f.Pos[0]
		for _, p := range f.Pos[1:] {
			lo.X, lo.Y = math.Min(lo.X, p.X), math.Min(lo.Y, p.Y)
			hi.X, hi.Y = math.Max(hi.X, p.X), math.Max(hi.Y, p.Y)
		}
		return (hi.X - lo.X) * (hi.Y - lo.Y)
	}

	was := spread()
	for i := 0; i < 900; i++ {
		f.Step(1.0/120, min, max, 0.3)
	}

	for i, p := range f.Pos {
		if math.IsNaN(p.X) || math.IsNaN(p.Y) {
			t.Fatalf("particle %d went to NaN, the solver came apart", i)
		}
		if p.X < min.X-0.01 || p.X > max.X+0.01 || p.Y < min.Y-0.01 || p.Y > max.Y+0.01 {
			t.Fatalf("particle %d escaped to %v", i, p)
		}
	}

	fast := 0
	for _, v := range f.Vel {
		if v.Len() > 2 {
			fast++
		}
	}
	if fast > len(f.Vel)/10 {
		t.Errorf("%d of %d particles are still moving fast; it is not settling",
			fast, len(f.Vel))
	}

	// A block dropped into a wider box spreads, so this is a band.
	now := spread()
	if now < was*0.4 {
		t.Errorf("the fluid occupies %.0f against %.0f at the start; it collapsed", now, was)
	}
	if now > was*6 {
		t.Errorf("the fluid occupies %.0f against %.0f at the start; it came apart", now, was)
	}
}

func TestTheFluidFallsAndPoolsAtTheFloor(t *testing.T) {
	f := pool(10, 8)
	min, max := Vec2{X: 0, Y: 0}, Vec2{X: 20, Y: 30}

	before := 0.0
	for _, p := range f.Pos {
		before += p.Y
	}
	before /= float64(len(f.Pos))

	for i := 0; i < 1200; i++ {
		f.Step(1.0/120, min, max, 0.2)
	}

	after := 0.0
	for _, p := range f.Pos {
		after += p.Y
	}
	after /= float64(len(f.Pos))

	if after <= before {
		t.Errorf("the fluid's middle is at %.1f, no lower than the %.1f it started at",
			after, before)
	}
	if after < max.Y*0.5 {
		t.Errorf("the fluid settled at %.1f in a box %.0f deep; it is not on the floor",
			after, max.Y)
	}
}

// Pressure is the excess over rest density; a particle with nothing near it is
// at the surface and must not be pulled inward.
func TestOnlyCrowdedParticlesAreUnderPressure(t *testing.T) {
	f := NewFluid()
	for x := 0; x < 6; x++ {
		for y := 0; y < 6; y++ {
			f.Add(Vec2{X: 10 + float64(x)*f.Spacing()*0.8, Y: 10 + float64(y)*f.Spacing()*0.8})
		}
	}
	lone := f.Len()
	f.Add(Vec2{X: 100, Y: 100})

	f.grid.build(f.Pos, f.h)
	f.densities()

	if f.pressure[lone] != 0 {
		t.Errorf("a particle on its own is under pressure %v, want none", f.pressure[lone])
	}
	middle := 0
	for i := 0; i < lone; i++ {
		if f.pressure[i] > 0 {
			middle++
		}
	}
	if middle == 0 {
		t.Error("nothing in a tight clump is under pressure; it would never spread")
	}
}

func TestAnEmptyFluidSteps(t *testing.T) {
	f := NewFluid()
	f.Step(1.0/60, Vec2{}, Vec2{X: 10, Y: 10}, 0.5)
	if f.Len() != 0 {
		t.Errorf("an empty fluid grew to %d particles", f.Len())
	}
}

// There is no emission rate: particles arrive only as fast as the last ones
// leave, which makes a jet instead of an explosion.
func TestAMouthEmitsOnlyAsFastAsItClears(t *testing.T) {
	f := NewFluid()
	mouth := []Emitter{{At: Vec2{X: 10, Y: 10}, Vel: Vec2{Y: -6}, Reach: f.Spacing() * 2}}

	f.Emit(mouth)
	first := f.Len()
	if first == 0 {
		t.Fatal("the mouth emitted nothing at all")
	}

	f.Emit(mouth)
	if f.Len() != first {
		t.Errorf("a second emission with nothing moved added %d more; there is a rate "+
			"somewhere and there should not be", f.Len()-first)
	}

	for i := 0; i < 40; i++ {
		f.Step(1.0/120, Vec2{X: 0, Y: -50}, Vec2{X: 20, Y: 20}, 0.2)
	}
	f.Emit(mouth)
	if f.Len() <= first {
		t.Errorf("the mouth did not refill once its output had cleared: %d then %d",
			first, f.Len())
	}
}

func TestAFluidIsBounded(t *testing.T) {
	f := NewFluid()
	f.Gravity = Vec2{Y: 30}
	mouths := []Emitter{
		{At: Vec2{X: 10, Y: 30}, Vel: Vec2{Y: -8}, Reach: 3},
		{At: Vec2{X: 30, Y: 30}, Vel: Vec2{Y: -8}, Reach: 3},
	}
	for i := 0; i < 4000; i++ {
		f.Emit(mouths)
		f.Step(1.0/120, Vec2{X: 0, Y: 0}, Vec2{X: 40, Y: 30}, 0.2)
	}
	if f.Len() > MaxParticles {
		t.Errorf("the fluid grew to %d, over the cap of %d", f.Len(), MaxParticles)
	}
	if f.Len() == 0 {
		t.Error("nothing was emitted at all over four thousand frames")
	}
}

// Upstream returns from every emitter once the cap is reached, which with five
// mouths would let the first two starve the rest.
func TestEveryMouthGetsATurn(t *testing.T) {
	f := NewFluid()
	var mouths []Emitter
	for i := 0; i < 5; i++ {
		mouths = append(mouths, Emitter{
			At:    Vec2{X: float64(10 + i*20), Y: 10},
			Vel:   Vec2{Y: -6},
			Reach: f.Spacing() * 2,
		})
	}
	f.Emit(mouths)

	for i, e := range mouths {
		found := false
		for _, p := range f.Pos {
			if math.Abs(p.X-e.At.X) <= e.Reach+0.01 {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("mouth %d at x=%.0f emitted nothing", i, e.At.X)
		}
	}
}

func TestAnEmittedParticleLeavesWithItsMouthsSpeed(t *testing.T) {
	f := NewFluid()
	up := Vec2{Y: -7}
	f.Emit([]Emitter{{At: Vec2{X: 5, Y: 20}, Vel: up, Reach: 0}})
	if f.Len() == 0 {
		t.Fatal("nothing was emitted")
	}
	if f.Vel[0] != up {
		t.Errorf("a particle left the mouth at %v, want %v", f.Vel[0], up)
	}
}
