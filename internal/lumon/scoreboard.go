package lumon

import (
	"strconv"
	"strings"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// The scoreboard: three boards behind one key, turned with tab or the arrows.
// This quarter's standings, lifetime standings, and the record of Employees of
// the Quarter. It always opens on this quarter.

// scoreTop is the first row of the standings, below the heading and its rule.
// The renderer and the scroll arithmetic both need it.
const scoreTop = 5

// scoreTab is which board is up.
type scoreTab int

const (
	tabQuarter scoreTab = iota
	tabLifetime
	tabRecord
	scoreTabs
)

// scoreTabNames are the tabs as they are drawn, long and then short for a
// terminal too narrow for the long ones.
var scoreTabNames = [scoreTabs][2]string{
	{"THIS QUARTER", "QUARTER"},
	{"LIFETIME", "LIFETIME"},
	{"THE RECORD", "RECORD"},
}

// ledger is the standings this terminal can see. It checks the registry as
// well as the room, since a refiner in the lobby has no room.
func (m *Model) ledger() *Ledger {
	if m.Room != nil {
		if l := m.Room.Ledger(); l != nil {
			return l
		}
	}
	return m.rooms.Ledger()
}

// openScores puts the scoreboard up, on this quarter, remembering where to go
// back to.
func (m *Model) openScores(from ScreenState) {
	m.scoresFrom, m.Screen = from, ScreenScores
	m.scoresTab, m.scoresAt = tabQuarter, 0
}

// handleScoresKey takes a keystroke on the scoreboard. Anything that is not
// scrolling or turning closes it.
func (m *Model) handleScoresKey(key string) {
	switch key {
	case "up", "k":
		m.scrollScores(true)
	case "down", "j":
		m.scrollScores(false)
	case "tab", "right":
		m.turnScores(1)
	case "left":
		m.turnScores(-1)
	default:
		m.Screen = m.scoresFrom
	}
}

// turnScores goes to the next board, or the one before, round and round.
func (m *Model) turnScores(by int) {
	m.scoresTab = (m.scoresTab + scoreTab(by) + scoreTabs) % scoreTabs
	m.scoresAt = 0
}

// standingsFor is the board a tab shows. The record has none.
func (m *Model) standingsFor(tab scoreTab) []Standing {
	l := m.ledger()
	if l == nil {
		return nil
	}
	switch tab {
	case tabQuarter:
		return l.QuarterStandings()
	case tabLifetime:
		return l.Standings()
	}
	return nil
}

// scrollScores moves the marker one row up or down, for the arrow keys and the
// wheel alike.
func (m *Model) scrollScores(up bool) {
	if up {
		if m.scoresAt > 0 {
			m.scoresAt--
		}
		return
	}
	n := len(m.standingsFor(m.scoresTab))
	if m.scoresTab == tabRecord {
		n = len(m.recordLines())
	}
	if m.scoresAt < n-1 {
		m.scoresAt++
	}
}

// scoreWindow is which slice of the standings is on screen. Like lobbyWindow,
// clicks and the render both use it so they agree on which row is where. It
// stops four rows short of the bottom, leaving room for the standing line, a
// blank and the hint.
func (m *Model) scoreWindow(height, n int) (first, visible int) {
	visible = height - scoreTop - 4
	if visible > n {
		visible = n
	}
	if visible < 0 {
		visible = 0
	}
	if m.scoresAt >= visible && visible > 0 {
		first = m.scoresAt - visible + 1
	}
	return first, visible
}

// drawScores puts up whichever board is chosen.
func (m *Model) drawScores(buf *canvas.Buffer) {
	w, h := buf.Size()
	if w < 24 || h < 8 {
		return
	}

	const title = "REFINEMENT PERFORMANCE"
	buf.Text(2, 1, "LUMON INDUSTRIES", m.Theme.Header)
	if w >= 2+len("LUMON INDUSTRIES")+4+len(title)+2 {
		buf.Text(w-2-len(title), 1, title, m.Theme.Footer)
	}
	m.drawScoreTabs(buf, w)
	buf.HLine(0, 3, w, '═', m.Theme.Chrome)

	hint := pick(scoresLadder, w-4)
	defer func() { buf.Text(2, h-2, hint, m.Theme.Footer) }()

	if m.ledger() == nil {
		buf.Text(4, scoreTop, clipTo("no records", w-6), m.Theme.Footer)
		return
	}
	if m.scoresTab == tabRecord {
		m.drawRecord(buf, w, h)
		return
	}
	m.drawStandings(buf, w, h)
}

// scoresLadder is the scoreboard's hint, longest first.
var scoresLadder = ladder([]string{"tab boards", "↑↓ scroll", "any other key to go back"})

// drawScoreTabs is the row of three, with the one that is up picked out.
func (m *Model) drawScoreTabs(buf *canvas.Buffer, w int) {
	form := 0
	long := 0
	for _, n := range scoreTabNames {
		long += len(n[0]) + 3
	}
	if long > w-4 {
		form = 1
	}
	x := 2
	for i, n := range scoreTabNames {
		attr := m.Theme.Footer
		if scoreTab(i) == m.scoresTab {
			attr = m.Theme.Hovered
		}
		buf.Text(x, 2, clipTo(n[form], w-2-x), attr)
		x += len(n[form]) + 3
		if x >= w-2 {
			break
		}
	}
}

// scoreColumn is one column of numbers on a board.
type scoreColumn struct {
	head  string
	width int
	value func(Standing) string
}

// drawStandings is this quarter's board, or the lifetime's.
func (m *Model) drawStandings(buf *canvas.Buffer, w, h int) {
	all := m.standingsFor(m.scoresTab)
	if len(all) == 0 {
		empty := "nothing has been refined yet"
		if m.scoresTab == tabQuarter {
			empty = "nobody has refined this quarter yet"
		}
		buf.Text(4, scoreTop, clipTo(empty, w-6), m.Theme.Footer)
		return
	}

	// This file's attribution. Nil when the refiner is in the lobby.
	var here map[string]int
	if m.Room != nil {
		here = m.Room.Tally()
	}

	// Columns: refined, this file's share when there is a file, files worked
	// and files topped. A narrow terminal drops files first, then this file,
	// then topped.
	number := func(f func(Standing) int) func(Standing) string {
		return func(s Standing) string { return strconv.Itoa(f(s)) }
	}
	order := []scoreColumn{{"REFINED", 9, number(func(s Standing) int { return s.Refined })}}
	if here != nil {
		order = append(order, scoreColumn{"FILE", 6, func(s Standing) string {
			if c := here[s.Name]; c > 0 {
				return strconv.Itoa(c)
			}
			return "–"
		}})
	}
	order = append(order,
		scoreColumn{"FILES", 7, number(func(s Standing) int { return s.Files })},
		scoreColumn{"TOPPED", 8, number(func(s Standing) int { return s.Topped })},
	)

	// The rank sits in the columns to the left of the names, with a gap.
	const x = 6
	nameW := func() int {
		n := w - x - 2
		for _, c := range order {
			n -= c.width
		}
		return n
	}
	for _, drop := range []string{"FILES", "FILE", "TOPPED"} {
		if nameW() >= 10 {
			break
		}
		for i, c := range order {
			if c.head == drop {
				order = append(order[:i], order[i+1:]...)
				break
			}
		}
	}
	// No wider than a nameplate needs, so the numbers sit beside the names.
	nw := nameW()
	if nw > NameLimit+6 {
		nw = NameLimit + 6
	}
	if nw < 4 {
		nw = 4
	}

	row := func(y int, name string, values []string, attr canvas.Attr) {
		cx := x
		buf.Text(cx, y, clipTo(name, nw-1), attr)
		cx += nw
		for i, c := range order {
			buf.Text(cx, y, clipTo(values[i], c.width-1), attr)
			cx += c.width
		}
	}
	heads := make([]string, len(order))
	for i, c := range order {
		heads[i] = c.head
	}
	row(scoreTop-1, "REFINER", heads, m.Theme.BinOff)

	if m.scoresAt >= len(all) {
		m.scoresAt = len(all) - 1
	}
	first, visible := m.scoreWindow(h, len(all))

	// If your own row is below the window, it replaces the window's last row,
	// with its actual rank.
	mine := refinerID(m.Name)
	at := -1
	for i, st := range all {
		if st.Name == mine {
			at = i
			break
		}
	}
	pinned := at >= 0 && (at < first || at >= first+visible) && visible > 1
	if pinned {
		visible--
	}

	draw := func(i, y int) {
		st := all[i]
		// Your own row is bold in your desk's colour; the cursor row is
		// highlighted.
		attr := m.Theme.BinOff
		switch {
		case i == m.scoresAt:
			attr = m.Theme.Hovered
		case st.Name == mine:
			attr = m.Theme.On(m.deskColour(m.Desk), true)
		}
		buf.Text(x-4, y, clipTo(strconv.Itoa(i+1)+".", 3), m.Theme.Footer)
		values := make([]string, len(order))
		for j, c := range order {
			values[j] = c.value(st)
		}
		row(y, st.Name, values, attr)
	}
	for i := 0; i < visible; i++ {
		draw(first+i, scoreTop+i)
	}
	if pinned {
		draw(at, scoreTop+visible)
	}

	// The standing line: who leads the quarter, or who has refined the most.
	if best := all[0]; best.Refined > 0 {
		line := "MOST REFINED OF ALL: " + best.Name
		if m.scoresTab == tabQuarter {
			line = "LEADING " + m.ledger().Quarter() + ": " + best.Name
		}
		buf.Text(2, h-4, clipTo(line, w-4), m.Theme.BinOn)
	}
}

// recordLine is one line of the record.
type recordLine struct {
	text string
	attr canvas.Attr
}

// recordLines is the record, top to bottom: the quarters that have closed and
// who was named for them, then the files most recently finished.
func (m *Model) recordLines() []recordLine {
	l := m.ledger()
	if l == nil {
		return nil
	}
	var out []recordLine
	add := func(text string, attr canvas.Attr) { out = append(out, recordLine{text, attr}) }

	add("EMPLOYEES OF THE QUARTER", m.Theme.BinOff)
	past := l.Past()
	if len(past) == 0 {
		add("  no quarter has closed yet", m.Theme.Footer)
	}
	for _, q := range past {
		top := q.Top[0]
		add("  "+q.Quarter+"  "+top.Name+"  "+strconv.Itoa(top.Refined), m.Theme.BinOn)
		if len(q.Top) > 1 {
			var rest []string
			for _, s := range q.Top[1:] {
				rest = append(rest, s.Name+" "+strconv.Itoa(s.Refined))
			}
			add("           "+strings.Join(rest, " · "), m.Theme.Footer)
		}
	}

	add("", m.Theme.Footer)
	add("FINISHED FILES", m.Theme.BinOff)
	recent := l.Recent()
	if len(recent) == 0 {
		add("  no file has been finished yet", m.Theme.Footer)
	}
	for _, f := range recent {
		total := 0
		for _, n := range f.Counts {
			total += n
		}
		when := strings.ToUpper(f.Finished.UTC().Format("02 Jan 15:04"))
		line := "  " + f.File + "  " + when
		if f.Top != "" {
			line += "  TOP " + f.Top + "  " + strconv.Itoa(total)
		}
		add(line, m.Theme.BinOff)
	}
	return out
}

// drawRecord is the record, scrolled with the same marker the boards use.
func (m *Model) drawRecord(buf *canvas.Buffer, w, h int) {
	lines := m.recordLines()
	if m.scoresAt >= len(lines) {
		m.scoresAt = len(lines) - 1
	}
	first, visible := m.scoreWindow(h+2, len(lines))
	for i := 0; i < visible; i++ {
		ln := lines[first+i]
		buf.Text(2, scoreTop-1+i, clipTo(ln.text, w-4), ln.attr)
	}
}
