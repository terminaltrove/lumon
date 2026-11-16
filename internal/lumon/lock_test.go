package lumon

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// lockOntoACluster puts the cursor on a woken cluster and commits to it,
// returning the group.
func lockOntoACluster(t *testing.T, m *Model) *BadGroup {
	t.Helper()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	id := grp.NumberIDs[0]
	m.CursorX, m.CursorY = id%GridSize, id/GridSize
	m.Update(key("f"))
	return grp
}

// A lock that survived the cursor walking away would hold a cluster open with
// nothing pointing at it.
func TestPlainMovementLetsGoOfTheCluster(t *testing.T) {
	m := ready()
	lockOntoACluster(t, m)
	if len(m.held) == 0 {
		t.Fatal("f did not lock onto the cluster under the cursor")
	}

	m.Update(key("right"))
	if len(m.held) > 0 {
		t.Errorf("the cursor moved and the ringfence survived: %v", m.held)
	}
}

// Shift navigates within a commitment, so crossing a corner of the cluster on
// the way to its far side must not drop it.
func TestShiftMovementKeepsTheCluster(t *testing.T) {
	m := ready()
	grp := lockOntoACluster(t, m)

	for _, k := range []string{"shift+right", "shift+down", "shift+left", "shift+up"} {
		m.Update(key(k))
		if !m.held[grp.ID] {
			t.Fatalf("%s dropped the ringfence; held is %v, want group %d", k, m.held, grp.ID)
		}
	}
}

// Holding it afterwards would lift cells that are now ordinary digits on their
// way to a bin.
func TestRefiningLetsGoOfTheCluster(t *testing.T) {
	m := ready()
	grp := lockOntoACluster(t, m)
	m.Update(key("enter"))

	if m.held[grp.ID] {
		t.Errorf("the cluster was refined and the ringfence survived")
	}
	if m.Grid.Group(grp.ID) != nil {
		t.Error("the cluster was not refined at all")
	}
}

// The lift is gated on the cluster still being there. Retire takes a group off
// the board without touching the room's list of awake ones, so
// `Group(id) != nil` catches a detonation and `IsActive(id)` catches a cluster
// put back to sleep. Asserted against the targets, not the drawn height:
// MaybeActivate may wake the same cluster again a second later.
func TestALockOnAClusterThatWentLiftsNothing(t *testing.T) {
	m := ready()
	grp := lockOntoACluster(t, m)
	tick(m, 120)

	m.Room.mu.Lock()
	m.Room.Retire(grp)
	m.Room.mu.Unlock()

	if m.Grid.Group(grp.ID) != nil {
		t.Fatal("the cluster is still on the board; this is not testing what it means to")
	}
	// Compared against the same board with nothing locked: Retire leaves the
	// cluster in the room's awake list, so its breathe still has a height. The lock
	// must add nothing.
	held := map[int]float64{}
	for id, h := range m.glyphTargets() {
		held[id] = h
	}
	was := m.held
	m.held = map[int]bool{}
	free := m.glyphTargets()
	m.held = was

	for _, id := range grp.NumberIDs {
		if held[id] != free[id] {
			t.Fatalf("cell %d is lifted to %v held and %v free; a lock on a cluster "+
				"that has gone is still adding to it", id, held[id], free[id])
		}
	}
}

// Tested at the message level: bubbletea defines shift+arrows in three
// encodings and the terminal emulator may swallow them first, so the capitals
// are the form no terminal can swallow.
func TestShiftArrowsAndCapitalsBothKeepTheLock(t *testing.T) {
	for _, c := range []struct {
		what   string
		msg    tea.KeyMsg
		dx, dy int
	}{
		{"shift+up", tea.KeyMsg{Type: tea.KeyShiftUp}, 0, -1},
		{"shift+down", tea.KeyMsg{Type: tea.KeyShiftDown}, 0, 1},
		{"shift+left", tea.KeyMsg{Type: tea.KeyShiftLeft}, -1, 0},
		{"shift+right", tea.KeyMsg{Type: tea.KeyShiftRight}, 1, 0},
		{"K", key("K"), 0, -1},
		{"J", key("J"), 0, 1},
		{"H", key("H"), -1, 0},
		{"L", key("L"), 1, 0},
	} {
		t.Run(c.what, func(t *testing.T) {
			m := ready()
			grp := lockOntoACluster(t, m)
			if !m.held[grp.ID] {
				t.Fatalf("f did not ringfence; held is %v, want group %d", m.held, grp.ID)
			}

			// Somewhere the cluster is not, so the lock has to survive on its own.
			m.CursorX, m.CursorY = 40, 40
			was := len(m.held)
			x, y := m.CursorX, m.CursorY

			m.Update(c.msg)

			if m.CursorX != x+c.dx || m.CursorY != y+c.dy {
				t.Errorf("%s moved the cursor to (%d,%d), want (%d,%d), the key "+
					"never reached the handler", c.what, m.CursorX, m.CursorY, x+c.dx, y+c.dy)
			}
			if len(m.held) != was {
				t.Errorf("%s dropped the lock; it is meant to navigate within one", c.what)
			}
		})
	}
}

func TestAPlainArrowOffTheClusterLetsGo(t *testing.T) {
	m := ready()
	lockOntoACluster(t, m)
	m.CursorX, m.CursorY = 40, 40
	m.Update(key("up"))
	if len(m.held) > 0 {
		t.Error("a plain arrow away from the cluster kept the ringfence")
	}
}

// A held cluster's digits are drawn at full size and lifted off the board on a
// plate of their own. Checked in the drawn buffer: it is bigger than the same
// cluster free, and it has the plate.
func TestRingfencedNumbersAreDrawnHeld(t *testing.T) {
	m := ready()
	m.Resize(140, 44)
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	parkCursorAwayHelper(m)
	tick(m, int(60*HintSeconds))
	free := clusterPeak(m, grp)

	id := grp.NumberIDs[0]
	m.CursorX, m.CursorY = id%GridSize, id/GridSize
	m.Update(key("f"))
	tick(m, 60)

	if held := clusterPeak(m, grp); held <= free {
		t.Errorf("ringfenced the cluster is %v rows and free it was %v", held, free)
	}

	buf := drawTo(m)
	plated := 0
	for _, b := range m.boxes {
		if !m.held[m.Grid.Cells[b.id].BadGroupID] {
			continue
		}
		for y := b.y; y < b.y+b.h; y++ {
			for x := b.x; x < b.x+b.w; x++ {
				if buf.AttrAt(x, y).BG == m.Theme.Plate {
					plated++
				}
			}
		}
	}
	if plated == 0 {
		t.Error("no ringfenced digit is drawn on a plate")
	}

	// Letting go takes it away again.
	m.CursorX, m.CursorY = 40, 40
	m.Update(key("up"))
	tick(m, 120)
	if len(m.held) > 0 {
		t.Fatal("moving away did not let go")
	}
	buf = drawTo(m)
	for _, b := range m.boxes {
		for y := b.y; y < b.y+b.h; y++ {
			for x := b.x; x < b.x+b.w; x++ {
				if buf.AttrAt(x, y).BG == m.Theme.Plate {
					t.Fatalf("a digit is still plated at (%d,%d) after letting go", x, y)
				}
			}
		}
	}
}

// The swell says "over here" and stops well short of legible; only ringfencing
// brings a cluster to full size.
func TestRingfencingIsWhatMakesAClusterReadable(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	parkCursorAwayHelper(m)
	tick(m, int(60*HintSeconds)) // the swell has had its whole run

	free := m.glyphTargets()[grp.NumberIDs[0]]
	if free > BreatheHigh+0.01 {
		t.Errorf("left alone the cluster is asked for %v rows, past the %v ceiling",
			free, BreatheHigh)
	}

	id := grp.NumberIDs[0]
	m.CursorX, m.CursorY = id%GridSize, id/GridSize
	m.Update(key("f"))
	held := m.glyphTargets()[id]

	if held != MaxGlyphHeight {
		t.Errorf("ringfenced the cluster is asked for %v, want the full %v",
			held, MaxGlyphHeight)
	}
	if held <= free*2 {
		t.Errorf("ringfencing took it from %v to %v; the gesture has to be the "+
			"thing that reveals it", free, held)
	}
}

// The growth is how a cluster asks to be found; one that has finished asking
// before you look up is decoration.
func TestAClusterIsAPlainCharacterForItsFirstTwentySeconds(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	parkCursorAwayHelper(m)

	for s := 1; s <= 20; s++ {
		tick(m, 60)
		if got := clusterPeak(m, grp); got >= MinGlyphHeight {
			t.Fatalf("after %ds the cluster is already %v rows; it is meant to still "+
				"be an ordinary character", s, got)
		}
	}

	// It does get there, or "slow" would just mean "never".
	for i := 0; i < int(60*EscalateSeconds) && clusterPeak(m, grp) < MinGlyphHeight; i++ {
		tick(m, 1)
	}
	if got := clusterPeak(m, grp); got < MinGlyphHeight {
		t.Errorf("the cluster never became a glyph at all; it reached %v", got)
	}
}

// Nothing is held and nothing is pressed; the cluster is readable while you are
// on it.
func TestPointingAtAClusterRevealsIt(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	parkCursorAwayHelper(m)
	tick(m, int(60*HintSeconds))
	away := clusterPeak(m, grp)

	id := grp.NumberIDs[0]
	m.CursorX, m.CursorY = id%GridSize, id/GridSize
	m.updateHover(m.CursorX, m.CursorY)
	tick(m, 60)

	on := clusterPeak(m, grp)
	if on < MaxGlyphHeight-0.5 {
		t.Errorf("pointed at, the cluster is %v rows; want it up to %v",
			on, MaxGlyphHeight)
	}
	if len(m.held) > 0 {
		t.Errorf("pointing at a cluster held it: %v", m.held)
	}

	// Away again, and it drops back to the hint.
	m.CursorX, m.CursorY = 40, 40
	m.updateHover(40, 40)
	tick(m, 180)
	if back := clusterPeak(m, grp); back > away+0.5 {
		t.Errorf("the cursor left and the cluster stayed at %v, having been %v",
			back, away)
	}
}

// The swell is a hint (something is over here, and how long it has been
// ignored); the reveal belongs to the gesture.
func TestALeftAloneClusterNeverBecomesReadable(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	parkCursorAwayHelper(m)

	for i := 0; i < clusterLifeFrames()-120; i++ {
		tick(m, 1)
		if got := clusterPeak(m, grp); got > BreatheHigh+0.1 {
			t.Fatalf("after %.1fs an unattended cluster is %v rows, past the %v "+
				"ceiling the hint is meant to stop at", float64(i)/60, got, BreatheHigh)
		}
	}
}

func TestSweepingHoldsEveryClusterItTouches(t *testing.T) {
	m := ready()
	m.Resize(160, 50)

	// Wake everything on screen, so there is more than one thing to gather.
	l := m.Layout
	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			if n := m.Grid.At(gx, gy); n != nil && n.BadGroupID >= 0 {
				m.Room.mu.Lock()
				m.Activate(m.Grid.Group(n.BadGroupID))
				m.Room.mu.Unlock()
			}
		}
	}
	if len(m.Actives()) < 2 {
		t.Skip("fewer than two clusters woke")
	}

	// Sweep the viewport with shift+arrows.
	m.CursorX, m.CursorY = l.ViewportX, l.ViewportY
	for i := 0; i < l.ViewportW-1; i++ {
		m.Update(key("shift+right"))
	}
	for i := 0; i < l.ViewportH-1; i++ {
		m.Update(key("shift+down"))
	}

	if len(m.held) < 2 {
		t.Fatalf("a sweep of the whole board held %d clusters", len(m.held))
	}
	for id := range m.held {
		if !m.IsActive(id) {
			t.Errorf("the sweep held group %d, which is not awake", id)
		}
	}
}

// A cluster is a blob and a sweep is a rectangle; requiring all of it inside
// would mean drawing carefully round an L.
func TestASweepTakesAClusterItOnlyClips(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	if len(grp.NumberIDs) < 2 {
		t.Skip("the cluster is a single digit")
	}

	// A sweep of exactly one of its cells.
	id := grp.NumberIDs[0]
	m.CursorX, m.CursorY = id%GridSize, id/GridSize
	m.sweep = &sweepBox{ax: m.CursorX, ay: m.CursorY, bx: m.CursorX, by: m.CursorY}
	m.takeSweep()

	if !m.held[grp.ID] {
		t.Fatal("a sweep over one of the cluster's cells did not take it")
	}
	targets := m.glyphTargets()
	for _, member := range grp.NumberIDs {
		if m.Flights.InFlight(member) {
			continue
		}
		if targets[member] != MaxGlyphHeight {
			t.Errorf("member %d is asked for %v, not the full %v, the cluster was "+
				"taken in part", member, targets[member], MaxGlyphHeight)
		}
	}
}

func TestPlainMovementClearsTheSweep(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	id := grp.NumberIDs[0]
	m.CursorX, m.CursorY = id%GridSize, id/GridSize
	m.Update(key("f"))
	if len(m.held) == 0 {
		t.Fatal("f did not ringfence")
	}

	m.Update(key("right"))
	if len(m.held) > 0 {
		t.Errorf("a plain arrow left %v ringfenced", m.held)
	}
	if m.sweep != nil {
		t.Error("a plain arrow started a sweep")
	}
}

func TestRefiningSendsEverythingHeld(t *testing.T) {
	m := ready()
	m.Resize(160, 50)
	l := m.Layout
	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			if n := m.Grid.At(gx, gy); n != nil && n.BadGroupID >= 0 {
				m.Room.mu.Lock()
				m.Activate(m.Grid.Group(n.BadGroupID))
				m.Room.mu.Unlock()
			}
		}
	}
	if len(m.Actives()) < 2 {
		t.Skip("fewer than two clusters woke")
	}

	m.CursorX, m.CursorY = l.ViewportX, l.ViewportY
	for i := 0; i < l.ViewportW-1; i++ {
		m.Update(key("shift+right"))
	}
	for i := 0; i < l.ViewportH-1; i++ {
		m.Update(key("shift+down"))
	}
	want := len(m.heldGroups())
	if want < 2 {
		t.Skipf("the sweep held %d clusters", want)
	}

	// Counted off the board, not the bin: a bin is credited when the digits land, a
	// third of a second of flight away.
	held := m.heldGroups()
	m.Update(key("enter"))

	sent := 0
	for _, grp := range held {
		if m.Grid.Group(grp.ID) == nil {
			sent++
		}
	}
	if sent < 1 {
		t.Fatal("refining a ringfenced sweep sent nothing")
	}
	// A bin with room for one of two takes one and leaves the other ringfenced.
	if sent+len(m.held) != want {
		t.Errorf("%d of %d went and %d are still held; they should account for "+
			"all of it", sent, want, len(m.held))
	}
}

// theme.Theme.Hovered, white and bold, is reserved for the single digit under
// the cursor. The ringfence must be legible while your hand moves and must not
// be louder than the selection or the board.
func TestTheRingfenceIsDrawnQuietly(t *testing.T) {
	m := ready()
	m.sweep = &sweepBox{
		ax: m.CursorX - 2, ay: m.CursorY - 1,
		bx: m.CursorX + 2, by: m.CursorY + 1,
	}
	buf := drawTo(m)

	// Only the dashes, and only inside the board: the corner runes are box drawing
	// that the header and the bins use too.
	l := m.Layout
	found := 0
	for y := l.BoardY; y < l.BoardY+l.BoardH; y++ {
		row := []rune(buf.Row(y))
		for x, r := range row {
			if r != '┄' && r != '┊' {
				continue
			}
			found++
			got := buf.AttrAt(x, y)
			if got.FG == m.Theme.Peak {
				t.Fatalf("the ringfence at (%d,%d) is drawn in the peak %v, the "+
					"brightest colour there is", x, y, m.Theme.Peak)
			}
			if got.Bold {
				t.Fatalf("the ringfence at (%d,%d) is bold", x, y)
			}
			if got.FG != m.Theme.Dim {
				t.Fatalf("the ringfence at (%d,%d) is %v, want the dim %v",
					x, y, got.FG, m.Theme.Dim)
			}
		}
	}
	if found == 0 {
		t.Fatal("no ringfence was drawn at all")
	}
}
