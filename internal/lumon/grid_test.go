package lumon

import (
	"testing"

	"github.com/terminaltrove/lumon/internal/noise"
)

func TestNewGridFillsEveryCellWithItsOwnCoordinates(t *testing.T) {
	g := NewGrid(1)

	for y := 0; y < GridSize; y++ {
		for x := 0; x < GridSize; x++ {
			n := g.At(x, y)
			if n == nil {
				t.Fatalf("cell (%d,%d) is nil", x, y)
			}
			if n.GridX != x || n.GridY != y {
				t.Fatalf("cell (%d,%d) reports (%d,%d)", x, y, n.GridX, n.GridY)
			}
			if n.ID != GridIndex(x, y) {
				t.Fatalf("cell (%d,%d) ID = %d, want %d", x, y, n.ID, GridIndex(x, y))
			}
		}
	}
}

func TestGridAtRejectsOutOfBounds(t *testing.T) {
	g := NewGrid(1)

	for _, c := range [][2]int{{-1, 0}, {0, -1}, {GridSize, 0}, {0, GridSize}} {
		if got := g.At(c[0], c[1]); got != nil {
			t.Errorf("At(%d,%d) = %v, want nil", c[0], c[1], got)
		}
	}
}

func TestGridValuesAreSingleDigits(t *testing.T) {
	g := NewGrid(1)

	for y := 0; y < GridSize; y++ {
		for x := 0; x < GridSize; x++ {
			if v := g.At(x, y).Value; v < 0 || v > 9 {
				t.Fatalf("cell (%d,%d) value = %d, want 0-9", x, y, v)
			}
		}
	}
}

// The alternating drift axis gives the grid its woven look. This measures the
// motion on screen, since a field recording the axis can be set and never read.
func TestNeighbouringDigitsDriftAlongDifferentAxes(t *testing.T) {
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			want := (x+y)%2 == 0
			if got := DriftsHorizontally(x, y); got != want {
				t.Errorf("cell (%d,%d) drifts horizontally = %v, want %v", x, y, got, want)
			}
		}
	}

	// The drift moves them on that axis and not the other.
	m := ready()
	d := m.Drift
	tick(m, 240)

	var horiz, vert int
	l := m.Layout
	for y := l.ViewportY; y < l.ViewportY+min(l.ViewportH, 12); y++ {
		for x := l.ViewportX; x < l.ViewportX+min(l.ViewportW, 12); x++ {
			dx, dy := d.Offset(x, y)
			if DriftsHorizontally(x, y) {
				if dy != 0 {
					t.Fatalf("cell (%d,%d) drifts horizontally but moved %v vertically", x, y, dy)
				}
				if dx != 0 {
					horiz++
				}
				continue
			}
			if dx != 0 {
				t.Fatalf("cell (%d,%d) drifts vertically but moved %v horizontally", x, y, dx)
			}
			if dy != 0 {
				vert++
			}
		}
	}

	// Both families have to be moving, or "woven" is just "still".
	if horiz == 0 || vert == 0 {
		t.Errorf("after four seconds %d digits drift across and %d down; want both", horiz, vert)
	}
}

func TestBadFlagsFollowThePerlinMap(t *testing.T) {
	g := NewGrid(1)

	for y := 0; y < GridSize; y++ {
		for x := 0; x < GridSize; x++ {
			n := g.At(x, y)
			isBad := n.BadGroupID >= 0
			if want := noise.IsBadNumber(x, y); isBad != want {
				t.Fatalf("cell (%d,%d) bad = %v, want %v", x, y, isBad, want)
			}
		}
	}
}

func TestGoodNumbersHaveNoGroup(t *testing.T) {
	g := NewGrid(1)

	n := 0
	for y := 0; y < GridSize; y++ {
		for x := 0; x < GridSize; x++ {
			if !noise.IsBadNumber(x, y) {
				if got := g.At(x, y).BadGroupID; got != -1 {
					t.Fatalf("good cell (%d,%d) has group %d, want -1", x, y, got)
				}
				n++
			}
		}
	}
	if n == 0 {
		t.Fatal("no good numbers at all; the map is wrong")
	}
}

// 277 8-connected clusters over 808 bad cells. Both numbers are fixed by the
// Perlin map, so a change here means the clustering or the noise moved.
func TestClusteringProducesTheExpectedGroups(t *testing.T) {
	g := NewGrid(1)

	if got := len(g.Groups); got != 277 {
		t.Errorf("group count = %d, want 277", got)
	}

	total := 0
	for _, grp := range g.Groups {
		total += len(grp.NumberIDs)
	}
	if total != 808 {
		t.Errorf("clustered cells = %d, want 808", total)
	}
}

// Otherwise the flood fill merged unrelated regions.
func TestEveryGroupIsEightConnected(t *testing.T) {
	g := NewGrid(1)

	for _, grp := range g.Groups {
		members := make(map[int]bool, len(grp.NumberIDs))
		for _, id := range grp.NumberIDs {
			members[id] = true
		}

		seen := map[int]bool{grp.NumberIDs[0]: true}
		stack := []int{grp.NumberIDs[0]}
		for len(stack) > 0 {
			id := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			x, y := id%GridSize, id/GridSize
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					nid := GridIndex(x+dx, y+dy)
					if x+dx < 0 || y+dy < 0 || x+dx >= GridSize || y+dy >= GridSize {
						continue
					}
					if members[nid] && !seen[nid] {
						seen[nid] = true
						stack = append(stack, nid)
					}
				}
			}
		}

		if len(seen) != len(grp.NumberIDs) {
			t.Fatalf("group %d has %d members but only %d are connected", grp.ID, len(grp.NumberIDs), len(seen))
		}
	}
}

func TestEveryBadNumberPointsAtAGroupThatClaimsIt(t *testing.T) {
	g := NewGrid(1)

	for y := 0; y < GridSize; y++ {
		for x := 0; x < GridSize; x++ {
			n := g.At(x, y)
			if n.BadGroupID < 0 {
				continue
			}
			grp := g.Group(n.BadGroupID)
			if grp == nil {
				t.Fatalf("cell (%d,%d) references missing group %d", x, y, n.BadGroupID)
			}
			found := false
			for _, id := range grp.NumberIDs {
				if id == n.ID {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("group %d does not list cell (%d,%d)", grp.ID, x, y)
			}
		}
	}
}

// Group numbering and bin assignment must not depend on map iteration order.
func TestGridGenerationIsDeterministicForASeed(t *testing.T) {
	a, b := NewGrid(42), NewGrid(42)

	for y := 0; y < GridSize; y++ {
		for x := 0; x < GridSize; x++ {
			na, nb := a.At(x, y), b.At(x, y)
			if *na != *nb {
				t.Fatalf("cell (%d,%d): %+v vs %+v", x, y, *na, *nb)
			}
		}
	}

	if len(a.Groups) != len(b.Groups) {
		t.Fatalf("group counts differ: %d vs %d", len(a.Groups), len(b.Groups))
	}
	for i := range a.Groups {
		if len(a.Groups[i].NumberIDs) != len(b.Groups[i].NumberIDs) {
			t.Errorf("group %d size: %d vs %d", i,
				len(a.Groups[i].NumberIDs), len(b.Groups[i].NumberIDs))
		}
	}
}

// Group 0 is the first bad cell reading top-left to bottom-right.
func TestGroupIDsFollowScanOrder(t *testing.T) {
	g := NewGrid(1)

	wantSeeds := [][2]int{{7, 0}, {25, 0}, {43, 0}, {80, 0}, {85, 0}}
	wantSizes := []int{7, 1, 1, 2, 3}

	for i, want := range wantSeeds {
		grp := g.Groups[i]
		first := grp.NumberIDs[0]
		if got := [2]int{first % GridSize, first / GridSize}; got != want {
			t.Errorf("group %d starts at %v, want %v", i, got, want)
		}
		if got := len(grp.NumberIDs); got != wantSizes[i] {
			t.Errorf("group %d size = %d, want %d", i, got, wantSizes[i])
		}
	}
}

// The bad map comes from Perlin, which is seeded independently of the game.
func TestSeedChangesDigitsButNotTheBadMap(t *testing.T) {
	a, b := NewGrid(1), NewGrid(999)

	differing := 0
	for y := 0; y < GridSize; y++ {
		for x := 0; x < GridSize; x++ {
			if a.At(x, y).Value != b.At(x, y).Value {
				differing++
			}
			if (a.At(x, y).BadGroupID >= 0) != (b.At(x, y).BadGroupID >= 0) {
				t.Fatalf("bad map differs at (%d,%d) between seeds", x, y)
			}
		}
	}
	if differing == 0 {
		t.Error("different seeds produced identical digits")
	}
}

func TestBinsAccountForEveryGroup(t *testing.T) {
	g := NewGrid(1)

	total := 0
	for i := range g.Bins {
		total += g.Bins[i].Quota
	}

	if total != len(g.Groups) {
		t.Errorf("bins claim %d groups, but there are %d", total, len(g.Groups))
	}
}

// Otherwise the file can never reach 100%.
func TestTheBinQuotasAccountForEveryCluster(t *testing.T) {
	g := NewGrid(1)

	total := 0
	for i := range g.Bins {
		if g.Bins[i].Quota < 0 {
			t.Fatalf("bin %d has a negative quota", i)
		}
		total += g.Bins[i].Quota
	}
	if total != len(g.Groups) {
		t.Errorf("bins want %d clusters between them, board has %d", total, len(g.Groups))
	}
}

// An even share, give or take the remainder.
func TestTheBinQuotasAreShared(t *testing.T) {
	g := NewGrid(1)

	lo, hi := g.Bins[0].Quota, g.Bins[0].Quota
	for i := range g.Bins {
		if q := g.Bins[i].Quota; q < lo {
			lo = q
		} else if q > hi {
			hi = q
		}
	}
	if hi-lo > 1 {
		t.Errorf("bin quotas range from %d to %d, want them within one of each other", lo, hi)
	}
}
