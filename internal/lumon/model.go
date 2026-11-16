package lumon

import (
	"image"
	"math"
	"math/rand"
	"time"

	"github.com/terminaltrove/lumon/internal/canvas"
	"github.com/terminaltrove/lumon/internal/physics"
	"github.com/terminaltrove/lumon/internal/theme"
)

// ScreenState is which screen is showing.
type ScreenState int

const (
	ScreenMain ScreenState = iota
	ScreenIdle

	// New screens are appended so ScreenMain stays the zero value, which tests
	// that build a Model without setting Screen rely on.
	ScreenIntro

	// ScreenName asks who is sitting down, and ScreenLobby which file they are
	// going to work. A session is on these before it has a board.
	ScreenName
	ScreenLobby

	// ScreenScores is who has refined what.
	ScreenScores

	// ScreenOff is a dark set waiting to be switched on. The browser starts
	// here, since it may play the startup sound only after the refiner has
	// pressed or clicked something.
	ScreenOff
)

// Model is one player's terminal: their view of a Room, and everything animated
// that is theirs alone.
//
// The Room is embedded so the board is reachable by promotion (m.Grid,
// m.Actives()). View code may read the board, but only the board's own methods
// may change it, and Model must never declare a name Room has
// (TestModelAndRoomHaveNoNamesInCommon).
type Model struct {
	*Room

	Layout *Layout
	Screen ScreenState

	CursorX, CursorY int
	SelectedBin      int // 0-based

	// Simulation, each a thin arrangement of physics primitives.
	Clock     *physics.Clock
	Agitation *Agitation

	// lastSteps is how many simulation steps the last frame ran. See Stepped.
	lastSteps int

	Drift   *Drift
	Flights *Flights
	Regen   *Regen

	// idleMark is the screensaver's bouncing mark, there only while ScreenIdle
	// shows it. screensaverOnly is a terminal that came for nothing else: it has
	// no board to go back to, so waking it ends the program.
	idleMark        *Perpetuity
	screensaverOnly bool

	// newsHidden is this terminal's choice not to read the floor news.
	newsHidden bool

	// reseat asks for this player to stand up and sit again at the same file,
	// once the room's lock is released: a host who renamed at the board has a
	// desk under the old name.
	reseat bool

	// Magnify is how large each visible digit is drawn. Only corrupt numbers
	// grow.
	Magnify *Magnify

	// watch is this session's place in the queue for a desk, or nil. It is the
	// only case where Room != nil with Desk == nil is legal: Desk == nil means
	// "may look, may not refine", and watch says whether they are queueing.
	watch *Watcher

	// watchAt is the queue position watchText was built for. See watchLine.
	watchAt   int
	watchText string

	// miniLayers are the minimap's braille sheets, one per colour, reused
	// between frames. A braille layer has one attribute for all its cells, so
	// four refiners need four. See minimap.go.
	miniLayers []*canvas.Braille

	// link is the round trip to this terminal and the light drawn for it, and
	// links is where cmd/lumon posts measurements. A local run leaves links nil,
	// so nothing is measured or drawn.
	link  Link
	links <-chan time.Duration

	// linkAt is what linkMs was built from, and linkOpen whether the spread is
	// showing. linkSpanned is where the light was drawn, so a click can find it.
	// See linkText and clickedLink.
	linkAt      linkKey
	linkMs      string
	linkOpen    bool
	linkSpanned linkSpan

	// depositSeen is the last deposit this view has flown, the counterpart of
	// Ripple.seen. See deposit.go.
	depositSeen int

	// ripple is the mass-spring sheet, one particle per visible cell, that a
	// blast's wave travels through. View-local, like flights.
	ripple *Ripple

	// Lens is this refiner's own fisheye: where they are looking, and how far the
	// board is pushed apart around it. remoteLens holds one per other occupied
	// desk. Both are owned by this view, never stored on the Desk: Step mutates
	// a Lens's physics.World, so a shared one would be stepped once per player
	// per frame from several goroutines.
	Lens       *Lens
	remoteLens map[int]*Lens

	// lensList is the frame's lenses in seat order, reused between frames.
	// Bulge reads it once per visible cell, so it must not allocate or range a
	// map: map order varies, and floating-point addition is not associative, so
	// digits would jitter while nobody moved.
	lensList []*Lens

	// anchors is where each visible digit was drawn last frame, and anchorAt the
	// layout it was drawn against: the reverse of the render, which is how a
	// click finds its cell once lenses have bulged the grid past inverting.
	// anchorFar is how far each stamped cell is from the digit that claimed it,
	// so a nearer claim is never overwritten by a further one. See markAnchor.
	anchors   []int32
	anchorFar []uint8
	anchorAt  anchorStamp

	// styler turns this terminal's attributes into escape sequences. Remotely it
	// has a renderer bound to the client's own terminal, so each client gets the
	// colour depth it supports.
	styler *canvas.Styler

	// frame is the buffer View draws into, reused between frames. Per session,
	// since two sessions sharing one would draw over each other.
	frame *canvas.Buffer

	// The Lumon mark as a picture, for terminals that can show one. See mark.go.
	// Off unless set, so a terminal nobody asked about draws braille.
	graphics   Graphics    // how this terminal is asked for a picture, if at all
	markImg    image.Image // the mask tinted in this session's own colour
	markFor    string      // which theme markImg was tinted for
	markInline string      // the iTerm2 sequence for it, built once per theme
	markSent   bool        // kitty: the picture is on the terminal already
	markDrawn  int         // how many copies the frame being drawn has placed
	markUp     int         // how many the frame before this one placed

	// markSpot is where each placement was last drawn, so a delete is sent
	// from the rows the picture is leaving. See sweepMark.
	markSpot [markPlacements][2]int

	// Name is what this refiner typed on the way in, and what their nameplate
	// reads once they sit down.
	Name string

	// rooms is the registry a session picks from. Nil in a local run, which is
	// given a board directly.
	rooms *Rooms

	// notice is the lobby's one line of explanation, usually why a join was
	// refused.
	notice string

	// files is the drawer of macrodata files this session is choosing from;
	// its reel index is the lobby's only selection, shared by all three views.
	// Built on first use, because a session reaches the selector before it has
	// a registry.
	files *Rolodex

	// lobbyView is which of the three ways of looking at the drawer is up.
	lobbyView lobbyView

	// watchOffer is the file a full-room refusal has just offered to let this
	// session watch, so that choosing it again means yes. Choosing anything else
	// clears it, so a stale offer cannot turn a later join into a watch.
	watchOffer string

	// arrival is the board coming up, and nil the rest of the time. See arrive.
	arrival *Arrival

	// askName is whether this session is asked who it is on the way in: true
	// remotely, where other refiners read the nameplate. It cannot be derived
	// from Name == "", because the name is typed on the screen this decides,
	// and backing out of the drawer has to know which way is back.
	askName bool

	// The running diversion's simulation. At most one is set.
	sandbox  *Sandbox
	matrix   *Matrix
	sand     *Sand
	primes   *Primes
	praise   *Praise
	halls    *Corridors
	fountain *Fountain
	sky      *Fireworks
	comply   *Statement
	deep     *Matrix3D
	mark     *Perpetuity

	// finale is the display the bins put on when a file is finished, and
	// noticeOpen how far open the notice over it has got. Both are per
	// terminal; terminals agree because they read the same Room.Complete() on
	// the same frame. finale is separate from m.sky (the FIREWORKS diversion),
	// so entering that during a hold cannot take the celebration away.
	finale     *Fireworks
	noticeOpen *physics.Scalar

	// typed is how much of the hidden word has been entered, and divertAt the
	// row the diversions menu is resting on.
	typed    string
	divertAt int

	// divertOpen is the menu modal, and diverting the name of the one running.
	// Neither is a screen: a diversion runs inside the grid's panel with the
	// chrome standing.
	divertOpen bool
	diverting  string

	// transit is the digits in the air between the grid and a diversion. While
	// it runs, the diversion itself is neither stepped nor drawn.
	transit *Transit

	// scoresAt is the row the scoreboard is resting on, and scoresFrom the
	// screen to return to (the grid or the lobby), so the lobby never drops
	// somebody onto a board they had not chosen.
	scoresAt   int
	scoresFrom ScreenState

	// scoresTab is which board is up: this quarter, lifetime or the record.
	scoresTab scoreTab

	// Helping is whether the controls overlay is up, and About whether the
	// credits are. They are overlays, not screens, so they can open from the
	// grid, the lobby or the scoreboard.
	Helping bool
	About   bool

	// Escaped is whether the escape notice is up. See escape.go.
	Escaped bool

	// spectating is a player who has got up to watch the computer refiners,
	// keeping the board but no desk. See bot.go.
	spectating bool

	// allowBots is whether this player may have computer refiners (local and
	// browser, never a remote session), and company how many join them from the files.
	// lookAround is how long the spectator's camera stays where they moved it.
	allowBots  bool
	company    int
	lookAround float64

	// panning is whether the spectator's camera is gliding to centre the
	// refiner it follows, and panClock the time toward its next cell.
	panning  bool
	panClock float64

	// followSeat is the desk the camera follows, and followFor how long it
	// has. See followed.
	followSeat   int
	followFor    float64
	followPinned bool

	// agentNote is the notice about the refiner's coding agent, empty when
	// none is up. See agent_notice.go.
	agentNote string

	// agents is the coding agents on the strip in the file bar's lower edge.
	// See agent_strip.go.
	agents []AgentLine
	strip  stripCache

	// sounds is what this update has cued, handed to the host with its
	// effects.
	sounds []Effect

	// soundRng times the set's beeps, beepIn is how long until the next (0
	// for none yet), and cardIn how long until a card may sound again.
	// overlays is which boxes over the screen were showing at the last frame
	// (see popOpened).
	soundRng       *rand.Rand
	beepIn, cardIn float64
	overlays       uint16

	// heardRoom and heardPercent are the file and the percent its header
	// read at the last frame on the board (see chimeProgress).
	heardRoom    *Room
	heardPercent int

	// logoLength is how long the boot sequence's logo lasts, or 0 for its
	// own pace. See FitIntroTo.
	logoLength time.Duration

	// Theme is this terminal's palette. Per session, never global: remotely the
	// package is shared, so a global palette would change everybody's colours.
	Theme *theme.Theme

	// addrLine is the address readout, cached for the range it describes.
	addrLine string
	addrFrom uint32
	addrTo   uint32

	// binLabel is scratch for the bins' readouts, reused across frames and
	// bins so the draw path does not allocate.
	binLabel []byte

	// BinFill is how full each bin is drawn and which are being fed.
	BinFill *BinFill

	// Drawer is the four tempers rising out of each bin as it is fed. The
	// counts are the board's; the motion is this terminal's.
	Drawer *Drawer

	// sessionAge is how long this terminal has been running, in simulated
	// seconds; the name-screen cursor blinks off it. It runs on every screen.
	sessionAge float64

	// boardTime is how far this session has carried the shared board's clock,
	// seeded from the room on sitting down so a newcomer does not wind the
	// board back. See Room.StepTo.
	boardTime float64

	// seenEpoch is the file this view's own state belongs to. When the room
	// moves on, everything here about the old board is dropped. It is not
	// called epoch because Model embeds *Room, and the name would shadow the
	// room's own.
	seenEpoch int

	// Desk is this player's place at the board: nameplate, cursor as others see
	// it, and tally. It is on the room because others in the room read it.
	Desk *Desk

	// Intro is the boot sequence that plays before the grid appears, nil once
	// it is over.
	Intro *Intro

	// File is which macrodata file this terminal is refining, and how far along
	// it is.
	File *File

	// Session is the hex identifier the boot log signs off with.
	Session string

	// lastCursorX and lastCursorY are where the keyboard cursor was when the
	// lens last followed it, so whichever of keyboard and mouse moved last owns
	// the focus.
	lastCursorX, lastCursorY int

	// targets is reused each frame so building the growth map allocates nothing.
	targets map[int]float64

	// reachSeat is which seat is reaching for each lifted cluster, rebuilt with
	// targets every frame, so a glyph's plate takes that refiner's colour. See
	// glyphAttr.
	reachSeat map[int]int

	// Per-frame glyph placement, computed before the grid is drawn so the grid
	// pass knows which cells are painted large. Both are reused.
	boxes    []glyphBox
	enlarged map[int]bool

	// plates is where the nameplates went on the last pass. Tags legitimately
	// differ between two refiners at one room, so the test comparing two
	// terminals rune for rune asks where they are.
	plates []plateRect

	// header is where the file bar put its lanes on the last pass. The bar
	// divides its width at draw time and nothing else can recover which columns
	// each lane got; the tests read it.
	header headerBands

	// glyphing is which cells are currently drawn as multi-cell glyphs, kept
	// between frames for hysteresis: a cell on the size threshold, as at a
	// lens's rim, would otherwise flip every frame.
	glyphing map[int]bool

	// solid is the board seen from an angle, when it is. Not a diversion: it
	// refines the file.
	solid *Solid

	// tint is every visible digit's colour and how far through a change it is,
	// so a cursor arriving on a cell fades it up.
	tint *Tint

	// lastFile is the file this refiner last left, which the files drawer
	// comes back up on.
	lastFile string

	// finaleHeard is whether the music for a finished file is playing.
	finaleHeard bool

	// blastHeard is the last detonation this view has sounded.
	blastHeard int

	// pointerOnNumbers is whether the pointer was last over the board's
	// numbers, for the browser's sound of them.
	pointerOnNumbers bool

	// hoveredGroup is the awake cluster the cursor is on, or -1. Hovering makes a
	// cluster "super active" and visibly more agitated.
	hoveredGroup int

	// held is every cluster this refiner has ringfenced; Enter refines the lot.
	// A sweep takes everything corrupt it crosses. Per player, so two refiners
	// at one board can hold different sets.
	held map[int]bool

	// sweep is the ringfence being drawn right now: the anchor cell and wherever
	// the pointer or cursor has got to. Nil when nothing is being swept. The
	// rectangle is drawn only while sweeping; held clusters show by being large
	// and lifted.
	sweep *sweepBox

	// zoom is the fractional level the board is drawn at, travelling towards
	// Layout.WantZoom on a spring, since a spacing that jumps three columns
	// moves a digit thirty screen columns in one frame. It is on Model, as
	// Rig holds wantDolly, because Layout is rebuilt whole on every resize and
	// stays passive geometry a test can set directly.
	zoom *physics.Scalar

	// viewRng drives everything cosmetic: the scramble in a refilling cell, the
	// screensaver. The board has its own stream, so redraw rate cannot move it.
	viewRng  *rand.Rand
	Quitting bool
	// keepOpen is for an embedded browser session: navigation may leave a file,
	// but keyboard input must never shut down its host runtime.
	keepOpen bool
	// touchScreen is a browser with no keys to press: its hints speak of taps.
	touchScreen bool
	// wipeAsk is the browser lobby asking whether to wipe the saved progress,
	// and wipe the answer yes, for the host to act on (WipeRequested).
	wipeAsk, wipe bool

	// lan is the host and join panel's network side, nil for anybody who is not
	// at their own terminal. See lan.go.
	lan   LAN
	lanUI lanPanel

	// lanShown is the request last shown to this host, and lanShownAt when it
	// came up. They are not on the panel, which is reset as it opens and closes.
	lanShown   uint64
	lanShownAt time.Time

	// joined is somebody a LAN host let in. Unlike askName, they have no name
	// screen to go back to.
	joined bool
	// exitOpen asks whether to return to the files or end this terminal.
	// leaveForLobby is completed after Update releases the current room lock.
	exitOpen, exitQuit, leaveForLobby bool
	calloutOpen                       bool
	calloutAt                         int

	// idle is how long nobody has touched this terminal. See idle.go.
	idle idleWatch

	// badge is the name this terminal holds on the floor, once it has given
	// one, and nameNote what the name screen says beneath the field: a welcome
	// back or, when nameRefused, why the name cannot be had. See roll.go.
	badge       *Badge
	nameNote    string
	nameRefused bool

	// key is the fingerprint of the client key this terminal connected with,
	// and empty for one that offered none. See recognise.go.
	key string
}

// NewModel builds a model from a seed. The seed fixes the digits, each cluster's
// target bin, and the agitation; which cells are corrupt comes from Perlin and
// is the same in every session.
func NewModel(seed int64) *Model {
	return JoinRoom(NewRoom(seed), seed)
}

// JoinRoom builds a terminal onto an existing board, or returns nil if every
// desk is taken. The seed fixes only what belongs to this player (the scramble,
// the screensaver, the boot log); the board's decisions come from the room's
// stream, so everyone at a board agrees about it.
func JoinRoom(room *Room, seed int64) *Model {
	m := newView(seed)
	if !m.Join(room) {
		return nil
	}

	// A constructed terminal is already at its board, so the next frame draws
	// the board, not the first frame of an arrival. A local run started with
	// --file gets its arrival from finishIntro when the grid is first shown.
	m.arrival = nil
	return m
}

// newView builds a terminal with no board behind it yet. A lobby session plays
// the boot sequence, asks for a name and lists the rooms before taking a desk.
func newView(seed int64) *Model {
	viewRng := rand.New(rand.NewSource(seed ^ 0x5EA5CA1E))

	m := &Model{
		soundRng:     rand.New(rand.NewSource(seed ^ 0xBEE9)),
		Layout:       NewLayout(80, 24),
		Clock:        physics.NewClock(),
		Agitation:    NewAgitation(viewRng),
		Drift:        NewDrift(),
		ripple:       NewRipple(),
		Magnify:      NewMagnify(),
		Lens:         NewLens(),
		remoteLens:   map[int]*Lens{},
		targets:      make(map[int]float64, 64),
		reachSeat:    make(map[int]int, MaxDesks),
		hoveredGroup: -1,
		held:         map[int]bool{},
		enlarged:     make(map[int]bool, 32),
		glyphing:     make(map[int]bool, 32),
		tint:         NewTint(),
		Flights:      NewFlights(),
		BinFill:      NewBinFill(),
		Drawer:       NewDrawer(),
		Theme:        theme.DefaultTheme,
		Regen:        NewRegen(viewRng),
		viewRng:      viewRng,
	}
	m.Drift.Sync(m.Layout)
	m.Session = sessionID(viewRng)
	return m
}

// Join takes a desk at a board, and reports whether there was one going.
//
// It must not be called while this player holds a board's lock: it takes the
// new room's lock to sit down and the old one's to stand up, and a Go RWMutex
// is not reentrant. The lobby is only reachable before taking a desk, so in
// practice no lock is held.
func (m *Model) Join(room *Room) bool {
	if room == nil {
		return false
	}
	desk := room.Seat(m.Name)
	if desk == nil {
		return false
	}

	m.Leave()

	// The board is read under its lock: other refiners are stepping it.
	room.mu.RLock()
	m.Room, m.Desk = room, desk
	m.File = NewFileNamed(room.FileName)
	m.seenEpoch = room.Epoch()
	m.boardTime = room.stepped

	m.freshBoard()
	m.catchUp(room)

	m.settleOnBoard()
	room.mu.RUnlock()
	if m.rooms != nil {
		m.rooms.news.PostArrival(m.Name, m.File.Name, m.Desk.Seat)
	}
	if m.allowBots && m.company > 0 {
		room.addBots(m.company)
	}
	return true
}

// Watch takes a place in the queue at a full board and shows it. A watcher
// sees everything a refiner sees; refineAtCursor refuses a nil desk.
func (m *Model) Watch(room *Room) bool {
	if room == nil {
		return false
	}
	w := room.Wait(m.Name)
	if w == nil {
		return false
	}

	m.Leave()

	room.mu.RLock()
	defer room.mu.RUnlock()
	m.Room, m.Desk, m.watch = room, nil, w
	m.watchAt, m.watchText = -2, ""
	m.File = NewFileNamed(room.FileName)
	m.seenEpoch = room.Epoch()
	m.boardTime = room.stepped

	m.freshBoard()
	m.catchUp(room)
	m.settleOnBoard()
	return true
}

// settleOnBoard centres the cursor in the viewport and starts the arrival,
// whose wavefront starts in the middle. It scrolls to the origin first, so the
// centre is not measured against wherever the last file left the viewport.
func (m *Model) settleOnBoard() {
	m.Layout.ScrollTo(0, 0)
	m.CursorX = m.Layout.ViewportX + m.Layout.ViewportW/2
	m.CursorY = m.Layout.ViewportY + m.Layout.ViewportH/2
	m.focusLens()
	m.syncAgitation()
	m.arrive()
	m.Screen = ScreenMain
}

// arrive starts the board coming up. Other places set Screen = ScreenMain
// (waking from the screensaver, leaving the scoreboard) but only these are
// arrivals:
//
//   - settleOnBoard, from Join and Watch;
//   - followFile, when the room rotates a finished file out from under you;
//   - finishIntro, for a local run. JoinRoom calls Join before the boot
//     sequence plays, so an arrival started then would finish behind the boot
//     log.
func (m *Model) arrive() { m.arrival = NewArrival(m.Layout) }

// Leave gives up this player's desk, if they have one.
func (m *Model) Leave() {
	if m.Room != nil {
		m.lastFile = m.Room.FileName
		m.Room.Unseat(m.Desk)
		m.Room.Unwait(m.watch)
		if m.spectating {
			m.Room.unspectate()
		}
	}
	m.watch, m.spectating = nil, false
	// Drop the board too, or this session would go on locking and stepping a
	// board it had left.
	m.Room, m.Desk = nil, nil
}

// followFile drops everything this view held of a file the room has finished,
// and starts the next file's arrival. The bar easing up from nothing reads as
// the terminal loading the file.
func (m *Model) followFile() {
	if m.Room == nil || m.seenEpoch == m.Room.Epoch() {
		return
	}
	m.seenEpoch = m.Room.Epoch()
	m.File = NewFileNamed(m.Room.FileName)

	m.freshBoard()
	m.syncAgitation()
	m.arrive()
}

// freshBoard drops everything this terminal was animating, so a board arrives
// clean instead of painted with the last board's leftovers. Join, Watch and
// followFile all use it.
func (m *Model) freshBoard() {
	m.Magnify = NewMagnify()
	m.Flights = NewFlights()
	m.BinFill = NewBinFill()
	m.Drawer = NewDrawer()
	m.zoom = physics.NewScalar(m.Layout.Zoom(), zoomFrequency)
	m.Regen = NewRegen(m.viewRng)
	m.Agitation = NewAgitation(m.viewRng)
	m.hoveredGroup = -1
	clear(m.held)
	m.sweep = nil
	clear(m.enlarged)
	clear(m.glyphing)
	m.tint = NewTint()
	m.dropIndex()
}

// catchUp takes the board's event sequences at the door, so a newcomer does not
// replay blasts and deposits that happened before they arrived.
func (m *Model) catchUp(room *Room) {
	if room == nil {
		return
	}
	m.ripple.Missed(room.blastSeq)
	m.blastHeard = room.blastSeq
	m.depositSeen = room.depositSeq
}

// deskColour is the shade this terminal draws a desk in. It comes from the
// viewer's theme, never the Desk, so one player's palette choice does not leak
// onto somebody else's screen.
func (m *Model) deskColour(d *Desk) canvas.Color {
	if d == nil {
		return m.Theme.Primary
	}
	return m.Theme.DeskColour(d.Seat)
}

// publish puts this player's cursor and bin on their desk, so the other people
// in the room can see where they are working. Called under the board's lock.
func (m *Model) publish() {
	if m.Desk == nil {
		return
	}
	m.Desk.CursorX, m.Desk.CursorY = m.CursorX, m.CursorY
	m.Desk.Bin = m.SelectedBin
	l := m.Layout
	m.Desk.ViewX, m.Desk.ViewY = l.ViewportX, l.ViewportY
	m.Desk.ViewW, m.Desk.ViewH = l.ViewportW, l.ViewportH

	// What this refiner is reaching for, so every other screen can lift it too.
	// Truncated and re-appended, since this runs every frame of every session.
	m.Desk.Reach = m.Desk.Reach[:0]
	if m.hoveredGroup >= 0 {
		m.Desk.Reach = append(m.Desk.Reach, m.hoveredGroup)
	}
	for id := range m.held {
		if id != m.hoveredGroup {
			m.Desk.Reach = append(m.Desk.Reach, id)
		}
	}

	// Reaching for a cluster is also how the room learns it has been found, and
	// finding one is what puts it on everybody's map. See Room.Found.
	for _, id := range m.Desk.Reach {
		m.Room.Found(id)
	}
}

// syncAgitation brings this player's copy of the shake into line with the
// clusters the board says are awake. Each view does this for itself, on its own
// goroutine, holding the board's lock; the board never touches a player's
// simulation, which another goroutine may be drawing with.
func (m *Model) syncAgitation() {
	if m.Room == nil {
		return
	}
	awake := make(map[int]bool, len(m.Room.actives))
	for _, c := range m.Room.actives {
		awake[c.grp.ID] = true
		if !m.Agitation.Tracked(c.grp.ID) {
			m.Agitation.Track(c.grp, m.Grid)
			m.Agitation.SetIntensity(c.grp.ID, activeIntensity)
		}
	}
	for _, id := range m.Agitation.TrackedIDs() {
		if !awake[id] {
			m.Agitation.Release(id)
		}
	}
}

// NumbersShowing reports whether the board's numbers are on screen: a file
// open and unfinished, with no diversion in their place.
func (m *Model) NumbersShowing() bool {
	return m.Screen == ScreenMain && m.Room != nil && !m.Room.Complete() &&
		!m.Diverting() && (m.mark == nil || !m.mark.Fullscreen())
}

// PointerOnNumbers reports whether the pointer is over the numbers showing.
func (m *Model) PointerOnNumbers() bool { return m.pointerOnNumbers && m.NumbersShowing() }

// PointerLeft is the host saying the pointer has left the screen.
func (m *Model) PointerLeft() { m.pointerOnNumbers = false }

// StartOff puts the model on a dark set that any key or click switches on,
// starting the boot sequence.
func (m *Model) StartOff() { m.Screen = ScreenOff }

// switchOn starts the boot sequence on a set just switched on: a moment of
// cursor in the corner, then the log.
func (m *Model) switchOn() {
	m.StartIntro()
	m.Intro.WakeFirst()
}

// StartIntro puts the model on the boot sequence instead of straight into the
// grid.
func (m *Model) StartIntro() {
	m.Intro = NewIntro(m.viewRng)
	if m.logoLength > 0 {
		m.Intro.FitLogo(m.logoLength.Seconds())
	}
	m.Screen = ScreenIntro
}

// FitIntroTo makes every boot sequence's logo last d, morphing smoothly
// between its frames. The browser fits it to the startup sound, which plays
// as the logo starts.
func (m *Model) FitIntroTo(d time.Duration) { m.logoLength = d }

// ShowingLogo reports whether the boot sequence is drawing the Lumon logo.
func (m *Model) ShowingLogo() bool {
	return m.Screen == ScreenIntro && m.Intro != nil && m.Intro.Stage() == stageLogo
}

// Resize recomputes the layout, preserving the scroll position as far as the new
// viewport allows.
func (m *Model) Resize(width, height int) {
	x, y := m.Layout.ViewportX, m.Layout.ViewportY
	want := m.Layout.WantZoom()
	m.Layout = NewLayout(width, height)

	// The spacing is this terminal's choice, so it survives a resize like the
	// scroll position does. A zoom in progress lands at its target.
	m.Layout.applyZoom(want)
	m.Layout.aimZoom(want)
	m.Layout.recompute()
	m.Layout.ScrollTo(x, y)
	if m.zoom != nil {
		m.zoom.Set(want)
	}

	if m.idleMark != nil {
		m.idleMark.Resize(m.idlePanel())
	}
	m.afterLayout()
}

// Zoom spreads the board out, or brings it back in, by delta levels (only ever
// in from the default; see zoomLevels). The spacing is per terminal, so this
// touches neither the Room nor its lock.
func (m *Model) Zoom(delta int) {
	want := clampF(math.Round(m.Layout.WantZoom())+float64(delta), 0, float64(m.Layout.MaxZoom()))
	if want == m.Layout.WantZoom() {
		return
	}
	m.Layout.aimZoom(want)

	m.dropIndex()
}

// SetZoomNow respaces the board without travelling, for a resize and for tests.
func (m *Model) SetZoomNow(z float64) {
	if !m.Layout.SetZoom(z) {
		return
	}
	m.zoom.Set(m.Layout.Zoom())
	m.afterLayout()
}

// zooming reports whether the spacing is still travelling.
func (m *Model) zooming() bool {
	return math.Abs(m.zoom.Value()-m.Layout.WantZoom()) > zoomSettled ||
		math.Abs(m.zoom.Velocity()) > zoomStill
}

// stepZoom carries the board a frame further into its new spacing. Only the
// cells move: the chrome is measured against the default-spacing board
// rectangle. physics.Scalar is critically damped, so the zoom eases in and out
// with only the frequency to tune.
func (m *Model) stepZoom(dt float64) {
	if !m.zooming() {
		return
	}
	l := m.Layout
	m.zoom.Step(dt, l.WantZoom())

	if !m.zooming() {
		// Land exactly on the level, so the spacing matches the level table
		// that tests and cell-derived constants assume.
		m.zoom.Set(l.WantZoom())
		l.applyZoom(l.WantZoom())
		l.respace(m.CursorX, m.CursorY)
		m.arriveZoom()
		return
	}

	l.applyZoom(m.zoom.Value())
	l.respace(m.CursorX, m.CursorY)
	m.scrollToCursor()
	m.refitLenses()
}

// arriveZoom is the work done once when a zoom lands, not every frame on the way.
func (m *Model) arriveZoom() {
	m.scrollToCursor()

	// Not Agitation.ResetSteps: that is for a discontinuous respace. An eased
	// zoom keeps the column commitments current, since columnStep re-derives
	// them at the live spacing every frame. Resize still calls it, through
	// afterLayout.
	m.resizeDiversion()
	m.focusLens()
	m.dropIndex()
}

// refitLenses gives every lens the reach of the board as it is now. Fit copies
// the radii, which scale with the cell; focusLens would also Warp the focus,
// which every frame would drag a pointer that has moved away back onto the
// cursor's cell.
func (m *Model) refitLenses() {
	m.Lens.Fit(m.Layout)
	for _, l := range m.remoteLens {
		l.Fit(m.Layout)
	}
	m.reprojectLenses()
}

// afterLayout brings everything measured against the old screen into line with
// the new one, after a resize or a zoom alike. The order matters:
// scrollToCursor first, because everything below reads ScreenOf against the
// viewport it moves; dropIndex last, so nothing can re-stamp the index behind
// it.
func (m *Model) afterLayout() {
	// A terminal may drop its images when the screen changes, and Bubble Tea
	// skips unchanged lines, so resend the mark. See MarkRefresh.
	m.markSent = false

	m.scrollToCursor()
	m.Drift.Sync(m.Layout)
	m.Agitation.ResetSteps()
	if m.ripple != nil {
		m.ripple.Sync(m.Layout)
	}
	// Re-measured against the new viewport, but its clock is left alone: a
	// resize part way through must not replay the arrival.
	m.arrival.Sync(m.Layout)
	m.resizeDiversion()
	m.focusLens()
	m.dropIndex()
}

// focusLens puts the lens on the cursor at once, with no travel. Springing it
// across the screen after a resize or a scroll would read as a swipe.
func (m *Model) focusLens() {
	m.Lens.Fit(m.Layout)
	sx, sy := m.Layout.ScreenOf(m.CursorX, m.CursorY)
	m.Lens.Warp(float64(sx), float64(sy))
	m.lastCursorX, m.lastCursorY = m.CursorX, m.CursorY

	m.reprojectLenses()
}

// reprojectLenses moves everybody else's lens to wherever their cell now falls,
// so a resize or scroll does not make their lens spring across the board. Each
// lens remembers its own cell, so this reads nothing of the room and needs no
// lock.
func (m *Model) reprojectLenses() {
	for _, l := range m.remoteLens {
		l.Fit(m.Layout)
		l.Reproject(m.Layout)
	}
}

// lenses is every fisheye bulging this board: this refiner's, and one for each
// of the others.
func (m *Model) lenses() []*Lens {
	// Rebuilt by trackRemoteLenses once a frame. The guard is for the paths that
	// draw without stepping first, which the tests do.
	if len(m.lensList) == 0 || m.lensList[0] != m.Lens {
		m.rebuildLenses()
	}
	return m.lensList
}

// rebuildLenses refreshes the frame's lens list in seat order, reusing the
// backing array.
func (m *Model) rebuildLenses() {
	m.lensList = append(m.lensList[:0], m.Lens)
	for seat := 0; seat < MaxDesks; seat++ {
		if l := m.remoteLens[seat]; l != nil {
			m.lensList = append(m.lensList, l)
		}
	}
}

// trackRemoteLenses keeps one lens per other refiner, aimed at where they are
// looking. Cursors are published in grid cells, so each viewer projects them
// through its own layout.
func (m *Model) trackRemoteLenses() {
	if m.Room == nil {
		clear(m.remoteLens)
		m.rebuildLenses()
		return
	}

	seen := map[int]bool{}
	for _, d := range m.Roster() {
		if d == m.Desk {
			continue
		}
		seen[d.Seat] = true

		l, ok := m.remoteLens[d.Seat]
		if !ok {
			// A new lens starts where its owner is, so it does not sweep in.
			l = NewLens()
			sx, sy := m.Layout.ScreenOf(d.CursorX, d.CursorY)
			l.Warp(float64(sx), float64(sy))
			m.remoteLens[d.Seat] = l
		}
		sx, sy := m.Layout.ScreenOf(d.CursorX, d.CursorY)
		l.AimAtCell(d.CursorX, d.CursorY, float64(sx), float64(sy))
	}

	for seat := range m.remoteLens {
		if !seen[seat] {
			delete(m.remoteLens, seat)
		}
	}
	m.rebuildLenses()
}

// Intensity levels for a cluster's shake: activeIntensity when it wakes,
// furiousIntensity if left alone, and superActiveIntensity while hovered.
const (
	activeIntensity      = 0.45
	furiousIntensity     = 0.85
	superActiveIntensity = 1.0
)
