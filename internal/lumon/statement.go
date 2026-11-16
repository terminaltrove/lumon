package lumon

import (
	"math"
	"strings"

	"github.com/terminaltrove/lumon/internal/canvas"
	"github.com/terminaltrove/lumon/internal/physics"
	"github.com/terminaltrove/lumon/internal/theme"
)

// COMPUNCTION: the Compunction Statement, read off the board. The passage is
// wrapped to the panel and centred, in ordinary characters, and writes itself
// out clause by clause.
//
// The board's digits fly in to a rectangular coil wound into the panel, then
// each character rises off the coil to its place in the lines. Digits returns
// a place for every character with one of the board's digits; Transit uses a
// Placed's digit only on the way home, so the statement assembles out of the
// grid and scatters back into it. The coil is only the way in: a passage set
// along a curve cannot be read.
//
// where reports each character's present position for both drawing and
// flight, so the two never disagree: at construction it gives the coil, at
// the end the settled lines.
//
// It draws actual characters, not glyph.LetterStrokes, which has no J, Q or X
// and no punctuation but the full stop and comma. See fit for why it is not
// set large.

// theStatement is what a refiner is made to say: Kier Eagan's, as on the card
// Ms Cobel holds, in capitals like all of Lumon's printed matter. The last
// clause is "AND THAT IS ALL THAT I AM"; transcriptions drop a word.
var theStatement = []string{
	"FORGIVE ME FOR THE HARM I HAVE CAUSED THIS WORLD.",
	"NONE MAY ATONE FOR MY ACTIONS BUT ME,",
	"AND ONLY IN ME SHALL THEIR STAIN LIVE ON.",
	"I AM THANKFUL TO HAVE BEEN CAUGHT,",
	"MY FALL CUT SHORT BY THOSE WITH WIZENED HANDS.",
	"ALL I CAN BE IS SORRY,",
	"AND THAT IS ALL THAT I AM.",
}

// statementMargin is how much of the panel's width is left either side, as a
// fraction. Prose set full width on a wide terminal is hard to read.
const statementMargin = 0.1

const (
	// statementTurns is how many times round the coil goes, and
	// statementDepth how far in it winds as a fraction of the panel. Stopping
	// well short of the middle keeps the innermost run several characters wide.
	statementTurns = 2.5
	statementDepth = 0.62

	// charDelay is how long each character waits behind the one before it, and
	// lineDelay how long each line waits behind the line above, in seconds.
	// Together they are the recital.
	charDelay = 0.035
	lineDelay = 0.40

	// statementRise is how long a character takes to travel up off the coil,
	// and statementFall how long to go back down, in seconds.
	statementRise = 0.5
	statementFall = 0.35

	// statementRead is how long a window is held after its last character has
	// arrived, in seconds. It sets the recital's pace.
	statementRead = 3.0
)

// statementCell is one character of the statement: what it is, where it sits on
// the coil, and where in the passage it belongs.
type statementCell struct {
	// line is which wrapped line it is on and col how far along it.
	line, col int

	sx, sy float64 // its place on the coil
	ch     rune
}

// coil lays n points along a rectangular spiral wound into the panel. A
// rectangle matches the panel and needs no aspect correction, and its long
// runs are horizontal, so clauses lie flat as they unwind. Points are spaced
// evenly by arc length in visual columns; uniform angle would bunch them at
// the corners.
func coil(panel Panel, n int) []physics.Vec2 {
	if n < 1 || panel.W < 2 || panel.H < 2 {
		return nil
	}
	hx, hy := float64(panel.W-1)/2, float64(panel.H-1)/2
	cx, cy := float64(panel.X)+hx, float64(panel.Y)+hy

	// Walk it finely first, keeping how far along each sample is.
	const steps = 2048
	pts := make([]physics.Vec2, steps+1)
	run := make([]float64, steps+1)
	for i := 0; i <= steps; i++ {
		f := float64(i) / steps
		a := f * statementTurns * 2 * math.Pi
		c, s := math.Cos(a), math.Sin(a)

		// Where that angle meets the rectangle: the nearer of the two edges it
		// could cross.
		t := math.Inf(1)
		if c != 0 {
			t = math.Min(t, hx/math.Abs(c))
		}
		if s != 0 {
			t = math.Min(t, hy/math.Abs(s))
		}
		k := 1 - statementDepth*f
		pts[i] = physics.Vec2{X: cx + t*c*k, Y: cy + t*s*k}
		if i > 0 {
			d := pts[i].Sub(pts[i-1])
			run[i] = run[i-1] + math.Hypot(d.X, d.Y*RowAspect)
		}
	}

	// Then pick n of them at equal distances along it.
	out := make([]physics.Vec2, n)
	total := run[steps]
	at := 0
	for i := 0; i < n; i++ {
		want := total * (float64(i) + 0.5) / float64(n)
		for at < steps && run[at+1] < want {
			at++
		}
		out[i] = pts[at]
	}
	return out
}

// Statement is the Compunction Statement, recited.
type Statement struct {
	// cells are the characters and their places, rebuilt whenever the panel
	// changes, and up how far each has risen off the coil, 0 to 1.
	cells []statementCell
	up    []float64

	// lines is the passage wrapped to the panel, rows how many of them are shown
	// (all, unless the panel is too short) and lead the blank rows between
	// them. See fit.
	lines []string
	rows  int
	lead  int

	// at is the first line of the window that is up, t how long it has been,
	// in seconds, and round how many times the recital has been through.
	at    int
	t     float64
	round int

	// digits are the board's own numbers, handed out to the places in turn.
	digits []rune
	next   int

	// laid is the panel cells was built for, so only a resize rebuilds.
	laid Panel

	Fade
}

// NewStatement collects the board's digits and prepares the passage.
func NewStatement(m *Model) *Statement {
	s := &Statement{Fade: NewFade()}

	l := m.Layout
	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			if n := m.Grid.At(gx, gy); n != nil {
				s.digits = append(s.digits, rune('0'+n.Value))
			}
		}
	}
	if len(s.digits) == 0 {
		s.digits = []rune("0")
	}
	return s
}

// digitAt is the next board digit, round-robin.
func (s *Statement) digitAt() rune {
	d := s.digits[s.next%len(s.digits)]
	s.next++
	return d
}

// wrapStatement folds the passage to a width. The authored line breaks are the
// clauses and are kept; a clause too wide for the panel is wrapped again on
// words, never mid-word.
func wrapStatement(text []string, width int) []string {
	if width < 1 {
		return nil
	}
	var out []string
	for _, line := range text {
		if len([]rune(line)) <= width {
			out = append(out, line)
			continue
		}
		cur := ""
		for _, word := range strings.Fields(line) {
			switch {
			case cur == "":
				cur = word
			case len([]rune(cur))+1+len([]rune(word)) <= width:
				cur += " " + word
			default:
				out = append(out, cur)
				cur = word
			}
		}
		if cur != "" {
			out = append(out, cur)
		}
	}
	return out
}

// fit is how the passage is laid out: how wide the lines are and how much air
// goes between them.
//
// The whole passage goes on one page, which rules out a large setting. It is
// 262 characters: in four-row letters five columns wide, a 120 column panel
// fits 19 to the line, 17 lines, 68 rows against a panel of 21. Even two-row
// letters need 24. Ordinary characters fit at every size, seven lines in a 21
// row panel or a 9 row one.
func fit(panel Panel) (lead, cols int) {
	cols = panel.W - 2*int(float64(panel.W)*statementMargin)
	if cols < 1 || panel.H < 1 {
		return 0, 0
	}
	// A blank row between clauses when the panel has the rows for it. It is
	// the first thing to give way, since a clause off the bottom is worse than
	// a tight one.
	if n := len(wrapStatement(theStatement, cols)); n*2-1 <= panel.H {
		lead = 1
	}
	return lead, cols
}

// layout places every character of the passage on the coil and in its line.
// Draw and Digits share it, as with Praise and Matrix3D.
func (s *Statement) layout(panel Panel) {
	if panel == s.laid && s.cells != nil {
		return
	}
	s.laid, s.cells, s.up = panel, nil, nil

	var width int
	s.lead, width = fit(panel)
	s.lines = wrapStatement(theStatement, width)
	if len(s.lines) == 0 {
		return
	}

	// The window is normally all of the passage. On a short panel the air
	// gives way first, then lines are clipped.
	s.rows = len(s.lines)
	for s.rows > 1 && s.rows*(1+s.lead)-s.lead > panel.H {
		if s.lead > 0 {
			s.lead = 0
			continue
		}
		s.rows--
	}

	for li, line := range s.lines {
		for j, ch := range []rune(line) {
			if ch == ' ' {
				continue // nothing to fly to and nothing to draw
			}
			s.cells = append(s.cells, statementCell{line: li, col: j, ch: ch})
		}
	}
	s.up = make([]float64, len(s.cells))

	// The coil positions, in reading order, so the passage winds inward from
	// the outside.
	if c := coil(panel, len(s.cells)); c != nil {
		for i := range s.cells {
			s.cells[i].sx, s.cells[i].sy = c[i].X, c[i].Y
		}
	}
	if s.at+s.rows > len(s.lines) {
		s.at = 0
	}
}

// slot is where a character sits while it is being said, in cells.
func (s *Statement) slot(c statementCell, panel Panel) (float64, float64) {
	// A character is a column and a line is a row; only the air between lines
	// varies. See fit.
	lead := s.lead

	wide := len([]rune(s.lines[c.line]))
	x := panel.X + (panel.W-wide)/2 + c.col

	// Clamp into the window, for characters on the way out: a line already
	// said has a slot above the panel, and would streak away off screen.
	row := c.line - s.at
	if row < 0 {
		row = 0
	} else if row >= s.rows {
		row = s.rows - 1
	}

	tall := s.rows*(1+lead) - lead
	y := panel.Y + (panel.H-tall)/2 + row*(1+lead)
	return float64(x), float64(y)
}

// where is a character's present place, between the coil and its line. Drawing
// and Digits both use it, so the flight starts from what is on screen.
func (s *Statement) where(i int, panel Panel) (float64, float64) {
	c := s.cells[i]
	u := s.up[i]
	if u <= 0 {
		return c.sx, c.sy
	}
	x, y := s.slot(c, panel)
	// Snap home at u >= 1. A character a hundredth of the way out can round
	// into the next cell and break a word; see hysteresis.go for the same rule.
	if u >= 1 {
		return x, y
	}
	return c.sx + (x-c.sx)*u, c.sy + (y-c.sy)*u
}

// Wound reports whether the recital has been through the passage once.
func (s *Statement) Wound() bool { return s.round > 0 }

// Resize rebuilds the layout for a new panel.
func (s *Statement) Resize(panel Panel) { s.layout(panel) }

// Step advances the fade and the recital. Each character waits its turn and
// then travels for a fixed time, a clock and a ramp, so delays are expressible
// and the arrival is exact; a spring has neither.
func (s *Statement) Step(dt float64) {
	s.StepFade(dt)
	s.layout(s.laid)
	if len(s.cells) == 0 || len(s.lines) == 0 {
		return
	}

	s.t += dt
	if hold := s.hold(); s.t >= hold {
		if s.rows >= len(s.lines) {
			// The whole passage is up, so it stays up. Pin the clock so due
			// stays satisfied without the number growing all session.
			s.t, s.round = hold, 1
		} else {
			s.t = 0
			s.at += s.rows
			if s.at >= len(s.lines) {
				s.at, s.round = 0, s.round+1
			}
		}
	}

	for i := range s.cells {
		c := s.cells[i]
		want := 0.0
		if s.saying(c) && s.t >= s.due(c) {
			want = 1
		}
		// Three branches, not two: with else doing the falling, an arrived
		// character (want == up == 1) would fall, rise and flutter for ever.
		switch {
		case want > s.up[i]:
			s.up[i] = math.Min(want, s.up[i]+dt/statementRise)
		case want < s.up[i]:
			s.up[i] = math.Max(want, s.up[i]-dt/statementFall)
		}
	}
}

// saying reports whether a character belongs to the window that is up.
func (s *Statement) saying(c statementCell) bool {
	return c.line >= s.at && c.line < s.at+s.rows
}

// due is when a character's turn comes, in seconds into the window.
func (s *Statement) due(c statementCell) float64 {
	return float64(c.line-s.at)*lineDelay + float64(c.col)*charDelay
}

// hold is how long the window that is up stays up: long enough for its last
// character to have arrived, and then long enough to read.
func (s *Statement) hold() float64 {
	last := 0.0
	for _, c := range s.cells {
		if c.line >= s.at && c.line < s.at+s.rows {
			last = math.Max(last, s.due(c))
		}
	}
	return last + statementRise + statementRead
}

// Digits is where every character is now, one board digit apiece. The digit is
// used only on the way home, but is filled both ways since an empty one would
// send back a zero rune.
func (s *Statement) Digits(panel Panel) []Placed {
	s.layout(panel)
	out := make([]Placed, 0, len(s.cells))
	for i := range s.cells {
		x, y := s.where(i, panel)
		out = append(out, Placed{X: x, Y: y, Digit: s.digitAt()})
	}
	return out
}

// Draw paints each character that has risen off the coil, at its present
// place, fading with the diversion.
func (s *Statement) Draw(buf *canvas.Buffer, t *theme.Theme, panel Panel) {
	s.layout(panel)
	level := s.Level()
	if level < 0.02 || len(s.cells) == 0 {
		return
	}

	// Only characters being said are drawn; the coil is the arrival, and
	// letters left on it would be noise around the passage.
	lit := canvas.Attr{FG: theme.BlendColors(t.Backdrop, t.Glow, level)}

	// One pass: every character is one cell at either end of its journey.
	for i := range s.cells {
		if s.up[i] <= 0 {
			continue
		}
		x, y := s.where(i, panel)
		cx, cy := int(math.Round(x)), int(math.Round(y))
		if !panel.Contains(cx, cy) {
			continue
		}
		buf.Set(cx, cy, s.cells[i].ch, lit)
	}
}
