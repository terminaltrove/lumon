package lumon

import (
	"strings"
	"testing"
	"time"
)

// footerRow is the drawn footer, as characters.
func footerRow(t *testing.T, m *Model) string {
	t.Helper()
	return drawTo(m).Row(m.Layout.FooterY())
}

// footerRunes is the same row as cells, since the lamp is not one byte wide.
func footerRunes(t *testing.T, m *Model) []rune {
	t.Helper()
	return []rune(footerRow(t, m))
}

// With no samples the footer is unchanged character for character, which other
// footer tests rely on.
func TestALocalRunHasNoLinkLight(t *testing.T) {
	m := ready()
	tick(m, 30)
	row := footerRow(t, m)

	for _, r := range row {
		if r == LinkLamp {
			t.Fatalf("a local run drew a link light: %q", row)
		}
	}
	if m.link.Live() {
		t.Error("a local run believes it has measured its round trip")
	}
	// Nothing is reserved for it: the footer's right-hand boundary is unchanged.
	if got := m.drawLinkLight(drawTo(m), 0); got != m.Layout.Width-1 {
		t.Errorf("an unmeasured session reserved room for a light: boundary %d, want %d",
			got, m.Layout.Width-1)
	}
}

func TestTheLightSaysWhichBandTheLinkIsIn(t *testing.T) {
	for _, c := range []struct {
		name string
		rtt  time.Duration
		band int
	}{
		{"a quick link", 5 * time.Millisecond, 0},
		{"a fair one", 70 * time.Millisecond, 1},
		{"a poor one", 400 * time.Millisecond, 2},
	} {
		m := ready()
		m.Update(LinkMsg(c.rtt))
		tick(m, 2)

		if got := m.link.Band(); got != c.band {
			t.Errorf("%s: band %d, want %d", c.name, got, c.band)
		}

		buf := drawTo(m)
		y := m.Layout.FooterY()
		x := strings.IndexRune(buf.Row(y), LinkLamp)
		if x < 0 {
			t.Errorf("%s: no lamp in the footer", c.name)
			continue
		}
		x = len([]rune(buf.Row(y)[:x]))
		if got, want := buf.AttrAt(x, y).FG, m.Theme.SignalColour(c.band); got != want {
			t.Errorf("%s: the lamp is the wrong colour", c.name)
		}
	}
}

// Every change to the drawn cell is a row on the wire, for every session on the
// server. This is the same Schmitt trigger glyphOn applies.
func TestTheLightHoldsItsColourThroughTheBand(t *testing.T) {
	// Just inside good, then just past the boundary but within the dead band. A
	// link settled at the first must not change state at the second.
	settled := linkGood - 2*time.Millisecond
	nudged := linkGood + linkBand/2

	var l Link
	for i := 0; i < 40; i++ {
		l.Sample(settled)
	}
	if l.Band() != 0 {
		t.Fatalf("a link at %v did not settle in the good band", settled)
	}
	for i := 0; i < 40; i++ {
		l.Sample(nudged)
	}
	if got := l.Band(); got != 0 {
		t.Errorf("a link nudged %v past the boundary changed band to %d, inside the dead band",
			nudged-linkGood, got)
	}

	// Well past it, it must change, or it is a stuck light.
	for i := 0; i < 40; i++ {
		l.Sample(linkGood + 4*linkBand)
	}
	if got := l.Band(); got != 1 {
		t.Errorf("a link well past the boundary stayed in band %d", got)
	}
}

// Smoothed from zero, a bad link would show green for the first several
// samples, the moment somebody wondering about their connection looks.
func TestTheFirstSampleIsNotSmoothedFromNothing(t *testing.T) {
	var l Link
	l.Sample(300 * time.Millisecond)
	if got := l.RTT(); got != 300*time.Millisecond {
		t.Errorf("the first sample came out as %v, want it taken as measured", got)
	}
	if l.Band() != 2 {
		t.Error("a first sample of 300ms did not read as a poor link")
	}
}

func TestOneOutlierDoesNotMoveTheLight(t *testing.T) {
	var l Link
	for i := 0; i < 40; i++ {
		l.Sample(8 * time.Millisecond)
	}
	before := l.Band()
	l.Sample(900 * time.Millisecond)
	if got := l.Band(); got != before {
		t.Errorf("a single outlier moved a settled link from band %d to %d", before, got)
	}
}

// Bubble Tea sends only the rows that changed, so a footer cell that changes on
// every sample costs every session bandwidth. This is why the milliseconds are
// not on screen; it is asserted against the rendered row.
func TestTheLinkLightIsStillWhileTheLinkIs(t *testing.T) {
	m := ready()
	m.Update(LinkMsg(70 * time.Millisecond))
	tick(m, 10)
	want := footerRow(t, m)

	for i := 0; i < 300; i++ {
		// A steady link, sampled as often as one would be.
		if i%120 == 0 {
			m.Update(LinkMsg(70 * time.Millisecond))
		}
		tick(m, 1)
		if got := footerRow(t, m); got != want {
			t.Fatalf("the footer changed on frame %d with the link steady:\n  was %q\n  now %q",
				i, want, got)
		}
	}
}

// The light takes columns from the footer's other contents and is never drawn
// over them.
func TestTheLightNeverCostsTheRosterItsNameplates(t *testing.T) {
	_, a, b := twoInARoom(t)
	a.Resize(120, 40)
	b.Resize(120, 40)
	tick(a, 5)
	tick(b, 5)

	plain := footerRow(t, a)
	a.Update(LinkMsg(70 * time.Millisecond))
	tick(a, 2)
	lit := footerRow(t, a)

	if plain == lit {
		t.Fatal("measuring the link changed nothing in the footer")
	}

	// The nameplates all still say what they said, just further left.
	for _, d := range a.Roster() {
		if !strings.Contains(lit, d.Name) {
			t.Errorf("%s lost their nameplate to the link light:\n  %q", d.Name, lit)
		}
	}

	// The lamp leads and the figure follows it, ending at the right edge.
	runes := []rune(lit)
	txt := a.linkText()
	at := a.Layout.Width - 1 - len(txt)
	if got := string(runes[at : at+len(txt)]); got != txt {
		t.Errorf("the corner reads %q, want %q at the edge:\n  %q", got, txt, lit)
	}
	if r := runes[at-2]; r != LinkLamp {
		t.Errorf("the lamp does not lead the figure, found %q:\n  %q", r, lit)
	}
}

func TestTheFooterSaysTheRoundTrip(t *testing.T) {
	m := ready()
	m.Update(LinkMsg(63 * time.Millisecond))
	tick(m, 2)

	if got := footerRow(t, m); !strings.Contains(got, "63ms") {
		t.Errorf("the footer does not say the round trip:\n  %q", got)
	}

	// It follows the link. A second reading moves the median, which the lamp is
	// also lit from, so the two cannot disagree.
	for i := 0; i < 6; i++ {
		m.Update(LinkMsg(140 * time.Millisecond))
	}
	tick(m, 2)
	if got := footerRow(t, m); strings.Contains(got, "63ms") {
		t.Errorf("the footer still reports the old round trip:\n  %q", got)
	}
}

// Somebody who cannot see the number can still act on the colour. The file bar
// drops its own lanes in the same order.
func TestTheNumberGivesWayBeforeTheLamp(t *testing.T) {
	m := ready()
	m.Resize(30, 24)
	m.Update(LinkMsg(70 * time.Millisecond))
	tick(m, 2)

	row := footerRow(t, m)
	if !strings.ContainsRune(row, LinkLamp) {
		t.Errorf("a narrow terminal lost the lamp as well:\n  %q", row)
	}
	if strings.Contains(row, "70ms") {
		t.Errorf("a thirty column terminal printed the figure anyway:\n  %q", row)
	}
}

// trimRight drops trailing blanks.
func trimRight(s string) string {
	r := []rune(s)
	for len(r) > 0 && r[len(r)-1] == ' ' {
		r = r[:len(r)-1]
	}
	return string(r)
}

// The two are laid out independently, the readout centred and the banner
// against the right, so the banner's guard compares its left edge with where
// the readout ends. At 80 columns, the commonest size, a wrong guard draws
// "0xFFC0CE00 : 0xFFCLUMON INDUSTRIES".
func TestTheBannerNeverPrintsOverTheAddress(t *testing.T) {
	const banner = "LUMON INDUSTRIES"
	for w := 40; w <= 200; w++ {
		for _, lit := range []bool{false, true} {
			m := ready()
			m.Resize(w, 40)
			if lit {
				m.Update(LinkMsg(70 * time.Millisecond))
			}
			tick(m, 3)
			row := footerRow(t, m)

			// The readout is drawn whole or not at all, so a partial one means something
			// was written across it.
			addr := strings.Index(row, "0xFF")
			if addr < 0 {
				continue
			}
			if !strings.Contains(row, banner) {
				continue // the banner gave way, which is the intended answer
			}
			// Both are up. They must not touch.
			runes := []rune(row)
			at := len([]rune(row[:addr]))
			tail := string(runes[at:])
			if i := strings.Index(tail, banner); i >= 0 && i < len("0xFFC0CE00 : 0xFFC3FB00")+1 {
				t.Errorf("at %d columns (light=%v) the banner starts %d in from the readout:\n  %q",
					w, lit, i, row)
			}
		}
	}
}

// litLink is a session whose link has been measured with a known spread.
func litLink(t *testing.T) *Model {
	t.Helper()
	m := ready()
	m.Resize(120, 40)
	for _, d := range []int{63, 150, 90, 107, 97} {
		m.Update(LinkMsg(time.Duration(d) * time.Millisecond))
	}
	tick(m, 2)
	drawTo(m) // so the light has recorded where it is
	return m
}

// A steady link and one swinging between two extremes can report the same
// median: a radio dropping in and out of power save reads as an ordinary
// latency until you see the range.
func TestClickingTheLightOpensTheSpread(t *testing.T) {
	m := litLink(t)

	if got := footerRow(t, m); strings.Contains(got, "63-150") {
		t.Fatalf("the spread was showing before anybody asked:\n  %q", got)
	}

	s := m.linkSpanned
	m.Update(click(s.x0, s.y))
	tick(m, 1)

	if got := footerRow(t, m); !strings.Contains(got, "63-150") {
		t.Errorf("clicking the light did not open the spread:\n  %q", got)
	}

	// It closes again.
	m.Update(click(s.x0, s.y))
	tick(m, 1)
	if got := footerRow(t, m); strings.Contains(got, "63-150") {
		t.Errorf("clicking it again did not close it:\n  %q", got)
	}
}

// The lamp is one character and the board is clicked on cells six columns wide,
// so the lamp alone would be a poor target.
func TestTheWholeReadoutIsTheTarget(t *testing.T) {
	m := litLink(t)
	s := m.linkSpanned
	if s.x1 <= s.x0 {
		t.Fatal("the figure did not draw, so there is no target to test")
	}
	for x := s.x0; x <= s.x1; x++ {
		m.linkOpen = false
		m.Update(click(x, s.y))
		if !m.linkOpen {
			t.Errorf("a click at column %d of the readout (%d..%d) did nothing", x, s.x0, s.x1)
		}
	}
}

func TestAClickBesideTheLightIsNotAClickOnIt(t *testing.T) {
	m := litLink(t)
	s := m.linkSpanned

	m.Update(click(s.x0-1, s.y))
	if m.linkOpen {
		t.Error("a click one column left of the readout opened it")
	}

	// A click on the board still refines: the guard returns early only for the
	// light.
	before := m.CursorX
	m.Update(click(20, m.Layout.GridTop+2))
	if m.CursorX == before && m.CursorY == 0 {
		t.Error("a click on the grid stopped moving the cursor")
	}
}

func TestALocalRunHasNothingToClick(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	tick(m, 2)
	drawTo(m)

	m.Update(click(m.Layout.Width-2, m.Layout.FooterY()))
	if m.linkOpen {
		t.Error("a local run opened a spread it has never measured")
	}
}
