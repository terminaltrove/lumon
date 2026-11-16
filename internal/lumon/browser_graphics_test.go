package lumon

import (
	"testing"

	"github.com/terminaltrove/lumon/internal/canvas"
)

func TestBrowserGraphicsFollowStaticLogoVisibilityAndTheme(t *testing.T) {
	m := sessionOnFile(t)
	m.KeepOpen()
	m.UseGraphics(GraphicsBrowser)
	m.View()
	images := m.BrowserImages()
	if len(images) != 1 || images[0].Asset != "lumon-mark" || len(images[0].Clips) == 0 {
		t.Fatalf("header image: %+v", images)
	}
	color := images[0].Color
	m.Update(key("t"))
	m.View()
	if m.BrowserImages()[0].Color == color {
		t.Fatal("logo did not follow theme")
	}
	m.Update(key("a"))
	m.View()
	if len(m.BrowserImages()) < 2 {
		t.Fatal("about logo missing")
	}
	m.Update(key("q"))
	m.View()
	if len(m.BrowserImages()) != 1 {
		t.Fatal("about logo survived closing overlay")
	}
	m.Update(key("q"))
	m.View()
	if len(m.BrowserImages()) != 0 {
		t.Fatal("header logo survived returning to file chooser")
	}
}

// Two identical sessions, one drawn from the cell diff and one from the flattened
// frame, must write the same bytes through boot, the files and a board, including
// a resize.
func TestTheBrowserSendsTheSameBytesWhicheverWayItDiffs(t *testing.T) {
	session := func() *Model {
		rs := NewRooms(7)
		rs.EnsureOpen(2)
		m := NewLocalSession(rs, 1, "mark")
		m.KeepOpen()
		m.UseGraphics(GraphicsBrowser)
		m.StartIntro()
		m.Resize(120, 40)
		return m
	}
	cells, text := session(), session()
	var byCell, byText canvas.FrameDiffer
	step := func(i int, msg any) {
		cells.Update(msg)
		text.Update(msg)
		if got, want := cells.ViewChanged(&byCell), byText.Render(text.View()); got != want {
			t.Fatalf("frame %d: cell diff sent %d bytes, text diff sent %d", i, len(got), len(want))
		}
	}
	for i := 0; i < 60*12; i++ {
		switch i {
		case 400:
			step(i, key("enter")) // skip whatever is left of the intro
		case 450:
			step(i, key("enter")) // join a file
		case 500:
			cells.Resize(100, 32)
			text.Resize(100, 32)
			byText.Reset()
			step(i, FrameMsg(frameInterval))
		default:
			step(i, FrameMsg(frameInterval))
		}
	}
	if cells.Screen != ScreenMain {
		t.Fatalf("the sessions ended on %v, so the board was never compared", cells.Screen)
	}
}

// The browser skips a frame that ran no steps and saw no input. That is safe only
// if such a frame draws nothing new on every screen it can happen on.
func TestAFrameWithNoStepsDrawsNothingNew(t *testing.T) {
	rs := NewRooms(7)
	rs.EnsureOpen(2)
	m := NewLocalSession(rs, 1, "mark")
	m.KeepOpen()
	m.UseGraphics(GraphicsBrowser)
	m.StartIntro()
	m.Resize(120, 40)
	var d canvas.FrameDiffer
	check := func(where string) {
		t.Helper()
		m.UpdateEvent(FrameMsg(frameInterval))
		m.ViewChanged(&d)
		// Zero elapsed time runs no steps, as jitter shorter than a step does.
		m.UpdateEvent(FrameMsg(0))
		if m.Stepped() {
			t.Fatalf("%s: a frame with no time ran a step", where)
		}
		if got := m.ViewChanged(&d); got != "" {
			t.Fatalf("%s: a frame with no steps redrew %d bytes", where, len(got))
		}
	}
	for i := 0; i < 60; i++ {
		check("the intro")
	}
	m.Update(key("enter"))
	for i := 0; i < 60; i++ {
		check("the files")
	}
	m.Update(key("enter"))
	for i := 0; i < 240; i++ {
		check("the board")
	}
	if m.Screen != ScreenMain {
		t.Fatalf("ended on %v, so the board was never checked", m.Screen)
	}
}
