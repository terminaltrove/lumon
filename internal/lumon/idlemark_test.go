package lumon

import (
	"slices"
	"strings"
	"testing"
)

// brailleOn counts the lit braille cells on a screen.
func brailleOn(screen string) int {
	n := 0
	for _, r := range screen {
		if r > '⠀' && r <= '⣿' {
			n++
		}
	}
	return n
}

// --screensaver is the mark alone. It opens no file, takes no desk and has no
// room to answer to.
func TestTheScreensaverHasNoGameBehindIt(t *testing.T) {
	m := NewScreensaver(1)
	m.Resize(120, 40)
	tick(m, 120)

	if m.Screen != ScreenIdle {
		t.Fatalf("it opened on screen %v", m.Screen)
	}
	if m.Room != nil || m.rooms != nil || m.Desk != nil {
		t.Error("the screensaver is sitting in a game")
	}
	if m.Diverting() {
		t.Error("the screensaver is a diversion of a board")
	}
	got := m.View()
	if brailleOn(got) == 0 {
		t.Error("the mark is not on screen")
	}
	for _, chrome := range []string{"LUMON INDUSTRIES", "Complete", "? help"} {
		if strings.Contains(got, chrome) {
			t.Errorf("the game's %q is on screen", chrome)
		}
	}
}

// It crosses the whole terminal, not the board's panel.
func TestTheScreensaverBouncesAcrossTheWholeTerminal(t *testing.T) {
	m := NewScreensaver(1)
	m.Resize(60, 20)
	x0, y0 := m.idleMark.x, m.idleMark.y
	top, bottom := m.idleMark.y, m.idleMark.y
	for range 60 * 30 {
		tick(m, 1)
		top = min(top, m.idleMark.y)
		bottom = max(bottom, m.idleMark.y)
	}
	if m.idleMark.x == x0 && m.idleMark.y == y0 {
		t.Error("the mark never moved")
	}
	ry := float64(m.idleMark.mh-1) / 2
	if top > ry+2 || bottom < 20*4-1-ry-2 {
		t.Errorf("the mark's centre only reached rows %.0f to %.0f of 80 dots", top, bottom)
	}
}

func TestAnyKeyEndsTheScreensaver(t *testing.T) {
	for _, k := range []string{"x", "enter", "q", "?", "i", "esc", "ctrl+c"} {
		m := NewScreensaver(1)
		m.Resize(80, 24)
		if !lanPress(m, k) || !m.Quitting {
			t.Errorf("%s did not end the screensaver", k)
		}
	}
}

func TestAClickEndsTheScreensaver(t *testing.T) {
	m := NewScreensaver(1)
	m.Resize(80, 24)
	effects := m.UpdateEvent(MouseMsg{X: 3, Y: 3, Button: MouseLeft, Action: MousePress})
	if !slices.ContainsFunc(effects, func(e Effect) bool { return e.Kind == EffectQuit }) {
		t.Errorf("a click gave %+v", effects)
	}
}

// Moving the mouse is not asking to leave; the program tracks all motion.
func TestMovingTheMouseDoesNotEndTheScreensaver(t *testing.T) {
	m := NewScreensaver(1)
	m.Resize(80, 24)
	if m.UpdateEvent(MouseMsg{X: 5, Y: 5, Action: MouseMotion}); m.Quitting {
		t.Error("the mouse moved and the screensaver ended")
	}
}

func TestTheMarksOwnKeysDoNotEndTheScreensaver(t *testing.T) {
	m := NewScreensaver(1)
	m.Resize(80, 24)
	was := m.idleMark.hue
	if lanPress(m, "c") || m.Quitting || m.idleMark.hue == was {
		t.Error("c did not change the colour and carry on")
	}
	if lanPress(m, "p") || m.Quitting || !m.idleMark.physics {
		t.Error("p did not turn on physics and carry on")
	}
}

// The command line builds the model before Bubble Tea delivers the first window
// size, so the mark starts against the default layout and is then resized, down
// to a terminal smaller than itself.
func TestTheScreensaverSurvivesEveryResize(t *testing.T) {
	m := NewScreensaver(1)
	tick(m, 10)
	for _, size := range [][2]int{{140, 44}, {84, 26}, {10, 3}, {1, 1}, {100, 30}} {
		m.Resize(size[0], size[1])
		tick(m, 60)
		_ = m.View()

		p := m.idleMark
		if p.x < 0 || p.y < 0 || p.x > p.w || p.y > p.h {
			t.Errorf("at %v the mark is at %.0f,%.0f in %.0fx%.0f", size, p.x, p.y, p.w, p.h)
		}
	}
}

// On a board, i is the same mark, and any key goes back to the board.
func TestIShowsTheMarkOverTheBoard(t *testing.T) {
	m := ready()
	m.Update(key("i"))
	if m.idleMark == nil || !m.idleMark.Fullscreen() {
		t.Fatal("i did not bring the mark up on the whole terminal")
	}
	tick(m, 120)
	if brailleOn(m.View()) == 0 {
		t.Error("the mark is not on screen")
	}
	if m.Diverting() {
		t.Error("i started a diversion")
	}

	if lanPress(m, "c") || m.Screen != ScreenIdle {
		t.Error("c left the screensaver")
	}
	m.Update(key("x"))
	if m.Screen != ScreenMain || m.Quitting || m.idleMark != nil {
		t.Errorf("a key gave screen %v, quitting %v", m.Screen, m.Quitting)
	}
}

// Physics lets the mouse pick the mark up, and a press anywhere else wakes it.
func TestTheMarkCanBeThrownInTheScreensaver(t *testing.T) {
	m := ready()
	m.Update(key("i"))
	m.Update(key("p"))
	p := m.idleMark

	// A press on the mark holds it.
	m.handleIdleMouse(int(p.x)/2, int(p.y)/4, true, false, false)
	if !p.Held() || m.Screen != ScreenIdle {
		t.Fatal("a press on the mark did not take it")
	}
	m.handleIdleMouse(0, 0, false, true, false)
	if p.Held() {
		t.Error("letting go did not drop it")
	}

	// Away from it, the press wakes.
	far := 0
	if int(p.x)/2 < m.Layout.Width/2 {
		far = m.Layout.Width - 1
	}
	m.handleIdleMouse(far, 0, true, false, false)
	if m.Screen != ScreenMain {
		t.Error("a press away from the mark did not wake the screensaver")
	}
}

// PERPETUITY stays in the drawer, in the board's panel.
func TestTheDrawerStillHasTheBouncingMark(t *testing.T) {
	found := false
	for _, d := range diversions {
		found = found || d.Name == "PERPETUITY"
	}
	if !found {
		t.Error("PERPETUITY has gone from the drawer")
	}
}
