package lumon

import "github.com/terminaltrove/lumon/internal/canvas"

// The screensaver: the mark bouncing around the whole terminal, with nothing of
// the game on screen. The board it came from, if any, goes on underneath. The
// PERPETUITY diversion is the same mark inside the board's panel.

// StartScreensaver covers the terminal with the mark, from a board or from the
// command line. It draws from viewRng, so it leaves the board's stream alone.
func (m *Model) StartScreensaver() {
	m.idleMark = newIdleMark(m.Layout.Width, m.Layout.Height, m.viewRng)
	m.Screen = ScreenIdle
}

// idlePanel is the whole terminal.
func (m *Model) idlePanel() Panel {
	return Panel{W: m.Layout.Width, H: m.Layout.Height}
}

// wakeScreensaver goes back to the board, or ends the program if there is none
// to go back to.
func (m *Model) wakeScreensaver() {
	if m.screensaverOnly {
		m.Quitting = true
		return
	}
	m.idleMark = nil
	m.Screen = ScreenMain
}

// handleIdleKey takes the mark's own keys, and reports whether the key was one.
// Like PERPETUITY's, they are outside the handlers keys_test.go scans and are
// not in Bindings: they are there to be found.
func (m *Model) handleIdleKey(key string) bool {
	if m.idleMark == nil {
		return false
	}
	switch key {
	case "c":
		m.idleMark.Colour()
		return true
	case "p":
		m.idleMark.Physics()
		return true
	}
	return false
}

// handleIdleMouse lets physics mode pick the mark up. Any other press wakes the
// screensaver, as a key does.
func (m *Model) handleIdleMouse(x, y int, press, release, motion bool) {
	if m.idleMark == nil {
		if press {
			m.wakeScreensaver()
		}
		return
	}
	dotX := float64(x*canvas.DotsX) + float64(canvas.DotsX)/2
	dotY := float64(y*canvas.DotsY) + float64(canvas.DotsY)/2
	switch {
	case press:
		if !m.idleMark.Take(dotX, dotY) {
			m.wakeScreensaver()
		}
	case release:
		m.idleMark.Drop()
	case motion:
		m.idleMark.DragTo(dotX, dotY)
	}
}

// drawIdle paints the screensaver.
func (m *Model) drawIdle(buf *canvas.Buffer) {
	if m.idleMark != nil {
		m.idleMark.Draw(buf, m.Theme, m.idlePanel())
	}
}
