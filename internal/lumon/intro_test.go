package lumon

import (
	"math"
	"strings"
	"testing"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// introReady returns a model on the boot sequence at a usable size.
func introReady() *Model {
	m := NewModel(1)
	m.Resize(120, 40)
	m.StartIntro()
	return m
}

func TestTheIntroPlaysBeforeTheGrid(t *testing.T) {
	m := introReady()

	if m.Screen != ScreenIntro {
		t.Fatalf("screen is %v after StartIntro, want the intro", m.Screen)
	}

	buf := drawTo(m)
	if strings.Contains(buf.Row(0), "% Complete") {
		t.Error("the file bar is showing while the boot sequence is playing")
	}
}

// Otherwise a player who touches nothing never gets a game.
func TestTheIntroFinishesOnItsOwn(t *testing.T) {
	m := introReady()

	for i := 0; i < 60*30 && m.Screen == ScreenIntro; i++ {
		tick(m, 1)
	}

	if m.Screen != ScreenMain {
		t.Fatal("the intro never handed over to the grid")
	}
	if m.Intro != nil {
		t.Error("the finished intro is still being held")
	}
}

func TestTheIntroPassesThroughBothStages(t *testing.T) {
	m := introReady()

	seen := map[introStage]bool{}
	for i := 0; i < 60*30 && m.Screen == ScreenIntro; i++ {
		seen[m.Intro.Stage()] = true
		tick(m, 1)
	}

	if !seen[stageBoot] {
		t.Error("the boot log never played")
	}
	if !seen[stageLogo] {
		t.Error("the logo never played")
	}
}

func TestAnyKeySkipsTheIntro(t *testing.T) {
	m := introReady()
	tick(m, 5)

	m.Update(key("x"))

	if m.Screen != ScreenMain {
		t.Error("a key press did not skip the boot sequence")
	}
}

// The log is clipped to a window, so its end, which it builds to, is always on
// screen.
func TestTheBootLogNeverRunsOffTheScreen(t *testing.T) {
	m := NewModel(1)
	m.Resize(80, 24)
	m.StartIntro()

	for i := 0; i < 60*30 && m.Screen == ScreenIntro; i++ {
		tick(m, 1)
		if m.Intro == nil || m.Intro.Stage() != stageBoot {
			continue
		}
		buf := drawTo(m)
		_, h := buf.Size()
		// The buffer drops anything past the last row, so check that the newest line is
		// still visible.
		lines := m.Intro.Lines()
		if len(lines) == 0 {
			continue
		}
		newest := strings.TrimSpace(lines[len(lines)-1])
		if newest == "" {
			continue
		}
		found := false
		for y := 0; y < h; y++ {
			if strings.Contains(buf.Row(y), newest) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("the newest log line %q is not on screen", newest)
		}
	}
}

// So a seeded session is reproducible from its first frame.
func TestTheBootLogIsDeterministicForASeed(t *testing.T) {
	a := NewModel(7)
	a.StartIntro()
	b := NewModel(7)
	b.StartIntro()

	la, lb := a.Intro.lines, b.Intro.lines
	if len(la) != len(lb) {
		t.Fatalf("same seed produced %d lines and %d", len(la), len(lb))
	}
	for i := range la {
		if la[i] != lb[i] {
			t.Fatalf("line %d differs:\n  %q\n  %q", i, la[i], lb[i])
		}
	}
}

// A boot log is a sequence of events, so its clock never jumps backwards.
func TestBootLogTimestampsOnlyGoForwards(t *testing.T) {
	m := NewModel(3)
	m.StartIntro()

	prev := ""
	for _, line := range m.Intro.lines {
		if !strings.HasPrefix(line, "[") {
			continue
		}
		stamp := line[1:strings.Index(line, "]")]
		if prev != "" && stamp < prev {
			t.Fatalf("timestamp went backwards: %s then %s", prev, stamp)
		}
		prev = stamp
	}
	if prev == "" {
		t.Fatal("the log has no timestamps at all")
	}
}

// It is the one line that types out instead of appearing.
func TestTheAuthorizeLineTypesItselfOut(t *testing.T) {
	m := introReady()

	partial := false
	for i := 0; i < 60*30 && m.Screen == ScreenIntro; i++ {
		tick(m, 1)
		if m.Intro == nil || !m.Intro.Typing() {
			continue
		}
		lines := m.Intro.Lines()
		last := lines[len(lines)-1]
		if last != authorizeLine && strings.HasPrefix(authorizeLine, last) {
			partial = true
			break
		}
	}

	if !partial {
		t.Error("the authorize line never appeared half-typed")
	}
}

// The constants and the asset's header state the same size. If they disagree,
// the mark is laid out by one and drawn at the other, which looks like a
// corrupt frame.
func TestTheLogoAssetDecodes(t *testing.T) {
	w, h, ok := LogoSize()
	if !ok {
		t.Fatalf("the logo asset did not decode: %s", logoErrStr)
	}
	if w != 96 || h != 24 {
		t.Errorf("logo frames are %dx%d, want 96x24", w, h)
	}
	if got := len(logoFrames()); got != 90 {
		t.Errorf("decoded %d frames, want 90", got)
	}
	for i := 0; i < 90; i++ {
		f := LogoFrame(i)
		if len(f) != w*h {
			t.Fatalf("frame %d has %d runes, want %d", i, len(f), w*h)
		}
		for _, r := range f {
			if r < 0x2800 || r > 0x28FF {
				t.Fatalf("frame %d contains %q, which is not braille", i, r)
			}
		}
	}
}

// A braille dot is visually square, half a column across and half a column
// down (a row is two columns tall). So the mark's aspect is its dots, 2w by 4h,
// and should match the GIF's 1462x752 within what flooring a row can move it.
// This measures against the source: a rule like four columns to the row is true
// at this width only by chance, and fails at 160, whose height is 41.
func TestTheLogoKeepsTheSourcesShape(t *testing.T) {
	const want = 1462.0 / 752.0
	got := float64(logoW*canvas.DotsX) / float64(logoH*canvas.DotsY)
	if math.Abs(got-want) > 0.08 {
		t.Errorf("the mark is %.3f wide to tall against the source's %.3f", got, want)
	}
}

// The mark is one fixed size on every terminal, centred, so this asserts that
// every terminal draws the same mark and leaves the number to the constants.
func TestTheLogoIsTheSameSizeOnEveryTerminal(t *testing.T) {
	// The last is the mark's own size exactly, the smallest terminal that still
	// gets the animation and not the static mark.
	for _, size := range [][2]int{{200, 60}, {140, 40}, {114, 30}, {96, 24}} {
		m := NewModel(1)
		m.Resize(size[0], size[1])
		m.StartIntro()
		for i := 0; i < 60*40 && m.Intro != nil && m.Intro.Stage() != stageLogo; i++ {
			tick(m, 1)
		}
		if m.Intro == nil || m.Intro.Stage() != stageLogo {
			t.Fatalf("%dx%d never reached the logo stage", size[0], size[1])
		}
		tick(m, 45)

		// The drawn extent, which is what somebody sees, not the constants.
		buf := drawTo(m)
		left, right, top, bottom := size[0], -1, size[1], -1
		for y := 0; y < size[1]; y++ {
			for x, r := range []rune(buf.Row(y)) {
				if r < 0x2800 || r > 0x28FF || r == canvas.BrailleBlank {
					continue
				}
				left, right = mini(left, x), maxi(right, x)
				top, bottom = mini(top, y), maxi(bottom, y)
			}
		}
		if right < 0 {
			t.Errorf("%dx%d drew no braille at all", size[0], size[1])
			continue
		}
		if w := right - left + 1; w > logoW {
			t.Errorf("%dx%d drew the mark %d cells wide, want at most %d",
				size[0], size[1], w, logoW)
		}
		if h := bottom - top + 1; h > logoH {
			t.Errorf("%dx%d drew the mark %d rows tall, want at most %d",
				size[0], size[1], h, logoH)
		}
	}
}
func TestASmallTerminalFallsBackToTheCompactLogo(t *testing.T) {
	m := NewModel(1)
	m.Resize(40, 16)
	m.StartIntro()

	for i := 0; i < 60*30 && m.Screen == ScreenIntro; i++ {
		tick(m, 1)
		if m.Intro != nil && m.Intro.Stage() == stageLogo {
			break
		}
	}
	if m.Intro == nil || m.Intro.Stage() != stageLogo {
		t.Skip("never reached the logo stage")
	}

	buf := drawTo(m)
	painted := 0
	_, h := buf.Size()
	for y := 0; y < h; y++ {
		for _, r := range buf.Row(y) {
			if r >= 0x2800 && r <= 0x28FF && r != '⠀' {
				painted++
			}
		}
	}
	if painted == 0 {
		t.Error("nothing was drawn on a terminal too small for the animation")
	}
}

// The ink fades, not the frames. The flipbook already draws itself stroke by
// stroke, and fading the frames would hold back strokes the asset has drawn.
// Dimming the ink overlaps with it, so the mark is drawn as the lights come up.
func TestTheLogoFadesUpWhenItStarts(t *testing.T) {
	m := introReady()
	for i := 0; i < 60*30 && (m.Intro == nil || m.Intro.Stage() != stageLogo); i++ {
		tick(m, 1)
	}
	if m.Intro == nil || m.Intro.Stage() != stageLogo {
		t.Fatal("never reached the logo stage")
	}

	if got := m.Intro.LogoFade(); got > 0.01 {
		t.Errorf("the mark starts at %.2f of its brightness, not in the dark", got)
	}
	first := m.introInk().FG

	// Part way up, and then all the way.
	tick(m, 18)
	mid := m.Intro.LogoFade()
	if mid <= 0.05 || mid >= 0.95 {
		t.Errorf("a third of a second in the fade is at %.2f; it should be under way", mid)
	}

	for i := 0; i < 60 && m.Intro != nil && m.Intro.LogoFade() < 1; i++ {
		tick(m, 1)
	}
	if m.Intro == nil {
		t.Fatal("the intro ended before the fade did")
	}
	if got := m.Intro.LogoFade(); got != 1 {
		t.Errorf("the fade stalled at %.2f", got)
	}

	// The first frame's ink is nearer the page than the last one's, measured on the
	// attribute the renderer uses, not the ratio.
	full := m.introInk().FG
	if first == full {
		t.Error("the ink is the same colour at both ends, so nothing faded")
	}
	fr, fg, fb, ok := first.Parts()
	lr, lg, lb, ok2 := full.Parts()
	if ok && ok2 && int(fr)+int(fg)+int(fb) >= int(lr)+int(lg)+int(lb) {
		t.Errorf("the mark starts at #%02x%02x%02x and ends at #%02x%02x%02x; it is not getting brighter",
			fr, fg, fb, lr, lg, lb)
	}
}

// The boot log before, and any other mark after.
func TestOnlyTheLogoStageIsFaded(t *testing.T) {
	m := introReady()
	if got := m.Intro.LogoFade(); got != 1 {
		t.Errorf("the boot log is faded to %.2f", got)
	}
	if m.introInk() != m.Theme.Boot {
		t.Error("the boot log is not drawn in its own colour")
	}

	m.Intro = nil
	if m.introInk() != m.Theme.Boot {
		t.Error("a mark drawn outside the intro is faded")
	}
}

// The animation dissolves the mark over its last dozen frames, and frame 89 has
// no dot lit. Opening the file drawer at that instant would read as the mark
// being interrupted.
func TestTheScreenHoldsDarkBeforeTheDrawer(t *testing.T) {
	m := introReady()

	// Run to the frame the flipbook ends on.
	last := len(logoFrames()) - 1
	for i := 0; i < 60*30; i++ {
		if m.Intro != nil && m.Intro.Stage() == stageLogo && m.Intro.Frame() >= last {
			break
		}
		tick(m, 1)
	}
	if m.Intro == nil || m.Intro.Frame() < last {
		t.Fatal("never reached the end of the animation")
	}

	// Nothing is lit, and it is still the intro.
	buf := drawTo(m)
	_, h := buf.Size()
	lit := 0
	for y := 0; y < h; y++ {
		for _, r := range buf.Row(y) {
			if r >= 0x2800 && r <= 0x28FF && r != canvas.BrailleBlank {
				lit++
			}
		}
	}
	if lit != 0 {
		t.Errorf("the last frame still has %d dots on it", lit)
	}

	// It holds, and then it goes.
	tick(m, int(60*logoHold)-10)
	if m.Screen != ScreenIntro {
		t.Error("the intro handed over before the hold was up")
	}
	tick(m, 20)
	if m.Screen == ScreenIntro {
		t.Error("the intro never handed over after the hold")
	}
}
