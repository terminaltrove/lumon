package lumon

import (
	"math/rand"

	"github.com/terminaltrove/lumon/internal/noise"
)

// GridSize is the width and height of the number grid, matching the 100x100
// grid in LumonMDR's NumberGrid.
const GridSize = 100

// BinCount is the number of refinement bins along the bottom of the screen.
const BinCount = 5

// Number is one digit in the grid. Cells are values in a flat array because
// the renderer walks every visible cell each frame.
type Number struct {
	ID    int // GridIndex(GridX, GridY)
	GridX int
	GridY int
	Value int // 0-9

	// BadGroupID is the cluster this digit belongs to, or -1 if it is a normal
	// number. "Bad" is decided by Perlin noise, not by the seed.
	BadGroupID int
}

// BadGroup is a cluster of connected bad numbers, refined as a unit.
type BadGroup struct {
	ID        int
	NumberIDs []int
}

// DriftsHorizontally reports which axis a digit's ambient wobble runs along. It
// alternates in a checkerboard, so neighbours drift across each other, as the
// C++ reference does by giving each number one axis (Number.h).
func DriftsHorizontally(x, y int) bool { return (x+y)%2 == 0 }

// Bin is a refinement bucket.
type Bin struct {
	ID      int // 1-based, as displayed
	Refined int
	Quota   int // how many clusters this bin is waiting for

	// Tempers is how much of each of the four this bin holds (Woe, Frolic,
	// Dread, Malice, in Kier's order), as the drawer above the bin reads out.
	// Unlike Quota it is saved, since the temper belonged to a cluster that is
	// gone and cannot be reconstructed.
	Tempers [TemperCount]int
}

// TemperShare is how much of the bin's quota temper t is to fill: a quarter,
// in whole clusters, the first tempers taking any left over, so the four add
// up to the quota.
func (b *Bin) TemperShare(t Temper) int {
	share := b.Quota / TemperCount
	if int(t) < b.Quota%TemperCount {
		share++
	}
	return share
}

// take credits the bin with one cluster of temper t, or, if t already has its
// share, of the temper with the most of its share still to fill, so a bin
// filled to its quota has every temper's share exactly.
func (b *Bin) take(t Temper) {
	if b.Tempers[t] >= b.TemperShare(t) {
		most := -1
		for u := range Temper(TemperCount) {
			if left := b.TemperShare(u) - b.Tempers[u]; left > most {
				t, most = u, left
			}
		}
	}
	b.Refined++
	b.Tempers[t]++
}

// Percentage returns completion in [0,1].
func (b *Bin) Percentage() float64 {
	if b.Quota == 0 {
		return 0
	}
	return float64(b.Refined) / float64(b.Quota)
}

// Full reports whether the bin has taken everything it is owed. Refining into a
// full bin is refused.
func (b *Bin) Full() bool { return b.Refined >= b.Quota }

// PercentInt returns completion as a whole percentage, capped at 100: a full
// bin refuses work, but two people refining the same cluster at once can
// overfill it.
func (b *Bin) PercentInt() int {
	if p := int(b.Percentage() * 100); p < 100 {
		return p
	}
	return 100
}

// Grid is the full board: every digit, the bad clusters over it, and the bins
// those clusters are destined for.
type Grid struct {
	Cells  [GridSize * GridSize]Number
	Groups []*BadGroup
	Bins   [BinCount]*Bin

	byID map[int]*BadGroup
}

// shareOutBins divides the board's clusters evenly between the bins. The
// remainder goes to the leftmost bins (as Layout does with columns), since
// dropping it would make the file uncompletable.
func (g *Grid) shareOutBins() {
	base, extra := len(g.Groups)/BinCount, len(g.Groups)%BinCount
	for i := range g.Bins {
		g.Bins[i].Quota = base
		if i < extra {
			g.Bins[i].Quota++
		}
	}
}

// GridIndex converts coordinates to a flat index.
func GridIndex(x, y int) int { return y*GridSize + x }

// NewGrid builds a grid. The seed controls the digits and each cluster's target
// bin; which cells are bad comes from Perlin noise seeded independently to match
// the C++ reference. Generation walks in scan order, never over a map, so a
// seeded grid is reproducible.
func NewGrid(seed int64) *Grid { return NewGridAt(seed, 0) }

// NewGridAt builds the board for one file. The epoch picks which file: 0 is the
// canonical layout, and each one after it is a board of its own.
func NewGridAt(seed int64, epoch int) *Grid {
	r := rand.New(rand.NewSource(seed))
	g := &Grid{byID: make(map[int]*BadGroup)}

	for i := range g.Bins {
		g.Bins[i] = &Bin{ID: i + 1}
	}

	bad := make([]bool, GridSize*GridSize)
	for y := 0; y < GridSize; y++ {
		for x := 0; x < GridSize; x++ {
			id := GridIndex(x, y)
			g.Cells[id] = Number{
				ID:         id,
				GridX:      x,
				GridY:      y,
				Value:      r.Intn(10),
				BadGroupID: -1,
			}
			bad[id] = noise.IsBadNumberAt(x, y, epoch)
		}
	}

	visited := make([]bool, GridSize*GridSize)
	for y := 0; y < GridSize; y++ {
		for x := 0; x < GridSize; x++ {
			id := GridIndex(x, y)
			if !bad[id] || visited[id] {
				continue
			}
			grp := &BadGroup{
				ID:        len(g.Groups),
				NumberIDs: make([]int, 0, 16),
			}
			g.floodFill(id, bad, visited, grp)
			g.Groups = append(g.Groups, grp)
			g.byID[grp.ID] = grp
		}
	}

	g.shareOutBins()
	return g
}

// floodFill claims every bad cell 8-connected to start for grp.
func (g *Grid) floodFill(start int, bad, visited []bool, grp *BadGroup) {
	stack := make([]int, 0, 64)
	stack = append(stack, start)

	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if visited[id] || !bad[id] {
			continue
		}
		visited[id] = true

		g.Cells[id].BadGroupID = grp.ID
		grp.NumberIDs = append(grp.NumberIDs, id)

		x, y := id%GridSize, id/GridSize
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}
				nx, ny := x+dx, y+dy
				if nx < 0 || ny < 0 || nx >= GridSize || ny >= GridSize {
					continue
				}
				if nid := GridIndex(nx, ny); bad[nid] && !visited[nid] {
					stack = append(stack, nid)
				}
			}
		}
	}
}

// At returns the digit at (x, y), or nil if the coordinates are off the grid.
func (g *Grid) At(x, y int) *Number {
	if x < 0 || y < 0 || x >= GridSize || y >= GridSize {
		return nil
	}
	return &g.Cells[GridIndex(x, y)]
}

// Group returns a cluster by ID, or nil if it has been refined away.
func (g *Grid) Group(id int) *BadGroup { return g.byID[id] }
