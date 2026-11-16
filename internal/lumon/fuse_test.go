package lumon

import (
	"github.com/terminaltrove/lumon/internal/theme"
	"math"
	"testing"
)

func TestAnIgnoredClusterDetonates(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	parkCursorAwayHelper(m)

	// Alive right up to the moment.
	tick(m, int(60*(theme.ScaryRedSeconds+FuseWait+FuseSeconds-1)))
	if !m.IsActive(grp.ID) {
		t.Fatal("the cluster went early")
	}

	tick(m, 120)
	if m.IsActive(grp.ID) {
		t.Error("the cluster is still awake past its fuse")
	}
	if m.Grid.Group(grp.ID) != nil {
		t.Error("the cluster is still on the board")
	}

	// Its cells are ordinary digits again, both halves, or they would be drawn as
	// dormant-corrupt for ever with nothing able to clear them.
	for _, id := range grp.NumberIDs {
		if m.Grid.Cells[id].BadGroupID != -1 {
			t.Fatalf("cell %d still belongs to group %d", id, m.Grid.Cells[id].BadGroupID)
		}
	}
}

func TestRefiningStopsTheFuse(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	before := len(m.Room.lost)

	m.Update(key("enter"))
	tick(m, int(60*(theme.ScaryRedSeconds+FuseWait+FuseSeconds+2)))

	for _, id := range m.Room.lost {
		if id == grp.ID {
			t.Error("a refined cluster detonated anyway")
		}
	}
	if len(m.Room.lost) < before {
		t.Error("the lost list went backwards")
	}
}

func TestTheFlashQuickensAndThenHolds(t *testing.T) {
	// How many times it crosses the middle, per second, early and late.
	crossings := func(from, to float64) int {
		n, was := 0, FuseFlash(from) > 0.5
		for at := from; at > to; at -= 1.0 / 240 {
			if now := FuseFlash(at) > 0.5; now != was {
				n++
				was = now
			}
		}
		return n
	}

	early := crossings(FuseSeconds, FuseSeconds-2)
	late := crossings(FuseHold+2, FuseHold)
	if late <= early {
		t.Errorf("the flash crossed %d times in the first two seconds and %d in the last two;"+
			" it is not quickening", early, late)
	}

	// The last second is solid, not flickering.
	for at := FuseHold; at >= 0; at -= 0.01 {
		if got := FuseFlash(at); got != 1 {
			t.Fatalf("with %.2fs left the flash is %v, want it held white", at, got)
		}
	}

	// It stays in range throughout, since it drives a colour blend.
	for at := FuseSeconds; at >= 0; at -= 1.0 / 240 {
		if got := FuseFlash(at); got < 0 || got > 1 {
			t.Fatalf("with %.2fs left the flash is %v, outside [0,1]", at, got)
		}
	}
}

// The clock is the board's; only the flash and the shockwave are each
// terminal's own.
func TestTwoRefinersLoseTheClusterTogether(t *testing.T) {
	room := NewRoom(1)
	a, b := JoinRoom(room, 1), JoinRoom(room, 2)
	if a == nil || b == nil {
		t.Fatal("could not seat two refiners")
	}
	a.Resize(120, 40)
	b.Resize(120, 40)

	grp := activateFirstVisibleGroup(a)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	parkCursorAwayHelper(a)
	parkCursorAwayHelper(b)

	for i := 0; i < 60*int(theme.ScaryRedSeconds+FuseWait+FuseSeconds+2); i++ {
		tick(a, 1)
		if a.IsActive(grp.ID) != b.IsActive(grp.ID) {
			t.Fatalf("frame %d: one refiner still has the cluster and the other does not", i)
		}
	}
	if a.IsActive(grp.ID) {
		t.Error("the cluster never went")
	}
}

// Dropping the bar needs the numerator to fall, and keeping the file finishable
// needs the denominator to fall. Doing only the second makes the bar go up.
func TestADetonationCostsProgressAndLeavesTheFileFinishable(t *testing.T) {
	m := ready()
	grownCluster(t, m)
	m.Update(key("enter"))
	tick(m, 300) // let the credit land

	if m.File.Completion(m.Grid) <= 0 {
		t.Skip("nothing was credited to lose")
	}
	before := m.File.Completion(m.Grid)
	lost := len(m.Room.lost)

	for i := 0; i < clusterLifeFrames()+120 && len(m.Room.lost) == lost; i++ {
		tick(m, 1)
	}
	if len(m.Room.lost) == lost {
		t.Skip("nothing detonated")
	}

	if after := m.File.Completion(m.Grid); after >= before {
		t.Errorf("the file went from %v to %v across a detonation; it should have cost something",
			before, after)
	}

	// Still finishable: refining everything that is left must reach 100%.
	refined, quota := 0, 0
	for i := range m.Grid.Bins {
		refined += m.Grid.Bins[i].Refined
		quota += m.Grid.Bins[i].Quota
	}
	if left := quota - refined; left != len(m.Grid.Groups) {
		t.Errorf("%d clusters are left on the board but the bins are waiting for %d",
			len(m.Grid.Groups), left)
	}
}

// The sheet takes a disturbance and lets it go again, leaving every digit
// exactly where it found it.
func TestTheRippleRunsOutAndLetsGo(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	tick(m, 2)

	l := m.Layout
	mid := func() (int, int) { return l.ViewportX + l.ViewportW/2, l.ViewportY + l.ViewportH/2 }
	cx, cy := mid()
	m.ripple.Kick(float64(cx), float64(cy))

	moved := 0.0
	for i := 0; i < 60*8 && m.ripple.Moving(); i++ {
		tick(m, 1)
		for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
			for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
				dx, dy := m.ripple.Offset(gx, gy)
				if math.IsNaN(dx) || math.IsNaN(dy) {
					t.Fatalf("the sheet gave NaN at (%d,%d)", gx, gy)
				}
				if d := math.Hypot(dx, dy); d > moved {
					moved = d
				}
			}
		}
	}
	if moved < 0.2 {
		t.Errorf("the biggest displacement was %v cells; nothing really moved", moved)
	}
	if m.ripple.Moving() {
		t.Error("the sheet is still ringing after eight seconds")
	}
	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			if dx, dy := m.ripple.Offset(gx, gy); dx != 0 || dy != 0 {
				t.Fatalf("cell (%d,%d) was left displaced by (%v,%v)", gx, gy, dx, dy)
			}
		}
	}
}

// The disturbance travels through the springs, so it reaches a distant cell
// strictly later than a near one. A ring drawn from an equation would light
// both on the frame their radius matched. Both probes are outside the kicked
// core, or this would only show that the impulse has an edge.
func TestTheRipplePropagatesOutward(t *testing.T) {
	m := ready()
	m.Resize(160, 44)
	tick(m, 2)

	l := m.Layout
	cx, cy := l.ViewportX+2, l.ViewportY+l.ViewportH/2
	m.ripple.Kick(float64(cx), float64(cy))

	core := int(math.Ceil((rippleCoreCells * CellWidth) / CellWidth))
	nearAt, farAt := core+1, core+3

	const wake = 0.02
	near, far := -1, -1
	for i := 0; i < 60*6 && (near < 0 || far < 0); i++ {
		tick(m, 1)
		if near < 0 {
			if dx, dy := m.ripple.Offset(cx+nearAt, cy); math.Hypot(dx, dy) > wake {
				near = i
			}
		}
		if far < 0 {
			if dx, dy := m.ripple.Offset(cx+farAt, cy); math.Hypot(dx, dy) > wake {
				far = i
			}
		}
	}
	if near < 0 || far < 0 {
		t.Fatalf("the wave reached %d cells at frame %d and %d cells at frame %d",
			nearAt, near, farAt, far)
	}
	if far <= near {
		t.Errorf("%d cells out moved at frame %d, no later than %d cells out at %d;"+
			" nothing is propagating", farAt, far, nearAt, near)
	}
}

// A cell is CellWidth columns across and RowPitch rows down, so one stiffness
// on both axes would send the wave down the screen faster than across it.
func TestTheRippleIsRoundOnTheScreen(t *testing.T) {
	m := ready()
	m.Resize(160, 50)
	tick(m, 2)

	l := m.Layout
	cx, cy := l.ViewportX+l.ViewportW/2, l.ViewportY+l.ViewportH/2
	m.ripple.Kick(float64(cx), float64(cy))

	// Two cells the same distance away on screen: RowAspect times fewer rows than
	// columns.
	const outX = 4
	outY := int(outX * CellWidth / (RowPitch * RowAspect))

	const wake = 0.02
	across, down := -1, -1
	for i := 0; i < 60*6 && (across < 0 || down < 0); i++ {
		tick(m, 1)
		if across < 0 {
			if dx, dy := m.ripple.Offset(cx+outX, cy); math.Hypot(dx, dy) > wake {
				across = i
			}
		}
		if down < 0 {
			if dx, dy := m.ripple.Offset(cx, cy+outY); math.Hypot(dx, dy) > wake {
				down = i
			}
		}
	}
	if across < 0 || down < 0 {
		t.Skip("the wave did not reach both probes")
	}
	if d := across - down; d > 6 || d < -6 {
		t.Errorf("the wave reached %d columns out at frame %d and the same screen distance"+
			" down at frame %d; it is not round", outX*CellWidth, across, down)
	}
}

// The quotas are rebuilt from the seed on load, so they expect every cluster
// the file ever had, including the ones that blew up. The save does not record
// why a cluster left: the bins' quotas are derived from what is still on the
// board and what they already hold.
func TestADetonatedFileComesBackFinishable(t *testing.T) {
	rs := NewRooms(5)
	room := rs.Open()
	m := JoinRoom(room, 1)
	if m == nil {
		t.Fatal("could not take a desk")
	}
	m.Resize(120, 40)

	grownCluster(t, m)
	m.Update(key("enter"))
	tick(m, 300)

	lost := len(room.lost)
	for i := 0; i < clusterLifeFrames()+120 && len(room.lost) == lost; i++ {
		tick(m, 1)
	}
	if len(room.lost) == lost {
		t.Skip("nothing detonated")
	}

	left := func(r *Room) (groups, waiting int) {
		for i := range r.Grid.Bins {
			waiting += r.Grid.Bins[i].Quota - r.Grid.Bins[i].Refined
		}
		return len(r.Grid.Groups), waiting
	}
	wantGroups, wantWaiting := left(room)
	wantPct := room.percent()

	back := restoreFloor(floorState{rooms: []fileState{room.capture()}}, 5)
	if len(back.rooms) != 1 {
		t.Fatalf("the floor came back with %d rooms", len(back.rooms))
	}
	gotGroups, gotWaiting := left(back.rooms[0])

	if gotGroups != wantGroups {
		t.Errorf("%d clusters came back, want %d", gotGroups, wantGroups)
	}
	if gotWaiting != wantWaiting {
		t.Errorf("the bins came back waiting for %d, want %d", gotWaiting, wantWaiting)
	}
	if gotWaiting != gotGroups {
		t.Errorf("the restored file has %d clusters and expects %d; it cannot be finished",
			gotGroups, gotWaiting)
	}
	if got := back.rooms[0].percent(); got != wantPct {
		t.Errorf("the restored file reads %d%%, want %d%%", got, wantPct)
	}
}

// A drawn ring knows nothing about any other ring. In the sheet the two
// disturbances add, so where they meet the digits move further than either wave
// alone would take them.
func TestTwoRipplesInterfere(t *testing.T) {
	peak := func(kick func(m *Model, l *Layout)) float64 {
		m := ready()
		m.Resize(140, 44)
		tick(m, 2)
		kick(m, m.Layout)

		l, best := m.Layout, 0.0
		for i := 0; i < 90; i++ {
			tick(m, 1)
			mx := l.ViewportX + l.ViewportW/2
			for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
				dx, dy := m.ripple.Offset(mx, gy)
				if d := math.Hypot(dx, dy); d > best {
					best = d
				}
			}
		}
		return best
	}

	one := peak(func(m *Model, l *Layout) {
		m.ripple.Kick(float64(l.ViewportX+2), float64(l.ViewportY+l.ViewportH/2))
	})
	both := peak(func(m *Model, l *Layout) {
		m.ripple.Kick(float64(l.ViewportX+2), float64(l.ViewportY+l.ViewportH/2))
		m.ripple.Kick(float64(l.ViewportX+l.ViewportW-3), float64(l.ViewportY+l.ViewportH/2))
	})

	if one <= 0 {
		t.Skip("the wave never reached the middle")
	}
	if both <= one*1.05 {
		t.Errorf("one wave moved the middle %v cells and two moved it %v;"+
			" they are not adding", one, both)
	}
}

// The sheet costs nothing while nothing is exploding, which is almost always.
func TestAStillSheetCostsNothing(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	tick(m, 120)

	if m.ripple.Moving() {
		t.Fatal("the sheet is moving with nothing having gone off")
	}
	for gy := m.Layout.ViewportY; gy < m.Layout.ViewportY+m.Layout.ViewportH; gy++ {
		for gx := m.Layout.ViewportX; gx < m.Layout.ViewportX+m.Layout.ViewportW; gx++ {
			if dx, dy := m.ripple.Offset(gx, gy); dx != 0 || dy != 0 {
				t.Fatalf("cell (%d,%d) is displaced by (%v,%v) on a quiet board", gx, gy, dx, dy)
			}
		}
	}
}

func TestADetonationReachesTheSheet(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	tick(m, 2)

	m.Room.blastSeq++
	m.Room.blasts = append(m.Room.blasts, Blast{
		X:  float64(m.Layout.ViewportX + m.Layout.ViewportW/2),
		Y:  float64(m.Layout.ViewportY + m.Layout.ViewportH/2),
		At: m.Room.blastTime, Seq: m.Room.blastSeq,
	})

	tick(m, 1)
	if !m.ripple.Moving() {
		t.Fatal("the board detonated and the sheet did not move")
	}
	seen := m.ripple.seen

	// The record stays in the list for another second and must not kick it again;
	// a wave restarted every frame never travels.
	tick(m, 30)
	if m.ripple.seen != seen {
		t.Errorf("the same detonation was applied again: %d then %d", seen, m.ripple.seen)
	}
}
