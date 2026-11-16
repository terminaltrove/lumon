package lumon

import (
	"sort"
	"sync"
	"time"
)

// The Ledger is what each refiner has done in all and this quarter, across
// every file. It belongs to the registry, not to a room; Desk.Credit is only the
// current file. See quarter.go for the quarter.

// refinerID is what the scoreboard remembers somebody by: the typed name,
// normalised like the nameplate. Anyone who types the same name shares the row;
// the server has no authentication. Keying on client keys instead would change only
// this function and the Ledger's map.
func refinerID(name string) string { return tidyName(name) }

// Standing is one row of the scoreboard.
type Standing struct {
	Name string

	// Refined is clusters credited, ever.
	Refined int

	// Files is how many finished files they had a hand in, and Topped how many
	// of those they did the most of.
	Files  int
	Topped int

	Seen time.Time

	// Quarter is the same three numbers for the quarter they were last credited
	// in. A row from an earlier quarter counts as zero in this one.
	Quarter QuarterScore

	// Tempers is how much of each temper they have sorted, ever, in Temper
	// order. Not drawn yet, but recorded now because it cannot be reconstructed
	// once the clusters are gone.
	Tempers [TemperCount]int

	// Key is the fingerprint of the client key that first refined under this
	// name, which the name belongs to from then on. Empty for a name only ever
	// worked from a terminal with no key. Never drawn. See recognise_terminal.go.
	Key string
}

// Ledger is every refiner a server has seen.
type Ledger struct {
	mu sync.RWMutex
	by map[string]*Standing

	// sorted and sortedQuarter cache Standings and QuarterStandings until
	// record clears them. See Standings.
	sorted, sortedQuarter []Standing

	// quarter is the period being counted; past and recent are the closed
	// quarters and finished files, oldest first. See quarter.go.
	quarter string
	past    []QuarterRecord
	recent  []FileRecord

	// now is the clock, replaced in tests.
	now func() time.Time

	// news receives file and quarter honours on a shared floor; nil for a solo
	// ledger. Its lock never calls back into the ledger.
	news *floorNews
}

// NewLedger returns an empty ledger.
func NewLedger() *Ledger {
	return &Ledger{by: map[string]*Standing{}, now: time.Now}
}

// Refined credits somebody with n clusters, of no temper in particular.
func (l *Ledger) Refined(name string, n int) { l.credit(name, n, noTemper) }

// Credit is one refined cluster, of the given temper.
func (l *Ledger) Credit(name string, temper Temper) { l.credit(name, 1, temper) }

// noTemper marks a credit given as a bare count, with no temper.
const noTemper Temper = -1

func (l *Ledger) credit(name string, n int, temper Temper) {
	id := refinerID(name)
	if id == "" || n == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock()
	l.turnLocked(now)
	r := l.record(id)
	r.Refined += n
	l.thisQuarter(r).Refined += n
	if temper >= 0 && temper < TemperCount {
		r.Tempers[temper] += n
	}
	r.Seen = now
}

// CloseFile records a finished file: everyone who worked it gets a file,
// whoever did the most has topped it, and the file goes on the record. Cluster
// counts were credited as they happened.
func (l *Ledger) CloseFile(file string, tally map[string]int) (top string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock()
	l.turnLocked(now)

	top = refinerID(topOf(tally))
	for name := range tally {
		id := refinerID(name)
		if id == "" {
			continue
		}
		r := l.record(id)
		r.Files++
		l.thisQuarter(r).Files++
		r.Seen = now
	}
	if top != "" {
		r := l.record(top)
		r.Topped++
		l.thisQuarter(r).Topped++
	}
	l.recordFile(file, now, top, tally)
	line := "FILE " + file + " COMPLETE"
	if top != "" {
		line += " · TOP REFINER " + top
	}
	l.news.Post(line)
	return top
}

// thisQuarter is a row's numbers for the current quarter, reset if they are
// from an earlier one. The caller holds the write lock.
func (l *Ledger) thisQuarter(r *Standing) *QuarterScore {
	if r.Quarter.Of != l.quarter {
		r.Quarter = QuarterScore{Of: l.quarter}
	}
	return &r.Quarter
}

// Standings is the scoreboard, best first. It returns copies, so a caller never
// holds a pointer into the ledger.
//
// drawScores calls this from View every frame, so the sorted result is cached
// until record clears it. Sorting a large ledger per frame per session would
// starve the floor.
func (l *Ledger) Standings() []Standing {
	l.mu.RLock()
	if l.sorted != nil {
		out := l.sorted
		l.mu.RUnlock()
		return out
	}
	l.mu.RUnlock()

	l.mu.Lock()
	defer l.mu.Unlock()
	// Checked again under the write lock: several readers can find it stale at
	// once, and only the first needs to rebuild it.
	if l.sorted != nil {
		return l.sorted
	}

	out := make([]Standing, 0, len(l.by))
	for _, r := range l.by {
		out = append(out, *r)
	}
	sortStandings(out)
	l.sorted = out
	return out
}

// Of is one refiner's standing, with Quarter reset if it is from an earlier
// quarter.
func (l *Ledger) Of(name string) (Standing, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	r, ok := l.by[refinerID(name)]
	if !ok {
		return Standing{}, false
	}
	out := *r
	if out.Quarter.Of != l.quarter {
		out.Quarter = QuarterScore{Of: l.quarter}
	}
	return out, true
}

// LedgerLimit is how many refiners a server remembers at once. Names cost
// nothing to make, so without a bound anyone could grow the map forever.
// ledgerCap bounds only what is saved to disk; this is well above it, and well
// above a full floor (MaxDesks plus MaxWatchers per room, times MaxRooms, is 660).
const LedgerLimit = 4096

// record finds or opens somebody's row. The caller holds the write lock.
func (l *Ledger) record(id string) *Standing {
	if l.by == nil {
		l.by = map[string]*Standing{}
	}
	r, ok := l.by[id]
	if !ok {
		if len(l.by) >= LedgerLimit {
			l.forgetQuietest()
		}
		r = &Standing{Name: id, Seen: l.clock()}
		// Nothing could be forgotten: this refiner plays, unrecorded, and their
		// row goes when the caller drops it.
		if len(l.by) >= LedgerLimit {
			return r
		}
		l.by[id] = r
	}
	// Clear the sorted caches; the next reader rebuilds them, so a refinement
	// does not pay for a sort.
	l.sorted, l.sortedQuarter = nil, nil
	return r
}

// boundNameKept is how long a name bound to a key is safe from being forgotten
// after it was last seen. Forgetting one lets anybody take it and bind it to their
// own key, and keys cost nothing, so filling the ledger must not be a way to do
// that. Keeping them for ever would let a popular server fill up with its own
// past visitors, and shut new ones out of the scores for good.
const boundNameKept = 90 * 24 * time.Hour

// forgetQuietest drops the least recently seen tenth of what it may forget, and
// unbound names before bound ones. Evicting one row at a time would make every
// arrival past the limit walk and sort the map.
func (l *Ledger) forgetQuietest() {
	type aged struct {
		id    string
		seen  time.Time
		bound bool
	}
	keptSince := l.clock().Add(-boundNameKept)
	all := make([]aged, 0, len(l.by))
	for id, r := range l.by {
		bound := r.Key != ""
		if bound && r.Seen.After(keptSince) {
			continue
		}
		all = append(all, aged{id, r.Seen, bound})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].bound != all[j].bound {
			return !all[i].bound
		}
		return all[i].seen.Before(all[j].seen)
	})

	// Rounded up, so a few that may be forgotten make room.
	for i := 0; i < (len(all)+9)/10; i++ {
		delete(l.by, all[i].id)
	}
}

// clock is the ledger's time, which the tests replace.
func (l *Ledger) clock() time.Time {
	if l.now != nil {
		return l.now()
	}
	return time.Now()
}

// sortStandings orders a scoreboard: most refined first, then files topped,
// then by name, so equal refiners do not swap places between frames.
func sortStandings(s []Standing) {
	sort.Slice(s, func(i, j int) bool {
		a, b := s[i], s[j]
		if a.Refined != b.Refined {
			return a.Refined > b.Refined
		}
		if a.Topped != b.Topped {
			return a.Topped > b.Topped
		}
		return a.Name < b.Name
	})
}

// topOf is who did the most of a file, ties broken by name so the result does
// not depend on map iteration order.
func topOf(tally map[string]int) string {
	best, bestN := "", 0
	for name, n := range tally {
		if n > bestN || (n == bestN && name < best) {
			best, bestN = name, n
		}
	}
	return best
}
