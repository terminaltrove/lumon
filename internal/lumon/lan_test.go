package lumon

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/terminaltrove/lumon/internal/canvas"
	"github.com/terminaltrove/lumon/internal/theme"
)

// fakeLAN stands in for the program's network side.
type fakeLAN struct {
	hosting  bool
	hostErr  error
	games    []LANGame
	looking  bool
	stopped  int
	hostings int
	hostName string
	waiting  []LANRequest
	answers  []string
	players  []LANPlayer
	kicked   []uint64
}

func (f *fakeLAN) Answer(id uint64, accept bool) {
	word := "deny"
	if accept {
		word = "accept"
	}
	for i, r := range f.waiting {
		if r.ID == id {
			f.answers = append(f.answers, word+" "+r.Name)
			f.waiting = append(f.waiting[:i], f.waiting[i+1:]...)
			return
		}
	}
}

func (f *fakeLAN) Host(name string) error {
	if f.hostErr != nil {
		return f.hostErr
	}
	f.hostName = name
	f.hosting = true
	f.hostings++
	return nil
}
func (f *fakeLAN) StopHosting()   { f.hosting = false; f.stopped++ }
func (f *fakeLAN) Kick(id uint64) { f.kicked = append(f.kicked, id) }
func (f *fakeLAN) Hosting() (LANHosting, bool) {
	return LANHosting{File: "SIENA", Addr: "192.168.1.5:2424", Joined: 1, Waiting: f.waiting, Players: f.players}, f.hosting
}
func (f *fakeLAN) Games() []LANGame { f.looking = true; return f.games }
func (f *fakeLAN) StopLooking()     { f.looking = false }

func lanLobby(t *testing.T) (*Model, *fakeLAN) {
	t.Helper()
	rooms := NewRooms(1)
	rooms.EnsureOpen(1)
	m := NewLocalSession(rooms, 1, "host")
	lan := &fakeLAN{}
	m.OfferLAN(lan)
	return m, lan
}

func lanPress(m *Model, keys ...string) bool {
	quit := false
	for _, k := range keys {
		msg := KeyMsg{Key: k}
		if len([]rune(k)) == 1 {
			msg.Runes = []rune(k)
		}
		if m.handleKey(msg) {
			quit = true
		}
	}
	return quit
}

func TestOnlyALocalPlayerIsOfferedLAN(t *testing.T) {
	rooms := NewRooms(1)
	rooms.EnsureOpen(1)

	// Somebody who came in over a connection, like a LAN game's own joiner.
	remote := NewSession(rooms, 1)
	remote.Screen = ScreenLobby
	lanPress(remote, "l")
	if remote.lanUI.step != lanClosed {
		t.Fatal("a remote session opened the LAN panel")
	}
	if strings.Contains(remote.lobbyHint(200), "LAN") {
		t.Fatal("a remote session's hint offers LAN")
	}
	for _, group := range remote.bindings() {
		for _, b := range group.Bindings {
			if b.LAN {
				t.Fatal("a remote session's controls list LAN")
			}
		}
	}

	// A local player on a build with LAN off has no LAN either.
	off := NewLocalSession(rooms, 1, "off")
	lanPress(off, "l")
	if off.lanUI.step != lanClosed || strings.Contains(off.lobbyHint(200), "LAN") {
		t.Fatal("a build with LAN off offered it")
	}
}

func TestTheBrowserIsNotOfferedLAN(t *testing.T) {
	for _, group := range browserBindings {
		for _, b := range group.Bindings {
			if b.LAN {
				t.Fatal("the browser's controls list LAN")
			}
		}
	}
}

func TestALocalPlayerSeesLANInTheControlsAndTheHint(t *testing.T) {
	m, _ := lanLobby(t)
	if !strings.Contains(m.lobbyHint(200), "l LAN") {
		t.Fatal("the hint does not say l LAN")
	}
	found := false
	for _, group := range m.bindings() {
		for _, b := range group.Bindings {
			found = found || b.LAN
		}
	}
	if !found {
		t.Fatal("the controls do not list LAN")
	}
}

func TestHostingFromThePanel(t *testing.T) {
	m, lan := lanLobby(t)
	lanPress(m, "l", "enter", "enter") // host, keeping the name
	if lan.hostName != "HOST" {
		t.Errorf("hosted as %q", lan.hostName)
	}
	if !lan.hosting || m.lanUI.step != lanHosted {
		t.Fatalf("hosting=%v step=%v", lan.hosting, m.lanUI.step)
	}

	// Back to the files, still hosting; l comes back to the running game.
	lanPress(m, "enter")
	if m.lanUI.step != lanClosed || !lan.hosting {
		t.Fatal("closing the panel stopped hosting")
	}
	lanPress(m, "l")
	if m.lanUI.step != lanHosted {
		t.Fatal("l did not return to the running game")
	}
	lanPress(m, "x")
	if lan.hosting || m.lanUI.step != lanMenu {
		t.Fatal("x did not stop hosting")
	}
}

func TestAHostingFailureIsSaidAndTheNameStays(t *testing.T) {
	m, lan := lanLobby(t)
	lan.hostErr = errors.New("port in use")
	lanPress(m, "l", "enter", "enter")
	if m.lanUI.step != lanTypingHostName || m.lanUI.note != "Port in use" {
		t.Fatalf("step=%v note=%q", m.lanUI.step, m.lanUI.note)
	}
}

func TestJoiningAGameThatWasFound(t *testing.T) {
	m, lan := lanLobby(t)
	lan.games = []LANGame{{Host: "A", Addr: "192.168.1.5:2424"}, {Host: "B", Addr: "192.168.1.6:2424"}}
	if lanPress(m, "l", "down", "enter", "down", "enter") {
		t.Fatal("quit before the name was confirmed")
	}
	if m.lanUI.step != lanTypingName || m.lanUI.name != m.Name {
		t.Fatalf("step=%v name=%q", m.lanUI.step, m.lanUI.name)
	}
	if !lanPress(m, "2", "enter") {
		t.Fatal("enter on a name did not end the program")
	}
	got, ok := m.TakeLANJoin()
	if !ok || got != (LANTarget{Addr: "192.168.1.6:2424", Name: "HOST2"}) {
		t.Fatalf("join = %+v, %v", got, ok)
	}
	if _, again := m.TakeLANJoin(); again {
		t.Fatal("the join was handed over twice")
	}
	if lan.looking {
		t.Fatal("still looking for games after choosing one")
	}
}

func TestJoiningByAddress(t *testing.T) {
	m, _ := lanLobby(t)
	lanPress(m, "l", "down", "enter", "a")
	if m.lanUI.step != lanTypingAddr {
		t.Fatalf("step=%v", m.lanUI.step)
	}
	// Enter on nothing goes nowhere.
	lanPress(m, "enter")
	if m.lanUI.step != lanTypingAddr {
		t.Fatal("an empty address moved on")
	}
	lanPress(m, "1", ".", "2", "x", "backspace", "enter", "enter")
	got, ok := m.TakeLANJoin()
	if !ok || got != (LANTarget{Addr: "1.2", Name: "HOST"}) {
		t.Fatalf("join = %+v, %v", got, ok)
	}
}

func TestAFailedJoinComesBackSayingWhy(t *testing.T) {
	m, _ := lanLobby(t)
	m.Quitting = true
	m.LANJoinEnded(errors.New("the host said no"))
	if m.Quitting || m.lanUI.step != lanFinding || m.lanUI.note != "The host said no" {
		t.Fatalf("quitting=%v step=%v note=%q", m.Quitting, m.lanUI.step, m.lanUI.note)
	}
	m.LANJoinEnded(nil)
	if m.lanUI.step != lanClosed {
		t.Fatal("a clean end left the panel open")
	}
}

func TestThePanelTakesTheKeysButNotCtrlC(t *testing.T) {
	m, _ := lanLobby(t)
	lanPress(m, "l", "esc")
	if m.Escaped || m.lanUI.step != lanClosed {
		t.Fatal("esc in the panel should close it, not show the notice")
	}
	lanPress(m, "l", "?", "a")
	if m.Helping || m.About {
		t.Fatal("keys reached behind the panel")
	}
	if !lanPress(m, "ctrl+c") {
		t.Fatal("ctrl+c did not quit")
	}
}

func TestTheMouseDoesNotReachBehindTheLANPanel(t *testing.T) {
	m, _ := lanLobby(t)
	lanPress(m, "l")
	before := m.Room
	m.handleMouseEvent(MouseMsg{X: 10, Y: 10, Button: MouseLeft, Action: MousePress})
	if m.Room != before || m.lanUI.step != lanMenu {
		t.Fatal("a click got through the panel")
	}
}

func TestThePanelDrawsAtEveryStepAndEverySize(t *testing.T) {
	m, lan := lanLobby(t)
	lan.games = []LANGame{{Host: "\x1b[31mevil\x07", Addr: "192.168.1.5:2424"}}
	lan.hosting = true
	steps := []lanStep{lanMenu, lanHosted, lanFinding, lanTypingAddr, lanTypingName}
	for _, size := range [][2]int{{200, 60}, {80, 24}, {40, 12}, {10, 3}, {1, 1}} {
		buf := canvas.NewBuffer(size[0], size[1])
		for _, step := range steps {
			m.lanUI.step = step
			m.drawLAN(buf)
			m.drawLANBanner(buf)
		}
	}
}

func TestANameFromTheNetworkIsTidied(t *testing.T) {
	if got := tidyName("\x1b[31mevil\x07"); strings.ContainsAny(got, "\x1b\x07[") {
		t.Fatalf("tidyName let through %q", got)
	}
}

func TestOfferingLANClaimsTheHostsName(t *testing.T) {
	m, _ := lanLobby(t)
	joiner := NewSession(m.rooms, 2)
	joiner.Name = m.Name
	if joiner.claimName() {
		t.Fatal("a joiner took the host's name")
	}
}

func TestAnEmptyNameGoesNowhere(t *testing.T) {
	m, lan := lanLobby(t)
	lan.games = []LANGame{{Host: "A", Addr: "192.168.1.5:2424"}}
	lanPress(m, "l", "down", "enter", "enter")
	for range NameLimit {
		lanPress(m, "backspace")
	}
	if lanPress(m, "enter") {
		t.Fatal("asked to join with no name")
	}
	if _, ok := m.TakeLANJoin(); ok {
		t.Fatal("a join with no name was handed over")
	}
}

func TestNamesAndAddressesTakeOnlyTheirCharacters(t *testing.T) {
	key := func(s string) KeyMsg { return KeyMsg{Key: s, Runes: []rune(s)} }
	if got := typeLANField("", key("aB1 .-\x1b[0m"), true); got != "AB1 .-0M" {
		t.Fatalf("name = %q", got)
	}
	if got := typeLANField("", key("fe80::1%en0"), false); got != "fe80::1%en0" {
		t.Fatalf("address = %q", got)
	}
	if got := typeLANField("", key("a b\x07"), false); got != "ab" {
		t.Fatalf("address = %q", got)
	}
	if got := typeLANField(strings.Repeat("A", NameLimit), key("B"), true); len(got) != NameLimit {
		t.Fatalf("name over the limit: %d", len(got))
	}
	if got := typeLANField(strings.Repeat("A", lanFieldLimit), key("B"), false); len(got) != lanFieldLimit {
		t.Fatalf("address over the limit: %d", len(got))
	}
}

// hostWithRequest is a host with somebody asking, past the guard on its keys.
func hostWithRequest(t *testing.T) (*Model, *fakeLAN) {
	t.Helper()
	m, lan := lanLobby(t)
	lan.hosting = true
	lan.waiting = []LANRequest{{ID: 1, Name: "JOE", Addr: "192.168.1.12"}, {ID: 2, Name: "ANN", Addr: "192.168.1.13"}}
	m.lanRequest()
	m.lanShownAt = time.Now().Add(-time.Minute)
	return m, lan
}

func TestTheHostAnswersOldestFirstWithYAndN(t *testing.T) {
	m, lan := hostWithRequest(t)
	lanPress(m, "y")
	if len(lan.answers) != 1 || lan.answers[0] != "accept JOE" {
		t.Fatalf("answers = %v", lan.answers)
	}

	// The next one has its own guard, so a doubled key does not answer it.
	lanPress(m, "n")
	if len(lan.answers) != 1 {
		t.Fatalf("the key went through the guard: %v", lan.answers)
	}
	m.lanShownAt = time.Now().Add(-time.Minute)
	lanPress(m, "n")
	if len(lan.answers) != 2 || lan.answers[1] != "deny ANN" {
		t.Fatalf("answers = %v", lan.answers)
	}
}

func TestANewRequestIgnoresItsKeysAtFirst(t *testing.T) {
	m, lan := lanLobby(t)
	lan.hosting = true
	lan.waiting = []LANRequest{{ID: 1, Name: "JOE", Addr: "192.168.1.12"}}
	lanPress(m, "n")
	if len(lan.answers) != 0 {
		t.Fatal("a key pressed as the request arrived answered it")
	}
	if m.Room != nil {
		t.Fatal("the key went on to the files")
	}
}

func TestYAndNDoNothingWithNoRequest(t *testing.T) {
	m, lan := lanLobby(t)
	lan.hosting = true
	if m.answerLANRequest("y") || m.answerLANRequest("n") {
		t.Fatal("y or n was taken with nobody asking")
	}
	lanPress(m, "n")
	if m.Room == nil {
		t.Fatal("n no longer opens a new file")
	}
}

func TestOtherKeysWorkWhileARequestShows(t *testing.T) {
	m, lan := hostWithRequest(t)
	lanPress(m, "?")
	if !m.Helping || len(lan.answers) != 0 {
		t.Fatalf("helping=%v answers=%v", m.Helping, lan.answers)
	}
	// Over the controls too, and without closing them.
	lanPress(m, "y")
	if !m.Helping || len(lan.answers) != 1 {
		t.Fatalf("helping=%v answers=%v", m.Helping, lan.answers)
	}
}

func TestTheWipeQuestionKeepsItsKeys(t *testing.T) {
	m, lan := hostWithRequest(t)
	m.wipeAsk = true
	if m.answerLANRequest("y") {
		t.Fatal("a request took the wipe question's y")
	}
	buf := canvas.NewBuffer(120, 10)
	m.drawLANRequest(buf)
	if strings.Contains(bufferText(buf), "wants to join") {
		t.Fatal("the request showed over the wipe question")
	}
	if len(lan.answers) != 0 {
		t.Fatal("answered")
	}
}

func TestAJoinerNeverSeesARequestAndHasNoLAN(t *testing.T) {
	rooms := NewRooms(1)
	rooms.EnsureOpen(1)
	j := NewJoinedSession(rooms, 1, "JOE")
	if j.lan != nil {
		t.Fatal("a joiner has a LAN")
	}
	lanPress(j, "l")
	if j.lanUI.step != lanClosed {
		t.Fatal("a joiner opened the LAN panel")
	}
	if j.answerLANRequest("y") {
		t.Fatal("a joiner answered a request")
	}
	buf := canvas.NewBuffer(120, 10)
	j.drawLANRequest(buf)
	if strings.Contains(bufferText(buf), "wants to join") {
		t.Fatal("a joiner saw a request")
	}
}

func TestAJoinerGoesStraightToTheFilesWithTheHintsOfARemotePlayer(t *testing.T) {
	rooms := NewRooms(1)
	rooms.EnsureOpen(1)
	j := NewJoinedSession(rooms, 1, "joe")
	if j.Screen != ScreenLobby || j.Name != "JOE" {
		t.Fatalf("screen=%v name=%q", j.Screen, j.Name)
	}
	hint := j.lobbyHint(200)
	if !strings.Contains(hint, "q back") || strings.Contains(hint, "company") {
		t.Fatalf("hint = %q", hint)
	}
	if j.backOut() {
		t.Fatal("q from a joiner's files went somewhere in the game")
	}
	if j.Screen == ScreenName {
		t.Fatal("a joiner was sent to a name screen")
	}
}

func TestATakenNameIsNumberedAndStaysInTheLimit(t *testing.T) {
	rooms := NewRooms(1)
	rooms.EnsureOpen(1)
	long := strings.Repeat("A", NameLimit)
	a := NewJoinedSession(rooms, 1, long)
	b := NewJoinedSession(rooms, 2, long)
	c := NewJoinedSession(rooms, 3, long)
	if a.Name != long || b.Name != long[:NameLimit-2]+"-2" || c.Name != long[:NameLimit-2]+"-3" {
		t.Fatalf("names %q %q %q", a.Name, b.Name, c.Name)
	}
	b.Close()
	if d := NewJoinedSession(rooms, 4, long); d.Name != long[:NameLimit-2]+"-2" {
		t.Fatalf("the name was not freed: %q", d.Name)
	}
	if e := NewJoinedSession(rooms, 5, "\x1b\x07"); e.Name != "REFINER" {
		t.Fatalf("no usable name: %q", e.Name)
	}
}

func TestAJoinerCannotTakeTheHostsName(t *testing.T) {
	m, _ := lanLobby(t)
	if j := NewJoinedSession(m.rooms, 2, m.Name); j.Name != m.Name+"-2" {
		t.Fatalf("name = %q", j.Name)
	}
}

func TestTheRequestDrawsOverEveryScreenAndEverySize(t *testing.T) {
	m, _ := hostWithRequest(t)
	for _, size := range [][2]int{{200, 60}, {80, 24}, {40, 12}, {10, 3}, {1, 1}} {
		m.Resize(size[0], size[1])
		for _, open := range []func(){
			func() {},
			func() { m.Helping = true },
			func() { m.About = true },
			func() { m.Escaped = true },
			func() { m.agentNote = "note" },
			func() { m.lanUI.step = lanHosted },
		} {
			open()
			buf := canvas.NewBuffer(size[0], size[1])
			m.Draw(buf)
			if size[0] >= 40 && !strings.Contains(bufferText(buf), "wants to join") {
				t.Fatalf("no request at %v", size)
			}
		}
		m.Helping, m.About, m.Escaped, m.agentNote, m.lanUI.step = false, false, false, "", lanClosed
	}
}

func TestTheRequestSaysWhoAndHowManyMore(t *testing.T) {
	m, _ := hostWithRequest(t)
	buf := canvas.NewBuffer(120, 3)
	m.drawLANRequest(buf)
	s := bufferText(buf)
	for _, want := range []string{"JOE", "192.168.1.12", "y accept", "n deny", "1 more waiting"} {
		if !strings.Contains(s, want) {
			t.Fatalf("%q not in %q", want, s)
		}
	}
}

func TestAGameIsNamedForWhereItsHostIs(t *testing.T) {
	rooms := NewRooms(1)
	rooms.EnsureOpen(2)
	if got := rooms.GameName("alex"); got != "ALEX's files" {
		t.Fatalf("nobody on a file: %q", got)
	}
	list := rooms.List()
	other := NewLocalSession(rooms, 1, "ann")
	if !other.Join(rooms.Find(list[0].Name)) {
		t.Fatal("could not seat")
	}
	if got, want := rooms.GameName("alex"), CleanLANFile(list[0].Name); got != want {
		t.Fatalf("the busiest: %q, want %q", got, want)
	}
	host := NewLocalSession(rooms, 2, "alex")
	if !host.Join(rooms.Find(list[1].Name)) {
		t.Fatal("could not seat")
	}
	if got, want := rooms.GameName("alex"), CleanLANFile(list[1].Name); got != want {
		t.Fatalf("the host's file: %q, want %q", got, want)
	}
}

func TestAGameNameFromTheNetworkIsCleaned(t *testing.T) {
	for in, want := range map[string]string{
		"Sunset Park":            "Sunset Park",
		"ALEX's files":           "ALEX's files",
		"\x1b[31mevil\x07":       "31mevil",
		"Le Mars · ünïcode":      "Le Mars  ncode",
		strings.Repeat("x", 100): strings.Repeat("x", lanFileLimit),
		"":                       "",
	} {
		if got := CleanLANFile(in); got != want {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
}

func TestAGameLineHasEverything(t *testing.T) {
	got := lanGameLine(LANGame{File: "Minsk", Host: "alex", Addr: "192.168.1.5:2424", Players: 2})
	if got != "Minsk  ALEX  2 refiners  192.168.1.5:2424" {
		t.Fatalf("line = %q", got)
	}
	if got := lanGameLine(LANGame{Addr: "a", Players: 1}); got != "A game  REFINER  1 refiner  a" {
		t.Fatalf("line = %q", got)
	}
}

// bufferText is what a buffer says. A test has no terminal, so no colour.
func bufferText(buf *canvas.Buffer) string { return buf.String() }

// contrast is the WCAG contrast ratio of two colours.
func contrast(a, b canvas.Color) float64 {
	lum := func(c canvas.Color) float64 {
		r, g, bl, _ := c.Parts()
		line := func(v uint8) float64 {
			s := float64(v) / 255
			if s <= 0.03928 {
				return s / 12.92
			}
			return math.Pow((s+0.055)/1.055, 2.4)
		}
		return 0.2126*line(r) + 0.7152*line(g) + 0.0722*line(bl)
	}
	la, lb := lum(a), lum(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// A message has to be read. Bad is for digits that lurk in the board and BadLive
// for what somebody is being told, on every theme.
func TestMessagesCanBeReadOnEveryTheme(t *testing.T) {
	// Themes with no paper of their own sit on the terminal's background, so a
	// lighter dark one is the hard case.
	dark := canvas.RGB(0x2B, 0x30, 0x38)
	for _, name := range theme.ThemeNames() {
		th, ok := theme.FindTheme(name)
		if !ok {
			t.Fatalf("no theme %q", name)
		}
		bg := dark
		if th.Paper.Set() {
			bg = th.Paper
		}
		if got := contrast(th.BadLive.FG, bg); got < 4.5 {
			t.Errorf("%s: messages are %.1f:1 against the background", name, got)
		}
		if got := contrast(th.Hint.FG, bg); got < 4.5 {
			t.Errorf("%s: hints and news are %.1f:1 against the background", name, got)
		}
		if got := contrast(th.Footer.FG, bg); got < 4.5 {
			t.Errorf("%s: footer text is %.1f:1 against the background", name, got)
		}
		// Muted still has to be muted: it should not read as the hint does.
		if th.Footer.FG == th.Hint.FG {
			t.Errorf("%s: footer text is as bright as the hint", name)
		}
	}
}

func TestTheLANNoteUsesTheColourForMessages(t *testing.T) {
	m, _ := lanLobby(t)
	m.LANJoinEnded(errors.New("the host said no"))
	buf := canvas.NewBuffer(120, 40)
	m.drawLAN(buf)
	x, y := findText(t, buf, "The host said no")
	if got := buf.AttrAt(x, y); got != m.Theme.BadLive {
		t.Fatalf("note drawn in %+v", got)
	}
}

func TestTheFilesScreenNoticeUsesTheColourForMessages(t *testing.T) {
	m, _ := lanLobby(t)
	m.Resize(120, 40)
	m.notice = "wipe all saved progress and start again? y / n"
	buf := canvas.NewBuffer(120, 40)
	m.Draw(buf)
	x, y := findText(t, buf, "wipe all saved")
	if got := buf.AttrAt(x, y); got != m.Theme.BadLive {
		t.Fatalf("notice drawn in %+v", got)
	}
}

// findText is where some text starts on a buffer.
func findText(t *testing.T, buf *canvas.Buffer, want string) (int, int) {
	t.Helper()
	for y, line := range strings.Split(buf.String(), "\n") {
		if x := strings.Index(line, want); x >= 0 {
			return len([]rune(line[:x])), y
		}
	}
	t.Fatalf("%q is not on the screen", want)
	return 0, 0
}

func TestTheFloorNewsCanBeRead(t *testing.T) {
	m, _ := lanLobby(t)
	m.Resize(120, 40)
	m.rooms.news.Post("ANN joined Narva")
	buf := canvas.NewBuffer(120, 40)
	m.drawFloorNews(buf)
	x, y := findText(t, buf, "FLOOR NEWS")
	if got := buf.AttrAt(x, y); got != m.newsAttr() || got.FG != m.Theme.Chrome.FG || got.BG.Set() {
		t.Fatalf("news drawn in %+v", got)
	}
}

func TestANameThatLooksLikeAnAddressIsNotUsed(t *testing.T) {
	for _, name := range []string{"192.168.1.12", "1.2", "10.0.0.1"} {
		if got := LANName(name); got != "REFINER" {
			t.Errorf("%s = %s", name, got)
		}
	}
	for _, name := range []string{"007", "J.R.", "AGENT-7"} {
		if got := LANName(name); got != name {
			t.Errorf("%s = %s", name, got)
		}
	}
}

func TestTheAddressIsTheLastThingTheRequestLoses(t *testing.T) {
	r := LANRequest{ID: 1, Name: "192.168.1.1X", Addr: "fd12:3456:789a:1:2:3:4:5"}
	for w := len(r.Addr) + 5; w <= 120; w++ {
		if got := lanRequestLine(r, 2, w); !strings.Contains(got, r.Addr) || len([]rune(got)) > w {
			t.Fatalf("width %d: %q", w, got)
		}
	}
	if got := lanRequestLine(r, 0, 3); len([]rune(got)) > 3 {
		t.Fatalf("over the width: %q", got)
	}
}

func TestTheHostRemovesAPlayerFromThePanel(t *testing.T) {
	m, lan := lanLobby(t)
	lan.hosting = true
	lan.players = []LANPlayer{{ID: 7, Name: "ANN", Addr: "192.168.1.8"}, {ID: 9, Name: "JOE", Addr: "192.168.1.9"}}
	lanPress(m, "l")
	if m.lanUI.step != lanHosted {
		t.Fatalf("step=%v", m.lanUI.step)
	}
	buf := canvas.NewBuffer(120, 40)
	m.drawLAN(buf)
	if s := bufferText(buf); !strings.Contains(s, "ANN") || !strings.Contains(s, "192.168.1.9") || !strings.Contains(s, "r remove") {
		t.Fatalf("not listed:\n%s", s)
	}
	lanPress(m, "down", "down", "down", "r")
	if len(lan.kicked) != 1 || lan.kicked[0] != 9 {
		t.Fatalf("kicked = %v", lan.kicked)
	}
	lanPress(m, "up", "up", "up", "up", "r")
	if len(lan.kicked) != 2 || lan.kicked[1] != 7 {
		t.Fatalf("kicked = %v", lan.kicked)
	}

	// With nobody there, r does nothing.
	lan.players, lan.kicked = nil, nil
	lanPress(m, "r")
	if len(lan.kicked) != 0 {
		t.Fatalf("kicked = %v", lan.kicked)
	}
	m.drawLAN(canvas.NewBuffer(120, 40))
}

func TestThePlayerListFitsAnySize(t *testing.T) {
	m, lan := lanLobby(t)
	lan.hosting = true
	for i := range 8 {
		lan.players = append(lan.players, LANPlayer{ID: uint64(i + 1), Name: "ANN", Addr: "192.168.1.8"})
	}
	m.lanUI.step = lanHosted
	for _, size := range [][2]int{{200, 60}, {80, 24}, {40, 12}, {10, 3}, {1, 1}} {
		for _, pick := range []int{0, 3, 7, 20} {
			m.lanUI.pick = pick
			m.drawLAN(canvas.NewBuffer(size[0], size[1]))
		}
	}
}

func TestANewGameCannotMoveTheChoice(t *testing.T) {
	m, lan := lanLobby(t)
	lan.games = []LANGame{{Host: "A", Addr: "192.168.1.5:2424"}, {Host: "B", Addr: "192.168.1.6:2424"}}
	lanPress(m, "l", "down", "enter", "down")
	if m.lanUI.pickAddr != "192.168.1.6:2424" {
		t.Fatalf("pick = %q", m.lanUI.pickAddr)
	}
	// A game turns up above the one chosen, just before enter.
	lan.games = []LANGame{{Host: "X", Addr: "192.168.1.1:2424"}, lan.games[0], lan.games[1]}
	lanPress(m, "enter")
	if m.lanUI.addr != "192.168.1.6:2424" {
		t.Fatalf("addr = %q", m.lanUI.addr)
	}

	// The choice going away falls back on something still listed.
	m2, lan2 := lanLobby(t)
	lan2.games = []LANGame{{Addr: "a"}, {Addr: "b"}, {Addr: "c"}}
	lanPress(m2, "l", "down", "enter", "down", "down")
	lan2.games = lan2.games[:1]
	m2.drawLAN(canvas.NewBuffer(120, 40))
	if m2.lanUI.pick != 0 || m2.lanUI.pickAddr != "a" {
		t.Fatalf("pick=%d addr=%q", m2.lanUI.pick, m2.lanUI.pickAddr)
	}
}

// The request and the news share the rule over the grid. The request is the one somebody is
// waiting on, so it covers the news, and its keys beat n for hiding it.
func TestALANRequestCoversTheNewsAndStillTakesItsKeys(t *testing.T) {
	m, lan := lanLobby(t)
	m.Resize(120, 40)
	if !m.Join(m.rooms.Open()) {
		t.Fatal("no desk")
	}
	if got := drawTo(m).Row(m.Layout.GridTop - 2); !strings.Contains(got, "FLOOR NEWS") {
		t.Fatalf("no news on the top row: %q", got)
	}

	lan.hosting = true
	lan.waiting = []LANRequest{{ID: 1, Name: "JOE", Addr: "192.168.1.9"}}
	got := drawTo(m).Row(m.Layout.GridTop - 2)
	if !strings.Contains(got, "JOE 192.168.1.9 wants to join") {
		t.Fatalf("the request is not on the top row: %q", got)
	}
	if strings.Contains(got, "FLOOR NEWS") || strings.Contains(got, "LUMON NOTICE") {
		t.Errorf("the news shows through the request: %q", got)
	}

	// Past the guard, n denies; it does not hide the news.
	m.lanShownAt = time.Now().Add(-lanRequestGuard - time.Second)
	lanPress(m, "n")
	if len(lan.answers) != 1 || lan.answers[0] != "deny JOE" {
		t.Errorf("answers = %v", lan.answers)
	}
	if m.newsHidden {
		t.Error("answering the request hid the news")
	}

	// A hidden news line does not hide a request.
	m.newsHidden = true
	lan.waiting = []LANRequest{{ID: 2, Name: "ANN", Addr: "192.168.1.8"}}
	if got := drawTo(m).Row(m.Layout.GridTop - 2); !strings.Contains(got, "ANN 192.168.1.8") {
		t.Errorf("hiding the news hid a request: %q", got)
	}
}

// With no rule to sit in, on the files for one, a request is at the very top.
func TestALANRequestOffTheBoardIsAtTheTop(t *testing.T) {
	m, lan := lanLobby(t)
	m.Resize(120, 40)
	lan.hosting = true
	lan.waiting = []LANRequest{{ID: 1, Name: "JOE", Addr: "192.168.1.9"}}
	buf := canvas.NewBuffer(120, 40)
	m.Draw(buf)
	if got := buf.Row(0); !strings.Contains(got, "JOE 192.168.1.9 wants to join") {
		t.Errorf("the top row is %q", got)
	}
}

// The status row sits between the rule's two lines at every size, and a request
// fits there.
func TestTheStatusRowIsBetweenTheRulesAtEverySize(t *testing.T) {
	for _, size := range [][2]int{{200, 60}, {120, 40}, {80, 24}, {60, 18}, {40, 12}} {
		m, lan := lanLobby(t)
		m.Resize(size[0], size[1])
		if !m.Join(m.rooms.Open()) {
			t.Fatal("no desk")
		}
		y := m.Layout.GridTop - 2
		if got := drawTo(m).Row(y - 1); !strings.Contains(got, "═") {
			t.Errorf("%v: the row above the status row is %q", size, got)
		}
		if got := drawTo(m).Row(y + 1); !strings.Contains(got, "═") {
			t.Errorf("%v: the row below the status row is %q", size, got)
		}
		lan.hosting = true
		lan.waiting = []LANRequest{{ID: 1, Name: "JOE", Addr: "192.168.1.9"}}
		if got := drawTo(m).Row(y); !strings.Contains(got, "192.168.1.9") {
			t.Errorf("%v: the status row is %q", size, got)
		}
	}
}

// The host chooses the name they play under, filled in with the one they have.
func TestTheHostChoosesTheirName(t *testing.T) {
	m, lan := lanLobby(t)
	lanPress(m, "l", "enter")
	if m.lanUI.step != lanTypingHostName || m.lanUI.name != "HOST" || lan.hosting {
		t.Fatalf("step=%v name=%q hosting=%v", m.lanUI.step, m.lanUI.name, lan.hosting)
	}
	if !strings.Contains(bufferText(drawnLAN(m)), "Your name, for this game") {
		t.Error("the name box is not asked for")
	}
	lanPress(m, "backspace", "backspace", "backspace", "backspace", "j", "o", "e", "enter")
	if !lan.hosting || lan.hostName != "JOE" || m.Name != "JOE" || m.lanUI.step != lanHosted {
		t.Fatalf("hosting=%v as %q, name=%q, step=%v", lan.hosting, lan.hostName, m.Name, m.lanUI.step)
	}

	// The old name is given back to the floor, and the new one is held.
	other := NewLocalSession(m.rooms, 2, "host")
	if !other.claimName() {
		t.Error("the old name is still held")
	}
	other.Name = "JOE"
	if other.claimName() {
		t.Error("the new name is not held")
	}
}

// The name is the host's everywhere: the board they sit at shows it.
func TestTheHostsNewNameIsOnTheBoard(t *testing.T) {
	m, _ := lanLobby(t)
	m.Resize(120, 40)
	lanPress(m, "l", "enter", "backspace", "backspace", "backspace", "backspace", "k", "i", "e", "r", "enter")
	lanPress(m, "enter") // back to the files
	if !m.Join(m.rooms.Open()) {
		t.Fatal("no desk")
	}
	if m.Desk.Name != "KIER" {
		t.Errorf("the desk is %q", m.Desk.Name)
	}
}

func TestAHostNameMustBeUsable(t *testing.T) {
	m, lan := lanLobby(t)
	lanPress(m, "l", "enter", "backspace", "backspace", "backspace", "backspace", "enter")
	if lan.hosting || m.lanUI.step != lanTypingHostName {
		t.Fatal("an empty name started hosting")
	}

	lanPress(m, "1", ".", "2", "enter")
	if lan.hosting || m.Name != "HOST" || !strings.Contains(m.lanUI.note, "address") {
		t.Fatalf("an address-like name: hosting=%v name=%q note=%q", lan.hosting, m.Name, m.lanUI.note)
	}

	// Somebody on the floor has it.
	ann := NewLocalSession(m.rooms, 2, "ann")
	if !ann.claimName() {
		t.Fatal("setup: ann")
	}
	m.lanUI.name = ""
	lanPress(m, "a", "n", "n", "enter")
	if lan.hosting || m.Name != "HOST" || m.lanUI.note != "That name is already on the floor" {
		t.Fatalf("a taken name: hosting=%v name=%q note=%q", lan.hosting, m.Name, m.lanUI.note)
	}

	lanPress(m, "esc")
	if m.lanUI.step != lanMenu {
		t.Errorf("esc went to %v", m.lanUI.step)
	}
}

// If hosting fails the player keeps the name they had.
func TestAFailedHostKeepsTheOldName(t *testing.T) {
	m, lan := lanLobby(t)
	lan.hostErr = errors.New("could not host")
	lanPress(m, "l", "enter", "backspace", "backspace", "backspace", "backspace", "m", "e", "enter")
	if m.Name != "HOST" || m.lanUI.note != "Could not host" {
		t.Fatalf("name=%q note=%q", m.Name, m.lanUI.note)
	}
	other := NewLocalSession(m.rooms, 2, "me")
	if !other.claimName() {
		t.Error("the name that was not used is still held")
	}
}

func drawnLAN(m *Model) *canvas.Buffer {
	buf := canvas.NewBuffer(120, 40)
	m.drawLAN(buf)
	return buf
}

// m opens the LAN panel from the board, where l moves the cursor.
func TestMOpensTheLANPanelOnTheBoard(t *testing.T) {
	m, _ := lanLobby(t)
	m.Resize(120, 40)
	if !m.Join(m.rooms.Open()) {
		t.Fatal("no desk")
	}
	x := m.CursorX
	m.Update(key("l"))
	if m.lanUI.step != lanClosed || m.CursorX != x+1 {
		t.Fatalf("l on the board: step=%v cursor %d to %d", m.lanUI.step, x, m.CursorX)
	}
	m.Update(key("m"))
	if m.lanUI.step != lanMenu {
		t.Fatalf("m gave step %v", m.lanUI.step)
	}
	if !strings.Contains(bufferText(drawTo(m)), "Host a LAN game") {
		t.Error("the panel is not drawn over the board")
	}
}

// A terminal without LAN does nothing with m, and is not offered it.
func TestMIsNothingWithoutLAN(t *testing.T) {
	rooms := NewRooms(1)
	m := NewLocalSession(rooms, 1, "mark")
	m.Resize(120, 40)
	if !m.Join(rooms.Open()) {
		t.Fatal("no desk")
	}
	m.Update(key("m"))
	if m.lanUI.step != lanClosed {
		t.Errorf("m opened %v with no LAN", m.lanUI.step)
	}
	for _, g := range Bindings() {
		for _, b := range g.Bindings {
			if b.Keys == "m" {
				t.Error("the controls offer m with no LAN")
			}
		}
	}
	m.Helping = true
	if strings.Contains(bufferText(drawTo(m)), "host a LAN game") {
		t.Error("the help offers LAN with no LAN")
	}
}

// Hosting from the board under a new name sits the host back down under it.
func TestHostingFromTheBoardUnderANewNameSitsAgain(t *testing.T) {
	m, lan := lanLobby(t)
	m.Resize(120, 40)
	room := m.rooms.Open()
	if !m.Join(room) {
		t.Fatal("no desk")
	}
	m.Update(key("m"))
	m.Update(key("enter")) // host
	for range 4 {
		m.Update(key("backspace"))
	}
	for _, k := range []string{"k", "i", "e", "r", "enter"} {
		m.Update(key(k))
	}
	if !lan.hosting || lan.hostName != "KIER" {
		t.Fatalf("hosting=%v as %q", lan.hosting, lan.hostName)
	}
	if m.Room != room || m.Desk == nil || m.Desk.Name != "KIER" || m.Screen != ScreenMain {
		t.Fatalf("room same=%v desk=%+v screen=%v", m.Room == room, m.Desk, m.Screen)
	}
	for _, d := range room.Roster() {
		if d.Name == "HOST" {
			t.Error("a desk is still held under the old name")
		}
	}
}

// Joining somebody else's game from the board gives the desk up.
func TestJoiningFromTheBoardFreesTheDesk(t *testing.T) {
	m, lan := lanLobby(t)
	m.Resize(120, 40)
	room := m.rooms.Open()
	if !m.Join(room) {
		t.Fatal("no desk")
	}
	lan.games = []LANGame{{File: "Siena", Host: "ANN", Addr: "192.168.1.8:2424", Players: 1}}
	m.Update(key("m"))
	m.Update(key("down"))
	m.Update(key("enter")) // join a LAN game
	m.Update(key("enter")) // the game
	m.Update(key("enter")) // the name
	if _, ok := m.TakeLANJoin(); !ok {
		t.Fatal("no join was chosen")
	}
	if m.Room != nil || m.Desk != nil || m.Screen != ScreenLobby {
		t.Fatalf("still seated: room=%v desk=%v screen=%v", m.Room != nil, m.Desk != nil, m.Screen)
	}
	if n := room.Seated(); n != 0 {
		t.Errorf("the desk is still held: %d seated", n)
	}
}

func TestFileOfIsWhereSomebodySits(t *testing.T) {
	rooms := NewRooms(1)
	rooms.EnsureOpen(2)
	if rooms.FileOf("kier") != nil {
		t.Error("nobody is seated, yet a file was found")
	}
	host := NewLocalSession(rooms, 1, "kier")
	room := rooms.Find(rooms.List()[1].Name)
	if !host.Join(room) {
		t.Fatal("no desk")
	}
	if rooms.FileOf("kier") != room {
		t.Error("the host's file was not found")
	}
}

// A joined player is brought to the host's file, and told if it is full.
func TestAJoinerSitsAtTheHostsFile(t *testing.T) {
	rooms := NewRooms(1)
	room := rooms.Open()
	host := NewLocalSession(rooms, 1, "kier")
	host.Resize(120, 40)
	if !host.Join(room) {
		t.Fatal("no desk")
	}
	j := NewJoinedSession(rooms, 2, "joe")
	j.Resize(120, 40)
	j.SitAt(rooms.FileOf("kier"))
	if j.Room != room || j.Desk == nil || j.Screen != ScreenMain {
		t.Fatalf("joiner room same=%v desk=%v screen=%v", j.Room == room, j.Desk != nil, j.Screen)
	}

	for i := range MaxDesks - 2 {
		o := NewJoinedSession(rooms, int64(3+i), "ann")
		o.Resize(120, 40)
		o.SitAt(room)
	}
	full := NewJoinedSession(rooms, 9, "bob")
	full.Resize(120, 40)
	full.SitAt(room)
	if full.Room != nil || full.Screen != ScreenLobby || !strings.Contains(full.notice, "full") {
		t.Fatalf("a full file: room=%v screen=%v notice=%q", full.Room != nil, full.Screen, full.notice)
	}

	// A host on the files leaves their joiners there, as before.
	lone := NewJoinedSession(NewRooms(1), 10, "zed")
	lone.SitAt(nil)
	if lone.Screen != ScreenLobby || lone.notice != "" {
		t.Errorf("no host file: screen=%v notice=%q", lone.Screen, lone.notice)
	}
}
