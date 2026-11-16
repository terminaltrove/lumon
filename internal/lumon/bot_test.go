package lumon

import (
	"strings"
	"testing"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// withCompany is a player at a board with n computer refiners beside them.
func withCompany(t *testing.T, seed int64, n int) (*Room, *Model) {
	t.Helper()
	room := NewRoom(seed)
	m := JoinRoom(room, 1)
	if m == nil {
		t.Fatal("no desk")
	}
	m.Resize(120, 40)
	m.AllowBots(n)
	return room, m
}

func seconds(m *Model, s int) { tick(m, s*60) }

// bots is the computer refiners' desks, in seat order.
func bots(room *Room) []*Desk {
	var out []*Desk
	for _, d := range room.Roster() {
		if d.Bot {
			out = append(out, d)
		}
	}
	return out
}

func refinedSoFar(room *Room) int {
	n := len(room.creditIn)
	for _, b := range room.Grid.Bins {
		n += b.Refined
	}
	return n
}

// Computer refiners sit at the empty desks, under names of their own, and
// refine the file alongside the player.
func TestComputerRefinersWorkTheFile(t *testing.T) {
	room, m := withCompany(t, 11, 3)
	company := bots(room)
	if len(company) != 3 || room.Seated() != 4 {
		t.Fatalf("seated %d with %d computer refiners", room.Seated(), len(company))
	}
	names := map[string]bool{}
	for _, d := range company {
		if strings.HasPrefix(d.Name, "REFINER") || names[d.Name] {
			t.Errorf("computer refiner named %q", d.Name)
		}
		names[d.Name] = true
	}

	seconds(m, 90)
	if refinedSoFar(room) == 0 {
		t.Fatal("ninety seconds of company refined nothing")
	}
	worked := 0
	for _, d := range company {
		if d.Credit > 0 {
			worked++
		}
	}
	if worked == 0 {
		t.Fatal("no computer refiner was credited on the file")
	}
}

// A computer refiner counts what is still flying to a bin, so it never sends
// a bin more than its share.
func TestComputerRefinersNeverOverfillABin(t *testing.T) {
	room, m := withCompany(t, 11, 3)
	for i := range room.Grid.Bins {
		room.Grid.Bins[i].Refined = room.Grid.Bins[i].Quota
	}
	last := room.Grid.Bins[BinCount-1]
	last.Refined = last.Quota - 1

	for range 30 {
		seconds(m, 1)
		for i, b := range room.Grid.Bins {
			if b.Refined > b.Quota {
				t.Fatalf("bin %d took %d of %d", i+1, b.Refined, b.Quota)
			}
		}
	}
	if last.Refined != last.Quota {
		t.Fatalf("the last place in bin 5 was never filled: %d of %d", last.Refined, last.Quota)
	}
}

// The cluster the player has their hands on is theirs.
func TestComputerRefinersLeaveThePlayersClusterAlone(t *testing.T) {
	room, m := withCompany(t, 11, 3)
	grp := lockOntoACluster(t, m)
	seconds(m, 20)
	if !room.IsActive(grp.ID) {
		t.Fatal("a computer refiner took the cluster the player was holding")
	}
	if refinedSoFar(room) == 0 {
		t.Fatal("the computer refiners did nothing at all, so this proves nothing")
	}
}

// Watched alone, computer refiners finish a file and start the next, with
// none lost to a detonation: clusters wake only where somebody looks, so they
// have to go looking for the last ones.
func TestComputerRefinersFinishAFileOnTheirOwn(t *testing.T) {
	room, m := withCompany(t, 11, 0)
	total := len(room.Grid.Groups)
	m.Update(key("w"))
	for range 10 * 60 {
		if room.Epoch() > 0 {
			break
		}
		seconds(m, 1)
	}
	if room.Epoch() == 0 {
		t.Fatalf("ten minutes left the file at %d of %d", refinedSoFar(room), total)
	}
	if len(room.lost) != 0 {
		t.Fatalf("%d clusters detonated", len(room.lost))
	}
}

// The file's progress is theirs to make, but the scores are for people.
func TestComputerRefinersEarnNoScores(t *testing.T) {
	room, m := withCompany(t, 11, 3)
	seconds(m, 90)
	worked := 0
	for _, d := range bots(room) {
		worked += d.Credit
		if room.tally[refinerID(d.Name)] != 0 {
			t.Errorf("%s is on the file's tally", d.Name)
		}
		for _, row := range room.ledger.Standings() {
			if row.Name == refinerID(d.Name) {
				t.Errorf("%s is in the standings", d.Name)
			}
		}
	}
	if worked == 0 {
		t.Fatal("the computer refiners refined nothing, so this proves nothing")
	}
}

// When the last person leaves, so do the computer refiners, or the room could
// never be reclaimed.
func TestComputerRefinersGoHomeWithTheLastPerson(t *testing.T) {
	room, m := withCompany(t, 11, 3)
	m.Leave()
	if room.Seated() != 0 || len(room.bots) != 0 {
		t.Fatalf("seated %d, %d computer refiners left behind", room.Seated(), len(room.bots))
	}
	if room.emptyAt.IsZero() {
		t.Fatal("the empty room is not marked for reclaiming")
	}
}

// A person never waits for a desk a computer refiner is sitting at.
func TestAPersonAlwaysGetsADeskFromComputerRefiners(t *testing.T) {
	room, _ := withCompany(t, 11, 3)
	if room.Seated() != MaxDesks {
		t.Fatalf("seated %d", room.Seated())
	}
	if other := JoinRoom(room, 2); other == nil {
		t.Fatal("a person was refused a desk held by a computer refiner")
	}
	if n := len(bots(room)); n != 2 {
		t.Fatalf("%d computer refiners after one gave up a desk", n)
	}
}

// c brings company one at a time, and sends them all home once every desk is
// full.
func TestCBringsCompanyAndSendsItHome(t *testing.T) {
	room, m := withCompany(t, 11, 0)
	for want := 1; want <= MaxDesks-1; want++ {
		m.Update(key("c"))
		if n := len(bots(room)); n != want {
			t.Fatalf("after %d presses, %d computer refiners", want, n)
		}
	}
	m.Update(key("c"))
	if n := len(bots(room)); n != 0 || room.Seated() != 1 {
		t.Fatalf("a full board's c left %d computer refiners, %d seated", n, room.Seated())
	}
}

// w stands the player up to watch the computer refiners, who keep working the
// file with the camera following them, and w again sits the player back down.
func TestWatchingTheComputerRefinersWork(t *testing.T) {
	room, m := withCompany(t, 11, 0)
	m.Update(key("w"))
	if m.Desk != nil || !m.spectating {
		t.Fatal("w did not stand the player up")
	}
	if n := len(bots(room)); n != MaxDesks {
		t.Fatalf("watching an empty board brought %d computer refiners", n)
	}
	seconds(m, 60)
	if refinedSoFar(room) == 0 {
		t.Fatal("a minute of watching saw nothing refined")
	}
	following := false
	for _, d := range bots(room) {
		if d.CursorX == m.CursorX && d.CursorY == m.CursorY {
			following = true
		}
	}
	if !following {
		t.Error("the camera is not on any computer refiner")
	}

	m.Update(key("w"))
	if m.Desk == nil || m.spectating {
		t.Fatal("w did not sit the player back down")
	}
	if room.Seated() != MaxDesks {
		t.Fatalf("seated %d after sitting back down", room.Seated())
	}
}

// The same board and the same company play out the same way.
func TestComputerRefinersPlayTheSameGameEveryTime(t *testing.T) {
	run := func() []Desk {
		room, m := withCompany(t, 11, 3)
		seconds(m, 45)
		var out []Desk
		for _, d := range bots(room) {
			out = append(out, Desk{Name: d.Name, CursorX: d.CursorX, CursorY: d.CursorY, Credit: d.Credit})
		}
		return out
	}
	a, b := run(), run()
	if len(a) != 3 || a[0].Credit == 0 {
		t.Fatalf("nothing to compare: %+v", a)
	}
	for i := range a {
		if a[i].Name != b[i].Name || a[i].CursorX != b[i].CursorX || a[i].CursorY != b[i].CursorY || a[i].Credit != b[i].Credit {
			t.Fatalf("two runs differ: %+v and %+v", a[i], b[i])
		}
	}
}

// Where computer refiners are not allowed (remotely), c and w do nothing and
// the controls do not mention them.
func TestNoCompanyWhereItIsNotAllowed(t *testing.T) {
	room := NewRoom(11)
	m := JoinRoom(room, 1)
	m.Resize(120, 40)
	m.Update(key("c"))
	m.Update(key("w"))
	if len(bots(room)) != 0 || m.Desk == nil {
		t.Fatal("c or w worked where computer refiners are not allowed")
	}
	for _, g := range m.bindings() {
		for _, b := range g.Bindings {
			if b.Keys == "c" || b.Keys == "w" {
				t.Errorf("the controls list %q", b.Keys)
			}
		}
	}
	if strings.Contains(m.hintLine(200), "company") {
		t.Error("the footer offers company")
	}
}

// In the files, c says how many computer refiners will come along, and they
// do.
func TestCompanyChosenInTheFilesComesAlong(t *testing.T) {
	rs := NewRooms(7)
	rs.EnsureOpen(2)
	m := NewLocalSession(rs, 1, "mark")
	m.AllowBots(0)
	m.Resize(120, 40)
	m.Update(key("c"))
	m.Update(key("c"))
	if !strings.Contains(m.notice, "2 refiners will join you") {
		t.Fatalf("notice %q", m.notice)
	}
	m.Update(key("enter"))
	if m.Room == nil {
		t.Fatal("enter did not join a file")
	}
	if n := len(bots(m.Room)); n != 2 {
		t.Fatalf("%d computer refiners came along", n)
	}
}

// The camera keeps the refiner it follows near the middle of the view, and
// glides there a cell at a time instead of jumping.
func TestTheCameraGlidesToKeepTheRefinerCentred(t *testing.T) {
	room, m := withCompany(t, 11, 0)
	m.Update(key("w"))
	seconds(m, 3) // to settle on someone
	far := 0
	for i := range 60 * 60 {
		x, y := m.Layout.ViewportX, m.Layout.ViewportY
		tick(m, 1)
		if dx, dy := abs(m.Layout.ViewportX-x), abs(m.Layout.ViewportY-y); dx > 1 || dy > 1 {
			t.Fatalf("frame %d: the view jumped %d,%d cells", i, dx, dy)
		}
		d := room.desk[m.followSeat]
		if d == nil || !d.Bot {
			continue
		}
		l := m.Layout
		if cx, cy := l.ViewportX+l.ViewportW/2, l.ViewportY+l.ViewportH/2; abs(d.CursorX-cx) > l.ViewportW/3+2 || abs(d.CursorY-cy) > l.ViewportH/3+2 {
			far++
		}
	}
	// A refiner who moves on is caught up with over a few frames, so allow a
	// little time off centre, never most of it.
	if far > 60*60/5 {
		t.Fatalf("the refiner was off centre for %d of %d frames", far, 60*60)
	}
}

// A watcher can tell whose work the camera is showing: that refiner's plate in
// the roster is drawn inverted, a box in their colour.
func TestTheWatchedRefinersPlateIsInverted(t *testing.T) {
	room, m := withCompany(t, 11, 0)
	m.Update(key("w"))
	seconds(m, 3)
	watched := room.desk[m.followSeat]
	if watched == nil || !watched.Bot {
		t.Fatal("the camera is on nobody")
	}
	buf := canvas.NewBuffer(120, 40)
	m.Draw(buf)
	y := m.Layout.FooterY()
	for _, d := range bots(room) {
		row := strings.Split(buf.String(), "\n")[y]
		x := strings.Index(row, d.Name)
		if x >= 0 {
			x = len([]rune(row[:x]))
		}
		if x < 0 {
			t.Fatalf("%s is not in the roster", d.Name)
		}
		got := buf.AttrAt(x, y)
		inverted := got.BG == m.deskColour(d) && got.FG == m.Theme.Ink
		if d == watched && !inverted {
			t.Errorf("the watched refiner %s is drawn in %+v", d.Name, got)
		}
		if d != watched && inverted {
			t.Errorf("%s is drawn as watched", d.Name)
		}
		// The box is a cell wider than the name on each side.
		if d == watched && buf.AttrAt(x-1, y) != got {
			t.Errorf("the box around %s has no margin", d.Name)
		}
	}
}

// A board seats different computer refiners from one file to the next, not
// the same three every time.
func TestComputerRefinersComeFromALargerPool(t *testing.T) {
	seen := map[string]bool{}
	for seed := int64(1); seed <= 20; seed++ {
		room, _ := withCompany(t, seed, 3)
		names := map[string]bool{}
		for _, d := range bots(room) {
			if names[d.Name] {
				t.Fatalf("seed %d seats two refiners called %s", seed, d.Name)
			}
			names[d.Name], seen[d.Name] = true, true
		}
	}
	if len(seen) < 12 {
		t.Fatalf("twenty boards seated only %d different names", len(seen))
	}
}

// Every phrase fits the phrase menu and a narrow board.
func TestEveryPhraseFitsTheMenu(t *testing.T) {
	if len(callouts) < 16 {
		t.Errorf("only %d phrases", len(callouts))
	}
	seen := map[string]bool{}
	for _, c := range callouts {
		if n := len([]rune(c)); n > 30 {
			t.Errorf("%q is %d characters", c, n)
		}
		if seen[c] {
			t.Errorf("%q twice", c)
		}
		seen[c] = true
	}
}
