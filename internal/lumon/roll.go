package lumon

import (
	"fmt"
	"sync"
)

// Who is on the floor, by name.
//
// The scoreboard knows a refiner only by name (see refinerID), so anybody may
// use any free name. The roll stops two terminals using one name at once,
// which would merge their nameplate, scoreboard row and tally.

// Badge is a name held on the floor by one terminal.
type Badge struct{ id string }

// roll is every name on the floor right now.
//
// Its lock is a leaf: Claim and Release touch only this map, so they are safe
// with or without a room lock held. Names are claimed on the name screen and
// not in Seat, because Seat runs under the room's lock and the registry lock
// is always taken before a room's.
type roll struct {
	mu    sync.Mutex
	names map[string]*Badge
}

// Claim takes a name for one terminal, or reports that somebody already has it.
// A nil registry is a terminal alone, which may use any name.
func (rs *Rooms) Claim(name string) (*Badge, bool) {
	b := &Badge{id: refinerID(name)}
	if rs == nil || b.id == "" {
		return b, true
	}
	rs.roll.mu.Lock()
	defer rs.roll.mu.Unlock()
	if rs.roll.names == nil {
		rs.roll.names = map[string]*Badge{}
	}
	if _, taken := rs.roll.names[b.id]; taken {
		return nil, false
	}
	rs.roll.names[b.id] = b
	return b, true
}

// Release gives a name back. Only the badge that holds it can, so a terminal
// releasing late can never free a name somebody else has since taken.
func (rs *Rooms) Release(b *Badge) {
	if rs == nil || b == nil {
		return
	}
	rs.roll.mu.Lock()
	defer rs.roll.mu.Unlock()
	if rs.roll.names[b.id] == b {
		delete(rs.roll.names, b.id)
	}
}

// nameTaken is what the name screen says when somebody has the name already.
// It says "on the floor" because the holder may be in the lobby, not at a desk.
const nameTaken = "THAT NAME IS ALREADY ON THE FLOOR"

// claimName takes the typed name for this terminal, giving back whatever it held
// before. It reports false, and says why, if the name is somebody else's.
//
// Ownership is checked with the name held. A name is bound only by the terminal
// holding it, so from here nobody else can bind it; checked before claiming, its
// holder could bind it and leave in between, and the name would go to the wrong
// refiner.
func (m *Model) claimName() bool {
	mine := m.badge != nil && m.badge.id == refinerID(m.Name)
	b, free := m.badge, mine
	if !mine {
		b, free = m.rooms.Claim(m.Name)
	}
	if m.ownedByAnother() {
		if !mine {
			m.rooms.Release(b)
		}
		m.nameNote, m.nameRefused = nameOwned, true
		return false
	}
	if !free {
		m.nameNote, m.nameRefused = nameTaken, true
		return false
	}
	if !mine {
		m.rooms.Release(m.badge)
		m.badge = b
	}
	return true
}

// noteName sets the note beneath the name field: a welcome for a name owned by
// this terminal's key, a refusal for one owned by another key, and the record
// for a name no key owns.
//
// It runs when the name changes, not on draw, because it reads the ledger,
// which every room on the floor shares.
func (m *Model) noteName() {
	m.nameNote, m.nameRefused = "", false
	if m.rooms == nil || m.Name == "" {
		return
	}
	s, ok := m.rooms.Ledger().Of(m.Name)
	switch {
	case !ok:
	case s.Key != "" && s.Key != m.key:
		m.nameNote, m.nameRefused = nameOwned, true
	case s.Refined == 0:
	case s.Key != "":
		m.nameNote = fmt.Sprintf("WELCOME BACK · %d THIS QUARTER · %d IN ALL", s.Quarter.Refined, s.Refined)
	default:
		m.nameNote = fmt.Sprintf("ON RECORD · %d THIS QUARTER · %d IN ALL", s.Quarter.Refined, s.Refined)
	}
}

// ownedByAnother reports whether the typed name belongs to a key that is not
// this terminal's.
func (m *Model) ownedByAnother() bool {
	if m.rooms == nil {
		return false
	}
	s, ok := m.rooms.Ledger().Of(m.Name)
	return ok && s.Key != "" && s.Key != m.key
}

// Close is a terminal going: it gives up its desk and its name.
func (m *Model) Close() {
	m.Leave()
	m.rooms.Release(m.badge)
	m.badge = nil
}

// nameOwned is what the name screen says when the name is somebody else's.
const nameOwned = "THAT NAME BELONGS TO ANOTHER REFINER"
