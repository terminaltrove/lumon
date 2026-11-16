package lumon

import (
	"github.com/terminaltrove/lumon/internal/canvas"
	"github.com/terminaltrove/lumon/internal/theme"
)

// The about box is an overlay, like the controls: it opens from any screen and
// leaves the board visible around it. Its layout matches moltbook-tui, the other
// Terminal Trove program: the mark, what this is and who made it, where to find
// it, what it is built on, and how to get out.

// aboutRole is what one credit line is, which decides its colour.
type aboutRole int

const (
	aboutTitle   aboutRole = iota // what this is
	aboutCredit                   // who made it, and their address
	aboutVersion                  // which one this is
	aboutBody                     // what it does
	aboutFaint                    // the disclaimer, and the way out
	aboutBlank
)

type aboutLine struct {
	text string
	role aboutRole

	// shed is the order lines are dropped in when the box cannot hold them
	// all: 2 goes first, then 1; 0 stays. Cutting from the bottom would lose
	// the github address and keep the flavour text.
	shed int
}

// aboutText is the credit, in reading order. The mark above it is SmallLogo,
// the same one the intro and the file bar draw.
func aboutText() []aboutLine {
	return []aboutLine{
		{"Lumon Industries MDR", aboutTitle, 0},
		{"a Terminal Trove re-creation", aboutCredit, 0},
		{"https://terminaltrove.com/lumon/", aboutCredit, 0},
		{"v" + Version, aboutVersion, 0},
		{"", aboutBlank, 2},
		{"Refine the macrodata. Sort the numbers", aboutBody, 2},
		{"that feel wrong into the bins.", aboutBody, 2},
		{"The work is mysterious and important.", aboutBody, 2},
		{"", aboutBlank, 1},
		{"Built with:", aboutCredit, 1},
		{"Bubble Tea", aboutBody, 1},
		{"", aboutBlank, 0},
		{"https://github.com/terminaltrove/lumon", aboutCredit, 0},
	}
}

// aboutFit drops what a box this tall cannot hold, cheapest first.
func aboutFit(lines []aboutLine, rows int) []aboutLine {
	for rank := 1; rank <= 2 && len(lines) > rows; rank++ {
		kept := make([]aboutLine, 0, len(lines))
		for _, l := range lines {
			if l.shed != rank {
				kept = append(kept, l)
			}
		}
		lines = kept
	}
	if len(lines) > rows && rows >= 0 {
		lines = lines[:rows]
	}
	return lines
}

// attr is the colour for a role in this session's theme.
func (r aboutRole) attr(t *theme.Theme) canvas.Attr {
	switch r {
	case aboutTitle:
		return t.Header
	case aboutCredit:
		return t.BinOn
	case aboutBody:
		return t.Hint
	}
	return t.Footer // aboutVersion, aboutFaint and anything unnamed
}

// drawAbout paints the credits over whatever is already on the buffer.
func (m *Model) drawAbout(buf *canvas.Buffer) {
	lines := aboutText()

	logoW, logoH := 0, len(SmallLogo)
	for _, row := range SmallLogo {
		if n := len([]rune(row)); n > logoW {
			logoW = n
		}
	}

	widest := logoW
	for _, l := range lines {
		if n := len([]rune(l.text)); n > widest {
			widest = n
		}
	}

	w, h := buf.Size()
	box, ok := m.modalBox(buf, widest+8, logoH+len(lines)+4, "")
	if !ok {
		// Nothing legible fits, so show just the address.
		if w >= 12 && h >= 1 {
			buf.Text(0, h/2, clipTo("terminaltrove.com/lumon", w), m.Theme.BinOn)
		}
		return
	}

	y := box.Y + 1

	// On a short terminal the logo gives way before any of the words.
	if y+logoH+len(lines)+2 <= box.Y+box.H {
		m.drawMark(buf, box.X+(box.W-logoW)/2, y, m.Theme.Chrome)
		y += logoH + 1
	}

	// Then the lines that fit, dropped cheapest first.
	for _, l := range aboutFit(lines, box.Y+box.H-2-y) {
		if l.role != aboutBlank {
			m.modalCentre(buf, box, y, l.text, l.role.attr(m.Theme))
		}
		y++
	}

	m.modalCentre(buf, box, box.Y+box.H-2, "a or any key to go back", m.Theme.Footer)
}
