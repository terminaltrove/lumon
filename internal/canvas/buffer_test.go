package canvas

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"io"
)

// lipgloss detects colour support from the output device, and `go test` is not
// a TTY, so without a forced profile all styling would be stripped.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

func TestNewBufferIsFilledWithSpaces(t *testing.T) {
	b := NewBuffer(3, 2)

	if got := b.String(); got != "   \n   " {
		t.Errorf("String() = %q, want two rows of three spaces", got)
	}
}

func TestBufferRejectsNonPositiveDimensions(t *testing.T) {
	b := NewBuffer(0, -5)

	w, h := b.Size()
	if w < 1 || h < 1 {
		t.Errorf("Size = %dx%d, want at least 1x1", w, h)
	}
}

func TestSetWritesASingleCell(t *testing.T) {
	b := NewBuffer(3, 1)

	b.Set(1, 0, 'X', Attr{})

	if got := b.String(); got != " X " {
		t.Errorf("String() = %q, want \" X \"", got)
	}
}

// Every region of the screen blits into one shared buffer, so writes off the
// edge must clip, not panic.
func TestSetIgnoresOutOfBoundsWrites(t *testing.T) {
	b := NewBuffer(2, 1)

	b.Set(-1, 0, 'X', Attr{})
	b.Set(0, -1, 'X', Attr{})
	b.Set(99, 0, 'X', Attr{})
	b.Set(0, 99, 'X', Attr{})

	if got := b.String(); got != "  " {
		t.Errorf("String() = %q, want untouched spaces", got)
	}
}

func TestTextWritesARun(t *testing.T) {
	b := NewBuffer(6, 1)

	b.Text(1, 0, "abc", Attr{})

	if got := b.String(); got != " abc  " {
		t.Errorf("String() = %q", got)
	}
}

func TestTextClipsAtTheRightEdge(t *testing.T) {
	b := NewBuffer(3, 1)

	b.Text(1, 0, "abcdef", Attr{})

	if got := b.String(); got != " ab" {
		t.Errorf("String() = %q, want the overflow dropped", got)
	}
}

func TestTextHandlesMultiByteRunes(t *testing.T) {
	b := NewBuffer(3, 1)

	b.Text(0, 0, "⣿⠿", Attr{})

	if got := b.String(); got != "⣿⠿ " {
		t.Errorf("String() = %q", got)
	}
}

func TestHLineDrawsARunOfOneRune(t *testing.T) {
	b := NewBuffer(5, 1)

	b.HLine(1, 0, 3, '═', Attr{})

	if got := b.String(); got != " ═══ " {
		t.Errorf("String() = %q", got)
	}
}

func TestBoxDrawsABorder(t *testing.T) {
	b := NewBuffer(4, 3)

	b.Box(0, 0, 4, 3, Attr{})

	want := "┌──┐\n│  │\n└──┘"
	if got := b.String(); got != want {
		t.Errorf("String() =\n%q\nwant\n%q", got, want)
	}
}

func TestBoxIgnoresDegenerateSizes(t *testing.T) {
	b := NewBuffer(4, 3)

	b.Box(0, 0, 1, 3, Attr{})
	b.Box(0, 0, 4, 1, Attr{})

	if strings.ContainsAny(b.String(), "┌┐└┘─│") {
		t.Errorf("String() = %q, want nothing drawn", b.String())
	}
}

func TestUnstyledBufferEmitsNoEscapeSequences(t *testing.T) {
	b := NewBuffer(10, 2)
	b.Text(0, 0, "plain", Attr{})

	if strings.Contains(b.String(), "\x1b") {
		t.Errorf("String() contains an escape sequence: %q", b.String())
	}
}

func TestAdjacentCellsWithTheSameAttrRenderAsOneRun(t *testing.T) {
	styled := Attr{FG: ParseColor("#FF0000")}
	b := NewBuffer(4, 1)
	b.Text(0, 0, "abcd", styled)

	out := b.String()

	// One run means the escape sequence opening the colour appears once.
	if n := strings.Count(out, "\x1b["); n == 0 {
		t.Fatalf("expected styling, got %q", out)
	}
	single := NewBuffer(1, 1)
	single.Set(0, 0, 'a', styled)
	perCell := strings.Count(single.String(), "\x1b[")
	if n := strings.Count(out, "\x1b["); n != perCell {
		t.Errorf("4 same-attr cells produced %d escape sequences, want %d (one run)", n, perCell)
	}
}

func TestDifferentAttrsBreakTheRun(t *testing.T) {
	b := NewBuffer(2, 1)
	b.Set(0, 0, 'a', Attr{FG: ParseColor("#FF0000")})
	b.Set(1, 0, 'b', Attr{FG: ParseColor("#00FF00")})

	out := b.String()
	if !strings.Contains(out, "a") || !strings.Contains(out, "b") {
		t.Fatalf("lost content: %q", out)
	}
	if n := strings.Count(out, "\x1b["); n < 2 {
		t.Errorf("got %d escape sequences, want at least 2 for two different attrs", n)
	}
}

func TestRowsAreNewlineSeparatedWithoutATrailingNewline(t *testing.T) {
	b := NewBuffer(2, 3)

	got := b.String()

	if n := strings.Count(got, "\n"); n != 2 {
		t.Errorf("got %d newlines for 3 rows, want 2", n)
	}
	if strings.HasSuffix(got, "\n") {
		t.Errorf("String() = %q, want no trailing newline", got)
	}
}

func TestClearResetsContentAndStyling(t *testing.T) {
	b := NewBuffer(3, 1)
	b.Text(0, 0, "abc", Attr{FG: ParseColor("#FF0000")})

	b.Clear()

	if got := b.String(); got != "   " {
		t.Errorf("String() = %q, want blanks with no styling", got)
	}
}

func TestBlitBrailleWritesCellsIntoTheBuffer(t *testing.T) {
	br := NewBraille(2, 1)
	br.Set(0, 0)
	b := NewBuffer(4, 1)

	b.BlitBraille(1, 0, br, Attr{})

	if got := b.String(); got != " ⠁⠀ " {
		t.Errorf("String() = %q, want the braille cells at x=1", got)
	}
}

func BenchmarkBufferStringMostlyBlank(b *testing.B) {
	buf := NewBuffer(120, 40)
	buf.Text(2, 1, "LUMON INDUSTRIES", Attr{FG: ParseColor("#00FFFF")})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buf.String()
	}
}

func BenchmarkBufferStringDenseGrid(b *testing.B) {
	buf := NewBuffer(120, 40)
	cyan := Attr{FG: ParseColor("#00FFFF")}
	dim := Attr{FG: ParseColor("#3A8A9A")}
	for y := 0; y < 40; y++ {
		for x := 0; x < 120; x++ {
			a := dim
			if (x+y)%7 == 0 {
				a = cyan
			}
			buf.Set(x, y, rune('0'+(x+y)%10), a)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buf.String()
	}
}

// The buffer caches escape sequences instead of calling Render per run; the
// cached form must be byte-identical to lipgloss's.
func TestStyledRunMatchesLipglossRender(t *testing.T) {
	cases := []Attr{
		{FG: ParseColor("#00FFFF")},
		{BG: ParseColor("#1A1A2E")},
		{FG: ParseColor("#FF0000"), BG: ParseColor("#000000")},
		{FG: ParseColor("#00FF88"), Bold: true},
		{FG: ParseColor("#3A8A9A"), Faint: true},
		{FG: ParseColor("#FFCC00"), BG: ParseColor("#222222"), Bold: true, Faint: true},
	}

	for _, a := range cases {
		b := NewBuffer(4, 1)
		b.Text(0, 0, "abcd", a)

		style := lipgloss.NewStyle()
		if a.FG.Set() {
			style = style.Foreground(a.FG.Lipgloss())
		}
		if a.BG.Set() {
			style = style.Background(a.BG.Lipgloss())
		}
		if a.Bold {
			style = style.Bold(true)
		}
		if a.Faint {
			style = style.Faint(true)
		}

		if got, want := b.String(), style.Render("abcd"); got != want {
			t.Errorf("Attr %+v:\n got %q\nwant %q", a, got, want)
		}
	}
}

func TestRuneAtReadsBackWhatWasWritten(t *testing.T) {
	b := NewBuffer(3, 2)
	b.Set(1, 1, 'Z', Attr{})

	if got := b.RuneAt(1, 1); got != 'Z' {
		t.Errorf("RuneAt(1,1) = %q, want 'Z'", got)
	}
	if got := b.RuneAt(0, 0); got != ' ' {
		t.Errorf("RuneAt(0,0) = %q, want a space", got)
	}
}

func TestRuneAtOutOfBoundsReturnsASpace(t *testing.T) {
	b := NewBuffer(2, 2)

	for _, c := range [][2]int{{-1, 0}, {0, -1}, {9, 0}, {0, 9}} {
		if got := b.RuneAt(c[0], c[1]); got != ' ' {
			t.Errorf("RuneAt(%d,%d) = %q, want a space", c[0], c[1], got)
		}
	}
}

func TestAttrAtReadsBackTheStyling(t *testing.T) {
	want := Attr{FG: ParseColor("#00FFFF"), Bold: true}
	b := NewBuffer(2, 1)
	b.Set(0, 0, 'x', want)

	if got := b.AttrAt(0, 0); got != want {
		t.Errorf("AttrAt(0,0) = %+v, want %+v", got, want)
	}
	if got := b.AttrAt(1, 0); got != (Attr{}) {
		t.Errorf("AttrAt(1,0) = %+v, want the zero Attr", got)
	}
}

func TestRowReturnsARowAsAString(t *testing.T) {
	b := NewBuffer(4, 2)
	b.Text(0, 1, "abc", Attr{})

	if got := b.Row(1); got != "abc " {
		t.Errorf("Row(1) = %q, want %q", got, "abc ")
	}
	if got := b.Row(99); got != "" {
		t.Errorf("Row(99) = %q, want empty", got)
	}
}

// rendererAt returns a renderer pinned to a colour depth. The profile must be
// set on the renderer: termenv.WithProfile passed to NewRenderer configures
// only the output, and the renderer still detects Ascii from a non-terminal
// writer, stripping every colour.
func rendererAt(p termenv.Profile) *lipgloss.Renderer {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(p)
	return r
}

// A cache keyed by attribute alone would let the first client on a server fix
// the escape sequences for everybody, at its colour depth.
func TestTwoStylersRenderTheSameBufferDifferently(t *testing.T) {
	draw := func(st *Styler) string {
		b := NewBuffer(6, 1)
		b.Text(0, 0, "LUMON", Attr{FG: ParseColor("#00FFFF")})
		return b.StringWith(st)
	}

	truecolor := NewStyler(rendererAt(termenv.TrueColor))
	plain := NewStyler(rendererAt(termenv.Ascii))

	rich, bare := draw(truecolor), draw(plain)

	if rich == bare {
		t.Fatal("a truecolor terminal and a monochrome one were given identical output")
	}
	if !strings.Contains(rich, "\x1b[") {
		t.Errorf("the truecolor terminal got no escape sequences: %q", rich)
	}
	if strings.Contains(bare, "\x1b[") {
		t.Errorf("the monochrome terminal got escape sequences: %q", bare)
	}
	for _, out := range []string{rich, bare} {
		if !strings.Contains(out, "LUMON") {
			t.Errorf("the text went missing: %q", out)
		}
	}
}

func TestAStylersCacheIsItsOwn(t *testing.T) {
	a := Attr{FG: ParseColor("#FF0000"), Bold: true}

	rich := NewStyler(rendererAt(termenv.TrueColor))
	bare := NewStyler(rendererAt(termenv.Ascii))

	// Warm the rich one first, the order that would poison a shared cache.
	_ = rich.wrap(a)

	if got := bare.wrap(a); got.prefix != "" || got.suffix != "" {
		t.Errorf("the monochrome terminal inherited %q/%q from the other", got.prefix, got.suffix)
	}
}

// A nil Styler is the process's own terminal, for a local run.
func TestANilStylerStillRenders(t *testing.T) {
	b := NewBuffer(6, 1)
	b.Text(0, 0, "LUMON", Attr{FG: ParseColor("#00FFFF")})

	if got := b.StringWith(nil); !strings.Contains(got, "LUMON") {
		t.Errorf("a nil Styler dropped the text: %q", got)
	}
	if b.StringWith(nil) != b.String() {
		t.Error("String and StringWith(nil) disagree")
	}
}

// A page with a background must start with it everywhere, or every cell
// nothing draws into shows the terminal's own colour.
func TestClearToPaintsEveryCell(t *testing.T) {
	page := Attr{FG: ParseColor("#123456"), BG: ParseColor("#ABCDEF")}
	b := NewBuffer(20, 5)
	b.ClearTo(page)

	w, h := b.Size()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if got := b.AttrAt(x, y); got != page {
				t.Fatalf("cell (%d,%d) is %v, want the page %v", x, y, got, page)
			}
		}
	}

	// It reaches the output instead of being optimised away as unstyled.
	if out := b.String(); !strings.Contains(out, "\x1b[") {
		t.Error("a buffer cleared to a background emitted no escape sequences")
	}
}

// Clear stays the zero attribute, for a terminal that keeps its own
// background.
func TestClearIsStillUnstyled(t *testing.T) {
	b := NewBuffer(8, 2)
	b.ClearTo(Attr{FG: ParseColor("#FF0000")})
	b.Clear()
	if got := b.AttrAt(0, 0); got != (Attr{}) {
		t.Errorf("Clear left %v behind, want the zero attribute", got)
	}
}
