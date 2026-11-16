package lumon

import (
	"testing"
	"time"
)

func TestAFifthRefinerIsTurnedAway(t *testing.T) {
	room := NewRoom(1)

	for i := 0; i < MaxDesks; i++ {
		if m := JoinRoom(room, int64(i)); m == nil {
			t.Fatalf("refiner %d could not take a desk", i+1)
		}
	}
	if m := JoinRoom(room, 99); m != nil {
		t.Errorf("a fifth refiner took a desk in a room of %d", MaxDesks)
	}
	if got := room.Seated(); got != MaxDesks {
		t.Errorf("%d seated, want %d", got, MaxDesks)
	}
}

func TestLeavingFreesADesk(t *testing.T) {
	room := NewRoom(1)
	var last *Model
	for i := 0; i < MaxDesks; i++ {
		last = JoinRoom(room, int64(i))
	}
	if JoinRoom(room, 99) != nil {
		t.Fatal("the room was not full")
	}

	last.Leave()

	if m := JoinRoom(room, 100); m == nil {
		t.Error("could not take the desk that was just given back")
	}
}

// A cluster wakes for each person, so nobody is left with nothing to hunt.
func TestDesksFollowThePeople(t *testing.T) {
	room := NewRoom(1)

	a := JoinRoom(room, 1)
	if got := room.DeskCount(); got != 1 {
		t.Errorf("one refiner, %d desks", got)
	}
	b := JoinRoom(room, 2)
	if got := room.DeskCount(); got != 2 {
		t.Errorf("two refiners, %d desks", got)
	}

	b.Leave()
	if got := room.DeskCount(); got != 1 {
		t.Errorf("one refiner left, %d desks", got)
	}
	a.Leave()
	// An empty room keeps a desk so it is not a degenerate board when rejoined.
	if got := room.DeskCount(); got != 1 {
		t.Errorf("empty room has %d desks, want 1", got)
	}
}

func TestArrivalsFillARoomBeforeOpeningAnother(t *testing.T) {
	rs := NewRooms(7)

	var rooms []*Room
	for i := 0; i < MaxDesks; i++ {
		r := rs.Assign()
		if JoinRoom(r, int64(i)) == nil {
			t.Fatalf("could not seat refiner %d", i+1)
		}
		rooms = append(rooms, r)
	}

	for i, r := range rooms {
		if r != rooms[0] {
			t.Errorf("refiner %d was sent to a different room", i+1)
		}
	}
	if got := rs.Count(); got != 1 {
		t.Errorf("%d rooms open for %d refiners, want 1", got, MaxDesks)
	}

	// The fifth opens a second room.
	fifth := rs.Assign()
	if fifth == rooms[0] {
		t.Error("a fifth refiner was sent to a full room")
	}
	if got := rs.Count(); got != 2 {
		t.Errorf("%d rooms open, want 2", got)
	}
}

// The name is the one handle the lobby gives anyone.
func TestRoomsTakeDifferentFiles(t *testing.T) {
	rs := NewRooms(3)

	seen := map[string]bool{}
	for i := 0; i < 12; i++ {
		name := rs.Open().FileName
		if seen[name] {
			t.Errorf("two rooms are both working %q", name)
		}
		seen[name] = true
	}
}

func TestTheListingReportsWhatTheLobbyNeeds(t *testing.T) {
	rs := NewRooms(5)
	room := rs.Assign()
	JoinRoom(room, 1)

	// Fill a bin so the percentage is not zero.
	room.mu.Lock()
	room.Grid.Bins[0].Refined = room.Grid.Bins[0].Quota
	room.mu.Unlock()

	list := rs.List()
	if len(list) != 1 {
		t.Fatalf("%d rooms listed, want 1", len(list))
	}
	l := list[0]
	if l.Name != room.FileName {
		t.Errorf("listing names %q, room is working %q", l.Name, room.FileName)
	}
	if l.Occupied != 1 {
		t.Errorf("listing says %d occupied, want 1", l.Occupied)
	}
	if l.Desks != MaxDesks {
		t.Errorf("listing says %d desks, want %d", l.Desks, MaxDesks)
	}
	if l.Percent <= 0 || l.Percent >= 100 {
		t.Errorf("listing says %d%%, want somewhere between", l.Percent)
	}
	if l.Full() {
		t.Error("a room with one refiner reports itself full")
	}
}

func TestTheListingLeadsWithTheBusiestRoom(t *testing.T) {
	rs := NewRooms(11)
	quiet := rs.Open()
	busy := rs.Open()
	JoinRoom(busy, 1)
	JoinRoom(busy, 2)
	JoinRoom(quiet, 3)

	list := rs.List()
	if len(list) != 2 {
		t.Fatalf("%d rooms listed", len(list))
	}
	if list[0].Name != busy.FileName {
		t.Errorf("lobby leads with %q, want the busier %q", list[0].Name, busy.FileName)
	}
}

func TestAnEmptiedRoomIsKeptAndRejoinable(t *testing.T) {
	rs := NewRooms(13)
	room := rs.Assign()
	m := JoinRoom(room, 1)

	room.mu.Lock()
	room.Grid.Bins[1].Refined = 3
	room.mu.Unlock()
	name := room.FileName

	m.Leave()

	if got := rs.Count(); got != 1 {
		t.Fatalf("%d rooms after the last refiner left, want it kept", got)
	}
	again := rs.Find(name)
	if again == nil {
		t.Fatal("the room cannot be found by its file name")
	}
	if got := again.Grid.Bins[1].Refined; got != 3 {
		t.Errorf("the room came back with %d clusters binned, want 3", got)
	}
}

// Otherwise the lobby silts up.
func TestALongEmptyRoomIsReclaimed(t *testing.T) {
	rs := NewRooms(17)
	now := time.Now()
	rs.now = func() time.Time { return now }

	room := rs.Assign()
	m := JoinRoom(room, 1)
	m.Leave()

	rs.Reclaim()
	if got := rs.Count(); got != 1 {
		t.Fatalf("a just-emptied room was reclaimed immediately")
	}

	now = now.Add(idleReclaim + time.Minute)
	rs.Reclaim()

	if got := rs.Count(); got != 0 {
		t.Errorf("%d rooms after sitting empty past the reclaim window, want 0", got)
	}
}

func TestAnOccupiedRoomIsNeverReclaimed(t *testing.T) {
	rs := NewRooms(19)
	now := time.Now()
	rs.now = func() time.Time { return now }

	room := rs.Assign()
	JoinRoom(room, 1)

	now = now.Add(100 * idleReclaim)
	rs.Reclaim()

	if got := rs.Count(); got != 1 {
		t.Errorf("an occupied room was reclaimed")
	}
}

// A choice of one file is no choice.
func TestAFloorHasSeveralFilesOnItBeforeAnybodyArrives(t *testing.T) {
	rs := NewRooms(7)
	if n := len(rs.List()); n != 0 {
		t.Fatalf("a fresh registry already has %d rooms", n)
	}

	rs.EnsureOpen(DefaultRooms)
	list := rs.List()
	if len(list) != DefaultRooms {
		t.Fatalf("the floor has %d files, want %d", len(list), DefaultRooms)
	}

	// Different files, since the name is the one handle the lobby gives anyone.
	seen := map[string]bool{}
	for _, l := range list {
		if seen[l.Name] {
			t.Errorf("two rooms are both called %q", l.Name)
		}
		seen[l.Name] = true
		if l.Occupied != 0 {
			t.Errorf("%q already has %d people in it", l.Name, l.Occupied)
		}
	}
}

func TestKeepingTheFloorOpenIsNotCumulative(t *testing.T) {
	rs := NewRooms(7)
	rs.EnsureOpen(3)
	rs.EnsureOpen(3)
	if n := len(rs.List()); n != 3 {
		t.Errorf("the floor has %d files after asking for three twice", n)
	}
	// It is a floor, not a ceiling: a fifth room may still be opened.
	rs.Open()
	if n := len(rs.List()); n != 4 {
		t.Errorf("opening one more left %d files", n)
	}
}

// Sweeping a long-idle file away is fine; leaving the next arrival an empty
// lobby is not.
func TestAQuietServerDoesNotDwindleToNothing(t *testing.T) {
	rs := NewRooms(7)
	at := time.Now()
	rs.now = func() time.Time { return at }
	rs.EnsureOpen(DefaultRooms)

	// Worked and then left, which starts a room's clock. A file nobody ever sat at
	// has no idle time to run out.
	before := map[string]bool{}
	for _, r := range rs.rooms {
		before[r.FileName] = true
		if d := r.Seat("someone"); d != nil {
			r.Unseat(d)
		}
	}

	// Long enough that every one of them is past reclaiming.
	at = at.Add(idleReclaim * 3)
	after := rs.List()
	if len(after) != DefaultRooms {
		t.Fatalf("after an idle afternoon the floor has %d files, want %d",
			len(after), DefaultRooms)
	}

	// They are new files: the floor is kept, but idle files are still let go.
	fresh := 0
	for _, l := range after {
		if !before[l.Name] {
			fresh++
		}
	}
	if fresh == 0 {
		t.Error("nothing was reclaimed; the same files are still standing")
	}
}

// The busiest room with a free desk is still the answer.
func TestSeveralFilesDoNotScatterTheRefiners(t *testing.T) {
	rs := NewRooms(7)
	rs.EnsureOpen(DefaultRooms)

	first := rs.Assign()
	first.Seat("a")
	for i := 0; i < MaxDesks-1; i++ {
		got := rs.Assign()
		if got != first {
			t.Fatalf("refiner %d was sent to a different file with room going spare", i+2)
		}
		got.Seat("b")
	}

	// Only once it is full does anybody go elsewhere.
	if rs.Assign() == first {
		t.Error("a fifth refiner was sent to a full room")
	}
}

// Every file is offered, whether or not anyone has opened it.
func TestTheDirectoryOffersTheWholeAlphabet(t *testing.T) {
	rs := NewRooms(1)
	dir := rs.Directory()

	if len(dir) != len(FileNames()) {
		t.Fatalf("the directory has %d entries against %d files", len(dir), len(FileNames()))
	}
	have := map[string]bool{}
	for _, e := range dir {
		have[e.Name] = true
		if e.Open {
			t.Errorf("%q is marked open on a registry with no rooms", e.Name)
		}
	}
	for _, name := range FileNames() {
		if !have[name] {
			t.Errorf("%q is a file name and not in the directory", name)
		}
	}

	// Alphabetical, because the letter tabs are how anybody finds a card in a
	// drawer of fifty-two.
	for i := 1; i < len(dir); i++ {
		if dir[i-1].Name > dir[i].Name {
			t.Fatalf("the directory runs %q before %q", dir[i-1].Name, dir[i].Name)
		}
	}
}

// A save from before the names were checked against the show may hold a
// file on a name no longer drawn; while it is open, the drawer must list it,
// or nobody could join the refiner on it.
func TestTheDirectoryListsEveryOpenRoom(t *testing.T) {
	const old = "Ouray"
	rs := restoreFloor(floorState{rooms: []fileState{{file: old, seed: 3, gridSeed: 3}}}, 7)

	for _, e := range rs.Directory() {
		if e.Name == old {
			if !e.Open {
				t.Errorf("%q is open and the directory says otherwise", old)
			}
			return
		}
	}
	t.Errorf("%q is open and missing from the directory entirely", old)
}

// Taking a card somebody else has already taken means joining them.
func TestOpeningANamedFileTwiceIsOneRoom(t *testing.T) {
	rs := NewRooms(3)
	a := rs.OpenNamed("Pacoima")
	b := rs.OpenNamed("Pacoima")

	if a != b {
		t.Error("opening the same file twice gave two different rooms")
	}
	if a.FileName != "Pacoima" {
		t.Errorf("the room is on %q, want Pacoima", a.FileName)
	}
	if n := len(rs.List()); n != 1 {
		t.Errorf("the registry holds %d rooms, want 1", n)
	}
}

// Otherwise a standing would stop at the files somebody picked by hand.
func TestANamedRoomSharesTheLedger(t *testing.T) {
	rs := NewRooms(5)
	if got := rs.OpenNamed("Minsk").ledger; got != rs.Ledger() {
		t.Error("a room opened by name keeps a ledger of its own")
	}
}
