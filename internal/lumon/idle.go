package lumon

import (
	"fmt"
	"time"
)

// A terminal nobody is touching gives its place back.
//
// Idleness is tracked in the model because only a key or the mouse counts. The
// server's idle timeout counts bytes, and an unattended session still moves
// plenty: frames, and the link light's keepalive every two seconds.
//
// Time is the sum of raw frame durations, not sessionAge. The Clock caps
// catch-up, so sessionAge runs slow under load, and a busy server would keep
// its idlers longer when it most needs the room.

// idleWatch is how long this terminal has been left, and how long it may be.
// Zero limits mean never, which is what a local run has.
type idleWatch struct {
	// door is the limit before anybody has said who they are, on the boot
	// sequence and the name screen, and board the limit everywhere past it.
	door, board time.Duration

	untouched time.Duration
	atDoor    time.Duration
	farewell  string
}

// doorPatience is how many door limits a terminal may stay at the door, however
// busy its keys: at the server's two minutes, ten. Keys reset the idle clock, so
// without this a client could hold a place at the name screen for ever, without
// ever saying who it is.
const doorPatience = 5

// SendHomeAfter sets how long this terminal may be left before it is closed:
// door before the refiner has given a name, board after. The door limit is
// shorter because scanners that open a terminal never press anything. The
// screensaver counts as idle.
func (m *Model) SendHomeAfter(door, board time.Duration) {
	m.idle.door, m.idle.board = door, board
}

// Farewell is what to say to a terminal that was sent home, and empty for one
// that left of its own accord. It is printed after the alternate screen closes,
// so it stays in the client's scrollback.
func (m *Model) Farewell() string { return m.idle.farewell }

// touched is a person at the keyboard or the mouse.
func (m *Model) touched() { m.idle.untouched = 0 }

// leftFor counts a frame's wall time and reports whether this terminal has now
// been left for longer than it may be. It sets the farewell when it does.
func (m *Model) leftFor(elapsed time.Duration) bool {
	limit := m.idle.board
	atDoor := m.Screen == ScreenOff || m.Screen == ScreenIntro || m.Screen == ScreenName
	if atDoor {
		limit = m.idle.door
	}
	if limit <= 0 {
		return false
	}

	if atDoor {
		m.idle.atDoor += elapsed
	} else {
		m.idle.atDoor = 0
	}
	m.idle.untouched += elapsed
	if m.idle.untouched < limit && !(atDoor && m.idle.atDoor >= limit*doorPatience) {
		return false
	}
	if atDoor {
		m.idle.farewell = "NO REFINER WAS IDENTIFIED. THIS TERMINAL HAS BEEN RELEASED."
	} else {
		m.idle.farewell = "LUMON THANKS YOU FOR YOUR TIME. THIS TERMINAL WAS LEFT UNATTENDED FOR " +
			spokenDuration(limit) + "."
	}
	return true
}

// spokenDuration is a limit in the words a notice would use: whole minutes, or
// seconds for anything shorter.
func spokenDuration(d time.Duration) string {
	if d >= time.Minute {
		n := int(d / time.Minute)
		if n == 1 {
			return "1 MINUTE"
		}
		return fmt.Sprintf("%d MINUTES", n)
	}
	n := int((d + time.Second - 1) / time.Second)
	if n <= 1 {
		return "1 SECOND"
	}
	return fmt.Sprintf("%d SECONDS", n)
}
