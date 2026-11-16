package physics

import (
	"math"
	"math/rand"
	"testing"
)

// The broad phase may offer more than its reach, since it returns whole cells,
// but never miss anything inside it. A miss lets a pair pass through each other
// on some frames and not others, depending on where the grid's origin lands.
func TestTheHashMissesNothingWithinItsReach(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	pos := make([]Vec2, 400)
	for i := range pos {
		pos[i] = Vec2{X: rng.Float64() * 90, Y: rng.Float64() * 40}
	}

	const reach = 2.0
	var g hashGrid
	g.build(pos, reach)

	for i, p := range pos {
		offered := map[int]bool{}
		g.near(p, func(j int) { offered[j] = true })

		for j, q := range pos {
			if q.Sub(p).Len() > reach {
				continue
			}
			if !offered[j] {
				t.Fatalf("%d is %.2f from %d, inside the reach of %.2f, and was not offered",
					j, q.Sub(p).Len(), i, reach)
			}
		}
	}
}

// A world flung to the far corners falls back to one cell (all pairs), slow
// but correct, instead of allocating gigabytes of grid.
func TestAScatteredWorldFallsBackToOneCell(t *testing.T) {
	pos := []Vec2{{X: 0, Y: 0}, {X: 1e12, Y: 1e12}, {X: 5, Y: 5}}
	var g hashGrid
	g.build(pos, 1)

	if g.cols != 1 || g.rows != 1 {
		t.Errorf("the grid is %dx%d, want one cell", g.cols, g.rows)
	}
	if !math.IsInf(g.cell, 1) {
		t.Errorf("the cell is %v, want infinite so everything lands in it", g.cell)
	}

	// It still offers everybody.
	for _, p := range pos {
		n := 0
		g.near(p, func(int) { n++ })
		if n != len(pos) {
			t.Errorf("one cell offered %d of %d particles", n, len(pos))
		}
	}
}

// Rebuilding runs once a substep for the life of a diversion.
func TestTheHashDoesNotReallocateOnEveryBuild(t *testing.T) {
	pos := make([]Vec2, 200)
	for i := range pos {
		pos[i] = Vec2{X: float64(i % 20), Y: float64(i / 20)}
	}
	var g hashGrid
	g.build(pos, 1)
	heads, next := &g.heads[0], &g.next[0]

	for i := 0; i < 50; i++ {
		g.build(pos, 1)
	}
	if &g.heads[0] != heads || &g.next[0] != next {
		t.Error("a rebuild at the same size allocated new arrays")
	}
}

func TestAnEmptyHashOffersNothing(t *testing.T) {
	var g hashGrid
	g.build(nil, 1)
	g.near(Vec2{}, func(int) { t.Error("an empty grid offered a particle") })
	if g.size() != 0 {
		t.Errorf("an empty grid reports %d particles", g.size())
	}
}
