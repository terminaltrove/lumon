package lumon

import (
	"strings"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// The Lumon mark, drawn as a picture where the terminal can show one. At
// twenty cells by five the braille SmallLogo reads as a globe, but its
// wordmark is about 2.4 braille dots tall and cannot be read; a graphics
// protocol draws it in pixels in the same box. Nothing above drawMark knows
// which it got.
//
// PERPETUITY spins the mark and kitty cannot rotate a placement, so it keeps
// braille and markDots still reads SmallLogo as a bitmap.

// MarkW and MarkH are the mark's box, in cells. They are read off SmallLogo, so
// the picture takes exactly the room the braille did.
var (
	MarkW = len([]rune(SmallLogo[0]))
	MarkH = len(SmallLogo)
)

// The kitty ids the mark claims. They are fixed: an id that changed between
// frames would leave the terminal holding every image the session ever sent.
// The value is arbitrary and only has to avoid other programs' ids.
const (
	markImageID = 7311

	// markPlacements is how many copies of the mark one frame may show. The
	// about box overlays the file bar and both draw the mark; a placement id
	// drawn twice moves the one placement, so each copy needs its own id. A
	// site past the limit falls back to braille.
	markPlacements = 4

	// markLevels is how many steps of the tint the encoded picture has. See
	// markImage: this is a running cost on the inline protocol.
	markLevels = 32
)

// Graphics is how a terminal is asked to draw a picture, if at all. There are
// two terminal protocols because WezTerm advertises kitty graphics but draws
// nothing for a transmit-then-place-by-id; its iTerm2 support works.
type Graphics int

const (
	// GraphicsOff is the braille mark, and the zero value, so a Model nobody
	// has configured draws dots, as the tests expect.
	GraphicsOff Graphics = iota

	// GraphicsKitty transmits once and places by id. Cheap in the steady state;
	// correct in kitty and Ghostty.
	GraphicsKitty

	// GraphicsITerm sends the picture inline every time it is drawn. Costlier;
	// it is what WezTerm and iTerm2 draw.
	GraphicsITerm

	// GraphicsBrowser exports cached pixel placements to the browser compositor.
	GraphicsBrowser
)

// ParseGraphics reads the --graphics flag. "auto" is reported as ok with a false
// second result, since what auto means is "ask the terminal", not a protocol.
func ParseGraphics(name string) (g Graphics, forced, ok bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "auto":
		return GraphicsOff, false, true
	case "kitty":
		return GraphicsKitty, true, true
	case "iterm", "iterm2":
		return GraphicsITerm, true, true
	case "off", "none", "braille":
		return GraphicsOff, true, true
	}
	return GraphicsOff, false, false
}

// DetectGraphics picks a protocol from what a terminal announces about itself.
//
// It never queries the terminal (as with rendererFor): a terminal that does not
// answer turns the query into a ten second stall on connect. A terminal that
// ignores an image reports nothing either, so the choice must be right by name;
// --graphics is the manual override. Only TERM survives a remote connection, so remote sessions
// mostly get braille.
func DetectGraphics(term string, env func(string) string) Graphics {
	term = strings.ToLower(term)
	get := func(k string) string {
		if env == nil {
			return ""
		}
		return env(k)
	}
	prog := strings.ToLower(get("TERM_PROGRAM"))

	// A multiplexer answers for the terminal it is inside, and answers wrongly.
	if Multiplexed(term, env) {
		return GraphicsOff
	}

	switch {
	// WezTerm before kitty, because it announces itself as kitty-capable.
	case strings.Contains(term, "wezterm"), prog == "wezterm":
		return GraphicsITerm
	case strings.Contains(term, "kitty"), get("KITTY_WINDOW_ID") != "":
		return GraphicsKitty
	case strings.Contains(term, "ghostty"), prog == "ghostty":
		return GraphicsKitty
	case prog == "iterm.app", prog == "mintty", strings.Contains(term, "mintty"):
		return GraphicsITerm
	}
	return GraphicsOff
}

// Multiplexed reports whether something is sitting between this program and the
// terminal that would have to draw the picture.
//
// A multiplexer keeps its own model of the screen, which has no images unless
// it implements a protocol, so a picture is dropped or lands in the wrong pane.
// Multiplexers pass TERM and TERM_PROGRAM through (herdr reports
// TERM_PROGRAM=WezTerm), so this must be checked before the terminal, using
// the variables the multiplexer sets for itself. A wrong guess prints escape
// bytes as text across a shared screen, so this answers off; `--graphics
// kitty` covers a tmux with passthrough configured.
func Multiplexed(term string, env func(string) string) bool {
	term = strings.ToLower(term)
	if strings.HasPrefix(term, "screen") || strings.HasPrefix(term, "tmux") {
		return true
	}
	if env == nil {
		return false
	}
	for _, k := range []string{
		"HERDR_ENV", // herdr
		"HERDR_PANE_ID",
		"TMUX",       // tmux
		"STY",        // GNU screen
		"ZELLIJ",     // zellij
		"ABDUCO_CMD", // abduco
		"DVTM",       // dvtm
	} {
		if env(k) != "" {
			return true
		}
	}
	switch strings.ToLower(env("TERM_PROGRAM")) {
	case "tmux", "zellij", "herdr":
		return true
	}
	return false
}

// UseGraphics tells this session how its terminal can be asked for a picture.
func (m *Model) UseGraphics(g Graphics) {
	m.graphics = g
	m.markSent = false
}

// BrowserImages describes images in the most recently rendered frame.
func (m *Model) BrowserImages() []canvas.PixelImage {
	if m.frame == nil {
		return nil
	}
	return m.frame.PixelImages()
}

// MarkRefresh makes the next frame send the picture again. A terminal may drop
// its images when the screen changes, and Bubble Tea skips unchanged lines, so
// a mark lost that way never repairs itself; call this on resize.
func (m *Model) MarkRefresh() { m.markSent = false }

// drawMark draws the Lumon mark in a MarkW by MarkH box with its top left at
// (x, y), as a picture where this terminal can show one and as SmallLogo where
// it cannot.
func (m *Model) drawMark(buf *canvas.Buffer, x, y int, attr canvas.Attr) {
	switch {
	case m.graphics == GraphicsBrowser:
		color := attr.FG.String()
		if color == "" {
			color = m.Theme.Primary.String()
		}
		opacity := 1.0
		if attr.Faint {
			opacity = 0.5
		}
		buf.PlaceImage(canvas.PixelImage{CellRect: canvas.CellRect{X: x, Y: y, W: MarkW, H: MarkH}, Asset: "lumon-mark", Color: color, Opacity: opacity})
		return
	case m.graphics == GraphicsKitty && m.markDrawn < markPlacements:
		// The transmit first, so the placement after it in the same stream
		// has something to place. Graphic keeps them in the order added.
		if seq := m.markTransmit(); seq != "" {
			buf.Graphic(x, y, seq)
		}
		buf.Graphic(x, y, markPlaces()[m.markDrawn])
		m.markSpot[m.markDrawn] = [2]int{x, y}
		m.markDrawn++

		// The cells underneath stay blank: the picture floats above them, and
		// anything drawn under it is wasted bytes.
		return

	case m.graphics == GraphicsITerm:
		if seq := m.markInlineSeq(); seq != "" {
			buf.Graphic(x, y, seq)
			// The picture is the cell content, so reserve all its cells; a
			// blank written into one would rub it out. For the same reason
			// sweepMark only concerns kitty.
			buf.Reserve(x, y, MarkW, MarkH)
			return
		}
	}

	for i, row := range SmallLogo {
		// Skip blank braille cells, so the mark's corners do not rub out a
		// box's frame or the file bar's border underneath.
		for j, r := range []rune(row) {
			if r != canvas.BrailleBlank {
				buf.Set(x+j, y+i, r, attr)
			}
		}
	}
}

// sweepMark takes down every kitty copy of the picture this frame stopped
// drawing. Call sites cannot do it, because the sites that stop drawing the
// mark are the ones that stop running (leaving the intro, closing the about
// box, a resize). An inline picture needs no sweep: the next frame overwrites
// its cells.
func (m *Model) sweepMark(buf *canvas.Buffer) {
	if m.graphics == GraphicsKitty {
		for p := m.markDrawn; p < m.markUp; p++ {
			// At the cell the picture was at. Bubble Tea skips unchanged rows,
			// so a delete parked on an unchanged row would be dropped; the rows
			// the mark just left are certain to change.
			at := m.markSpot[p]
			buf.Graphic(at[0], at[1], markDeletes()[p])
		}
	}
	m.markUp, m.markDrawn = m.markDrawn, 0
}
