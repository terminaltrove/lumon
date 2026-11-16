package lumon

import (
	"strconv"
	"time"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// The agents strip is set into the file bar's lower edge, one entry per coding
// agent the refiner is running:
//
//	└─ lumon ● Editing view.go 3m ─ api ◐ waiting 1m ─ web ✓ done ─┘
//
// Where the edge is too short, every entry drops its step, then its time,
// then its name, and last the agents that still do not fit become "+N".

const (
	stripWorking = '●'
	stripWaiting = '◐'
	stripDone    = '✓'
)

// stripDetail is how much of each entry is drawn, most first.
type stripDetail int

const (
	stripFull stripDetail = iota
	stripTimed
	stripNamed
	stripMarks
)

// stripRole is what a run of the strip is, so its colour is taken from the
// palette drawn with, which changes with the theme and while a board arrives.
type stripRole int

const (
	stripEdge stripRole = iota
	stripText
	stripUrgent
	stripQuiet
)

// stripPart is one run of text in the strip.
type stripPart struct {
	x    int
	text string
	role stripRole
}

// stripCache keeps the laid-out strip between frames; it is redone when the
// agents or the width change.
type stripCache struct {
	width  int
	agents []AgentLine
	parts  []stripPart
}

func (m *Model) drawAgentStrip(buf *canvas.Buffer) {
	if len(m.agents) == 0 || m.Layout.HeaderH < 3 {
		return
	}
	c := &m.strip
	if c.width != m.Layout.Width || !sameAgents(c.agents, m.agents) {
		c.width, c.agents = m.Layout.Width, m.agents
		c.parts = m.layAgentStrip(m.Layout.Width)
	}
	y := m.Layout.HeaderH - 1
	for _, p := range c.parts {
		buf.Text(p.x, y, p.text, m.stripAttr(p.role))
	}
}

func (m *Model) stripAttr(r stripRole) canvas.Attr {
	switch r {
	case stripText:
		return m.Theme.Header
	case stripUrgent:
		return m.Theme.On(m.Theme.Urgent, true)
	case stripQuiet:
		return m.Theme.Hint
	}
	return m.Theme.Chrome
}

func sameAgents(a, b []AgentLine) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// layAgentStrip places the entries between the corner's "└─" and "─┘".
func (m *Model) layAgentStrip(width int) []stripPart {
	from, to := 2, width-2 // to is the first column left alone
	if to-from < 4 {
		return nil
	}
	for detail := stripFull; detail <= stripMarks; detail++ {
		if parts, ok := m.stripAt(detail, from, to, len(m.agents)); ok {
			return parts
		}
	}
	// Marks only, as many as fit, and how many are left out.
	for shown := len(m.agents) - 1; shown >= 0; shown-- {
		if parts, ok := m.stripAt(stripMarks, from, to, shown); ok {
			return parts
		}
	}
	return nil
}

// stripAt lays out the first shown agents at detail, with "+N" for the rest,
// and reports whether it fits between from and to.
func (m *Model) stripAt(detail stripDetail, from, to, shown int) ([]stripPart, bool) {
	var parts []stripPart
	x := from
	put := func(s string, r stripRole) {
		parts = append(parts, stripPart{x, s, r})
		x += len([]rune(s))
	}
	for i, a := range m.agents[:shown] {
		if i > 0 {
			put("─", stripEdge)
		}
		put(" ", stripEdge)
		if detail < stripMarks {
			put(a.Name+" ", stripText)
		}
		mark, role := stripMark(a.State)
		put(string(mark), role)
		if detail <= stripTimed {
			if s := stripWords(a, detail); s != "" {
				put(" "+s, stripText)
			}
		}
		put(" ", stripEdge)
	}
	if rest := len(m.agents) - shown; rest > 0 {
		if shown > 0 {
			put("─", stripEdge)
		}
		put(" +"+strconv.Itoa(rest)+" ", stripText)
	}
	return parts, x <= to
}

// stripWords is what follows an entry's mark: its step and time when working,
// "waiting" and its time when waiting on the refiner, "done" when finished.
func stripWords(a AgentLine, detail stripDetail) string {
	var words string
	switch a.State {
	case AgentDone:
		return "done"
	case AgentWaiting:
		words = "waiting"
	default:
		if detail == stripFull {
			words = a.Doing
		}
	}
	if a.For >= time.Minute {
		if words != "" {
			words += " "
		}
		words += spanText(a.For)
	}
	return words
}

func stripMark(s AgentState) (rune, stripRole) {
	switch s {
	case AgentWaiting:
		return stripWaiting, stripUrgent
	case AgentDone:
		return stripDone, stripQuiet
	}
	return stripWorking, stripText
}
