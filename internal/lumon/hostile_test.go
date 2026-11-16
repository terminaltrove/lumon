package lumon

import (
	"testing"
	"time"
)

// A remote window size is a client-supplied uint32 that reaches NewLayout, and
// the frame buffer costs about eighteen bytes a cell. 100000x100000 asks for ten
// billion cells, and a Go out-of-memory is fatal (no recover catches it), so one
// unauthenticated connection would take down the floor. The bound is on Layout,
// since every drawing decision is made against it; clamping only the buffer
// would leave a silently cropped screen.
func TestAHostileTerminalSizeIsRefused(t *testing.T) {
	for _, c := range []struct {
		name string
		w, h int
	}{
		{"the size that kills the process", 100000, 100000},
		{"one large enough to hurt a small machine", 20000, 20000},
		{"a merely absurd one", 5000, 2000},
		{"an int32 away from overflowing", 1 << 30, 1 << 30},
	} {
		m := ready()
		m.Resize(c.w, c.h)

		if m.Layout.Width > MaxCols || m.Layout.Height > MaxRows {
			t.Errorf("%s: %dx%d was taken as %dx%d, over the %dx%d bound",
				c.name, c.w, c.h, m.Layout.Width, m.Layout.Height, MaxCols, MaxRows)
			continue
		}
		// It still draws after being clamped.
		if buf := drawTo(m); buf == nil {
			t.Errorf("%s: nothing drew", c.name)
		}
	}
}

func TestARealTerminalIsLeftAlone(t *testing.T) {
	for _, c := range [][2]int{{80, 24}, {120, 40}, {200, 50}, {400, 100}} {
		m := ready()
		m.Resize(c[0], c[1])
		if m.Layout.Width != c[0] || m.Layout.Height != c[1] {
			t.Errorf("%dx%d came back as %dx%d", c[0], c[1], m.Layout.Width, m.Layout.Height)
		}
	}
}

func TestATinyTerminalStillDraws(t *testing.T) {
	for _, c := range [][2]int{{0, 0}, {1, 1}, {-5, -5}, {12, 3}} {
		m := ready()
		m.Resize(c[0], c[1])
		if m.Layout.Width < 1 || m.Layout.Height < 1 {
			t.Errorf("%dx%d left the layout at %dx%d", c[0], c[1], m.Layout.Width, m.Layout.Height)
		}
		drawTo(m)
	}
}

// Open is one keypress from the lobby and an abandoned room is swept only after
// thirty minutes, so connect, press n, disconnect, repeat must not grow without
// limit (about half a megabyte a go).
func TestOpeningFilesForeverIsRefused(t *testing.T) {
	rs := NewRooms(1)
	for i := 0; i < MaxRooms*3; i++ {
		if r := rs.Open(); r == nil {
			t.Fatalf("open %d handed back no room at all", i)
		}
	}
	if got := len(rs.rooms); got > MaxRooms {
		t.Errorf("the floor holds %d files, over the cap of %d", got, MaxRooms)
	}
}

// Somebody pressing "new file" on a busy server gets a file, without being told
// why it is not a fresh one.
func TestAFullFloorStillHandsOutAFile(t *testing.T) {
	rs := NewRooms(1)
	for i := 0; i < MaxRooms+10; i++ {
		rs.Open()
	}
	r := rs.Open()
	if r == nil {
		t.Fatal("a full floor refused to hand out any file at all")
	}
	if d := r.Seat("REFINER 1"); d == nil {
		t.Error("the file handed out on a full floor had no desk free")
	}
}

// A name costs nothing on an anonymous server: connect, type twelve fresh
// characters, refine one cluster, leave. The in-memory ledger must be bounded,
// as well as what reaches disk.
func TestTheLedgerDoesNotGrowWithoutBound(t *testing.T) {
	l := NewLedger()
	for i := 0; i < LedgerLimit*2; i++ {
		l.Refined(nameFor(i), 1)
	}
	if got := len(l.by); got > LedgerLimit {
		t.Errorf("the ledger holds %d refiners, over the cap of %d", got, LedgerLimit)
	}
	if len(l.by) == 0 {
		t.Error("the ledger forgot everybody")
	}
}

func TestTheLedgerForgetsTheQuietestFirst(t *testing.T) {
	now := time.Unix(0, 0)
	l := &Ledger{by: map[string]*Standing{}, now: func() time.Time { return now }}

	l.Refined("EARLY ONE", 1)
	for i := 0; i < LedgerLimit; i++ {
		now = now.Add(time.Second)
		l.Refined(nameFor(i), 1)
	}
	// Seen again right at the end, so they are among the most recent.
	now = now.Add(time.Hour)
	l.Refined("EARLY ONE", 1)
	for i := 0; i < LedgerLimit/2; i++ {
		now = now.Add(time.Second)
		l.Refined(nameFor(LedgerLimit+i), 1)
	}

	if _, ok := l.Of("EARLY ONE"); !ok {
		t.Error("somebody who had just refined was forgotten")
	}
}

// drawScores asks for it inside View, sixty times a second per session.
func TestTheScoreboardIsBuiltOncePerChange(t *testing.T) {
	l := NewLedger()
	for i := 0; i < 200; i++ {
		l.Refined(nameFor(i), i%7+1)
	}

	first := l.Standings()
	second := l.Standings()
	if &first[0] != &second[0] {
		t.Error("asking twice with nothing changed built the scoreboard twice")
	}

	// A refinement is a change, so the next reader gets the new answer.
	l.Refined("SOMEBODY NEW", 99)
	third := l.Standings()
	if len(third) != len(second)+1 {
		t.Errorf("the scoreboard has %d rows after a new refiner, want %d",
			len(third), len(second)+1)
	}
	if third[0].Name != "SOMEBODY NEW" {
		t.Errorf("the new leader is %q, want SOMEBODY NEW", third[0].Name)
	}
}

// nameFor is a distinct valid nameplate per index.
func nameFor(i int) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	return string([]byte{
		alphabet[i/26/26%26], alphabet[i/26%26], alphabet[i%26],
	}) + " " + itoa(i%1000)
}

// A tally key becomes Standing.Name and is drawn to every terminal in the room,
// and the canvas filters nothing. Typed names go through tidyName; the load path
// must sanitise too.
func TestATamperedSaveCannotDrawArbitraryBytes(t *testing.T) {
	sv := fileState{
		file:  "Point Reyes",
		seed:  1,
		tally: map[string]int{"\x1b[2JOWNED": 5, "MARK S.": 3},
	}
	r := restoreRoom(sv, NewLedger(), time.Now)
	if r == nil {
		t.Fatal("the save did not restore at all")
	}

	for _, st := range r.FileStandings() {
		for _, ch := range st.Name {
			if ch < 0x20 || ch == 0x7f {
				t.Errorf("a standing named %q survived the load with a control character", st.Name)
				break
			}
		}
	}
	// The legitimate one is still there, so this filters and does not drop.
	found := false
	for _, st := range r.FileStandings() {
		if st.Name == "MARK S." {
			found = true
		}
	}
	if !found {
		t.Error("filtering the tally lost an ordinary refiner")
	}
}

// A save with impossible counts restores at once and within bounds: negative
// bins once made reconcileQuotas loop for ever, before the server listened.
func TestASaveWithImpossibleCountsRestoresQuickly(t *testing.T) {
	var bins [BinCount]int
	for i := range bins {
		bins[i] = -1 << 30
	}
	files := make([]fileState, MaxRooms+10)
	for i := range files {
		files[i] = fileState{file: "COLD HARBOR", seed: 1, gridSeed: 1, bins: bins}
	}
	fs := floorState{
		rooms:  files,
		ledger: []Standing{{Name: "MARK S.", Refined: -5, Files: -1}},
	}

	done := make(chan *Rooms, 1)
	go func() { done <- restoreFloor(fs, 1) }()
	var rs *Rooms
	select {
	case rs = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("a save with negative bin counts did not restore")
	}
	if n := len(rs.rooms); n > MaxRooms {
		t.Errorf("a save of %d files restored %d, past MaxRooms (%d)", len(files), n, MaxRooms)
	}
	for _, r := range rs.rooms {
		for i, b := range r.Grid.Bins {
			if b.Refined < 0 {
				t.Fatalf("bin %d restored holding %d", i, b.Refined)
			}
		}
	}
	for _, st := range rs.Ledger().Standings() {
		if st.Refined < 0 || st.Files < 0 {
			t.Errorf("%s restored with %d refined and %d files", st.Name, st.Refined, st.Files)
		}
	}
}

// fullOfBoundNames is a ledger at its limit, every row bound and last seen at seen.
func fullOfBoundNames(now, seen time.Time) *Ledger {
	l := &Ledger{by: map[string]*Standing{}, now: func() time.Time { return now }}
	for i := 0; i < LedgerLimit; i++ {
		name := nameFor(i)
		l.by[name] = &Standing{Name: name, Key: "key-" + name, Seen: seen}
	}
	return l
}

// Filling the ledger must not free a name somebody is still using.
func TestAFullLedgerKeepsARecentlySeenBoundName(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	l := fullOfBoundNames(now, now.Add(-boundNameKept+time.Hour))
	l.Refined("NEWCOMER", 1)
	if got := len(l.by); got != LedgerLimit {
		t.Fatalf("the ledger holds %d rows, want it left at %d", got, LedgerLimit)
	}
	if _, ok := l.by[nameFor(0)]; !ok {
		t.Error("a bound name seen recently was forgotten")
	}
	if _, ok := l.by["NEWCOMER"]; ok {
		t.Error("a newcomer was recorded in a ledger with no room")
	}
}

// A server must not be full for good of people who never came back.
func TestAFullLedgerForgetsALongAbandonedBoundName(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	l := fullOfBoundNames(now, now.Add(-boundNameKept-time.Hour))
	l.Refined("NEWCOMER", 1)
	if _, ok := l.by["NEWCOMER"]; !ok {
		t.Error("a newcomer found no room among names abandoned for months")
	}
	if got := len(l.by); got > LedgerLimit {
		t.Errorf("the ledger holds %d rows, over its limit", got)
	}
}

// One name that may go is enough, and it goes before anything bound and recent.
func TestAFullLedgerForgetsAnUnboundNameFirst(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	l := fullOfBoundNames(now, now)
	l.by[nameFor(0)].Key = ""
	l.Refined("NEWCOMER", 1)
	if _, ok := l.by[nameFor(0)]; ok {
		t.Error("the one unbound name was kept")
	}
	if _, ok := l.by["NEWCOMER"]; !ok {
		t.Error("the newcomer got no row")
	}
	if _, ok := l.by[nameFor(1)]; !ok {
		t.Error("a bound name went while an unbound one could")
	}
}
