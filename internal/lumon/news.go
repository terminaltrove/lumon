package lumon

import (
	"strings"
	"sync"
	"time"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// floorNews holds the latest event for every room on one server. It is a leaf
// lock: a room or the ledger may post while holding its own lock, and reading a
// headline never calls back into either of them.
type floorNews struct {
	mu   sync.RWMutex
	line string
	at   time.Time

	// who is the refiner an arrival is about, and seat their seat, so the name
	// can be drawn in their colour. seat is -1 for any other news.
	who  string
	seat int
}

// The news is not always up. An event shows for newsHold after it happens,
// which includes your own arrival on the floor, and a Lumon notice comes up for
// noticeShow seconds in every noticeEvery. The rest of the time the rule shows.
const (
	newsHold    = 30 * time.Second
	noticeEvery = 120
	noticeShow  = 30
)

var standingNotices = [...]string{
	"THE WORK IS MYSTERIOUS AND IMPORTANT",
	"YOUR CONTRIBUTION TO THE FLOOR IS RECOGNISED",
	"PLEASE ENJOY EACH FILE EQUALLY",
	"LUMON THANKS YOU FOR YOUR ATTENTION",
}

// newsPrefix and noticePrefix start the two kinds of line.
const (
	newsPrefix   = "FLOOR NEWS · "
	noticePrefix = "LUMON NOTICE · "
)

func (n *floorNews) Post(line string) { n.post(line, "", -1) }

// PostArrival is somebody sitting down at a file.
func (n *floorNews) PostArrival(who, file string, seat int) {
	n.post(who+" JOINED "+file, who, seat)
}

func (n *floorNews) post(line, who string, seat int) {
	if n == nil || line == "" {
		return
	}
	n.mu.Lock()
	n.line, n.at, n.who, n.seat = strings.ToUpper(line), time.Now(), strings.ToUpper(who), seat
	n.mu.Unlock()
}

// Line is what the news says now, or nothing.
func (n *floorNews) Line(now time.Time) string {
	line, _, _ := n.show(now)
	return line
}

// show is the line, and for an arrival who it is about and their seat.
func (n *floorNews) show(now time.Time) (line, who string, seat int) {
	if n == nil {
		return "", "", -1
	}
	n.mu.RLock()
	line, at, who, seat := n.line, n.at, n.who, n.seat
	n.mu.RUnlock()
	if age := now.Sub(at); line != "" && age >= 0 && age < newsHold {
		return newsPrefix + line, who, seat
	}
	if now.Unix()%noticeEvery < noticeShow {
		return noticePrefix + standingNotices[(now.Unix()/noticeEvery)%int64(len(standingNotices))], "", -1
	}
	return "", "", -1
}

// statusRow is where the board says things: the row between the two lines of
// the rule over the grid (see TopRuleHeight). A LAN request is drawn later on
// the same row, covering the news until it is answered. Any screen without the
// rule uses the top of the terminal.
func (m *Model) statusRow() int {
	if m.Screen == ScreenMain && m.Room != nil && (m.mark == nil || !m.mark.Fullscreen()) {
		return m.Layout.GridTop - 2
	}
	return 0
}

// drawFloorNews puts the news in the status row, in plain text on the page, while
// there is any (see floorNews.Line). Only a LAN request is a bright bar, so it
// stands out from the news. n hides the news, but never a request.
func (m *Model) drawFloorNews(buf *canvas.Buffer) {
	if m.rooms == nil || m.newsHidden || m.Layout.Width < 24 {
		return
	}
	line, who, seat := m.rooms.news.show(time.Now())
	if line == "" {
		return
	}
	line = clipTo(line, m.Layout.Width-2)
	m.drawStatusBar(buf, line, m.newsAttr())

	// Somebody arriving is named in their seat's colour, as on their nameplate,
	// if the whole name is on screen.
	if who == "" || seat < 0 || !strings.HasPrefix(line, newsPrefix+who+" ") {
		return
	}
	x := (m.Layout.Width-len([]rune(line)))/2 + len([]rune(newsPrefix))
	buf.Text(x, m.statusRow(), who, canvas.Attr{FG: m.Theme.DeskColour(seat), BG: m.Theme.Page.BG})
}

// newsAttr is the rules' colour, so the news sits in the rule as part of it.
func (m *Model) newsAttr() canvas.Attr {
	return canvas.Attr{FG: m.Theme.Chrome.FG, BG: m.Theme.Page.BG}
}

// drawStatusBar fills the status row and centres a line on it.
func (m *Model) drawStatusBar(buf *canvas.Buffer, s string, a canvas.Attr) {
	w, _ := buf.Size()
	y := m.statusRow()
	for x := range w {
		buf.Set(x, y, ' ', a)
	}
	buf.Text((w-len([]rune(s)))/2, y, s, a)
}

var callouts = [...]string{
	"PRAISE KIER",
	"WELLNESS CHECK",
	"THE WORK IS MYSTERIOUS",
	"YOU ARE APPRECIATED",
	"REFINE AND REJOICE",
	"WOE, FROLIC, DREAD, MALICE",
	"THESE NUMBERS ARE SCARY",
	"FINGER TRAPS FOR ALL",
	"MELON BAR?",
	"EGG BAR?",
	"MUSIC DANCE EXPERIENCE",
	"DEFIANT JAZZ",
	"VERVE AND WIT",
	"HOW IS YOUR OUTIE?",
	"QUOTA IN SIGHT",
	"ONE MORE BIN",
	"NICE REFINING",
}

const calloutHold = 6.0 // seconds of shared board time

// sayCallout publishes a fixed phrase on this refiner's desk, where every view
// of the room can read it. A watcher has no desk and cannot speak for one.
func (m *Model) sayCallout() {
	if m.Room == nil || m.Desk == nil {
		return
	}
	m.Desk.Callout = callouts[m.calloutAt]
	m.Desk.CalloutUntil = m.Room.stepped + calloutHold
}

func (m *Model) drawCalloutMenu(buf *canvas.Buffer) {
	box, ok := m.modalBox(buf, 42, len(callouts)+5, "LUMON PHRASES")
	if !ok {
		return
	}
	visible := box.H - 4 // title, one spacer, hint, and lower border
	if visible > len(callouts) {
		visible = len(callouts)
	}
	start := m.calloutAt - visible + 1
	if start < 0 {
		start = 0
	}
	for i := start; i < start+visible; i++ {
		line := callouts[i]
		attr, prefix := m.Theme.Footer, "  "
		if i == m.calloutAt {
			attr, prefix = m.Theme.BinOn, "> "
		}
		m.modalCentre(buf, box, box.Y+2+i-start, prefix+line, attr)
	}
	m.modalCentre(buf, box, box.Y+box.H-2, "↑↓ choose · enter say · q back", m.Theme.Footer)
}

// drawCallout places a refiner's words above their cursor for everyone who can
// see that cursor. The board clock expires it for every view at the same time.
func (m *Model) drawCallout(buf *canvas.Buffer, d *Desk, cx, cy int) {
	if d.Callout == "" || d.CalloutUntil <= m.Room.stepped {
		return
	}
	l := m.Layout
	y := cy - 2
	if y < l.GridTop {
		y = cy + 2
	}
	if y >= l.BoardY+l.BoardH {
		return
	}
	line := clipTo(" "+d.Callout+" ", l.Width)
	x := cx - len([]rune(line))/2
	if x < 0 {
		x = 0
	}
	if x+len([]rune(line)) > l.Width {
		x = l.Width - len([]rune(line))
	}
	buf.Text(x, y, line, m.Theme.DeskCursor(d.Seat))
}
