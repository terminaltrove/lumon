package canvas

import (
	"io"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// The browser writes its own escape sequences without lipgloss. They must be
// byte-identical to lipgloss's for a truecolour terminal, or the browser draws
// a different screen from the one these tests pin down.
func TestTrueColorWrapMatchesLipgloss(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)
	colours := []Color{0, RGB(0, 255, 255), RGB(5, 10, 13), RGB(255, 0, 7), ANSI(0), ANSI(7), ANSI(8), ANSI(15), ANSI(16), ANSI(214), ANSI(255)}
	for _, noBold := range []bool{false, true} {
		NoBold = noBold
		for _, fg := range colours {
			for _, bg := range colours {
				for _, weight := range []Attr{{}, {Bold: true}, {Faint: true}, {Bold: true, Faint: true}, {Over: true}, {Bold: true, Over: true}} {
					a := Attr{FG: fg, BG: bg, Bold: weight.Bold, Faint: weight.Faint, Over: weight.Over}
					want, got := buildWrap(a, r.NewStyle()), trueColorWrap(a)
					if got != want {
						t.Errorf("NoBold=%v %+v: wrote %q…%q, lipgloss writes %q…%q", noBold, a, got.prefix, got.suffix, want.prefix, want.suffix)
					}
				}
			}
		}
	}
	NoBold = false
}
