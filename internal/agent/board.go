package agent

import (
	"slices"
	"strconv"
	"time"
)

// Line is one agent on the game's board of agents.
type Line struct {
	Session string
	Name    string // the project, numbered when two sessions share one
	State   State
	Doing   string        // the step it is on, while working
	For     time.Duration // how long it has been on this prompt, to the minute
}

// How long the board keeps a session that has gone quiet: a finished one for
// a while after, and a working one that has sent nothing for hours, which most
// likely ended without saying so.
const (
	doneShown  = 10 * time.Minute
	quietShown = 2 * time.Hour
)

// Board is the agents to show at now, oldest prompt first.
func Board(now time.Time, read []Status) []Line {
	var keep []Status
	for _, s := range read {
		quiet := now.Sub(s.At)
		if s.State == Done && quiet > doneShown || quiet > quietShown {
			continue
		}
		keep = append(keep, s)
	}
	slices.SortFunc(keep, func(a, b Status) int {
		if c := a.started().Compare(b.started()); c != 0 {
			return c
		}
		if a.Session < b.Session {
			return -1
		}
		if a.Session > b.Session {
			return 1
		}
		return 0
	})

	lines := make([]Line, 0, len(keep))
	named := map[string]int{}
	for _, s := range keep {
		named[s.Project]++
		name := s.Project
		if n := named[s.Project]; n > 1 {
			name += " " + strconv.Itoa(n)
		}
		l := Line{Session: s.Session, Name: name, State: s.State}
		if s.State != Done {
			l.For = max(now.Sub(s.started()), 0).Truncate(time.Minute)
			if s.State == Working {
				l.Doing = s.Doing
			}
		}
		lines = append(lines, l)
	}
	return lines
}

// started is when the session's prompt was sent; a record from an older
// release has only the time it last changed.
func (s Status) started() time.Time {
	if !s.Since.IsZero() {
		return s.Since
	}
	return s.At
}

// Took is how long a finished turn took, or zero when the record cannot say.
func (s Status) Took() time.Duration {
	if s.Since.IsZero() || s.At.Before(s.Since) {
		return 0
	}
	return s.At.Sub(s.Since)
}
