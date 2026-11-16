package lumon

import "time"

// Recognising a refiner who comes back remotely.
//
// The client's key fingerprint is read as it connects, without asking. The
// first key to refine under a name owns it: that key is welcomed back with the
// name filled in, and any other terminal is told the name belongs to another
// refiner. A name is bound by refining, not by typing, or anybody could
// reserve every name by typing them and leaving. Names used without a key are
// first come, first served.
//
// This package only ever sees the fingerprint cmd/lumon passes in, and never
// draws it.

// Recognise tells a terminal which key its client connected with. It is called
// once, before the first frame; an empty key is a client that offered none. A
// key that has refined before gets the name it last used filled in.
func (m *Model) Recognise(key string) {
	m.key = key
	if key == "" {
		return
	}
	if name, ok := m.rooms.Ledger().NameFor(key); ok && m.Screen == ScreenName {
		m.Name = name
		m.noteName()
	}
}

// bindName gives the name this terminal is refining under to its key, if
// nobody's key has it yet. Called after every credit, under the board's lock;
// the ledger's lock is a leaf below it, the same order a credit takes them in.
func (m *Model) bindName() {
	if m.key == "" || m.Desk == nil {
		return
	}
	if l := m.ledger(); l != nil {
		l.Bind(m.Desk.Name, m.key)
	}
}

// namesPerKey is how many names one key may hold, so one client cannot
// reserve names by refining once under each.
const namesPerKey = 3

// Bind gives a name to a key, if the name is on record, no key has it, and
// the key holds fewer than namesPerKey. The first key keeps it; a later one
// refining under the name adds to its standing without taking it.
func (l *Ledger) Bind(name, key string) {
	id := refinerID(name)
	if id == "" || key == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	r, ok := l.by[id]
	if !ok || r.Key != "" {
		return
	}
	held := 0
	for _, other := range l.by {
		if other.Key == key {
			held++
		}
	}
	if held >= namesPerKey {
		return
	}
	r.Key = key
	l.sorted, l.sortedQuarter = nil, nil
}

// NameFor is the name a key last refined under.
func (l *Ledger) NameFor(key string) (string, bool) {
	if l == nil || key == "" {
		return "", false
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	best, at := "", time.Time{}
	for id, r := range l.by {
		if r.Key == key && (best == "" || r.Seen.After(at)) {
			best, at = id, r.Seen
		}
	}
	return best, best != ""
}

// tidyKey returns a fingerprint read from a save, or "" if it does not look
// like one. A save is untrusted input, so it is held to what this program
// writes.
func tidyKey(s string) string {
	if len(s) > 100 {
		return ""
	}
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9',
			r == '+', r == '/', r == ':', r == '=':
		default:
			return ""
		}
	}
	return s
}
