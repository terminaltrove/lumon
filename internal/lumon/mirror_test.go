package lumon

import (
	"strings"
	"testing"
)

// These tests compare two sessions' rendered output: checks on the file name,
// buffer widths, bin counts or Tracked(id) all pass while one refiner sees a
// wall of blocks and the other a row of plain digits on the same cluster.

// seatTwo puts two refiners at one board, both on the same size terminal.
func seatTwo(t *testing.T) (a, b *Model, room *Room) {
	t.Helper()
	room = NewRoom(3)
	a = JoinRoom(room, 1)
	b = JoinRoom(room, 2)
	if a == nil || b == nil {
		t.Fatal("could not seat two refiners")
	}
	a.Resize(120, 40)
	b.Resize(120, 40)
	return a, b, room
}

// wakeVisible wakes a cluster both of them can see and returns it.
func wakeVisible(t *testing.T, m *Model) *BadGroup {
	t.Helper()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	return grp
}

// Size means corrupt, so it cannot mean one thing at one desk and something
// else at the next. Only the plate behind it is private.
func TestAReachShowsOnEveryScreen(t *testing.T) {
	a, b, _ := seatTwo(t)
	grp := wakeVisible(t, a)

	// A ringfences it. B does nothing at all.
	id := grp.NumberIDs[0]
	a.CursorX, a.CursorY = id%GridSize, id/GridSize
	a.Update(key("f"))
	if len(a.held) == 0 {
		t.Fatal("f did not ringfence the cluster under the cursor")
	}

	for i := 0; i < 120; i++ {
		tick(a, 1)
		tick(b, 1)
	}

	for _, member := range grp.NumberIDs {
		gx, gy := member%GridSize, member/GridSize
		ah, bh := a.Magnify.Height(gx, gy), b.Magnify.Height(gx, gy)
		if ah < MaxGlyphHeight-0.5 {
			t.Fatalf("the refiner holding the cluster has member %d at %.2f rows, "+
				"want the full %v", member, ah, MaxGlyphHeight)
		}
		if bh < MaxGlyphHeight-0.5 {
			t.Errorf("member %d is %.2f rows for the refiner holding it and %.2f "+
				"for the one watching; a cluster somebody is working has to be "+
				"the same size on both screens", member, ah, bh)
			break
		}
	}
}

// Your own reach keeps the selection colour, and somebody else's arrives in
// the colour of their cursor and nameplate.
func TestAReachIsMarkedAsWhoseItIs(t *testing.T) {
	a, b, _ := seatTwo(t)
	grp := wakeVisible(t, a)

	id := grp.NumberIDs[0]
	a.CursorX, a.CursorY = id%GridSize, id/GridSize
	a.Update(key("f"))
	for i := 0; i < 120; i++ {
		tick(a, 1)
		tick(b, 1)
	}

	// Drawn, because reachSeat is rebuilt per frame and glyphAttr reads it.
	drawTo(a)
	drawTo(b)

	if seat, ok := a.reachSeat[grp.ID]; !ok || seat != mineSeat {
		t.Errorf("the refiner holding the cluster has it marked as seat %d (%v); "+
			"their own reach should read as theirs", seat, ok)
	}
	seat, ok := b.reachSeat[grp.ID]
	if !ok {
		t.Fatal("the watching refiner does not see the cluster as reached for at all")
	}
	if seat == mineSeat {
		t.Error("the watching refiner sees somebody else's reach as their own")
	}
	if want := a.Desk.Seat; seat != want {
		t.Errorf("the reach is marked seat %d, want %d, the seat of the refiner "+
			"actually holding it", seat, want)
	}

	// Otherwise the two could not be told apart.
	if got := b.Theme.DeskColour(seat); got == b.Theme.Plate {
		t.Errorf("another refiner's reach is drawn in %q, the same as your own "+
			"selection plate", got)
	}
}

// The grid rows are compared rune for rune. The chrome is left out: the header
// shows a name and the footer a roster, and those differ between refiners.
func TestTwoSessionsDrawTheSameBoard(t *testing.T) {
	a, b, _ := seatTwo(t)
	grp := wakeVisible(t, a)

	// Same cursor, since the cursor is allowed to differ.
	id := grp.NumberIDs[0]
	a.CursorX, a.CursorY = id%GridSize, id/GridSize
	b.CursorX, b.CursorY = a.CursorX, a.CursorY
	a.Update(key("f"))

	for i := 0; i < 180; i++ {
		tick(a, 1)
		tick(b, 1)
	}

	rowsA, rowsB := boardRows(a), boardRows(b)
	if len(rowsA) != len(rowsB) {
		t.Fatalf("the two boards are %d and %d rows", len(rowsA), len(rowsB))
	}
	for y := range rowsA {
		if rowsA[y] != rowsB[y] {
			t.Fatalf("row %d of the board differs between two refiners on it:\n"+
				"  holding: %q\n  watching: %q", y, rowsA[y], rowsB[y])
		}
	}
}

// boardRows is the grid region as drawn, without the chrome above and below it,
// and with the nameplates blanked. Each refiner sees the others named and
// itself as YOU, so the tags are blanked using m.plates, which records where
// each one went; nothing else can recover that from the buffer's runes.
func boardRows(m *Model) []string {
	buf := drawTo(m)
	l := m.Layout
	out := make([]string, 0, l.BoardH)
	for y := l.BoardY; y < l.BoardY+l.BoardH; y++ {
		row := []rune(buf.Row(y))
		for _, p := range m.plates {
			if p.Y != y {
				continue
			}
			for x := p.X; x < p.X+p.W && x < len(row); x++ {
				row[x] = ' '
			}
		}
		out = append(out, strings.TrimRight(string(row), " "))
	}
	return out
}

// Every session steps the shared board on its own frames, so Room.Step must not
// simply add each session's dt, or a room ages once per refiner: fuses burn
// early, credit comes due ahead of the digits and files rotate under everybody.
func TestTheBoardRunsAtOneRateHoweverManyAreWatching(t *testing.T) {
	const frames = 300

	alone := NewRoom(3)
	solo := JoinRoom(alone, 1)
	solo.Resize(120, 40)
	grp := wakeVisible(t, solo)
	for i := 0; i < frames; i++ {
		tick(solo, 1)
	}
	want := solo.Awake(grp.ID)
	if want <= 0 {
		t.Fatal("the cluster did not age at all with one refiner watching")
	}

	crowded := NewRoom(3)
	ms := make([]*Model, 0, MaxDesks)
	for i := 0; i < MaxDesks; i++ {
		m := JoinRoom(crowded, int64(i+1))
		if m == nil {
			t.Fatalf("could not seat refiner %d", i+1)
		}
		m.Resize(120, 40)
		ms = append(ms, m)
	}
	crowd := wakeVisible(t, ms[0])
	for i := 0; i < frames; i++ {
		for _, m := range ms {
			tick(m, 1)
		}
	}
	got := ms[0].Awake(crowd.ID)

	if diff := got - want; diff > 0.05 || diff < -0.05 {
		t.Errorf("after the same %d frames a cluster is %.2fs old with %d refiners "+
			"watching and %.2fs old with one, the board is running %.1fx too fast",
			frames, got, MaxDesks, want, got/want)
	}
}

// A blast stays on the board for blastLife so a session drawing at its own rate
// cannot miss one, but a fresh Ripple has seen nothing and must not replay the
// blasts still on the list.
func TestAJoinerDoesNotReplayOldBlasts(t *testing.T) {
	room := NewRoom(3)
	a := JoinRoom(room, 1)
	a.Resize(120, 40)

	room.mu.Lock()
	room.blastSeq++
	room.blasts = append(room.blasts, Blast{X: 10, Y: 10, Seq: room.blastSeq})
	room.mu.Unlock()

	b := JoinRoom(room, 2)
	b.Resize(120, 40)
	tick(b, 1)

	if b.ripple.Moving() {
		t.Error("a refiner who has just sat down is rippling from a detonation " +
			"that was over before they arrived")
	}
}
