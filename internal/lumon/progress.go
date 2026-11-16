package lumon

import (
	"sort"
	"time"
)

// Progress: what a floor has to remember, independent of how it is written
// down. The server writes a JSON state file (save_terminal.go) and the browser
// its own format. Both carry a floorState, and every sanitiser
// for what comes back is here, so neither format restores anything itself.
//
// The board is a pure function of NewGridAt(seed, epoch), and refining only
// replaces a cell's digit, clears its group and credits a bin. So a file is
// (seed, epoch) plus which clusters have gone and what the bins hold. The
// replacement digits written at refinement are not restored; a restored
// refined cell shows whatever the board generates for it.

// floorState is a whole floor: its files, its standings and its record.
type floorState struct {
	saved time.Time
	rooms []fileState

	ledger []Standing

	// quarter is the quarter being counted, and pastQuarters and recent the
	// record: the quarters that have closed and the files finished, oldest
	// first. See quarter.go.
	quarter      string
	pastQuarters []QuarterRecord
	recent       []FileRecord
}

// fileState is one macrodata file, mid-refinement.
type fileState struct {
	file     string
	seed     int64
	gridSeed int64
	epoch    int
	history  []string
	refined  []int
	bins     [BinCount]int

	// tempers is how much of each of the four each bin holds. Unlike quotas,
	// which are reconciled on load, tempers cannot be derived because the
	// clusters are gone. An older save without them reads as zeroes.
	tempers [BinCount][TemperCount]int

	tally map[string]int
}

// ledgerCap bounds how many standings are kept from each board (lifetime and
// this quarter), best first, so a long-running server's state cannot grow
// without limit. Both tops are kept so a quarter leader is not forgotten on
// restart for not yet being among the lifetime's best.
const ledgerCap = 250

// capture copies the floor into a plain value with no pointer into any
// board. It takes read locks one room at a time; encoding and writing happen
// with nothing held, so the render loop never waits on the disk.
func (rs *Rooms) capture() floorState {
	if rs == nil {
		return floorState{}
	}

	rs.mu.Lock()
	rooms := make([]*Room, len(rs.rooms))
	copy(rooms, rs.rooms)
	ledger := rs.ledger
	rs.mu.Unlock()

	out := floorState{saved: rs.now()}
	for _, r := range rooms {
		out.rooms = append(out.rooms, r.capture())
	}
	if ledger != nil {
		ledger.captureInto(&out)
	}
	return out
}

// captureInto copies the standings and the record. It turns the quarter first,
// which is what turns it on an idle floor, since the server autosaves every
// thirty seconds. Rows go lifetime order first, then anyone only on the
// quarter's board, so an unchanged floor captures identically.
func (l *Ledger) captureInto(out *floorState) {
	l.turn()
	life, quarter := l.Standings(), l.QuarterStandings()

	l.mu.RLock()
	defer l.mu.RUnlock()
	out.quarter = l.quarter
	seen := map[string]bool{}
	take := func(rows []Standing) {
		if len(rows) > ledgerCap {
			rows = rows[:ledgerCap]
		}
		for _, row := range rows {
			if r, ok := l.by[row.Name]; ok && !seen[row.Name] {
				seen[row.Name] = true
				out.ledger = append(out.ledger, *r)
			}
		}
	}
	take(life)
	take(quarter)

	// And every name bound to a key, wherever it stands: dropping one on a
	// restart would give that refiner's name to whoever typed it next. Still
	// bounded, since the ledger holds at most LedgerLimit rows.
	var bound []Standing
	for _, r := range l.by {
		if r.Key != "" && !seen[r.Name] {
			bound = append(bound, *r)
		}
	}
	sort.Slice(bound, func(i, j int) bool { return bound[i].Name < bound[j].Name })
	out.ledger = append(out.ledger, bound...)
	out.pastQuarters = append([]QuarterRecord(nil), l.past...)
	out.recent = append([]FileRecord(nil), l.recent...)
}

// capture copies one room. It takes the board's read lock for the length of a
// few slice copies.
func (r *Room) capture() fileState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := fileState{
		file:     r.FileName,
		seed:     r.seed,
		gridSeed: r.gridSeed,
		epoch:    r.epoch,
		history:  append([]string(nil), r.History...),
		tally:    map[string]int{},
	}
	for k, v := range r.tally {
		out.tally[k] = v
	}

	// Which clusters have gone: those in a regenerated board but missing from
	// Groups, since Retire removes them.
	present := make(map[int]bool, len(r.Grid.Groups))
	for _, g := range r.Grid.Groups {
		present[g.ID] = true
	}
	// The cause (refined or detonated) is not recorded: reconcileQuotas derives
	// what the bins should hold from the clusters left and the credits in.
	for _, g := range NewGridAt(r.gridSeed, r.epoch).Groups {
		if !present[g.ID] {
			out.refined = append(out.refined, g.ID)
		}
	}

	for i := range r.Grid.Bins {
		out.bins[i] = r.Grid.Bins[i].Refined
		out.tempers[i] = r.Grid.Bins[i].Tempers
	}

	// Credits still on their timer count as landed, so no animation state is
	// persisted.
	for _, c := range r.creditIn {
		if c.bin >= 0 && c.bin < BinCount {
			out.bins[c.bin]++
			out.tempers[c.bin][c.temper]++
		}
	}
	return out
}

// restoreFloor rebuilds a registry from a captured floor.
func restoreFloor(fs floorState, seed int64) *Rooms {
	rs := NewRooms(seed)
	rs.ledger.restoreFrom(fs)
	if len(fs.rooms) > MaxRooms {
		fs.rooms = fs.rooms[:MaxRooms]
	}
	for _, f := range fs.rooms {
		if room := restoreRoom(f, rs.ledger, rs.now); room != nil {
			rs.rooms = append(rs.rooms, room)
		}
	}
	return rs
}

// restoreRoom rebuilds one macrodata file.
func restoreRoom(f fileState, l *Ledger, now func() time.Time) *Room {
	if f.file == "" {
		return nil
	}

	// File names go through tidyFile: they are drawn on every card in the
	// drawer, and a save is untrusted input.
	r := NewRoom(f.seed)
	r.FileName = tidyFile(f.file)
	if r.FileName == "" {
		return nil
	}
	r.epoch = f.epoch
	r.gridSeed = f.gridSeed
	r.Grid = NewGridAt(f.gridSeed, f.epoch)
	r.dressGrid()
	for _, h := range f.history {
		if h = tidyFile(h); h != "" {
			r.History = append(r.History, h)
		}
	}
	r.ledger = l

	// Tally keys go through refinerID, as on the live path. A key becomes
	// Standing.Name and is drawn to every terminal, and the canvas filters
	// nothing, so an escape sequence in a save would reach the wire.
	r.tally = map[string]int{}
	for k, v := range f.tally {
		if id := refinerID(k); id != "" {
			r.tally[id] += v
		}
	}

	// Put the refined clusters back. Retire takes the group off the board, and
	// clearing each member's BadGroupID stops the cells being drawn as
	// dormant-corrupt for ever, since nothing could clear them once the group
	// is gone.
	for _, id := range f.refined {
		grp := r.Grid.Group(id)
		if grp == nil {
			continue // a board that has moved on; skip it
		}
		for _, cell := range grp.NumberIDs {
			r.Grid.Cells[cell].BadGroupID = -1
		}
		r.Retire(grp)
	}

	// A bin cannot hold more credits than the file has clusters, or fewer
	// than none.
	most := len(NewGridAt(f.gridSeed, f.epoch).Groups)
	for i := range r.Grid.Bins {
		r.Grid.Bins[i].Refined = clampCount(f.bins[i], most)
		for t, n := range f.tempers[i] {
			r.Grid.Bins[i].Tempers[t] = clampCount(n, most)
		}
	}

	// Then the quotas, which come back from the seed expecting every cluster
	// the file ever had, including ones that blew up. Derive them from what is
	// left.
	r.reconcileQuotas()

	// Nobody is connected yet, so the room starts empty and its idle clock
	// starts now, not at save time, or a server down longer than idleReclaim
	// would sweep its whole floor when somebody opened the lobby.
	r.seated = 0
	r.setDesks(1)
	r.emptyAt = now()
	r.holding = -1
	r.activateIn = 30
	return r
}

// restoreFrom puts a captured floor's standings and record back into a fresh
// ledger, and turns the quarter if the clock has left the one it was counting.
// Every name goes through refinerID, every file name through tidyFile, and a
// quarter not matching periodShape is dropped: all are drawn on screens, and
// the canvas filters nothing.
func (l *Ledger) restoreFrom(fs floorState) {
	l.mu.Lock()
	for _, st := range fs.ledger {
		id := refinerID(st.Name)
		if id == "" {
			continue
		}
		r := l.record(id)
		r.Refined, r.Files, r.Topped, r.Seen = max(st.Refined, 0), max(st.Files, 0), max(st.Topped, 0), st.Seen
		for t, n := range st.Tempers {
			r.Tempers[t] = max(n, 0)
		}
		r.Key = tidyKey(st.Key)
		if periodShape.MatchString(st.Quarter.Of) {
			r.Quarter = st.Quarter
			r.Quarter.Refined = max(r.Quarter.Refined, 0)
			r.Quarter.Files = max(r.Quarter.Files, 0)
			r.Quarter.Topped = max(r.Quarter.Topped, 0)
		}
	}
	if periodShape.MatchString(fs.quarter) {
		l.quarter = fs.quarter
	}
	for _, q := range fs.pastQuarters {
		if !periodShape.MatchString(q.Quarter) {
			continue
		}
		rec := QuarterRecord{Quarter: q.Quarter}
		for _, st := range q.Top {
			if st.Name = refinerID(st.Name); st.Name != "" {
				rec.Top = append(rec.Top, st)
			}
		}
		if len(rec.Top) > 0 {
			l.past = append(l.past, rec)
		}
	}
	if n := len(l.past); n > pastQuarters {
		l.past = l.past[n-pastQuarters:]
	}
	for _, f := range fs.recent {
		rec := FileRecord{File: tidyFile(f.File), Finished: f.Finished, Top: refinerID(f.Top), Counts: map[string]int{}}
		if rec.File == "" {
			continue
		}
		for k, v := range f.Counts {
			if id := refinerID(k); id != "" {
				rec.Counts[id] += v
			}
		}
		l.recent = append(l.recent, rec)
	}
	if n := len(l.recent); n > recentFiles {
		l.recent = l.recent[n-recentFiles:]
	}
	l.mu.Unlock()

	l.turn()
}

// clampCount bounds a count read back from a save to [0, most].
func clampCount(n, most int) int {
	return min(max(n, 0), most)
}
