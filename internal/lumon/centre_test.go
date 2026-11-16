package lumon

import (
	"strings"
	"testing"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// lobbyAt is a drawer at a size, in a view, drawn.
func lobbyAt(t *testing.T, w, h int, view lobbyView) (*Model, *canvas.Buffer) {
	t.Helper()
	m, _ := lanLobby(t)
	m.Resize(w, h)
	m.lobbyView = view
	tick(m, 120) // the drawer fades up
	riffled(m)
	buf := canvas.NewBuffer(w, h)
	m.Draw(buf)
	return m, buf
}

func TestTheFilesNoticeIsBetweenTheDrawerAndTheControls(t *testing.T) {
	const say = "nobody is refining anything just now"
	for _, size := range [][2]int{{200, 60}, {120, 40}, {100, 30}, {80, 24}} {
		w, h := size[0], size[1]

		for view, last := range map[lobbyView]func(m *Model) int{
			viewRolodex: func(m *Model) int {
				box, _ := cardFits(w, h)
				return box.hinge + box.spec.down + 1
			},
			viewList: func(m *Model) int {
				first, visible := m.lobbyWindow(h, m.rolodex().Len(), m.rolodex().Want())
				_ = first
				return lobbyListTop(h, visible) + lobbyHeadRows + visible - 1
			},
			viewGrid: func(m *Model) int {
				cols, n := m.gridCols(), m.rolodex().Len()
				return lobbyGridTop(h, n, cols) + lobbyGridRows(h, n, cols)*lobbyTileH - 2
			},
		} {
			m, _ := lobbyAt(t, w, h, view)
			m.notice = say
			buf := canvas.NewBuffer(w, h)
			m.Draw(buf)
			x, y := findText(t, buf, say)

			if want := (w - len(say)) / 2; x != want {
				t.Errorf("%dx%d view %d: notice at column %d, want %d", w, h, view, x, want)
			}
			above, below := y-last(m), h-2-y
			if above < 1 || below < 1 || above-below > 1 || below-above > 1 {
				t.Errorf("%dx%d view %d: notice has %d rows above and %d below", w, h, view, above, below)
			}
		}
	}
}

func TestTheFileListIsCentred(t *testing.T) {
	m, buf := lobbyAt(t, 200, 60, viewList)
	x, y := findText(t, buf, "FILE")
	const block = lobbyListNameW + 10 + 8 + lobbyListWhoW
	if want := (200 - block) / 2; x != want {
		t.Errorf("the list starts at column %d, want %d", x, want)
	}
	_, visible := m.lobbyWindow(60, m.rolodex().Len(), m.rolodex().Want())
	above := y - lobbyTop
	below := 60 - lobbyBottom - (y + lobbyHeadRows + visible)
	if above-below > 1 || below-above > 1 {
		t.Errorf("the list has %d rows above and %d below", above, below)
	}
}

// With no room to spare it hugs the margin and the top, as it always did.
func TestATightFileListKeepsItsPlace(t *testing.T) {
	_, buf := lobbyAt(t, 60, 20, viewList)
	x, y := findText(t, buf, "FILE")
	if x != 4 || y != lobbyTop {
		t.Errorf("the list starts at %d,%d", x, y)
	}
}

func TestTheFileGridIsCentredVertically(t *testing.T) {
	m, _ := lobbyAt(t, 200, 60, viewGrid)
	cols, n := m.gridCols(), m.rolodex().Len()
	rows := lobbyGridRows(60, n, cols)
	top := lobbyGridTop(60, n, cols)
	if rows*lobbyTileH >= lobbyContentRows(60) {
		t.Fatalf("setup: %d rows of tiles fill the screen", rows)
	}
	above := top - lobbyTop
	below := 60 - lobbyBottom - (top + rows*lobbyTileH - 1)
	if above-below > 1 || below-above > 1 {
		t.Errorf("the grid has %d rows above and %d below", above, below)
	}
}

// Paging through a grid bigger than the screen must not make it jump.
func TestTheGridDoesNotMoveBetweenPages(t *testing.T) {
	m, _ := lobbyAt(t, 80, 24, viewGrid)
	r := m.rolodex()
	top := lobbyGridTop(24, r.Len(), m.gridCols())
	if top != lobbyTop+maxi((lobbyContentRows(24)-(lobbyContentRows(24)/lobbyTileH)*lobbyTileH+1)/2, 0) {
		t.Errorf("a full grid starts at row %d", top)
	}
	if r.Len() <= lobbyContentRows(24)/lobbyTileH*m.gridCols() {
		t.Skip("everything fits on one page here")
	}
	r.GoTo(r.Len() - 1)
	buf := canvas.NewBuffer(80, 24)
	m.Draw(buf)
	if got := lobbyGridTop(24, r.Len(), m.gridCols()); got != top {
		t.Errorf("the last page starts at %d, the first at %d", got, top)
	}
}

// A click goes through the same sums as the drawing, whatever the size.
func TestAClickLandsOnTheFileDrawn(t *testing.T) {
	for _, size := range [][2]int{{200, 60}, {120, 40}, {100, 30}, {80, 24}, {60, 20}} {
		w, h := size[0], size[1]

		m, buf := lobbyAt(t, w, h, viewList)
		r := m.rolodex()
		first, visible := m.lobbyWindow(h, r.Len(), r.Want())
		for i := first; i < first+visible; i++ {
			_, y := findText(t, buf, r.Files()[i].Name)
			if got, ok := m.lobbyRowAt(y, r); !ok || got != i {
				t.Errorf("%dx%d list: row %d resolved to %d, %v, want file %d", w, h, y, got, ok, i)
			}
		}
		top := lobbyListTop(h, visible) + lobbyHeadRows
		if _, ok := m.lobbyRowAt(top-1, r); ok {
			t.Errorf("%dx%d list: the row above the first file took a click", w, h)
		}
		if _, ok := m.lobbyRowAt(top+visible, r); ok && visible < r.Len() {
			t.Errorf("%dx%d list: the row below the last file took a click", w, h)
		}

		m, buf = lobbyAt(t, w, h, viewGrid)
		r = m.rolodex()
		cols := m.gridCols()
		perPage := lobbyContentRows(h) / lobbyTileH * cols
		page := r.Want() / perPage * perPage
		for i := page; i < min(page+perPage, r.Len()); i++ {
			x, y := findText(t, buf, r.Files()[i].Name)
			if got, ok := m.lobbyTileAt(x, y, r); !ok || got != i {
				t.Errorf("%dx%d grid: %d,%d resolved to %d, %v, want file %d", w, h, x, y, got, ok, i)
			}
		}
		if _, ok := m.lobbyTileAt((w)/2, lobbyGridTop(h, r.Len(), cols)-1, r); ok {
			t.Errorf("%dx%d grid: the row above the first tile took a click", w, h)
		}
	}
}

func TestTheLANMenuIsInTheMiddleOfItsBox(t *testing.T) {
	m, _ := lanLobby(t)
	m.Resize(120, 40)
	m.openLAN()

	draw := func() *canvas.Buffer {
		buf := canvas.NewBuffer(120, 40)
		m.drawLAN(buf)
		return buf
	}

	buf := draw()
	hx, hy := findText(t, buf, "Host a LAN game")
	jx, jy := findText(t, buf, "Join a LAN game")
	box := modal{X: (120 - 68) / 2, Y: (40 - 20) / 2, W: 68, H: 20}

	// The words themselves are centred, not the words and their arrow.
	if want := box.X + (box.W-len("Host a LAN game"))/2; hx != want || jx != want {
		t.Errorf("the choices are at columns %d and %d, want %d", hx, jx, want)
	}
	// Midway between the title and the hint.
	above := hy - box.Y - 1
	below := box.Y + box.H - 3 - jy
	if above-below > 1 || below-above > 1 {
		t.Errorf("the choices have %d rows above and %d below", above, below)
	}

	// Moving the selection moves only the arrow.
	if got, _ := findText(t, buf, "> Host"); got != hx-2 {
		t.Errorf("the arrow is at %d, the words at %d", got, hx)
	}
	lanPress(m, "down")
	buf = draw()
	if x, _ := findText(t, buf, "Host a LAN game"); x != hx {
		t.Errorf("selecting moved HOST from %d to %d", hx, x)
	}
	if x, _ := findText(t, buf, "Join a LAN game"); x != jx {
		t.Errorf("selecting moved JOIN from %d to %d", jx, x)
	}
	if got, _ := findText(t, buf, "> Join"); got != jx-2 {
		t.Errorf("the arrow is at %d, the words at %d", got, jx)
	}
}

// Every step of the panel keeps inside its frame and asks in lower case.
func TestEveryLANStepFitsItsBox(t *testing.T) {
	setups := map[string]func(m *Model, lan *fakeLAN){
		"menu":    func(m *Model, _ *fakeLAN) { m.lanUI.step = lanMenu },
		"finding": func(m *Model, _ *fakeLAN) { m.lanUI.step = lanFinding },
		"address": func(m *Model, _ *fakeLAN) { m.lanUI.step = lanTypingAddr },
		"name":    func(m *Model, _ *fakeLAN) { m.lanUI.step = lanTypingName },
		"hosting": func(m *Model, lan *fakeLAN) {
			lan.hosting = true
			for range 8 {
				lan.players = append(lan.players, LANPlayer{Name: "JOE", Addr: "192.168.1.12"})
			}
			m.lanUI.step = lanHosted
		},
	}
	for name, set := range setups {
		for _, size := range [][2]int{{120, 40}, {80, 24}, {60, 14}, {26, 10}} {
			w, h := size[0], size[1]
			m, lan := lanLobby(t)
			m.Resize(w, h)
			set(m, lan)
			m.lanUI.note = ""

			buf := canvas.NewBuffer(w, h)
			m.drawLAN(buf)

			// The frame's bottom edge is unbroken, and the hint is on the row above.
			boxW, boxH := min(68, w-2), min(20, h-2)
			boxX, boxY := (w-boxW)/2, (h-boxH)/2
			bottom := []rune(buf.Row(boxY + boxH - 1))
			for x := boxX + 1; x < boxX+boxW-1; x++ {
				if bottom[x] != '─' {
					t.Fatalf("%s at %dx%d: the bottom edge is %q", name, w, h, string(bottom))
				}
			}
			hint := buf.Row(boxY + boxH - 2)
			if hint != strings.ToLower(hint) {
				t.Errorf("%s at %dx%d: the hint is %q", name, w, h, hint)
			}
		}
	}
}
