package lumon

import (
	"math"
	"strings"
	"testing"

	"github.com/terminaltrove/lumon/internal/canvas"
)

func drawer(t *testing.T, w, h int) *Model {
	t.Helper()
	rs := NewRooms(17)
	rs.EnsureOpen(DefaultRooms)
	m := NewLocalSession(rs, 1, "mark")
	m.Resize(w, h)
	tick(m, 90) // let the drawer come up
	riffled(m)
	return m
}

// riffled runs m until its drawer has riffled onto its card.
func riffled(m *Model) {
	for i := 0; i < 60*5 && m.rolodex().Riffling(); i++ {
		tick(m, 1)
	}
}

// A card half way through a turn is edge-on and shows nothing (the blank frames
// in the reference), so "it turned" and "it shows the next one" are different
// moments and the test checks both.
func TestTheReelTurnsAndSettles(t *testing.T) {
	m := drawer(t, 120, 40)
	r := m.rolodex()
	from := r.At()

	m.Update(key("down"))
	tick(m, 1)
	if r.At() == from {
		t.Fatal("a keypress moved the drawer not at all")
	}
	if r.at.Velocity() <= 0 {
		t.Errorf("a press left the drawer going at %v; it is meant to have been "+
			"shoved, not placed", r.at.Velocity())
	}

	// It keeps going after the press, which a spring to a card cannot do.
	was := r.At()
	tick(m, 1)
	if r.At() <= was {
		t.Error("the drawer stopped the frame after the press rather than coasting")
	}

	settleReel(t, m)
	if got := math.Abs(r.At() - math.Round(r.At())); got > reelArrived {
		t.Errorf("it came to rest %v cards off a card; the detent did not claim it", got)
	}
}

// Sampled across a full turn, the drawn card must pass through nothing and out
// the other side.
func TestACardFlipsThroughEdgeOn(t *testing.T) {
	m := drawer(t, 120, 40)
	r := m.rolodex()

	m.Update(key("down"))
	thin, thick := 1.0, 0.0
	for i := 0; i < 900 && r.Turning(); i++ {
		tick(m, 1)
		_, scale, ok := r.Showing()
		if !ok {
			t.Fatal("nothing is showing mid-turn")
		}
		if s := math.Abs(scale); s < thin {
			thin = s
		} else if s > thick {
			thick = s
		}
	}
	if thin > 0.35 {
		t.Errorf("the card never got closer to edge-on than %v of full size; "+
			"it is squashing rather than turning over", thin)
	}
	if _, scale, _ := r.Showing(); math.Abs(scale) < 0.99 {
		t.Errorf("the reel settled with the card at %v of full size", scale)
	}
}

// That is the difference between a drawer turning and one card breathing in
// place.
func TestPastTheFoldTheNextCardIsShowing(t *testing.T) {
	m := drawer(t, 120, 40)
	r := m.rolodex()
	was, _ := r.Selected()

	m.Update(key("down"))
	swapped := false
	for i := 0; i < 900 && r.Turning(); i++ {
		tick(m, 1)
		if e, _, _ := r.Showing(); e.Name != was.Name {
			swapped = true
			break
		}
	}
	if !swapped {
		t.Error("the card on screen never changed during the turn")
	}
}

// The tabs make fifty-two cards navigable: one press moves a letter, and
// lands on the first file under it either way.
func TestTheLetterTabsJumpAWholeLetter(t *testing.T) {
	m := drawer(t, 120, 40)
	r := m.rolodex()
	r.GoTo(0) // the front of the drawer, which is the first card under A

	first, _ := r.Selected()
	m.Update(key("right"))
	next, _ := r.Selected()

	if initialOf(next.Name) == initialOf(first.Name) {
		t.Errorf("right went from %q to %q, still under %c",
			first.Name, next.Name, initialOf(first.Name))
	}

	// Back again lands on the first of the letter, not its last card.
	m.Update(key("left"))
	back, _ := r.Selected()
	if back.Name != first.Name {
		t.Errorf("right then left went %q -> %q -> %q, want back where it started",
			first.Name, next.Name, back.Name)
	}

	// Every jump forward changes letter, all the way to the end.
	for i := 0; i < r.Len(); i++ {
		was, _ := r.Selected()
		m.Update(key("right"))
		got, _ := r.Selected()
		if got.Name == was.Name {
			break // the back of the drawer
		}
		if initialOf(got.Name) == initialOf(was.Name) {
			t.Fatalf("a jump from %q landed on %q, under the same letter", was.Name, got.Name)
		}
	}
}

// The name is gone mid-turn, as in the reference.
func TestTheCardCarriesItsNameAndLosesItMidTurn(t *testing.T) {
	m := drawer(t, 120, 40)
	e, _ := m.rolodex().Selected()

	if !onScreen(drawTo(m), e.Name) {
		t.Fatalf("the card is not carrying %q", e.Name)
	}
	tab := string(initialOf(e.Name))
	if !onScreen(drawTo(m), tab) {
		t.Errorf("the tab does not carry %q, the letter %q is filed behind", tab, e.Name)
	}

	// Mid-turn, at its thinnest, it shows nothing.
	m.Update(key("down"))
	r := m.rolodex()
	for i := 0; i < 900 && r.Turning(); i++ {
		tick(m, 1)
		if _, scale, _ := r.Showing(); math.Abs(scale) < 0.3 {
			shown, _, _ := r.Showing()
			if onScreen(drawTo(m), shown.Name) {
				t.Errorf("a card edge-on is still carrying %q", shown.Name)
			}
			return
		}
	}
	t.Error("the card never got thin enough to check")
}

func TestTheDrawerFadesIn(t *testing.T) {
	rs := NewRooms(17)
	rs.EnsureOpen(DefaultRooms)
	m := NewLocalSession(rs, 1, "mark")
	m.Resize(120, 40)

	tick(m, 1)
	early := m.rolodex().Level()
	if early <= 0 || early > 0.5 {
		t.Errorf("one frame in the drawer is at %v; want it on its way up", early)
	}
	tick(m, 120)
	if late := m.rolodex().Level(); late < 0.95 {
		t.Errorf("after two seconds the drawer is still at %v", late)
	}
}

// Too small for a card, it gives the file's name in words instead of drawing
// nothing.
func TestASmallTerminalStillNamesTheFile(t *testing.T) {
	m := drawer(t, 44, 12)
	e, _ := m.rolodex().Selected()

	if _, ok := cardFits(44, 12); ok {
		t.Skip("the card fits after all")
	}
	if !onScreen(drawTo(m), e.Name) {
		t.Errorf("a terminal too small for the card does not name %q either", e.Name)
	}
}

// The status is on the card holding the name. The place in the drawer is not
// shown: the tabs show it roughly, and the list and grid show it directly.
func TestTheCardSaysWhatTheFileIsDoing(t *testing.T) {
	m := drawer(t, 120, 40)
	e, _ := m.rolodex().Selected()
	box, ok := cardFits(m.Layout.Width, m.Layout.Height)
	if !ok {
		t.Fatal("the card does not fit")
	}
	buf := drawTo(m)

	want := cardStatus(e, box.spec.w-6)
	if want == "" {
		t.Fatal("the card has nothing to say about its own file")
	}
	if !onScreen(buf, want) {
		t.Errorf("the card does not carry %q", want)
	}

	// And it is inside the card, not under it.
	found := -1
	for y := 0; y < m.Layout.Height; y++ {
		if strings.Contains(string(buf.Row(y)), want) {
			found = y
		}
	}
	lo, hi := box.hinge+1, box.hinge+box.spec.down
	if found < lo || found > hi {
		t.Errorf("%q is drawn on row %d, outside the card's lower panel (%d..%d)",
			want, found, lo, hi)
	}
}

// The directory offers every file, so most of it has never been worked.
func TestTakingAnUnopenedCardOpensTheFile(t *testing.T) {
	rs := NewRooms(19)
	m := NewLocalSession(rs, 1, "mark")
	m.Resize(120, 40)
	tick(m, 30)
	riffled(m)

	e, _ := m.rolodex().Selected()
	if e.Open {
		t.Skip("the drawer opened on a file that was already going")
	}
	m.Update(key("enter"))

	if m.Room == nil {
		t.Fatal("taking an unopened card did not seat anybody")
	}
	if m.FileName != e.Name {
		t.Errorf("took %q and landed on %q", e.Name, m.FileName)
	}
	if rs.Find(e.Name) == nil {
		t.Error("the file was joined without being opened in the registry")
	}
}

// The directory shifts whenever somebody opens a file outside the alphabetical
// run.
func TestTheReelKeepsItsPlaceWhenTheDirectoryGrows(t *testing.T) {
	rs := NewRooms(23)
	m := NewLocalSession(rs, 1, "mark")
	m.Resize(120, 40)
	tick(m, 30)

	m.rolodex().GoTo(20)
	settleReel(t, m)
	was, _ := m.rolodex().Selected()

	// A file whose name sorts before the card being looked at, so every index after
	// it moves along by one.
	rs.OpenNamed("AAAA Test File")
	tick(m, 2)

	if got, _ := m.rolodex().Selected(); got.Name != was.Name {
		t.Errorf("the drawer slipped from %q to %q when a file opened elsewhere",
			was.Name, got.Name)
	}
	if m.rolodex().Turning() {
		t.Error("the reel is turning, and nobody turned it")
	}
}

// The show's files, Mark's completed ones too, and the Bell Labs ones are all
// in the drawer.
func TestTheDrawerHoldsTheFilesFromTheShow(t *testing.T) {
	m := drawer(t, 120, 40)
	have := map[string]bool{}
	for _, e := range m.rolodex().Files() {
		have[e.Name] = true
	}
	for _, name := range []string{
		"Kingsport", "Labrador", "Le Mars", "Longbranch", "Moonbeam",
		"Nanning", "Narva", "Ocula", "Pacoima", "Siena", "Tumwater",
		"Kernighan", "Ritchie", "Thompson", "Pike",
		"Allentown", "Cold Harbor", "Dranesville", "Wellington",
	} {
		if !have[name] {
			t.Errorf("%q is not in the drawer", name)
		}
	}
}

// A Scalar toward a card would make two quick presses two separate journeys.
// With momentum the second press lands while the first is still moving and the
// drawer winds up.
func TestTheDrawerCoastsAndPushesAddUp(t *testing.T) {
	m := drawer(t, 120, 40)
	r := m.rolodex()

	one := func(n int) int {
		r.GoTo(0)
		tick(m, 2)
		from := r.Want()
		for i := 0; i < n; i++ {
			m.Update(key("down"))
			tick(m, 1)
		}
		settleReel(t, m)
		return r.Want() - from
	}

	single := one(1)
	if single < 1 {
		t.Fatalf("one press carried the drawer %d cards", single)
	}
	quick := one(4)
	if quick <= single*2 {
		t.Errorf("four presses in a row carried it %d cards and one carried it %d; "+
			"they are not winding it up", quick, single)
	}
}

// Turning is a shove and the letter tabs are a destination, so they must not
// share a mechanism: a jump lands exactly, with nothing left over.
func TestALetterJumpLandsExactlyAndDoesNotCoast(t *testing.T) {
	m := drawer(t, 120, 40)
	r := m.rolodex()
	r.GoTo(0)
	tick(m, 2)

	m.Update(key("right"))
	if got := r.at.Velocity(); got != 0 {
		t.Errorf("a letter jump left the drawer going at %v; it is a destination, "+
			"not a shove", got)
	}
	if got := r.At(); got != math.Round(got) {
		t.Errorf("a letter jump landed at %v, between two cards", got)
	}
}

// On the braille layer a two-dot horizontal edge lights two of a cell's four dot
// rows and reads as dashes, and vertical flanks come out as lone dots a cell
// apart.
func TestTheCardIsDrawnInRules(t *testing.T) {
	m := drawer(t, 120, 40)
	box, ok := cardFits(m.Layout.Width, m.Layout.Height)
	if !ok {
		t.Fatal("the card does not fit")
	}
	buf := drawTo(m)

	rules, dots := 0, 0
	for y := box.hinge - box.spec.up - 1; y <= box.hinge+box.spec.down+1; y++ {
		for _, r := range buf.Row(y) {
			switch {
			case r >= 0x2500 && r <= 0x257F: // box drawing
				rules++
			case r >= 0x2800 && r <= 0x28FF && r != canvas.BrailleBlank:
				dots++
			}
		}
	}
	if rules < box.spec.w {
		t.Errorf("the card is drawn with %d box-drawing characters; it should be "+
			"made of them", rules)
	}
	if dots > 0 {
		t.Errorf("%d braille cells are still being drawn in the card", dots)
	}
}

// Weight contrast is what makes it read: blocks for the hardware, light rules
// for the faces, and the open gap that makes it a hinge.
func TestTheCardCarriesItsFurniture(t *testing.T) {
	m := drawer(t, 120, 40)
	box, ok := cardFits(m.Layout.Width, m.Layout.Height)
	if !ok {
		t.Fatal("the card does not fit")
	}
	buf := drawTo(m)
	s := box.spec
	left := box.left()

	// The pins are several columns of block, bridging the gap. One column is a
	// stroke; four is an object.
	if s.pinW < 2 {
		t.Errorf("the pins are %d column(s) wide; one is a stroke, not hardware", s.pinW)
	}
	for _, x := range []int{left + s.pinL, left + s.pinR} {
		for dx := 0; dx < s.pinW; dx++ {
			if got := []rune(buf.Row(box.hinge))[x+dx]; got != '█' {
				t.Errorf("pin column %d carries %q at the hinge, want a block", x+dx, got)
			}
		}
	}

	// Symmetric about the card's own centre, or the card looks as though it hangs
	// crooked.
	lc := float64(s.pinL) + float64(s.pinW-1)/2
	rc := float64(s.pinR) + float64(s.pinW-1)/2
	if mid := float64(s.w-1) / 2; math.Abs((lc+rc)/2-mid) > 0.01 {
		t.Errorf("the pins straddle %.1f, not the card's centre %.1f", (lc+rc)/2, mid)
	}

	// The gap is open at the sides. A wall through the hinge row would make the
	// card one box with a shelf, not two panels on a spindle.
	row := []rune(buf.Row(box.hinge))
	for _, x := range []int{left, left + s.w - 1} {
		if got := row[x]; got != ' ' {
			t.Errorf("the hinge row carries %q at the card's edge (%d); the gap "+
				"must be open at the sides", got, x)
		}
	}

	// The comb bars either side, level with the hinge, not touching the card.
	// Counted at the column each stack is flush against, next to the card; the far
	// column is ragged, so counting there finds only the long bars.
	for _, x := range []int{left - s.combGap - 1, left + s.w + s.combGap} {
		bars := 0
		for y := box.hinge - s.combN; y <= box.hinge+s.combN; y++ {
			if []rune(buf.Row(y))[x] == '▄' {
				bars++
			}
		}
		if bars != s.combN {
			t.Errorf("the comb at column %d has %d bars, want %d", x, bars, s.combN)
		}
	}
	if got := []rune(buf.Row(box.hinge))[left-s.combGap]; got != ' ' {
		t.Errorf("the comb touches the card: %q in the column between them", got)
	}
}

// Identical bars evenly spaced read as a barcode or a comb at any count or
// width. A stack of cards seen edge-on is held at one end, uneven at the other,
// and its members do not all catch the light the same.
func TestTheDrawerLooksLikeAStackAndNotAComb(t *testing.T) {
	m := drawer(t, 120, 40)
	box, ok := cardFits(m.Layout.Width, m.Layout.Height)
	if !ok {
		t.Fatal("the card does not fit")
	}
	s := box.spec
	buf := drawTo(m)
	top := box.hinge - s.combN/2

	// Ragged at the free edge, flush at the card. Measured on the left stack, whose
	// free edge is its left-hand end.
	lens := map[int]int{}
	for i := 0; i < s.combN; i++ {
		row := []rune(buf.Row(top + i))
		n := 0
		for x := box.left() - s.combGap - 1; x >= 0 && row[x] == '▄'; x-- {
			n++
		}
		lens[n]++
		if row[box.left()-s.combGap-1] != '▄' {
			t.Errorf("bar %d does not reach the card; the stack is held at the "+
				"spindle and ragged the other way, not the reverse", i)
		}
	}
	if len(lens) < 2 {
		t.Errorf("every bar in the stack is %v columns long; identical bars read "+
			"as a comb, not as the edges of paper", lens)
	}

	// Two tones, so a run of bars all at one screen depth has some depth.
	shades := map[canvas.Attr]int{}
	for i := 0; i < s.combN; i++ {
		shades[buf.AttrAt(box.left()-s.combGap-1, top+i)]++
	}
	if len(shades) < 2 {
		t.Error("the whole stack is drawn in one shade; nothing in it recedes")
	}
}

// The tab's column follows its letter, so the dividers are staggered as a
// rolodex's are, and the tab doubles as a coarse place-marker.
func TestTheTabIsStaggeredByItsLetter(t *testing.T) {
	for _, s := range []cardSpec{cardFull, cardCompact} {
		a, m, z := cardTabX(s, 'A'), cardTabX(s, 'M'), cardTabX(s, 'Z')
		if !(a < m && m < z) {
			t.Errorf("A, M and Z put their tabs at %d, %d, %d; they should march right",
				a, m, z)
		}
		if a < 0 || z+s.tabW > s.w {
			t.Errorf("the tabs run from %d to %d, outside a card %d wide", a, z+s.tabW, s.w)
		}
		if cardTabX(s, 'n') != cardTabX(s, 'N') {
			t.Error("a lower-case initial files behind a different divider")
		}
	}

	// The full card is wide enough to give every letter its own column, so no two
	// letters share a divider.
	seen := map[int]bool{}
	for r := 'A'; r <= 'Z'; r++ {
		seen[cardTabX(cardFull, r)] = true
	}
	if len(seen) != 26 {
		t.Errorf("the full card's tabs take %d distinct columns, want one per letter",
			len(seen))
	}
}

// The pins are the axis and never go; the name goes instead of being written
// through them; and the card never changes width, since a trapezoid at this
// resolution reads as broken.
func TestTheCardDegradesInOrderThroughTheFlip(t *testing.T) {
	m := drawer(t, 120, 40)
	box, ok := cardFits(m.Layout.Width, m.Layout.Height)
	if !ok {
		t.Fatal("the card does not fit")
	}
	e, _ := m.rolodex().Selected()
	sp := box.spec
	left, right := box.left(), box.left()+sp.w-1

	nameGone, pinsGone := -1.0, -1.0
	for i := 20; i >= 0; i-- {
		s := float64(i) / 20
		buf := canvas.NewBuffer(m.Layout.Width, m.Layout.Height)
		drawCard(buf, box, s, e, "", m.Theme.Header)

		name, pins := false, false
		for y := box.hinge - sp.up - 1; y <= box.hinge+sp.down+1; y++ {
			row := []rune(buf.Row(y))
			if strings.Contains(string(row), e.Name) {
				name = true
			}
			if row[left+sp.pinL] == '█' {
				pins = true
			}
			// Never wider or narrower than the card.
			for x := 0; x < len(row); x++ {
				if row[x] != ' ' && (x < left-sp.combW-sp.combGap || x > right+sp.combW+sp.combGap) {
					t.Fatalf("at %.2f something is drawn at column %d, outside the card", s, x)
				}
			}
		}
		if !name && nameGone < 0 {
			nameGone = s
		}
		if !pins && pinsGone < 0 {
			pinsGone = s
		}
	}

	if pinsGone >= 0 {
		t.Errorf("the pins vanished at %.2f; they are the axis and never go", pinsGone)
	}
	if nameGone < 0 {
		t.Error("the name was drawn at every scale, edge-on included")
	}
}

// Measured off the drawn buffer, not the constants, since a wrong call can
// misplace it with cardW correct.
func TestTheNameSitsInTheMiddleOfTheCard(t *testing.T) {
	m := drawer(t, 120, 40)
	box, ok := cardFits(m.Layout.Width, m.Layout.Height)
	if !ok {
		t.Fatal("the card does not fit")
	}

	// A short name and the longest in the pool, where a centring bug that works at
	// one length shows up.
	for _, name := range []string{"Cork", "Nanning", "Todos Santos"} {
		e := Entry{Listing: Listing{Name: name}}
		buf := canvas.NewBuffer(m.Layout.Width, m.Layout.Height)
		drawCard(buf, box, 1, e, "", m.Theme.Header)

		found := false
		for y := box.hinge - box.spec.up - 1; y <= box.hinge+box.spec.down+1; y++ {
			row := string(buf.Row(y))
			at := strings.Index(row, name)
			if at < 0 {
				continue
			}
			found = true
			// A byte offset is not a column: the row is full of box-drawing, which is three
			// bytes a rune.
			i := len([]rune(row[:at]))
			mid := float64(i) + float64(len([]rune(name))-1)/2
			want := float64(box.left()) + float64(box.spec.w-1)/2
			if math.Abs(mid-want) > 1 {
				t.Errorf("%q is centred on column %.1f; the card is centred on %.1f",
					name, mid, want)
			}
		}
		if !found {
			t.Errorf("%q was not drawn on the card at all", name)
		}
	}
}

// Two things make it a tab. Proportion: what protrudes must be much wider than
// tall in visual units, and a terminal row is about two columns tall, so five
// columns by two rows (1.25:1) is square. And three sides: the card's top edge
// turns up into the tab and back down (`╯` … `╰`), instead of running underneath
// it (`┴` … `┴`), which makes the tab a separate object resting on the card.
func TestTheTabReadsAsATabAndNotABox(t *testing.T) {
	for _, s := range []cardSpec{cardFull, cardCompact} {
		box := cardBox{cx: 60, hinge: 20, spec: s}
		buf := canvas.NewBuffer(120, 40)
		drawCard(buf, box, 1, Entry{Listing: Listing{Name: "Nanning"}}, "", canvas.Attr{})

		edgeY := box.hinge - 1 - (s.up - 1)
		cap := []rune(buf.Row(edgeY - 1))

		// The cap protrudes one row, and only one row: the row above it is clear.
		if got := strings.TrimSpace(string(buf.Row(edgeY - 2))); got != "" {
			t.Errorf("something is drawn two rows above the card's edge: %q", got)
		}
		wide := 0
		for _, r := range cap {
			if r != ' ' && r != 0 {
				wide++
			}
		}
		if ratio := float64(wide) / RowAspect; ratio < 3 {
			t.Errorf("the tab protrudes %d columns by one row, %.2f:1 in visual "+
				"units, which reads as a box; want at least 3:1", wide, ratio)
		}

		// Three sides: the edge turns into the tab instead of passing under it.
		edge := string(buf.Row(edgeY))
		if !strings.Contains(edge, "╯") || !strings.Contains(edge, "╰") {
			t.Errorf("the card's top edge does not turn up into the tab: %q",
				strings.TrimSpace(edge))
		}
		if strings.Contains(edge, "┴") {
			t.Error("the top edge runs underneath the tab, which makes it a box on " +
				"the card rather than part of its outline")
		}
	}
}

// The card must not shrink back to 30 × 16, which was too small.
func TestTheCardIsBiggerThanItWas(t *testing.T) {
	box, ok := cardFits(120, 40)
	if !ok {
		t.Fatal("the card does not fit a 120x40 terminal")
	}
	if box.spec.w < 36 || box.spec.up+box.spec.down+1 < 19 {
		t.Errorf("the full card is %d x %d; it was rejected at 30 x 16 and grown "+
			"deliberately", box.spec.w, box.spec.up+box.spec.down+1)
	}
	if box.spec.up != box.spec.down {
		t.Errorf("the hinge is %d above and %d below; an asymmetric hinge cramps "+
			"the panel carrying the name and makes the flip look lopsided",
			box.spec.up, box.spec.down)
	}
}

// The status is on the card. A status line hung a fixed distance under the foot
// would land on h-2, the hint, on a twenty-four-row terminal.
func TestTheCardClearsTheFootOfThePage(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {96, 30}, {120, 40}} {
		w, h := sz[0], sz[1]
		m := drawer(t, w, h)
		box, ok := cardFits(w, h)
		if !ok {
			t.Fatalf("%dx%d gets no card at all", w, h)
		}
		buf := drawTo(m)

		foot := box.hinge + box.spec.down + 1
		hint := -1
		for y := 0; y < h; y++ {
			if strings.Contains(string(buf.Row(y)), "choose") {
				hint = y
			}
		}
		if hint < 0 {
			t.Fatalf("%dx%d: no hint on screen", w, h)
		}
		if foot >= hint {
			t.Errorf("%dx%d: the card's foot is on row %d and the hint on %d",
				w, h, foot, hint)
		}
		if got := h - hint; got != 2 {
			t.Errorf("%dx%d: the hint is %d rows off the bottom, want 2", w, h, got)
		}
	}
}

// The card does not stretch: a prop at a fixed size reads deliberate and a
// scaled one reads like layout. A terminal too short for the full card gets a
// smaller one, and only one too small for that gives the name in words.
func TestASmallTerminalGetsTheCompactCard(t *testing.T) {
	box, ok := cardFits(80, 24)
	if !ok {
		t.Fatal("an 80x24 terminal gets no card")
	}
	if box.spec.w != cardCompact.w {
		t.Errorf("80x24 draws a card %d wide, want the compact one at %d",
			box.spec.w, cardCompact.w)
	}
	if _, ok := cardFits(120, 40); !ok {
		t.Fatal("120x40 gets no card")
	}

	// It is a card, not a fallback: the name is on it and centred.
	m := drawer(t, 80, 24)
	e, _ := m.rolodex().Selected()
	buf := drawTo(m)
	if !onScreen(buf, e.Name) {
		t.Errorf("the compact card does not carry %q", e.Name)
	}
}

// The reference steps the bottom rail down, runs it flat and steps it back up.
// So the edge turns down into the notch (`╮` … `╭`) and the floor is a rounded
// run below it, the mirror of the tab, not a separate `╲▁▁▁▁╱` a row below.
func TestTheFootIsANotchInTheCardsEdge(t *testing.T) {
	for _, s := range []cardSpec{cardFull, cardCompact} {
		box := cardBox{cx: 60, hinge: 20, spec: s}
		buf := canvas.NewBuffer(120, 40)
		drawCard(buf, box, 1, Entry{Listing: Listing{Name: "Nanning"}}, "", canvas.Attr{})

		edgeY := box.hinge + 1 + (s.down - 1)
		edge, floor := string(buf.Row(edgeY)), string(buf.Row(edgeY+1))

		if strings.ContainsAny(edge+floor, "╲╱▁") {
			t.Error("the foot is still drawn in diagonals; it is part of the " +
				"card's outline now, with the same rounded corners as the tab")
		}
		if !strings.Contains(edge, "╮") || !strings.Contains(edge, "╭") {
			t.Errorf("the bottom edge does not turn down into the notch: %q",
				strings.TrimSpace(edge))
		}
		if !strings.Contains(floor, "╰") || !strings.Contains(floor, "╯") {
			t.Errorf("the notch has no rounded floor: %q", strings.TrimSpace(floor))
		}

		// Centred on the card, like the foot in the reference.
		run := []rune(strings.TrimRight(floor, " "))
		lo := len(run) - len([]rune(strings.TrimLeft(string(run), " ")))
		mid := float64(lo+len(run)-1) / 2
		if want := float64(box.left()) + float64(s.w-1)/2; math.Abs(mid-want) > 1 {
			t.Errorf("the notch is centred on %.1f, the card on %.1f", mid, want)
		}
	}
}

// The pin's rounding is in its corners. Capping each end with a half-block
// inset a column gives a narrower nub with two hard shoulders, and five rows for
// a pin that only has to bridge three.
func TestThePinsAreCapsulesAndTheDrawerIsDeep(t *testing.T) {
	for _, s := range []cardSpec{cardFull, cardCompact} {
		box := cardBox{cx: 60, hinge: 20, spec: s}
		buf := canvas.NewBuffer(120, 40)
		drawCard(buf, box, 1, Entry{Listing: Listing{Name: "Nanning"}}, "", canvas.Attr{})

		x := box.left() + s.pinL
		at := func(y, dx int) rune { return []rune(buf.Row(y))[x+dx] }

		// Three rows, and nothing above or below them: the pin bridges the gap and
		// stops.
		for _, y := range []int{box.hinge - 2, box.hinge + 2} {
			for dx := 0; dx < s.pinW; dx++ {
				if got := at(y, dx); got == '█' || got == '▄' || got == '▀' {
					t.Errorf("width %d: the pin reaches row %+d off the hinge (%q); "+
						"it is meant to be the three rows that bridge the gap",
						s.pinW, y-box.hinge, got)
				}
			}
		}

		// The corners are cut and the middles are not, which is the capsule.
		for _, c := range []struct {
			y    int
			want rune
		}{{box.hinge - 1, '▄'}, {box.hinge + 1, '▀'}} {
			for _, dx := range []int{0, s.pinW - 1} {
				if got := at(c.y, dx); got != c.want {
					t.Errorf("width %d: the pin's corner at row %+d column %d is %q, "+
						"want %q, the rounding is a chamfer, not a nub",
						s.pinW, c.y-box.hinge, dx, got, c.want)
				}
			}
			for dx := 1; dx < s.pinW-1; dx++ {
				if got := at(c.y, dx); got != '█' {
					t.Errorf("width %d: the pin's own body at row %+d column %d is %q, "+
						"want a full block", s.pinW, c.y-box.hinge, dx, got)
				}
			}
		}

		// The stack is a short band level with the hinge, a little over a third of the
		// card's height, as in the reference. Sparseness comes from identical bars, and
		// the grain fixes that, not the height.
		if got := float64(s.combN) / float64(s.up+s.down+1); got < 0.2 || got > 0.45 {
			t.Errorf("width %d: %d bars against a card %d rows tall covers %.0f%% "+
				"of it; the reference band is a little under a third",
				s.pinW, s.combN, s.up+s.down+1, got*100)
		}
		if s.combW > 4 {
			t.Errorf("width %d: the comb bars are %d columns; long bars read as a "+
				"ladder bolted on rather than as the edges of paper", s.pinW, s.combW)
		}
	}
}

// The combs are level with the hinge by construction, so at the top the drawer
// shows itself through the dividers: the neighbouring letters' tabs stick up
// either side of this one, staggered by cardTabX into a nest.
func TestTheTopOfTheDrawerIsAStackOfDividers(t *testing.T) {
	m := drawer(t, 120, 40)
	r := m.rolodex()
	r.GoTo(r.Len() / 2) // well inside the alphabet, so there are letters both sides
	settleReel(t, m)

	box, ok := cardFits(m.Layout.Width, m.Layout.Height)
	if !ok {
		t.Fatal("the card does not fit")
	}
	e, _ := r.Selected()
	buf := drawTo(m)
	y := box.hinge - box.spec.up - 1

	// More than one tab on the cap row, either side of this card's own.
	row := string(buf.Row(y))
	if got := strings.Count(row, "╭"); got < 2 {
		t.Errorf("the cap row carries %d left corners; the dividers around this "+
			"one should be standing behind it: %q", got, strings.TrimSpace(row))
	}
	if got := strings.Count(row, "╮"); got < 2 {
		t.Errorf("the cap row carries %d right corners: %q", got, strings.TrimSpace(row))
	}

	// In the middle of the drawer there are cards behind on both sides, and the
	// count says where in it you are.
	if bf, af := r.Behind(cardStackDepth); bf == 0 || af == 0 {
		t.Errorf("in the middle of the drawer %d stand before this card and %d "+
			"after; both sides should have some", bf, af)
	}
	r.GoTo(0)
	settleReel(t, m)
	if bf, _ := r.Behind(cardStackDepth); bf != 0 {
		t.Errorf("%d cards stand before the first one in the drawer", bf)
	}

	// Drawn back, so the front tab stays the one being read.
	front := box.left() + cardTabX(box.spec, cardInitial(e.Name))
	behind := 0
	for x := range []rune(row) {
		if buf.AttrAt(x, y) != (canvas.Attr{}) && buf.AttrAt(x, y) != buf.AttrAt(front, y) {
			behind++
		}
	}
	if behind == 0 {
		t.Error("every tab on the row is drawn in the same shade; the ones behind " +
			"should recede")
	}

	// The drawer's top edge is under the card while it is flat; the tabs stand on
	// it once the card turns down and away from them.
	if got := []rune(buf.Row(y + 1))[box.left()]; got != '╭' {
		t.Errorf("the card's top-left corner is %q, not the card's own", got)
	}
	if buf.AttrAt(box.left(), y+1) != buf.AttrAt(box.left(), box.hinge+box.spec.down) {
		t.Error("the drawer's top edge is showing through the flat card; the card " +
			"in front should cover it exactly")
	}
}

// busyDrawer is a floor with three files worked by four, two and one refiners.
func busyDrawer(t *testing.T) *Model {
	t.Helper()
	rs := NewRooms(29)
	rs.EnsureOpen(DefaultRooms)
	for name, who := range map[string][]string{
		"Siena":    {"DYLAN G.", "MARK S.", "HELLY R.", "IRVING B."},
		"Minsk":    {"MARK S.", "HELLY R."},
		"Eminence": {"IRVING B."},
	} {
		room := rs.OpenNamed(name)
		for _, p := range who {
			room.Seat(p)
		}
	}
	m := NewLocalSession(rs, 1, "mark")
	m.OfferLAN(&fakeLAN{})
	m.Resize(120, 40)
	tick(m, 150)
	return m
}

// Unlike the letter tabs, where GoTo changes the card between two frames, this
// winds the drawer up, runs it through the cards and eases it onto the target.
// That is why the reel has momentum instead of being a spring to a number.
func TestTheReelFliesToADistantFile(t *testing.T) {
	m := drawer(t, 120, 40)
	r := m.rolodex()
	r.GoTo(0)
	settleReel(t, m)

	const to = 30
	if r.Len() <= to {
		t.Skipf("only %d files in the drawer", r.Len())
	}
	r.Seek(to)

	var top, mid, end float64
	seen := map[int]bool{}
	for f := 0; f < 400 && r.Turning(); f++ {
		tick(m, 1)
		v := math.Abs(r.at.Velocity())
		if v > top {
			top = v
		}
		if f == 20 {
			mid = v
		}
		end = v
		seen[int(math.Round(r.At()))] = true
	}

	if got := r.Want(); got != to {
		t.Errorf("the drawer was sent to %d and came to rest on %d", to, got)
	}
	if top > reelTop+0.01 {
		t.Errorf("it reached %v cards a second, past the %v cap where the cards "+
			"stop reading as a stack", top, reelTop)
	}
	if mid <= end {
		t.Errorf("it was going %v a fifth of the way in and %v at the end; it is "+
			"meant to be fast and then slow down as it arrives", mid, end)
	}
	// It went through the drawer, not straight to the card.
	if len(seen) < to/2 {
		t.Errorf("it passed through %d of the %d cards on the way; a flight is not "+
			"a jump", len(seen), to)
	}
}

func TestAHandOnTheWheelBeatsADestination(t *testing.T) {
	m := drawer(t, 120, 40)
	r := m.rolodex()
	r.GoTo(0)
	settleReel(t, m)

	r.Seek(30)
	tick(m, 10)
	if !r.Seeking() {
		t.Fatal("the flight was over before it could be interrupted")
	}

	r.Turn(-1)
	if r.Seeking() {
		t.Error("a shove did not cancel the destination")
	}
	settleReel(t, m)
	if got := r.Want(); got == 30 {
		t.Error("the drawer resumed travelling after the wheel was taken")
	}
	if r.Lift() > 0.5 {
		t.Errorf("the card is still standing out of the drawer at %v after the "+
			"wheel was taken", r.Lift())
	}
}

func TestPlusGoesToTheBusiestAndMinusComesBack(t *testing.T) {
	m := busyDrawer(t)
	r := m.rolodex()

	name := func() string { e, _ := r.Selected(); return e.Name }
	press := func(k string) string {
		m.Update(key(k))
		settleReel(t, m)
		return name()
	}

	// From anywhere, + climbs to the fullest file and stays there.
	r.GoTo(0)
	settleReel(t, m)
	for _, want := range []string{"Eminence", "Minsk", "Siena", "Siena"} {
		if got := press("+"); got != want {
			t.Fatalf("+ landed on %q, want %q", got, want)
		}
	}
	for _, want := range []string{"Minsk", "Eminence", "Eminence"} {
		if got := press("-"); got != want {
			t.Fatalf("- landed on %q, want %q", got, want)
		}
	}
}

// The contrast makes it read as one file picked out of a stack, not the screen
// sliding.
func TestThePresentedCardLiftsOutOfTheDrawer(t *testing.T) {
	m := busyDrawer(t)
	r := m.rolodex()

	// Where the combs are is a fact about the drawer, not about the card.
	box, ok := cardFits(m.Layout.Width, m.Layout.Height)
	if !ok {
		t.Fatal("the card does not fit")
	}
	comb := box.left() - box.spec.combGap - 1
	combRows := func() []int {
		buf := drawTo(m)
		var out []int
		for y := 0; y < m.Layout.Height; y++ {
			if []rune(buf.Row(y))[comb] == '▄' {
				out = append(out, y)
			}
		}
		return out
	}
	topEdge := func() int {
		buf := drawTo(m)
		for y := 0; y < m.Layout.Height; y++ {
			if strings.Contains(string(buf.Row(y)), "╯") {
				return y // the row the tab opens out of, i.e. the card's top edge
			}
		}
		return -1
	}

	restingCombs, restingTop := combRows(), topEdge()
	if restingTop < 0 {
		t.Fatal("no card on screen to begin with")
	}

	m.Update(key("+"))
	settleReel(t, m)
	tick(m, 60)
	if r.Lift() < 0.9 {
		t.Fatalf("the presented card is only %v out of the drawer", r.Lift())
	}

	if got := topEdge(); got >= restingTop {
		t.Errorf("the card's top edge is on row %d, was %d; it has not lifted",
			got, restingTop)
	}
	if got := combRows(); !equalInts(got, restingCombs) {
		t.Errorf("the comb moved from %v to %v; the drawer stays put while the "+
			"card comes out of it", restingCombs, got)
	}

	// And taking the wheel puts it back.
	m.Update(key("down"))
	tick(m, 90)
	if r.Lift() > 0.1 {
		t.Errorf("the card is still up at %v after the wheel was taken", r.Lift())
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Otherwise a keypress looks broken.
func TestAnEmptyFloorSaysSoRatherThanMoving(t *testing.T) {
	m := drawer(t, 120, 40)
	m.OfferLAN(&fakeLAN{})
	r := m.rolodex()
	if len(r.Busiest()) > 0 {
		t.Skip("somebody is refining something")
	}

	r.GoTo(7)
	settleReel(t, m)
	m.Update(key("+"))

	if got := r.Want(); got != 7 {
		t.Errorf("the drawer moved to %d with nobody on the floor", got)
	}
	if m.notice == "" {
		t.Error("pressing + on an empty floor said nothing at all")
	}
}

// riffleOf runs m's drawer through its riffle, frame by frame, and returns
// where it was on each.
func riffleOf(m *Model) []float64 {
	var at []float64
	for i := 0; i < 60*5 && m.rolodex().Riffling(); i++ {
		at = append(at, m.rolodex().At())
		tick(m, 1)
	}
	return append(at, m.rolodex().At())
}

// As the files first come up, the drawer riffles from its far end onto the
// first file, as the show's does as it fades in: fast at first, slowing onto
// the card, and sounding the cards it passes.
func TestTheFilesFirstComeUpRifflingOntoTheFirstFile(t *testing.T) {
	rs := NewRooms(17)
	rs.EnsureOpen(DefaultRooms)
	m := NewLocalSession(rs, 1, "mark")
	m.Resize(120, 40)
	r := m.rolodex()
	n := r.Len()
	if got := int(math.Round(r.At())); got != n-1 {
		t.Fatalf("the drawer came up on card %d, want the last, %d (%s)", got, n-1, r.Files()[n-1].Name)
	}
	if r.Level() > 0.1 {
		t.Error("the drawer was already lit as the riffle began")
	}

	cards := 0
	var at []float64
	for i := 0; i < 60*5 && r.Riffling(); i++ {
		at = append(at, r.At())
		for _, c := range cuesOf(m.UpdateEvent(FrameMsg(frameInterval))) {
			if c.Sound == SoundCard {
				cards++
			}
		}
	}
	if r.Riffling() {
		t.Fatal("the riffle never finished")
	}
	if e, _ := r.Selected(); r.Want() != 0 {
		t.Errorf("the riffle came to rest on %s, want the first file, %s", e.Name, r.Files()[0].Name)
	}
	if seconds := float64(len(at)) / 60; seconds > 4 {
		t.Errorf("the riffle took %.1fs", seconds)
	}

	// The last three cards go one at a time, each slower than the one before.
	var took [3]int
	for _, a := range at {
		if a > 0 && a < 3 {
			took[int(a)]++
		}
	}
	if !(took[2] < took[1] && took[1] < took[0]) {
		t.Errorf("the last three cards took %d, %d and %d frames; each should take longer than the one before", took[2], took[1], took[0])
	}
	for i := 1; i < len(at); i++ {
		if at[i] > at[i-1] {
			t.Fatalf("frame %d: the drawer turned back, from %.2f to %.2f", i, at[i-1], at[i])
		}
	}
	early, late := at[0]-at[10], at[len(at)-11]-at[len(at)-1]
	if early < 5*late {
		t.Errorf("the riffle passed %.1f cards in its first ten frames and %.1f in its last; it must start fast and slow down", early, late)
	}
	if cards < 10 {
		t.Errorf("only %d cards were heard flicking past", cards)
	}
	if r.Level() < 0.9 {
		t.Errorf("the drawer had come up only %.2f by the end of the riffle", r.Level())
	}
}

// Coming back to the files, the drawer riffles onto the file last opened, from
// whichever end of it is further.
func TestComingBackTheFilesRiffleOntoTheLastFileOpened(t *testing.T) {
	m := drawer(t, 120, 40)
	m.KeepOpen()
	r := m.rolodex()
	n := r.Len()
	for _, want := range []int{3, n - 4} {
		r.GoTo(want)
		tick(m, 10)
		name := r.Files()[want].Name
		m.Update(key("enter"))
		if m.FileName != name {
			t.Fatalf("took %s, landed on %s", name, m.FileName)
		}
		m.Update(key("q"))
		tick(m, 1)
		if m.Screen != ScreenLobby {
			t.Fatalf("q left the file for %v", m.Screen)
		}
		at := riffleOf(m)
		far := n - 1
		if want > n/2 {
			far = 0
		}
		// A frame in, the riffle's fast start has carried it a card or two.
		if got := int(math.Round(at[0])); math.Abs(float64(got-far)) > 3 {
			t.Errorf("back from %s the riffle was on card %d a frame in, want the far end, %d", name, got, far)
		}
		if e, _ := r.Selected(); e.Name != name {
			t.Errorf("back from %s the drawer came to rest on %s", name, e.Name)
		}
	}
}

// Turning the drawer mid-riffle takes it over from wherever the riffle had
// got to.
func TestTurningTheDrawerMidRiffleTakesItOver(t *testing.T) {
	rs := NewRooms(17)
	rs.EnsureOpen(DefaultRooms)
	m := NewLocalSession(rs, 1, "mark")
	m.Resize(120, 40)
	r := m.rolodex()
	tick(m, 20)
	was := r.Want()
	m.Update(key("down"))
	if r.Riffling() {
		t.Fatal("a key did not stop the riffle")
	}
	settleReel(t, m)
	if got := r.Want(); got == 0 || math.Abs(float64(got-was)) > 3 {
		t.Errorf("the drawer went from card %d to %d; a turn mid-riffle should carry on from there", was, got)
	}
}

// Taking a card while the drawer riffles takes the one it was coming to, not
// whichever was flicking past.
func TestTakingACardMidRiffleTakesTheOneItWasComingTo(t *testing.T) {
	rs := NewRooms(17)
	m := NewLocalSession(rs, 1, "mark")
	m.Resize(120, 40)
	r := m.rolodex()
	tick(m, 10)
	if !r.Riffling() {
		t.Fatal("the drawer was not riffling")
	}
	want := r.Files()[0].Name
	m.Update(key("enter"))
	if m.FileName != want {
		t.Errorf("enter mid-riffle took %q, want %q", m.FileName, want)
	}
}
