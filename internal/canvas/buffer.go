package canvas

import (
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// Attr describes the styling of a single cell. It is a small comparable value
// with no pointers: it keys the style caches, StringWith compares it once per
// cell to find runs, and clearing a frame of them needs no write barriers. Keep
// it that way (see Color).
type Attr struct {
	FG    Color
	BG    Color
	Bold  bool
	Faint bool

	// Over draws a line along the cell's top in the foreground colour (SGR
	// 53), where the terminal can. It goes only with a colour, which is what
	// tells a terminal that strips styling from one that does not.
	Over bool
}

// defaultAttr is the zero value: an unstyled cell, rendered with no escape
// sequences.
var defaultAttr = Attr{}

// Buffer is a fixed-size grid of runes and their attributes. Every region and
// overlay draws into one shared Buffer, so a digit can fly across regions
// without any of them knowing about the others.
type Buffer struct {
	w, h  int
	runes []rune
	attrs []Attr

	// lastLen is the previous flatten's length, used to size the next one.
	lastLen int

	// graphics are escape sequences to emit at a cell. See Graphic.
	graphics    []graphic
	pixelImages []PixelImage
	pixelOwners []int
}

// graphic is one escape sequence, the cell it is emitted at, and how many
// cells the terminal owns from there. See Graphic and Reserve.
type graphic struct {
	x, y int
	seq  string
	skip int
}

// Graphic puts an escape sequence into the frame at a cell, for terminal image
// protocols. The cells are still emitted, so callers leave the image's
// footprint blank and the line keeps its width for Bubble Tea.
//
// The sequence must not move the cursor: everything downstream assumes one cell
// per rune. The kitty protocol has C=1 for this.
func (b *Buffer) Graphic(x, y int, seq string) {
	if seq == "" || x < 0 || y < 0 || x >= b.w || y >= b.h {
		return
	}
	b.graphics = append(b.graphics, graphic{x: x, y: y, seq: seq})
}

// Reserve gives a rectangle of cells to the terminal: the buffer steps the
// cursor over them with CUF instead of painting them. The iTerm2 inline
// protocol makes the image the cell content, so a space written over it erases
// it (kitty keeps placements in their own layer and does not need this).
//
// A CUF is not printable, so the row measures short downstream. That is fine:
// Bubble Tea's erase to the right of a short line then starts at the row's end
// and erases nothing.
func (b *Buffer) Reserve(x, y, w, h int) {
	if w <= 0 || x < 0 || x >= b.w {
		return
	}
	if w > b.w-x {
		w = b.w - x
	}
	for i := 0; i < h; i++ {
		if y+i < 0 || y+i >= b.h {
			continue
		}
		b.graphics = append(b.graphics, graphic{x: x, y: y + i, skip: w})
	}
}

// Graphics returns the sequences placed this frame, for tests.
func (b *Buffer) Graphics() []string {
	out := make([]string, 0, len(b.graphics))
	for _, g := range b.graphics {
		out = append(out, g.seq)
	}
	return out
}

// GraphicRows returns where each sequence was placed, in the same order. The
// frame is diffed by row, so a sequence on an unchanged row is never sent.
func (b *Buffer) GraphicRows() [][2]int {
	out := make([][2]int, 0, len(b.graphics))
	for _, g := range b.graphics {
		out = append(out, [2]int{g.x, g.y})
	}
	return out
}

// NewBuffer returns a blank Buffer. Dimensions below 1 are clamped, so a
// zero-size terminal cannot produce a zero-length backing array.
func NewBuffer(w, h int) *Buffer { return NewBufferWith(w, h, defaultAttr) }

// NewBufferWith returns a Buffer blanked to a given attribute in one pass.
func NewBufferWith(w, h int, a Attr) *Buffer {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	b := &Buffer{
		w:     w,
		h:     h,
		runes: make([]rune, w*h),
		attrs: make([]Attr, w*h),
	}
	b.ClearTo(a)
	return b
}

// Size returns the buffer dimensions in cells.
func (b *Buffer) Size() (w, h int) { return b.w, b.h }

// Reuse returns a buffer of the given size cleared to a, reusing b's storage
// when it is already that size. A fresh frame per session sixty times a second
// is gigabytes a second of garbage across a full floor. A nil receiver returns
// a new buffer.
//
// Whatever the last frame drew is gone. That is safe only because a Buffer is
// per-session and nothing keeps one past the call that drew it.
func (b *Buffer) Reuse(w, h int, a Attr) *Buffer {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	if b == nil || b.w != w || b.h != h {
		return NewBufferWith(w, h, a)
	}
	b.ClearTo(a)
	return b
}

// Clear resets every cell to an unstyled space, reusing the backing arrays.
func (b *Buffer) Clear() { b.ClearTo(defaultAttr) }

// ClearTo blanks the buffer to a given attribute, so a page with a background
// colour has it in every cell. Most themes use the zero attribute, which
// clears with the clear builtin (a memclr).
func (b *Buffer) ClearTo(a Attr) {
	// Images are placed per frame, so they are cleared with the runes.
	b.graphics = b.graphics[:0]
	b.pixelImages = b.pixelImages[:0]
	clear(b.pixelOwners)

	for i := range b.runes {
		b.runes[i] = ' '
	}
	if a == (Attr{}) {
		clear(b.attrs)
		return
	}
	for i := range b.attrs {
		b.attrs[i] = a
	}
}

// Set writes a single cell. Out-of-bounds coordinates are dropped so callers can
// blit freely, for instance particles flying off the edge of the viewport.
func (b *Buffer) Set(x, y int, r rune, a Attr) {
	if x < 0 || y < 0 || x >= b.w || y >= b.h {
		return
	}
	i := y*b.w + x
	b.runes[i] = r
	b.attrs[i] = a
	if b.pixelOwners != nil {
		b.pixelOwners[i] = 0
	}
}

// RuneAt returns the rune in a cell, or a space if the coordinates are outside
// the buffer.
func (b *Buffer) RuneAt(x, y int) rune {
	if x < 0 || y < 0 || x >= b.w || y >= b.h {
		return ' '
	}
	return b.runes[y*b.w+x]
}

// AttrAt returns a cell's styling, or the zero Attr if the coordinates are
// outside the buffer.
func (b *Buffer) AttrAt(x, y int) Attr {
	if x < 0 || y < 0 || x >= b.w || y >= b.h {
		return defaultAttr
	}
	return b.attrs[y*b.w+x]
}

// Row returns one row as an unstyled string, or empty if the row is outside the
// buffer.
func (b *Buffer) Row(y int) string {
	if y < 0 || y >= b.h {
		return ""
	}
	return string(b.runes[y*b.w : (y+1)*b.w])
}

// Text writes a left-to-right run of runes starting at (x, y), one cell per
// rune regardless of how many bytes it takes.
func (b *Buffer) Text(x, y int, s string, a Attr) {
	for _, r := range s {
		b.Set(x, y, r, a)
		x++
	}
}

// HLine draws a horizontal run of a single rune.
func (b *Buffer) HLine(x, y, length int, r rune, a Attr) {
	for i := 0; i < length; i++ {
		b.Set(x+i, y, r, a)
	}
}

// VLine draws a vertical run of a single rune.
func (b *Buffer) VLine(x, y, length int, r rune, a Attr) {
	for i := 0; i < length; i++ {
		b.Set(x, y+i, r, a)
	}
}

// Box draws a rectangular border in light box-drawing characters. Sizes below
// 2x2 draw nothing.
func (b *Buffer) Box(x, y, w, h int, a Attr) {
	if w < 2 || h < 2 {
		return
	}
	b.Set(x, y, '┌', a)
	b.Set(x+w-1, y, '┐', a)
	b.Set(x, y+h-1, '└', a)
	b.Set(x+w-1, y+h-1, '┘', a)
	b.HLine(x+1, y, w-2, '─', a)
	b.HLine(x+1, y+h-1, w-2, '─', a)
	b.VLine(x, y+1, h-2, '│', a)
	b.VLine(x+w-1, y+1, h-2, '│', a)
}

// BlitBraille copies every cell of a braille canvas into the buffer with its
// top-left cell at (x, y), blank cells included.
func (b *Buffer) BlitBraille(x, y int, br *Braille, a Attr) {
	w, h := br.CellSize()
	for cy := 0; cy < h; cy++ {
		for cx := 0; cx < w; cx++ {
			b.Set(x+cx, y+cy, br.Rune(cx, cy), a)
		}
	}
}

// BlitBrailleOver writes only the cells with a dot lit, leaving the rest as
// they were. Use it for an outline over existing content, such as a lid.
func (b *Buffer) BlitBrailleOver(x, y int, br *Braille, a Attr) {
	w, h := br.CellSize()
	for cy := 0; cy < h; cy++ {
		for cx := 0; cx < w; cx++ {
			if r := br.Rune(cx, cy); r != BrailleBlank {
				b.Set(x+cx, y+cy, r, a)
			}
		}
	}
}

// wrap is the pair of escape sequences that bracket a styled run.
type wrap struct{ prefix, suffix string }

// wrapCache memoises those sequences per attribute for a nil Styler.
// lipgloss.Style.Render costs about fifty allocations per call, so each
// attribute is rendered once against a sentinel and the sequences either side
// are kept. lipgloss still decides them, so colour-profile degradation is
// preserved.
var (
	wrapCache = map[Attr]wrap{}
	wrapMu    sync.RWMutex
)

// sentinel cannot appear in a frame. It marks where lipgloss puts the content.
const sentinel = "\x00"

// Styler turns attributes into the escape sequences one terminal understands.
// lipgloss's default renderer is bound to the process's stdout, so remotely
// each session needs its own renderer and its own cache, or every client gets
// the first client's colour depth.
type Styler struct {
	renderer *termRenderer

	mu    sync.RWMutex
	cache map[Attr]wrap

	// seq holds the component sequences for moving between attributes. See
	// sgr.go.
	seq seqs
}

// wrap is the prefix and suffix for an attribute on this terminal.
func (s *Styler) wrap(a Attr) wrap {
	if s == nil {
		return a.wrap()
	}
	s.mu.RLock()
	w, ok := s.cache[a]
	s.mu.RUnlock()
	if ok {
		return w
	}
	w = s.build(a)
	s.mu.Lock()
	s.cache[a] = w
	s.mu.Unlock()
	return w
}

func (a Attr) wrap() wrap {
	wrapMu.RLock()
	w, ok := wrapCache[a]
	wrapMu.RUnlock()
	if ok {
		return w
	}

	w = buildDefault(a)

	wrapMu.Lock()
	wrapCache[a] = w
	wrapMu.Unlock()
	return w
}

// NoBold suppresses SGR 1 everywhere, for terminals that cannot draw it. Some
// terminals fall back to a font with no glyph for the character when there is
// no bold face, so bold text comes out as blocks. It is process-wide because it
// describes the terminal lumon was started in, and it covers every attribute,
// including ones no theme names.
var NoBold bool

// String flattens the buffer into a newline-separated frame with no trailing
// newline, using the process's own terminal.
func (b *Buffer) String() string { return b.StringWith(nil) }

// StringWith flattens the buffer using one terminal's Styler. A nil Styler uses
// lipgloss's process-wide default, which is what a program with a terminal of
// its own wants.
func (b *Buffer) StringWith(st *Styler) string {
	var sb strings.Builder
	// Size from the last frame plus a margin; consecutive frames are close in
	// length. w*h*2+h ignores escape sequences, so on its own the builder
	// would grow mid-write every frame. It stays as the floor.
	if grow := b.lastLen + b.lastLen/16; grow > b.w*b.h*2+b.h {
		sb.Grow(grow)
	} else {
		sb.Grow(b.w*b.h*2 + b.h)
	}

	// Every line begins in the default state and restores it before the
	// newline. Bubble Tea skips a line whose bytes match the last frame's, so
	// SGR state carried between rows would be wrong after a skipped line and
	// would make unchanged rows differ. See
	// TestASkippedLineLeavesTheNextOneCorrect.
	for y := 0; y < b.h; y++ {
		b.writeRow(&sb, st, y)
		if y < b.h-1 {
			sb.WriteByte('\n')
		}
	}
	b.lastLen = sb.Len()
	return sb.String()
}

// writeRow writes one row, starting and ending in the default state.
func (b *Buffer) writeRow(sb *strings.Builder, st *Styler, y int) {
	base := y * b.w
	cur := defaultAttr

	// The next image on this row and its column, or -1. Hoisted so a row
	// with no images costs one integer compare per cell.
	gi, gx := 0, -1
	for i := range b.graphics {
		if b.graphics[i].y == y {
			gi, gx = i, b.graphics[i].x
			break
		}
	}

	for x := 0; x < b.w; x++ {
		if x == gx {
			// Every image at this column in drawing order, so a later one
			// paints over an earlier one. Then find the next one along.
			skip := 0
			for ; gi < len(b.graphics); gi++ {
				g := b.graphics[gi]
				if g.y != y || g.x != x {
					break
				}
				sb.WriteString(g.seq)
				if g.skip > skip {
					skip = g.skip
				}
			}
			gx = -1
			for i := gi; i < len(b.graphics); i++ {
				if b.graphics[i].y == y {
					gi, gx = i, b.graphics[i].x
					break
				}
			}

			// Cells the terminal owns: step over them. See Reserve.
			if skip > 0 {
				if skip > b.w-x {
					skip = b.w - x
				}
				sb.WriteString("\x1b[")
				sb.WriteString(strconv.Itoa(skip))
				sb.WriteByte('C')
				// CUF changes neither cells nor SGR state, so cur stays
				// correct.
				x += skip - 1 // the loop's own increment carries the last one
				continue
			}
		}
		a, r := b.attrs[base+x], b.runes[base+x]
		if !carries(cur, a, r) {
			st.writeDelta(sb, cur, a)
			cur = a
		}
		// WriteByte inlines and WriteRune does not, and nearly every cell is
		// ASCII. For a rune under RuneSelf they write the same byte.
		if r < utf8.RuneSelf {
			sb.WriteByte(byte(r))
		} else {
			sb.WriteRune(r)
		}
	}
	if cur != defaultAttr {
		sb.WriteString(st.resetSeq())
	}
}
