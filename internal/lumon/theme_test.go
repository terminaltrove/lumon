package lumon

import (
	"github.com/terminaltrove/lumon/internal/theme"
	"strings"
	"testing"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// Remotely the package is shared, so a palette in package-level vars would let
// one refiner's colours change everybody's.
func TestAThemeBelongsToOneTerminalAndNotTheProgram(t *testing.T) {
	_, a, b := twoInARoom(t)

	before := b.Theme
	a.Update(key("t"))

	if a.Theme == before {
		t.Fatal("pressing t did not change the theme")
	}
	if b.Theme != before {
		t.Errorf("one refiner's theme change moved the other's to %q", b.Theme.Name)
	}

	// And it reaches the screen: the two draw the same board in different colours.
	ba, bb := drawTo(a), drawTo(b)
	l := a.Layout
	same := 0
	for y := 0; y < l.Height; y++ {
		for x := 0; x < l.Width; x++ {
			if ba.AttrAt(x, y) == bb.AttrAt(x, y) {
				same++
			}
		}
	}
	if same == l.Width*l.Height {
		t.Error("two refiners in different themes rendered identical screens")
	}
}

// Including the other refiners' cursors, or one player's choice leaks onto
// another's screen.
func TestEveryoneIsDrawnInTheViewersOwnPalette(t *testing.T) {
	_, a, b := twoInARoom(t)

	// Several palettes share a shade or two with the Lumon one, so use a theme whose
	// seat colours all differ from the default's.
	a.Theme, _ = mustFind(t, "dracula")
	seat := b.Desk.Seat
	if a.Theme.DeskColour(seat) == b.Theme.DeskColour(seat) {
		t.Fatalf("the two themes agree about seat %d; pick a different pair", seat)
	}

	got := a.deskColour(b.Desk)
	if want := a.Theme.DeskColour(seat); got != want {
		t.Errorf("A draws B in %v, want A's own palette's %v", got, want)
	}
	if other := b.Theme.DeskColour(seat); got == other {
		t.Errorf("A drew B in B's palette (%v) rather than A's", other)
	}
}

// Otherwise adding themes restyles what everybody already has.
func TestTheDefaultThemeIsTheTerminalAsItWas(t *testing.T) {
	for _, c := range []struct {
		name string
		got  canvas.Color
		want string
	}{
		{"primary", theme.DefaultTheme.Primary, "#00FFFF"},
		{"dim", theme.DefaultTheme.Dim, "#3A8A9A"},
		{"live", theme.DefaultTheme.Live, "#7DE3FF"},
		{"glow", theme.DefaultTheme.Glow, "#CFF6FF"},
		{"ink", theme.DefaultTheme.Ink, "#1A1A2E"},
		{"backdrop", theme.DefaultTheme.Backdrop, "#1B4149"},
		{"lurk", theme.DefaultTheme.Lurk, "#215059"},
		{"urgent", theme.DefaultTheme.Urgent, "#FF3020"},
	} {
		if c.got.String() != c.want {
			t.Errorf("the default theme's %s is %s, want %s", c.name, c.got, c.want)
		}
	}

	// And a fresh terminal starts on it.
	if m := ready(); m.Theme != theme.DefaultTheme {
		t.Errorf("a new terminal starts on %q, want the default", m.Theme.Name)
	}
}

// A zero colour renders as the terminal's default foreground, which reads as a
// hole in the palette.
func TestEveryThemeFillsEveryRole(t *testing.T) {
	for _, th := range theme.Themes() {
		roles := map[string]canvas.Color{
			"primary": th.Primary, "dim": th.Dim, "live": th.Live,
			"glow": th.Glow, "ink": th.Ink, "peak": th.Peak,
			"backdrop": th.Backdrop, "lurk": th.Lurk, "urgent": th.Urgent,
		}
		for name, c := range roles {
			if !c.Set() {
				t.Errorf("theme %q leaves %s unset", th.Name, name)
			}
		}
		for seat, c := range th.Desks {
			if !c.Set() {
				t.Errorf("theme %q leaves desk %d unset", th.Name, seat)
			}
		}
		// Paper is optional: empty means "inherit the terminal", as the dark themes
		// want. A theme that sets one must have it reach the resolved attributes.
		if th.Paper.Set() && th.Page.BG != th.Paper {
			t.Errorf("theme %q names paper %v but its page is %v", th.Name, th.Paper, th.Page.BG)
		}
		if th.Paper.Set() == false && th.Page != (canvas.Attr{}) {
			t.Errorf("theme %q names no paper but its page is %v, not the zero attribute "+
				",  which costs the unstyled fast path on every blank cell", th.Name, th.Page)
		}
		if th.Name == "" || th.Name != strings.ToLower(th.Name) {
			t.Errorf("theme name %q should be lower case and non-empty", th.Name)
		}
	}
}

// Otherwise a green terminal reddens into a colour from a different palette.
func TestAClusterReddensInItsOwnThemesColours(t *testing.T) {
	for _, th := range theme.Themes() {
		if got := th.ScaryColour(0); got != th.Primary {
			t.Errorf("theme %q starts its ramp at %v, want its primary %v", th.Name, got, th.Primary)
		}
		if got := th.ScaryColour(theme.ScaryRedSeconds * 2); got != th.Urgent {
			t.Errorf("theme %q ends its ramp at %v, want its urgent %v", th.Name, got, th.Urgent)
		}
	}
}

// A typo in --theme must not give you the default without saying so.
func TestAnUnknownThemeIsRefusedRatherThanIgnored(t *testing.T) {
	if _, ok := theme.FindTheme("no-such-theme"); ok {
		t.Error("an unknown theme name was accepted")
	}
	if th, ok := theme.FindTheme("  AMBER "); !ok || th.Name != "amber" {
		t.Error("a theme name is not matched case-insensitively and trimmed")
	}
}

func TestCyclingVisitsEveryThemeAndWrapsAround(t *testing.T) {
	seen := map[string]bool{}
	th := theme.DefaultTheme
	for i := 0; i < len(theme.Themes()); i++ {
		seen[th.Name] = true
		th = theme.NextTheme(th)
	}
	if th != theme.DefaultTheme {
		t.Errorf("a full cycle ended on %q, want back at the default", th.Name)
	}
	if len(seen) != len(theme.Themes()) {
		t.Errorf("a full cycle saw %d of %d themes", len(seen), len(theme.Themes()))
	}
}

func mustFind(t *testing.T, name string) (*theme.Theme, bool) {
	t.Helper()
	th, ok := theme.FindTheme(name)
	if !ok {
		t.Fatalf("no theme %q", name)
	}
	return th, ok
}

// The inverse of canvas's TestUnstyledBufferEmitsNoEscapeSequences. Clearing
// the buffer to the page is not enough: a rule, box edge or digit drawn with a
// foreground-only attribute replaces the cell's background, letting the
// terminal's own black show through.
func TestAPaperThemeLeavesNoCellBare(t *testing.T) {
	for _, c := range []struct {
		what string
		set  func(*Model)
	}{
		{"the grid", func(*Model) {}},
		{"the controls", func(m *Model) { m.Helping = true }},
		{"the scoreboard", func(m *Model) { m.Screen = ScreenScores }},
		{"the screensaver", func(m *Model) { m.StartScreensaver() }},
	} {
		t.Run(c.what, func(t *testing.T) {
			m := ready()
			m.Resize(96, 24)
			m.Theme = mustTheme(t, "inverted")
			c.set(m)
			tick(m, 120)

			buf := drawTo(m)
			l := m.Layout
			for y := 0; y < l.Height; y++ {
				for x := 0; x < l.Width; x++ {
					if buf.AttrAt(x, y).BG.Set() == false {
						t.Fatalf("cell (%d,%d) has no background; the terminal's own shows through", x, y)
					}
				}
			}
		})
	}
}

// Otherwise adding a pale theme would restyle the dark ones.
func TestAThemeWithNoPaperStillInheritsTheTerminal(t *testing.T) {
	m := ready()
	m.Resize(96, 24)
	tick(m, 120)

	buf := drawTo(m)
	l := m.Layout
	for y := 0; y < l.Height; y++ {
		row := []rune(buf.Row(y))
		for x := 0; x < l.Width; x++ {
			// Only blank cells, where the page shows through. The cursor and the bins are
			// blocks of colour by design.
			if x < len(row) && row[x] != ' ' {
				continue
			}
			// Entirely unstyled, foreground included: canvas writes the zero
			// attribute as raw runes with no escape, and a foreground on every blank cell
			// costs five percent of the frame.
			if got := buf.AttrAt(x, y); got.BG.Set() {
				t.Fatalf("the default theme painted a background (%v) at (%d,%d)", got.BG, x, y)
			}
		}
	}
}

// Inverted means the roles land in different places, not that the hues are
// flipped. Backdrop and Lurk recede, so against paper they must be nearer the
// paper than the text is.
func TestTheInvertedThemeRecedesTowardsItsPaper(t *testing.T) {
	th := mustTheme(t, "inverted")

	paper := luminance(th.Paper.String())
	text := luminance(th.Primary.String())
	back := luminance(th.Backdrop.String())

	if text >= paper {
		t.Errorf("the chrome (%.2f) is not darker than the paper (%.2f)", text, paper)
	}
	if back <= text || back >= paper {
		t.Errorf("the resting grid (%.2f) should sit between the chrome (%.2f) and the paper (%.2f)",
			back, text, paper)
	}
	// Peak is what you are looking at: on paper that is the darkest colour, not the
	// white the dark themes use.
	if luminance(th.Peak.String()) >= text {
		t.Error("the hovered digit is not the darkest thing on a pale screen")
	}
}

func mustTheme(t *testing.T, name string) *theme.Theme {
	t.Helper()
	th, ok := theme.FindTheme(name)
	if !ok {
		t.Fatalf("no theme %q", name)
	}
	return th
}

// theme.Palette.Desks and Tempers are slices, so `p := t.Palette` copies only
// their headers. Writing through the copy would permanently fade the colours
// every session on the server shares.
func TestDimmingTakesACopy(t *testing.T) {
	th := theme.DefaultTheme
	if len(th.Desks) == 0 || len(th.Tempers) == 0 {
		t.Fatal("the default theme names no desks or no tempers")
	}

	desks := append([]canvas.Color(nil), th.Desks...)
	tempers := append([]canvas.Color(nil), th.Tempers...)

	dim := th.Dimmed(0.4)

	for i := range desks {
		if th.Desks[i] != desks[i] {
			t.Errorf("desk %d of the source theme changed from %q to %q when a "+
				"dimmed copy was made", i, desks[i], th.Desks[i])
		}
	}
	for i := range tempers {
		if th.Tempers[i] != tempers[i] {
			t.Errorf("temper %d of the source theme changed from %q to %q when a "+
				"dimmed copy was made", i, tempers[i], th.Tempers[i])
		}
	}

	// And the copy is dimmer, or the test above would pass on a Dimmed that did
	// nothing.
	if dim.Desks[0] == th.Desks[0] {
		t.Error("the dimmed theme's first desk is the same colour as the source's")
	}
}
