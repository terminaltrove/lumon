package lumon

import (
	"strings"
	"testing"
	"time"
)

func TestFloorNewsReachesDifferentFiles(t *testing.T) {
	rs := NewRooms(11)
	a := NewLocalSession(rs, 1, "mark")
	b := NewLocalSession(rs, 2, "helly")
	a.Resize(120, 40)
	b.Resize(120, 40)
	if !a.Join(rs.Open()) || !b.Join(rs.Open()) || a.Room == b.Room {
		t.Fatal("two refiners did not reach different files")
	}
	line := strings.ToUpper("HELLY joined " + b.Room.FileName)
	if !strings.Contains(a.View(), line) || !strings.Contains(b.View(), line) {
		t.Errorf("the arrival is not visible across both files: %q", line)
	}
}

func TestTheFloorReportsFileAndQuarterHonours(t *testing.T) {
	rs := NewRooms(1)
	now := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)
	rs.ledger.now = func() time.Time { return now }
	rs.ledger.Credit("mark", Woe)
	now = time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)
	rs.ledger.turn()
	if line := rs.news.Line(time.Now()); !strings.Contains(line, "EMPLOYEE OF THE QUARTER · MARK · 2026-Q1") {
		t.Errorf("quarter winner absent from floor news: %q", line)
	}
	rs.ledger.CloseFile("COLD HARBOR", map[string]int{"MARK": 3})
	if line := rs.news.Line(time.Now()); !strings.Contains(line, "FILE COLD HARBOR COMPLETE · TOP REFINER MARK") {
		t.Errorf("file completion absent from floor news: %q", line)
	}
}

func TestAChosenPhraseAppearsAboveTheRefinersCursorForCompany(t *testing.T) {
	rs := NewRooms(1)
	room := rs.Open()
	a := NewLocalSession(rs, 1, "mark")
	b := NewLocalSession(rs, 2, "helly")
	a.Resize(120, 40)
	b.Resize(120, 40)
	if !a.Join(room) || !b.Join(room) {
		t.Fatal("the refiners did not reach the same file")
	}
	a.Update(key("b"))
	if !a.calloutOpen || !strings.Contains(a.View(), "LUMON PHRASES") {
		t.Fatal("b did not open the phrase menu")
	}
	a.Update(key("down"))
	a.Update(key("enter"))
	if !strings.Contains(b.View(), "WELLNESS CHECK") {
		t.Error("the other refiner cannot see the phrase")
	}
	tick(a, 400)
	if strings.Contains(b.View(), "WELLNESS CHECK") {
		t.Error("the phrase did not expire on the shared board clock")
	}
}

func TestPhraseMenuKeepsTheSelectedLineOnAShortTerminal(t *testing.T) {
	rs := NewRooms(1)
	m := NewLocalSession(rs, 1, "mark")
	m.Resize(40, 10)
	if !m.Join(rs.Open()) {
		t.Fatal("the refiner did not join")
	}
	m.Update(key("b"))
	for i := 0; i < len(callouts)-1; i++ {
		m.Update(key("down"))
	}
	if !strings.Contains(m.View(), callouts[len(callouts)-1]) {
		t.Error("the selected phrase fell off the short menu")
	}
}

func TestTheNewsIsInTheRuleOverTheGrid(t *testing.T) {
	rs := NewRooms(1)
	m := NewLocalSession(rs, 1, "mark")
	m.Resize(120, 40)
	if !m.Join(rs.Open()) {
		t.Fatal("no desk")
	}
	tick(m, 300) // the board has finished arriving, so the colours are its own
	buf := drawTo(m)
	row := buf.Row(m.Layout.GridTop - 2)
	if !strings.Contains(row, "MARK JOINED") {
		t.Fatalf("the rule is %q", row)
	}
	for y := range m.Layout.Height {
		if y != m.Layout.GridTop-2 && strings.Contains(buf.Row(y), "MARK JOINED") {
			t.Errorf("the news is on row %d too", y)
		}
	}
	if top := buf.Row(0); strings.Contains(top, "FLOOR NEWS") || !strings.HasPrefix(top, "┌──") {
		t.Errorf("the file bar's top edge is %q", top)
	}

	// Centred, with the frame either side of it.
	line := strings.TrimRight(row, " ")
	left := strings.Index(line, " "+rs.news.Line(time.Now()))
	if left < 0 {
		t.Fatalf("no news in %q", line)
	}
	x := len([]rune(line[:left])) + 1
	width := len([]rune(rs.news.Line(time.Now())))
	if want := (120 - width) / 2; x != want {
		t.Errorf("the news starts at %d, want %d", x, want)
	}
	// White on the page, not a bar: only a LAN request is bright. The rule
	// gives way to it from edge to edge.
	for _, at := range []int{0, x, 119} {
		if got := buf.AttrAt(at, m.Layout.GridTop-2); got != m.newsAttr() || got.BG.Set() {
			t.Errorf("column %d of the news row is %+v", at, got)
		}
	}
	if strings.Contains(row, "═") {
		t.Errorf("the rule shows through the news: %q", row)
	}
}

func TestNHidesTheNewsAndHStillMovesLeft(t *testing.T) {
	rs := NewRooms(1)
	m := NewLocalSession(rs, 1, "mark")
	m.Resize(120, 40)
	if !m.Join(rs.Open()) {
		t.Fatal("no desk")
	}
	m.Update(key("n"))
	if got := drawTo(m).Row(m.Layout.GridTop - 2); strings.Contains(got, "MARK JOINED") {
		t.Errorf("n did not hide the news: %q", got)
	}
	m.Update(key("n"))
	if got := drawTo(m).Row(m.Layout.GridTop - 2); !strings.Contains(got, "MARK JOINED") {
		t.Errorf("n did not bring the news back: %q", got)
	}

	m.Update(key("l"))
	x := m.CursorX
	m.Update(key("h"))
	if m.CursorX != x-1 {
		t.Errorf("h moved the cursor from %d to %d", x, m.CursorX)
	}
	if m.newsHidden {
		t.Error("h hid the news")
	}
}

// The news comes and goes. An event shows for 30 s, a notice for 30 s in every
// two minutes, and the rest of the time there is nothing, so the rule shows.
func TestTheNewsComesAndGoes(t *testing.T) {
	var n floorNews
	start := time.Unix(noticeEvery*1000, 0) // the start of a notice's turn
	first := n.Line(start)
	if !strings.HasPrefix(first, "LUMON NOTICE · ") {
		t.Fatalf("no notice at the start of its turn: %q", first)
	}
	if got := n.Line(start.Add(29 * time.Second)); got != first {
		t.Errorf("the notice changed within its 30 s: %q to %q", first, got)
	}
	if got := n.Line(start.Add(30 * time.Second)); got != "" {
		t.Errorf("the notice outstayed 30 s: %q", got)
	}
	if got := n.Line(start.Add(119 * time.Second)); got != "" {
		t.Errorf("something showed between notices: %q", got)
	}
	if got := n.Line(start.Add(120 * time.Second)); got == "" || got == first {
		t.Errorf("the next notice is %q after %q", got, first)
	}

	n.Post("HELLY joined Narva")
	if got := n.Line(time.Now().Add(29 * time.Second)); got != "FLOOR NEWS · HELLY JOINED NARVA" {
		t.Errorf("the event went within 29 s: %q", got)
	}
	if got := n.Line(time.Now().Add(31 * time.Second)); strings.Contains(got, "HELLY") {
		t.Errorf("the event outstayed 30 s: %q", got)
	}
}

// Arriving on the floor is news, so it shows when you sit down, and goes.
func TestTheNewsShowsOnArrivalThenGoes(t *testing.T) {
	rs := NewRooms(1)
	m := NewLocalSession(rs, 1, "mark")
	m.Resize(120, 40)
	if !m.Join(rs.Open()) {
		t.Fatal("no desk")
	}
	if got := drawTo(m).Row(m.Layout.GridTop - 2); !strings.Contains(got, "MARK JOINED") {
		t.Errorf("no news on arrival: %q", got)
	}
	if got := rs.news.Line(time.Now().Add(newsHold)); strings.Contains(got, "MARK") {
		t.Errorf("the arrival is still news after %v: %q", newsHold, got)
	}
}

// The news is in capitals, as the rest of the Lumon chrome is.
func TestTheNewsIsInCapitals(t *testing.T) {
	for _, s := range standingNotices {
		if s != strings.ToUpper(s) {
			t.Errorf("%q is not in capitals", s)
		}
	}
	var n floorNews
	n.PostArrival("joe", "Siena", 1)
	if got := n.Line(time.Now()); got != "FLOOR NEWS · JOE JOINED SIENA" {
		t.Errorf("an arrival reads %q", got)
	}
}

// The news is in the rules' colour, and somebody arriving is named in the colour
// of their seat, as on their nameplate.
func TestAnArrivalIsNamedInTheirSeatsColour(t *testing.T) {
	rs := NewRooms(1)
	room := rs.Open()
	a := NewLocalSession(rs, 1, "mark")
	b := NewLocalSession(rs, 2, "helly")
	a.Resize(120, 40)
	b.Resize(120, 40)
	if !a.Join(room) || !b.Join(room) {
		t.Fatal("no desks")
	}
	tick(a, 300)
	buf := drawTo(a)
	y := a.Layout.GridTop - 2
	x, _ := findText(t, buf, "HELLY")
	if got := buf.AttrAt(x, y).FG; got != a.Theme.DeskColour(b.Desk.Seat) {
		t.Errorf("HELLY is drawn in %v, not their seat's %v", got, a.Theme.DeskColour(b.Desk.Seat))
	}
	if got := buf.AttrAt(x+len("HELLY"), y).FG; got != a.Theme.Chrome.FG {
		t.Errorf("the rest of the line is in %v", got)
	}
	fx, _ := findText(t, buf, "FLOOR NEWS")
	if got := buf.AttrAt(fx, y).FG; got != a.Theme.Chrome.FG {
		t.Errorf("FLOOR NEWS is in %v, not the rules' %v", got, a.Theme.Chrome.FG)
	}
}

// A file named like the refiner does not get coloured by mistake.
func TestOnlyTheArrivalsNameIsColoured(t *testing.T) {
	rs := NewRooms(1)
	m := NewLocalSession(rs, 1, "mark")
	m.Resize(120, 40)
	if !m.Join(rs.Open()) {
		t.Fatal("no desk")
	}
	tick(m, 300)
	rs.news.Post("FILE MARK COMPLETE")
	buf := drawTo(m)
	x, y := findText(t, buf, "MARK")
	if got := buf.AttrAt(x, y).FG; got != m.Theme.Chrome.FG {
		t.Errorf("news that is not an arrival coloured a name: %v", got)
	}
}
