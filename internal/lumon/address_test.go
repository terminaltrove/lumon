package lumon

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The readout is the range of memory the visible block occupies, so it changes
// when the view scrolls under a stationary cursor.
func TestTheAddressMovesWithTheView(t *testing.T) {
	m := ready()
	m.Resize(120, 34)

	seen := map[string]bool{}
	last := m.addressLine()
	seen[last] = true

	for i := 0; i < 6; i++ {
		before := m.Layout.ViewportY
		for k := 0; k < m.Layout.ViewportH; k++ {
			m.Update(key("down"))
		}
		if m.Layout.ViewportY == before {
			t.Fatal("the view never scrolled")
		}
		got := m.addressLine()
		if got == last {
			t.Errorf("the view scrolled from row %d to %d and the address stayed %s",
				before, m.Layout.ViewportY, got)
		}
		seen[got] = true
		last = got
	}
	if len(seen) < 5 {
		t.Errorf("six scrolls produced only %d distinct addresses", len(seen))
	}
}

// One half of the readout tracks the cursor, so it moves without a scroll.
func TestTheAddressFollowsTheCursor(t *testing.T) {
	m := ready()
	m.Resize(120, 34)

	seen := map[string]bool{}
	last := m.addressLine()

	// Well inside the viewport, so nothing scrolls and only the cursor moves.
	for i := 0; i < 6; i++ {
		m.Update(key("right"))
		if m.Layout.ViewportX != 0 || m.Layout.ViewportY != 0 {
			t.Fatal("the view scrolled; this is meant to test the cursor alone")
		}
		got := m.addressLine()
		if got == last {
			t.Errorf("the cursor moved to (%d,%d) and the address stayed %s",
				m.CursorX, m.CursorY, got)
		}
		seen[got] = true
		last = got
	}
	if len(seen) < 6 {
		t.Errorf("six cursor moves produced only %d distinct addresses", len(seen))
	}
}

func TestTwoRefinersAtOnePlaceReadTheSameAddress(t *testing.T) {
	_, a, b := twoInARoom(t)
	a.Resize(120, 34)
	b.Resize(96, 28) // a different terminal entirely

	a.Layout.ScrollTo(12, 7)
	b.Layout.ScrollTo(12, 7)

	sa, _ := AddressRange(a.Layout, 12, 7, a.Room.Epoch())
	sb, _ := AddressRange(b.Layout, 12, 7, b.Room.Epoch())
	if sa != sb {
		t.Errorf("two refiners at cell (12,7) read %08X and %08X", sa, sb)
	}
}

// Otherwise the readout would show the same numbers again.
func TestANewFileIsMappedSomewhereElse(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	before := m.addressLine()

	completeTheFile(m)
	tick(m, int(FileHoldSeconds*60)+10)

	if got := m.addressLine(); got == before {
		t.Errorf("a new file is mapped at the same address, %s", got)
	}
}

// Fixed width, so the readout does not jitter as the refiner moves and the two
// halves stay aligned under each other.
func TestTheAddressNeverChangesWidth(t *testing.T) {
	m := ready()
	m.Resize(120, 34)

	want := len(m.addressLine())
	for i := 0; i < 40; i++ {
		for k := 0; k < 5; k++ {
			m.Update(key("down"))
			m.Update(key("right"))
		}
		if got := len(m.addressLine()); got != want {
			t.Fatalf("the readout became %d characters: %q", got, m.addressLine())
		}
	}

	// The show's format: 0x, eight digits, uppercase.
	line := m.addressLine()
	half, _, ok := strings.Cut(line, " : ")
	if !ok {
		t.Fatalf("the readout is not a pair: %q", line)
	}
	if len(half) != 10 || !strings.HasPrefix(half, "0x") {
		t.Errorf("half the readout is %q, want 0x and eight digits", half)
	}
	if strings.ToUpper(half[2:]) != half[2:] {
		t.Errorf("the digits are not uppercase: %q", half)
	}
}

// You can work without knowing where in memory you are. The hint has its own
// row, so the readout gives way to the width alone and the keys are shown at
// every size.
func TestTheAddressIsDrawnAndGivesWayOnANarrowTerminal(t *testing.T) {
	m := ready()
	m.Resize(140, 34)

	top, hint := m.Layout.FooterY(), m.Layout.HintY()
	if row := drawTo(m).Row(top); !strings.Contains(row, m.addressLine()) {
		t.Errorf("the readout is not in the footer:\n%q", row)
	}
	if row := drawTo(m).Row(hint); !strings.Contains(row, "move") {
		t.Errorf("the controls are not on their own row:\n%q", row)
	}

	m.Resize(48, 24)
	top, hint = m.Layout.FooterY(), m.Layout.HintY()
	if row := drawTo(m).Row(top); strings.Contains(row, "0x") {
		t.Errorf("a 48-column footer still carries the readout:\n%q", row)
	}
	if row := drawTo(m).Row(hint); !strings.Contains(row, "move") {
		t.Errorf("the controls went with the readout:\n%q", row)
	}
}

// A terminal nobody is touching has nothing to report: no restamped hash, no
// sweeping scan pointer.
func TestTheAddressNeverMovesOnItsOwn(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	tick(m, 4)

	want := m.addressLine()
	for i := 0; i < 60*6; i++ {
		tick(m, 1)
		if got := m.addressLine(); got != want {
			t.Fatalf("the readout changed to %s after %d idle frames, from %s", got, i+1, want)
		}
	}
}

// This reads the footer; the other tests call addressLine, which is the string
// and not the screen.
func TestTheDrawnReadoutHoldsStill(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	tick(m, 4)

	// The readout's own row. Height-1 is the hint's row.
	row := m.Layout.FooterY()
	want := drawTo(m).Row(row)
	for i := 0; i < 60*3; i++ {
		tick(m, 1)
		if got := drawTo(m).Row(row); got != want {
			t.Fatalf("the footer changed after %d idle frames:\n got %q\nwant %q", i+1, got, want)
		}
	}
}

func TestTheAddressAnswersToTheKeyboardAndTheMouse(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	tick(m, 4)

	// A keypress, with no time passing, so only the cursor can be responsible.
	before := m.addressLine()
	m.Update(key("right"))
	if got := m.addressLine(); got == before {
		t.Errorf("the cursor moved and the readout stayed %s", before)
	}

	// And a click somewhere else on the board.
	before = m.addressLine()
	l := m.Layout
	sx, sy := l.ScreenOf(l.ViewportX+l.ViewportW-2, l.ViewportY+l.ViewportH-2)
	m.Update(tea.MouseMsg{X: sx, Y: sy, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if got := m.addressLine(); got == before {
		t.Errorf("a click across the board left the readout at %s", before)
	}
}

// That lets two refiners on one cell read the same pair on terminals of
// different sizes.
func TestTheAddressIsAFunctionOfWhereYouAre(t *testing.T) {
	l := NewLayout(120, 40)

	a, _ := AddressRange(l, 4, 9, 0)
	b, _ := AddressRange(l, 4, 9, 0)
	if a != b {
		t.Errorf("the same cell read %08X and then %08X", a, b)
	}

	// A step to the next cell moves a digit you can see. That is what the stride is
	// for: at four bytes a cell only the bottom hex digit would change.
	next, _ := AddressRange(l, 5, 9, 0)
	if next-a != addressStride {
		t.Errorf("a cell step moved the address by %d, want %d", next-a, addressStride)
	}
	if formatAddress(a)[:6] == formatAddress(next)[:6] &&
		formatAddress(a)[6:] == formatAddress(next)[6:] {
		t.Error("a cell step changed nothing in the printed address")
	}

	// And two cells far apart in the file never look alike.
	far, _ := AddressRange(l, 4, 90, 0)
	if far == a {
		t.Error("cells eighty rows apart read the same address")
	}
}
