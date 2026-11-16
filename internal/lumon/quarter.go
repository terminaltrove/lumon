package lumon

import (
	"fmt"
	"sort"
	"time"
	"unicode"
)

// The quarter. A quarter is a calendar quarter in UTC, so every refiner on the
// floor agrees when it turns, and its Employee is whoever refined the most in
// it. Lifetime totals are kept beside it, so a newcomer has a race they can
// still win. One row per refiner has both, the quarter's numbers inside the
// lifetime's, so a refiner has one eviction.

// QuarterScore is a refiner's numbers for one quarter.
type QuarterScore struct {
	// Of is which quarter, as periodOf writes it.
	Of      string
	Refined int
	Files   int
	Topped  int
}

// QuarterRecord is a quarter that has closed: who led it, as they stood when
// it did.
type QuarterRecord struct {
	Quarter string

	// Top is the Employee of the Quarter and the two behind them, with the
	// quarter's numbers, not their lifetime's.
	Top []Standing
}

// FileRecord is a finished file.
type FileRecord struct {
	File     string
	Finished time.Time

	// Top is who did the most of it, and Counts what the leading few did.
	// Counts can sum to more than the file held: two refiners inside the
	// co-refine window are both credited for one cluster.
	Top    string
	Counts map[string]int
}

const (
	// pastQuarters is how many closed quarters the record keeps: ten years.
	pastQuarters = 40

	// recentFiles is how many finished files it keeps, about a day of them on
	// a busy floor.
	recentFiles = 50

	// quarterTop is how many of a quarter's leaders are kept when it closes,
	// and fileTop how many of a file's refiners.
	quarterTop = 3
	fileTop    = 8
)

// periodOf is the quarter a moment falls in, as "2026-Q4". It is the one place
// that defines a period; any replacement must still sort in time order, which
// turnLocked relies on.
func periodOf(t time.Time) string {
	u := t.UTC()
	return fmt.Sprintf("%04d-Q%d", u.Year(), (int(u.Month())-1)/3+1)
}

// periodShape is what a quarter read from a save must look like: four digits,
// "-Q", and 1 to 4. It is checked by hand because regexp would pull its whole
// engine into the browser build.
var periodShape periodPattern

type periodPattern struct{}

func (periodPattern) MatchString(s string) bool {
	if len(s) != 7 || s[4] != '-' || s[5] != 'Q' || s[6] < '1' || s[6] > '4' {
		return false
	}
	for i := 0; i < 4; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// turnLocked closes the quarter if the clock has left it. The caller holds the
// write lock. It runs on every credit, every snapshot (so an idle floor turns
// on the autosave) and on load (so a server down across the turn crowns the
// quarter it missed). Quarters compare as strings, and only forwards: a clock
// that goes backwards never reopens a closed quarter.
func (l *Ledger) turnLocked(now time.Time) {
	cur := periodOf(now)
	switch {
	case l.quarter == "":
		l.quarter = cur
		return
	case cur <= l.quarter:
		return
	}

	// A quarter nobody worked crowns nobody. The record drops keys.
	if top := l.quarterRowsLocked(); len(top) > 0 {
		if len(top) > quarterTop {
			top = top[:quarterTop]
		}
		for i := range top {
			top[i].Key = ""
		}
		l.past = append(l.past, QuarterRecord{Quarter: l.quarter, Top: top})
		l.news.Post("EMPLOYEE OF THE QUARTER · " + top[0].Name + " · " + l.quarter)
		if n := len(l.past); n > pastQuarters {
			l.past = append([]QuarterRecord(nil), l.past[n-pastQuarters:]...)
		}
	}
	l.quarter = cur
	l.sorted, l.sortedQuarter = nil, nil
}

// turn is turnLocked for a caller holding nothing.
func (l *Ledger) turn() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.turnLocked(l.clock())
}

// quarterRowsLocked is the quarter being counted, best first, each row carrying
// the quarter's numbers where its lifetime's would be. Anybody who has done
// nothing this quarter is left out. The caller holds the lock.
func (l *Ledger) quarterRowsLocked() []Standing {
	var out []Standing
	for _, r := range l.by {
		q := r.Quarter
		if q.Of != l.quarter || (q.Refined == 0 && q.Files == 0) {
			continue
		}
		row := *r
		row.Refined, row.Files, row.Topped = q.Refined, q.Files, q.Topped
		out = append(out, row)
	}
	sortStandings(out)
	return out
}

// QuarterStandings is this quarter's scoreboard, best first, cached until the
// next change as Standings is.
func (l *Ledger) QuarterStandings() []Standing {
	l.mu.RLock()
	if l.sortedQuarter != nil {
		out := l.sortedQuarter
		l.mu.RUnlock()
		return out
	}
	l.mu.RUnlock()

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.sortedQuarter == nil {
		l.sortedQuarter = l.quarterRowsLocked()
		if l.sortedQuarter == nil {
			l.sortedQuarter = []Standing{} // empty, and cached as empty
		}
	}
	return l.sortedQuarter
}

// Quarter is the quarter being counted.
func (l *Ledger) Quarter() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.quarter
}

// Past is the quarters that have closed, newest first.
func (l *Ledger) Past() []QuarterRecord {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]QuarterRecord, 0, len(l.past))
	for i := len(l.past) - 1; i >= 0; i-- {
		out = append(out, l.past[i])
	}
	return out
}

// Recent is the files that have been finished, newest first.
func (l *Ledger) Recent() []FileRecord {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]FileRecord, 0, len(l.recent))
	for i := len(l.recent) - 1; i >= 0; i-- {
		out = append(out, l.recent[i])
	}
	return out
}

// recordFile puts a finished file on the record, with its leading refiners.
// The caller holds the write lock.
func (l *Ledger) recordFile(file string, at time.Time, top string, tally map[string]int) {
	type count struct {
		name string
		n    int
	}
	var all []count
	for name, n := range tally {
		if id := refinerID(name); id != "" && n > 0 {
			all = append(all, count{id, n})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].n != all[j].n {
			return all[i].n > all[j].n
		}
		return all[i].name < all[j].name
	})
	if len(all) > fileTop {
		all = all[:fileTop]
	}
	counts := make(map[string]int, len(all))
	for _, c := range all {
		counts[c.name] += c.n
	}

	l.recent = append(l.recent, FileRecord{File: file, Finished: at, Top: top, Counts: counts})
	if n := len(l.recent); n > recentFiles {
		l.recent = append([]FileRecord(nil), l.recent[n-recentFiles:]...)
	}
}

// tidyFile makes a file name read from a save safe to draw. It differs from
// tidyName because file names are mixed case and longer than a nameplate. It
// drops control and non-printing characters, which the canvas would put
// straight on the wire, and cuts to 40 runes.
func tidyFile(s string) string {
	var out []rune
	for _, r := range s {
		if unicode.IsControl(r) || !unicode.IsPrint(r) {
			continue
		}
		if len(out) >= 40 {
			break
		}
		out = append(out, r)
	}
	return string(out)
}
