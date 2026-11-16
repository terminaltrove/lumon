package lumon

import (
	"strconv"
	"strings"
	"time"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// Hosting and joining a game on the local network. This file is the panel in
// the files screen; the sockets belong to the program, behind the LAN interface,
// so this package stays free of the network.
//
// Only somebody at their own terminal is offered any of it. A session that came
// in over a connection has no LAN, so it can never start a listener or dial out
// from the host's process.
//
// Nobody joins without the host's say. A request shows on the host's screen,
// whatever they are doing, until they answer it or the person gives up.

// LAN is the network side of hosting and joining.
type LAN interface {
	// Host starts accepting players on the local network, with the host playing
	// as name.
	Host(name string) error
	// StopHosting ends the game for everybody who joined.
	StopHosting()
	// Hosting reports the game being hosted, if there is one. It is called with
	// a room's lock held, so it must not touch the Rooms.
	Hosting() (LANHosting, bool)
	// Answer accepts or denies somebody waiting to join.
	Answer(id uint64, accept bool)
	// Kick removes somebody who joined.
	Kick(id uint64)
	// Games is the games found on the network so far. Looking starts on the
	// first call and goes on until StopLooking.
	Games() []LANGame
	StopLooking()
}

// LANHosting is a game this terminal is hosting.
type LANHosting struct {
	File, Addr string
	// Key is the start of the host's key fingerprint, which a joiner compares
	// with their own screen the first time they join.
	Key    string
	Joined int
	// Waiting is who is asking to join, the longest waiting first.
	Waiting []LANRequest
	// Players is who has joined, the first to join first.
	Players []LANPlayer
}

// LANPlayer is somebody who joined.
type LANPlayer struct {
	ID         uint64
	Name, Addr string
}

// LANRequest is somebody asking to join. Anybody can claim any name, so the
// address is shown with it.
type LANRequest struct {
	ID         uint64
	Name, Addr string
}

// LANGame is a game found on the network. File and Host come off the network, so
// they are cleaned before they are drawn. Players counts the host.
type LANGame struct {
	File, Host, Addr string
	Players          int
}

// LANTarget is the game somebody chose to join.
type LANTarget struct{ Addr, Name string }

type lanStep int

const (
	lanClosed lanStep = iota
	lanMenu
	lanHosted
	lanFinding
	lanTypingAddr
	lanTypingName
	lanTypingHostName
)

const (
	// lanFieldLimit is the longest address the panel takes.
	lanFieldLimit = 48

	// lanFileLimit is the longest game name taken off the network.
	lanFileLimit = 24

	// lanRequestGuard is how long a new request ignores its keys, so somebody
	// about to press n for a new file does not deny a person they never saw.
	lanRequestGuard = 500 * time.Millisecond
)

// lanPanel is the panel's state on the Model.
type lanPanel struct {
	step       lanStep
	pick       int
	addr, name string
	note       string
	// pickAddr holds the choice in the list of games by what it is, so that a
	// new answer cannot move a different game under the cursor.
	pickAddr string
	join     *LANTarget
}

// NewJoinedSession is somebody the host let in. They asked under this name, so
// they go straight to the files with it and are never asked again. If it is
// taken it gets a number. It has no LAN and no computer refiners.
func NewJoinedSession(rooms *Rooms, seed int64, name string) *Model {
	m := newView(seed)
	m.rooms = rooms
	m.joined = true
	m.Name = LANName(name)
	m.Screen = ScreenLobby
	base := m.Name
	for n := 2; !m.claimName() && n < 100; n++ {
		suffix := "-" + strconv.Itoa(n)
		m.Name = tidyName(clipTo(base, NameLimit-len(suffix)) + suffix)
	}
	m.nameNote, m.nameRefused = "", false
	return m
}

// FileOf is the file somebody is sitting at, or nil. A host's joiners are seated
// there. It takes the registry's and rooms' locks, so never call it holding a
// room's.
func (rs *Rooms) FileOf(name string) *Room {
	name = LANName(name)
	for _, l := range rs.List() {
		for _, who := range l.Refiners {
			if who.Name == name {
				return rs.Find(l.Name)
			}
		}
	}
	return nil
}

// SitAt seats a joined player at the host's file, as choosing it would. If it is
// full they stay on the files, told so.
func (m *Model) SitAt(room *Room) {
	if room != nil {
		m.tryJoin(room)
	}
}

// LANName is the name to show for somebody on a LAN game. It also makes a name
// that came off the network safe to draw. A name shows beside an address on the
// host's screen, so one that could be taken for an address is not used.
func LANName(name string) string {
	name = tidyName(name)
	if name == "" || (strings.Contains(name, ".") && strings.Trim(name, "0123456789.") == "") {
		return "REFINER"
	}
	return name
}

// HostFiles is what a game is called while its host is on no file.
func HostFiles(host string) string { return LANName(host) + "'s files" }

// CleanLANFile makes a game's name that came off the network safe to draw. It
// keeps the case, as file names are written in the drawer.
func CleanLANFile(s string) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len() >= lanFileLimit {
			break
		}
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune(" .-'", r) {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// GameName is what a LAN game is called: the file its host is on, or else the
// busiest file with anybody at it, or else the host's files. It takes the
// registry's and rooms' locks, so never call it holding a room's.
func (rs *Rooms) GameName(host string) string {
	host = LANName(host)
	busiest := ""
	for _, l := range rs.List() {
		if len(l.Refiners) == 0 {
			continue
		}
		if busiest == "" {
			busiest = l.Name
		}
		for _, who := range l.Refiners {
			if who.Name == host {
				return CleanLANFile(l.Name)
			}
		}
	}
	if busiest != "" {
		return CleanLANFile(busiest)
	}
	return HostFiles(host)
}

// lanRequest is the request to show, and how many more are behind it. The time
// a request first came up is noted here because that is when its keys start to
// count, including for the one behind it once it is answered.
func (m *Model) lanRequest() (r LANRequest, more int, ok bool) {
	if m.lan == nil {
		return LANRequest{}, 0, false
	}
	g, on := m.lan.Hosting()
	if !on || len(g.Waiting) == 0 {
		return LANRequest{}, 0, false
	}
	r = g.Waiting[0]
	if r.ID != m.lanShown {
		m.lanShown, m.lanShownAt = r.ID, time.Now()
	}
	return r, len(g.Waiting) - 1, true
}

// answerLANRequest takes y and n while a request is showing, on any screen, and
// reports whether it took the key. The wipe question has those keys while it is
// open, and the request waits under it.
func (m *Model) answerLANRequest(key string) bool {
	yes := key == "y" || key == "Y"
	if (!yes && key != "n" && key != "N") || m.wipeAsk {
		return false
	}
	r, _, ok := m.lanRequest()
	if !ok {
		return false
	}
	if time.Since(m.lanShownAt) >= lanRequestGuard {
		m.lan.Answer(r.ID, yes)
	}
	return true
}

// drawLANRequest paints the request across the status row (see statusRow).
func (m *Model) drawLANRequest(buf *canvas.Buffer) {
	if m.wipeAsk {
		return
	}
	r, more, ok := m.lanRequest()
	if !ok {
		return
	}
	w, h := buf.Size()
	if w < 1 || h < 1 {
		return
	}
	m.drawStatusBar(buf, lanRequestLine(r, more, w), m.Theme.BinFullText)
}

// lanRequestLine says who is asking in the width there is. The address is what
// can be trusted, so the name goes before it does.
func lanRequestLine(r LANRequest, more, w int) string {
	s := r.Name + " " + r.Addr + " wants to join · y accept · n deny"
	if more > 0 {
		s += " · " + strconv.Itoa(more) + " more waiting"
	}
	for _, shorter := range []string{
		r.Name + " " + r.Addr + " wants to join · y/n",
		r.Addr + " wants to join · y/n",
		r.Addr + " · y/n",
	} {
		if len([]rune(s)) <= w {
			break
		}
		s = shorter
	}
	return clipTo(s, w)
}

// OfferLAN gives this player the host and join panel. The name is claimed so
// nobody who joins can take it.
func (m *Model) OfferLAN(lan LAN) {
	m.lan = lan
	if m.rooms != nil && m.Name != "" {
		m.claimName()
	}
}

// ShowLANHosting opens the panel on the game being hosted.
func (m *Model) ShowLANHosting() { m.lanUI.step = lanHosted }

// TakeLANJoin returns the game this player chose to join, once. The program
// runs the join when the terminal is free, and calls LANJoinEnded after.
func (m *Model) TakeLANJoin() (LANTarget, bool) {
	t := m.lanUI.join
	if t == nil {
		return LANTarget{}, false
	}
	m.lanUI.join = nil
	return *t, true
}

// LANJoinEnded brings this player back to the files after a join, saying why it
// ended if it went wrong.
func (m *Model) LANJoinEnded(err error) {
	m.Quitting = false
	m.lanUI.step = lanClosed
	if err != nil {
		m.lanUI.step = lanFinding
		m.lanUI.pick = 0
		m.lanUI.note = sentence(err.Error())
	}
}

// openLAN opens the panel, on the running game if there is one.
func (m *Model) openLAN() {
	m.lanUI = lanPanel{step: lanMenu}
	if _, on := m.lan.Hosting(); on {
		m.lanUI.step = lanHosted
	}
}

// closeLAN shuts the panel and stops looking for games.
func (m *Model) closeLAN() {
	m.lan.StopLooking()
	m.lanUI.step = lanClosed
}

// handleLANKey takes every key but ctrl+c while the panel is open, so a name
// can hold the letters that mean something else on this screen. It returns
// true when a join was chosen, which ends the program to free the terminal.
func (m *Model) handleLANKey(msg KeyMsg) bool {
	key := msg.String()
	switch m.lanUI.step {
	case lanMenu:
		m.lanMenuKey(key)
	case lanHosted:
		switch key {
		case "x":
			m.lan.StopHosting()
			m.lanUI = lanPanel{step: lanMenu}
		case "up", "k":
			if m.lanUI.pick > 0 {
				m.lanUI.pick--
			}
		case "down", "j":
			m.lanUI.pick++
		case "r":
			if g, on := m.lan.Hosting(); on && len(g.Players) > 0 {
				m.lan.Kick(g.Players[min(m.lanUI.pick, len(g.Players)-1)].ID)
			}
		case "enter", "esc", "q":
			m.closeLAN()
		}
	case lanFinding:
		m.lanFindingKey(key)
	case lanTypingAddr:
		switch key {
		case "enter":
			if strings.TrimSpace(m.lanUI.addr) != "" {
				m.lanUI.note = ""
				m.lanUI.name = m.Name
				m.lanUI.step = lanTypingName
			}
		case "esc":
			m.lanUI.step = lanFinding
		default:
			m.lanUI.addr = typeLANField(m.lanUI.addr, msg, false)
		}
	case lanTypingHostName:
		switch key {
		case "enter":
			m.hostAs(tidyName(m.lanUI.name))
		case "esc":
			m.lanUI = lanPanel{step: lanMenu}
		default:
			m.lanUI.name = typeLANField(m.lanUI.name, msg, true)
		}
	case lanTypingName:
		switch key {
		case "enter":
			if name := tidyName(m.lanUI.name); name != "" {
				// From the board, give the desk up: this player is going to
				// somebody else's game, and comes back to the files after.
				if m.Room != nil {
					m.Screen, m.leaveForLobby = ScreenLobby, true
				}
				m.lanUI.join = &LANTarget{Addr: strings.TrimSpace(m.lanUI.addr), Name: name}
				m.lan.StopLooking()
				m.Quitting = true
				return true
			}
		case "esc":
			m.lanUI.step = lanFinding
		default:
			m.lanUI.name = typeLANField(m.lanUI.name, msg, true)
		}
	}
	return false
}

func (m *Model) lanMenuKey(key string) {
	switch key {
	case "up", "k", "down", "j":
		m.lanUI.pick = 1 - m.lanUI.pick
	case "enter":
		if m.lanUI.pick == 1 {
			m.lanUI.step, m.lanUI.pick, m.lanUI.note = lanFinding, 0, ""
			return
		}
		m.lanUI.step, m.lanUI.name, m.lanUI.note = lanTypingHostName, m.Name, ""
	case "esc", "q":
		m.closeLAN()
	}
}

// hostAs starts hosting with this player renamed. The new name is theirs
// everywhere, on the board and the scores as well as in the game's name, so it
// is claimed as a typed name is; one somebody else holds is refused. If hosting
// fails, the old name comes back.
func (m *Model) hostAs(name string) {
	switch {
	case name == "":
		return
	case LANName(name) != name:
		m.lanUI.note = "A name that looks like an address is not used"
		return
	}
	old := m.Name
	if !m.renameTo(name) {
		m.lanUI.note = sentence(strings.ToLower(m.nameNote))
		m.nameNote, m.nameRefused = "", false
		return
	}
	if err := m.lan.Host(m.Name); err != nil {
		m.renameTo(old)
		m.lanUI.note = sentence(err.Error())
		return
	}
	m.lanUI.step, m.lanUI.note = lanHosted, ""
	// At the board, the desk still carries the old name. Sitting down again
	// takes the room's lock, which a key is handled under, so it waits for
	// UpdateEvent to let go (see sitAgain).
	if m.Desk != nil && m.Name != old {
		m.reseat = true
	}
}

// sitAgain stands this player up and sits them at the same file, under the name
// they have now. Without a room's lock held.
func (m *Model) sitAgain() {
	room := m.Room
	if room == nil {
		return
	}
	m.Leave()
	if !m.Join(room) {
		m.Screen = ScreenLobby
	}
}

// renameTo takes a new name, giving the old one back to the floor, or keeps the
// old one if the new is refused.
func (m *Model) renameTo(name string) bool {
	old := m.Name
	m.Name = name
	if m.rooms != nil && !m.claimName() {
		m.Name = old
		return false
	}
	return true
}

// lanHoldPick puts the choice back on the game it was on, if that is still
// listed, and remembers which that is.
func (m *Model) lanHoldPick(games []LANGame) {
	for i, g := range games {
		if g.Addr == m.lanUI.pickAddr {
			m.lanUI.pick = i
			return
		}
	}
	if m.lanUI.pick >= len(games) {
		m.lanUI.pick = max(len(games)-1, 0)
	}
	if m.lanUI.pick < len(games) {
		m.lanUI.pickAddr = games[m.lanUI.pick].Addr
	}
}

func (m *Model) lanFindingKey(key string) {
	games := m.lan.Games()
	m.lanHoldPick(games)
	switch key {
	case "up", "k":
		if m.lanUI.pick > 0 {
			m.lanUI.pick--
			m.lanUI.pickAddr = games[m.lanUI.pick].Addr
		}
	case "down", "j":
		if m.lanUI.pick < len(games)-1 {
			m.lanUI.pick++
			m.lanUI.pickAddr = games[m.lanUI.pick].Addr
		}
	case "enter":
		if m.lanUI.pick < len(games) {
			m.lanUI.addr, m.lanUI.name, m.lanUI.note = games[m.lanUI.pick].Addr, m.Name, ""
			m.lanUI.step = lanTypingName
		}
	case "a":
		m.lanUI.addr, m.lanUI.note = "", ""
		m.lanUI.step = lanTypingAddr
	case "esc", "q":
		m.lan.StopLooking()
		m.lanUI = lanPanel{step: lanMenu, pick: 1}
	}
}

// typeLANField adds what was typed to an address or a name. Only what can be in
// one gets through; a name is upper case, as it is on the floor.
func typeLANField(field string, msg KeyMsg, name bool) string {
	if msg.String() == "backspace" {
		if r := []rune(field); len(r) > 0 {
			return string(r[:len(r)-1])
		}
		return field
	}
	if msg.Alt {
		return field
	}
	limit := lanFieldLimit
	if name {
		limit = NameLimit
	}
	for _, r := range msg.Runes {
		if len(field) >= limit {
			break
		}
		switch {
		case name && r >= 'a' && r <= 'z':
			field += string(r - 'a' + 'A')
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			field += string(r)
		case name && strings.ContainsRune(" .-", r):
			field += string(r)
		case !name && (r >= 'a' && r <= 'z' || strings.ContainsRune(".:-[]%", r)):
			field += string(r)
		}
	}
	return field
}

// drawLAN paints the open panel over the files.
func (m *Model) drawLAN(buf *canvas.Buffer) {
	box, ok := m.modalBox(buf, 68, 20, "LAN GAME")
	if !ok {
		w, h := buf.Size()
		if w > 0 && h > 0 {
			buf.Text(0, h/2, clipTo("make the terminal bigger for LAN games", w), m.Theme.BinOn)
		}
		return
	}
	// Each step lists its rows, then they are centred between the title and the
	// hint. The two rows above the bottom border are the note and the hint.
	room := box.H - 4
	var rows []pickRow
	line := func(s string, a canvas.Attr) {
		rows = append(rows, pickRow{text: s, attr: a})
	}
	gap := func() { rows = append(rows, pickRow{}) }
	choice := func(s string, on bool) {
		if on {
			rows = append(rows, pickRow{text: s, attr: m.Theme.BinOn, picked: true})
		} else {
			rows = append(rows, pickRow{text: s, attr: m.Theme.Hint})
		}
	}
	hint := ""
	switch m.lanUI.step {
	case lanMenu:
		for i, s := range []string{"Host a LAN game", "Join a LAN game"} {
			if i > 0 {
				gap()
			}
			choice(s, i == m.lanUI.pick)
		}
		hint = "↑↓ choose · enter select · esc close"
	case lanHosted:
		if g, on := m.lan.Hosting(); on {
			line("People on this network can ask to join.", m.Theme.Hint)
			line("You accept or deny each one.", m.Theme.Hint)
			gap()
			line("Game     "+g.File, m.Theme.BinOn)
			line("Address  "+g.Addr, m.Theme.BinOn)
			if g.Key != "" {
				line("Key      "+g.Key, m.Theme.BinOn)
			}
			gap()
			line(sentence(lanJoinedLine(g.Joined)), m.Theme.Hint)
			left := room - len(rows)
			pick := min(m.lanUI.pick, max(len(g.Players)-1, 0))
			m.lanUI.pick = pick
			for i, p := range g.Players {
				if left < 1 || i < pick-left+1 || i > max(pick, left-1) {
					continue
				}
				choice(p.Name+"  "+p.Addr, i == pick)
			}
			if len(g.Players) > 0 {
				hint = "↑↓ choose · r remove · enter back · x stop hosting"
				break
			}
		}
		hint = "enter back to files · x stop hosting"
	case lanFinding:
		games := m.lan.Games()
		m.lanHoldPick(games)
		if len(games) == 0 {
			line("Looking on this network...", m.Theme.Hint)
		}
		for i, g := range games {
			if i >= 6 {
				break
			}
			choice(lanGameLine(g), i == m.lanUI.pick)
		}
		hint = "enter join · a type an address · esc back"
	case lanTypingAddr:
		line("Address of the game", m.Theme.Hint)
		gap()
		line(m.lanUI.addr+"_", m.Theme.BinOn)
		hint = "enter next · esc back"
	case lanTypingName:
		line("Your name, for the host at "+m.lanUI.addr, m.Theme.Hint)
		gap()
		line(m.lanUI.name+"_", m.Theme.BinOn)
		hint = "enter ask to join · esc back"
	case lanTypingHostName:
		line("Your name, for this game", m.Theme.Hint)
		gap()
		line(m.lanUI.name+"_", m.Theme.BinOn)
		hint = "enter start hosting · esc back"
	}
	y := box.Y + 1 + max((room+1-len(rows))/2, 0)
	for _, r := range rows {
		if y > box.Y+box.H-3 {
			break
		}
		m.drawPickRow(buf, box, y, r)
		y++
	}
	if m.lanUI.note != "" {
		m.modalCentre(buf, box, box.Y+box.H-3, m.lanUI.note, m.Theme.BadLive)
	}
	m.modalCentre(buf, box, box.Y+box.H-2, hint, m.Theme.Hint)
}

// pickRow is one line of a menu. A blank one has no text.
type pickRow struct {
	text   string
	attr   canvas.Attr
	picked bool
}

// drawPickRow centres a row's words, with the picker's arrow out to their left, so
// the words stay put as the selection moves. With no room for the arrow beside
// them, the words give way.
func (m *Model) drawPickRow(buf *canvas.Buffer, box modal, y int, r pickRow) {
	if r.text == "" {
		return
	}
	const arrow = 2 // "> "
	s := clipTo(r.text, box.W-2-2*arrow)
	x := box.X + (box.W-len([]rune(s)))/2
	buf.Text(x, y, s, r.attr)
	if r.picked {
		buf.Text(x-arrow, y, ">", r.attr)
	}
}

// drawLANBanner says on the files screen that a game is being hosted, and how to
// to reach it, with the panel closed.
func (m *Model) drawLANBanner(buf *canvas.Buffer) {
	if m.lan == nil {
		return
	}
	g, on := m.lan.Hosting()
	if !on {
		return
	}
	w, _ := buf.Size()
	s := clipTo("Hosting "+g.File+" · "+g.Addr+" · "+lanJoinedLine(g.Joined), w-4)
	buf.Text((w-len([]rune(s)))/2, 0, s, m.Theme.BinOn)
}

// lanGameLine is one game in the list. The address is last so that a narrow
// terminal loses that first.
func lanGameLine(g LANGame) string {
	file := CleanLANFile(g.File)
	if file == "" {
		file = "A game"
	}
	who := strconv.Itoa(g.Players) + " refiners"
	if g.Players == 1 {
		who = "1 refiner"
	}
	return file + "  " + LANName(g.Host) + "  " + who + "  " + g.Addr
}

func lanJoinedLine(n int) string {
	switch n {
	case 0:
		return "nobody has joined yet"
	case 1:
		return "1 has joined"
	}
	return strconv.Itoa(n) + " have joined"
}

// sentence starts a line with a capital, for a message written in lower case.
func sentence(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}
