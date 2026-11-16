package lumon

import (
	"strings"
	"testing"
)

func TestANewFileStartsEmpty(t *testing.T) {
	m := ready()

	if got := m.File.Completion(m.Grid); got != 0 {
		t.Errorf("a fresh file is %v complete, want 0", got)
	}
	if got := m.File.Percent(m.Grid); got != 0 {
		t.Errorf("a fresh file reports %d%%, want 0", got)
	}
	if m.File.Complete(m.Grid) {
		t.Error("a fresh file reports itself finished")
	}
}

// The bins already know this, so there is nothing else to keep in step.
func TestAFileIsCompleteWhenEveryClusterIsRefined(t *testing.T) {
	m := ready()

	for _, bin := range m.Grid.Bins {
		bin.Refined = bin.Quota
	}

	if !m.File.Complete(m.Grid) {
		t.Errorf("every cluster is refined but the file is %v complete", m.File.Completion(m.Grid))
	}
	if got := m.File.Percent(m.Grid); got != 100 {
		t.Errorf("file reports %d%%, want 100", got)
	}
}

func TestRefiningAClusterAdvancesTheFile(t *testing.T) {
	m := ready()
	before := m.File.Completion(m.Grid)

	grownCluster(t, m)
	m.Update(key("enter"))
	tick(m, 900)

	if got := m.File.Completion(m.Grid); got <= before {
		t.Errorf("file is %v complete after refining a cluster, was %v", got, before)
	}
}

func TestTheBarLagsBehindTheNumber(t *testing.T) {
	m := ready()
	for _, bin := range m.Grid.Bins {
		bin.Refined = bin.Quota
	}

	if got := m.File.Percent(m.Grid); got != 100 {
		t.Fatalf("the number did not update at once: %d%%", got)
	}
	if got := m.File.Fill(); got > 0.01 {
		t.Errorf("the bar jumped straight to %v instead of easing", got)
	}

	for i := 0; i < 600; i++ {
		m.File.Step(1.0/60, m.Grid)
	}
	if got := m.File.Fill(); got < 0.99 {
		t.Errorf("the bar only reached %v after ten seconds", got)
	}
}

// A cluster left past its fuse takes one refinement's work back with it, so
// refining advances the bar and only a detonation moves it back.
func TestTheBarOnlyEverAdvancesUnlessSomethingDetonates(t *testing.T) {
	m := ready()
	grownCluster(t, m)
	m.Update(key("enter"))

	// Checked against the true completion, not a count of detonations. The bar is a
	// spring easing toward completion, so one detonation is a run of backward
	// steps while it settles; it may only move back because its target did.
	prev := m.File.Fill()
	for i := 0; i < 1800; i++ {
		tick(m, 1)
		got := m.File.Fill()
		if got < prev-1e-9 && m.File.Completion(m.Grid) >= prev {
			t.Fatalf("the bar went backwards from %v to %v with the file at %v",
				prev, got, m.File.Completion(m.Grid))
		}
		prev = got
	}
}

func TestTheSameSeedAlwaysDrawsTheSameFile(t *testing.T) {
	a, b := NewModel(42), NewModel(42)
	if a.File.Name != b.File.Name {
		t.Errorf("seed 42 gave %q and %q", a.File.Name, b.File.Name)
	}
	if NewModel(43).File.Name == a.File.Name {
		t.Log("seeds 42 and 43 happened to draw the same file, which is allowed")
	}
}

// Title Case, one or two words, nothing punctuated beyond a single period, no
// state or country suffix.
func TestFileNamesFollowThePattern(t *testing.T) {
	for _, name := range FileNames() {
		if name == "" {
			t.Fatal("there is an empty file name in the pool")
		}
		if strings.ContainsAny(name, ",0123456789-_/") {
			t.Errorf("%q has punctuation or digits the show never uses", name)
		}
		if words := strings.Fields(name); len(words) > 2 {
			t.Errorf("%q is %d words; the show never goes past two", name, len(words))
		}
		for _, word := range strings.Fields(name) {
			first := []rune(word)[0]
			if first < 'A' || first > 'Z' {
				t.Errorf("%q is not Title Case", name)
				break
			}
		}
	}
}

func TestTheFilePoolHasNoDuplicates(t *testing.T) {
	seen := map[string]bool{}
	for _, name := range FileNames() {
		if seen[name] {
			t.Errorf("%q appears twice in the pool", name)
		}
		seen[name] = true
	}
}

// Twenty-five files, twenty-five rooms on the testing floor.
func TestTheCompletedFilesAreTheTwentyFive(t *testing.T) {
	if got := len(CompletedFiles); got != 25 {
		t.Errorf("there are %d completed files, want 25", got)
	}
	if CompletedFiles[0] != "Allentown" {
		t.Errorf("the first file is %q, want Allentown", CompletedFiles[0])
	}
	if last := CompletedFiles[len(CompletedFiles)-1]; last != "Cold Harbor" {
		t.Errorf("the last file is %q, want Cold Harbor", last)
	}
}

// A room takes its name from this pool, and a floor holds a room for each.
func TestThereIsANameForEveryRoom(t *testing.T) {
	if got := len(FileNames()); got != MaxRooms {
		t.Errorf("%d names in the pool for %d rooms", got, MaxRooms)
	}
}

// However many files are asked for, no two rooms share a name, so the
// drawer can tell every one apart.
func TestAFullFloorNeverRepeatsAName(t *testing.T) {
	rs := NewRooms(3)
	rs.EnsureOpen(MaxRooms + 10)
	for range 10 {
		rs.Open()
		rs.Assign()
	}
	seen := map[string]bool{}
	for _, l := range rs.List() {
		if seen[l.Name] {
			t.Errorf("two rooms are on %s", l.Name)
		}
		seen[l.Name] = true
	}
	if len(seen) != MaxRooms {
		t.Errorf("the floor holds %d files, want all %d", len(seen), MaxRooms)
	}
}

// Without this you could send every cluster to bin one. Every bin is filled,
// since with only one full, nearestOpenBin moves the selection and the refine
// goes through. Grown first and filled after, because a complete file puts its
// clusters to sleep on the next tick, and no tick between the filling and the
// key, which would move the selection.
func TestAFullBinRefusesMore(t *testing.T) {
	m := ready()
	m.SelectedBin = 0

	grp := grownCluster(t, m)
	for _, b := range m.Grid.Bins {
		b.Refined = b.Quota
	}

	before := len(m.Flights.Active())
	m.Update(key("enter"))

	if got := len(m.Flights.Active()); got != before {
		t.Errorf("%d digits launched into a full bin", got-before)
	}
	if !m.IsActive(grp.ID) {
		t.Error("the cluster was consumed by a refine that should have been refused")
	}
}

func TestNoBinCanOverfill(t *testing.T) {
	m := ready()
	m.SelectedBin = 0

	for i := 0; i < 40; i++ {
		if grp := activateFirstVisibleGroup(m); grp != nil {
			m.CursorX, m.CursorY = grp.NumberIDs[0]%GridSize, grp.NumberIDs[0]/GridSize
			m.Update(key("enter"))
		}
		tick(m, 200)
		for i := range m.Grid.Bins {
			if got := m.Grid.Bins[i].PercentInt(); got > 100 {
				t.Fatalf("bin %d reports %d%%", i+1, got)
			}
		}
	}
}

func TestOneFullBinIsNotAFinishedFile(t *testing.T) {
	m := ready()
	m.Grid.Bins[0].Refined = m.Grid.Bins[0].Quota

	if m.File.Complete(m.Grid) {
		t.Error("the file reports itself finished with four bins still empty")
	}
	if got := m.File.Percent(m.Grid); got >= 100 {
		t.Errorf("file reports %d%% with one bin filled", got)
	}
}

// refineHere does nothing when the selected bin is full, and the sweep stops
// for the same reason, so pressing f would fail silently. Filled from a middle
// bin so left and right are both candidates; the tie goes to the lower index,
// the rule Layout uses for the leftover columns.
func TestAFullBinNeverKeepsTheSelection(t *testing.T) {
	m := ready()
	m.SelectedBin = 2
	m.Grid.Bins[2].Refined = m.Grid.Bins[2].Quota

	tick(m, 1)
	if m.SelectedBin == 2 {
		t.Fatal("the selection is still on the full bin; the refine key does nothing " +
			"and the screen does not say why")
	}
	if m.SelectedBin != 1 {
		t.Errorf("the selection moved to bin %d, want 1: bins 1 and 3 are both one "+
			"away and the tie goes to the lower", m.SelectedBin+1)
	}

	// It keeps moving as the board fills, which a hook on the bin keys alone would
	// miss: credit lands on a timer, and remotely somebody else fills a bin this
	// terminal points at.
	m.Grid.Bins[1].Refined = m.Grid.Bins[1].Quota
	tick(m, 1)
	if m.Grid.Bins[m.SelectedBin].Full() {
		t.Errorf("bin %d is full and still selected", m.SelectedBin+1)
	}

	// With nowhere to go it stays put. The file is finished at that point anyway.
	for _, b := range m.Grid.Bins {
		b.Refined = b.Quota
	}
	was := m.SelectedBin
	tick(m, 1)
	if m.SelectedBin != was {
		t.Errorf("with every bin full the selection moved from %d to %d", was, m.SelectedBin)
	}
}

func TestChoosingAFullBinLandsOnAnOpenOne(t *testing.T) {
	m := ready()
	m.Grid.Bins[0].Refined = m.Grid.Bins[0].Quota

	m.SelectedBin = 4
	m.Update(key("1"))
	if m.SelectedBin != 1 {
		t.Errorf("pressing 1 at a full bin one selected bin %d, want 2", m.SelectedBin+1)
	}

	// The click path is a separate assignment in handleMouse.
	m.SelectedBin = 4
	m.Update(press(m.Layout.BinOriginX(0)+2, m.Layout.BinTop+1))
	if m.SelectedBin == 0 {
		t.Errorf("clicking a full bin selected it")
	}
}
