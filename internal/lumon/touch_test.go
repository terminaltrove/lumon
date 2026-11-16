package lumon

import (
	"strings"
	"testing"
)

// A phone has no keys to press, so its hint lines speak of taps, at a phone's
// width; a browser with a keyboard keeps its keys.
func TestATouchScreenIsToldToTapNotToPressKeys(t *testing.T) {
	for _, touch := range []bool{true, false} {
		rs := NewRooms(7)
		rs.EnsureOpen(2)
		m := NewLocalSession(rs, 1, "mark")
		m.KeepOpen()
		if touch {
			m.UseTouch()
		}
		m.Resize(60, 46)

		files := m.View()
		m.Update(key("enter"))
		if m.Screen != ScreenMain {
			t.Fatalf("enter on the files left the screen at %v", m.Screen)
		}
		board := m.View()

		if touch {
			for _, want := range []struct{ screen, text, where string }{
				{files, "tap a file", "the files"},
				{board, "tap to refine", "the board"},
			} {
				if !strings.Contains(want.screen, want.text) {
					t.Errorf("a touch screen's %s do not say %q", want.where, want.text)
				}
			}
			for _, key := range []string{"enter join", "f focus", "1-5 bin"} {
				if strings.Contains(files+board, key) {
					t.Errorf("a touch screen is told to press %q", key)
				}
			}
			continue
		}
		if !strings.Contains(files, "enter join") || strings.Contains(files+board, "tap ") {
			t.Error("a browser with a keyboard lost its key hints")
		}
	}
}
