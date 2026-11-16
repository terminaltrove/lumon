package lumon

import "github.com/terminaltrove/lumon/internal/canvas"

// The minimap: the board seen from above, showing who else is on it, where they
// are looking, and which corrupt clusters anybody has found.
//
// The 100x100 board is drawn into 10x6 characters, which braille makes 20x24
// dots, so a dot is about five cells. It goes in the margin left of the bins; on
// a terminal too narrow for that margin it is not drawn.

const (
	// miniW and miniH are the map's size in characters, and miniMin the
	// narrowest margin it will draw in. 10x6 fits the margin at 120 columns.
	miniW   = 10
	miniH   = 6
	miniMin = 8

	// miniPad is the gap between the map and the first bin, leaving room for
	// bin one's lid to swing.
	miniPad = 2
)

// miniBox is where the map goes, in characters.
type miniBox struct{ X, Y, W, H int }

// minimapFits measures the margin beside the bins and refuses it if it is too
// small. It measures from Layout because the bins lose their inset on a narrow
// terminal (below five bins of binMinWidth), leaving no margin.
func minimapFits(l *Layout) (miniBox, bool) {
	if l == nil {
		return miniBox{}, false
	}
	margin := l.BinOriginX(0) - miniPad
	if margin < miniMin {
		return miniBox{}, false
	}

	w := miniW
	if margin < w {
		w = margin
	}
	h := miniH
	if h > BinsHeight {
		h = BinsHeight
	}
	// Against the left wall, aligned with the footer's hint at column one.
	return miniBox{X: 1, Y: l.BinTop, W: w, H: h}, true
}

// drawMinimap paints the board's overview into the margin. It draws only when
// at least two refiners are seated.
func (m *Model) drawMinimap(buf *canvas.Buffer) {
	if m.Room == nil || m.Seated() < 2 {
		return
	}
	box, ok := minimapFits(m.Layout)
	if !ok {
		return
	}

	// One layer per colour, since a braille layer is blitted with one
	// attribute. The layers are kept on the Model and cleared, because this runs
	// every frame.
	m.miniLayers = ensureLayers(m.miniLayers, box.W, box.H)

	dim, mine := m.miniLayers[0], m.miniLayers[1]

	// Corners only: six rows is too few to spend two on a border.
	miniCorners(dim)

	// Only clusters somebody has found. Showing every awake cluster would give
	// the game away.
	for _, grp := range m.Actives() {
		if !m.WasFound(grp.ID) {
			continue
		}
		c := centroidOf(grp.NumberIDs)
		miniDot(dim, int(c.X), int(c.Y))
	}

	miniRect(mine, m.Layout.ViewportX, m.Layout.ViewportY,
		m.Layout.ViewportW, m.Layout.ViewportH)

	buf.BlitBrailleOver(box.X, box.Y, dim, m.Theme.Number)
	buf.BlitBrailleOver(box.X, box.Y, mine, m.Theme.Chrome)

	// Other refiners last, so they are drawn on top.
	for _, d := range m.Roster() {
		if d == m.Desk {
			continue
		}
		l := m.miniLayers[2+d.Seat%MaxDesks]
		miniRect(l, d.ViewX, d.ViewY, d.ViewW, d.ViewH)
		miniBlob(l, d.CursorX, d.CursorY)
		buf.BlitBrailleOver(box.X, box.Y, l, m.Theme.On(m.deskColour(d), false))
	}
}

// ensureLayers returns the map's braille layers at the given size, reusing them
// when it can.
func ensureLayers(in []*canvas.Braille, w, h int) []*canvas.Braille {
	want := 2 + MaxDesks
	if len(in) != want {
		in = make([]*canvas.Braille, want)
	}
	for i := range in {
		if in[i] == nil {
			in[i] = canvas.NewBraille(w, h)
			continue
		}
		if cw, ch := in[i].CellSize(); cw != w || ch != h {
			in[i] = canvas.NewBraille(w, h)
			continue
		}
		in[i].Clear()
	}
	return in
}

// miniAt maps a grid cell to a dot in the map.
func miniAt(b *canvas.Braille, gx, gy int) (int, int) {
	w, h := b.DotSize()
	x := gx * w / GridSize
	y := gy * h / GridSize
	return clampInt(x, 0, w-1), clampInt(y, 0, h-1)
}

// miniDot lights one dot.
func miniDot(b *canvas.Braille, gx, gy int) {
	x, y := miniAt(b, gx, gy)
	b.Set(x, y)
}

// miniBlob lights a 2x2 block of dots, so a person stands out from a cluster.
func miniBlob(b *canvas.Braille, gx, gy int) {
	x, y := miniAt(b, gx, gy)
	for dy := 0; dy <= 1; dy++ {
		for dx := 0; dx <= 1; dx++ {
			b.Set(x+dx, y+dy)
		}
	}
}

// miniRect outlines a window of the board, hollow so the dots inside it stay
// readable.
func miniRect(b *canvas.Braille, gx, gy, gw, gh int) {
	if gw <= 0 || gh <= 0 {
		return
	}
	x0, y0 := miniAt(b, gx, gy)
	x1, y1 := miniAt(b, gx+gw-1, gy+gh-1)
	for x := x0; x <= x1; x++ {
		b.Set(x, y0)
		b.Set(x, y1)
	}
	for y := y0; y <= y1; y++ {
		b.Set(x0, y)
		b.Set(x1, y)
	}
}

// miniCorners marks the extent of the board.
func miniCorners(b *canvas.Braille) {
	w, h := b.DotSize()
	if w < 3 || h < 3 {
		return
	}
	for _, c := range [4][2]int{{0, 0}, {w - 1, 0}, {0, h - 1}, {w - 1, h - 1}} {
		b.Set(c[0], c[1])
	}
	// A tick along each edge from every corner, so they read as corners.
	b.Set(1, 0)
	b.Set(0, 1)
	b.Set(w-2, 0)
	b.Set(w-1, 1)
	b.Set(1, h-1)
	b.Set(0, h-2)
	b.Set(w-2, h-1)
	b.Set(w-1, h-2)
}
