package lumon

import (
	"image"
	"math/rand"
	"strings"
	"testing"

	"github.com/terminaltrove/lumon/internal/canvas"
	"github.com/terminaltrove/lumon/internal/theme"
)

// markBox is one whole frame that draws the mark once: the draw and the sweep
// after it, because the sweep returns the placement ids.
func markBox(m *Model) (braille int, graphics []string) {
	buf := canvas.NewBuffer(40, 10)
	m.drawMark(buf, 2, 2, m.Theme.Chrome)
	m.sweepMark(buf)
	for y := 0; y < 10; y++ {
		for _, r := range buf.Row(y) {
			if r >= 0x2800 && r <= 0x28FF && r != canvas.BrailleBlank {
				braille++
			}
		}
	}
	return braille, buf.Graphics()
}

// Every other test in the package compares runes, and a default of "on" would
// leave most of the mark's box blank and change what they all expect.
func TestTheMarkIsBrailleUnlessTheTerminalCanShowAPicture(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)

	if m.graphics != GraphicsOff {
		t.Fatal("a fresh model believes its terminal can show pictures")
	}

	dots, gfx := markBox(m)
	if dots == 0 {
		t.Error("the braille mark drew no dots")
	}
	if len(gfx) != 0 {
		t.Errorf("a braille mark emitted %d escape sequences", len(gfx))
	}
}

func TestAKittyTerminalDrawsThePictureInsteadOfTheDots(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	m.UseGraphics(GraphicsKitty)

	dots, gfx := markBox(m)
	if dots != 0 {
		t.Errorf("the picture left %d braille dots under it", dots)
	}
	if len(gfx) == 0 {
		t.Fatal("nothing was placed on the terminal")
	}
	for _, seq := range gfx {
		if !strings.HasPrefix(seq, "\x1b_G") {
			t.Errorf("emitted %q, which is not a kitty graphics escape", seq[:min(12, len(seq))])
		}
	}
}

// The mark shares its rows with the file bar, which changes constantly, so
// transmitting each time would put a payload several times a frame's size on
// the wire whenever the progress moved.
func TestThePictureIsTransmittedOnceASession(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	m.UseGraphics(GraphicsKitty)

	const frames = 8
	transmitted, placed := 0, 0
	for i := 0; i < frames; i++ {
		_, gfx := markBox(m)
		for _, seq := range gfx {
			// The transmit has the image and is large; the placement is a handful of
			// options.
			if len(seq) > 1024 {
				transmitted++
			} else {
				placed++
			}
		}
	}

	if transmitted != 1 {
		t.Errorf("the picture was sent %d times over %d frames, want once", transmitted, frames)
	}
	if placed != frames {
		t.Errorf("%d placements over %d frames", placed, frames)
	}
	if n := len(markPlaces()[0]); n > 100 {
		t.Errorf("a placement is %d bytes, which is too much to repaint a line with", n)
	}
}

// A placement for an id the terminal has forgotten draws nothing and reports
// nothing, and Bubble Tea skips unchanged lines, so a lost mark would never
// come back on its own.
func TestAResizeSendsThePictureAgain(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	m.UseGraphics(GraphicsKitty)
	markBox(m) // the first send

	m.Resize(100, 36)

	sent := false
	_, gfx := markBox(m)
	for _, seq := range gfx {
		if len(seq) > 1024 {
			sent = true
		}
	}
	if !sent {
		t.Error("the picture was not re-sent after a resize")
	}
}

// A one-colour asset would be cyan on all nine palettes. The asset is coverage
// and the session tints it.
func TestTheMarkTakesTheThemesColour(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	m.UseGraphics(GraphicsKitty)

	// Fully opaque only. RGBA reports premultiplied channels, so an antialiased
	// edge pixel comes back scaled by its coverage.
	opaque := func(img image.Image) (r, g, b uint8, found bool) {
		bounds := img.Bounds()
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				cr, cg, cb, ca := img.At(x, y).RGBA()
				if ca>>8 == 0xFF {
					return uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8), true
				}
			}
		}
		return 0, 0, 0, false
	}

	for _, name := range []string{"lumon", "amber", "dracula"} {
		th, ok := theme.FindTheme(name)
		if !ok {
			t.Fatalf("no theme called %q", name)
		}
		m.Theme = th

		img := m.markImage()
		if img == nil {
			t.Fatal("the mask did not decode")
		}
		r, g, b, found := opaque(img)
		if !found {
			t.Fatalf("the %s mark is transparent everywhere", name)
		}
		wr, wg, wb, ok := th.Primary.Parts()
		if !ok {
			continue // an unset role has no colour to check against
		}
		if r != wr || g != wg || b != wb {
			t.Errorf("the %s mark is #%02x%02x%02x, want its own Primary #%02x%02x%02x",
				name, r, g, b, wr, wg, wb)
		}
	}
}

// Otherwise `t` would leave the old colour up.
func TestChangingThemeSendsANewPicture(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	m.UseGraphics(GraphicsKitty)
	markBox(m)

	th, ok := theme.FindTheme("amber")
	if !ok {
		t.Fatal("no amber theme")
	}
	m.Theme = th

	sent := false
	_, gfx := markBox(m)
	for _, seq := range gfx {
		if len(seq) > 1024 {
			sent = true
		}
	}
	if !sent {
		t.Error("a theme change did not re-send the picture")
	}
}

// kitty keeps placements in their own layer, so the EraseEntireLine Bubble Tea
// repaints with leaves the image where it was. Checked on the frame, since the
// call sites that stop drawing the mark are the ones that stop running.
func TestThePictureIsTakenDownWhenItStopsBeingDrawn(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	m.UseGraphics(GraphicsKitty)

	first := canvas.NewBuffer(40, 10)
	m.drawMark(first, 2, 2, m.Theme.Chrome)
	m.sweepMark(first)

	// A frame that draws no mark at all.
	next := canvas.NewBuffer(40, 10)
	m.sweepMark(next)

	gfx := next.Graphics()
	if len(gfx) != 1 {
		t.Fatalf("a frame without the mark emitted %d sequences, want one delete", len(gfx))
	}
	if !strings.Contains(gfx[0], "a=d") {
		t.Errorf("%q is not a delete", gfx[0])
	}

	// Only once: a third frame has nothing left to take down.
	third := canvas.NewBuffer(40, 10)
	m.sweepMark(third)
	if n := len(third.Graphics()); n != 0 {
		t.Errorf("the delete was sent again on a later frame (%d sequences)", n)
	}
}

// So nothing moves when a terminal turns out to be capable.
func TestThePictureTakesExactlyTheBrailleMarksRoom(t *testing.T) {
	if MarkH != len(SmallLogo) {
		t.Errorf("MarkH is %d and SmallLogo is %d rows", MarkH, len(SmallLogo))
	}
	if MarkW != len([]rune(SmallLogo[0])) {
		t.Errorf("MarkW is %d and SmallLogo is %d cells wide", MarkW, len([]rune(SmallLogo[0])))
	}
	seq := markPlaces()[0]
	for _, want := range []string{"c=20", "r=5", "C=1"} {
		if !strings.Contains(seq, want) {
			t.Errorf("the placement %q does not carry %s", seq, want)
		}
	}
}

// The buffer counts a column per rune, so an escape that moved the cursor would
// shift the rest of the row. C=1 is the protocol's option for this.
func TestTheFrameStillMeasuresRightWithAPictureInIt(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	m.UseGraphics(GraphicsKitty)

	buf := canvas.NewBuffer(40, 3)
	buf.Text(0, 0, strings.Repeat("x", 40), m.Theme.Chrome)
	m.drawMark(buf, 5, 0, m.Theme.Chrome)

	// Whatever escapes it has, the row is still forty cells of text.
	row := buf.Row(0)
	if len([]rune(row)) != 40 {
		t.Errorf("the row is %d runes, want 40", len([]rune(row)))
	}
	if !strings.Contains(markPlaces()[0], "C=1") {
		t.Error("the placement moves the cursor")
	}
}

// kitty cannot rotate a placement, so PERPETUITY keeps reading SmallLogo as a
// bitmap however capable the terminal is.
func TestPerpetuityStillSpinsTheBrailleMark(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	m.UseGraphics(GraphicsKitty)

	dots, _, _ := markDots()
	if len(dots) == 0 {
		t.Fatal("the mark has no dots to spin")
	}

	panel := panelOf(100, 30)
	p := NewPerpetuity(m, panel, rand.New(rand.NewSource(1)))
	for i := 0; i < 30; i++ {
		p.Step(1.0 / 60.0)
	}

	buf := canvas.NewBuffer(100, 30)
	p.Draw(buf, m.Theme, panel)

	if n := len(buf.Graphics()); n != 0 {
		t.Errorf("the diversion placed %d pictures; it has to stay dots, because "+
			"it spins and kitty cannot rotate a placement", n)
	}
	painted := 0
	for y := 0; y < 30; y++ {
		for _, r := range buf.Row(y) {
			if r >= 0x2800 && r <= 0x28FF && r != canvas.BrailleBlank {
				painted++
			}
		}
	}
	if painted == 0 {
		t.Error("the spinning mark drew nothing")
	}
}

// WezTerm announces the kitty protocol, has it enabled, and draws nothing with
// transmit-then-place-by-id. It must be matched before the kitty rules, since it
// answers those too.
func TestEachTerminalGetsTheProtocolItActuallyDraws(t *testing.T) {
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}

	cases := []struct {
		term string
		vars map[string]string
		want Graphics
	}{
		{"xterm-kitty", nil, GraphicsKitty},
		{"XTERM-KITTY", nil, GraphicsKitty},
		{"xterm-ghostty", nil, GraphicsKitty},
		{"xterm-256color", map[string]string{"KITTY_WINDOW_ID": "3"}, GraphicsKitty},
		{"xterm-256color", map[string]string{"TERM_PROGRAM": "ghostty"}, GraphicsKitty},

		// WezTerm: kitty-capable by its own account and drawing nothing.
		{"xterm-256color", map[string]string{"TERM_PROGRAM": "WezTerm"}, GraphicsITerm},
		{"wezterm", nil, GraphicsITerm},
		{"xterm-256color", map[string]string{"TERM_PROGRAM": "iTerm.app"}, GraphicsITerm},

		{"xterm-256color", nil, GraphicsOff},
		{"screen", nil, GraphicsOff},
		{"", nil, GraphicsOff},
		{"dumb", nil, GraphicsOff},
		{"xterm-256color", map[string]string{"TERM_PROGRAM": "Apple_Terminal"}, GraphicsOff},
	}
	for _, c := range cases {
		if got := DetectGraphics(c.term, env(c.vars)); got != c.want {
			t.Errorf("DetectGraphics(%q, %v) = %v, want %v", c.term, c.vars, got, c.want)
		}
	}
}

// Nothing comes back from a terminal that ignored an image, and asking stalls a
// connect for ten seconds on a terminal that does not reply. So the fallback is
// a flag, and an unknown name is an error, not a silent "auto".
func TestTheGraphicsFlagForcesAProtocol(t *testing.T) {
	cases := []struct {
		in     string
		want   Graphics
		forced bool
		ok     bool
	}{
		{"auto", GraphicsOff, false, true},
		{"", GraphicsOff, false, true},
		{"kitty", GraphicsKitty, true, true},
		{"iterm", GraphicsITerm, true, true},
		{"iTerm2", GraphicsITerm, true, true},
		{"off", GraphicsOff, true, true},
		{"braille", GraphicsOff, true, true},
		{"sixel", GraphicsOff, false, false},
		{"yes", GraphicsOff, false, false},
	}
	for _, c := range cases {
		g, forced, ok := ParseGraphics(c.in)
		if g != c.want || forced != c.forced || ok != c.ok {
			t.Errorf("ParseGraphics(%q) = (%v, %v, %v), want (%v, %v, %v)",
				c.in, g, forced, ok, c.want, c.forced, c.ok)
		}
	}
}

// The inline picture is the cell content, so its cells are stepped over and
// there is nothing to take down afterwards.
func TestTheInlineMarkOwnsItsCells(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	m.UseGraphics(GraphicsITerm)

	buf := canvas.NewBuffer(60, 20)
	buf.Text(0, 0, strings.Repeat("#", 60), m.Theme.Chrome)
	m.drawMark(buf, 4, 0, m.Theme.Chrome)
	m.sweepMark(buf)

	gfx := buf.Graphics()
	if len(gfx) == 0 {
		t.Fatal("nothing was drawn")
	}
	if !strings.Contains(gfx[0], "\x1b]1337;File=") {
		t.Error("the sequence is not an iTerm2 inline image")
	}
	// DECSC and DECRC, because this protocol has no C=1: the image is cell content
	// and drawing it advances the cursor.
	if !strings.HasPrefix(gfx[0], "\x1b7") || !strings.HasSuffix(gfx[0], "\x1b8") {
		t.Error("the image does not save and restore the cursor around itself")
	}

	// The owned cells are stepped over, so the row is short by exactly the mark's
	// width and the rest is intact.
	out := strings.Split(buf.StringWith(nil), "\n")[0]
	if !strings.Contains(out, "\x1b[20C") {
		t.Errorf("the mark's cells were painted rather than stepped over: %q", out[:min(80, len(out))])
	}

	// Nothing is left to delete: the next frame's runes in those cells erase it.
	next := canvas.NewBuffer(60, 20)
	m.sweepMark(next)
	if n := len(next.Graphics()); n != 0 {
		t.Errorf("the inline mark asked for %d deletes; it needs none", n)
	}
}

// The inline picture is sent whole every time it is drawn, so a palette keeps
// its size affordable.
func TestTheInlinePictureIsSmallEnoughToRepeat(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	m.UseGraphics(GraphicsITerm)

	seq := m.markInlineSeq()
	if seq == "" {
		t.Fatal("no inline sequence was built")
	}
	// Full colour it is 26KB. BenchmarkRealFrame puts a frame at about 8.5KB, and
	// this has to fit alongside one.
	if n := len(seq); n > 16*1024 {
		t.Errorf("the inline picture is %d bytes, which is too much to repeat", n)
	}

	// Built once per theme, not once per draw.
	if again := m.markInlineSeq(); again != seq {
		t.Error("the sequence was rebuilt rather than reused")
	}
}

// The about box overlays the file bar and both draw the mark. With one
// placement id the second draw would move the first, and the header would lose
// its globe while the box was open.
func TestTwoMarksInOneFrameAreTwoPlacements(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	m.UseGraphics(GraphicsKitty)

	buf := canvas.NewBuffer(60, 20)
	m.drawMark(buf, 0, 0, m.Theme.Chrome)
	m.drawMark(buf, 30, 10, m.Theme.Chrome)
	m.sweepMark(buf)

	ids := map[string]bool{}
	for _, seq := range buf.Graphics() {
		if len(seq) > 1024 {
			continue // the transmit
		}
		for _, p := range []string{"p=1", "p=2", "p=3"} {
			if strings.Contains(seq, p) {
				ids[p] = true
			}
		}
	}
	if len(ids) != 2 {
		t.Errorf("two marks used placement ids %v, want two distinct ones", ids)
	}

	// The frame after, showing one mark, takes the other down.
	next := canvas.NewBuffer(60, 20)
	m.drawMark(next, 0, 0, m.Theme.Chrome)
	m.sweepMark(next)

	deletes := 0
	for _, seq := range next.Graphics() {
		if strings.Contains(seq, "a=d") {
			deletes++
			if !strings.Contains(seq, "p=2") {
				t.Errorf("%q takes down the wrong placement", seq)
			}
		}
	}
	if deletes != 1 {
		t.Errorf("closing one of two marks sent %d deletes, want one", deletes)
	}
}

func TestMoreMarksThanPlacementsFallBackToDots(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	m.UseGraphics(GraphicsKitty)

	buf := canvas.NewBuffer(60, 40)
	for i := 0; i < markPlacements+1; i++ {
		m.drawMark(buf, 0, i*MarkH, m.Theme.Chrome)
	}
	m.sweepMark(buf)

	dots := 0
	for y := 0; y < 40; y++ {
		for _, r := range buf.Row(y) {
			if r >= 0x2800 && r <= 0x28FF && r != canvas.BrailleBlank {
				dots++
			}
		}
	}
	if dots == 0 {
		t.Error("the mark past the last placement id drew nothing at all")
	}
}

// The boot log shows the mark as five braille lines. The rows are left blank
// and the picture placed over them, only while all five are on screen and
// revealed.
func TestTheBootLogShowsThePictureAndThenScrollsItAway(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	m.UseGraphics(GraphicsKitty)
	m.StartIntro()

	if m.Intro.MarkAt() < 0 {
		t.Fatal("the boot log has no mark in it")
	}

	placed, dotted := false, false
	for i := 0; i < 60*30 && m.Screen == ScreenIntro; i++ {
		tick(m, 1)
		if m.Intro == nil || m.Intro.Stage() != stageBoot {
			continue
		}
		buf := drawTo(m)

		pictures := 0
		for _, seq := range buf.Graphics() {
			if strings.Contains(seq, "a=p") {
				pictures++
			}
		}
		if pictures > 0 {
			placed = true
		}

		// The dots and the picture are alternatives, never both, or the mark would be
		// drawn twice over itself.
		dots := 0
		_, h := buf.Size()
		for y := 0; y < h; y++ {
			for _, r := range buf.Row(y) {
				if r >= 0x2800 && r <= 0x28FF && r != canvas.BrailleBlank {
					dots++
				}
			}
		}
		if dots > 0 {
			dotted = true
		}
		if pictures > 0 && dots > 0 {
			t.Fatalf("frame %d drew the mark as a picture and as %d dots at once", i, dots)
		}
	}

	if !placed {
		t.Error("the boot log never showed the mark as a picture")
	}
	if !dotted {
		t.Error("the mark never fell back to dots as it scrolled off the top")
	}
}

// markIDs is which placement ids a frame put up and which it took down.
func markIDs(buf *canvas.Buffer) (up, down map[int]bool) {
	up, down = map[int]bool{}, map[int]bool{}
	for _, seq := range buf.Graphics() {
		id := 0
		for p := 1; p <= markPlacements; p++ {
			if strings.Contains(seq, "p="+string(rune('0'+p))) {
				id = p
			}
		}
		if id == 0 {
			continue
		}
		switch {
		case strings.Contains(seq, "a=d"):
			down[id] = true
		case strings.Contains(seq, "a=p"):
			up[id] = true
		}
	}
	return up, down
}

// kitty keeps placements in their own layer and Bubble Tea's erases do not
// touch them, so a mark that stops being drawn stays over whatever comes next.
func TestTheIntroLeavesNoPictureBehind(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	m.UseGraphics(GraphicsKitty)
	m.StartIntro()

	live := map[int]bool{}
	for i := 0; i < 60*30; i++ {
		tick(m, 1)
		up, down := markIDs(drawTo(m))

		for id := range live {
			if !up[id] && !down[id] {
				t.Fatalf("frame %d left placement %d up without drawing it", i, id)
			}
		}
		for id := range down {
			delete(live, id)
		}
		for id := range up {
			live[id] = true
		}
		if len(live) > markPlacements {
			t.Fatalf("frame %d has %d pictures live, more than the %d ids", i, len(live), markPlacements)
		}
		if m.Screen != ScreenIntro && i > 60 {
			break
		}
	}
}

// Bubble Tea skips a row whose text has not changed, so a delete on row zero is
// dropped whenever row zero is unchanged, which is most frames. The rows the
// mark just left are certainly changing.
func TestTheDeleteRidesOnTheRowsThePictureIsLeaving(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	m.UseGraphics(GraphicsKitty)

	const markX, markY = 7, 12
	first := canvas.NewBuffer(60, 30)
	m.drawMark(first, markX, markY, m.Theme.Chrome)
	m.sweepMark(first)

	next := canvas.NewBuffer(60, 30)
	m.sweepMark(next)

	rows := next.GraphicRows()
	if len(rows) != 1 {
		t.Fatalf("%d sequences on the frame that dropped the mark, want one delete", len(rows))
	}
	if rows[0][0] != markX || rows[0][1] != markY {
		t.Errorf("the delete was sent from (%d, %d), want the mark's own cell (%d, %d)",
			rows[0][0], rows[0][1], markX, markY)
	}
}

// The picture is drawn over the file bar and the about box's frame, so any
// background alpha is a slab of theme colour around the mark. The resize filter
// undershoots at a hard edge, so normalising carelessly lifts black to a
// visible alpha. Checked on the asset on disk, since nothing else checks it.
func TestTheMarkIsTransparentWhereItIsNothing(t *testing.T) {
	shape := markShape()
	if shape == nil {
		t.Fatal("the mask did not decode")
	}

	b := shape.Bounds()
	clear, lit, floor := 0, 0, 255
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			a := int(shape.AlphaAt(x, y).A)
			switch {
			case a == 0:
				clear++
			case a == 0xFF:
				lit++
			}
			if a < floor {
				floor = a
			}
		}
	}

	if floor != 0 {
		t.Errorf("the mask's floor is alpha %d, so every pixel of it paints: "+
			"the whole box comes out as a slab of the theme colour", floor)
	}
	// A mark on a background is mostly background; well under half clear means a
	// wash.
	if total := b.Dx() * b.Dy(); clear*2 < total {
		t.Errorf("only %d of %d pixels are fully transparent", clear, total)
	}
	if lit == 0 {
		t.Error("no pixel is fully opaque, so the mark can only ever be a ghost")
	}
}

// A multiplexer paints its own model of the screen into another pane, and image
// protocols are not part of it. It passes the outer terminal's announcement
// through, so a pane in herdr reports TERM_PROGRAM=WezTerm. Guessing wrong
// prints escape bytes as text across a shared screen, so the answer is braille.
func TestAMultiplexerGetsTheBrailleMark(t *testing.T) {
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}

	// Each of these is a capable terminal by its own account, so the multiplexer
	// check must come first.
	cases := []struct {
		what string
		term string
		vars map[string]string
	}{
		{"herdr in WezTerm", "xterm-256color", map[string]string{
			"HERDR_ENV": "1", "HERDR_PANE_ID": "w3:p1", "TERM_PROGRAM": "WezTerm"}},
		{"herdr in kitty", "xterm-kitty", map[string]string{"HERDR_ENV": "1"}},
		{"tmux by env", "xterm-kitty", map[string]string{"TMUX": "/tmp/tmux-501/default,1,0"}},
		{"tmux by TERM", "tmux-256color", map[string]string{"KITTY_WINDOW_ID": "3"}},
		{"screen by TERM", "screen-256color", map[string]string{"TERM_PROGRAM": "ghostty"}},
		{"screen by env", "xterm-ghostty", map[string]string{"STY": "1234.pts-0.host"}},
		{"zellij", "xterm-256color", map[string]string{
			"ZELLIJ": "0", "TERM_PROGRAM": "WezTerm"}},
	}

	for _, c := range cases {
		if !Multiplexed(c.term, env(c.vars)) {
			t.Errorf("%s was not recognised as a multiplexer", c.what)
		}
		if got := DetectGraphics(c.term, env(c.vars)); got != GraphicsOff {
			t.Errorf("%s was sent %v; a multiplexer gets dots", c.what, got)
		}
	}

	// A bare terminal is not a multiplexer.
	for _, c := range []struct {
		term string
		vars map[string]string
	}{
		{"xterm-kitty", nil},
		{"xterm-256color", map[string]string{"TERM_PROGRAM": "WezTerm"}},
		{"xterm-ghostty", map[string]string{"KITTY_WINDOW_ID": "1"}},
	} {
		if Multiplexed(c.term, env(c.vars)) {
			t.Errorf("%q %v was called a multiplexer", c.term, c.vars)
		}
	}
}

// For somebody who has set up their own passthrough; detection cannot know it
// works.
func TestTheOverrideStillWorksInsideAMultiplexer(t *testing.T) {
	g, forced, ok := ParseGraphics("kitty")
	if !ok || !forced || g != GraphicsKitty {
		t.Fatalf("ParseGraphics(kitty) = (%v, %v, %v)", g, forced, ok)
	}

	// ParseGraphics never reads the environment, so detection cannot overrule a
	// forced mode.
	m := NewModel(1)
	m.Resize(120, 40)
	m.UseGraphics(g)
	if _, gfx := markBox(m); len(gfx) == 0 {
		t.Error("a forced kitty mode drew no picture")
	}
}
