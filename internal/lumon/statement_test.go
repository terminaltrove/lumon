package lumon

import (
	"math"
	"strings"
	"testing"

	"github.com/terminaltrove/lumon/internal/canvas"
	"github.com/terminaltrove/lumon/internal/glyph"
)

// diverting starts a diversion by name, as perpetuity_test.go's bouncing does,
// and waits for the digits to land.
func diverting(t *testing.T, name string, w, h int) *Model {
	t.Helper()
	m := ready()
	m.Resize(w, h)
	typeWord(m, theWord)
	found := false
	for i, d := range diversions {
		if d.Name == name {
			m.divertAt, found = i, true
		}
	}
	if !found {
		t.Fatalf("%s is not in the menu", name)
	}
	m.Update(key("enter"))
	arrive(t, m)
	tick(m, 30)
	return m
}

// set() skips a character with no outline without saying so, and the alphabet
// has no J, Q or X.
func TestEveryCharacterPraiseSetsIsDrawable(t *testing.T) {
	for _, line := range praiseLines {
		for _, r := range line {
			if glyph.LetterStrokes(r) == nil {
				t.Errorf("no shape for %q, which PRAISE needs", r)
			}
		}
	}
}

// said runs a statement until its first window is up and set.
func said(t *testing.T, m *Model) {
	t.Helper()
	for i := 0; i < 60*20; i++ {
		if s := m.comply; s != nil && len(s.up) > 0 {
			for j := range s.up {
				if s.up[j] >= 1 {
					return
				}
			}
		}
		tick(m, 1)
	}
	t.Fatal("nothing was ever said")
}

// recited runs it until the full passage has been through once.
func recited(t *testing.T, m *Model) {
	t.Helper()
	for i := 0; i < 60*240; i++ {
		if m.comply != nil && m.comply.Wound() {
			return
		}
		tick(m, 1)
	}
	t.Fatal("the recital never got through the passage")
}

// Once the passage is set in glyphs its words are not strings in the rendered
// row, so this asks the layout what it holds and watches the recital to the end
// to see every line reached.
func TestTheStatementReadsAsTheWholePassage(t *testing.T) {
	m := diverting(t, "COMPUNCTION", 120, 40)
	s := m.comply

	// Every word of the passage is somewhere in the layout.
	var laid strings.Builder
	for _, line := range s.lines {
		laid.WriteString(line)
		laid.WriteByte(' ')
	}
	for _, line := range theStatement {
		for _, word := range strings.Fields(line) {
			if !strings.Contains(laid.String(), word) {
				t.Errorf("the layout is missing %q", word)
				break
			}
		}
	}

	// And the recital reaches every line, past the first window.
	seen := map[int]bool{}
	for i := 0; i < 60*240 && !s.Wound(); i++ {
		tick(m, 1)
		for j := range s.cells {
			if s.up[j] >= 1 {
				seen[s.cells[j].line] = true
			}
		}
	}
	if len(seen) < len(s.lines) {
		t.Errorf("%d of %d lines were ever said", len(seen), len(s.lines))
	}
}

// It sets to whatever it is given, and never outside the panel.
func TestTheStatementWrapsToWhateverItIsGiven(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}, {200, 50}, {60, 18}} {
		w, h := size[0], size[1]
		m := diverting(t, "COMPUNCTION", w, h)
		panel := m.gridPanel().Play()

		s := m.comply
		if s == nil {
			t.Fatalf("%dx%d: the statement is not running", w, h)
		}
		if len(s.cells) == 0 {
			t.Errorf("%dx%d: the statement laid out no characters", w, h)
			continue
		}

		// A character is one cell, so its slot is its full footprint.
		for i := range s.cells {
			x, y := s.slot(s.cells[i], panel)
			if !panel.Contains(int(x), int(y)) {
				t.Errorf("%dx%d: a character at (%.0f,%.0f) does not fit the panel %+v",
					w, h, x, y, panel)
				break
			}
		}
	}
}

func TestTheStatementNeverBreaksAWord(t *testing.T) {
	for _, width := range []int{20, 30, 48, 72, 110} {
		joined := strings.Join(wrapStatement(theStatement, width), " ")
		want := strings.Join(theStatement, " ")
		if strings.Join(strings.Fields(joined), " ") != strings.Join(strings.Fields(want), " ") {
			t.Errorf("at width %d the words came back changed:\n got %q\nwant %q",
				width, joined, want)
		}
		for _, line := range wrapStatement(theStatement, width) {
			if n := len([]rune(line)); n > width {
				t.Errorf("at width %d a line is %d wide: %q", width, n, line)
			}
		}
	}
}

// Digits gives each character a place with a board number, and Transit takes
// that number on the way home, so the statement assembles out of the grid and
// scatters back into it. A place with no digit would send back a zero rune.
func TestTheWordsAreMadeOfTheBoardsOwnDigits(t *testing.T) {
	m := diverting(t, "COMPUNCTION", 120, 40)
	places := m.comply.Digits(m.gridPanel().Play())

	if len(places) < 100 {
		t.Fatalf("the statement offered %d places, want one per character", len(places))
	}
	for _, p := range places {
		if p.Digit < '0' || p.Digit > '9' {
			t.Errorf("a place carries %q, want one of the board's digits", p.Digit)
			break
		}
	}

	// And there is a place for every character it draws.
	if len(places) != len(m.comply.cells) {
		t.Errorf("%d places against %d characters drawn", len(places), len(m.comply.cells))
	}
}

// Digits is what Transit flies the board to, so on the way in it reports where
// the digits land, and at the end where the flight home sets off from. It
// reports the present state, so one function serves both; this checks both ends.
func TestTheStatementArrivesAsACoilAndIsRecitedOutOfIt(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	panel := m.gridPanel().Play()

	s := NewStatement(m)
	s.layout(panel)
	if len(s.cells) < 100 {
		t.Fatalf("the passage laid out %d characters", len(s.cells))
	}

	// It arrives wound: nothing has been said, so every character is on the coil
	// and nowhere near its line.
	start := s.Digits(panel)
	near := 0
	for i := range s.cells {
		x, y := s.slot(s.cells[i], panel)
		if math.Hypot(start[i].X-x, start[i].Y-y) <= 2 {
			near++
		}
	}
	if near > len(s.cells)/4 {
		t.Errorf("%d of %d characters land within two cells of their own line; the "+
			"digits are flying to the passage rather than to a coil",
			near, len(s.cells))
	}

	// And every one lands inside the panel, or the flight goes where nothing is
	// drawn.
	for i, p := range start {
		if !panel.Contains(int(p.X), int(p.Y)) {
			t.Fatalf("character %d lands at %.1f,%.1f, outside the panel %v",
				i, p.X, p.Y, panel)
		}
	}

	// A spiral, not a ring: the coil starts against the rim and finishes well
	// inside it.
	hx, hy := float64(panel.W-1)/2, float64(panel.H-1)/2
	cx, cy := float64(panel.X)+hx, float64(panel.Y)+hy
	out := func(p Placed) float64 {
		return math.Max(math.Abs(p.X-cx)/hx, math.Abs(p.Y-cy)/hy)
	}
	first, last := out(start[0]), out(start[len(start)-1])
	if first < 0.9 {
		t.Errorf("the coil starts %.2f of the way out; it should begin at the rim", first)
	}
	if last > first-statementDepth/2 {
		t.Errorf("the coil ends %.2f of the way out against %.2f at the start; it is "+
			"a ring rather than a spiral", last, first)
	}

	// Then it is said, a character at a time and in order. The first character of a
	// line is up before the last one of it has begun to move.
	for i := 0; i < 60*20; i++ {
		s.Step(1.0 / 60)
	}
	up, still := 0, 0
	for i := range s.cells {
		if s.cells[i].line != s.at {
			continue
		}
		if s.up[i] >= 1 {
			up++
		}
		if s.up[i] <= 0 {
			still++
		}
	}
	if up == 0 {
		t.Error("twenty seconds in, nothing of the line being said has arrived")
	}

	// Exactly on its line, not asymptotically near it: a character a hundredth out
	// rounds into the next cell and breaks a word.
	end := s.Digits(panel)
	for i := range s.cells {
		if s.up[i] < 1 {
			continue
		}
		x, y := s.slot(s.cells[i], panel)
		if end[i].X != x || end[i].Y != y {
			t.Fatalf("character %d has arrived but is at %v,%v against a slot at %v,%v",
				i, end[i].X, end[i].Y, x, y)
		}
	}
}

// The stagger makes it a recital. Without it every character of a window rises
// together, like a card being held up.
func TestAWindowWritesItselfOutRatherThanAppearing(t *testing.T) {
	m := diverting(t, "COMPUNCTION", 120, 40)
	s := m.comply

	// Watch one window come up and count how often the line being said is part way
	// through: some characters arrived, some not yet begun.
	partly := 0
	for i := 0; i < 60*8; i++ {
		tick(m, 1)
		up, waiting := 0, 0
		for j := range s.cells {
			if s.cells[j].line != s.at {
				continue
			}
			// Arrived, or not yet. "Has not started" can never overlap: by the time the
			// first character has finished rising the last one has begun.
			if s.up[j] >= 1 {
				up++
			} else {
				waiting++
			}
		}
		if up > 0 && waiting > 0 {
			partly++
		}
	}
	if partly < 30 {
		t.Errorf("the line being said was part written on only %d of 480 frames; it "+
			"is appearing rather than being recited", partly)
	}
}

// The arithmetic is in fit: at the four-row setting the passage wraps to 17
// lines and needs 68 rows against a panel of 21, and even at two rows it needs
// 24. So it is set in ordinary characters. Asserted against the rendered
// buffer: every clause is found as a string on screen in one frame, and no
// clause is off the panel.
func TestTheWholePassageIsOnOnePage(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {160, 50}, {100, 30}, {80, 24}} {
		w, h := size[0], size[1]
		m := diverting(t, "COMPUNCTION", w, h)
		s := m.comply
		panel := m.gridPanel().Play()

		// Every line of the passage is shown, not a window on to some of them.
		if s.rows != len(s.lines) {
			t.Errorf("%dx%d: the window shows %d of %d lines; the passage is not all "+
				"on one page", w, h, s.rows, len(s.lines))
		}
		if tall := s.rows*(1+s.lead) - s.lead; tall > panel.H {
			t.Errorf("%dx%d: %d lines at a lead of %d need %d rows of %d",
				w, h, s.rows, s.lead, tall, panel.H)
		}

		// The recital writes the clauses out, so wait for the last of them.
		var screen string
		want := len(s.lines)
		for i := 0; i < 60*90; i++ {
			tick(m, 1)
			screen = screenOf(m)
			got := 0
			for _, line := range s.lines {
				if strings.Contains(screen, line) {
					got++
				}
			}
			if got == want {
				break
			}
		}
		for _, line := range s.lines {
			if !strings.Contains(screen, line) {
				t.Errorf("%dx%d: %q is not on screen with the rest of the passage:\n%s",
					w, h, line, screen)
				break
			}
		}
	}
}

// A block rune anywhere on the panel means a large setting has come back, and
// with it the window that showed a few clauses at a time.
func TestThePassageIsPlainTextAtEverySize(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {60, 18}} {
		m := diverting(t, "COMPUNCTION", size[0], size[1])
		said(t, m)
		panel := m.gridPanel().Play()
		buf := drawTo(m)
		for y := panel.Y; y < panel.Y+panel.H; y++ {
			for _, r := range []rune(buf.Row(y))[panel.X : panel.X+panel.W] {
				if r == glyph.GlyphSolid || strings.ContainsRune(glyph.GlyphRamp, r) {
					t.Fatalf("%dx%d: a glyph block is on screen", size[0], size[1])
				}
				if r >= canvas.BrailleBlank && r <= canvas.BrailleBlank+0xFF {
					t.Fatalf("%dx%d: a braille letter is on screen", size[0], size[1])
				}
			}
		}
	}
}

// The card it is read off in the show is caps throughout, as is the rest of
// this terminal.
func TestTheStatementIsSetInCapitals(t *testing.T) {
	for _, line := range theStatement {
		if line != strings.ToUpper(line) {
			t.Errorf("%q has lower case in it", line)
		}
	}

	// The last clause is the card's; the transcription drops a word.
	want := "AND THAT IS ALL THAT I AM."
	if got := theStatement[len(theStatement)-1]; got != want {
		t.Errorf("the statement ends %q, want %q", got, want)
	}

	// Asked of the layout, not the screen: once set in glyphs the passage is not a
	// string in the buffer.
	m := diverting(t, "COMPUNCTION", 120, 40)
	for _, c := range m.comply.cells {
		if c.ch >= 'a' && c.ch <= 'z' {
			t.Fatalf("the layout is setting %q", c.ch)
		}
	}
}

// With all of it on one page there is nowhere to advance to, so cycling would
// only take the passage away from somebody reading it. Watched well past a hold.
func TestThePassageStaysUpOnceItIsSaid(t *testing.T) {
	m := diverting(t, "COMPUNCTION", 120, 40)
	s := m.comply
	recited(t, m)

	// Every character is home, and stays home.
	for n := 0; n < 60*40; n++ {
		tick(m, 1)
		for i := range s.up {
			if s.up[i] < 1 {
				t.Fatalf("%.1fs after the passage was said, character %d is at %.2f; "+
					"it is falling back to the coil", float64(n)/60, i, s.up[i])
			}
		}
	}
	if s.at != 0 {
		t.Errorf("the window moved to line %d; there is nowhere for it to go", s.at)
	}

	// And it is still readable at the end of that.
	screen := screenOf(m)
	for _, line := range s.lines {
		if !strings.Contains(screen, line) {
			t.Errorf("%q is gone from the screen:\n%s", line, screen)
			break
		}
	}
}
