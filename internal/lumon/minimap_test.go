package lumon

import "testing"

// mapRows is the minimap's rows of the drawn frame.
func mapRows(t *testing.T, m *Model) []string {
	t.Helper()
	box, ok := minimapFits(m.Layout)
	if !ok {
		t.Fatal("the map does not fit a terminal this test assumed it would")
	}
	buf := drawTo(m)
	out := make([]string, 0, box.H)
	for y := box.Y; y < box.Y+box.H; y++ {
		out = append(out, buf.Row(y)[box.X:box.X+box.W])
	}
	return out
}

// mapDots is how many braille dots the map has lit. It counts dots, not cells:
// a cell holds eight, so a new mark beside an existing one may not change the
// number of occupied cells.
func mapDots(t *testing.T, m *Model) int {
	t.Helper()
	n := 0
	for _, row := range mapRows(t, m) {
		for _, r := range row {
			if r < 0x2800 || r > 0x28FF {
				continue
			}
			for bits := r - 0x2800; bits > 0; bits >>= 1 {
				n += int(bits & 1)
			}
		}
	}
	return n
}

// The bins sit two cells inside the board's edges, and Layout gives up that
// inset first when the terminal is narrow. Without it there is no margin, and
// the map would be drawn through the bins.
func TestTheMinimapRefusesANarrowTerminal(t *testing.T) {
	wide := ready()
	if _, ok := minimapFits(wide.Layout); !ok {
		t.Error("the map does not fit a hundred and twenty columns, where the margin is fifteen")
	}

	narrow := ready()
	narrow.Resize(80, 24)
	if _, ok := minimapFits(narrow.Layout); ok {
		t.Error("the map claimed to fit eighty columns, where the bins have taken the margin back")
	}

	// And nothing is drawn there.
	buf := drawTo(narrow)
	for y := narrow.Layout.BinTop; y < narrow.Layout.BinTop+BinsHeight; y++ {
		for _, r := range buf.Row(y)[:narrow.Layout.BinOriginX(0)] {
			if r >= 0x2801 && r <= 0x28FF {
				t.Fatalf("the map drew into the bins' margin on a terminal it does not fit")
			}
		}
	}
}

func TestASoloBoardHasNoMinimap(t *testing.T) {
	m := ready()
	tick(m, 5)
	if n := mapDots(t, m); n != 0 {
		t.Errorf("a board with one refiner at it drew %d cells of map", n)
	}
}

func TestTheMinimapShowsWhereTheOtherRefinerIsLooking(t *testing.T) {
	_, a, b := twoInARoom(t)
	a.Resize(120, 40)
	b.Resize(120, 40)

	b.CursorX, b.CursorY = 5, 5
	tick(b, 2)
	tick(a, 2)
	near := mapRows(t, a)

	b.CursorX, b.CursorY = GridSize-5, GridSize-5
	tick(b, 2)
	tick(a, 2)
	far := mapRows(t, a)

	same := true
	for i := range near {
		if near[i] != far[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("the map is the same with the other refiner in either corner of the board")
	}

	// In the colour their nameplate and grid cursor already use.
	box, _ := minimapFits(a.Layout)
	buf := drawTo(a)
	want := a.Theme.DeskColour(b.Desk.Seat)
	found := false
	for y := box.Y; y < box.Y+box.H && !found; y++ {
		for x := box.X; x < box.X+box.W; x++ {
			if buf.AttrAt(x, y).FG == want {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("nothing on the map is drawn in the other refiner's colour")
	}
}

// Hunting is the game, and no key finds a cluster. A map that marked every
// cluster as it woke would give the game away. A cluster appears once somebody
// finds it.
func TestAnUnfoundClusterIsNotOnTheMap(t *testing.T) {
	room, a, b := twoInARoom(t)
	a.Resize(120, 40)
	b.Resize(120, 40)

	// Mid-board, because a cluster near an edge maps to the same dot as the corner
	// ticks that mark the board's extent, and finding it would change no count.
	var grp *BadGroup
	for _, g := range a.Grid.Groups {
		c := centroidOf(g.NumberIDs)
		if c.X > 30 && c.X < 70 && c.Y > 30 && c.Y < 70 {
			grp = g
			break
		}
	}
	if grp == nil {
		t.Skip("no cluster away from the board's edges")
	}
	room.mu.Lock()
	room.Activate(grp)
	room.mu.Unlock()

	tick(a, 2)
	tick(b, 2)
	before := mapDots(t, b)

	room.mu.Lock()
	room.Found(grp.ID)
	room.mu.Unlock()

	tick(b, 2)
	if after := mapDots(t, b); after <= before {
		t.Errorf("finding a cluster put nothing on the map: %d cells before, %d after",
			before, after)
	}
}

// Bubble Tea sends only the rows that changed, so anything animated here costs
// every session bandwidth while it is up. The map draws only what changes on a
// keypress: a cursor, a viewport, a cluster somebody found.
func TestTheMinimapIsStillWhenNobodyMoves(t *testing.T) {
	_, a, b := twoInARoom(t)
	a.Resize(120, 40)
	b.Resize(120, 40)
	b.CursorX, b.CursorY = 40, 40
	tick(a, 30)
	tick(b, 30)

	want := mapRows(t, a)
	for i := 0; i < 300; i++ {
		tick(a, 1)
		got := mapRows(t, a)
		for y := range got {
			if got[y] != want[y] {
				t.Fatalf("the map changed on frame %d with nobody moving:\n  was %q\n  now %q",
					i, want[y], got[y])
			}
		}
	}
}
