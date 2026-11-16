package lumon

import "testing"

// refineFrom drives one model to refine the first cluster it can reach into the
// given bin, and returns the cluster it took.
func refineFrom(t *testing.T, m *Model, bin int) *BadGroup {
	t.Helper()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	id := grp.NumberIDs[0]
	m.CursorX, m.CursorY = id%GridSize, id/GridSize
	m.SelectedBin = bin
	m.Update(key("enter"))
	return grp
}

func TestTheOtherRefinerSeesTheDigitsFly(t *testing.T) {
	_, a, b := twoInARoom(t)

	grp := refineFrom(t, a, 2)
	want := len(grp.NumberIDs)

	// One frame is enough: the deposit is caught in stepBoard, which runs before
	// anything else this view does.
	tick(b, 1)

	got := b.Flights.Active()
	if len(got) == 0 {
		t.Fatal("the other refiner has nothing in the air; the refinement was invisible to them")
	}
	if len(got) != want {
		t.Errorf("the other refiner is flying %d digits, want the cluster's %d", len(got), want)
	}
	for _, f := range got {
		if f.BinIdx != 2 {
			t.Errorf("a mirrored digit is aimed at bin %d, want the bin it was refined into", f.BinIdx+1)
			break
		}
	}
}

// Neither binfill.go nor drawer.go knows about deposits. The lid opens because
// BinFill counts flights aimed at a bin, and the drawer comes up because the
// bin reached a quarter, so launching the digits is enough.
func TestTheOtherRefinersLidOpensAndDrawerFollows(t *testing.T) {
	_, a, b := twoInARoom(t)
	fillTo(b, 0, shortOfAQuarter(b, 0))
	refineFrom(t, a, 0)

	tick(b, 2)
	if !b.BinFill.Receiving(0) {
		t.Fatal("the other refiner's bin is not receiving anything")
	}
	if !b.BinFill.Ajar(0) {
		t.Error("the other refiner's lid stayed shut while digits were on their way")
	}
	for i := 0; i < 60*4 && !b.Drawer.Showing(0); i++ {
		tick(b, 1)
	}
	if !b.Drawer.Showing(0) {
		t.Error("the other refiner's drawer never came up as the bin reached a quarter")
	}
}

// The refiner launches their own cluster inside refine, which is why
// RecordDeposit returns the sequence and a deposit has no owner.
func TestTheRefinerDoesNotFlyTheirOwnClusterTwice(t *testing.T) {
	_, a, _ := twoInARoom(t)
	grp := refineFrom(t, a, 1)

	tick(a, 1)
	if got, want := len(a.Flights.Active()), len(grp.NumberIDs); got != want {
		t.Errorf("the refiner is flying %d digits for a cluster of %d", got, want)
	}
}

func TestAJoinerDoesNotReplayOldDeposits(t *testing.T) {
	room, a, _ := twoInARoom(t)
	refineFrom(t, a, 3)

	c := JoinRoom(room, 99)
	if c == nil {
		t.Fatal("could not seat a third refiner")
	}
	c.Resize(120, 40)
	tick(c, 1)

	if n := len(c.Flights.Active()); n != 0 {
		t.Errorf("somebody who just sat down is flying %d digits from a refinement "+
			"that was over before they arrived", n)
	}
}

func TestADepartureIsForgottenOnceEverybodyHasFlownIt(t *testing.T) {
	room := NewRoom(3)
	room.RecordDeposit(0, []int{1, 2, 3}, []rune{'4', '5', '6'})
	if len(room.deposits) != 1 {
		t.Fatalf("the board is holding %d deposits, want 1", len(room.deposits))
	}

	room.Step(depositLife + 0.1)
	if len(room.deposits) != 0 {
		t.Errorf("a deposit older than %v is still on the board", depositLife)
	}
}

// Flights are only stepped while the grid is drawn, so one launched behind a
// diversion would hold the lid open until it ended. The bin fills on the
// board's own clock.
func TestADivertedRefinerMissesTheFlightAndNotTheCredit(t *testing.T) {
	_, a, b := twoInARoom(t)

	typeWord(b, theWord)
	b.Update(key("enter"))
	for i := 0; i < 60*4 && b.transitioning(); i++ {
		tick(b, 1)
	}
	if !b.Diverting() {
		t.Skip("the diversion did not start")
	}

	refineFrom(t, a, 4)
	tick(b, 2)

	if n := len(b.Flights.Active()); n != 0 {
		t.Errorf("a refiner inside a diversion is flying %d digits over it", n)
	}

	before := b.Grid.Bins[4].Refined
	for i := 0; i < 60*3; i++ {
		tick(a, 1)
		tick(b, 1)
	}
	if b.Grid.Bins[4].Refined <= before {
		t.Error("the bin was never credited for a refinement somebody did not watch")
	}
}
