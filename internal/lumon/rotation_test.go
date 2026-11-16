package lumon

import (
	"github.com/terminaltrove/lumon/internal/noise"
	"strings"
	"testing"
)

func corruptCells(g *Grid) map[int]bool {
	out := map[int]bool{}
	for i := range g.Cells {
		if g.Cells[i].BadGroupID >= 0 {
			out[i] = true
		}
	}
	return out
}

// Corruption comes from one globally seeded noise field, so the file has to vary
// it, or a room working through several files would re-refine a layout it had
// memorised.
func TestEachFileIsADifferentBoard(t *testing.T) {
	first := corruptCells(NewGridAt(1, 0))
	if len(first) == 0 {
		t.Fatal("the first board has no corrupt cells at all")
	}

	for epoch := 1; epoch < 5; epoch++ {
		next := corruptCells(NewGridAt(1, epoch))

		shared := 0
		for id := range next {
			if first[id] {
				shared++
			}
		}
		// Independent samples of one field overlap somewhat by chance; the layouts must
		// not be the same.
		overlap := float64(shared) / float64(len(first))
		if overlap > 0.5 {
			t.Errorf("file %d shares %.0f%% of its corrupt cells with the first, want a board of its own",
				epoch, overlap*100)
		}
	}
}

// Which cells are corrupt is fixed by the C++ reference noise, so only the file
// may change it.
func TestTheSeedStillDoesNotMoveTheCorruption(t *testing.T) {
	a := corruptCells(NewGridAt(1, 0))
	b := corruptCells(NewGridAt(9999, 0))

	if len(a) != len(b) {
		t.Fatalf("%d corrupt cells against %d for a different seed", len(a), len(b))
	}
	for id := range a {
		if !b[id] {
			t.Fatalf("cell %d is corrupt for one seed and not another", id)
		}
	}
}

// The golden values describe the first file.
func TestTheFirstFileIsTheCanonicalBoard(t *testing.T) {
	for _, c := range [][2]int{{0, 0}, {13, 7}, {50, 50}, {99, 99}} {
		if noise.IsBadNumber(c[0], c[1]) != noise.IsBadNumberAt(c[0], c[1], 0) {
			t.Errorf("cell (%d,%d) disagrees between noise.IsBadNumber and file 0", c[0], c[1])
		}
	}
}

// completeTheFile fills every bin, as refining every cluster would.
func completeTheFile(m *Model) {
	m.Room.mu.Lock()
	for i := range m.Grid.Bins {
		m.Grid.Bins[i].Refined = m.Grid.Bins[i].Quota
	}
	m.Room.mu.Unlock()
}

// openNotice ticks until the completion notice has finished opening. It grows
// out of the board on a spring, so the frame after completion shows an empty box
// two rows tall.
func openNotice(m *Model) {
	for i := 0; i < 120; i++ {
		if m.noticeOpen != nil && m.noticeOpen.Value() >= noticeShows {
			return
		}
		tick(m, 1)
	}
}

func TestAFinishedFileIsHeldAndThenReplaced(t *testing.T) {
	m := ready()
	first := m.FileName
	completeTheFile(m)
	tick(m, 1)

	if !m.Room.Complete() {
		t.Fatal("a full board is not reported as a finished file")
	}
	if m.Screen != ScreenMain {
		t.Errorf("screen is %v; the notice is drawn from the main screen", m.Screen)
	}

	openNotice(m)
	buf := drawTo(m)
	var screen string
	_, h := buf.Size()
	for y := 0; y < h; y++ {
		screen += buf.Row(y)
	}
	if !strings.Contains(screen, "COMPLETE") {
		t.Errorf("the finished file is not announced:\n%s", screen)
	}

	// Held, then replaced.
	tick(m, int(FileHoldSeconds*60)+10)

	if m.Room.Complete() {
		t.Error("the room is still holding the finished file")
	}
	if m.FileName == first {
		t.Error("the room took the same file again")
	}
	if got := m.Room.Epoch(); got != 1 {
		t.Errorf("the room is on epoch %d, want 1", got)
	}
	if got := m.Room.History; len(got) != 1 || got[0] != first {
		t.Errorf("history is %v, want just %q", got, first)
	}
}

func TestTheNextFileStartsFresh(t *testing.T) {
	m := ready()
	before := corruptCells(m.Grid)
	completeTheFile(m)
	tick(m, int(FileHoldSeconds*60)+10)

	if got := m.File.Percent(m.Grid); got != 0 {
		t.Errorf("the new file is already %d%% done", got)
	}
	for i := range m.Grid.Bins {
		if got := m.Grid.Bins[i].Refined; got != 0 {
			t.Errorf("bin %d starts the new file with %d in it", i+1, got)
		}
	}

	after := corruptCells(m.Grid)
	shared := 0
	for id := range after {
		if before[id] {
			shared++
		}
	}
	if overlap := float64(shared) / float64(len(before)); overlap > 0.5 {
		t.Errorf("the new file shares %.0f%% of the old board's corrupt cells", overlap*100)
	}
}

// A view drops what it held of the finished file, so no leftovers are painted on
// the new one.
func TestAViewFollowsTheRoomOntoTheNextFile(t *testing.T) {
	m := ready()
	grownCluster(t, m)
	m.Update(key("enter")) // digits in the air

	if len(m.Flights.Active()) == 0 {
		t.Skip("nothing took off")
	}

	completeTheFile(m)
	tick(m, int(FileHoldSeconds*60)+10)

	if got := len(m.Flights.Active()); got != 0 {
		t.Errorf("%d digits are still flying to bins on a file that is gone", got)
	}
	if got := m.File.Name; got != m.Room.FileName {
		t.Errorf("the terminal shows %q, the room is working %q", got, m.Room.FileName)
	}
	if got := m.File.Fill(); got > 0.01 {
		t.Errorf("the bar starts the new file at %v, want it easing up from nothing", got)
	}
}

func TestAFinishedFileTakesNoMoreWork(t *testing.T) {
	m := ready()
	grp := grownCluster(t, m)
	completeTheFile(m)
	tick(m, 1)

	id := grp.NumberIDs[0]
	m.CursorX, m.CursorY = id%GridSize, id/GridSize
	m.Update(key("enter"))

	if got := len(m.Flights.Active()); got != 0 {
		t.Errorf("%d digits launched from a finished file", got)
	}
}

func TestARoomTurnsThePageTogether(t *testing.T) {
	room := NewRoom(3)
	a, b := JoinRoom(room, 1), JoinRoom(room, 2)
	a.Resize(120, 40)
	b.Resize(120, 40)

	completeTheFile(a)
	for i := 0; i < int(FileHoldSeconds*60)+10; i++ {
		tick(a, 1)
		tick(b, 1)
	}

	if a.FileName != b.FileName {
		t.Errorf("the two refiners are on different files: %q and %q", a.FileName, b.FileName)
	}
	if a.seenEpoch != b.seenEpoch {
		t.Errorf("the two refiners are on different epochs: %d and %d", a.seenEpoch, b.seenEpoch)
	}
	if got := len(room.History); got != 1 {
		t.Errorf("the room filed %d completed files, want 1", got)
	}
}

// onFloorHolding is a refiner seated at a file on a floor that holds finished
// files for seconds.
func onFloorHolding(t *testing.T, seconds float64) *Model {
	t.Helper()
	rs := NewRooms(1)
	rs.HoldFinishedFiles(seconds)
	m := JoinRoom(rs.Open(), 1)
	if m == nil {
		t.Fatal("no desk")
	}
	m.Resize(120, 40)
	tick(m, 1)
	return m
}

// A floor can hold its finished files longer, as the browser does for the
// length of its music, and the notice counts down from that.
func TestAFloorHoldsAFinishedFileForAsLongAsItSays(t *testing.T) {
	m := onFloorHolding(t, 30)
	epoch := m.Room.Epoch()
	completeTheFile(m)
	openNotice(m)
	if got := screenOf(m); !strings.Contains(got, "next file in 30") && !strings.Contains(got, "next file in 29") {
		t.Errorf("the notice does not count down from 30:\n%s", got)
	}
	tick(m, 60*int(FileHoldSeconds+2))
	if m.Room.Epoch() != epoch {
		t.Fatal("the file moved on at the usual hold, not the floor's")
	}
	tick(m, 60*20)
	if m.Room.Epoch() == epoch {
		t.Error("the file was never moved on")
	}
	if NewRoom(1).HoldSeconds() != FileHoldSeconds {
		t.Error("a room on its own no longer holds for FileHoldSeconds")
	}
}

// The music for a finished file starts, quietly, as it finishes, and is
// hushed if the refiner leaves before it ends.
func TestAFinishedFilePlaysItsMusicUntilItIsLeft(t *testing.T) {
	m := onFloorHolding(t, 30)
	completeTheFile(m)
	cues := heard(m, 60)
	if v := volumeOf(cues, SoundFinale); v <= 0 || v > 0.6 {
		t.Fatalf("the music was cued at %v", v)
	}
	if volumeOf(heard(m, 60), SoundFinale) >= 0 {
		t.Error("the music was cued twice")
	}
	m.Leave()
	if volumeOf(heard(m, 2), SoundHush) < 0 {
		t.Error("leaving the finished file left its music playing")
	}
}
