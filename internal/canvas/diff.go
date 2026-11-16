package canvas

import (
	"slices"
	"strconv"
	"strings"
)

// FrameDiffer emits only changed terminal rows. Buffer.StringWith gives every
// row independent SGR state, so one row can be replaced without its neighbours.
// RenderBuffer compares cells; Render compares the rows of a flattened frame.
type FrameDiffer struct {
	previous []string

	runes []rune
	attrs []Attr
	rows  []string
	w, h  int
	out   strings.Builder
	row   strings.Builder
}

// Reset forces a complete repaint after a viewport change.
func (d *FrameDiffer) Reset() {
	d.previous, d.runes, d.attrs, d.rows, d.w, d.h = nil, nil, nil, nil, 0, 0
}

const firstFrame = "\x1b[?25l\x1b[?7l\x1b[H\x1b[2J"

// Render returns the escape sequences that turn the previous frame into frame.
func (d *FrameDiffer) Render(frame string) string {
	rows := strings.Split(frame, "\n")
	var out strings.Builder
	if d.previous == nil {
		// Hide the hardware cursor and disable wrapping: writing the bottom
		// right cell must not scroll the game into the terminal's history.
		out.WriteString(firstFrame)
	}
	for y := 0; y < max(len(rows), len(d.previous)); y++ {
		row := ""
		if y < len(rows) {
			row = rows[y]
		}
		if y < len(d.previous) && row == d.previous[y] {
			continue
		}
		writeRowHeader(&out, y)
		out.WriteString(row)
		out.WriteString("\x1b[0m\x1b[K")
	}
	d.previous = rows
	return out.String()
}

// RenderBuffer is Render for an unflattened buffer and gives the same bytes. A
// row whose cells changed is still skipped if its bytes did not, since cells
// can differ invisibly (the colour of a blank). Rows with a terminal graphic
// are always compared as bytes.
func (d *FrameDiffer) RenderBuffer(b *Buffer, st *Styler) string {
	d.out.Reset()
	full := d.w != b.w || d.h != b.h || d.runes == nil
	if full {
		d.out.WriteString(firstFrame)
		d.w, d.h = b.w, b.h
		d.runes = slices.Grow(d.runes[:0], len(b.runes))[:len(b.runes)]
		d.attrs = slices.Grow(d.attrs[:0], len(b.attrs))[:len(b.attrs)]
		d.rows = slices.Grow(d.rows[:0], b.h)[:b.h]
	}
	for y := 0; y < b.h; y++ {
		lo, hi := y*b.w, (y+1)*b.w
		if !full && !b.hasGraphic(y) &&
			slices.Equal(b.runes[lo:hi], d.runes[lo:hi]) && slices.Equal(b.attrs[lo:hi], d.attrs[lo:hi]) {
			continue
		}
		copy(d.runes[lo:hi], b.runes[lo:hi])
		copy(d.attrs[lo:hi], b.attrs[lo:hi])
		d.row.Reset()
		b.writeRow(&d.row, st, y)
		row := d.row.String()
		if !full && row == d.rows[y] {
			continue
		}
		d.rows[y] = row
		writeRowHeader(&d.out, y)
		d.out.WriteString(row)
		d.out.WriteString("\x1b[0m\x1b[K")
	}
	return d.out.String()
}

func writeRowHeader(out *strings.Builder, y int) {
	out.WriteString("\x1b[")
	out.WriteString(strconv.Itoa(y + 1))
	out.WriteString(";1H\x1b[0m")
}

func (b *Buffer) hasGraphic(y int) bool {
	for i := range b.graphics {
		if b.graphics[i].y == y {
			return true
		}
	}
	return false
}
