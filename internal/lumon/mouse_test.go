package lumon

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// press, drag and release are the parts of a mouse gesture on the grid, which
// tells a click from a ringfence by whether the pointer moved between press and
// release. Everywhere else (lobby, scoreboard, overlays) acts on the press alone,
// so click is a press.
func click(x, y int) tea.MouseMsg { return press(x, y) }

func press(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
}

func drag(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft}
}

func release(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft}
}

// tap is a press and release on one cell, which on the grid is a click.
func tap(m *Model, x, y int) {
	m.Update(press(x, y))
	m.Update(release(x, y))
}

// overTheGrid is a screen position inside the grid area, where HitTest resolves
// a cell whether or not there is a board behind it.
func overTheGrid(m *Model) (int, int) {
	l := m.Layout
	return l.GridOriginX + l.ViewportW*CellWidth/2, l.GridTop + l.ViewportH*RowPitch/2
}

// The diversions are not in this table: each is a mode of the grid, drawn
// inside its panel, so they have no separate mouse path. A session has no board
// until it takes a desk, and Model embeds *Room, so every promoted access is a
// nil dereference until then.
func TestTheMouseIsSafeOnEveryScreen(t *testing.T) {
	for _, c := range []struct {
		name  string
		build func() *Model
	}{
		{"boot sequence", func() *Model {
			m := named(NewSession(NewRooms(1), 1), "mark")
			m.Resize(120, 40)
			m.StartIntro()
			return m
		}},
		{"name screen", func() *Model {
			m := named(NewSession(NewRooms(1), 1), "mark")
			m.Resize(120, 40)
			return m
		}},
		{"lobby", func() *Model {
			rs := NewRooms(1)
			rs.Open()
			m := named(NewSession(rs, 1), "mark")
			m.Resize(120, 40)
			m.Update(key("enter")) // accept the name
			return m
		}},
		{"screensaver", func() *Model {
			m := ready()
			m.Update(key("i"))
			return m
		}},
		{"the scoreboard", func() *Model {
			m := ready()
			m.Update(key("s"))
			return m
		}},
		{"the grid", func() *Model { return ready() }},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := c.build()
			x, y := overTheGrid(m)

			// Motion fires constantly because the program runs with mouse cell motion, and
			// the press is what was reported.
			m.Update(mouseMotion(x, y))
			m.Update(click(x, y))
			m.Update(mouseMotion(x+1, y+1))

			// It must still draw afterwards.
			_ = drawTo(m)
		})
	}
}

func TestAClickSkipsTheBootSequence(t *testing.T) {
	m := named(NewSession(NewRooms(1), 1), "mark")
	m.Resize(120, 40)
	m.StartIntro()
	tick(m, 5)

	x, y := overTheGrid(m)
	m.Update(click(x, y))

	if m.Screen == ScreenIntro {
		t.Error("a click did not skip the boot sequence, though any key does")
	}
}

func TestAClickWakesTheScreensaver(t *testing.T) {
	m := ready()
	m.Update(key("i"))
	if m.Screen != ScreenIdle {
		t.Fatal("did not enter the screensaver")
	}

	x, y := overTheGrid(m)
	m.Update(click(x, y))

	if m.Screen != ScreenMain {
		t.Error("a click did not wake the screensaver, though any key does")
	}
}

func TestAClickOnTheNameScreenChangesNothing(t *testing.T) {
	m := named(NewSession(NewRooms(1), 1), "mark")
	m.Resize(120, 40)
	before := m.Name

	x, y := overTheGrid(m)
	m.Update(click(x, y))

	if m.Screen != ScreenName {
		t.Errorf("a click moved off the name screen to %v", m.Screen)
	}
	if m.Name != before {
		t.Errorf("a click changed the name from %q to %q", before, m.Name)
	}
}

// There is one card in the middle and nothing to hover: clicking the card takes
// it, and clicking off it does nothing.
func TestTheCardCanBeClicked(t *testing.T) {
	rs := NewRooms(31)
	m := named(NewSession(rs, 1), "mark")
	m.Resize(120, 40)
	m.Update(key("enter")) // into the drawer
	riffled(m)

	box, ok := cardFits(m.Layout.Width, m.Layout.Height)
	if !ok {
		t.Fatal("the card does not fit a 120x40 terminal")
	}
	want, _ := m.rolodex().Selected()

	// Off the card first: the heading, which must do nothing.
	m.Update(click(6, 1))
	if m.Room != nil {
		t.Fatal("clicking the heading took a file")
	}

	m.Update(click(box.cx, box.hinge))
	if m.Room == nil {
		t.Fatal("clicking the card did not take it")
	}
	if m.FileName != want.Name {
		t.Errorf("took %q, want the card that was showing, %q", m.FileName, want.Name)
	}
}

func TestAClickBelowTheLobbyListJoinsNothing(t *testing.T) {
	rs := NewRooms(37)
	rs.Open()

	m := named(NewSession(rs, 1), "mark")
	m.Resize(120, 40)
	m.Update(key("enter"))

	m.Update(click(6, m.Layout.Height-1)) // the hint row

	if m.Room != nil {
		t.Error("a click off the list joined a file")
	}
}

// Reachable through the exported API. finishIntro checks whether the session
// has a board.
func TestASessionWithNeitherBoardNorLobbySurvives(t *testing.T) {
	m := named(NewSession(nil, 1), "mark")
	m.Resize(120, 40)
	m.StartIntro()

	for i := 0; i < 60*30 && m.Screen == ScreenIntro; i++ {
		tick(m, 1)
	}

	if m.Screen == ScreenMain && m.Room == nil {
		t.Error("a session with no board was sent to the grid")
	}

	// Nothing it can be sent may kill it.
	x, y := overTheGrid(m)
	m.Update(key("enter"))
	m.Update(click(x, y))
	m.Update(mouseMotion(x, y))
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	tick(m, 60)
	_ = drawTo(m)
}

// Draw and the step path must agree on whether the boot sequence is there.
func TestAMissingIntroDoesNotKillTheStepPath(t *testing.T) {
	m := ready()
	m.Screen = ScreenIntro
	m.Intro = nil

	tick(m, 5)
	_ = drawTo(m)

	if m.Screen == ScreenIntro {
		t.Error("a session stuck on a boot sequence that is not there")
	}
}

// Otherwise a session that left goes on locking and stepping that board.
func TestLeavingGivesTheBoardBack(t *testing.T) {
	room := NewRoom(3)
	m := JoinRoom(room, 1)
	m.Resize(120, 40)

	m.Leave()

	if m.Room != nil {
		t.Error("a session that left is still holding the board")
	}
	if m.Desk != nil {
		t.Error("a session that left is still holding a desk")
	}
	// It must still be drivable without a board.
	x, y := overTheGrid(m)
	m.Update(click(x, y))
	m.Update(mouseMotion(x, y))
	tick(m, 30)
	_ = drawTo(m)
}

// A press and release on one cell is a click; anything that moved before the
// release is a ringfence.
func TestADragSweepsAndAClickStillRefines(t *testing.T) {
	m := ready()
	m.Resize(160, 50)
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	tick(m, 30)
	drawTo(m)

	l := m.Layout
	at := func(id int) (int, int) {
		return l.ScreenOf(id%GridSize, id/GridSize)
	}

	// A drag across the cluster holds it and refines nothing.
	x0, y0 := at(grp.NumberIDs[0])
	x1, y1 := at(grp.NumberIDs[len(grp.NumberIDs)-1])
	m.Update(press(x0, y0))
	m.Update(drag(x1, y1))
	m.Update(release(x1, y1))

	if !m.held[grp.ID] {
		t.Errorf("a drag across the cluster did not ringfence it; held is %v", m.held)
	}
	if m.Grid.Group(grp.ID) == nil {
		t.Error("a drag refined the cluster; it should only have held it")
	}
	if m.sweep != nil {
		t.Error("the sweep outlived the button")
	}

	// A click on one cell refines.
	m.Update(key("right")) // let go
	drawTo(m)
	x, y := at(grp.NumberIDs[0])
	m.Update(press(x, y))
	m.Update(release(x, y))

	if m.Grid.Group(grp.ID) != nil {
		t.Error("a click on a cluster member did not refine it")
	}
}
