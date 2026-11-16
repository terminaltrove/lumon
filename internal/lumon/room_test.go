package lumon

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"
)

// Model embeds *Room, so a name Model declares that Room also uses silently
// shadows Room's, and the view reads a stale copy of the board. Go reports an
// ambiguity only at equal depth. If this fails, rename or remove one of the two;
// do not rename the test.
func TestModelAndRoomHaveNoNamesInCommon(t *testing.T) {
	modelType := reflect.TypeOf(&Model{}).Elem()
	roomType := reflect.TypeOf(&Room{}).Elem()
	for i := 0; i < modelType.NumField(); i++ {
		f := modelType.Field(i)
		if f.Anonymous {
			continue
		}
		if _, clash := roomType.FieldByName(f.Name); clash {
			t.Errorf("Model.%s shadows Room.%s", f.Name, f.Name)
		}
	}

	// Methods, by reading the package: a promoted method and a declared one look the
	// same through reflection.
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", nil, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	declared := map[string]map[string]bool{"Model": {}, "Room": {}}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
					continue
				}
				star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
				if !ok {
					continue
				}
				ident, ok := star.X.(*ast.Ident)
				if !ok {
					continue
				}
				if set, watched := declared[ident.Name]; watched {
					set[fn.Name.Name] = true
				}
			}
		}
	}

	if len(declared["Room"]) == 0 {
		t.Fatal("found no methods on Room; the parse is not working")
	}
	for name := range declared["Model"] {
		if declared["Room"][name] {
			t.Errorf("Model.%s shadows Room.%s, the board's own version will never run", name, name)
		}
	}
}

func TestTwoViewsShareOneBoard(t *testing.T) {
	room := NewRoom(3)
	room.SetDesks(2)

	a, b := JoinRoom(room, 1), JoinRoom(room, 2)
	a.Resize(120, 40)
	b.Resize(80, 24) // deliberately a different terminal

	if a.Grid != b.Grid {
		t.Fatal("the two views are looking at different grids")
	}
	if a.FileName != b.FileName {
		t.Errorf("different files: %q and %q", a.FileName, b.FileName)
	}

	// Wake a cluster and refine it from A; B must lose it too.
	grp := a.FirstActive()
	for i := 0; grp == nil && i < 600; i++ {
		tick(a, 1)
		grp = a.FirstActive()
	}
	if grp == nil {
		t.Skip("no cluster woke")
	}
	if !b.IsActive(grp.ID) {
		t.Error("the cluster is awake for one player and not the other")
	}

	id := grp.NumberIDs[0]
	a.CursorX, a.CursorY = id%GridSize, id/GridSize
	a.Update(key("enter"))

	if b.IsActive(grp.ID) {
		t.Error("the cluster is still awake for the other player after being refined")
	}
	if got := b.Grid.At(id%GridSize, id/GridSize).BadGroupID; got >= 0 {
		t.Errorf("the refined cell still reads as corrupt to the other player (group %d)", got)
	}
}

func TestCreditIsCountedOncePerBoard(t *testing.T) {
	room := NewRoom(3)
	a, b := JoinRoom(room, 1), JoinRoom(room, 2)
	a.Resize(120, 40)
	b.Resize(120, 40)
	a.SelectedBin = 2

	grp := a.FirstActive()
	for i := 0; grp == nil && i < 600; i++ {
		tick(a, 1)
		grp = a.FirstActive()
	}
	if grp == nil {
		t.Skip("no cluster woke")
	}
	before := room.Grid.Bins[2].Refined

	id := grp.NumberIDs[0]
	a.CursorX, a.CursorY = id%GridSize, id/GridSize
	a.Update(key("enter"))

	for i := 0; i < 240; i++ {
		tick(a, 1)
		tick(b, 1)
	}

	if got := room.Grid.Bins[2].Refined; got != before+1 {
		t.Errorf("bin credited %d times, want once", got-before)
	}
}

func TestARoomStartsWithOneDesk(t *testing.T) {
	if got := NewRoom(1).DeskCount(); got != 1 {
		t.Errorf("a new room has %d desks, want 1", got)
	}
}

func TestLeavingUnseatsAView(t *testing.T) {
	room := NewRoom(3)
	a := JoinRoom(room, 1)
	a.Resize(120, 40)

	if got := room.Seated(); got != 1 {
		t.Fatalf("%d seated after one join, want 1", got)
	}
	a.Leave()
	if got := room.Seated(); got != 0 {
		t.Errorf("%d still seated after leaving, want 0", got)
	}
}

func TestAJoinerPicksUpTheLiveClusters(t *testing.T) {
	room := NewRoom(3)
	a := JoinRoom(room, 1)
	a.Resize(120, 40)

	grp := a.FirstActive()
	for i := 0; grp == nil && i < 600; i++ {
		tick(a, 1)
		grp = a.FirstActive()
	}
	if grp == nil {
		t.Skip("no cluster woke")
	}

	b := JoinRoom(room, 2)
	if !b.Agitation.Tracked(grp.ID) {
		t.Error("the joiner has no body for the cluster that is already awake")
	}
}

// Four goroutines, as four remote sessions would be. Run this under -race.
func TestFourPlayersOnOneBoardConcurrently(t *testing.T) {
	room := NewRoom(21)
	room.SetDesks(4)

	const players = 4
	views := make([]*Model, players)
	for i := range views {
		views[i] = JoinRoom(room, int64(i+1))
		// Different terminal sizes.
		views[i].Resize(90+i*30, 26+i*6)
		views[i].SelectedBin = i % BinCount
	}

	done := make(chan struct{})
	for _, m := range views {
		go func(m *Model) {
			defer close2(done)
			for i := 0; i < 400; i++ {
				m.Update(FrameMsg(frameInterval))
				_ = m.View()
				if i%20 == 0 {
					// The board is read under its own lock, as the render path does.
					m.Room.mu.RLock()
					var target int = -1
					if grp := m.FirstActive(); grp != nil {
						target = grp.NumberIDs[0]
					}
					m.Room.mu.RUnlock()

					if target >= 0 {
						// The cursor is this player's own, so it needs no lock.
						m.CursorX, m.CursorY = target%GridSize, target/GridSize
						m.Update(key("enter"))
					}
				}
			}
		}(m)
	}
	for i := 0; i < players; i++ {
		<-done
	}

	// No bin past its quota, and the file never past 100%.
	for i := range room.Grid.Bins {
		b := room.Grid.Bins[i]
		if b.Refined > b.Quota {
			t.Errorf("bin %d took %d clusters against a quota of %d", i+1, b.Refined, b.Quota)
		}
	}
	if got := views[0].File.Percent(room.Grid); got > 100 {
		t.Errorf("the file reports %d%%", got)
	}
}

// close2 lets several goroutines signal on one channel without closing it twice.
func close2(ch chan struct{}) { ch <- struct{}{} }
