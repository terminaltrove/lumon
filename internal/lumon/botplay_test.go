package lumon

import (
	"testing"
)

// watchFor watches four computer refiners for secs, calling each every frame.
func watchFor(t *testing.T, secs int, each func(room *Room)) (*Room, *Model) {
	t.Helper()
	room, m := withCompany(t, 11, 0)
	m.Update(key("w"))
	for range secs * 60 {
		tick(m, 1)
		each(room)
	}
	return room, m
}

// refines is each computer refiner's refines: the second it happened, and how
// many clusters it took at once.
type refineLog map[int][][2]float64

func logRefines(log refineLog, last map[int]int, room *Room) {
	for _, d := range bots(room) {
		if n := d.Credit - last[d.Seat]; n > 0 {
			log[d.Seat] = append(log[d.Seat], [2]float64{room.stepped, float64(n)})
		}
		last[d.Seat] = d.Credit
	}
}

// Watching, all four desks are computer refiners at work.
func TestWatchingFillsEveryDesk(t *testing.T) {
	room, _ := watchFor(t, 1, func(*Room) {})
	if n := len(bots(room)); n != MaxDesks {
		t.Fatalf("%d computer refiners while watching", n)
	}
}

// A refiner is not a machine: each stops refining for a while now and then,
// and the gaps between refines are irregular.
func TestComputerRefinersTakeBreaksAndKeepNoRhythm(t *testing.T) {
	log, last := refineLog{}, map[int]int{}
	watchFor(t, 180, func(room *Room) { logRefines(log, last, room) })
	for seat, refines := range log {
		if len(refines) < 5 {
			t.Errorf("seat %d refined only %d times in three minutes", seat, len(refines))
			continue
		}
		longest, sum, sumSq := 0.0, 0.0, 0.0
		for i := 1; i < len(refines); i++ {
			gap := refines[i][0] - refines[i-1][0]
			longest = max(longest, gap)
			sum += gap
			sumSq += gap * gap
		}
		n := float64(len(refines) - 1)
		mean := sum / n
		spread := sumSq/n - mean*mean
		if longest < 8 {
			t.Errorf("seat %d never paused for 8s in three minutes (longest gap %.1fs)", seat, longest)
		}
		if spread < mean*mean*0.25 {
			t.Errorf("seat %d refines like a metronome: mean gap %.1fs, variance %.2f", seat, mean, spread)
		}
	}
}

// Most refines take one cluster; now and then a refiner gathers several first.
func TestComputerRefinersSometimesGatherSeveralClusters(t *testing.T) {
	log, last := refineLog{}, map[int]int{}
	gathered := 0
	watchFor(t, 180, func(room *Room) {
		logRefines(log, last, room)
		for _, d := range bots(room) {
			if len(d.Reach) >= 2 {
				gathered++
			}
		}
	})
	single, several := 0, 0
	for _, refines := range log {
		for _, r := range refines {
			if r[1] == 1 {
				single++
			} else {
				several++
			}
		}
	}
	if gathered == 0 || several == 0 {
		t.Fatalf("no refiner ever gathered several clusters (%d frames reaching for several, %d multiple refines)", gathered, several)
	}
	if several*3 > single {
		t.Fatalf("gathering is the habit, not the exception: %d single refines, %d multiple", single, several)
	}
}

// A refiner sometimes looks a cluster over and moves on without refining it.
func TestComputerRefinersSometimesLookAndMoveOn(t *testing.T) {
	reached := map[int]int{} // seat: the cluster it last reached for alone
	movedOn := 0
	watchFor(t, 180, func(room *Room) {
		for _, d := range bots(room) {
			was := reached[d.Seat]
			now := -1
			if len(d.Reach) == 1 {
				now = d.Reach[0]
			}
			if was > 0 && now != was && room.IsActive(was) {
				movedOn++
			}
			reached[d.Seat] = now
		}
	})
	if movedOn == 0 {
		t.Fatal("no refiner ever left a cluster it was looking at")
	}
}

// Watching, 1-4 put the camera on that desk's refiner and keep it there, Tab
// moves it to the next, and 0 lets it choose again.
func TestTheWatcherChoosesWhoToFollow(t *testing.T) {
	_, m := watchFor(t, 2, func(*Room) {})
	m.Update(key("3"))
	tick(m, 60*15) // longer than the camera would stay with anyone by itself
	if m.followSeat != 2 || !m.followPinned {
		t.Fatalf("3 follows seat %d, pinned %v", m.followSeat, m.followPinned)
	}
	m.Update(key("tab"))
	if m.followSeat != 3 {
		t.Fatalf("tab after seat 3 follows seat %d", m.followSeat+1)
	}
	m.Update(key("0"))
	if m.followPinned {
		t.Fatal("0 left the camera pinned")
	}
	if m.Desk != nil {
		t.Fatal("a follow key sat the watcher down")
	}
}

// Getting up to watch fills every free desk, whatever company there was.
func TestWatchingFillsEveryDeskWhateverTheCompany(t *testing.T) {
	room, m := withCompany(t, 11, 2)
	m.Update(key("w"))
	if n := len(bots(room)); n != MaxDesks {
		t.Fatalf("%d computer refiners after getting up from a board of three", n)
	}
}

// Four computer refiners keep all five bins going, the fifth with no desk of
// its own included, each refine going to the bin furthest behind.
func TestComputerRefinersFillEveryBinEvenly(t *testing.T) {
	room, _ := watchFor(t, 120, func(*Room) {})
	lo, hi := 2.0, 0.0
	for i, b := range room.Grid.Bins {
		f := b.Percentage()
		if b.Refined == 0 {
			t.Errorf("bin %d took nothing in two minutes", i+1)
		}
		lo, hi = min(lo, f), max(hi, f)
	}
	if hi-lo > 0.1 {
		t.Errorf("the bins are %.0f%% to %.0f%% full; they should fill together", 100*lo, 100*hi)
	}
}

// With the bins level, a refiner's cluster goes to its own bin, or the
// nearest of those furthest behind; never to one that is further along.
func TestARefinerSendsItsClusterToTheBinFurthestBehind(t *testing.T) {
	r := NewRoom(1)
	for i, b := range r.Grid.Bins {
		b.Quota, b.Refined = 20, 10
		if i == 4 {
			b.Refined = 4
		}
	}
	if got := r.openBinNear(0); got != 4 {
		t.Errorf("bin 5 is furthest behind and a cluster went to bin %d", got+1)
	}
	r.Grid.Bins[4].Refined = 10
	if got := r.openBinNear(2); got != 2 {
		t.Errorf("with the bins level a cluster went to bin %d, not its refiner's own, 3", got+1)
	}
	for _, b := range r.Grid.Bins {
		b.Refined = b.Quota
	}
	if got := r.openBinNear(2); got != -1 {
		t.Errorf("every bin is full and a cluster went to bin %d", got+1)
	}
}
