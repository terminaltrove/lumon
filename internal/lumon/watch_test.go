package lumon

import (
	"strings"
	"testing"
	"time"
)

// fillARoom seats MaxDesks refiners at one board and returns them.
func fillARoom(t *testing.T) (*Room, []*Model) {
	t.Helper()
	room := NewRoom(5)
	ms := make([]*Model, 0, MaxDesks)
	for i := 0; i < MaxDesks; i++ {
		m := JoinRoom(room, int64(i+1))
		if m == nil {
			t.Fatalf("could not seat refiner %d", i+1)
		}
		m.Resize(120, 40)
		ms = append(ms, m)
	}
	return room, ms
}

func TestAFullBoardCanBeWatched(t *testing.T) {
	room, _ := fillARoom(t)

	fifth := newView(99)
	fifth.Name = "IRVING B."
	fifth.Resize(120, 40)

	if fifth.Join(room) {
		t.Fatal("a fifth refiner was seated at a full board")
	}
	if !fifth.Watch(room) {
		t.Fatal("a fifth refiner could not watch a full board")
	}

	if fifth.Room != room {
		t.Error("a watcher has no board")
	}
	if fifth.Desk != nil {
		t.Error("a watcher has a desk")
	}
	if fifth.Screen != ScreenMain {
		t.Errorf("a watcher is on screen %v, want the board", fifth.Screen)
	}

	// The board draws and the roster lists the four.
	tick(fifth, 30)
	if got := screenOf(fifth); !strings.Contains(got, room.FileName) {
		t.Error("a watcher is not shown the file they are watching")
	}
}

// refineAtCursor must gate on the desk as well as the room, or a session with no
// desk could consume a cluster, credit the bin and advance the file while Claim
// skips the attribution.
func TestAWatcherCannotRefine(t *testing.T) {
	room, ms := fillARoom(t)

	grp := activateFirstVisibleGroup(ms[0])
	if grp == nil {
		t.Skip("no bad group visible")
	}

	w := newView(99)
	w.Resize(120, 40)
	if !w.Watch(room) {
		t.Fatal("could not watch")
	}

	id := grp.NumberIDs[0]
	w.CursorX, w.CursorY = id%GridSize, id/GridSize
	before := room.Grid.Bins[0].Refined
	w.SelectedBin = 0
	w.Update(key("enter"))
	tick(w, 200)

	if room.Grid.Group(grp.ID) == nil {
		t.Error("a watcher refined a cluster off the board")
	}
	if room.Grid.Bins[0].Refined != before {
		t.Error("a watcher credited a bin")
	}
	if n := len(w.Flights.Active()); n != 0 {
		t.Errorf("a watcher launched %d digits of their own", n)
	}
}

func TestTheQueueHeadTakesTheNextDesk(t *testing.T) {
	room, ms := fillARoom(t)

	first, second := newView(101), newView(102)
	first.Name, second.Name = "IRVING B.", "BURT G."
	first.Resize(120, 40)
	second.Resize(120, 40)
	if !first.Watch(room) || !second.Watch(room) {
		t.Fatal("could not queue two watchers")
	}

	// Nobody is promoted while the board is full.
	tick(first, 5)
	tick(second, 5)
	if first.Desk != nil || second.Desk != nil {
		t.Fatal("somebody was seated at a board with no free desk")
	}

	ms[0].Leave()

	// The head of the queue takes it on its own next frame, and the one behind
	// stays where they are.
	tick(second, 2)
	if second.Desk != nil {
		t.Error("the second in the queue jumped the first")
	}
	tick(first, 2)
	if first.Desk == nil {
		t.Fatal("the head of the queue was not seated when a desk freed")
	}
	if first.watch != nil {
		t.Error("somebody who was seated is still in the queue")
	}
	if first.Desk.Name != "IRVING B." {
		t.Errorf("the new desk is named %q", first.Desk.Name)
	}

	if room.Waiting() != 1 {
		t.Errorf("%d people are still waiting, want 1", room.Waiting())
	}
}

func TestAQueuedDeskIsNotGivenAway(t *testing.T) {
	room, ms := fillARoom(t)

	w := newView(101)
	w.Resize(120, 40)
	if !w.Watch(room) {
		t.Fatal("could not watch")
	}
	ms[0].Leave()

	if d := room.Seat("PASSER BY"); d != nil {
		t.Error("somebody arriving from the lobby took a desk the queue was waiting for")
	}
	tick(w, 2)
	if w.Desk == nil {
		t.Error("the queue never got the desk it was holding")
	}
}

// A room of watchers is not empty. Reclaiming it would strand live connections
// on a board the registry has forgotten, where their promotion never comes.
func TestAWatchedRoomIsNotReclaimed(t *testing.T) {
	room, ms := fillARoom(t)
	w := newView(101)
	w.Resize(120, 40)
	if !w.Watch(room) {
		t.Fatal("could not watch")
	}

	for _, m := range ms {
		m.Leave()
	}
	if !room.emptyAt.IsZero() {
		t.Error("a room with somebody watching it started its idle clock")
	}

	w.Leave()
	if room.emptyAt.IsZero() {
		t.Error("a room nobody is at or waiting for never started its idle clock")
	}
	if time.Since(room.emptyAt) > time.Minute {
		t.Error("the idle clock started in the past")
	}
}

func TestTheQueueIsCapped(t *testing.T) {
	room, _ := fillARoom(t)
	for i := 0; i < MaxWatchers; i++ {
		if room.Wait("REFINER") == nil {
			t.Fatalf("could not queue watcher %d of %d", i+1, MaxWatchers)
		}
	}
	if room.Wait("ONE TOO MANY") != nil {
		t.Errorf("a %dth watcher was queued past the cap of %d", MaxWatchers+1, MaxWatchers)
	}
}

func TestAFullFileOffersAWatch(t *testing.T) {
	rs := NewRooms(7)
	room := rs.Open()
	for i := 0; i < MaxDesks; i++ {
		if JoinRoom(room, int64(i+1)) == nil {
			t.Fatalf("could not seat refiner %d", i+1)
		}
	}

	m := NewLocalSession(rs, 99, "IRVING B.")
	m.Resize(120, 40)

	m.tryJoin(room)
	if m.Room != nil {
		t.Fatal("the first press seated somebody at a full file")
	}
	if !strings.Contains(m.notice, "watch") {
		t.Errorf("the refusal does not offer a watch: %q", m.notice)
	}

	m.tryJoin(room)
	if m.Room == nil || m.Desk != nil {
		t.Fatal("the second press did not start watching")
	}
	if !strings.Contains(screenOf(m), "WATCHING") {
		t.Error("a watcher is not told they are watching")
	}
}

func TestAWatchOfferDoesNotFollowYouToAnotherFile(t *testing.T) {
	rs := NewRooms(11)
	full := rs.Open()
	for i := 0; i < MaxDesks; i++ {
		if JoinRoom(full, int64(i+1)) == nil {
			t.Fatalf("could not seat refiner %d", i+1)
		}
	}

	m := NewLocalSession(rs, 99, "IRVING B.")
	m.Resize(120, 40)
	m.tryJoin(full)

	other := rs.Open()
	m.tryJoin(other)
	if m.Desk == nil {
		t.Fatal("choosing an open file after a refusal did not seat anybody")
	}
	if m.watch != nil {
		t.Error("choosing an open file after a refusal made a watcher of them")
	}
}

// MaybeActivate counts down a shared timer, once per session, so a watcher
// calling it would hurry the next cluster along and steer which one it is by
// scrolling.
func TestAWatcherDoesNotWakeClusters(t *testing.T) {
	room, _ := fillARoom(t)
	w := newView(101)
	w.Resize(120, 40)
	if !w.Watch(room) {
		t.Fatal("could not watch")
	}

	before := room.activateIn
	tick(w, 10)
	if room.activateIn != before {
		t.Errorf("ten of a watcher's frames moved the wake countdown from %d to %d",
			before, room.activateIn)
	}
}
