package lumon

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func screenOf(m *Model) string {
	buf := drawTo(m)
	var s strings.Builder
	_, h := buf.Size()
	for y := 0; y < h; y++ {
		s.WriteString(buf.Row(y))
		s.WriteByte('\n')
	}
	return s.String()
}

// refineOne wakes a cluster and refines it, reporting whether it managed to.
func refineOne(t *testing.T, m *Model) bool {
	t.Helper()
	grp := m.FirstActive()
	if grp == nil {
		for i := 0; i < 60*20 && m.FirstActive() == nil; i++ {
			tick(m, 1)
		}
		grp = m.FirstActive()
	}
	if grp == nil {
		return false
	}
	id := grp.NumberIDs[0]
	m.CursorX, m.CursorY = id%GridSize, id/GridSize
	m.Update(key("enter"))
	tick(m, 2)
	return true
}

// Room.Seat opens a fresh Desk on every arrival, so the tally has to be kept in
// the ledger.
func TestATallySurvivesTheFileItWasEarnedOn(t *testing.T) {
	m := ready()
	m.Name = "MARK S."
	m.Desk.Name = m.Name

	if !refineOne(t, m) {
		t.Skip("nothing woke up")
	}
	before, ok := m.Ledger().Of("MARK S.")
	if !ok || before.Refined == 0 {
		t.Fatal("refining credited nothing to the ledger")
	}

	completeTheFile(m)
	tick(m, int(FileHoldSeconds*60)+10)

	after, ok := m.Ledger().Of("MARK S.")
	if !ok {
		t.Fatal("the refiner is gone from the ledger after the file rotated")
	}
	if after.Refined < before.Refined {
		t.Errorf("the tally fell from %d to %d when the file rotated", before.Refined, after.Refined)
	}
	if after.Files == 0 {
		t.Error("finishing a file credited nobody with having worked it")
	}
}

// Across rooms, through the registry's shared ledger.
func TestAStandingFollowsARefinerToAnotherFile(t *testing.T) {
	rs := NewRooms(21)
	rs.Open()
	rs.Open()

	m := named(NewSession(rs, 1), "HELLY R.")
	m.Resize(120, 40)
	m.Update(key("enter")) // lobby
	m.Update(key("enter")) // join the first
	if m.Room == nil {
		t.Fatal("did not join a file")
	}
	if !refineOne(t, m) {
		t.Skip("nothing woke up")
	}
	was, _ := m.Ledger().Of("HELLY R.")
	if was.Refined == 0 {
		t.Fatal("nothing was credited")
	}

	// Stand up and sit at the other file from outside Update, as a reconnect does.
	// Going through the lobby would deadlock: Update holds the board's write lock
	// for its call and Join takes it again through Leave/Unseat. Production never
	// takes that path, because a desk has no way back to the lobby.
	other := rs.Find(rs.List()[1].Name)
	if other == nil {
		t.Fatal("the second file is not in the listing")
	}
	m.Leave()
	if !m.Join(other) {
		t.Fatal("did not join the second file")
	}

	now, ok := m.Ledger().Of("HELLY R.")
	if !ok || now.Refined < was.Refined {
		t.Errorf("moving files lost the tally: %d became %d", was.Refined, now.Refined)
	}
}

func TestASoloRefinerIsStillNamedForTheirFile(t *testing.T) {
	m := ready()
	m.Name = "IRVING B."
	m.Desk.Name = m.Name
	if !refineOne(t, m) {
		t.Skip("nothing woke up")
	}

	rank := m.Room.FileStandings()
	if len(rank) == 0 || rank[0].Name != "IRVING B." {
		t.Fatalf("this file's standings are %v, want the only refiner at the top", rank)
	}

	completeTheFile(m)
	openNotice(m)
	if got := screenOf(m); !strings.Contains(got, "TOP REFINER") ||
		!strings.Contains(got, "IRVING B.") {
		t.Errorf("the finished file does not name its refiner:\n%s", got)
	}
	// "Employee of the Quarter" belongs to the quarter, not a file.
	if got := screenOf(m); strings.Contains(got, "Employee of the quarter") {
		t.Errorf("a finished file still crowns an Employee of the Quarter:\n%s", got)
	}
}

// Most of that file, not of all time.
func TestAFileIsToppedByWhoeverDidTheMostOfIt(t *testing.T) {
	l := NewLedger()
	l.Refined("MARK S.", 100) // a long-standing lead
	l.CloseFile("Molde", map[string]int{"HELLY R.": 9, "MARK S.": 2})

	helly, _ := l.Of("HELLY R.")
	mark, _ := l.Of("MARK S.")
	if helly.Topped != 1 {
		t.Errorf("the file's top refiner has topped %d, want 1", helly.Topped)
	}
	if mark.Topped != 0 {
		t.Errorf("the all-time leader was given a file they barely worked")
	}
	// Both worked it, so both get the file.
	if helly.Files != 1 || mark.Files != 1 {
		t.Errorf("files worked: helly %d, mark %d; want 1 each", helly.Files, mark.Files)
	}
}

// Two refiners inside the grace window are both credited, so a file's
// attribution can sum past the clusters it held.
func TestBothCoRefinersAppearInTheAttribution(t *testing.T) {
	_, a, b := twoInARoom(t)
	a.Name, b.Name = "MARK S.", "HELLY R."
	a.Desk.Name, b.Desk.Name = a.Name, b.Name

	grp := a.FirstActive()
	if grp == nil {
		t.Skip("nothing woke up")
	}
	id := grp.NumberIDs[0]
	a.CursorX, a.CursorY = id%GridSize, id/GridSize
	b.CursorX, b.CursorY = id%GridSize, id/GridSize

	a.Update(key("enter"))
	b.Update(key("enter")) // inside coRefineWindow

	tally := a.Room.Tally()
	if tally["MARK S."] == 0 || tally["HELLY R."] == 0 {
		t.Errorf("the file's attribution is %v, want both refiners credited", tally)
	}
}

func TestTheScoreboardOrderIsStable(t *testing.T) {
	l := NewLedger()
	for _, n := range []string{"DYLAN G.", "MARK S.", "HELLY R.", "IRVING B."} {
		l.Refined(n, 3) // all equal
	}
	first := l.Standings()
	for i := 0; i < 20; i++ {
		got := l.Standings()
		for j := range got {
			if got[j].Name != first[j].Name {
				t.Fatalf("the standings reordered themselves at row %d", j)
			}
		}
	}
}

// Pressing s shows it, and any other key goes back where you came from,
// including the lobby: dropping somebody onto the grid from there would seat
// them at a board they had not chosen.
func TestTheScoreboardReturnsYouWhereYouCameFrom(t *testing.T) {
	m := ready()
	m.Update(key("s"))
	if m.Screen != ScreenScores {
		t.Fatalf("s did not open the scoreboard; screen is %v", m.Screen)
	}
	m.Update(key("x"))
	if m.Screen != ScreenMain {
		t.Errorf("leaving the scoreboard landed on %v, want the grid", m.Screen)
	}

	rs := NewRooms(5)
	rs.Open()
	l := named(NewSession(rs, 1), "mark")
	l.Resize(120, 40)
	l.Update(key("enter")) // lobby
	l.Update(key("s"))
	if l.Screen != ScreenScores {
		t.Fatal("s did not open the scoreboard from the lobby")
	}
	l.Update(key("x"))
	if l.Screen != ScreenLobby {
		t.Errorf("leaving the scoreboard from the lobby landed on %v", l.Screen)
	}
	if l.Room != nil {
		t.Error("leaving the scoreboard seated the refiner at a board they never chose")
	}
}

// An unrouted screen falls through to the grid path, which reaches through the
// embedded *Room for a board a lobby session does not have. The program asks
// for motion reporting, so moving the pointer is enough.
func TestTheScoreboardIsSafeWithoutABoard(t *testing.T) {
	rs := NewRooms(7)
	rs.Open()
	m := named(NewSession(rs, 1), "mark")
	m.Resize(120, 40)
	m.Update(key("enter"))
	m.Update(key("s"))

	x, y := overTheGrid(m)
	m.Update(mouseMotion(x, y))
	m.Update(click(x, y))
	m.Update(mouseMotion(x+1, y+1))
	tick(m, 30)
	_ = drawTo(m)
}

// One person reading must not freeze the work for everybody.
func TestTheBoardKeepsMovingWhileSomebodyReadsTheScoreboard(t *testing.T) {
	_, a, b := twoInARoom(t)
	a.Update(key("s"))

	// A cluster's time awake is board state, so it only moves if the board is
	// being stepped.
	if b.FirstActive() == nil {
		t.Skip("nothing is awake to measure the board by")
	}
	before := b.ScaryElapsed()
	for i := 0; i < 60; i++ {
		tick(a, 1)
		tick(b, 1)
	}
	if b.ScaryElapsed() <= before {
		t.Error("the board stopped advancing while one refiner read the scoreboard")
	}
}

// scoresOpen is a terminal in the drawer with the scoreboard open.
func scoresOpen(t *testing.T, rs *Rooms, name string) *Model {
	t.Helper()
	m := session(t, rs, name)
	m.Update(key("enter"))
	m.Update(key("s"))
	if m.Screen != ScreenScores {
		t.Fatalf("setup: s in the drawer went to screen %v", m.Screen)
	}
	return m
}

// twoQuarters is a floor where OLD TIMER did everything last quarter and MARK S.
// has started this one.
func twoQuarters() *Rooms {
	rs := NewRooms(1)
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	rs.Ledger().now = func() time.Time { return now }
	rs.Ledger().Refined("OLD TIMER", 90)
	now = q4
	rs.Ledger().Refined("MARK S.", 4)
	return rs
}

// This quarter is the race anybody can still win.
func TestTheScoreboardOpensOnThisQuarter(t *testing.T) {
	m := scoresOpen(t, twoQuarters(), "helly")
	screen := screenOf(m)
	if !strings.Contains(screen, "THIS QUARTER") || !strings.Contains(screen, "MARK S.") {
		t.Errorf("the scoreboard did not open on this quarter:\n%s", screen)
	}
	if strings.Contains(screen, "OLD TIMER") {
		t.Errorf("last quarter's work is on this quarter's board:\n%s", screen)
	}
}

// This quarter, lifetime, the record, and back.
func TestTabTurnsToLifetimeAndTheRecord(t *testing.T) {
	m := scoresOpen(t, twoQuarters(), "helly")

	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if screen := screenOf(m); !strings.Contains(screen, "OLD TIMER") || !strings.Contains(screen, "MARK S.") {
		t.Errorf("the lifetime board does not have everybody on it:\n%s", screen)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if screen := screenOf(m); !strings.Contains(screen, "2026-Q3") || !strings.Contains(screen, "OLD TIMER") {
		t.Errorf("the record does not name 2026-Q3's Employee:\n%s", screen)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if screen := screenOf(m); strings.Contains(screen, "OLD TIMER") {
		t.Errorf("a third tab did not come back round to this quarter:\n%s", screen)
	}
	if m.Screen != ScreenScores {
		t.Error("tab left the scoreboard")
	}

	// The arrows go round as well, the other way first.
	m.Update(key("left"))
	if screen := screenOf(m); !strings.Contains(screen, "2026-Q3") {
		t.Errorf("left from this quarter did not reach the record:\n%s", screen)
	}
}

func TestYourOwnRowIsShownEvenOffScreen(t *testing.T) {
	rs := NewRooms(1)
	for i := 0; i < 60; i++ {
		rs.Ledger().Refined(nameFor(i), 50+i)
	}
	rs.Ledger().Refined("ZZ LAST", 1)

	m := scoresOpen(t, rs, "zz last")
	if screen := screenOf(m); !strings.Contains(screen, "ZZ LAST") {
		t.Errorf("a refiner ranked 61st cannot find themselves:\n%s", screen)
	}
}

// Names and what they refined stay; the other columns go first.
func TestTheScoreboardFitsANarrowTerminal(t *testing.T) {
	m := scoresOpen(t, twoQuarters(), "helly")
	m.Resize(30, 10)
	screen := screenOf(m)
	for _, want := range []string{"MARK S.", "REFINED"} {
		if !strings.Contains(screen, want) {
			t.Errorf("a 30-column scoreboard has no %q:\n%s", want, screen)
		}
	}
	for _, line := range strings.Split(screen, "\n") {
		if n := len([]rune(strings.TrimRight(line, " "))); n > 30 {
			t.Errorf("a line is %d wide on a 30-column terminal: %q", n, line)
		}
	}
}
