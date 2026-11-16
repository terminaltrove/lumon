package lumon

import (
	"time"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// The round trip to this terminal, shown as a traffic light in the footer, so
// a refiner can tell a slow network from a slow program.
//
// The samples come from cmd/lumon, which measures the connection. A local
// run has none, so no light is drawn.

// linkGood and linkFair are the boundaries between the three states, and
// linkBand is the dead band held either side of each.
const (
	linkGood = 40 * time.Millisecond
	linkFair = 120 * time.Millisecond
	linkBand = 8 * time.Millisecond
)

// linkWindow is how many samples the light is decided from. It takes their
// median: an average lets one outlier turn a steady link red
// (TestOneOutlierDoesNotMoveTheLight). At cmd/lumon's linkInterval of two
// seconds, five samples cover ten seconds.
const linkWindow = 5

// Link is the smoothed round trip to one terminal and which of the three states
// it is in. The state is held through a dead band (a Schmitt trigger, as in
// glyphOn), so a link on a boundary does not change colour every sample and
// resend a footer row.
type Link struct {
	// A ring of the last linkWindow samples. n counts arrivals up to the
	// window, so the light shows from the first sample.
	seen [linkWindow]time.Duration
	at   int
	n    int

	rtt  time.Duration
	band int
}

// Sample folds one measured round trip in.
func (l *Link) Sample(d time.Duration) {
	if d < 0 {
		return
	}
	l.seen[l.at] = d
	l.at = (l.at + 1) % linkWindow
	if l.n < linkWindow {
		l.n++
	}
	l.rtt = l.median()
	l.band = linkBandOf(l.rtt, l.band)
}

// median is the middle of what has been measured so far.
func (l *Link) median() time.Duration {
	var buf [linkWindow]time.Duration
	copy(buf[:], l.seen[:l.n])
	s := buf[:l.n]
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
	if len(s) == 0 {
		return 0
	}
	return s[len(s)/2]
}

// Live reports whether anything has been measured yet. A session that has not
// been told its round trip draws no light at all.
func (l *Link) Live() bool { return l != nil && l.n > 0 }

// Spread is the quickest and slowest of the samples still in the window. The
// median hides jitter, such as a radio in power save, so the light shows this
// when clicked.
func (l *Link) Spread() (lo, hi time.Duration) {
	if l.n == 0 {
		return 0, 0
	}
	lo, hi = l.seen[0], l.seen[0]
	for _, d := range l.seen[:l.n] {
		if d < lo {
			lo = d
		}
		if d > hi {
			hi = d
		}
	}
	return lo, hi
}

// Band is the state to draw: 0 good, 1 fair, 2 poor.
func (l *Link) Band() int { return l.band }

// RTT is the median round trip the light is showing.
func (l *Link) RTT() time.Duration { return l.rtt }

// LinkLamp is the light itself; the round trip is written beside it. It is
// exported for cmd/lumon's end-to-end test.
const LinkLamp = '●'

// linkRoom is how many columns the footer must have left over before the
// milliseconds are printed. On a narrow terminal the figure goes before the
// lamp.
const linkRoom = 32

// linkText is the round trip as written, cached so the footer does not
// allocate every frame (as watchLine does). It shows the median, so the number
// agrees with the lamp's colour.
func (m *Model) linkText() string {
	key := linkKey{ms: ms(m.link.RTT()), open: m.linkOpen}
	if key.open {
		lo, hi := m.link.Spread()
		key.lo, key.hi = ms(lo), ms(hi)
	}
	if m.linkAt == key && m.linkMs != "" {
		return m.linkMs
	}
	m.linkAt = key

	m.linkMs = itoa(key.ms) + "ms"
	if key.open {
		// The range the median came from, which shows jitter.
		m.linkMs += " " + itoa(key.lo) + "-" + itoa(key.hi)
	}
	return m.linkMs
}

// linkKey is what linkText was built from, so it is rebuilt when and only when
// one of those changes.
type linkKey struct {
	ms, lo, hi int
	open       bool
}

// ms is a duration in whole milliseconds.
func ms(d time.Duration) int {
	return int(d.Round(time.Millisecond) / time.Millisecond)
}

// linkSpan is where the light was last drawn, so a click can find it. It is
// recorded at draw time because only the draw knows how much room the footer
// had.
type linkSpan struct{ x0, x1, y int }

// clickedLink reports whether a press landed on the light, and opens or closes
// the spread if so. It is closed by default so the footer holds still.
func (m *Model) clickedLink(x, y int) bool {
	s := m.linkSpanned
	if !m.link.Live() || s.x1 < s.x0 || y != s.y || x < s.x0 || x > s.x1 {
		return false
	}
	m.linkOpen = !m.linkOpen
	return true
}

// drawLinkLight puts the lamp in the footer's right-hand corner and returns the
// column everything else in the footer must stay left of. With nothing
// measured it draws nothing and returns the screen's last column.
func (m *Model) drawLinkLight(buf *canvas.Buffer, y int) int {
	edge := m.Layout.Width - 1
	if !m.link.Live() || edge < 2 {
		return edge
	}
	// The figure goes right of the lamp, in the chrome colour, only when
	// linkRoom columns remain.
	x := edge - 1
	if txt := m.linkText(); x-len(txt)-1 >= linkRoom {
		buf.Text(x-len(txt)+1, y, txt, m.Theme.Chrome)
		x -= len(txt) + 1
	}
	buf.Set(x, y, LinkLamp, m.Theme.On(m.Theme.SignalColour(m.link.Band()), true))

	// The click target runs from the lamp to the edge, figure included, since
	// one character is a small target.
	m.linkSpanned = linkSpan{x0: x, x1: edge - 1, y: y}

	// Leave a blank column before the rest of the footer.
	return x - 1
}

// linkBandOf places a round trip in a band, holding the band it is already in
// through linkBand either side of each boundary.
func linkBandOf(d time.Duration, current int) int {
	good, fair := linkGood, linkFair
	// Widen whichever boundary must be crossed to leave the current state.
	switch current {
	case 0:
		good += linkBand
	case 1:
		good -= linkBand
		fair += linkBand
	default:
		fair -= linkBand
	}
	switch {
	case d < good:
		return 0
	case d < fair:
		return 1
	default:
		return 2
	}
}
