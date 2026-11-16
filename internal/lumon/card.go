package lumon

import (
	"github.com/terminaltrove/lumon/internal/ease"
	"math"
	"strings"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// A macrodata file, drawn as the rolodex card it is filed on: two panels
// hanging on a spindle, a lettered divider tab on top, a foot below, and the
// edges of the other cards in the drawer showing either side of the hinge.
//
// On a character grid, hierarchy comes from stroke weight:
//
//   - block characters for the hardware: the pins and the comb stacks;
//   - light rounded box-drawing for the card's faces;
//   - a dashed rule for the completion, since a solid one inside the lower
//     panel reads as a third panel boundary.
//
// The gap between the panels is open at the sides. A wall through the hinge
// row makes it read as one box with a shelf.

// cardSpec is one size of card. Drawing functions read every dimension from
// here, never from package constants.
type cardSpec struct {
	// w is the card's width; up and down are how many rows each panel spans,
	// top edge to bottom edge inclusive. Keep them equal, or the upper panel,
	// which holds the name, is cramped and the flip looks lopsided.
	w, up, down int

	// tabW is the divider tab's width. It is one row tall: the letter sits in
	// an opening in the card's own top edge.
	tabW int

	footW int

	// pinL and pinR are the columns the two pins stand in, from the card's left
	// edge, and pinW how wide each is. They are symmetric about the centre and
	// drawn last, in front of the panel borders, so they read as hardware.
	pinL, pinR, pinW int

	// The comb stacks, the other cards seen edge-on at the spindle: combW is
	// each bar's width, combGap the gap to the card, and combN the number of
	// bars, one per row, centred on the hinge. combW stays short (about a
	// twelfth of the card) so the bars read as paper edges, not a ladder.
	combW, combGap, combN int

	// belowRows is what is kept clear beneath the card: a blank row, the
	// notice, the hint, and one more.
	belowRows int
}

// The two sizes. Compact lets a stock 80×24 terminal draw a card.
var (
	// cardFull: body 36 × 19, which is 36 × 38 in visual units with the
	// terminal's 2:1 cell. Footprint 44 × 21 with the combs, tab and foot.
	cardFull = cardSpec{
		w: 36, up: 9, down: 9,
		tabW: 7, footW: 8,
		pinL: 10, pinR: 22, pinW: 4,
		combW: 3, combGap: 1, combN: 5,
		belowRows: 4,
	}

	// cardCompact: body 26 × 13, square in visual units. Footprint 34 × 15.
	//
	// The tab stays seven columns: at five it is 2.5:1 in visual units and
	// starts to read as a box. The pins are three columns because a pin's
	// rounding is in its outer columns, so two columns cannot be rounded.
	cardCompact = cardSpec{
		w: 26, up: 6, down: 6,
		tabW: 7, footW: 6,
		pinL: 8, pinR: 15, pinW: 3,
		combW: 3, combGap: 1, combN: 5,
		belowRows: 4,
	}
)

// cardBox is where a card is drawn (the screen cell its hinge is centred on)
// and which of the two sizes is drawn there.
type cardBox struct {
	cx, hinge int
	spec      cardSpec
}

// cardLiftRows is how far a presented card stands out of the drawer, in rows.
// It must not exceed cardTop, the rows clear above the tab's cap on the
// shortest terminal that draws a card.
const cardLiftRows = 2

// cardTop is the drawer's top margin, the same as lobbyTop.
const cardTop = 2

// How many cards stand behind the one being read, and how far each is offset,
// in columns.
const (
	cardStackDepth = 3
	cardStackStep  = 2
)

// cardFits reports whether a terminal can hold a card, which size, and where it
// goes. Cards do not stretch: it takes the largest size that fits, combs
// included, and below both the drawer falls back to words.
func cardFits(w, h int) (cardBox, bool) {
	for _, s := range []cardSpec{cardFull, cardCompact} {
		need := s.w + 2*(s.combW+s.combGap)
		tall := s.up + 1 + s.down + 1 + cardTop + s.belowRows
		if w < need || h < tall {
			continue
		}
		mid := cardTop + (h-cardTop-s.belowRows)/2
		return cardBox{cx: w / 2, hinge: mid, spec: s}, true
	}
	return cardBox{}, false
}

// cardLeft is the card's left-hand column.
func (b cardBox) left() int { return b.cx - b.spec.w/2 }

// cardTabX is the column a letter's divider tab stands in, from the card's left
// edge. It is staggered by the letter, A hard left and Z hard right, as on a
// rolodex divider. The full card has 26 distinct positions.
func cardTabX(s cardSpec, r rune) int {
	i := clampInt(int(upperRune(r))-'A', 0, 25)
	return 2 + int(math.Round(float64(s.w-s.tabW-4)*float64(i)/25))
}

// drawCard draws one card, foreshortened by scale about its hinge. scale runs
// from 1 (flat), through 0 (edge-on), to -1 (flat and facing away).
//
// The flip is a vertical squash about a fixed hinge only. Horizontal
// foreshortening or perspective would need diagonal edges, which look broken
// at this resolution.
//
// As the card closes, the status goes first, then the rule, the foot, the
// name and the tab, and finally the card is a single heavy line. The pins are
// the axis and never change.
func drawCard(buf *canvas.Buffer, box cardBox, scale float64, e Entry, status string, attr canvas.Attr) {
	s := box.spec
	a := math.Abs(scale)
	left, right := box.left(), box.left()+s.w-1

	// The panels close toward the hinge. The one-row gap between them holds
	// the pins.
	upBottom, loTop := box.hinge-1, box.hinge+1
	upTop := upBottom - int(math.Round(float64(s.up-1)*a))
	loBottom := loTop + int(math.Round(float64(s.down-1)*a))
	upH, loH := upBottom-upTop, loBottom-loTop

	// Edge-on the card is a single heavy line with blunt ends, and the pins
	// stand proud of it.
	if upH < 1 {
		cardEdge(buf, left, right, box.hinge, attr)
		cardPins(buf, box, upBottom, loTop, attr)
		return
	}

	cardPanel(buf, left, right, upTop, upBottom, attr)
	cardPanel(buf, left, right, loTop, loBottom, attr)

	if loH >= 5 {
		cardRule(buf, box, loBottom-2, e, attr)
	}
	if loH >= 3 {
		cardFoot(buf, box, loBottom, attr)
	}

	// The status line, centred above the rule like the name. The caller sizes
	// it (see cardStatus).
	if loH >= 6 && status != "" {
		n := len([]rune(status))
		buf.Text(left+1+(s.w-2-n)/2, loBottom-3, status, attr)
	}

	// The name is anchored to the panel's top edge, upH/4 below it, which
	// keeps a clear row above the pin caps at every height.
	if upH >= 2 {
		name := clipTo(e.Name, s.w-6)
		if scale < 0 {
			name = reverseString(name) // the back of the card
		}
		n := len([]rune(name))
		buf.Text(left+1+(s.w-2-n)/2, upTop+1+upH/4, name, attr)
	}
	if upH >= 1 {
		cardTab(buf, box, upTop, cardInitial(e.Name), attr)
	}

	cardPins(buf, box, upBottom, loTop, attr)
}

// cardTab is the lettered divider tab:
//
//	               ╭─────╮
//	╭──────────────╯  N  ╰─────────────╮
//
// It has three sides and the letter sits in an opening in the card's top edge,
// so the outline runs into the tab and back out as one stroke. `┴` junctions
// would make it read as a separate box sitting on the card. At seven columns
// by one row it is 3.5:1 in visual units, low and flat.
func cardTab(buf *canvas.Buffer, box cardBox, y int, r rune, attr canvas.Attr) {
	s := box.spec
	x := box.left() + cardTabX(s, r)

	// The opening in the top edge. The spaces overwrite the panel's `─`.
	buf.Set(x, y, '╯', attr)
	buf.HLine(x+1, y, s.tabW-2, ' ', attr)
	buf.Set(x+s.tabW/2, y, r, attr)
	buf.Set(x+s.tabW-1, y, '╰', attr)

	// The cap, one row proud.
	buf.Set(x, y-1, '╭', attr)
	buf.HLine(x+1, y-1, s.tabW-2, '─', attr)
	buf.Set(x+s.tabW-1, y-1, '╮', attr)
}

// drawCardStack draws the cards standing behind this one, their tabs above it
// and their feet below.
//
// They are offset by cardStackStep, not by their own letters' cardTabX: the
// alphabet stagger is about a column a letter, too little to show each tab.
// They are drawn furthest first, at the resting card's rows, so they stay
// still while the front card turns. The two full-width edges are hidden behind
// a flat card and appear as it turns.
func drawCardStack(buf *canvas.Buffer, box cardBox, initial rune, before, after int, attr canvas.Attr) {
	s := box.spec
	left, right := box.left(), box.left()+s.w-1
	capY := box.hinge - s.up - 1
	botY := box.hinge + s.down

	buf.Set(left, capY+1, '╭', attr)
	buf.HLine(left+1, capY+1, s.w-2, '─', attr)
	buf.Set(right, capY+1, '╮', attr)

	buf.Set(left, botY, '╰', attr)
	buf.HLine(left+1, botY, s.w-2, '─', attr)
	buf.Set(right, botY, '╯', attr)

	tabX := left + cardTabX(s, initial)
	footX := left + s.w/2 - s.footW/2

	for i := max(before, after); i >= 1; i-- {
		if i <= before {
			cardStackTab(buf, box, tabX-i*cardStackStep, capY, attr)
			cardStackFoot(buf, box, footX-i*cardStackStep, botY+1, attr)
		}
		if i <= after {
			cardStackTab(buf, box, tabX+i*cardStackStep, capY, attr)
			cardStackFoot(buf, box, footX+i*cardStackStep, botY+1, attr)
		}
	}
}

// cardStackTab is one card-behind's divider tab, clamped inside the card so a
// stack near the end of the alphabet piles up instead of hanging off the side.
func cardStackTab(buf *canvas.Buffer, box cardBox, x, y int, attr canvas.Attr) {
	s := box.spec
	x = clampInt(x, box.left()+1, box.left()+s.w-s.tabW-1)
	buf.Set(x, y, '╭', attr)
	buf.HLine(x+1, y, s.tabW-2, '─', attr)
	buf.Set(x+s.tabW-1, y, '╮', attr)
}

// cardStackFoot is one card-behind's notch, hanging below this card's own.
func cardStackFoot(buf *canvas.Buffer, box cardBox, x, y int, attr canvas.Attr) {
	s := box.spec
	x = clampInt(x, box.left()+1, box.left()+s.w-s.footW-1)
	buf.Set(x, y, '╰', attr)
	buf.HLine(x+1, y, s.footW-2, '─', attr)
	buf.Set(x+s.footW-1, y, '╯', attr)
}

// clearCard blanks the ground a card is about to be drawn on. Only a lifted
// card needs it, since it has the drawer's tabs behind it.
func clearCard(buf *canvas.Buffer, box cardBox, attr canvas.Attr) {
	s := box.spec
	for y := box.hinge - s.up - 1; y <= box.hinge+s.down+1; y++ {
		buf.HLine(box.left(), y, s.w, ' ', attr)
	}
}

// cardPanel draws one of the card's two faces.
func cardPanel(buf *canvas.Buffer, left, right, top, bottom int, attr canvas.Attr) {
	if bottom <= top {
		cardEdge(buf, left, right, top, attr)
		return
	}
	buf.Set(left, top, '╭', attr)
	buf.Set(right, top, '╮', attr)
	buf.Set(left, bottom, '╰', attr)
	buf.Set(right, bottom, '╯', attr)
	buf.HLine(left+1, top, right-left-1, '─', attr)
	buf.HLine(left+1, bottom, right-left-1, '─', attr)
	buf.VLine(left, top+1, bottom-top-1, '│', attr)
	buf.VLine(right, top+1, bottom-top-1, '│', attr)
}

// cardEdge is a card seen edge-on: one heavy line with blunt ends.
func cardEdge(buf *canvas.Buffer, left, right, y int, attr canvas.Attr) {
	buf.Set(left, y, '╺', attr)
	buf.Set(right, y, '╸', attr)
	buf.HLine(left+1, y, right-left-1, '━', attr)
}

// cardFoot is the notch in the card's bottom edge: the tab's construction
// mirrored, with the edge turning down into it and back up (`╮` … `╭`).
func cardFoot(buf *canvas.Buffer, box cardBox, edgeY int, attr canvas.Attr) {
	s := box.spec
	x := box.left() + s.w/2 - s.footW/2

	buf.Set(x, edgeY, '╮', attr)
	buf.HLine(x+1, edgeY, s.footW-2, ' ', attr)
	buf.Set(x+s.footW-1, edgeY, '╭', attr)

	buf.Set(x, edgeY+1, '╰', attr)
	buf.HLine(x+1, edgeY+1, s.footW-2, '─', attr)
	buf.Set(x+s.footW-1, edgeY+1, '╯', attr)
}

// cardPins draws the two pins the card hangs from. They are the axis and
// never change size. A pin is three rows with chamfered corners:
//
//	▄██▄
//	████
//	▀██▀
//
// A pin needs at least three columns to be rounded.
func cardPins(buf *canvas.Buffer, box cardBox, upBottom, loTop int, attr canvas.Attr) {
	s := box.spec
	inner := s.pinW - 2

	for _, x := range [2]int{box.left() + s.pinL, box.left() + s.pinR} {
		for y := upBottom; y <= loTop; y++ {
			buf.HLine(x, y, s.pinW, '█', attr)
		}
		if inner < 1 {
			continue // nothing to round against
		}
		// The top row's outer columns keep only their bottom halves and the
		// bottom row's only their top halves, so all four corners come off.
		buf.Set(x, upBottom, '▄', attr)
		buf.Set(x+s.pinW-1, upBottom, '▄', attr)
		buf.Set(x, loTop, '▀', attr)
		buf.Set(x+s.pinW-1, loTop, '▀', attr)
	}
}

// cardRule is the progress line low in the lower panel: dashed for what is
// left, solid for what is done, with partialBlock for the leading edge. An
// unopened file has none.
func cardRule(buf *canvas.Buffer, box cardBox, y int, e Entry, attr canvas.Attr) {
	if !e.Open {
		return
	}
	left := box.left()
	w := box.spec.w - 6
	exact := float64(w) * ease.Clamp01(float64(e.Percent)/100)
	whole := int(exact)

	buf.HLine(left+3, y, w, '┄', attr)
	if whole > 0 {
		buf.HLine(left+3, y, whole, '━', attr)
	}
	if whole < w {
		if r := partialBlock(exact - float64(whole)); r != 0 {
			buf.Set(left+3+whole, y, r, attr)
		}
	}
}

// drawCardCombs draws the rest of the drawer, edge-on, down both flanks. The
// combs do not flip, which gives the turning card something to move against.
//
// Identical bars read as a comb, so each bar is flush with the card and ragged
// at its free edge, and some are drawn in the far colour. Both come from
// combGrain, a hash of the bar's index, so the profile is stable across frames
// and cards.
func drawCardCombs(buf *canvas.Buffer, box cardBox, near, far canvas.Attr) {
	s := box.spec
	left := box.left()
	top := box.hinge - s.combN/2

	for i := 0; i < s.combN; i++ {
		y := top + i
		g := combGrain(i)

		// Bits 3 and 5: over the few indices a stack has, the low bits come
		// out mostly one way, while these split nearly evenly.
		w := s.combW - int(g>>3&1)
		attr := near
		if g>>5&1 != 0 {
			attr = far
		}

		buf.HLine(left-s.combGap-w, y, w, '▄', attr)
		buf.HLine(left+s.w+s.combGap, y, w, '▄', attr)
	}
}

// combGrain is the fixed unevenness of the stack, per bar. It is an integer
// hash like solidDigit, since smooth Perlin noise would put short bars in runs.
func combGrain(i int) uint32 {
	h := uint32(i)*2654435761 + 0x9E3779B9
	h ^= h >> 15
	h *= 2246822519
	h ^= h >> 13
	return h
}

// cardInitial is the letter a file is filed behind.
func cardInitial(name string) rune {
	r := []rune(name)
	if len(r) == 0 {
		return ' '
	}
	return upperRune(r[0])
}

// cardStatus is what a file says about itself, in one line, at most width
// columns wide: its percentage and who is on it, by name.
//
// The names come from Listing.Refiners, copied out under the room's
// read lock; a lobby session holds no board lock and must never read a room's
// desks directly. As width shrinks, names become initials, then a count. The
// percentage is never dropped.
func cardStatus(e Entry, width int) string {
	if !e.Open {
		return "UNOPENED"
	}
	head := itoa(e.Percent) + "% · "
	room := width - len([]rune(head))

	switch {
	case e.Occupied == 0:
		return head + "EMPTY"
	case room <= 0:
		return itoa(e.Percent) + "%"
	}

	// FULL comes before the names and is never dropped, so being refused a
	// desk is not a surprise.
	if e.Full() {
		head += "FULL"
		if room -= 4; room <= 3 {
			return head
		}
		head += " · "
		room -= 3
	}

	// Full names, then initials, then a count.
	for _, form := range []func(Occupant) string{
		func(o Occupant) string { return o.Name },
		func(o Occupant) string { return initials(o.Name) },
	} {
		names := make([]string, 0, len(e.Refiners))
		for _, o := range e.Refiners {
			names = append(names, form(o))
		}
		if s := strings.Join(names, ", "); s != "" && len([]rune(s)) <= room {
			return head + s
		}
	}
	if e.Full() {
		return strings.TrimSuffix(head, " · ")
	}

	tail := itoa(e.Occupied) + " REFINERS"
	if e.Occupied == 1 {
		tail = "1 REFINER"
	}
	if len([]rune(tail)) > room {
		tail = itoa(e.Occupied) + "/" + itoa(e.Desks)
	}
	if len([]rune(tail)) > room {
		return itoa(e.Percent) + "%"
	}
	return head + tail
}

func upperRune(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 'a' + 'A'
	}
	return r
}

func reverseString(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}
