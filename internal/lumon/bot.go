package lumon

import (
	"math/rand"
	"strconv"
)

// Computer refiners: scripted company at the empty desks of a local or browser
// board, never remotely, where the desks are for people.
//
// A bot is a desk the room drives itself, stepped in Room.Step under the lock
// already held. Everything other refiners show (lens, nameplate, roster,
// minimap, flying digits) is drawn from the desk and the deposits, so a bot
// needs no drawing of its own. Its work finishes the file but never reaches
// the scores (Room.credit). It walks at a person's pace, so the player still
// matters, and it never takes a cluster somebody else is reaching for.

// botNames are invented, so no computer refiner borrows a character's name.
var botNames = [...]string{
	"GRETA V.", "ODIN H.", "MILO T.", "AGNES P.",
	"OTTO L.", "WREN C.", "RUTH D.", "ELLIS B.",
	"HUGO S.", "IRMA K.", "FELIX N.", "VERA M.",
	"BORIS A.", "NELL F.", "SAUL W.", "TILDA J.",
	"AMOS R.", "EDITH Q.", "LEON Z.", "MAEVE O.",
	"CYRIL U.", "OLGA T.", "PERCY L.", "HAZEL D.",
}

// The bot's window on the board, in cells: about what a person sees.
const botViewW, botViewH = 48, 14

// seatBot sits a computer refiner at the first free desk. The caller holds the
// lock.
func (r *Room) seatBot() *Desk {
	if r.seated >= MaxDesks {
		return nil
	}
	d := r.seatLocked(r.freeBotName())
	if d == nil {
		return nil
	}
	d.Bot = true
	d.Bin = d.Seat % BinCount

	rng := rand.New(rand.NewSource(r.seed ^ int64(d.Seat+1)*0x6B07))
	b := newBot(d, rng)

	// Start near a person, so the company is seen arriving.
	x, y := GridSize/2, GridSize/2
	for _, other := range r.desk {
		if other != nil && !other.Bot {
			x, y = other.CursorX, other.CursorY
			break
		}
	}
	d.CursorX = clampInt(x+rng.Intn(31)-15, 0, GridSize-1)
	d.CursorY = clampInt(y+rng.Intn(15)-7, 0, GridSize-1)
	b.wait = 0.5 + rng.Float64()
	b.look()

	r.bots = append(r.bots, b)
	return d
}

// freeBotName is a name from botNames nobody at the board is using, taken in
// an order drawn from the room's seed, so each file seats its own company.
func (r *Room) freeBotName() string {
	for _, i := range rand.New(rand.NewSource(r.seed)).Perm(len(botNames)) {
		name := botNames[i]
		taken := false
		for _, d := range r.desk {
			if d != nil && refinerID(d.Name) == refinerID(name) {
				taken = true
				break
			}
		}
		if !taken {
			return name
		}
	}
	return ""
}

// addBots seats up to n computer refiners, as many as there are free desks.
func (r *Room) addBots(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for range n {
		if r.seatBot() == nil {
			return
		}
	}
}

// unspectate is a spectator leaving the room.
func (r *Room) unspectate() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.spectators > 0 {
		r.spectators--
	}
	r.markEmpty()
	r.botsLeaveIfAlone()
}

// sendBotsHome stands every computer refiner up. The caller holds the lock.
func (r *Room) sendBotsHome() {
	for len(r.bots) > 0 {
		r.sendOneBotHome()
	}
}

// sendOneBotHome stands up the computer refiner who sat down last, and reports
// whether there was one. The caller holds the lock.
func (r *Room) sendOneBotHome() bool {
	if len(r.bots) == 0 {
		return false
	}
	b := r.bots[len(r.bots)-1]
	r.bots = r.bots[:len(r.bots)-1]
	r.unseatLocked(b.desk)
	return true
}

// botsLeaveIfAlone sends the computer refiners home once no person is at the
// board, seated, queued or watching. Nobody would step the board, and the
// room could never be reclaimed. The caller holds the lock.
func (r *Room) botsLeaveIfAlone() {
	if len(r.bots) > 0 && r.seated == len(r.bots) && len(r.watchers) == 0 && r.spectators == 0 {
		r.sendBotsHome()
	}
}

// AllowBots lets this player have computer refiners, starting with company of
// them (0 to MaxDesks-1). Local and browser hosts call it; a remote session never does.
func (m *Model) AllowBots(company int) {
	m.allowBots = true
	m.company = clampInt(company, 0, MaxDesks-1)
	if m.Room != nil && m.Desk != nil && m.company > 0 {
		m.Room.addBots(m.company)
	}
}

// StartWatching joins a file if this player is not at one, and gets up to
// watch computer refiners work it. It must not be called while the player
// holds a board's lock.
func (m *Model) StartWatching() {
	if !m.allowBots {
		return
	}
	if m.Room == nil {
		if m.rooms == nil || !m.Join(m.rooms.Open()) {
			return
		}
		m.Intro = nil
	}
	m.Room.mu.Lock()
	m.standUp()
	m.Room.mu.Unlock()
}

// toggleCompany is c on the board: one more computer refiner, or, with every
// desk taken, all of them sent home. The caller holds the lock.
func (m *Model) toggleCompany() {
	if !m.allowBots || m.Room == nil {
		return
	}
	if m.Room.seatBot() == nil {
		m.Room.sendBotsHome()
	}
}

// toggleWatching is w on the board. The caller holds the lock.
func (m *Model) toggleWatching() {
	if !m.allowBots || m.Room == nil {
		return
	}
	if m.spectating {
		m.sitDown()
		return
	}
	if m.Desk != nil {
		m.standUp()
	}
}

// standUp gives up the desk to watch, with a computer refiner at every desk.
// Sitting back down sends one home. The caller holds the lock.
func (m *Model) standUp() {
	if m.Desk == nil || m.spectating {
		return
	}
	r := m.Room
	r.spectators++
	r.unseatLocked(m.Desk)
	m.Desk, m.spectating = nil, true
	clear(m.held)
	m.sweep = nil
	for r.seatBot() != nil {
	}
}

// sitDown takes a desk again after watching; a computer refiner gives up its
// desk if that is the only one. The caller holds the lock.
func (m *Model) sitDown() {
	r := m.Room
	d := r.seatPersonLocked(m.Name)
	if d == nil {
		return
	}
	r.spectators--
	m.Desk, m.spectating = d, false
}

// followBots keeps a spectator's camera on the computer refiner that refined
// last, unless the player has been looking around for themselves.
func (m *Model) followBots(dt float64) {
	if !m.spectating || m.Room == nil {
		return
	}
	if m.lookAround > 0 {
		m.lookAround -= dt
		return
	}
	d := m.followed(dt)
	if d == nil {
		return
	}
	m.panToward(d.CursorX, d.CursorY, dt)

	// The cursor rides with the refiner, so the cluster they are on swells
	// here as on their screen, held inside the view while it catches up.
	l := m.Layout
	x := clampInt(d.CursorX, l.ViewportX, l.ViewportX+l.ViewportW-1)
	y := clampInt(d.CursorY, l.ViewportY, l.ViewportY+l.ViewportH-1)
	if x != m.CursorX || y != m.CursorY {
		m.CursorX, m.CursorY = x, y
		m.updateHover(x, y)
	}
}

// watchedDesk reports whether d is the refiner a spectator's camera is on.
func (m *Model) watchedDesk(d *Desk) bool {
	return m.spectating && d != nil && d.Bot && d.Seat == m.followSeat
}

// followHold is how long the camera stays with one refiner before it moves to
// whoever refined last: switching at every refine would swing the view about.
const followHold = 10.0

// followed is the computer refiner the camera is on, which it keeps for
// followHold seconds while they are at the board.
func (m *Model) followed(dt float64) *Desk {
	r := m.Room
	m.followFor += dt
	if d := r.desk[m.followSeat]; d != nil && d.Bot && (m.followPinned || m.followFor < followHold) {
		return d
	}
	m.followPinned = false
	pick := r.desk[r.lastBot]
	if pick == nil || !pick.Bot {
		if len(r.bots) == 0 {
			return nil
		}
		pick = r.bots[0].desk
	}
	if pick.Seat != m.followSeat {
		m.followSeat, m.followFor = pick.Seat, 0
	}
	return pick
}

// follow is a watcher's choice of refiner: 1-4 a desk, tab the next one along,
// 0 back to the camera's own choice. A chosen refiner is kept until another
// choice or until they leave. The caller holds the lock.
func (m *Model) follow(key string) {
	r := m.Room
	switch key {
	case "0":
		m.followPinned, m.followFor = false, followHold
		return
	case "tab":
		for i := 1; i <= MaxDesks; i++ {
			seat := (m.followSeat + i) % MaxDesks
			if d := r.desk[seat]; d != nil && d.Bot {
				m.followSeat, m.followPinned, m.lookAround = seat, true, 0
				return
			}
		}
		return
	}
	seat := int(key[0] - '1')
	if seat >= 0 && seat < MaxDesks && r.desk[seat] != nil && r.desk[seat].Bot {
		m.followSeat, m.followPinned, m.lookAround = seat, true, 0
	}
}

// The camera holds still while the refiner it follows is in the middle half of
// the view, then glides a cell per step until they are centred: a terminal
// scrolls in whole cells, so moving often and little reads as a pan.
const (
	panStep    = 0.045 // seconds per cell
	panStepFar = 0.02  // when the refiner is more than half a view away
)

// panToward moves the view toward centring cell (x, y).
func (m *Model) panToward(x, y int, dt float64) {
	l := m.Layout
	tx := clampInt(x-l.ViewportW/2, 0, GridSize-l.ViewportW)
	ty := clampInt(y-l.ViewportH/2, 0, GridSize-l.ViewportH)
	dx, dy := tx-l.ViewportX, ty-l.ViewportY
	if dx == 0 && dy == 0 {
		m.panning, m.panClock = false, 0
		return
	}
	if !m.panning && abs(dx) <= l.ViewportW/4 && abs(dy) <= l.ViewportH/4 {
		return
	}
	m.panning = true
	step := panStep
	if abs(dx) > l.ViewportW/2 || abs(dy) > l.ViewportH/2 {
		step = panStepFar
	}
	if m.panClock += dt; m.panClock < step {
		return
	}
	m.panClock = 0
	l.ScrollTo(l.ViewportX+sign0(dx), l.ViewportY+sign0(dy))
	// The board moved under everybody else's cursor; see scrollToCursor.
	m.reprojectLenses()
}

// sign0 is -1, 0 or 1.
func sign0(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	}
	return 0
}

// lookAroundFor is how long a spectator's own movement holds the camera.
const lookAroundFor = 4.0

// companyNotice is what c in the files says.
func companyNotice(n int) string {
	switch n {
	case 0:
		return "you will refine alone"
	case 1:
		return "1 refiner will join you"
	}
	return strconv.Itoa(n) + " refiners will join you"
}

// bindings is the controls this player has: the browser's, or the terminal's
// with or without the computer refiners' keys.
func (m *Model) bindings() []BindingGroup {
	switch {
	case m.keepOpen:
		return browserBindings
	case m.allowBots && m.lan != nil:
		return companyLANBindings
	case m.lan != nil:
		return lanBindings
	case !m.multiplayerAvailable() && m.allowBots:
		return soloCompanyBindings
	case !m.multiplayerAvailable():
		return soloBindings
	case m.allowBots:
		return companyBindings
	}
	return terminalBindings
}
