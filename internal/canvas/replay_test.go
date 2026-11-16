package canvas

import (
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// These tests replay frames into a model of a terminal and check what it
// displays. The emitter keeps SGR state between runs and leaves a colour set
// across spaces, so asserting on the bytes would only restate the
// implementation.

// cellState is what a terminal would show for one cell, as the SGR parameter
// bodies in force.
type cellState struct {
	r           rune
	fg, bg      string
	bold, faint bool
	over        bool
}

// screen is a model of the terminal: a grid of cells, a cursor, and the SGR
// state in force. State persists between frames, as a terminal's does, so a
// frame that leaves something set is caught.
type screen struct {
	w, h  int
	cells []cellState
	cur   cellState
}

func newScreen(w, h int) *screen {
	return &screen{w: w, h: h, cells: make([]cellState, w*h)}
}

// apply paints one frame, from the home position the renderer moves to first.
// A line the renderer skips arrives as a bare newline and keeps what the last
// frame put there.
func (s *screen) apply(t *testing.T, frame string) {
	t.Helper()
	row, col := 0, 0
	for i := 0; i < len(frame); {
		if frame[i] == 0x1b {
			if i+1 >= len(frame) || frame[i+1] != '[' {
				t.Fatalf("an escape that is not CSI at %d", i)
			}
			j := i + 2
			for j < len(frame) && frame[j] != 'm' {
				if frame[j] != ';' && (frame[j] < '0' || frame[j] > '9') {
					t.Fatalf("a CSI that is not SGR at %d", i)
				}
				j++
			}
			if j >= len(frame) {
				t.Fatal("an unterminated sequence")
			}
			applySGR(t, &s.cur, frame[i+2:j])
			i = j + 1
			continue
		}
		if frame[i] == '\n' {
			row, col = row+1, 0
			i++
			continue
		}
		r, n := decodeRune(frame[i:])
		if row < s.h && col < s.w {
			c := s.cur
			c.r = r
			s.cells[row*s.w+col] = c
		}
		col++
		i += n
	}
}

// at is what the model is showing for one cell.
func (s *screen) at(x, y int) cellState { return s.cells[y*s.w+x] }

// applySGR folds one sequence's parameters into the state.
func applySGR(t *testing.T, s *cellState, body string) {
	t.Helper()
	if body == "" {
		// CSI m is SGR 0, a reset. sgrRun makes it unrepresentable.
		t.Fatal("the emitter wrote a bare CSI m, which is a reset")
	}
	parts := strings.Split(body, ";")
	for i := 0; i < len(parts); i++ {
		switch parts[i] {
		case "0":
			*s = cellState{}
		case "1":
			s.bold = true
		case "2":
			s.faint = true
		case "53":
			s.over = true
		case "38", "48":
			// Extended colour: 5;N or 2;R;G;B follows.
			n := 2
			if i+1 < len(parts) && parts[i+1] == "2" {
				n = 4
			}
			if i+n >= len(parts) {
				t.Fatalf("a truncated colour in %q", body)
			}
			seq := strings.Join(parts[i:i+n+1], ";")
			if parts[i] == "38" {
				s.fg = seq
			} else {
				s.bg = seq
			}
			i += n
		default:
			v, err := strconv.Atoi(parts[i])
			if err != nil {
				t.Fatalf("an SGR parameter that is not a number: %q", parts[i])
			}
			switch {
			case v >= 30 && v <= 37, v >= 90 && v <= 97:
				s.fg = parts[i]
			case v >= 40 && v <= 47, v >= 100 && v <= 107:
				s.bg = parts[i]
			default:
				t.Fatalf("an SGR parameter this emitter should not produce: %q", parts[i])
			}
		}
	}
}

// decodeRune is the next rune and how many bytes it took.
func decodeRune(s string) (rune, int) { return utf8.DecodeRuneInString(s) }

// corpus is a set of buffers covering what the game draws.
func corpus(t *testing.T) map[string]*Buffer {
	t.Helper()
	const w, h = 40, 6

	cyan := RGB(125, 227, 255)
	dim := RGB(27, 65, 73)
	bad := RGB(33, 80, 89)
	paper := RGB(220, 226, 230)
	ink := RGB(20, 30, 34)

	out := map[string]*Buffer{}

	// A grid row: a digit every six columns, gaps between.
	grid := NewBuffer(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x += 6 {
			a := Attr{FG: dim}
			switch {
			case x%18 == 0:
				a = Attr{FG: bad}
			case x%12 == 0:
				a = Attr{FG: cyan, Bold: true}
			}
			grid.Set(x, y, rune('0'+(x+y)%10), a)
		}
	}
	out["a grid row"] = grid

	// A background everywhere (the inverted theme), where a colour left set
	// across a gap must still be right.
	page := NewBufferWith(w, h, Attr{FG: ink, BG: paper})
	for x := 0; x < w; x += 4 {
		page.Set(x, 1, 'X', Attr{FG: cyan, BG: paper, Bold: true})
	}
	// A bin's inverted readout: a different background beside blanks, which
	// the background check in carries must catch.
	page.Text(2, 3, "  0%  0/55  ", Attr{FG: paper, BG: cyan})
	out["a page with paper"] = page

	// Faint, which nothing else in the corpus reaches.
	faint := NewBuffer(w, h)
	faint.Text(1, 1, "dormant", Attr{FG: dim, Faint: true})
	faint.Text(1, 2, "awake", Attr{FG: cyan})
	out["a faint run"] = faint

	// A bar's outline: its top an overline, over its fill's empty part and
	// the blanks around it, beside cells with none.
	bar := NewBuffer(w, h)
	for y := 1; y < 4; y++ {
		bar.Set(2, y, '▕', Attr{FG: cyan})
		bar.Text(3, y, "███", Attr{FG: cyan})
		bar.Text(6, y, "▁▁▁▁", Attr{FG: cyan, Over: true})
		bar.Text(10, y, "  ", Attr{FG: cyan, Over: true})
		bar.Set(12, y, '▏', Attr{FG: cyan})
		bar.Text(14, y, "  x", Attr{FG: dim})
	}
	bar.Text(6, 4, "  ", Attr{FG: dim, Over: true})
	out["a bar's outline"] = bar

	// Empty: the fast path, which must stay free of escapes.
	out["a blank page"] = NewBuffer(w, h)

	// A shuffle over a small alphabet, for orderings nobody thought of.
	rng := rand.New(rand.NewSource(7))
	alphabet := []Attr{
		{}, {FG: cyan}, {FG: dim}, {FG: cyan, Bold: true}, {FG: ink, BG: paper},
		{FG: paper, BG: cyan}, {FG: dim, Faint: true}, {BG: paper},
		{FG: cyan, Over: true}, {FG: dim, Bold: true, Over: true},
	}
	noise := NewBuffer(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r := ' '
			if rng.Intn(3) > 0 {
				r = rune('a' + rng.Intn(26))
			}
			noise.Set(x, y, r, alphabet[rng.Intn(len(alphabet))])
		}
	}
	out["a shuffle"] = noise

	return out
}

// The background must match everywhere; the foreground and the weights only
// where there is a glyph. A space is emitted under whatever state is current
// as long as the background agrees.
func TestFrameReplaysToTheSameScreen(t *testing.T) {
	for name, b := range corpus(t) {
		w, h := b.Size()
		sc := newScreen(w, h)
		sc.apply(t, b.StringWith(nil))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				got := sc.at(x, y)
				want := b.attrs[y*w+x]
				r := b.runes[y*w+x]

				if got.r != r {
					t.Errorf("%s: (%d,%d) shows %q, want %q", name, x, y, got.r, r)
					continue
				}
				if got.bg != bodyOf(want, true) {
					t.Errorf("%s: (%d,%d) has background %q, want %q",
						name, x, y, got.bg, bodyOf(want, true))
				}
				// An overline shows over a space as over a glyph.
				if over := strings.Contains(buildDefault(want).prefix, overline); got.over != over {
					t.Errorf("%s: (%d,%d) %q has overline %v, want %v", name, x, y, r, got.over, over)
				}
				if r == ' ' {
					continue // nothing else is visible
				}
				if got.fg != bodyOf(want, false) {
					t.Errorf("%s: (%d,%d) %q has foreground %q, want %q",
						name, x, y, r, got.fg, bodyOf(want, false))
				}
				if got.bold != (want.Bold && !NoBold) || got.faint != want.Faint {
					t.Errorf("%s: (%d,%d) %q has bold=%v faint=%v, want %v/%v",
						name, x, y, r, got.bold, got.faint, want.Bold, want.Faint)
				}
			}
		}
	}
}

// bodyOf is the parameter body lipgloss uses for one half of an attribute.
func bodyOf(a Attr, background bool) string {
	c := a.FG
	if background {
		c = a.BG
	}
	if !c.Set() {
		return ""
	}
	probe := Attr{FG: c}
	if background {
		probe = Attr{BG: c}
	}
	body, _ := splitBody(buildWrap(probe, (*Styler)(nil).newStyle()).prefix)
	return body
}

// Bubble Tea repaints only the lines whose bytes changed, emitting a bare
// newline for the rest, so a line must not inherit SGR state from the line
// above. This fails if the state is carried between rows to save the closing
// reset.
func TestASkippedLineLeavesTheNextOneCorrect(t *testing.T) {
	const w, h = 24, 4
	cyan := RGB(125, 227, 255)

	// Row 1 is heavily styled and identical in both frames, so the renderer
	// skips it; row 2 begins with spaces and changes, so it is painted after
	// whatever row 1 left behind.
	build := func(tail string) *Buffer {
		b := NewBuffer(w, h)
		b.Text(0, 1, "STYLED", Attr{FG: cyan, Bold: true})
		b.Text(4, 2, tail, Attr{FG: RGB(27, 65, 73)})
		return b
	}
	first, second := build("aaaa"), build("bbbb")

	a := strings.Split(first.StringWith(nil), "\n")
	c := strings.Split(second.StringWith(nil), "\n")

	// Frame one in full, then frame two the way the renderer sends it.
	sc := newScreen(w, h)
	sc.apply(t, first.StringWith(nil))

	var sb strings.Builder
	skipped := 0
	for i := range c {
		if i < len(a) && a[i] == c[i] {
			skipped++ // the renderer emits the newline and nothing else
		} else {
			sb.WriteString(c[i])
		}
		if i < len(c)-1 {
			sb.WriteByte('\n')
		}
	}
	if skipped == 0 {
		t.Fatal("no line was skipped, so this test proves nothing")
	}
	sc.apply(t, sb.String())

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			got, want, r := sc.at(x, y), second.attrs[y*w+x], second.runes[y*w+x]
			if got.r != r {
				t.Errorf("(%d,%d) shows %q, want %q", x, y, got.r, r)
				continue
			}
			if r == ' ' {
				continue
			}
			if got.fg != bodyOf(want, false) || got.bold != (want.Bold && !NoBold) {
				t.Errorf("(%d,%d) %q came out styled %q/bold=%v, want %q/bold=%v",
					x, y, r, got.fg, got.bold, bodyOf(want, false), want.Bold)
			}
		}
	}
}

func TestEveryLineEndsInTheDefaultState(t *testing.T) {
	for name, b := range corpus(t) {
		out := b.StringWith(nil)
		for i, line := range strings.Split(out, "\n") {
			var s cellState
			for j := 0; j < len(line); {
				if line[j] == 0x1b {
					k := j + 2
					for k < len(line) && line[k] != 'm' {
						k++
					}
					applySGR(t, &s, line[j+2:k])
					j = k + 1
					continue
				}
				_, n := decodeRune(line[j:])
				j += n
			}
			if s != (cellState{}) {
				t.Errorf("%s: line %d ends styled %+v, want the default state", name, i, s)
			}
		}
	}
}

func TestNoEmptySGRSequence(t *testing.T) {
	for name, b := range corpus(t) {
		if out := b.StringWith(nil); strings.Contains(out, "\x1b[m") {
			t.Errorf("%s: the frame holds a bare CSI m", name)
		}
	}
}

// carries emits a space under whatever state is set as long as the background
// agrees. That is safe for a foreground and a stroke weight, and unsafe for
// reverse, underline, strikethrough, blink and overline, which all make an
// empty cell visible. If this fails because Attr has a new field, teach carries
// about it before using the field.
func TestAttrCarriesNothingASpaceCouldShow(t *testing.T) {
	want := []string{"FG", "BG", "Bold", "Faint", "Over"}
	var got []string
	for i, ty := 0, reflect.TypeOf(Attr{}); i < ty.NumField(); i++ {
		got = append(got, ty.Field(i).Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Attr is now %v, was %v.\n"+
			"A space is drawn in whatever state is current as long as the background matches, "+
			"so a new field that a blank cell can display must be added to carries first.",
			got, want)
	}
}

func TestTheDeltaEmitterNeverWritesMoreThanTheOldOne(t *testing.T) {
	for name, b := range corpus(t) {
		got, want := len(b.StringWith(nil)), len(runsEmitter(b, nil))
		if got > want {
			t.Errorf("%s: %d bytes against the run emitter's %d", name, got, want)
		} else {
			t.Logf("%s: %d bytes against the run emitter's %d, %.0f%% saved",
				name, got, want, 100*(1-float64(got)/float64(want)))
		}
	}
}

// runsEmitter is the simple emitter: one full prefix and one full reset around
// every run of identical cells. It is the baseline the delta emitter's saving
// is measured against.
func runsEmitter(b *Buffer, st *Styler) string {
	var sb strings.Builder
	for y := 0; y < b.h; y++ {
		base := y * b.w
		x := 0
		for x < b.w {
			a := b.attrs[base+x]
			x2 := x + 1
			for x2 < b.w && b.attrs[base+x2] == a {
				x2++
			}
			w := st.wrap(a)
			if a != defaultAttr {
				sb.WriteString(w.prefix)
			}
			for i := x; i < x2; i++ {
				sb.WriteRune(b.runes[base+i])
			}
			if a != defaultAttr {
				sb.WriteString(w.suffix)
			}
			x = x2
		}
		if y < b.h-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// StringWith writes a byte instead of a rune for anything under RuneSelf,
// because strings.Builder.WriteRune exceeds the inlining budget (115 against
// 80). This checks every other rune the screen uses still comes through: box
// drawing, braille, the banner's mark and the glyph ramp.
func TestTheEmitterWritesEveryRuneItIsGiven(t *testing.T) {
	runes := []rune{
		'0', '9', 'A', 'z', ' ', '~', '!',
		0x7f, 0x80, // the boundary itself: the last byte and the first that is not

		'█', '▓', '▒', '░', '·', // the glyph ramp
		'─', '│', '┌', '┐', '└', '┘', // box drawing
		'⠁', '⠿', '⣿', BrailleBlank, // the braille layer
		'◉', '●', // the banner's mark and the link light
		'é', '中', '🙂', // not on this screen, and must still survive
	}

	b := NewBuffer(len(runes), 2)
	cyan := RGB(125, 227, 255)
	for i, r := range runes {
		b.Set(i, 0, r, Attr{})         // unstyled, the fast path
		b.Set(i, 1, r, Attr{FG: cyan}) // styled, so a delta precedes it
	}

	sc := newScreen(len(runes), 2)
	sc.apply(t, b.StringWith(nil))
	for y := 0; y < 2; y++ {
		for i, want := range runes {
			if got := sc.at(i, y).r; got != want {
				t.Errorf("row %d cell %d came out as %q, want %q", y, i, got, want)
			}
		}
	}
}
