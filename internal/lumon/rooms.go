package lumon

import (
	"math/rand"
	"sort"
	"sync"
	"time"
)

// MaxDesks is how many refiners a room seats, as in the show's department.
const MaxDesks = 4

// idleReclaim is how long an emptied room keeps its progress before the
// registry drops it.
const idleReclaim = 30 * time.Minute

// DefaultRooms is how many files a server keeps open whether or not anybody is
// working them, so the lobby always offers a choice.
const DefaultRooms = 4

// Rooms is every board on this server. A room is a macrodata file: its name is
// the file's name and its progress bar is the shared completion. An emptied
// room can be rejoined until idleReclaim passes.
type Rooms struct {
	// ledger is the standings every room on this server shares, so a
	// refiner's tally follows them across files.
	ledger *Ledger

	// A read-write lock because most callers are Directory readers from the
	// lobby.
	mu sync.RWMutex

	// The directory, and when it was last built. See Directory.
	dir   []Entry
	dirAt time.Time
	rooms []*Room
	rng   *rand.Rand

	// floor is how many files this server keeps open regardless of who is on
	// it. It is stored so reclaim can top the floor back up after sweeping idle
	// rooms.
	floor int

	// now is the clock, replaced in tests.
	now func() time.Time

	// roll is who is on the floor right now, by name. See roll.go.
	roll roll

	// news is the one bulletin line every file can see.
	news floorNews

	// hold is how long each room holds a finished file, or 0 for the default.
	hold float64
}

// HoldFinishedFiles has every file on the floor, open now or later, held for
// seconds once finished before the next is taken: the browser holds them for
// the length of its music.
func (rs *Rooms) HoldFinishedFiles(seconds float64) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.hold = seconds
	for _, r := range rs.rooms {
		r.mu.Lock()
		r.hold = seconds
		r.mu.Unlock()
	}
}

// NewRooms returns an empty registry.
func NewRooms(seed int64) *Rooms {
	rs := &Rooms{
		rng:    rand.New(rand.NewSource(seed)),
		now:    time.Now,
		ledger: NewLedger(),
	}
	rs.ledger.news = &rs.news
	return rs
}

// Occupant is one refiner as the lobby sees them: a name and the seat that fixes
// their colour. It is a copy, never a *Desk: desks are written under the room's
// lock, which a lobby session does not hold, so sharing the pointer would race.
type Occupant struct {
	Name string
	Seat int
}

// Listing is what the lobby shows about a room.
type Listing struct {
	Name     string
	Percent  int
	Occupied int
	Desks    int

	// Refiners is who is sitting there, copied under the room's lock. A lobby
	// session holds no room lock, so it must use this and not Room.Roster.
	Refiners []Occupant
}

// Full reports whether the room has no desk to offer.
func (l Listing) Full() bool { return l.Occupied >= MaxDesks }

// List describes every room, busiest first, then alphabetically so the order is
// stable.
func (rs *Rooms) List() []Listing {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.reclaim()

	out := make([]Listing, 0, len(rs.rooms))
	for _, r := range rs.rooms {
		out = append(out, r.listing())
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Occupied != out[j].Occupied {
			return out[i].Occupied > out[j].Occupied
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// name is the file this room is working, without the cost of building a
// listing. It takes the lock because rotate replaces FileName when a file is
// finished.
func (r *Room) name() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.FileName
}

// listing describes one room, under the room's read lock. It is the only place
// the registry reads a room on behalf of a caller holding no room lock, so the
// roster is copied out here.
func (r *Room) listing() Listing {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var who []Occupant
	for _, d := range r.desk {
		if d != nil {
			who = append(who, Occupant{Name: d.Name, Seat: d.Seat})
		}
	}
	return Listing{
		Name:     r.FileName,
		Percent:  r.percent(),
		Occupied: r.seated,
		Desks:    MaxDesks,
		Refiners: who,
	}
}

// percent is the file's completion as a whole number. The caller holds the lock.
func (r *Room) percent() int {
	refined, total := 0, 0
	for i := range r.Grid.Bins {
		refined += r.Grid.Bins[i].Refined
		total += r.Grid.Bins[i].Quota
	}
	if total == 0 {
		return 0
	}
	if p := refined * 100 / total; p < 100 {
		return p
	}
	return 100
}

// MaxRooms is how many files a server will hold open at once: one for every
// name in FileNames, since names are how the lobby tells files apart. Open is
// one keypress from the lobby and costs about half a megabyte, so without a
// ceiling anyone could exhaust memory. 55 rooms of 12 (desks and watchers) is
// far more than a floor needs.
const MaxRooms = 55

// Open starts a new room on a file nobody here is already working. When the
// floor is at MaxRooms it reclaims idle rooms first, and if still full returns
// the emptiest room, so pressing "new file" always gives a file.
func (rs *Rooms) Open() *Room {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if len(rs.rooms) >= MaxRooms {
		rs.reclaim()
	}
	if len(rs.rooms) >= MaxRooms {
		if r := rs.emptiest(); r != nil {
			return r
		}
	}
	if r := rs.open(); r != nil {
		return r
	}
	return rs.emptiest()
}

// emptiest is the open room with the most space at it, or nil if there are none.
// The caller holds the lock.
func (rs *Rooms) emptiest() *Room {
	var best *Room
	bestSeated := MaxDesks + 1
	for _, r := range rs.rooms {
		r.mu.RLock()
		n := r.seated
		r.mu.RUnlock()
		if n < bestSeated {
			best, bestSeated = r, n
		}
	}
	return best
}

// EnsureOpen keeps at least n files on the floor: it opens what is missing now,
// and remembers n so reclaim tops the floor back up later.
func (rs *Rooms) EnsureOpen(n int) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if n > rs.floor {
		rs.floor = n
	}
	rs.ensure()
}

// Floor is how many files this server keeps open with nobody on it.
func (rs *Rooms) Floor() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.floor
}

// ensure assumes the registry lock.
func (rs *Rooms) ensure() {
	for len(rs.rooms) < rs.floor {
		if rs.open() == nil {
			return
		}
	}
}

// open starts a room on a file name nobody here has, or returns nil when
// every name is taken. It assumes the registry lock.
func (rs *Rooms) open() *Room {
	taken := make(map[string]bool, len(rs.rooms))
	for _, r := range rs.rooms {
		taken[r.name()] = true
	}

	// Try for an unused file name, since names are how the lobby tells rooms
	// apart. The name is drawn from the room's own generator before the board
	// is built, so a rejected name costs no board (a board is about half a
	// megabyte, and this runs under the registry lock). newRoom keeps the same
	// advanced generator, so the room matches what NewRoom(seed) would build.
	var room *Room
	for attempt := 0; attempt < 64; attempt++ {
		seed := rs.rng.Int63()
		rng := rand.New(rand.NewSource(seed))
		if name := fileName(rng); !taken[name] {
			room = newRoom(seed, rng, name)
			break
		}
	}
	if room == nil {
		// Most names are taken: choose among those left.
		var free []string
		for _, name := range FileNames() {
			if !taken[name] {
				free = append(free, name)
			}
		}
		if len(free) == 0 {
			return nil
		}
		room = NewRoomNamed(rs.rng.Int63(), free[rs.rng.Intn(len(free))])
	}

	// Every room on a server shares one ledger, replacing the room's own.
	if rs.ledger != nil {
		room.ledger = rs.ledger
	}
	room.hold = rs.hold

	rs.rooms = append(rs.rooms, room)
	rs.dropDirectory()
	return room
}

// OpenNamed starts a room on a named file, or returns the one already on it.
// The directory offers files that are not open yet, so taking a card must be
// able to ask for a name. A floor at MaxRooms returns the emptiest room, as Open
// does.
func (rs *Rooms) OpenNamed(name string) *Room {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.reclaim()

	for _, r := range rs.rooms {
		if r.listing().Name == name {
			return r
		}
	}
	if len(rs.rooms) >= MaxRooms {
		if r := rs.emptiest(); r != nil {
			return r
		}
	}

	room := NewRoomNamed(rs.rng.Int63(), name)
	if rs.ledger != nil {
		room.ledger = rs.ledger
	}
	room.hold = rs.hold
	rs.rooms = append(rs.rooms, room)
	rs.dropDirectory()
	return room
}

// Entry is one file in the directory: a name, and what is going on at it.
type Entry struct {
	Listing

	// Open is whether anybody has opened this file yet. A closed file is a
	// name only, with no percentage or refiners.
	Open bool
}

// directoryTTL is how long a built directory is served before it is rebuilt.
// Every lobby session asks for it twice a frame, and each build locks and
// copies every open room, so during an arrival rush building per call
// dominates the floor's cost (see the BenchmarkLobby benchmarks in scale_test.go).
const directoryTTL = 100 * time.Millisecond

// Directory is every file that can be chosen, alphabetically: every name in
// FileNames, plus any open room on a name not among them (a save may hold an
// older one), since a room missing from the directory could not be joined.
//
// The returned slice is shared by every caller (Rolodex.Sync keeps it), so
// nobody may write into it.
func (rs *Rooms) Directory() []Entry {
	// The common case, under the read lock.
	rs.mu.RLock()
	if fresh := rs.dir != nil && rs.now().Sub(rs.dirAt) < directoryTTL; fresh {
		dir := rs.dir
		rs.mu.RUnlock()
		return dir
	}
	rs.mu.RUnlock()

	rs.mu.Lock()
	defer rs.mu.Unlock()
	// Checked again under the write lock: several readers can find it stale at
	// once, and only the first needs to rebuild it.
	if rs.dir != nil && rs.now().Sub(rs.dirAt) < directoryTTL {
		return rs.dir
	}
	rs.reclaim()

	live := make(map[string]Listing, len(rs.rooms))
	for _, r := range rs.rooms {
		l := r.listing()
		live[l.Name] = l
	}

	names := FileNames()
	seen := make(map[string]bool, len(names)+len(live))
	out := make([]Entry, 0, len(names)+len(live))
	add := func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		if l, ok := live[name]; ok {
			out = append(out, Entry{Listing: l, Open: true})
			return
		}
		out = append(out, Entry{Listing: Listing{Name: name, Desks: MaxDesks}})
	}
	for _, name := range names {
		add(name)
	}
	for name := range live {
		add(name)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	// Shared with every caller, not copied; see the doc comment.
	rs.dir, rs.dirAt = out, rs.now()
	return out
}

// dropDirectory forgets the cached listing so the next reader builds a fresh
// one. The caller holds the write lock.
//
// Only registry changes (a file opening or being reclaimed) call this. Seating
// goes through Room.Seat and never touches the registry, so a new nameplate
// reaches the directory within directoryTTL.
func (rs *Rooms) dropDirectory() { rs.dir = nil }

// Ledger is the standings every room on this server shares.
func (rs *Rooms) Ledger() *Ledger {
	if rs == nil {
		return nil
	}
	return rs.ledger
}

// Find returns the room working a named file, or nil.
func (rs *Rooms) Find(name string) *Room {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for _, r := range rs.rooms {
		if r.listing().Name == name {
			return r
		}
	}
	return nil
}

// Assign returns a room for a new refiner: the busiest one with a free desk, so
// people meet, or a fresh room when they are all full.
func (rs *Rooms) Assign() *Room {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.reclaim()

	var best *Room
	bestSeated := -1
	for _, r := range rs.rooms {
		l := r.listing()
		if l.Full() {
			continue
		}
		if l.Occupied > bestSeated {
			best, bestSeated = r, l.Occupied
		}
	}
	if best != nil {
		return best
	}
	if r := rs.open(); r != nil {
		return r
	}
	return rs.emptiest()
}

// Count reports how many rooms are open.
func (rs *Rooms) Count() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return len(rs.rooms)
}

// Reclaim drops rooms that have been empty too long. The registry also does
// this as it goes; a server calls it on a timer so an idle machine tidies up.
func (rs *Rooms) Reclaim() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.reclaim()
}

// reclaim assumes the registry lock.
func (rs *Rooms) reclaim() {
	now := rs.now()
	before := len(rs.rooms)
	kept := rs.rooms[:0]
	for _, r := range rs.rooms {
		if r.emptySince(now) < idleReclaim {
			kept = append(kept, r)
		}
	}
	if len(kept) != before {
		rs.dropDirectory()
	}
	rs.rooms = kept

	// Then top back up to the floor, so the lobby is never empty.
	rs.ensure()
}

// emptySince is how long the room has had nobody in it, or zero if somebody is
// still there.
func (r *Room) emptySince(now time.Time) time.Duration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.seated > 0 || r.emptyAt.IsZero() {
		return 0
	}
	return now.Sub(r.emptyAt)
}
