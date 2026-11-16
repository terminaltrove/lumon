package lumon

import (
	"fmt"
	"github.com/terminaltrove/lumon/internal/ease"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/terminaltrove/lumon/internal/physics"
)

// defaultDeskName is what a nameplate reads before anyone types a name.
func defaultDeskName(seat int) string { return fmt.Sprintf("REFINER %d", seat+1) }

// Room is the board itself: everything the people working it must agree on.
// It is not called World because physics.World is used throughout this
// package.
//
// A Model embeds a *Room and reads the board by promotion (m.Grid,
// m.Actives(), m.Awake(id)). A solo room holds one desk; a shared room up to
// MaxDesks.
type Room struct {
	// mu guards everything below. Bubble Tea calls Update and then View on one
	// goroutine per player; a player holds the write lock for all of Update
	// and the read lock for all of View, so the frame drawn is the
	// board just simulated.
	mu sync.RWMutex

	// stepped is how far the board's own clock has been carried, in seconds of
	// simulated time. See StepTo.
	stepped float64

	Grid *Grid

	// FileName is the macrodata file this room is refining. History is the ones
	// it has already finished, oldest first.
	FileName string
	History  []string

	// epoch is which file this is, counting from zero, and picks the board's
	// layout. A view compares it to notice the room has moved on.
	epoch int

	// holding is how long the finished file has been held on screen before the
	// room takes the next one, or -1 while there is still work to do.
	holding float64

	// hold is how long a finished file is held, in seconds, or 0 for
	// FileHoldSeconds. See Rooms.HoldFinishedFiles.
	hold float64

	// actives are the clusters currently awake, ordered by ID so the order
	// never depends on map iteration. There are up to hunts of them.
	actives    []*activeCluster
	activeByID map[int]*activeCluster

	// desks is how many refiners the room holds, and hunts how many corrupt
	// clusters are awake for them (HuntPerRefiner each).
	desks int
	hunts int

	// justRefined remembers clusters refined a moment ago, so somebody pressing
	// enter on the same cluster a fraction of a second later is credited too.
	// The bin is still credited once; only the personal tallies double up.
	justRefined map[int]*graceEntry

	// tally is who has refined how much of this file, by name. rotate folds it
	// into the ledger and clears it.
	tally map[string]int

	// ledger is the server-wide standings. NewRoom opens its own so a solo
	// board keeps a tally; Rooms.Open replaces it with the registry's, so
	// standings survive changing files.
	ledger *Ledger

	// seed is what this room was opened with, so a restored room draws the same
	// stream of file names it would have.
	seed int64

	// gridSeed is what this file's board was generated from, so it can be
	// rebuilt on restore.
	gridSeed int64

	// creditIn holds bins waiting to be credited. A refinement credits its bin
	// after a fixed interval of simulated time, because when a digit lands
	// depends on each player's screen.
	creditIn []pendingCredit

	// activateIn counts frames until the next cluster lights up.
	activateIn int

	// blasts are the detonations still travelling, blastTime the clock they are
	// stamped against, and lost the clusters that detonated instead of being
	// refined. They are on the board so every view agrees. A save records which
	// clusters have left, not why; lost tells a restored file which quota was
	// written off, or the file could never be finished.
	blasts    []Blast
	blastTime float64
	blastSeq  int
	lost      []int

	// deposits are the clusters that have just left for a bin, stamped against
	// blastTime, so every view flies the same refinement. See deposit.go.
	deposits   []Deposit
	depositSeq int

	// rng makes the board's decisions: replacement digits, which cluster wakes,
	// how long until the next one. Cosmetic randomness uses a view's own
	// stream, so redraw rate can never change the board.
	rng *rand.Rand

	// emptyAt is when the last person left, so the registry can reclaim a room
	// that nobody has come back to. Zero while anyone is here.
	emptyAt time.Time

	// desk is who is at the board. An empty seat is nil, so nameplates keep
	// their places when somebody leaves.
	desk [MaxDesks]*Desk

	// watchers are the people waiting for a desk, in arrival order. See
	// watch.go.
	watchers []*Watcher

	// found is every cluster somebody has had under their cursor on this file,
	// for the map. rotate clears it. See Room.Found.
	found map[int]bool

	// seated is how many people are at the board.
	//
	// The room holds no reference to any player's state. It owns only the set
	// of awake clusters; each view reconciles its own spring-mass bodies to
	// that set on its own goroutine (Model.syncAgitation). Touching another
	// player's bodies from here would race with their drawing.
	seated int

	// bots are the computer refiners at this board, in the order they sat
	// down, and spectators the people who got up to watch them. See bot.go.
	bots       []*bot
	spectators int

	// lastBot is the seat of the computer refiner that refined last, for a
	// spectator's camera to follow.
	lastBot int
}

// Desk is one refiner's place at the board: who they are, where they are
// looking, which bin they are feeding, and how much they have put in it. A view
// publishes to its own desk each frame and reads everybody else's.
type Desk struct {
	Name string
	// Callout is a fixed phrase shown by every view above this cursor until the
	// shared board time reaches CalloutUntil.
	Callout      string
	CalloutUntil float64

	// Seat is which of the room's places this is, 0-based. It fixes the
	// nameplate's position and colour.
	Seat int

	// CursorX and CursorY are where this refiner is looking, in grid cells.
	CursorX, CursorY int

	// ViewX, ViewY, ViewW and ViewH are the window of the board this refiner has
	// on their screen, in grid cells, so the map can show what they see.
	ViewX, ViewY, ViewW, ViewH int

	// Bin is the bin they are feeding, 0-based, and Credit how many clusters
	// they have refined. Two people refining the same cluster within the
	// co-refine window are both credited, while the file counts it once.
	Bin    int
	Credit int

	// Bot marks a computer refiner's desk. See bot.go.
	Bot bool

	// Reach is every cluster this refiner is pointing at or has ringfenced.
	// Reaching enlarges a cluster to full size, so it is published for every
	// view to draw the same.
	//
	// Written by Model.publish and read by other sessions in glyphTargets, both
	// under the room's write lock. The slice is truncated and reused, so a frame
	// allocates nothing.
	Reach []int
}

// Seat takes a place at the board under the given name and opens a desk for
// it. An empty name gets the default for that seat. It returns nil if the room
// is full or someone is queued.
func (r *Room) Seat(name string) *Desk {
	r.mu.Lock()
	defer r.mu.Unlock()

	// A free desk belongs to the head of the queue, who is promoted on their
	// next frame; an arrival from the lobby must not take it first.
	if len(r.watchers) > 0 {
		return nil
	}
	return r.seatPersonLocked(name)
}

// seatPersonLocked seats a person, sending a computer refiner home if that is
// the only way to make room. The caller holds the lock.
func (r *Room) seatPersonLocked(name string) *Desk {
	if r.seated >= MaxDesks {
		r.sendOneBotHome()
	}
	return r.seatLocked(name)
}

// seatLocked is Seat with the lock already held and the queue already
// consulted. Promotion happens inside a session's Update, which holds the write
// lock, and a Go RWMutex is not reentrant.
func (r *Room) seatLocked(name string) *Desk {
	if r.seated >= MaxDesks {
		return nil
	}
	for i := range r.desk {
		if r.desk[i] != nil {
			continue
		}
		if name == "" {
			name = defaultDeskName(i)
		}
		// Set the name here, under the lock. The desk is visible to other
		// Views as soon as Seat returns, so setting it afterwards is a race.
		d := &Desk{Name: name, Seat: i}
		r.desk[i] = d
		r.seated++
		r.emptyAt = time.Time{}
		r.setDesks(r.seated)
		return d
	}
	return nil
}

// Unseat gives a place back, closing the desk and putting its cluster down.
func (r *Room) Unseat(d *Desk) {
	if d == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.unseatLocked(d)
	r.botsLeaveIfAlone()
}

// unseatLocked is Unseat with the lock held.
func (r *Room) unseatLocked(d *Desk) {
	if d == nil || d.Seat < 0 || d.Seat >= MaxDesks || r.desk[d.Seat] != d {
		return
	}
	r.desk[d.Seat] = nil
	if r.seated > 0 {
		r.seated--
	}
	r.markEmpty()
	r.setDesks(r.seated)
}

// markEmpty starts or stops the idle clock the registry reclaims a room by.
// Watchers count as occupants: reclaiming their room would strand them on a
// board the registry has forgotten.
func (r *Room) markEmpty() {
	if r.seated == 0 && len(r.watchers) == 0 && r.spectators == 0 {
		r.emptyAt = time.Now()
		return
	}
	r.emptyAt = time.Time{}
}

// Roster returns everyone at the board, in seat order. The caller holds the
// lock, which the render path does.
func (r *Room) Roster() []*Desk {
	out := make([]*Desk, 0, MaxDesks)
	for _, d := range r.desk {
		if d != nil {
			out = append(out, d)
		}
	}
	return out
}

// Seated reports how many people are at the board. The caller holds the lock.
func (r *Room) Seated() int { return r.seated }

// activeCluster is one awake cluster: how long it has been ignored, and where it
// is in its swell.
type activeCluster struct {
	grp     *BadGroup
	elapsed float64
	breathe *physics.Scalar
	rising  bool
}

// NewRoom opens a board. The seed fixes the digits and the file; which cells are
// corrupt comes from Perlin and is the same in every room.
func NewRoom(seed int64) *Room {
	rng := rand.New(rand.NewSource(seed))
	return newRoom(seed, rng, fileName(rng))
}

// NewRoomNamed is NewRoom with the file name chosen, for opening a file from
// the directory. The board is still a function of the seed alone, and rotate
// draws the next name from the pool as usual.
func NewRoomNamed(seed int64, name string) *Room {
	rng := rand.New(rand.NewSource(seed))
	if name == "" {
		name = fileName(rng)
	}
	return newRoom(seed, rng, name)
}

func newRoom(seed int64, rng *rand.Rand, name string) *Room {
	r := &Room{
		Grid:       NewGrid(seed),
		seed:       seed,
		gridSeed:   seed,
		FileName:   name,
		activeByID: make(map[int]*activeCluster),
		desks:      1,
		activateIn: 30,
		holding:    -1,
		rng:        rng,

		// Rooms.Open swaps in the registry's ledger.
		ledger: NewLedger(),
	}
	r.dressGrid()
	return r
}

// Ledger is the standings this room credits. Never nil.
func (r *Room) Ledger() *Ledger { return r.ledger }

// Tally is who has refined how much of the file being worked now.
func (r *Room) Tally() map[string]int {
	out := make(map[string]int, len(r.tally))
	for k, v := range r.tally {
		out[k] = v
	}
	return out
}

// FileStandings is this file's attribution, best first.
func (r *Room) FileStandings() []Standing {
	out := make([]Standing, 0, len(r.tally))
	for name, n := range r.tally {
		out = append(out, Standing{Name: name, Refined: n})
	}
	sortStandings(out)
	return out
}

// DeskCount reports how many refiners the room currently holds, and HuntCount
// how many corrupt clusters it keeps awake for them.
func (r *Room) DeskCount() int { return r.desks }
func (r *Room) HuntCount() int { return r.hunts }

// SetDesks changes the room's size, putting surplus clusters back to sleep if it
// shrank.
func (r *Room) SetDesks(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setDesks(n)
}

// HuntPerRefiner is how many corrupt clusters are awake for each person at the
// board, so there is a choice of which to go for. MaxAwake is the ceiling on
// the room; it is separate from MaxDesks, the seat cap.
const (
	HuntPerRefiner = 2
	MaxAwake       = MaxDesks * HuntPerRefiner
)

// setDesks assumes the lock. n is the headcount; the wake cap follows from it.
func (r *Room) setDesks(n int) {
	if n < 1 {
		n = 1
	}
	if n > MaxDesks {
		n = MaxDesks
	}
	r.desks = n
	r.hunts = n * HuntPerRefiner
	if r.hunts > MaxAwake {
		r.hunts = MaxAwake
	}
	r.retireSurplusActives()
}

// Activate wakes a cluster. Each view picks it up on its own next frame.
func (r *Room) Activate(grp *BadGroup) {
	if grp == nil || r.activeByID[grp.ID] != nil {
		return
	}

	c := &activeCluster{
		grp: grp,
		// A new cluster starts small and is watched swelling.
		breathe: physics.NewScalar(PlainHeight, magnifyFrequency),
		rising:  true,
	}
	r.actives = append(r.actives, c)
	r.activeByID[grp.ID] = c
	sort.Slice(r.actives, func(i, j int) bool { return r.actives[i].grp.ID < r.actives[j].grp.ID })

	r.retireSurplusActives()
}

// retireSurplusActives puts the longest-awake clusters back to sleep when there
// are more of them than desks to work them.
func (r *Room) retireSurplusActives() {
	for len(r.actives) > r.hunts && len(r.actives) > 0 {
		oldest := 0
		for i, c := range r.actives {
			if c.elapsed > r.actives[oldest].elapsed {
				oldest = i
			}
		}
		r.Sleep(r.actives[oldest].grp.ID)
	}
}

// Sleep puts a cluster back to sleep. Each view releases its own body.
func (r *Room) Sleep(groupID int) {
	c := r.activeByID[groupID]
	if c == nil {
		return
	}
	delete(r.activeByID, groupID)
	for i, other := range r.actives {
		if other == c {
			r.actives = append(r.actives[:i], r.actives[i+1:]...)
			break
		}
	}
}

// IsActive reports whether a cluster is awake and so refinable.
func (r *Room) IsActive(groupID int) bool {
	return groupID >= 0 && r.activeByID[groupID] != nil
}

// FirstActive is the lowest-numbered awake cluster, or nil.
func (r *Room) FirstActive() *BadGroup {
	if len(r.actives) == 0 {
		return nil
	}
	return r.actives[0].grp
}

// Actives returns every awake cluster, lowest ID first.
func (r *Room) Actives() []*BadGroup {
	out := make([]*BadGroup, 0, len(r.actives))
	for _, c := range r.actives {
		out = append(out, c.grp)
	}
	return out
}

// ScaryElapsed reports how long the longest-ignored awake cluster has been
// waiting, in seconds.
func (r *Room) ScaryElapsed() float64 {
	worst := 0.0
	for _, c := range r.actives {
		if c.elapsed > worst {
			worst = c.elapsed
		}
	}
	return worst
}

// Awake reports how long a particular cluster has been ignored.
func (r *Room) Awake(groupID int) float64 {
	if c := r.activeByID[groupID]; c != nil {
		return c.elapsed
	}
	return 0
}

// swellOf is how large a cluster's shared swell currently is, in rows.
func (r *Room) swellOf(groupID int) float64 {
	if c := r.activeByID[groupID]; c != nil {
		return c.breathe.Value()
	}
	return PlainHeight
}

// escalation is how far a cluster has escalated, 0 to 1, on its own clock.
func (r *Room) escalation(groupID int) float64 {
	return ease.Smoothstep(ease.Clamp01(r.Awake(groupID) / EscalateSeconds))
}

// worstEscalation is how far the most-ignored awake cluster has got.
func (r *Room) worstEscalation() float64 {
	return ease.Smoothstep(ease.Clamp01(r.ScaryElapsed() / EscalateSeconds))
}

// sizeRamp is how far up its swell a cluster is, from 0 to 1. It runs on
// HintSeconds, separate from escalation, which paces the colour and the shake.
func (r *Room) sizeRamp(groupID int) float64 {
	return ease.Smoothstep(ease.Clamp01(r.Awake(groupID) / HintSeconds))
}

// breatheBand is the trough and crest a cluster is currently working between.
func (r *Room) breatheBand(groupID int) (low, high float64) {
	e := r.sizeRamp(groupID)
	return BreatheLowCalm + e*(BreatheLow-BreatheLowCalm),
		BreatheHighCalm + e*(BreatheHigh-BreatheHighCalm)
}

// Step advances the board by dt seconds: ages the awake clusters, moves their
// swell, and pays out credit that has come due. Waking clusters needs to know
// where people are looking, so it is a separate call (MaybeActivate).
func (r *Room) Step(dt float64) {
	for _, c := range r.actives {
		c.elapsed += dt
	}
	r.stepBlasts(dt)
	r.stepDeposits()
	r.burnFuses()
	r.advanceBreathe(dt)
	r.advanceCredits(dt)
	r.expireGrace(dt)
	r.advanceFile(dt)
	r.stepBots(dt)
}

// StepTo advances the board to time to, in seconds, and does nothing if another
// session has already taken it further.
//
// Every session calls this each frame, so stepping by each session's dt would
// make the board run faster with more people. Advancing to the furthest any
// session has reached keeps one rate for one refiner or four, and still lets any
// session drive the clock while another idles.
func (r *Room) StepTo(to float64) {
	if to <= r.stepped {
		return
	}
	dt := to - r.stepped
	r.stepped = to
	r.Step(dt)
}

// BoardTime is how far the board's clock has been advanced, in seconds. A
// session starts its own clock here when it sits down.
func (r *Room) BoardTime() float64 { return r.stepped }

// FileHoldSeconds is how long a finished file is held before the room takes the
// next one. Long enough for the bins' fireworks programme and for everyone to
// read the notice naming who did the work.
const FileHoldSeconds = 12.0

// HoldSeconds is how long this room holds a finished file.
func (r *Room) HoldSeconds() float64 {
	if r.hold > 0 {
		return r.hold
	}
	return FileHoldSeconds
}

// Epoch is which file the room is on, counting from zero. A view compares it
// against its own to notice the room has moved on.
func (r *Room) Epoch() int { return r.epoch }

// CurrentFile is the name of the file on the board, read under its lock, for
// callers that do not hold it.
func (r *Room) CurrentFile() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.FileName
}

// Complete reports whether the file is finished and being held before the next
// one. The caller holds the lock.
func (r *Room) Complete() bool { return r.holding >= 0 }

// Held is how long the finished file has been on screen.
func (r *Room) Held() float64 {
	if r.holding < 0 {
		return 0
	}
	return r.holding
}

// advanceFile notices a finished file, holds it, and then takes the next one.
func (r *Room) advanceFile(dt float64) {
	if r.holding < 0 {
		if r.percent() < 100 {
			return
		}
		// Just finished: put the clusters to sleep so the board is still.
		r.holding = 0
		for _, grp := range r.Actives() {
			r.Sleep(grp.ID)
		}
		return
	}

	r.holding += dt
	if r.holding >= r.HoldSeconds() {
		r.rotate()
	}
}

// rotate files the finished work and opens the next one, with a new board
// layout for the new file.
func (r *Room) rotate() {
	// Record the file's attribution on the ledger before the tally is cleared.
	if r.ledger != nil {
		r.ledger.CloseFile(r.FileName, r.tally)
	}

	r.History = append(r.History, r.FileName)
	r.epoch++
	r.FileName = fileName(r.rng)
	r.gridSeed = r.rng.Int63()
	r.Grid = NewGridAt(r.gridSeed, r.epoch)
	r.dressGrid()
	r.tally = nil

	r.actives = nil
	clear(r.activeByID)
	r.creditIn = nil
	clear(r.justRefined)

	// A leftover deposit or detonation would replay onto the new board.
	r.deposits = nil
	r.blasts = nil
	clear(r.found)
	r.activateIn = 60
	r.holding = -1
}

// expireGrace forgets clusters whose co-refine window has closed.
func (r *Room) expireGrace(dt float64) {
	for id, g := range r.justRefined {
		g.left -= dt
		if g.left <= 0 {
			delete(r.justRefined, id)
		}
	}
}

// refineInto takes a cluster off the board into bin for desk, and returns its
// cells, the digits they held and the deposit every view replays. The caller
// holds the lock.
func (r *Room) refineInto(grp *BadGroup, bin int, desk *Desk) (ids []int, digits []rune, seq int) {
	ids = append([]int(nil), grp.NumberIDs...)
	digits = make([]rune, len(ids))

	// The replacement digits are decided here, not when a flight lands:
	// landing depends on terminal size and scroll, and the board must not.
	for i, id := range ids {
		digits[i] = rune('0' + r.Grid.Cells[id].Value)
		r.Grid.Cells[id].Value = digitFor(r.FileName, r.rng.Intn(10))
		r.Grid.Cells[id].BadGroupID = -1
	}

	// The bin is credited on the board's clock, not on arrival, so every view
	// credits it at the same moment.
	temper := TemperOf(r.gridSeed, r.Epoch(), grp.ID)
	r.creditIn = append(r.creditIn, pendingCredit{bin: bin, in: creditDelay, temper: temper})
	r.Claim(grp.ID, ids, desk, temper)
	seq = r.RecordDeposit(bin, ids, digits)

	r.Sleep(grp.ID)
	r.Retire(grp)
	return ids, digits, seq
}

// Claim records a refinement and credits the refiner. For coRefineWindow
// afterwards, CoRefine credits anyone else who refines it too, with the same
// temper; the board counts the cluster once.
func (r *Room) Claim(groupID int, members []int, desk *Desk, temper Temper) {
	if r.justRefined == nil {
		r.justRefined = map[int]*graceEntry{}
	}
	g := &graceEntry{members: members, left: coRefineWindow, credited: map[*Desk]bool{}, temper: temper}
	r.justRefined[groupID] = g
	if desk != nil {
		g.credited[desk] = true
		desk.Credit++
		r.credit(desk, temper)
	}
}

// credit records one refined cluster against a desk's name, on this file and on
// the server. Because of CoRefine, a file's attribution can sum to more than
// the clusters it held.
func (r *Room) credit(desk *Desk, temper Temper) {
	// A computer refiner's work counts on the file and in the footer
	// (Desk.Credit), never in the scores.
	if desk == nil || desk.Bot {
		return
	}
	name := refinerID(desk.Name)
	if name == "" {
		return
	}
	if r.tally == nil {
		r.tally = map[string]int{}
	}
	r.tally[name]++
	if r.ledger != nil {
		r.ledger.Credit(name, temper)
	}
}

// CoRefine credits a second refiner for a cluster that has only just gone, and
// reports whether it did.
func (r *Room) CoRefine(gridID int, desk *Desk) bool {
	if desk == nil {
		return false
	}
	for _, g := range r.justRefined {
		if g.credited[desk] {
			continue
		}
		for _, id := range g.members {
			if id != gridID {
				continue
			}
			g.credited[desk] = true
			desk.Credit++
			r.credit(desk, g.temper)
			return true
		}
	}
	return false
}

// advanceBreathe advances each awake cluster's swell and turns it around on
// arrival. The swell is owned by the board so every player sees the same
// rhythm; each player's Magnify follows it.
func (r *Room) advanceBreathe(dt float64) {
	for _, c := range r.actives {
		low, high := r.breatheBand(c.grp.ID)
		want := low
		if c.rising {
			want = high
		}
		c.breathe.Step(dt, want)

		// Turn when the spring arrives.
		if c.rising && c.breathe.Value() >= high-breatheArrival {
			c.rising = false
		} else if !c.rising && c.breathe.Value() <= low+breatheArrival {
			c.rising = true
		}
	}
}

// advanceCredits pays out bin credit that has come due.
func (r *Room) advanceCredits(dt float64) {
	kept := r.creditIn[:0]
	for _, c := range r.creditIn {
		c.in -= dt
		if c.in > 0 {
			kept = append(kept, c)
			continue
		}
		if c.bin >= 0 && c.bin < BinCount {
			r.Grid.Bins[c.bin].take(c.temper)
		}
	}
	r.creditIn = kept
}

// coRefineWindow is how long after a refinement, in seconds, somebody else
// refining the same cluster still counts. The board's lock serialises
// refiners, so "at the same time" has to be a grace period.
const coRefineWindow = 0.25

// graceEntry is a cluster that has just been refined, and who has been credited
// for it.
type graceEntry struct {
	members  []int
	left     float64
	credited map[*Desk]bool
	temper   Temper
}

// creditDelay is how long after a refinement the bin takes the credit, in
// seconds of simulated time: roughly how long the digits take to fly.
const creditDelay = 1.95

// pendingCredit is a bin waiting to be credited.
type pendingCredit struct {
	bin int
	in  float64

	// temper is which of the four the cluster was. Stored here because the
	// group is off the board by the time the credit lands.
	temper Temper
}

// MaybeActivate wakes a cluster somewhere at least one person can see, if fewer
// than hunts are awake.
func (r *Room) MaybeActivate(views []Viewport) {
	if len(r.actives) >= r.hunts {
		return
	}
	if r.activateIn > 0 {
		r.activateIn--
		return
	}
	r.activateIn = 60 + r.rng.Intn(180)
	if grp := r.randomVisibleGroup(views); grp != nil {
		r.Activate(grp)
	}
}

// Viewport is a rectangle of the grid somebody is looking at.
type Viewport struct{ X, Y, W, H int }

// randomVisibleGroup picks a sleeping cluster somebody can see.
func (r *Room) randomVisibleGroup(views []Viewport) *BadGroup {
	var candidates []*BadGroup
	seen := map[int]bool{}
	for _, v := range views {
		for gy := v.Y; gy < v.Y+v.H; gy++ {
			for gx := v.X; gx < v.X+v.W; gx++ {
				n := r.Grid.At(gx, gy)
				if n == nil || n.BadGroupID < 0 || seen[n.BadGroupID] || r.IsActive(n.BadGroupID) {
					continue
				}
				seen[n.BadGroupID] = true
				if grp := r.Grid.Group(n.BadGroupID); grp != nil {
					candidates = append(candidates, grp)
				}
			}
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	// Sort so the choice cannot depend on the order the windows arrived in.
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	return candidates[r.rng.Intn(len(candidates))]
}

// Retire removes a refined cluster from the board.
func (r *Room) Retire(grp *BadGroup) {
	delete(r.Grid.byID, grp.ID)
	for i, g := range r.Grid.Groups {
		if g.ID == grp.ID {
			r.Grid.Groups = append(r.Grid.Groups[:i], r.Grid.Groups[i+1:]...)
			break
		}
	}
}

// burnFuses detonates every cluster whose time has run out. The caller holds the
// lock, which Step does. It is on the board so every refiner loses the cluster
// at the same instant, once.
func (r *Room) burnFuses() {
	for _, grp := range r.detonated() {
		r.Detonate(grp, r.rng.Intn)
	}
}
