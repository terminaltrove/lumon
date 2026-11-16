package lumon

import "testing"

func TestOpeningABinByHandRaisesItsLid(t *testing.T) {
	m := ready()
	m.SelectedBin = 2
	tick(m, 30)
	if m.BinFill.Ajar(2) {
		t.Fatal("the bin was already open; this is not testing anything")
	}

	m.Update(key("o"))
	tick(m, 30)
	if !m.BinFill.Ajar(2) {
		t.Errorf("after o the lid is %v degrees, want it open", m.BinFill.Lid(2))
	}

	// The same key puts it down again.
	m.Update(key("o"))
	tick(m, 60)
	if m.BinFill.Ajar(2) {
		t.Errorf("a second o left the lid at %v degrees", m.BinFill.Lid(2))
	}
}

func TestOpeningABinOpensTheSelectedOne(t *testing.T) {
	m := ready()
	m.Update(key("4"))
	m.Update(key("o"))
	tick(m, 30)

	for i := 0; i < BinCount; i++ {
		if open := m.BinFill.Ajar(i); open != (i == 3) {
			t.Errorf("bin %d is open=%v after opening bin 4", i+1, open)
		}
	}
}

// The lid's target is recomputed from the live flights every frame, so a hold
// written straight into the spring would last one frame. The hold is OR'd into
// the target, so a deposit during a manual open keeps the lid up, and a hold
// that runs out while digits are in the air leaves the flights holding it.
func TestAManualOpenDoesNotFightADeposit(t *testing.T) {
	m := ready()
	grp := grownCluster(t, m)
	m.SelectedBin = 0

	m.Update(key("o"))
	tick(m, 30)
	if !m.BinFill.Ajar(0) {
		t.Fatal("the manual open did not take")
	}

	// The lid must never drop during the flight, whichever of the two holds it.
	id := grp.NumberIDs[0]
	m.CursorX, m.CursorY = id%GridSize, id/GridSize
	m.Update(key("enter"))
	for i := 0; i < 240; i++ {
		tick(m, 1)
		if !m.BinFill.Ajar(0) && m.BinFill.Receiving(0) {
			t.Fatalf("frame %d: digits are on their way and the lid is shut", i)
		}
	}
}

// The drawer comes from refining something, and it stays down while anything is
// on its way.
func TestOpeningABinByHandDoesNotOpenTheDrawer(t *testing.T) {
	m := ready()
	m.SelectedBin = 1
	m.Update(key("o"))
	tick(m, 120)

	if m.BinFill.Ajar(1) == false {
		t.Fatal("the lid did not open")
	}
	if m.Drawer.Showing(1) {
		t.Error("opening the bin by hand raised the temper drawer too")
	}
}

// A key wants Hold, a toggle. FOUNTAIN and FIREWORKS say every frame which bins
// are open and want SetHold. Hold is written on SetHold so the two cannot
// disagree about what holding is.
func TestSettingAHoldIsNotAToggle(t *testing.T) {
	b := NewBinFill()

	b.SetHold(2, true)
	b.SetHold(2, true)
	if !b.Holding(2) {
		t.Error("asking twice for a bin to be open shut it again")
	}
	b.SetHold(2, false)
	b.SetHold(2, false)
	if b.Holding(2) {
		t.Error("asking twice for a bin to be shut opened it again")
	}

	// Hold still toggles.
	b.Hold(3)
	if !b.Holding(3) {
		t.Error("a first press did not open bin 3")
	}
	b.Hold(3)
	if b.Holding(3) {
		t.Error("a second press did not shut bin 3 again")
	}

	// Neither reaches past the row.
	for _, i := range []int{-1, BinCount, 99} {
		b.SetHold(i, true)
		b.Hold(i)
		if b.Holding(i) {
			t.Errorf("bin %d is being held and does not exist", i)
		}
	}
}
