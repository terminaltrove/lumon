package theme

import (
	"github.com/terminaltrove/lumon/internal/canvas"
	"testing"
)

func rgbOf(t *testing.T, c canvas.Color) (int, int, int) {
	t.Helper()
	r, g, b, ok := ParseHex(c.String())
	if !ok {
		t.Fatalf("colour %q did not parse", c)
	}
	return r, g, b
}

func TestBlendAtZeroIsTheFirstColour(t *testing.T) {
	if got := BlendColors(Cyan, ScaryRed, 0); got != Cyan {
		t.Errorf("BlendColors at 0 = %s, want %s", got, Cyan)
	}
}

func TestBlendAtOneIsTheSecondColour(t *testing.T) {
	if got := BlendColors(Cyan, ScaryRed, 1); got != ScaryRed {
		t.Errorf("BlendColors at 1 = %s, want %s", got, ScaryRed)
	}
}

func TestBlendAtAHalfIsBetweenTheTwo(t *testing.T) {
	ar, ag, ab := rgbOf(t, Cyan)
	br, bg, bb := rgbOf(t, ScaryRed)
	mr, mg, mb := rgbOf(t, BlendColors(Cyan, ScaryRed, 0.5))

	between := func(v, lo, hi int) bool {
		if lo > hi {
			lo, hi = hi, lo
		}
		return v >= lo && v <= hi
	}
	if !between(mr, ar, br) || !between(mg, ag, bg) || !between(mb, ab, bb) {
		t.Errorf("midpoint (%d,%d,%d) is not between (%d,%d,%d) and (%d,%d,%d)",
			mr, mg, mb, ar, ag, ab, br, bg, bb)
	}
}

func TestBlendClampsOutsideTheRange(t *testing.T) {
	if got := BlendColors(Cyan, ScaryRed, -3); got != Cyan {
		t.Errorf("BlendColors at -3 = %s, want %s", got, Cyan)
	}
	if got := BlendColors(Cyan, ScaryRed, 5); got != ScaryRed {
		t.Errorf("BlendColors at 5 = %s, want %s", got, ScaryRed)
	}
}

func TestBlendRedRisesMonotonically(t *testing.T) {
	last := -1
	for i := 0; i <= 20; i++ {
		r, _, _ := rgbOf(t, BlendColors(Cyan, ScaryRed, float64(i)/20))
		if r < last {
			t.Fatalf("red went %d -> %d at t=%v", last, r, float64(i)/20)
		}
		last = r
	}
}

func TestBlendHandlesUnparseableColours(t *testing.T) {
	// Indexed colours cannot be interpolated, so it falls back to one end.
	if got := BlendColors(canvas.ANSI(5), ScaryRed, 0.5); !got.Set() {
		t.Error("blending an unparseable colour produced an empty colour")
	}
}

// The urgency ramp itself.

func TestScaryColourStartsCyan(t *testing.T) {
	if got := ScaryColorAt(0); got != Cyan {
		t.Errorf("a freshly woken cluster is %s, want %s", got, Cyan)
	}
}

func TestScaryColourRedensOverTime(t *testing.T) {
	last := -1
	for s := 0.0; s <= ScaryRedSeconds; s += ScaryRedSeconds / 16 {
		r, _, _ := rgbOf(t, ScaryColorAt(s))
		if r < last {
			t.Fatalf("red went %d -> %d at %vs", last, r, s)
		}
		last = r
	}
}

func TestScaryColourClampsAtRed(t *testing.T) {
	if got := ScaryColorAt(ScaryRedSeconds * 3); got != ScaryRed {
		t.Errorf("after %vs the colour is %s, want %s", ScaryRedSeconds*3, got, ScaryRed)
	}
}

func TestScaryRedIsActuallyRed(t *testing.T) {
	r, g, b := rgbOf(t, ScaryRed)
	if r <= g || r <= b {
		t.Errorf("ScaryRed %s is not dominated by its red channel", ScaryRed)
	}
}

func TestHoverColourIsWhiteAndBold(t *testing.T) {
	r, g, b := rgbOf(t, AttrHovered.FG)
	if r < 240 || g < 240 || b < 240 {
		t.Errorf("AttrHovered.FG %s is not white", AttrHovered.FG)
	}
	if !AttrHovered.Bold {
		t.Error("AttrHovered is not bold")
	}
}

// The hint cannot use Footer: it has to stand out from the muted text around
// it. Checked across every theme because Peak is white on
// the dark themes and dark ink on the pale one, where a hardcoded white hint
// would vanish.
func TestEveryThemeDrawsItsHintInPeak(t *testing.T) {
	for _, th := range Themes() {
		if th.Hint.FG != th.Peak {
			t.Errorf("%s: the hint is %v, want the palette's Peak %v",
				th.Name, th.Hint.FG, th.Peak)
		}
		if th.Hint.FG == th.Footer.FG {
			t.Errorf("%s: the hint is drawn in the same shade as the furniture "+
				"beside it, %v", th.Name, th.Hint.FG)
		}
		// A pale theme sets its paper as the background, or the terminal's own
		// background shows through.
		if th.Paper.Set() && th.Hint.BG != th.Paper {
			t.Errorf("%s: the hint carries no paper; it was left out of the list "+
				"in NewTheme", th.Name)
		}
	}
}
