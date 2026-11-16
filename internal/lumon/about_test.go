package lumon

import (
	"strings"
	"testing"
)

func TestTheAboutBoxOpensAndCloses(t *testing.T) {
	m := ready()
	m.Resize(96, 32)

	m.Update(key("a"))
	if !m.About {
		t.Fatal("a did not open the credits")
	}

	var screen string
	buf := drawTo(m)
	for y := 0; y < m.Layout.Height; y++ {
		screen += buf.Row(y) + "\n"
	}
	for _, want := range []string{
		"Terminal Trove",
		"terminaltrove.com/lumon",
		"github.com/terminaltrove/lumon",
	} {
		if !strings.Contains(screen, want) {
			t.Errorf("the credits do not mention %q:\n%s", want, screen)
		}
	}

	// The mark is the intro's own, not a second drawing of it.
	if !strings.Contains(screen, strings.TrimRight(strings.ReplaceAll(SmallLogo[2], "⠀", " "), " ")) {
		t.Error("the credits do not carry the Lumon mark")
	}

	m.Update(key("a"))
	if m.About {
		t.Error("a did not close the credits again")
	}
}

// Like the controls, it takes every key, so a key pressed at the credits
// dismisses them instead of refining something hidden behind them.
func TestTheAboutBoxTakesTheKeyboard(t *testing.T) {
	m := ready()
	m.Resize(96, 32)
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	m.CursorX, m.CursorY = grp.NumberIDs[0]%GridSize, grp.NumberIDs[0]/GridSize

	m.About = true
	m.Update(key("enter"))
	if m.About {
		t.Error("a keypress did not dismiss the credits")
	}
	if !m.IsActive(grp.ID) {
		t.Error("the keypress refined the cluster behind the credits")
	}
}

// "a" is a letter a nameplate can hold.
func TestAboutDoesNotStealALetterFromTheNamePrompt(t *testing.T) {
	m := NewSession(NewRooms(3), 1)
	m.Resize(96, 32)
	m.Screen = ScreenName

	m.Update(key("a"))
	if m.About {
		t.Fatal("the credits opened at the name prompt")
	}
	if !strings.Contains(strings.ToUpper(m.Name), "A") {
		t.Errorf("the letter was swallowed; the name reads %q", m.Name)
	}
}

// aboutScreen is the full frame with the credits up, at a given size.
func aboutScreen(t *testing.T, w, h int) string {
	t.Helper()
	m := ready()
	m.Resize(w, h)
	m.About = true
	buf := drawTo(m)

	var sb strings.Builder
	for y := 0; y < h; y++ {
		sb.WriteString(buf.Row(y))
		sb.WriteByte('\n')
	}
	return sb.String()
}

func TestTheAboutBoxNamesTheVersion(t *testing.T) {
	screen := aboutScreen(t, 96, 32)
	if want := "v" + Version; !strings.Contains(screen, want) {
		t.Errorf("the credits do not show %q:\n%s", want, screen)
	}
}

// The list ends on the github address, so truncating at the bottom edge would
// drop the lines that say where to find the project. A box out of room still
// shows the project, the author and the address.
func TestAShortAboutBoxKeepsTheCreditAndDropsTheRest(t *testing.T) {
	for _, size := range [][2]int{{96, 32}, {80, 24}, {72, 20}} {
		screen := aboutScreen(t, size[0], size[1])
		for _, want := range []string{
			"Terminal Trove",
			"terminaltrove.com/lumon",
			"github.com/terminaltrove/lumon",
		} {
			if !strings.Contains(screen, want) {
				t.Errorf("%dx%d dropped %q:\n%s", size[0], size[1], want, screen)
			}
		}
	}
}

// The toolchain goes before the flavour text, and the flavour text before
// anything that identifies the project.
func TestTheAboutBoxShedsInOrder(t *testing.T) {
	full := aboutText()

	if got := aboutFit(full, len(full)); len(got) != len(full) {
		t.Errorf("a box with room to spare dropped %d lines", len(full)-len(got))
	}

	// One row short: the "Built with" block goes, heading and all.
	tight := aboutFit(full, len(full)-1)
	if has(tight, "Built with:") || has(tight, "Bubble Tea") {
		t.Error("a tight box kept the toolchain")
	}
	if !has(tight, "https://github.com/terminaltrove/lumon") {
		t.Error("a tight box dropped the address")
	}

	// Tighter still: the flavour goes too, leaving the credit.
	bare := aboutFit(full, 6)
	for _, want := range []string{
		"Lumon Industries MDR",
		"a Terminal Trove re-creation",
		"https://terminaltrove.com/lumon/",
		"https://github.com/terminaltrove/lumon",
	} {
		if !has(bare, want) {
			t.Errorf("the smallest box dropped %q", want)
		}
	}
	if len(bare) > 6 {
		t.Errorf("the smallest box kept %d lines, want at most 6", len(bare))
	}
}

func has(lines []aboutLine, text string) bool {
	for _, l := range lines {
		if l.text == text {
			return true
		}
	}
	return false
}
