package lumon

import (
	"math"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/terminaltrove/lumon/internal/canvas"
)

// session is a terminal that has come in through the lobby, with the boot
// sequence skipped, whose refiner has typed the given name and not yet pressed
// enter.
func session(t *testing.T, rs *Rooms, name string) *Model {
	t.Helper()
	m := named(NewSession(rs, 1), name)
	m.Resize(120, 40)
	return m
}

// named types a name at a session's name screen, as its refiner would.
func named(m *Model, name string) *Model {
	if name != "" {
		m.Update(key(name))
	}
	return m
}

func TestASessionIsAskedWhoItIs(t *testing.T) {
	m := session(t, NewRooms(1), "")

	if m.Screen != ScreenName {
		t.Fatalf("a new session is on screen %v, want the name screen", m.Screen)
	}
	if m.Room != nil {
		t.Error("a session took a desk before choosing a file")
	}
	// Nobody is assumed. The field is empty until somebody types in it.
	if m.Name != "" {
		t.Errorf("a new session arrived already called %q", m.Name)
	}

	// It has to render without a board behind it.
	buf := drawTo(m)
	var screen string
	_, h := buf.Size()
	for y := 0; y < h; y++ {
		screen += buf.Row(y)
	}
	if !strings.Contains(screen, "IDENTIFY") {
		t.Errorf("the name screen does not ask for a name:\n%s", screen)
	}
}

func TestTheNameCanBeTyped(t *testing.T) {
	m := session(t, NewRooms(1), "mark")

	for i := 0; i < 4; i++ {
		m.Update(key("backspace"))
	}
	for _, k := range []string{"h", "e", "l", "l", "y"} {
		m.Update(key(k))
	}

	if m.Name != "HELLY" {
		t.Errorf("typed name is %q, want HELLY", m.Name)
	}
}

func TestAnEmptyNameIsNotAccepted(t *testing.T) {
	m := session(t, NewRooms(1), "")

	m.Update(key("enter"))

	if m.Screen != ScreenName {
		t.Error("an empty name was accepted")
	}
}

func TestNamesAreHouseStyle(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"mark", "MARK"},
		{"  helly  ", "HELLY"},
		{"irving b.", "IRVING B."},
		{"dylan-g", "DYLAN-G"},
		{"a!@#$%^&*()b", "AB"},
		{"averyverylongnameindeed", "AVERYVERYLON"},
		{"mark  s.", "MARK S."},
	} {
		if got := tidyName(c.in); got != c.want {
			t.Errorf("tidyName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// space is the space bar as Bubble Tea delivers it: a key of its own type that
// also has the rune, and whose String is " ", not "space".
var space = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}

// The space must survive the tidy until the next letter follows it, or MARK S.
// can only be MARKS.
func TestASpaceCanBeTypedBetweenWords(t *testing.T) {
	m := session(t, NewRooms(1), "")
	for _, k := range []tea.KeyMsg{key("m"), key("a"), key("r"), key("k"), space, key("s"), key(".")} {
		m.Update(k)
	}
	if m.Name != "MARK S." {
		t.Errorf("typed name is %q, want MARK S.", m.Name)
	}
}

// A trailing space is dropped once the name is given.
func TestALeadingOrDoubledSpaceIsNotKept(t *testing.T) {
	m := session(t, NewRooms(1), "")
	for _, k := range []tea.KeyMsg{space, key("m"), space, space, key("s"), space} {
		m.Update(k)
	}
	m.Update(key("enter"))
	if m.Name != "M S" {
		t.Errorf("typed name is %q, want M S", m.Name)
	}
}

// Bubble Tea delivers a run of printable characters as one message, from a fast
// typist on a slow link or a client that writes a name in one go.
func TestKeysThatArriveTogetherAreAllTyped(t *testing.T) {
	m := session(t, NewRooms(1), "")
	m.Update(key("helly"))
	if m.Name != "HELLY" {
		t.Errorf("a run of keys typed %q, want HELLY", m.Name)
	}
}

func TestAPastedNameIsTyped(t *testing.T) {
	m := session(t, NewRooms(1), "")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("irving b!"), Paste: true})
	if m.Name != "IRVING B" {
		t.Errorf("a pasted name typed %q, want IRVING B", m.Name)
	}
}

func TestAnAltChordIsNotTyped(t *testing.T) {
	m := session(t, NewRooms(1), "")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x"), Alt: true})
	if m.Name != "" {
		t.Errorf("alt+x typed %q", m.Name)
	}
}

func TestTheLobbyListsTheOpenFiles(t *testing.T) {
	rs := NewRooms(5)
	rs.Open()
	rs.Open()

	m := session(t, rs, "mark")
	m.Update(key("enter")) // accept the name

	if m.Screen != ScreenLobby {
		t.Fatalf("screen is %v after naming, want the lobby", m.Screen)
	}
	tick(m, 60)

	// Every file, including unopened ones: the drawer is a directory and most of it
	// has never been worked.
	if got, want := m.rolodex().Len(), len(DirectoryFiles); got < want {
		t.Errorf("the drawer holds %d files against a directory of %d", got, want)
	}

	// The card that is showing is drawn, name and all.
	e, _ := m.rolodex().Selected()
	if !onScreen(drawTo(m), e.Name) {
		t.Errorf("the card for %q is not on screen", e.Name)
	}
}

// onScreen reports whether a string appears anywhere in a drawn frame.
func onScreen(buf *canvas.Buffer, want string) bool {
	_, h := buf.Size()
	for y := 0; y < h; y++ {
		if strings.Contains(buf.Row(y), want) {
			return true
		}
	}
	return false
}

func TestJoiningFromTheLobbyTakesADesk(t *testing.T) {
	rs := NewRooms(5)
	rs.Open()

	m := session(t, rs, "mark")
	m.Update(key("enter")) // name
	riffled(m)
	e, _ := m.rolodex().Selected()
	m.Update(key("enter")) // take the card the drawer is showing

	if m.Screen != ScreenMain {
		t.Fatalf("screen is %v after joining, want the grid", m.Screen)
	}
	if m.Room == nil || m.FileName != e.Name {
		t.Errorf("took the card for %q and landed on %q", e.Name, m.FileName)
	}
	room := m.Room
	if m.Desk == nil {
		t.Fatal("joined without taking a desk")
	}
	if m.Desk.Name != "MARK" {
		t.Errorf("the nameplate reads %q, want MARK", m.Desk.Name)
	}
	if m.FileName != room.FileName {
		t.Error("the terminal is showing a different file than the room is working")
	}
}

func TestTheLobbyCanOpenANewFile(t *testing.T) {
	rs := NewRooms(5)
	rs.Open()

	m := session(t, rs, "mark")
	m.Update(key("enter"))
	before := rs.Count()

	m.Update(key("n"))

	if got := rs.Count(); got != before+1 {
		t.Errorf("%d rooms after opening one, want %d", got, before+1)
	}
	if m.Screen != ScreenMain {
		t.Error("opening a file did not sit the refiner down at it")
	}
}

// A refused join leaves the refiner in the lobby to choose again.
func TestAFullFileCannotBeJoined(t *testing.T) {
	rs := NewRooms(5)
	room := rs.Open()
	for i := 0; i < MaxDesks; i++ {
		if JoinRoom(room, int64(i)) == nil {
			t.Fatalf("could not fill seat %d", i)
		}
	}

	m := session(t, rs, "mark")
	m.Update(key("enter"))
	turnTo(t, m, room.FileName)
	m.Update(key("enter")) // try to take the full file

	if m.Screen != ScreenLobby {
		t.Errorf("screen is %v after a refused join, want the lobby", m.Screen)
	}
	if m.Room != nil {
		t.Error("a refiner got into a full room")
	}

	e, _ := m.rolodex().Selected()
	if !e.Full() {
		t.Error("the drawer does not report the file as full")
	}
}

// turnTo winds the drawer round to a named file.
func turnTo(t *testing.T, m *Model, name string) {
	t.Helper()
	r := m.rolodex()
	for i, e := range r.Files() {
		if e.Name == name {
			r.GoTo(i)
			tick(m, 60)
			return
		}
	}
	t.Fatalf("no card for %q in the drawer", name)
}

func TestTheLobbyChoosesWhatIsHighlighted(t *testing.T) {
	rs := NewRooms(9)
	m := session(t, rs, "mark")
	m.Update(key("enter"))
	m.Update(key("down"))
	m.Update(key("down"))

	want, _ := m.rolodex().Selected()
	m.Update(key("enter"))

	if m.Room == nil {
		t.Fatal("did not join anything")
	}
	if m.FileName != want.Name {
		t.Errorf("joined %q, want the card that was showing, %q", m.FileName, want.Name)
	}
}

func TestTheIntroLeadsToTheNameScreen(t *testing.T) {
	m := named(NewSession(NewRooms(1), 1), "mark")
	m.Resize(120, 40)
	m.StartIntro()

	for i := 0; i < 60*30 && m.Screen == ScreenIntro; i++ {
		tick(m, 1)
	}

	if m.Screen != ScreenName {
		t.Errorf("the boot sequence handed over to %v, want the name screen", m.Screen)
	}
}

// NewModel builds such a session, and --file uses it for a local run that
// wants the board it was just on.
func TestASessionWithABoardSkipsTheLobby(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	m.StartIntro()

	for i := 0; i < 60*30 && m.Screen == ScreenIntro; i++ {
		tick(m, 1)
	}

	if m.Screen != ScreenMain {
		t.Errorf("a session with a board went to %v, want straight to the grid", m.Screen)
	}
}

// There is nobody to introduce yourself to alone, so the name prompt belongs
// only to sharing a board.
func TestALocalRunReachesTheFiles(t *testing.T) {
	rs := NewRooms(11)
	rs.EnsureOpen(DefaultRooms)

	m := NewLocalSession(rs, 1, "mark s")
	m.Resize(120, 40)
	m.StartIntro()

	for i := 0; i < 60*30 && m.Screen == ScreenIntro; i++ {
		tick(m, 1)
	}

	if m.Screen != ScreenLobby {
		t.Errorf("a local run went to %v, want the files", m.Screen)
	}
	if m.Name != "MARK S" {
		t.Errorf("the name is %q, want it taken from the environment", m.Name)
	}
	if m.Room != nil {
		t.Error("a session choosing a file is already sitting at one")
	}
}

// NewSession pre-fills the name from the login, so askName is a field and is
// not inferred from an empty name.
func TestAnSSHSessionIsStillAskedItsName(t *testing.T) {
	rs := NewRooms(12)
	rs.EnsureOpen(DefaultRooms)

	m := named(NewSession(rs, 1), "helly")
	m.Resize(120, 40)
	m.StartIntro()

	for i := 0; i < 60*30 && m.Screen == ScreenIntro; i++ {
		tick(m, 1)
	}
	if m.Screen != ScreenName {
		t.Errorf("a remote session went to %v, want the name prompt", m.Screen)
	}
}

// Otherwise the first thing the terminal does is refuse you.
func TestTheDrawerOpensOnAJoinableFile(t *testing.T) {
	rs := NewRooms(23)

	// Fill the first card in the directory, so resting on it would be wrong.
	full := rs.OpenNamed(DirectoryFiles[0])
	for i := 0; i < MaxDesks; i++ {
		JoinRoom(full, int64(i))
	}

	m := session(t, rs, "mark")
	m.Update(key("enter")) // into the drawer

	e, ok := m.rolodex().Selected()
	if !ok {
		t.Fatal("the drawer is empty")
	}
	if e.Full() {
		t.Errorf("the drawer opened on %q, which is full", e.Name)
	}
	m.Update(key("enter"))
	if m.Room == nil {
		t.Fatal("did not take the card the drawer was showing")
	}
	if m.FileName == full.FileName {
		t.Error("took the full file after all")
	}
}

// One card is showing, in the middle of the screen, so what is asserted is the
// card's text.
func TestAFullCardSaysItIsFull(t *testing.T) {
	rs := NewRooms(29)
	full := rs.OpenNamed("Minsk")
	for i := 0; i < MaxDesks; i++ {
		JoinRoom(full, int64(i))
	}

	m := session(t, rs, "mark")
	m.Update(key("enter"))
	turnTo(t, m, "Minsk")

	buf := drawTo(m)
	if !onScreen(buf, "Minsk") {
		t.Error("the card is not showing the file it is for")
	}
	if !onScreen(buf, "FULL") {
		t.Error("a file with every desk taken does not say so on its card")
	}
}

// The global quit is checked before any screen dispatch, so the name prompt
// must exempt Q or nobody could be called QUINN.
func TestYouCanBeCalledQuinn(t *testing.T) {
	m := NewSession(NewRooms(1), 1)
	m.Resize(120, 40)

	for _, k := range []string{"q", "u", "i", "n", "n"} {
		m.Update(key(k))
	}

	if m.Quitting {
		t.Fatal("typing a name that starts with Q quit the program")
	}
	if m.Screen != ScreenName {
		t.Fatalf("typing a name left the name screen for %v", m.Screen)
	}
	if m.Name != "QUINN" {
		t.Errorf("the name came out %q, want QUINN", m.Name)
	}
}

// ctrl+c is the terminal's own gesture and works everywhere, name screen
// included.
func TestCtrlCStillQuitsWhileTypingAName(t *testing.T) {
	m := named(NewSession(NewRooms(1), 1), "mark")
	m.Resize(120, 40)
	m.Update(key("ctrl+c"))
	if !m.Quitting {
		t.Error("ctrl+c did not quit from the name screen")
	}
}

func TestQStillQuitsOnTheGrid(t *testing.T) {
	m := ready()
	m.Update(key("q"))
	if !m.Quitting {
		t.Error("q no longer quits from the grid")
	}
}

// Join returns a bool; ignoring it leaves the lobby redrawing unchanged while
// the refiner presses enter at a file that will never let them in.
func TestAFullFileSaysSoRatherThanIgnoringYou(t *testing.T) {
	rs := NewRooms(11)
	room := rs.Open()
	for i := 0; i < MaxDesks; i++ {
		if room.Seat("") == nil {
			t.Fatalf("could not fill desk %d", i)
		}
	}

	m := named(NewSession(rs, 1), "mark")
	m.Resize(120, 40)
	m.Update(key("enter")) // into the lobby
	turnTo(t, m, room.FileName)
	m.Update(key("enter")) // try to take the full file

	if m.Room != nil {
		t.Fatal("joined a file with every desk taken")
	}
	if m.Screen != ScreenLobby {
		t.Fatalf("a refused join left the lobby for %v", m.Screen)
	}
	if m.notice == "" {
		t.Error("a refused join said nothing at all")
	}

	buf := drawTo(m)
	var screen string
	_, h := buf.Size()
	for y := 0; y < h; y++ {
		screen += buf.Row(y)
	}
	if !strings.Contains(screen, "full") {
		t.Errorf("the lobby does not tell the refiner the file is full:\n%s", screen)
	}
}

// Nothing is committed in the lobby, so backing out means going back; a refiner
// who mistyped their name has no other way back short of dropping the
// connection. q is backOut's bottom rung, since the terminal no longer issues
// esc.
func TestBackingOutOfTheLobbyReachesTheNamePrompt(t *testing.T) {
	rs := NewRooms(13)
	rs.Open()
	m := named(NewSession(rs, 1), "mark")
	m.Resize(120, 40)
	m.Update(key("enter"))
	if m.Screen != ScreenLobby {
		t.Fatal("did not reach the lobby")
	}

	m.Update(key("q"))

	if m.Screen != ScreenName {
		t.Errorf("q in the lobby left the screen at %v, want the name prompt", m.Screen)
	}
	if m.Quitting {
		t.Error("q in the lobby quit the program on the first press")
	}

	// Escape does not go back, because escape does nothing.
	m.Screen = ScreenLobby
	m.Update(key("esc"))
	if m.Screen != ScreenLobby {
		t.Errorf("esc moved the screen to %v; it is not a key this terminal has", m.Screen)
	}
	if !m.Escaped {
		t.Error("esc in the lobby did not raise the notice")
	}
}

// finishIntro nils the boot sequence before this screen is shown, so the
// blink must not depend on it.
func TestTheNameCursorActuallyBlinks(t *testing.T) {
	m := named(NewSession(NewRooms(1), 1), "mark")
	m.Resize(120, 40)

	seen := map[bool]bool{}
	for i := 0; i < 4; i++ {
		seen[m.cursorOn()] = true
		tick(m, int(cursorBlink*60)+1)
	}

	if !seen[true] || !seen[false] {
		t.Error("the name cursor never changed state; it is not blinking")
	}
}

func TestTheLobbyFooterComesFromTheTable(t *testing.T) {
	rs := NewRooms(3)
	rs.EnsureOpen(2)
	m := named(NewSession(rs, 1), "mark")
	m.Resize(120, 34)
	m.Screen = ScreenLobby

	buf := drawTo(m)
	footer := buf.Row(m.Layout.Height - 2)

	for _, want := range []string{"scores", "theme", "help"} {
		if !strings.Contains(footer, want) {
			t.Errorf("the lobby footer does not mention %q, though the lobby binds it:\n%q",
				want, footer)
		}
	}
	if got := m.lobbyHint(m.Layout.Width - 4); !strings.Contains(footer, got) {
		t.Errorf("the footer is not the ladder's line:\n got %q\nwant %q", footer, got)
	}
}

// The drawer's keys are the first thing a new refiner reads, so they are drawn
// as brightly as the board's, not in the backdrop colour.
func TestTheLobbyFooterIsAsReadableAsTheBoards(t *testing.T) {
	rs := NewRooms(3)
	rs.EnsureOpen(2)
	m := named(NewSession(rs, 1), "mark")
	m.Resize(120, 34)
	m.Screen = ScreenLobby

	buf := drawTo(m)
	y := m.Layout.Height - 2
	x := strings.Index(buf.Row(y), "help")
	if x < 0 {
		t.Fatalf("no help in the footer: %q", buf.Row(y))
	}
	if got := buf.AttrAt(len([]rune(buf.Row(y)[:x])), y).FG; got != m.Theme.Hint.FG {
		t.Errorf("the lobby footer is drawn in %v, not the board's hint colour %v", got, m.Theme.Hint.FG)
	}
}

// One notch is a shove: the drawer has momentum, so what is pinned is the
// direction, that it settles on a whole card, and that it stops at the ends.
func TestTheWheelTurnsTheDrawer(t *testing.T) {
	rs := NewRooms(3)
	rs.EnsureOpen(4)
	m := named(NewSession(rs, 1), "mark")
	m.Resize(120, 34)
	m.Screen = ScreenLobby

	r := m.rolodex()
	r.GoTo(10)
	tick(m, 5)
	at := r.Want()

	m.Update(wheel(false))
	settleReel(t, m)
	down := r.Want()
	if down <= at {
		t.Errorf("a notch down left the drawer on card %d, having been on %d", down, at)
	}

	m.Update(wheel(true))
	settleReel(t, m)
	if up := r.Want(); up >= down {
		t.Errorf("a notch up left the drawer on card %d, having been on %d", up, down)
	}

	// It stops at the ends.
	for i := 0; i < 60; i++ {
		m.Update(wheel(true))
		tick(m, 1)
	}
	settleReel(t, m)
	if got := r.Want(); got != 0 {
		t.Errorf("winding past the front of the drawer left it on card %d", got)
	}
	for i := 0; i < 200; i++ {
		m.Update(wheel(false))
		tick(m, 1)
	}
	settleReel(t, m)
	if got, last := r.Want(), r.Len()-1; got != last {
		t.Errorf("winding past the back left it on card %d of %d", got, last)
	}
}

// settleReel ticks until the drawer has come to rest on a card.
func settleReel(t *testing.T, m *Model) {
	t.Helper()
	r := m.rolodex()
	for i := 0; r.Turning(); i++ {
		if i > 900 {
			t.Fatalf("the drawer never settled: at %v going %v", r.At(), r.at.Velocity())
		}
		tick(m, 1)
	}
}

// The mark is on the boot screen, the name was typed two screens ago and the
// file's name is on the card, so a heading would only repeat them.
func TestTheDrawerHasNoHeading(t *testing.T) {
	m := drawer(t, 120, 40)
	box, ok := cardFits(m.Layout.Width, m.Layout.Height)
	if !ok {
		t.Fatal("the card does not fit")
	}
	buf := drawTo(m)

	for y := 0; y < box.hinge-box.spec.up-1; y++ {
		if got := strings.TrimSpace(string(buf.Row(y))); got != "" {
			t.Errorf("row %d above the card carries %q", y, got)
		}
	}
	if onScreen(buf, "LUMON INDUSTRIES") {
		t.Error("the heading is still on the drawer")
	}
}

// Nothing else on this screen uses the left margin.
func TestTheLobbyHintIsCentred(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {96, 30}, {120, 40}} {
		w, h := sz[0], sz[1]
		m := drawer(t, w, h)
		row := []rune(string(drawTo(m).Row(h - 2)))

		lo, hi := -1, -1
		for x, r := range row {
			if r != ' ' && r != 0 {
				if lo < 0 {
					lo = x
				}
				hi = x
			}
		}
		if lo < 0 {
			t.Fatalf("%dx%d: no hint drawn", w, h)
		}
		if mid, want := float64(lo+hi)/2, float64(w-1)/2; math.Abs(mid-want) > 1 {
			t.Errorf("%dx%d: the hint is centred on %.1f, the screen on %.1f",
				w, h, mid, want)
		}
	}
}

// A count is the same whoever the refiners are, and remotely the drawer exists
// to send you to them. The names come from Listing.Refiners, which is
// lock-correct: Room.Roster expects its caller to hold the lock, and a
// lobby session has no board and holds nothing.
func TestTheCardSaysWhoIsOnTheFile(t *testing.T) {
	rs := NewRooms(31)
	room := rs.OpenNamed("Minsk")
	room.Seat("MARK S.")
	room.Seat("HELLY R.")

	m := session(t, rs, "mark")
	m.Update(key("enter"))
	turnTo(t, m, "Minsk")

	buf := drawTo(m)
	for _, want := range []string{"MARK S.", "HELLY R."} {
		if !onScreen(buf, want) {
			t.Errorf("the card for Minsk does not say %q is on it", want)
		}
	}

	// A file nobody has opened has no roster.
	turnTo(t, m, "Narva")
	if e, _ := m.rolodex().Selected(); e.Open {
		t.Skip("Narva was opened by somebody")
	}
	if !onScreen(drawTo(m), "UNOPENED") {
		t.Error("an unopened file does not say so")
	}
}

// The percentage is never dropped, since it is what anyone looks for here, and
// the names go to initials before they go to a count.
func TestTheStatusDegradesWithTheCard(t *testing.T) {
	e := Entry{
		Listing: Listing{
			Name: "Minsk", Percent: 41, Occupied: 2, Desks: MaxDesks,
			Refiners: []Occupant{{Name: "MARK S."}, {Name: "HELLY R."}},
		},
		Open: true,
	}

	wide := cardStatus(e, 30)
	if !strings.Contains(wide, "MARK S.") || !strings.Contains(wide, "HELLY R.") {
		t.Errorf("a wide card says %q, want both names", wide)
	}
	mid := cardStatus(e, 14)
	if strings.Contains(mid, "MARK S.") || !strings.Contains(mid, "M.") {
		t.Errorf("a narrower card says %q, want initials", mid)
	}
	narrow := cardStatus(e, 11)
	if strings.Contains(narrow, "M.") {
		t.Errorf("a narrow card says %q, want a count", narrow)
	}

	for _, w := range []int{30, 20, 14, 11, 8, 4} {
		got := cardStatus(e, w)
		if !strings.HasPrefix(got, "41%") {
			t.Errorf("at width %d the status is %q; the percentage is never given up", w, got)
		}
		if len([]rune(got)) > w {
			t.Errorf("at width %d the status is %q, %d columns wide",
				w, got, len([]rune(got)))
		}
	}
}

// A desk's Name, Bin and Credit are written by that player's Update under the
// room's write lock. A pointer into one given to a lobby session, which holds
// no lock, is a data race.
func TestAListingCarriesNamesNotPointers(t *testing.T) {
	rs := NewRooms(37)
	room := rs.OpenNamed("Minsk")
	room.Seat("MARK S.")

	before := rs.Find("Minsk").listing()
	room.Seat("HELLY R.")
	room.Roster()[0].Name = "CHANGED"

	if len(before.Refiners) != 1 || before.Refiners[0].Name != "MARK S." {
		t.Errorf("the listing moved under us: %+v", before.Refiners)
	}
	if after := rs.Find("Minsk").listing(); len(after.Refiners) != 2 {
		t.Errorf("a fresh listing has %d refiners, want 2", len(after.Refiners))
	}
}

// The views are three renderings of one list with one cursor, so enter means
// the same in each and switching keeps your place.
func TestEveryViewJoinsTheSameFile(t *testing.T) {
	for view := 0; view < int(lobbyViews); view++ {
		rs := NewRooms(41)
		rs.EnsureOpen(DefaultRooms)
		m := NewLocalSession(rs, 1, "mark")
		m.Resize(120, 40)
		tick(m, 60)

		for i := 0; i < view; i++ {
			m.Update(key("v"))
		}
		if got := int(m.lobbyView); got != view {
			t.Fatalf("v pressed %d times leaves the view at %d", view, got)
		}

		m.rolodex().GoTo(12)
		settleReel(t, m)
		want, _ := m.rolodex().Selected()

		m.Update(key("enter"))
		if m.Room == nil {
			t.Fatalf("view %d: enter did not join anything", view)
		}
		if m.FileName != want.Name {
			t.Errorf("view %d joined %q, want %q", view, m.FileName, want.Name)
		}
	}
}

// In the rolodex and the list sideways means the next letter, because there is
// nothing beside the row you are on.
func TestTheGridArrowsAreSpatial(t *testing.T) {
	m := drawer(t, 120, 40)
	m.Update(key("v"))
	m.Update(key("v"))
	if m.lobbyView != viewGrid {
		t.Fatal("two presses of v did not reach the grid")
	}
	r := m.rolodex()
	cols := m.gridCols()
	if cols < 2 {
		t.Fatalf("only %d tiles fit across a 120-column terminal", cols)
	}

	r.GoTo(0)
	settleReel(t, m)
	m.Update(key("right"))
	settleReel(t, m)
	if got := r.Want(); got != 1 {
		t.Errorf("right in the grid moved to %d, want the next tile", got)
	}

	r.GoTo(0)
	settleReel(t, m)
	m.Update(key("down"))
	settleReel(t, m)
	if got := r.Want(); got != cols {
		t.Errorf("down in the grid moved to %d, want a whole row of %d", got, cols)
	}

	// Back in the list, sideways is a letter.
	m.Update(key("v"))
	if m.lobbyView != viewRolodex {
		t.Fatal("v did not wrap back round to the rolodex")
	}
	r.GoTo(0)
	settleReel(t, m)
	first, _ := r.Selected()
	m.Update(key("right"))
	settleReel(t, m)
	next, _ := r.Selected()
	if initialOf(next.Name) == initialOf(first.Name) {
		t.Errorf("right went %q -> %q, still under %c",
			first.Name, next.Name, initialOf(first.Name))
	}
}

// NewLocalSession goes straight to the files, so backOut must not send it to
// ScreenName, which it never came through. One field decides both the way in
// and the way back, so both halves are asserted.
func TestBackingOutOfTheDrawerOnlyAsksForANameOverSSH(t *testing.T) {
	rs := NewRooms(23)
	rs.EnsureOpen(DefaultRooms)

	local := NewLocalSession(rs, 1, "mark")
	local.Resize(120, 40)
	if local.Screen != ScreenLobby {
		t.Fatalf("a local session starts at %v, want the drawer", local.Screen)
	}
	local.Update(key("q"))
	if local.Screen == ScreenName {
		t.Error("q at the drawer asked a local session to identify itself")
	}
	if !local.Quitting {
		t.Error("q at the drawer did not quit; alone there is nothing behind it " +
			"to back out to")
	}

	remote := named(NewSession(rs, 1), "helly")
	remote.Resize(120, 40)
	remote.Screen = ScreenLobby
	remote.Update(key("q"))
	if remote.Screen != ScreenName {
		t.Errorf("q at the drawer took a remote session to %v, want the name prompt "+
			"it came through", remote.Screen)
	}
	if remote.Quitting {
		t.Error("q at the drawer quit a remote session that had somewhere to go back to")
	}
}

// The replay returns to the files, not to a board the visitor never chose.
func TestTheBrowserCanReplayTheBootSequenceFromTheFiles(t *testing.T) {
	rs := NewRooms(7)
	rs.EnsureOpen(2)
	m := NewLocalSession(rs, 1, "mark")
	m.KeepOpen()
	m.Resize(120, 40)

	if !strings.Contains(m.View(), "r intro") {
		t.Error("the browser's file line does not offer the replay")
	}
	m.Update(key("r"))
	if m.Screen != ScreenIntro || m.Intro == nil {
		t.Fatalf("r on the files left the screen at %v, want the intro", m.Screen)
	}
	for i := 0; i < 60*30 && m.Screen == ScreenIntro; i++ {
		tick(m, 1)
	}
	if m.Screen != ScreenLobby || m.Room != nil {
		t.Fatalf("the replayed intro finished on %v, want the files", m.Screen)
	}
}

// The files fade up out of the dark each time the visitor comes back to them:
// after the boot sequence, replayed or not, and after leaving a file.
func TestTheFilesFadeUpEachTimeTheyComeBack(t *testing.T) {
	rs := NewRooms(7)
	rs.EnsureOpen(2)
	m := NewLocalSession(rs, 1, "mark")
	m.KeepOpen()
	m.Resize(120, 40)
	tick(m, 120)
	if lit := m.rolodex().Level(); lit < 0.95 {
		t.Fatalf("the files came up only %.2f of the way", lit)
	}

	m.Update(key("r"))
	for i := 0; i < 60*30 && m.Screen == ScreenIntro; i++ {
		tick(m, 1)
	}
	if lit := m.rolodex().Level(); lit > 0.5 {
		t.Errorf("after the replayed boot sequence the files were already %.2f lit", lit)
	}

	tick(m, 120)
	m.Update(key("enter"))
	if m.Room == nil {
		t.Fatal("enter did not open a file")
	}
	m.Update(key("q"))
	tick(m, 1)
	if m.Screen != ScreenLobby {
		t.Fatalf("q left the file for %v", m.Screen)
	}
	if lit := m.rolodex().Level(); lit > 0.5 {
		t.Errorf("back from a file the files were already %.2f lit", lit)
	}
}

// Remotely and at a terminal the boot sequence is once per connection, so r
// does nothing there and is not advertised.
func TestOnlyTheBrowserReplaysTheBootSequence(t *testing.T) {
	rs := NewRooms(7)
	rs.EnsureOpen(2)
	m := NewLocalSession(rs, 1, "mark")
	m.Resize(120, 40)

	m.Update(key("r"))
	if m.Screen != ScreenLobby {
		t.Fatalf("r moved a terminal session to %v", m.Screen)
	}
	if strings.Contains(m.View(), "r intro") {
		t.Error("a terminal session is told about a browser-only key")
	}
	for _, g := range Bindings() {
		for _, b := range g.Bindings {
			if b.Browser {
				t.Errorf("Bindings() includes the browser-only %q", b.Keys)
			}
		}
	}
}

// x asks, y answers, and anything else is a no.
func TestTheBrowserWipesItsProgressOnlyWhenTheRefinerSaysYes(t *testing.T) {
	rs := NewRooms(7)
	rs.EnsureOpen(2)
	m := NewLocalSession(rs, 1, "mark")
	m.KeepOpen()
	m.Resize(160, 40)

	if !strings.Contains(m.View(), "x reset") {
		t.Error("the browser's file line does not offer the reset")
	}
	m.Update(key("x"))
	if !strings.Contains(m.View(), "wipe all saved progress") {
		t.Fatal("x did not ask before wiping")
	}
	m.Update(key("n"))
	if m.WipeRequested() {
		t.Fatal("answering n wiped the progress")
	}
	if strings.Contains(m.View(), "wipe all saved progress") {
		t.Error("the question stayed up after it was answered")
	}
	if n := len(rs.List()); n != 2 {
		t.Errorf("the n that answered the question also opened a file: %d files", n)
	}
	m.Update(key("x"))
	m.Update(key("y"))
	if !m.WipeRequested() {
		t.Error("answering y did not ask the host to wipe")
	}
}

// At a terminal or remotely there is no browser save to wipe, so x does
// nothing and is not advertised.
func TestOnlyTheBrowserCanWipeProgress(t *testing.T) {
	rs := NewRooms(7)
	rs.EnsureOpen(2)
	m := NewLocalSession(rs, 1, "mark")
	m.Resize(160, 40)

	m.Update(key("x"))
	m.Update(key("y"))
	if m.WipeRequested() || strings.Contains(m.View(), "wipe all saved progress") {
		t.Error("a terminal session was offered a browser wipe")
	}
	if strings.Contains(m.View(), "x reset") {
		t.Error("a terminal session is told about a browser-only key")
	}
}

// The browser's files have nobody else on them, so it has no busiest file to
// go to: + and - do nothing there, and neither its hints nor its help offer
// them. A terminal still has both.
func TestTheBrowserHasNoBusiestFile(t *testing.T) {
	rs := NewRooms(7)
	m := NewLocalSession(rs, 1, "mark")
	m.KeepOpen()
	m.Resize(120, 40)
	tick(m, 10)
	m.Update(key("+"))
	if strings.Contains(screenText(m), "nobody is refining") {
		t.Error("+ in the browser's files says nobody is refining")
	}
	if strings.Contains(screenText(m), "busiest") {
		t.Error("the browser's files offer the busiest file")
	}
	m.Update(key("?"))
	if strings.Contains(screenText(m), busiestWhat) {
		t.Error("the browser's help offers the busiest file")
	}

	term := NewLocalSession(NewRooms(7), 1, "mark")
	term.OfferLAN(&fakeLAN{})
	term.Resize(120, 40)
	tick(term, 10)
	if !strings.Contains(screenText(term), "busiest") {
		t.Error("a terminal's files no longer offer the busiest file")
	}
	term.Update(key("?"))
	if !strings.Contains(screenText(term), busiestWhat) {
		t.Error("a terminal's help no longer offers the busiest file")
	}
}

func TestBusiestControlsFollowHumanMultiplayerAvailability(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*Model)
		want  bool
	}{
		{"contained", func(*Model) {}, false},
		{"contained-with-bots", func(m *Model) { m.AllowBots(3) }, false},
		{"local-with-LAN", func(m *Model) { m.OfferLAN(&fakeLAN{}) }, true},
		{"remote-session", func(m *Model) { m.askName = true }, true},
		{"LAN-joiner", func(m *Model) { m.joined = true }, true},
		{"browser", func(m *Model) { m.KeepOpen(); m.AllowBots(3) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewLocalSession(NewRooms(7), 1, "mark")
			tc.setup(m)
			m.Resize(120, 60)
			for view := lobbyView(0); view < lobbyViews; view++ {
				m.lobbyView = view
				if got := strings.Contains(m.lobbyHint(400), "busiest"); got != tc.want {
					t.Errorf("view %d: busiest hint = %v, want %v", view, got, tc.want)
				}
			}
			helpHasBusiest := false
			for _, group := range m.bindings() {
				for _, binding := range group.Bindings {
					helpHasBusiest = helpHasBusiest || binding.What == busiestWhat
				}
			}
			if helpHasBusiest != tc.want {
				t.Errorf("busiest help = %v, want %v", helpHasBusiest, tc.want)
			}
			if !tc.want {
				for _, k := range []string{"+", "=", "-", "_"} {
					before := m.rolodex().Want()
					m.handleLobbyKey(k)
					if m.notice != "" || m.rolodex().Want() != before {
						t.Errorf("unavailable key %q changed the lobby or showed a notice", k)
					}
				}
			}
		})
	}
}
