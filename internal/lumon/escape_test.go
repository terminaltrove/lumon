package lumon

import (
	"strings"
	"testing"
)

// Escape is answered with a notice, unlike an unbound key.
func TestEscapeIsRefusedOnEveryScreen(t *testing.T) {
	for _, c := range []struct {
		what string
		make func() *Model
	}{
		{"the grid", func() *Model { m := ready(); m.Resize(120, 40); return m }},
		{"the name prompt", func() *Model {
			rs := NewRooms(5)
			rs.Open()
			m := NewSession(rs, 1)
			m.Resize(120, 40)
			return m
		}},
		{"the lobby", func() *Model {
			rs := NewRooms(5)
			rs.Open()
			m := named(NewSession(rs, 1), "mark")
			m.Resize(120, 40)
			m.Update(key("enter"))
			return m
		}},
		{"the controls", func() *Model {
			m := ready()
			m.Resize(120, 40)
			m.Helping = true
			return m
		}},
		{"the drawer", func() *Model {
			m := ready()
			m.Resize(120, 40)
			typeWord(m, theWord)
			return m
		}},
		{"a diversion", func() *Model { return diverting(t, "MATRIX", 120, 40) }},
		{"the third dimension", func() *Model {
			m := ready()
			m.Resize(120, 40)
			enter3D(t, m)
			return m
		}},
	} {
		m := c.make()
		was := m.Screen
		m.Update(key("esc"))

		if !m.Escaped {
			t.Errorf("esc at %s was swallowed rather than refused", c.what)
		}
		if m.Quitting {
			t.Errorf("esc at %s quit the session", c.what)
		}
		if m.Screen != was {
			t.Errorf("esc at %s moved the screen from %v to %v", c.what, was, m.Screen)
		}
	}
}

func TestTheEscapeNoticeSaysWhatItSays(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	m.Update(key("esc"))

	// The notice is wrapped, so match it a clause at a time.
	screen := screenOf(m)
	for _, part := range []string{
		"Error:", "not supported", "MDR Terminal", "reported to your supervisor",
	} {
		if !strings.Contains(screen, part) {
			t.Errorf("the notice does not say %q:\n%s", part, screen)
		}
	}

	// Escape does not dismiss the notice; any other key does.
	m.Update(key("esc"))
	if !m.Escaped {
		t.Error("pressing esc at the notice dismissed it; it is not a key that works")
	}
	m.Update(key("x"))
	if m.Escaped {
		t.Error("a key did not dismiss the notice")
	}
}

func TestTheEscapeNoticeFitsWhateverItIsGiven(t *testing.T) {
	for _, size := range [][2]int{{200, 60}, {120, 40}, {80, 24}, {60, 18}, {30, 10}, {10, 4}} {
		w, h := size[0], size[1]
		m := ready()
		m.Resize(w, h)
		m.Update(key("esc"))

		buf := drawTo(m)
		if buf == nil {
			t.Fatalf("%dx%d drew nothing", w, h)
		}
		for y := 0; y < h; y++ {
			if n := len([]rune(buf.Row(y))); n > w {
				t.Errorf("%dx%d: row %d is %d wide", w, h, y, n)
				break
			}
		}
	}

	// The wrap never breaks a word.
	for _, width := range []int{20, 30, 44, 56, 90} {
		lines := wrapNotice(escapeNotice, width)
		if strings.Join(lines, " ") != escapeNotice {
			t.Errorf("at width %d the notice came back changed: %q", width, lines)
		}
		for _, line := range lines {
			if n := len([]rune(line)); n > width {
				t.Errorf("at width %d a line is %d wide: %q", width, n, line)
			}
		}
	}
}

// Adding a row for escape to the controls would spoil the joke.
func TestEscapeIsNotInTheControls(t *testing.T) {
	for _, g := range Bindings() {
		for _, b := range g.Bindings {
			for _, k := range b.Bound {
				if k == "esc" {
					t.Errorf("%q is bound in the controls table", b.Keys)
				}
			}
			if strings.Contains(strings.ToLower(b.Keys+" "+b.What+" "+b.Short), "esc") {
				t.Errorf("the controls mention escape: %q / %q / %q", b.Keys, b.What, b.Short)
			}
		}
	}

	// Nor the help overlay, nor the footer.
	m := ready()
	m.Resize(120, 40)
	m.Helping = true
	if got := screenOf(m); strings.Contains(got, "esc") {
		t.Errorf("the controls overlay names esc:\n%s", got)
	}
	if got := footerHint(200); strings.Contains(got, "esc") {
		t.Errorf("the footer names esc: %q", got)
	}
}

func TestTheEscapeEntryRefusesInTheSameWords(t *testing.T) {
	m := ready()
	m.Resize(120, 40)
	typeWord(m, theWord)

	at := -1
	for i, d := range diversions {
		if d.Name == "ESCAPE" {
			at = i
		}
	}
	if at < 0 {
		t.Fatal("ESCAPE is not in the drawer")
	}
	m.divertAt = at
	m.Update(key("enter"))

	if m.divertOpen {
		t.Error("choosing ESCAPE left the drawer open")
	}
	if !m.Escaped {
		t.Fatal("choosing ESCAPE did not refuse")
	}
	if m.Diverting() {
		t.Error("choosing ESCAPE started a simulation; nothing happens")
	}
	if got := screenOf(m); !strings.Contains(got, "reported to your supervisor") {
		t.Errorf("the entry gave a different answer from the key:\n%s", got)
	}
}
