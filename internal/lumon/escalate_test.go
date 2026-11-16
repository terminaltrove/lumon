package lumon

import (
	"github.com/terminaltrove/lumon/internal/theme"
	"math"
	"testing"
)

// meanOffset is how far a cluster's digits stray from their cells on average,
// sampled over a window so a single frame's phase does not decide it.
func meanOffset(m *Model, grp *BadGroup, frames int) float64 {
	sum, n := 0.0, 0
	for i := 0; i < frames; i++ {
		tick(m, 1)
		for _, id := range grp.NumberIDs {
			dx, dy := m.Agitation.Offset(id)
			sum += math.Hypot(dx, dy)
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// peakOver is the largest a cluster gets over a window, which is what the player
// registers; one frame catches an arbitrary point of the breath.
func peakOver(m *Model, grp *BadGroup, frames int) float64 {
	peak := 0.0
	for i := 0; i < frames; i++ {
		tick(m, 1)
		if h := clusterPeak(m, grp); h > peak {
			peak = h
		}
	}
	return peak
}

func TestAnIgnoredClusterKeepsGrowing(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	parkCursorAwayHelper(m)

	// Sampled against the escalation clock, so the windows scale with the swell's
	// length.
	early := int(60 * EscalateSeconds / 4)
	tick(m, early)
	young := peakOver(m, grp, 240)
	tick(m, early*2) // three quarters of the way up, and short of the fuse
	old := peakOver(m, grp, 240)

	if old <= young {
		t.Errorf("cluster peaks at %v later and %v earlier, want it still growing", old, young)
	}
	// A quarter of the swell's full range.
	if want := (BreatheHigh - PlainHeight) / 4; old-young < want {
		t.Errorf("cluster only grew from %v to %v, less than %v; the escalation is "+
			"too subtle to read", young, old, want)
	}
}

func TestAnIgnoredClusterShakesHarder(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	parkCursorAwayHelper(m)

	early := int(60 * EscalateSeconds / 4)
	tick(m, early)
	young := meanOffset(m, grp, 240)
	tick(m, early*2)
	old := meanOffset(m, grp, 240)

	if old <= young {
		t.Errorf("mean offset is %v later and %v earlier, want a wider shake", old, young)
	}
}

// Otherwise a cluster left long enough would shake itself off the screen.
// Escalation saturates at EscalateSeconds and the fuse runs out at
// theme.ScaryRedSeconds + FuseWait + FuseSeconds, so this checks that it
// saturates and then holds for the rest of the cluster's life.
func TestEscalationSaturates(t *testing.T) {
	m := ready()
	if activateFirstVisibleGroup(m) == nil {
		t.Skip("no bad group visible")
	}
	parkCursorAwayHelper(m)

	tick(m, int(60*(EscalateSeconds+1)))
	early := m.worstEscalation()
	tick(m, int(60*(theme.ScaryRedSeconds+FuseWait+FuseSeconds-EscalateSeconds-2)))
	late := m.worstEscalation()

	if early < 0.999 {
		t.Errorf("escalation is only %v past EscalateSeconds, want saturated", early)
	}
	if late != early {
		t.Errorf("escalation moved from %v to %v after saturating", early, late)
	}
}

// Each one escalates on its own clock.
func TestEachClusterEscalatesOnItsOwnClock(t *testing.T) {
	m := ready()
	old := activateFirstVisibleGroup(m)
	if old == nil {
		t.Skip("no bad group visible")
	}
	// Wait on the escalation, not a frame count, so this follows EscalateSeconds.
	for i := 0; i < int(60*(EscalateSeconds+2)) && m.escalation(old.ID) < 0.9; i++ {
		tick(m, 1)
	}
	if m.escalation(old.ID) < 0.9 {
		t.Skip("the first cluster did not escalate")
	}

	// Room for a second cluster, then wake one.
	m.Room.SetDesks(2)
	var fresh *BadGroup
	for _, grp := range m.Grid.Groups {
		if !m.IsActive(grp.ID) {
			fresh = grp
			break
		}
	}
	if fresh == nil {
		t.Skip("no second cluster available")
	}
	m.Activate(fresh)

	if got := m.escalation(fresh.ID); got != 0 {
		t.Errorf("a freshly woken cluster is already at %v", got)
	}
	if got := m.escalation(old.ID); got < 0.9 {
		t.Errorf("waking a second cluster reset the first, now %v", got)
	}
}

func TestWakingAnAwakeClusterIsANoOp(t *testing.T) {
	m := ready()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	tick(m, 300)
	before := m.Awake(grp.ID)
	awake := len(m.Actives())

	m.Activate(grp)

	if got := m.Awake(grp.ID); got != before {
		t.Errorf("re-waking reset the clock from %v to %v", before, got)
	}
	// Asked as a delta, since three hundred frames is long enough for the room to
	// have woken the second cluster a lone refiner is entitled to.
	if got := len(m.Actives()); got != awake {
		t.Errorf("%d clusters awake after re-waking one, want the %d there were", got, awake)
	}
}

// Otherwise every refine leaves particles integrated and constraints relaxed for
// the rest of the session.
func TestReleasingAClusterFreesItsBody(t *testing.T) {
	m := ready()
	base := m.Agitation.world.Len()

	for i := 0; i < 12; i++ {
		grp := activateFirstVisibleGroup(m)
		if grp == nil {
			break
		}
		m.Agitation.Release(grp.ID)
	}

	if got := m.Agitation.world.Len(); got != base {
		t.Errorf("the agitation world holds %d particles after 12 refines, want %d", got, base)
	}
}

// The swell belongs to the board, so its turnaround must not read heights back
// from Magnify, which includes this player's cursor lens.
func TestTheSwellIgnoresWhereTheCursorIs(t *testing.T) {
	run := func(park bool) []float64 {
		m := NewModel(5)
		m.Resize(120, 40)
		grp := activateFirstVisibleGroup(m)
		if grp == nil {
			t.Skip("no bad group visible")
		}
		if park {
			parkCursorAwayHelper(m)
		} else {
			// Sit right on the cluster, where the lens is strongest.
			id := grp.NumberIDs[0]
			m.CursorX, m.CursorY = id%GridSize, id/GridSize
		}
		m.focusLens()

		var trace []float64
		for i := 0; i < 900; i++ {
			tick(m, 1)
			if i%30 == 0 {
				trace = append(trace, m.swellOf(grp.ID))
			}
		}
		return trace
	}

	onIt, away := run(false), run(true)

	for i := range onIt {
		if math.Abs(onIt[i]-away[i]) > 1e-9 {
			t.Fatalf("sample %d: swell is %v with the cursor on the cluster and %v with it parked",
				i, onIt[i], away[i])
		}
	}
}

// A refilling cell's scramble must not draw from the stream that picks the
// replacement digits.
func TestCosmeticsDoNotDisturbTheBoard(t *testing.T) {
	board := func(resizes int) []int {
		m := NewModel(11)
		m.Resize(120, 40)
		for i := 0; i < resizes; i++ {
			m.Resize(100+i%20, 30+i%10)
		}
		m.Resize(120, 40)

		var out []int
		for i := 0; i < 6; i++ {
			out = append(out, m.rng.Intn(1000))
		}
		return out
	}

	none, many := board(0), board(25)

	for i := range none {
		if none[i] != many[i] {
			t.Fatalf("draw %d: %d without resizes, %d after 25 of them", i, none[i], many[i])
		}
	}
}

// A room wakes HuntPerRefiner clusters for each occupied desk. The wake cap is a
// separate field from the seat cap: sharing one would cap a full room of four at
// four awake however many were asked for.
func TestClustersWakePerRefiner(t *testing.T) {
	for _, desks := range []int{1, 2, 4} {
		m := ready()
		m.Room.SetDesks(desks)
		parkCursorAwayHelper(m)

		// Not fifty seconds: a board left that long has its visible clusters detonate
		// until none are left to wake.
		tick(m, 300)

		want := desks * HuntPerRefiner
		if want > MaxAwake {
			want = MaxAwake
		}
		if got := m.Room.HuntCount(); got != want {
			t.Errorf("%d desks set a wake cap of %d, want %d", desks, got, want)
		}
		if got := m.Room.DeskCount(); got != desks {
			t.Errorf("%d refiners left the room reporting %d desks", desks, got)
		}

		// Only a visible cluster is chosen, and one viewport does not hold eight, so the
		// cap is asserted exactly and the count only has to reach it where the board can
		// supply it, which alone it always can. Sampled over a stretch, since at any
		// frame one cluster may have detonated before the next wakes.
		best := 0
		for i := 0; i < 600; i++ {
			tick(m, 1)
			got := len(m.Actives())
			if got > want {
				t.Fatalf("%d desks woke %d clusters, past the cap of %d", desks, got, want)
			}
			if got > best {
				best = got
			}
		}
		if desks == 1 && best != want {
			t.Errorf("a lone refiner never had more than %d clusters awake, want %d", best, want)
		}
	}
}

// Otherwise it thrashes with nobody to work it.
func TestClosingADeskRetiresACluster(t *testing.T) {
	m := ready()
	m.Room.SetDesks(3)
	parkCursorAwayHelper(m)
	tick(m, 3000)
	before := len(m.Actives())
	if before <= HuntPerRefiner {
		t.Skip("three desks never woke more than one refiner's worth")
	}

	m.Room.SetDesks(1)
	tick(m, 1) // the view reconciles its own bodies on its next frame

	if got := len(m.Actives()); got != HuntPerRefiner {
		t.Errorf("%d clusters still awake after closing two desks, want %d",
			got, HuntPerRefiner)
	}
	// The bodies must go back too, or they are integrated for nothing.
	for _, grp := range m.Grid.Groups {
		if !m.IsActive(grp.ID) && m.Agitation.Tracked(grp.ID) {
			t.Errorf("cluster %d is asleep but still has a body", grp.ID)
		}
	}
}

func TestAwakeClustersAreDistinct(t *testing.T) {
	m := ready()
	m.Room.SetDesks(4)
	parkCursorAwayHelper(m)
	tick(m, 3000)

	seen := map[int]bool{}
	for _, grp := range m.Actives() {
		if seen[grp.ID] {
			t.Fatalf("cluster %d is awake twice", grp.ID)
		}
		seen[grp.ID] = true
	}
}

// awakeAt reports whether a cell belongs to a cluster that is awake now. A room
// wakes HuntPerRefiner clusters per refiner, so an ordinary digit is one in no
// awake cluster, asked of the board.
func awakeAt(m *Model, gx, gy int) bool {
	n := m.Grid.At(gx, gy)
	return n != nil && m.IsActive(n.BadGroupID)
}
