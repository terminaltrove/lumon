package physics

import "math"

// hashGrid is a uniform spatial hash used by Collide and the fluid. With cells
// as wide as a query's reach, only the nine cells around a point need
// checking; all-pairs at six hundred digits, run Substeps x Iterations = 48
// times a step, is about 8.5 million tests a frame.
//
// heads indexes cells and next chains the particles in each as a linked list,
// so a rebuild after the first allocates nothing.
type hashGrid struct {
	cols, rows int
	origin     Vec2
	cell       float64
	heads      []int32
	next       []int32
}

// maxCells caps the grid. A world that has flown apart falls back to a single
// cell (all-pairs: slow but correct) so it cannot allocate gigabytes.
const maxCells = 1 << 20

// build bins every particle into a grid of the given cell size.
func (g *hashGrid) build(pos []Vec2, cell float64) {
	if len(pos) == 0 {
		g.cols, g.rows = 0, 0
		return
	}
	g.cell = cell

	min, max := pos[0], pos[0]
	for _, p := range pos[1:] {
		if p.X < min.X {
			min.X = p.X
		}
		if p.Y < min.Y {
			min.Y = p.Y
		}
		if p.X > max.X {
			max.X = p.X
		}
		if p.Y > max.Y {
			max.Y = p.Y
		}
	}
	g.origin = min

	g.cols = int((max.X-min.X)/g.cell) + 1
	g.rows = int((max.Y-min.Y)/g.cell) + 1
	if g.cols < 1 || g.rows < 1 || g.cols*g.rows > maxCells {
		g.cols, g.rows = 1, 1
		g.cell = math.Inf(1)
	}

	cells := g.cols * g.rows
	if cap(g.heads) < cells {
		g.heads = make([]int32, cells)
	}
	g.heads = g.heads[:cells]
	for i := range g.heads {
		g.heads[i] = -1
	}

	if cap(g.next) < len(pos) {
		g.next = make([]int32, len(pos))
	}
	g.next = g.next[:len(pos)]

	for i, p := range pos {
		cx, cy := g.cellOf(p)
		k := cy*g.cols + cx
		g.next[i] = g.heads[k]
		g.heads[k] = int32(i)
	}
}

// cellOf is which bin a point falls in, clamped to the grid.
func (g *hashGrid) cellOf(p Vec2) (int, int) {
	if math.IsInf(g.cell, 1) {
		return 0, 0
	}
	cx := int((p.X - g.origin.X) / g.cell)
	cy := int((p.Y - g.origin.Y) / g.cell)
	if cx < 0 {
		cx = 0
	}
	if cy < 0 {
		cy = 0
	}
	if cx >= g.cols {
		cx = g.cols - 1
	}
	if cy >= g.rows {
		cy = g.rows - 1
	}
	return cx, cy
}

// near calls f with every particle in the nine cells around a point. It is a
// broad phase: the caller does the exact test. Nothing within one cell width
// is missed, so the cell must be at least as wide as the caller's reach.
func (g *hashGrid) near(p Vec2, f func(i int)) {
	if g.cols == 0 {
		return
	}
	cx, cy := g.cellOf(p)
	for oy := -1; oy <= 1; oy++ {
		for ox := -1; ox <= 1; ox++ {
			gx, gy := cx+ox, cy+oy
			if gx < 0 || gy < 0 || gx >= g.cols || gy >= g.rows {
				continue
			}
			for j := g.heads[gy*g.cols+gx]; j >= 0; j = g.next[j] {
				f(int(j))
			}
		}
	}
}

// size is how many particles were binned, so a caller can notice the world
// changed.
func (g *hashGrid) size() int { return len(g.next) }
