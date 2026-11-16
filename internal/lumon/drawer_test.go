package lumon

import (
	"strings"
	"testing"
)

// refineIntoBin refines the first visible cluster into bin i and returns it.
func refineIntoBin(t *testing.T, m *Model, i int) *BadGroup {
	t.Helper()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	id := grp.NumberIDs[0]
	m.CursorX, m.CursorY = id%GridSize, id/GridSize
	m.SelectedBin = i
	m.Update(key("enter"))
	return grp
}

// fillTo puts refined digits in bin i, and gives m a drawer that has seen
// them, so they raise nothing.
func fillTo(m *Model, i, refined int) {
	m.Room.mu.Lock()
	m.Grid.Bins[i].Refined = refined
	m.Room.mu.Unlock()
	m.Drawer = NewDrawer()
	tick(m, 1)
}

// runUntilDrawerOpens ticks until bin i's drawer is all the way out, or fails.
func runUntilDrawerOpens(t *testing.T, m *Model, i int) {
	t.Helper()
	for n := 0; n < 60*20; n++ {
		tick(m, 1)
		if m.Drawer.Out(i) > 0.98 {
			return
		}
	}
	t.Fatalf("bin %d's drawer never opened", i+1)
}

// shortOfAQuarter is what bin i holds one refine short of its first quarter.
func shortOfAQuarter(m *Model, i int) int { return (m.Grid.Bins[i].Quota+3)/4 - 1 }

// raiseDrawer refines a cluster into bin i, one refine short of a quarter, and
// runs until its drawer is out.
func raiseDrawer(t *testing.T, m *Model, i int) {
	t.Helper()
	fillTo(m, i, shortOfAQuarter(m, i))
	refineIntoBin(t, m, i)
	runUntilDrawerOpens(t, m, i)
}

// A cluster that takes a bin to no new quarter opens its lid and leaves its
// drawer down.
func TestTheDrawerStaysDownBetweenQuarters(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	fillTo(m, 2, shortOfAQuarter(m, 2)+1)
	refineIntoBin(t, m, 2)
	opened := false
	for n := 0; n < 60*6; n++ {
		tick(m, 1)
		opened = opened || m.BinFill.Ajar(2)
		if m.Drawer.Showing(2) {
			t.Fatalf("frame %d: the drawer came up at %d%%", n, m.Grid.Bins[2].PercentInt())
		}
	}
	if !opened {
		t.Error("the lid never opened for the digits")
	}
	if p := m.Grid.Bins[2].PercentInt(); p < 26 || p >= 50 {
		t.Errorf("the bin went to %d%%; the test means it to rise between quarters", p)
	}
}

// Reaching a quarter raises the drawer: it rises out of the lid the digits
// came in by, blinks all four bars, each blink sounding, as the show's does,
// and goes back down before the lid shuts.
func TestAQuarterRaisesTheDrawerWhichBlinksAndGoesDown(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	fillTo(m, 2, shortOfAQuarter(m, 2))
	refineIntoBin(t, m, 2)

	var blinks int
	dark, lit := false, false
	for n := 0; n < 60*8; n++ {
		cues := heard(m, 1)
		if v := volumeOf(cues, SoundBlink); v >= 0 {
			blinks++
		}
		if m.Drawer.Showing(2) && !m.BinFill.Ajar(2) {
			t.Fatalf("frame %d: the lid shut over a drawer %.2f out", n, m.Drawer.Out(2))
		}
		if m.Drawer.Out(2) > 0.9 {
			buf := drawTo(m)
			for y := range m.Layout.BinTop {
				row := buf.Row(y)
				if !strings.Contains(row, "WO") {
					continue
				}
				if strings.ContainsRune(row, '▕') {
					lit = true
				} else if m.Drawer.Dark(2) {
					dark = true
				}
			}
		}
	}
	if m.Grid.Bins[2].PercentInt() < 25 {
		t.Fatalf("the bin reached only %d%%", m.Grid.Bins[2].PercentInt())
	}
	if blinks != drawerBlinks {
		t.Errorf("the drawer's blinks sounded %d times, want %d", blinks, drawerBlinks)
	}
	if !lit || !dark {
		t.Errorf("up, the bars were drawn %v and taken off %v; a blink does both", lit, dark)
	}
	if m.Drawer.Showing(2) || m.BinFill.Ajar(2) {
		t.Error("the drawer and lid never went back down")
	}
}

// However the clusters' tempers fall, a bin filled to its quota holds every
// temper's share, so all four bars are full at 100%.
func TestAFullBinHasEveryTemperFull(t *testing.T) {
	for _, quota := range []int{55, 56, 7, 1} {
		for _, every := range []Temper{Woe, Malice} {
			bin := &Bin{Quota: quota}
			total := 0
			for bin.Refined < bin.Quota {
				bin.take(every)
				total++
				for u := range Temper(TemperCount) {
					if bin.Tempers[u] > bin.TemperShare(u) {
						t.Fatalf("quota %d: %v holds %d of its share %d", quota, u, bin.Tempers[u], bin.TemperShare(u))
					}
				}
			}
			g := &Grid{}
			g.Bins[0] = bin
			for u := range Temper(TemperCount) {
				if bin.Tempers[u] != bin.TemperShare(u) || temperShare(g, 0, u) != 1 {
					t.Errorf("quota %d, every cluster %v: full, %v holds %d of %d", quota, every, u, bin.Tempers[u], bin.TemperShare(u))
				}
			}
		}
	}
}

// A full bin's bars are full even when what it holds is uneven, as a save
// from before the shares, or a quota shrunk by a detonation, can leave it.
func TestAFullBinDrawsEveryBarFullWhateverItHolds(t *testing.T) {
	g := &Grid{}
	g.Bins[0] = &Bin{Quota: 8, Refined: 8, Tempers: [TemperCount]int{8, 0, 0, 0}}
	for u := range Temper(TemperCount) {
		if got := temperShare(g, 0, u); got != 1 {
			t.Errorf("a full bin draws %v at %.2f", u, got)
		}
	}
}

// A bin's percent going up is heard, quietly, each time.
func TestABinsPercentRisingIsHeard(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	refineIntoBin(t, m, 2)
	rises := 0
	for range 60 * 4 {
		if v := volumeOf(heard(m, 1), SoundBinRise); v >= 0 {
			rises++
			if v > 0.6 {
				t.Errorf("the percent rose at volume %v", v)
			}
		}
	}
	if rises != 1 {
		t.Errorf("one cluster's credit was heard %d times", rises)
	}
}

// A temper's bar grows to what the bin holds as the digits land, not in one
// step.
func TestATempersBarGrowsAsTheDigitsLand(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	refineIntoBin(t, m, 2)
	bin := m.Grid.Bins[2]
	before := bin.Tempers

	var grew Temper = -1
	for n := 0; n < 60*5 && grew < 0; n++ {
		tick(m, 1)
		for t := range Temper(TemperCount) {
			if bin.Tempers[t] > before[t] {
				grew = t
			}
		}
	}
	if grew < 0 {
		t.Fatal("no temper was credited")
	}
	want := temperShare(m.Grid, 2, grew)
	if got := m.Drawer.Share(2, grew); got > want-0.01 {
		t.Errorf("the bar jumped to %.3f of %.3f the moment it was credited", got, want)
	}
	tick(m, 120)
	if got := m.Drawer.Share(2, grew); got < want-0.01 {
		t.Errorf("the bar reached only %.3f of %.3f", got, want)
	}
}

// Labelled, in Kier's order.
func TestTheDrawerNamesTheFourTempers(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	raiseDrawer(t, m, 2)

	buf := drawTo(m)
	l := m.Layout
	var panel []string
	for y := l.GridTop; y < l.BinTop; y++ {
		panel = append(panel, buf.Row(y))
	}
	all := strings.Join(panel, "\n")

	// Every one of them, in order down the panel.
	at := -1
	for _, name := range temperNames {
		i := strings.Index(all, name)
		if i < 0 {
			t.Fatalf("the drawer does not name %s:\n%s", name, all)
		}
		if i < at {
			t.Errorf("%s appears above the temper before it; the four are Kier's "+
				"order and the panel must keep it:\n%s", name, all)
		}
		at = i
	}

	// And the bin's own number, so a panel clear of the five bins says whose it is.
	if !strings.Contains(all, binLabel(m.Grid.Bins[2].ID)) {
		t.Errorf("the drawer does not say which bin it belongs to:\n%s", all)
	}
}

// The drawer is painted after the grid and the rule, and a box paints only its
// edges, so without clearing behind it the digits show through.
func TestTheDrawerIsOpaque(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	raiseDrawer(t, m, 2)

	buf := drawTo(m)
	l := m.Layout
	x, w := l.BinOriginX(2), l.BinWidth(2)

	// Every interior row is the drawer's drawing or blank: no rule, no stray digit
	// from the board behind.
	for y := l.BinTop - drawerRows; y < l.BinTop-1; y++ {
		row := []rune(buf.Row(y))
		for cx := x + 1; cx < x+w-1; cx++ {
			if r := row[cx]; r == '═' {
				t.Fatalf("the rule shows through the drawer at (%d,%d)", cx, y)
			}
		}
	}
}

// Each bar is outlined in its temper's colour, as the show's are, in one row
// with no dots: hairline sides, and along what is still to fill a hairline
// foot under an overline.
func TestTheBarsAreOutlinedNotDotted(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	m.Room.mu.Lock()
	m.Grid.Bins[2].Tempers = [TemperCount]int{3, 0, 1, 2}
	m.Room.mu.Unlock()
	raiseDrawer(t, m, 2)
	for m.Drawer.Dark(2) {
		tick(m, 1)
	}
	buf := drawTo(m)
	l := m.Layout
	x, w := l.BinOriginX(2), l.BinWidth(2)
	sides, feet := 0, 0
	for y := l.BinTop - drawerRows; y < l.BinTop; y++ {
		row := []rune(buf.Row(y))
		for cx := x + 1; cx < x+w-1 && cx < len(row); cx++ {
			switch row[cx] {
			case '·':
				t.Fatalf("a bar is dotted:\n%s", buf.Row(y))
			case '▕', '▏':
				sides++
			case '▁':
				feet++
				if !buf.AttrAt(cx, y).Over {
					t.Errorf("an empty stretch of bar at (%d,%d) has no top", cx, y)
				}
			case '█':
				if buf.AttrAt(cx, y).Over {
					t.Errorf("the fill at (%d,%d) is overlined", cx, y)
				}
			}
		}
	}
	if sides != 2*TemperCount {
		t.Errorf("%d bar sides, want %d", sides, 2*TemperCount)
	}
	if feet == 0 {
		t.Error("no bar has anything left to fill; the test means some to")
	}
}

// The temper is computed from the file and the group, not stored, so two
// refiners agree with nothing exchanged and the save stays derivable.
func TestATempersIsAFunctionOfTheFileAndTheCluster(t *testing.T) {
	if a, b := TemperOf(7, 0, 3), TemperOf(7, 0, 3); a != b {
		t.Errorf("the same cluster gave %v then %v", a, b)
	}
	if TemperOf(7, 0, 3) == TemperOf(7, 1, 3) && TemperOf(7, 0, 9) == TemperOf(7, 1, 9) &&
		TemperOf(7, 0, 12) == TemperOf(7, 1, 12) {
		t.Error("three clusters kept their temper across a new file; the epoch is not in the hash")
	}

	// The four come up in roughly equal measure, or a bin could never be filled
	// evenly.
	var seen [TemperCount]int
	for id := 0; id < 4000; id++ {
		seen[TemperOf(11, 0, id)]++
	}
	for i, n := range seen {
		if n < 4000/TemperCount/2 {
			t.Errorf("%s came up %d times in 4000; the four are meant to be even",
				temperNames[i], n)
		}
	}
}

// The quotas need not survive a save, since they follow from what is still on
// the board; which tempers went in does not.
func TestABinsTempersSurviveASave(t *testing.T) {
	rs := NewRooms(404)
	room := rs.Open()

	room.mu.Lock()
	room.Grid.Bins[1].Tempers = [TemperCount]int{3, 1, 4, 2}
	room.Grid.Bins[1].Refined = 10
	room.mu.Unlock()

	back := restoreFloor(rs.capture(), 404).Find(room.FileName)
	if back == nil {
		t.Fatalf("the file %q is not on the restored floor", room.FileName)
	}
	if got, want := back.Grid.Bins[1].Tempers, [TemperCount]int{3, 1, 4, 2}; got != want {
		t.Errorf("the bin came back holding %v, want %v", got, want)
	}
}

// FC, not FR: the show's terminal abbreviates Frolic that way, and we follow it.
func TestFrolicIsAbbreviatedTheWayTheShowDoesIt(t *testing.T) {
	if temperNames[Frolic] != "FC" {
		t.Errorf("frolic is labelled %q, want \"FC\"", temperNames[Frolic])
	}
	want := [4]string{"WO", "FC", "DR", "MA"}
	if temperNames != want {
		t.Errorf("the columns are labelled %v, want %v", temperNames, want)
	}
}
