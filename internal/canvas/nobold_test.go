package canvas

import (
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// newTestStyler is a renderer that emits colour. A lipgloss renderer over a
// non-terminal writer detects Ascii and strips every escape, which would make
// this test pass by drawing nothing.
func newTestStyler() *Styler {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)
	return NewStyler(r)
}

// With a font that has no bold face, some terminals fall back to a family with
// no glyph for the character, so bold text arrives as a row of blocks. The
// frame is correct either way; the only remedy is not to ask for bold.
func TestBoldCanBeTurnedOff(t *testing.T) {
	cyan := ParseColor("#00FFFF")
	bold := Attr{FG: cyan, Bold: true}

	// A new styler each time: wraps are cached per Attr, so a cache built while
	// NoBold was false would answer after it was set. main sets it before
	// drawing anything for the same reason.
	render := func(a Attr) string {
		buf := NewBuffer(8, 1)
		buf.Text(0, 0, "LUMON", a)
		return buf.StringWith(newTestStyler())
	}

	NoBold = false
	was := render(bold)
	if !strings.Contains(was, "\x1b[1;") && !strings.Contains(was, "\x1b[1m") {
		t.Fatalf("a bold attribute did not ask for bold: %q", was)
	}

	NoBold = true
	defer func() { NoBold = false }()

	got := render(bold)
	if strings.Contains(got, "\x1b[1;") || strings.Contains(got, "\x1b[1m") {
		t.Errorf("bold was still asked for with NoBold set: %q", got)
	}
	// The text itself is untouched.
	if !strings.Contains(got, "LUMON") {
		t.Errorf("the text went missing: %q", got)
	}
	// The colour survives.
	if !strings.Contains(got, "38;2;0;255;255") {
		t.Errorf("the foreground colour was dropped along with the bold: %q", got)
	}
}
