package physics

import (
	"math"
	"testing"
)

// falling builds a world with one particle falling in a box.
func falling(from Vec2, floorY float64) (*World, *Bounds, int) {
	w := NewWorld()
	i := w.Add(from, 1)
	b := NewBounds(Vec2{X: -100, Y: -100}, Vec2{X: 100, Y: floorY})
	w.AddConstraint(b)
	return w, b, i
}

func TestAFallingParticleComesToRestOnTheFloor(t *testing.T) {
	w, _, i := falling(Vec2{}, 10)

	for n := 0; n < 600; n++ {
		w.Step(DT, Vec2{Y: 400})
	}

	got := w.Pos(i)
	if got.Y > 10.001 {
		t.Errorf("the particle ended at y=%v, below the floor at 10", got.Y)
	}
	if got.Y < 9.5 {
		t.Errorf("the particle ended at y=%v, nowhere near the floor", got.Y)
	}
	if v := w.Velocity(i, DT).Len(); v > 1 {
		t.Errorf("the particle is still moving at %v after ten seconds on the floor", v)
	}
}

// A position clamp prevents tunnelling at speed.
func TestAFastParticleDoesNotPassThroughTheFloor(t *testing.T) {
	w, _, i := falling(Vec2{Y: -500}, 10)
	w.SetVelocity(i, Vec2{Y: 5000})

	for n := 0; n < 300; n++ {
		w.Step(DT, Vec2{Y: 2000})
		if got := w.Pos(i).Y; got > 10.001 {
			t.Fatalf("after %d steps the particle is at y=%v, through the floor", n, got)
		}
	}
}

// Zero restitution stops it dead; a lot returns most of the drop.
func TestRestitutionDecidesHowMuchBounceComesBack(t *testing.T) {
	// The height reached after the first landing; the whole run's maximum
	// would be the starting height.
	drop := func(r float64) float64 {
		w, b, i := falling(Vec2{}, 100)
		b.Restitution = r
		b.Friction = 0

		landed := false
		peak := math.Inf(1)
		for n := 0; n < 600; n++ {
			w.Step(DT, Vec2{Y: 400})
			y := w.Pos(i).Y
			if !landed {
				landed = y >= 99.99
				continue
			}
			if y < peak {
				peak = y
			}
		}
		return peak
	}

	dead := drop(0)
	lively := drop(0.9)
	if dead < 99.9 {
		t.Errorf("a dead bounce rose to y=%v after landing; it should stay on the floor", dead)
	}
	if lively >= dead {
		t.Errorf("a lively bounce rose to y=%v, no higher than a dead one at %v", lively, dead)
	}
}

func TestFrictionStopsAParticleSlidingForever(t *testing.T) {
	// A wide floor: in the default box (two hundred across) a frictionless
	// particle at this speed bounces off the far wall, which would measure the
	// wall.
	slide := func(f float64) float64 {
		w := NewWorld()
		i := w.Add(Vec2{Y: 9}, 1)
		b := NewBounds(Vec2{X: -1e6, Y: -100}, Vec2{X: 1e6, Y: 10})
		b.Friction = f
		w.AddConstraint(b)
		w.SetVelocity(i, Vec2{X: 200})
		for n := 0; n < 300; n++ {
			w.Step(DT, Vec2{Y: 400})
		}
		return w.Pos(i).X
	}

	slippery := slide(0)
	grippy := slide(0.5)
	if grippy >= slippery {
		t.Errorf("a high-friction floor let the particle reach x=%v, further than a frictionless one at %v",
			grippy, slippery)
	}
}

// A held particle is not moved by a wall.
func TestAPinnedParticleIgnoresTheWalls(t *testing.T) {
	w := NewWorld()
	i := w.Add(Vec2{X: 500, Y: 500}, 0) // pinned, well outside
	w.AddConstraint(NewBounds(Vec2{}, Vec2{X: 10, Y: 10}))

	for n := 0; n < 60; n++ {
		w.Step(DT, Vec2{Y: 400})
	}
	if got := w.Pos(i); got.X != 500 || got.Y != 500 {
		t.Errorf("a pinned particle was moved to %v by the walls", got)
	}
}
