package lumon

import "testing"

func TestSandSettlesOnTheFloor(t *testing.T) {
	m := ready()
	m.Resize(80, 20)
	startDiversionNamed(t, m, "SAND")
	if m.sand == nil {
		t.Fatalf("SAND did not start; screen is %v", m.Screen)
	}
	n := m.sand.Count()
	if n == 0 {
		t.Fatal("no grains at all")
	}

	tick(m, 60*8)

	if got := m.sand.Count(); got != n {
		t.Errorf("%d grains of %d survived; none should be lost", got, n)
	}
	// Everything should be near the bottom.
	high := 0
	for y := 0; y < m.sand.h/2; y++ {
		for x := 0; x < m.sand.w; x++ {
			if m.sand.at(x, y) != 0 {
				high++
			}
		}
	}
	if high > n/4 {
		t.Errorf("%d of %d grains are still in the top half after eight seconds", high, n)
	}
}

// Grains poured into one column spread at the angle of repose.
func TestPouringBuildsAConeRatherThanATower(t *testing.T) {
	m := ready()
	m.Resize(80, 20)
	startDiversionNamed(t, m, "SAND")

	mid := 40
	m.sand.Pour(mid, 1)
	tick(m, 60*6)
	m.sand.StopPouring()
	tick(m, 60)

	// The heap's width at the bottom against its height.
	var wide, tall int
	for x := 0; x < m.sand.w; x++ {
		if m.sand.at(x, m.sand.h-1) != 0 {
			wide++
		}
	}
	for y := 0; y < m.sand.h; y++ {
		if m.sand.at(mid, y) != 0 {
			tall++
		}
	}
	if wide < 8 {
		t.Errorf("a heap poured for six seconds is only %d cells wide; it is a tower", wide)
	}
	if wide <= tall {
		t.Errorf("the heap is %d wide and %d tall; sand should spread wider than it stacks", wide, tall)
	}
}

// Grains never overlap, leave the box, or vanish.
func TestSandKeepsEveryGrainInsideTheBox(t *testing.T) {
	m := ready()
	// The footer is two rows, so a sixteen-row terminal leaves the play panel one
	// row and no box.
	m.Resize(60, 18)
	startDiversionNamed(t, m, "SAND")
	m.sand.Pour(30, 0)
	tick(m, 60*4)
	m.sand.StopPouring()

	before := m.sand.Count()
	tick(m, 60*4)
	if got := m.sand.Count(); got != before {
		t.Errorf("the count went from %d to %d with nothing pouring", before, got)
	}

	// The box stops one row short of the panel's bottom, which is the hint's row.
	// The hint has no digits, so any digit there is a grain that got out.
	panel := m.gridPanel()
	if m.sand.h != panel.H-1 {
		t.Errorf("the sand box is %d rows tall in a %d-row panel; it should leave the hint a line",
			m.sand.h, panel.H)
	}
	row := drawTo(m).Row(panel.Y + panel.H - 1)
	for _, r := range row {
		if r >= '0' && r <= '9' {
			t.Errorf("a grain reached the hint row:\n%q", row)
			break
		}
	}
}

// Settling runs bottom-up. Top-down, each row would move into the space just
// vacated below it and a grain would fall the full height in one update.
func TestAGrainFallsOneCellPerUpdate(t *testing.T) {
	m := ready()
	m.Resize(40, 22)
	startDiversionNamed(t, m, "SAND")

	// A box with one grain near the top.
	for i := range m.sand.grain {
		m.sand.grain[i] = 0
	}
	m.sand.grain[2*m.sand.w+20] = '7'

	m.sand.settle()

	if m.sand.at(20, 3) != '7' {
		t.Errorf("after one update the grain is not one row down; row 3 holds %q", m.sand.at(20, 3))
	}
	if m.sand.at(20, 4) != 0 {
		t.Error("the grain fell more than one cell in a single update")
	}
}
