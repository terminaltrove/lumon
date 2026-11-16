package lumon

import "github.com/terminaltrove/lumon/internal/canvas"

// The controls, on screen. An overlay, so it opens from anywhere (grid, lobby,
// scoreboard) without each needing a way back, and the board stays visible.

// modal is a centred box with a title, and where its contents may be drawn.
// Every overlay (controls, diversions, about, exit, phrases) uses it.
type modal struct {
	X, Y, W, H int
}

// modalBox lays a modal out and paints its frame, returning where the contents
// go. It returns ok=false, having drawn nothing, if there is no room to be
// legible.
func (m *Model) modalBox(buf *canvas.Buffer, wantW, wantH int, title string) (modal, bool) {
	w, h := buf.Size()

	if wantW > w-2 {
		wantW = w - 2
	}
	if wantH > h-2 {
		wantH = h - 2
	}
	if wantW < modalMinW || wantH < modalMinH {
		return modal{}, false
	}

	box := modal{X: (w - wantW) / 2, Y: (h - wantH) / 2, W: wantW, H: wantH}

	// Clear with Surface, which has a background; Footer has none and would
	// leave the grid showing through.
	for row := box.Y; row < box.Y+box.H; row++ {
		for col := box.X; col < box.X+box.W; col++ {
			buf.Set(col, row, ' ', m.Theme.Surface)
		}
	}
	buf.Box(box.X, box.Y, box.W, box.H, m.Theme.Chrome)
	if title != "" {
		m.modalCentre(buf, box, box.Y, title, m.Theme.Header)
	}
	return box, true
}

// modalMinW and modalMinH are the smallest box modalBox will draw.
const (
	modalMinW = 24
	modalMinH = 8
)

// modalCentre writes one line centred across the box, clipped to it.
func (m *Model) modalCentre(buf *canvas.Buffer, box modal, y int, s string, a canvas.Attr) {
	s = clipTo(s, box.W-2)
	buf.Text(box.X+(box.W-len([]rune(s)))/2, y, s, a)
}

// drawHelp paints the controls over whatever is already on the buffer.
func (m *Model) drawHelp(buf *canvas.Buffer) {
	w, h := buf.Size()

	groups := m.bindings()

	// The widest key decides where descriptions start, across every group.
	keyW, textW, rows := 0, 0, 0
	for _, g := range groups {
		rows += len(g.Bindings) + 2 // the title and a blank line
		for _, b := range g.Bindings {
			if n := len([]rune(b.Keys)); n > keyW {
				keyW = n
			}
			if n := len([]rune(b.What)); n > textW {
				textW = n
			}
		}
	}
	rows++ // the closing hint

	box, ok := m.modalBox(buf, keyW+textW+7, rows+3, "LUMON MDR HELP")
	if !ok {
		// Nothing legible fits; show just the hint.
		if w >= 12 && h >= 1 {
			buf.Text(0, h/2, clipTo("? controls", w), m.Theme.BinOn)
		}
		return
	}
	x, y, boxW, boxH := box.X, box.Y, box.W, box.H

	cy := y + 2
	for _, g := range groups {
		if cy >= y+boxH-2 {
			break
		}
		buf.Text(x+2, cy, clipTo(g.Title, boxW-4), m.Theme.BinOn)
		cy++
		for _, b := range g.Bindings {
			if cy >= y+boxH-2 {
				break
			}
			buf.Text(x+3, cy, clipTo(b.Keys, boxW-5), m.Theme.BadLive)
			if tx := x + 3 + keyW + 2; tx < x+boxW-2 {
				buf.Text(tx, cy, clipTo(b.What, x+boxW-2-tx), m.Theme.Footer)
			}
			cy++
		}
		cy++
	}

	const close = "? or any key to go back"
	if cy < y+boxH-1 {
		m.modalCentre(buf, box, y+boxH-2, close, m.Theme.Footer)
	}
}

// clipTo trims a label to w runes. The chrome is full of multi-byte box-drawing
// characters and arrows.
func clipTo(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	return string(r[:w])
}
