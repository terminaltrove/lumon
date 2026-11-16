package lumon

import (
	"strings"
	"testing"
	"time"
)

// q4 is an afternoon in the fourth quarter of 2026.
var q4 = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

// ledgerAt is a ledger on a clock the test moves.
func ledgerAt(t time.Time) (*Ledger, *time.Time) {
	now := t
	l := NewLedger()
	l.now = func() time.Time { return now }
	return l, &now
}

// UTC, so the floor agrees on when a quarter turns whatever anybody's timezone.
func TestTheQuarterIsTheCalendarQuarterInUTC(t *testing.T) {
	for _, c := range []struct {
		at   time.Time
		want string
	}{
		{time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "2026-Q1"},
		{time.Date(2026, 3, 31, 23, 59, 59, 0, time.UTC), "2026-Q1"},
		{time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), "2026-Q2"},
		{time.Date(2026, 12, 31, 12, 0, 0, 0, time.UTC), "2026-Q4"},
		// New Year in Athens is still the old year in UTC.
		{time.Date(2027, 1, 1, 1, 0, 0, 0, time.FixedZone("EET", 2*60*60)), "2026-Q4"},
	} {
		if got := periodOf(c.at); got != c.want {
			t.Errorf("periodOf(%v) = %q, want %q", c.at, got, c.want)
		}
	}
}

// The two behind the Employee are kept with them.
func TestAQuarterTurningCrownsItsEmployee(t *testing.T) {
	l, now := ledgerAt(q4)
	l.Refined("MARK S.", 10)
	l.Refined("HELLY R.", 30)
	l.Refined("IRVING B.", 20)
	l.Refined("DYLAN G.", 5)

	*now = time.Date(2027, 1, 2, 9, 0, 0, 0, time.UTC)
	l.Refined("BURT G.", 1)

	past := l.Past()
	if len(past) != 1 {
		t.Fatalf("%d quarters are on the record after one turned, want 1", len(past))
	}
	if past[0].Quarter != "2026-Q4" {
		t.Errorf("the record is of %q, want 2026-Q4", past[0].Quarter)
	}
	var names []string
	for _, s := range past[0].Top {
		names = append(names, s.Name)
	}
	if got := strings.Join(names, ","); got != "HELLY R.,IRVING B.,MARK S." {
		t.Errorf("2026-Q4's top three are %s, want HELLY R.,IRVING B.,MARK S.", got)
	}
	if past[0].Top[0].Refined != 30 {
		t.Errorf("the Employee of the Quarter is recorded with %d refined, want 30", past[0].Top[0].Refined)
	}
}

func TestANewQuarterStartsFromNothingAndLifetimeIsKept(t *testing.T) {
	l, now := ledgerAt(q4)
	l.Refined("HELLY R.", 30)

	*now = time.Date(2027, 1, 2, 9, 0, 0, 0, time.UTC)
	l.Refined("BURT G.", 1)

	this := l.QuarterStandings()
	if len(this) != 1 || this[0].Name != "BURT G." || this[0].Refined != 1 {
		t.Errorf("this quarter's standings are %+v, want only BURT G. with 1", this)
	}
	helly, _ := l.Of("HELLY R.")
	if helly.Refined != 30 {
		t.Errorf("HELLY R.'s lifetime is %d after the quarter turned, want 30", helly.Refined)
	}
	if helly.Quarter.Refined != 0 {
		t.Errorf("HELLY R. carried %d into a quarter they have not worked", helly.Quarter.Refined)
	}
}

func TestAQuarterNobodyWorkedIsNotRecorded(t *testing.T) {
	l, now := ledgerAt(q4)
	l.Refined("HELLY R.", 3)

	// Two quarters on: Q4 had work and is recorded; Q1, which passed with the
	// server idle, had none and is not.
	*now = time.Date(2027, 5, 1, 0, 0, 0, 0, time.UTC)
	l.Refined("BURT G.", 1)

	if past := l.Past(); len(past) != 1 || past[0].Quarter != "2026-Q4" {
		t.Errorf("the record is %+v, want only 2026-Q4", past)
	}
	if got := l.Quarter(); got != "2027-Q2" {
		t.Errorf("the quarter is %q, want 2027-Q2", got)
	}
}

func TestAClockGoingBackDoesNotUnturnAQuarter(t *testing.T) {
	l, now := ledgerAt(time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC))
	l.Refined("BURT G.", 1)

	*now = q4
	l.Refined("BURT G.", 1)

	if got := l.Quarter(); got != "2027-Q1" {
		t.Errorf("the quarter went back to %q", got)
	}
	if past := l.Past(); len(past) != 0 {
		t.Errorf("going backwards put %+v on the record", past)
	}
	if burt, _ := l.Of("BURT G."); burt.Quarter.Refined != 2 {
		t.Errorf("BURT G. has %d this quarter, want both credits", burt.Quarter.Refined)
	}
}

func TestPastQuartersAreCapped(t *testing.T) {
	l, now := ledgerAt(time.Date(2000, 1, 2, 0, 0, 0, 0, time.UTC))
	for i := 0; i < pastQuarters+5; i++ {
		l.Refined("BURT G.", 1)
		*now = now.AddDate(0, 3, 0)
	}
	l.Refined("BURT G.", 1)

	past := l.Past()
	if len(past) != pastQuarters {
		t.Fatalf("the record holds %d quarters, want %d", len(past), pastQuarters)
	}
	if past[0].Quarter <= past[len(past)-1].Quarter {
		t.Errorf("the record is not newest first: %s ... %s", past[0].Quarter, past[len(past)-1].Quarter)
	}
	if want := periodOf(now.AddDate(0, -3, 0)); past[0].Quarter != want {
		t.Errorf("the newest quarter on the record is %s, want %s", past[0].Quarter, want)
	}
}

// The quarter turns on the autosave, so an idle floor still crowns on time.
func TestTheQuarterTurnsOnASnapshotWithNobodyRefining(t *testing.T) {
	rs := NewRooms(1)
	now := q4
	rs.Ledger().now = func() time.Time { return now }
	rs.Ledger().Refined("HELLY R.", 4)

	now = time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC)
	snap := rs.capture()
	if snap.quarter != "2027-Q1" {
		t.Errorf("the snapshot is of quarter %q, want 2027-Q1", snap.quarter)
	}
	if len(snap.pastQuarters) != 1 || snap.pastQuarters[0].Quarter != "2026-Q4" {
		t.Errorf("the snapshot's record is %+v, want 2026-Q4", snap.pastQuarters)
	}
}

// A server that was down over the turn crowns the missed quarter as it starts.
func TestTheQuarterTurnsOnLoadWhenTheSaveIsStale(t *testing.T) {
	sv := floorState{
		quarter: "2026-Q3",
		ledger: []Standing{{
			Name: "HELLY R.", Refined: 40,
			Quarter: QuarterScore{Of: "2026-Q3", Refined: 12},
		}},
	}
	rs := restoreFloor(sv, 1)

	if got, want := rs.Ledger().Quarter(), periodOf(time.Now()); got != want {
		t.Errorf("a stale save came back in quarter %q, want %q", got, want)
	}
	past := rs.Ledger().Past()
	if len(past) != 1 || past[0].Quarter != "2026-Q3" || past[0].Top[0].Name != "HELLY R." {
		t.Errorf("the quarter the server slept through was not crowned: %+v", past)
	}
}

// Both co-refiners sorted the same cluster, so both are credited its temper.
func TestACoRefinerIsCreditedTheClustersTemper(t *testing.T) {
	r, a, b := twoInARoom(t)
	a.Name, b.Name = "MARK S.", "HELLY R."
	a.Desk.Name, b.Desk.Name = a.Name, b.Name

	r.Claim(7, []int{100, 101}, a.Desk, Frolic)
	if !r.CoRefine(100, b.Desk) {
		t.Fatal("setup: the co-refine was not credited")
	}
	for _, name := range []string{"MARK S.", "HELLY R."} {
		s, _ := r.Ledger().Of(name)
		if s.Tempers[Frolic] != 1 {
			t.Errorf("%s has tempers %v, want one Frolic", name, s.Tempers)
		}
	}
}

func TestFinishedFilesSurviveARestartAndAreCapped(t *testing.T) {
	rs := NewRooms(1)
	for i := 0; i < recentFiles+5; i++ {
		rs.Ledger().CloseFile("FILE "+itoa(i), map[string]int{"MARK S.": i + 1, "HELLY R.": 1})
	}

	recent := restoreFloor(rs.capture(), 1).Ledger().Recent()
	if len(recent) != recentFiles {
		t.Fatalf("%d finished files came back, want %d", len(recent), recentFiles)
	}
	if want := "FILE " + itoa(recentFiles+4); recent[0].File != want {
		t.Errorf("the newest finished file is %q, want %q", recent[0].File, want)
	}
	if recent[0].Top != "MARK S." || recent[0].Counts["MARK S."] != recentFiles+5 {
		t.Errorf("the newest file's attribution came back as %+v", recent[0])
	}
}

func TestTheQuarterLeadersAreSavedOutsideTheLifetimeTop(t *testing.T) {
	rs := NewRooms(1)
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	rs.Ledger().now = func() time.Time { return now }
	for i := 0; i < ledgerCap+20; i++ {
		rs.Ledger().Refined(nameFor(i), 100)
	}
	now = q4
	rs.Ledger().Refined("NEWCOMER", 1)

	back := restoreFloor(rs.capture(), 1).Ledger()
	if s, ok := back.Of("NEWCOMER"); !ok || s.Refined != 1 {
		t.Error("this quarter's only refiner was not saved, because they are not in the lifetime top")
	}
}

// Every name and file name on the record reaches somebody's screen.
func TestATamperedRecordCannotDrawArbitraryBytes(t *testing.T) {
	const esc = "\x1b[2J"
	sv := floorState{
		quarter: periodOf(time.Now()),
		rooms:   []fileState{{file: "Sunset " + esc + "Park", seed: 1, history: []string{esc + "Molde"}}},
		pastQuarters: []QuarterRecord{
			{Quarter: "2026-Q1", Top: []Standing{{Name: esc + "MARK", Refined: 3}}},
			// A malformed quarter is dropped whole.
			{Quarter: "2026-Q2" + esc, Top: []Standing{{Name: "HELLY", Refined: 3}}},
		},
		recent: []FileRecord{{
			File: esc + "Cold Harbor", Top: esc + "HELLY",
			Counts: map[string]int{esc + "HELLY": 2},
		}},
	}
	rs := restoreFloor(sv, 1)

	var drawn []string
	for _, q := range rs.Ledger().Past() {
		drawn = append(drawn, q.Quarter)
		for _, s := range q.Top {
			drawn = append(drawn, s.Name)
		}
	}
	for _, f := range rs.Ledger().Recent() {
		drawn = append(drawn, f.File, f.Top)
		for k := range f.Counts {
			drawn = append(drawn, k)
		}
	}
	for _, r := range rs.List() {
		drawn = append(drawn, r.Name)
	}
	for _, room := range rs.rooms {
		drawn = append(drawn, room.History...)
	}
	if len(drawn) < 7 {
		t.Fatalf("the record did not come back at all: %q", drawn)
	}
	if past := rs.Ledger().Past(); len(past) != 1 {
		t.Errorf("a malformed quarter was kept: %+v", past)
	}
	for _, s := range drawn {
		if strings.ContainsAny(s, "\x1b\x07\x7f") {
			t.Errorf("%q came back from a save with a control character in it", s)
		}
	}
}
