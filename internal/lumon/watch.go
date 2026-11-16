package lumon

// Watching a board you have no desk at.
//
// When all desks are taken, an arrival can watch the whole board instead, and
// when a desk frees the person who has waited longest takes it. A watcher
// costs the server as much as a refiner (the same frame drawn and sent), which
// is why MaxWatchers exists.

// MaxWatchers is how many people may wait at one board. With MaxDesks it caps
// a room at twelve sessions, which bounds contention on the room's write lock
// that every session takes for its whole Update.
const MaxWatchers = 8

// Watcher is one person waiting for a desk. The pointer is the identity, so
// two people with the same name are still two places in the line.
type Watcher struct{ Name string }

// Wait puts somebody at the back of the queue, or returns nil if it is full.
func (r *Room) Wait(name string) *Watcher {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.watchers) >= MaxWatchers {
		return nil
	}
	w := &Watcher{Name: name}
	r.watchers = append(r.watchers, w)
	r.markEmpty()
	return w
}

// Unwait takes somebody out of the queue, wherever in it they are.
func (r *Room) Unwait(w *Watcher) {
	if w == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dropWatcher(w)
	r.markEmpty()
	r.botsLeaveIfAlone()
}

// dropWatcher removes one place in the line, keeping the order of the rest. The
// caller holds the lock.
func (r *Room) dropWatcher(w *Watcher) {
	for i, x := range r.watchers {
		if x == w {
			r.watchers = append(r.watchers[:i], r.watchers[i+1:]...)
			return
		}
	}
}

// Waiting is how many people are waiting for a desk. The caller holds the lock,
// which the render path does.
func (r *Room) Waiting() int { return len(r.watchers) }

// WaitPosition is how many people are ahead of somebody in the queue, or -1 if
// they are not in it. The caller holds the lock.
func (r *Room) WaitPosition(w *Watcher) int {
	for i, x := range r.watchers {
		if x == w {
			return i
		}
	}
	return -1
}

// PromoteIfNext seats somebody if they are at the head of the queue and a desk
// has come free. The caller holds the lock, which a session's own Update does.
//
// Each session asks for itself, on its own goroutine. The room must never
// reach into another session to seat it, or one player's input would write
// state another player's goroutine is drawing from.
func (r *Room) PromoteIfNext(w *Watcher, name string) *Desk {
	if w == nil || len(r.watchers) == 0 || r.watchers[0] != w {
		return nil
	}
	d := r.seatPersonLocked(name)
	if d == nil {
		return nil
	}
	r.dropWatcher(w)
	return d
}

// watchLine is the footer for somebody watching (their place in the queue),
// or empty for somebody at a desk. It is cached on the queue position, since
// the footer is drawn every frame and the position rarely changes.
func (m *Model) watchLine() string {
	if m.spectating {
		return "WATCHING · 1-4 follow · tab next · 0 any · w sit down · q leave"
	}
	if m.Room == nil || m.watch == nil || m.Desk != nil {
		return ""
	}
	n := m.Room.WaitPosition(m.watch)
	if m.watchAt == n && m.watchText != "" {
		return m.watchText
	}
	m.watchAt = n
	switch {
	case n < 0:
		m.watchText = "WATCHING · q leave"
	case n == 0:
		m.watchText = "WATCHING · NEXT FOR A DESK · q leave"
	case n == 1:
		m.watchText = "WATCHING · 1 AHEAD OF YOU · q leave"
	default:
		m.watchText = "WATCHING · " + itoa(n) + " AHEAD OF YOU · q leave"
	}
	return m.watchText
}

// Found remembers that somebody has had a cluster under their cursor, which
// puts it on the room's map for everyone. It is per file, so rotate clears it.
// The caller holds the lock, which publish does.
func (r *Room) Found(groupID int) {
	if groupID < 0 {
		return
	}
	if r.found == nil {
		r.found = map[int]bool{}
	}
	r.found[groupID] = true
}

// WasFound reports whether anybody has turned this cluster up yet. The caller
// holds the lock, which the render path does.
func (r *Room) WasFound(groupID int) bool { return groupID >= 0 && r.found[groupID] }
