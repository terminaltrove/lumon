package lumon

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
	"testing"
)

// arrive runs the model until nothing is in the air between the grid and a
// diversion.
func arrive(t *testing.T, m *Model) {
	t.Helper()
	for i := 0; i < 60*4; i++ {
		if !m.transitioning() {
			return
		}
		tick(m, 1)
	}
	t.Fatal("a transition never finished")
}

// typeWord feeds a word to a model one key at a time.
func typeWord(m *Model, word string) {
	for _, r := range word {
		m.Update(key(strings.ToLower(string(r))))
	}
}

// Typing the founder's name opens the drawer.
func TestTypingTheWordOpensTheDiversions(t *testing.T) {
	m := ready()
	typeWord(m, theWord)

	if !m.divertOpen {
		t.Fatalf("typing %q left the screen at %v", theWord, m.Screen)
	}
	if got := screenOf(m); !strings.Contains(got, diversions[0].Name) {
		t.Errorf("the menu does not list the first diversion:\n%s", got)
	}
}

// The letters have to be consecutive.
func TestTheWordHasToBeTypedNotStumbledInto(t *testing.T) {
	m := ready()
	for _, s := range []string{"k", "i", "j", "e", "r"} {
		m.Update(key(s))
	}
	if m.divertOpen {
		t.Error("a wrong letter in the middle still opened the diversions")
	}

	// A false start that begins the word again is fine: KKIER opens it, because
	// the failed letter is itself the first letter.
	m = ready()
	typeWord(m, "K"+theWord)
	if !m.divertOpen {
		t.Error("a stutter before the word stopped it opening")
	}
}

// The letters include hjkl and i, which move the cursor and open the
// screensaver. Every letter but the first is swallowed: a lone K still moves
// the cursor, or the board would have a key that did nothing. So the rest of
// the word must add nothing to what the first letter does alone.
func TestTypingTheWordDoesNotAlsoPlayTheGame(t *testing.T) {
	first := ready()
	typeWord(first, theWord[:1])
	wantX, wantY := first.CursorX, first.CursorY

	m := ready()
	typeWord(m, theWord)

	if m.CursorX != wantX || m.CursorY != wantY {
		t.Errorf("typing %q left the cursor at (%d,%d); %q alone leaves it at (%d,%d), "+
			"so the letters after the first are driving the grid",
			theWord, m.CursorX, m.CursorY, theWord[:1], wantX, wantY)
	}
}

// It is an easter egg: nothing in the controls may mention it.
func TestTheDiversionsAreNotDocumented(t *testing.T) {
	for _, g := range Bindings() {
		for _, b := range g.Bindings {
			all := b.Keys + " " + b.What + " " + b.Short
			for _, name := range append([]string{theWord, "diversion"}, namesOf()...) {
				if strings.Contains(strings.ToUpper(all), strings.ToUpper(name)) {
					t.Errorf("the controls mention %q, which is meant to be found: %q", name, all)
				}
			}
		}
	}

	// Nor the help screen it renders.
	m := ready()
	m.Helping = true
	if got := screenOf(m); strings.Contains(got, theWord) {
		t.Errorf("the controls overlay gives the word away:\n%s", got)
	}
}

func namesOf() []string {
	out := make([]string, 0, len(diversions))
	for _, d := range diversions {
		out = append(out, d.Name)
	}
	return out
}

// The width must be measured the way rows are drawn: margin, marker, the name
// field, a gap and the description.
func TestTheMenuFitsWhatItLists(t *testing.T) {
	m := ready()
	m.Resize(140, 40)
	typeWord(m, theWord)
	if !m.divertOpen {
		t.Fatal("the menu did not open")
	}
	screen := screenOf(m)

	for _, d := range diversions {
		// The full description.
		if !strings.Contains(screen, d.What) {
			t.Errorf("%s's description is cut off; the box is too narrow for %q",
				d.Name, d.What)
		}
		// And a gap between the name and it, or the two read as one word.
		if !strings.Contains(screen, d.Name+"  ") {
			t.Errorf("%s runs straight into its description", d.Name)
		}
	}
}

func TestTheDiversionsMenuScrollsAndCloses(t *testing.T) {
	m := ready()
	typeWord(m, theWord)

	m.Update(key("down"))
	m.Update(key("down"))
	if m.divertAt != 2 {
		t.Errorf("two downs left the menu on row %d", m.divertAt)
	}
	m.Update(key("up"))
	if m.divertAt != 1 {
		t.Errorf("an up left the menu on row %d", m.divertAt)
	}

	m.Update(key("x"))
	if m.divertOpen {
		t.Error("a stray key left the menu open")
	}
	if m.Screen != ScreenMain {
		t.Errorf("the menu moved off the grid to %v; it is a modal", m.Screen)
	}
}

func TestTheBoardKeepsMovingDuringADiversion(t *testing.T) {
	_, a, b := twoInARoom(t)

	// Wait for a cluster: its time awake is board state, so it only moves while
	// the board is being stepped, which is what is being measured.
	for i := 0; i < 60*25 && b.FirstActive() == nil; i++ {
		tick(a, 1)
		tick(b, 1)
	}
	if b.FirstActive() == nil {
		t.Skip("nothing woke up")
	}

	typeWord(a, theWord)
	if !a.divertOpen {
		t.Fatal("the menu did not open")
	}

	before := b.ScaryElapsed()
	for i := 0; i < 60; i++ {
		tick(a, 1)
		tick(b, 1)
	}
	if b.ScaryElapsed() <= before {
		t.Error("the board stopped while one refiner was in the menu")
	}
}

// Otherwise the list promises something enter does not deliver.
func TestEveryDiversionStarts(t *testing.T) {
	for i, d := range diversions {
		t.Run(d.Name, func(t *testing.T) {
			m := ready()
			m.Resize(110, 30)
			typeWord(m, theWord)
			m.divertAt = i
			m.Update(key("enter"))

			if m.divertOpen {
				t.Fatalf("%s did not start; the menu is still up", d.Name)
			}

			// One entry answers instead of starting: the terminal says no in the same words
			// the escape key gets.
			if d.Refuses {
				if !m.Escaped {
					t.Fatalf("%s neither started nor refused", d.Name)
				}
				if m.Diverting() {
					t.Fatalf("%s started something; it is meant to refuse", d.Name)
				}
				m.Update(key("x"))
				if m.Escaped {
					t.Errorf("a key did not dismiss the notice %s gives", d.Name)
				}
				return
			}

			if !m.Diverting() {
				t.Fatalf("%s did not start; nothing is running", d.Name)
			}
			// It runs inside the frame, so the screen never changes.
			if m.Screen != ScreenMain {
				t.Fatalf("%s moved off the grid to %v", d.Name, m.Screen)
			}

			// It runs, it draws, and it goes back to work.
			arrive(t, m)
			tick(m, 120)
			_ = drawTo(m)

			m.Update(key("x"))
			arrive(t, m)
			if m.Diverting() {
				t.Errorf("a stray key did not end %s", d.Name)
			}
		})
	}
}

// Leaving one must let its simulation go, or a long session accumulates every
// board anyone ever played with.
func TestLeavingADiversionLetsItGo(t *testing.T) {
	for i, d := range diversions {
		// A mode is not left: 3D is a way of drawing the board, x is an ordinary
		// keypress while it is up, and m.solid is meant to survive.
		// TestEveryDiversionIsWiredEverywhere skips modes for the same reason.
		if d.Mode {
			continue
		}
		m := ready()
		m.Resize(110, 30)
		typeWord(m, theWord)
		m.divertAt = i
		m.Update(key("enter"))
		arrive(t, m)
		tick(m, 60)
		m.Update(key("x"))
		arrive(t, m)

		// Every field: a diversion forgotten in dropDiversions is a simulation still
		// stepped and drawn under the next one.
		if m.sandbox != nil || m.matrix != nil || m.sand != nil ||
			m.primes != nil || m.praise != nil || m.comply != nil ||
			m.deep != nil || m.mark != nil || m.halls != nil || m.fountain != nil ||
			m.sky != nil {
			t.Errorf("leaving %s left a simulation running", d.Name)
		}
	}
}

// A terminal can change size at any moment, and they all hold screen-sized
// state.
func TestEveryDiversionSurvivesAResize(t *testing.T) {
	for i, d := range diversions {
		t.Run(d.Name, func(t *testing.T) {
			m := ready()
			m.Resize(120, 34)
			typeWord(m, theWord)
			m.divertAt = i
			m.Update(key("enter"))
			arrive(t, m)
			tick(m, 60)

			for _, wh := range [][2]int{{60, 18}, {200, 50}, {30, 8}} {
				m.Resize(wh[0], wh[1])
				tick(m, 30)
				buf := drawTo(m)
				w, h := buf.Size()
				for y := 0; y < h; y++ {
					if n := len([]rune(buf.Row(y))); n != w {
						t.Fatalf("%s at %dx%d drew a row of %d", d.Name, wh[0], wh[1], n)
					}
				}
			}
		})
	}
}

// A diversion happens inside the grid's own panel, and the terminal around it
// stays up.
func TestTheChromeSurvivesEveryDiversion(t *testing.T) {
	for i, d := range diversions {
		t.Run(d.Name, func(t *testing.T) {
			m := ready()
			m.Resize(110, 30)
			typeWord(m, theWord)
			m.divertAt = i
			m.Update(key("enter"))
			arrive(t, m)
			tick(m, 120)

			buf := drawTo(m)
			l := m.Layout
			var screen strings.Builder
			for y := 0; y < l.Height; y++ {
				screen.WriteString(buf.Row(y))
				screen.WriteByte('\n')
			}
			all := screen.String()

			if !strings.Contains(all, m.FileName) {
				t.Error("the file bar is gone")
			}
			// Asked of the band the bar says it drew, since the completion is set in glyphs
			// where there is room and is then not a string on screen.
			if m.header.pct.empty() {
				t.Error("the completion readout is gone")
			}
			for b := 0; b < BinCount; b++ {
				want := binLabel(b + 1)
				x := binTextX(l.BinCenterX[b], len(want), l.BinOriginX(b), l.BinWidth(b))
				row := []rune(buf.Row(l.BinTop + 1))
				if got := string(row[x : x+len(want)]); got != want {
					t.Errorf("bin %s is gone; that column reads %q", want, got)
				}
			}
			// Both rules, above and below the panel, at l.BoardY+l.BoardH where the rule is
			// drawn. The rectangle keeps the leftover BoardH%RowH rows inside itself (see
			// Layout), so rebuilding it from the cell count is wrong unless the spacing
			// divides the board.
			for _, y := range []int{l.GridTop - TopRuleHeight, l.GridTop - 1, l.BoardY + l.BoardH} {
				if !strings.Contains(buf.Row(y), "══") {
					t.Errorf("the rule at row %d is gone", y)
				}
			}
			// The footer has the game's controls, or the camera's in a mode; a diversion
			// has neither. The hint is on the last row; the row above is the address
			// readout and the banner.
			foot := buf.Row(l.Height - 1)
			if !strings.Contains(foot, "move") && !strings.Contains(foot, "fly") {
				t.Errorf("the footer is gone; that row reads %q", strings.TrimSpace(foot))
			}
		})
	}
}

func TestNoDiversionDrawsOutsideThePanel(t *testing.T) {
	for _, wh := range [][2]int{{80, 24}, {200, 50}} {
		for i, d := range diversions {
			m := ready()
			m.Resize(wh[0], wh[1])
			typeWord(m, theWord)
			m.divertAt = i
			m.Update(key("enter"))

			// A blank board to compare against, so the chrome's own digits (the bin counts,
			// the address readout) are not mistaken for the diversion's.
			panel := m.gridPanel()
			for f := 0; f < 6; f++ {
				tick(m, 40)
				buf := drawTo(m)
				for y := 0; y < wh[1]; y++ {
					if y >= panel.Y && y < panel.Y+panel.H {
						continue // inside the panel, anything goes
					}
					row := []rune(buf.Row(y))
					clean := []rune(m.blankChrome(y))
					for x := 0; x < len(row) && x < len(clean); x++ {
						if row[x] != clean[x] {
							t.Fatalf("%s at %dx%d drew %q at (%d,%d), outside the panel",
								d.Name, wh[0], wh[1], string(row[x]), x, y)
						}
					}
				}
			}
		}
	}
}

// blankChrome is what a row looks like with no diversion running.
func (m *Model) blankChrome(y int) string {
	was := m.diverting
	m.diverting = ""
	row := drawTo(m).Row(y)
	m.diverting = was
	return row
}

// A diversion is something a refiner does at their own terminal, not a pause
// button on the file.
func TestTheBoardGoesOnUnderneathADiversion(t *testing.T) {
	m := ready()
	m.Resize(110, 30)

	// Something in a bin, so the bar has somewhere to go.
	m.Room.mu.Lock()
	m.Grid.Bins[0].Refined = m.Grid.Bins[0].Quota / 2
	m.Room.mu.Unlock()

	startDiversionNamed(t, m, "MATRIX")
	arrive(t, m)
	before := m.File.Fill()
	tick(m, 180)

	if m.File.Fill() <= before {
		t.Errorf("the file bar sat at %v while a diversion ran; the board should keep working",
			m.File.Fill())
	}
	if !m.Diverting() {
		t.Error("the diversion stopped on its own")
	}
}

func TestTheMenuIsAModalOverTheBoard(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	typeWord(m, theWord)

	if m.Screen != ScreenMain {
		t.Fatalf("opening the menu moved off the grid to %v", m.Screen)
	}
	buf := drawTo(m)
	var all strings.Builder
	for y := 0; y < m.Layout.Height; y++ {
		all.WriteString(buf.Row(y))
	}
	s := all.String()
	if !strings.Contains(s, m.FileName) {
		t.Error("the file bar is hidden behind the menu")
	}
	if !strings.Contains(s, "PERPETUITY WING") {
		t.Error("the menu is not drawn")
	}
	if !strings.Contains(s, "SCATTER") {
		t.Error("the menu lists nothing")
	}
}

// Adding a diversion means adding an arm to every site that dispatches on which
// one is running. A missed arm is a silent nil, not a compile error: the
// diversion does not resize, answer the mouse, or give its digits back for the
// trip home. keys_test.go uses the same source-scanning idiom.
func TestEveryDiversionIsWiredEverywhere(t *testing.T) {
	sites := []string{
		"startDiversion",
		"stepDiversion",
		"resizeDiversion",
		"drawDiversion",
		"diversionPlaces",
		"handleDivertingMouse",
		"running",
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", nil, 0)
	if err != nil {
		t.Fatalf("parsing the package: %v", err)
	}

	// The source of each dispatch site, as text. A diversion is named in one if its
	// name or its field appears there: some sites switch on m.matrix, others on the
	// "MATRIX" string.
	bodies := map[string]string{}
	for _, pkg := range pkgs {
		for path, f := range pkg.Files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			for _, decl := range f.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				var b strings.Builder
				if err := printer.Fprint(&b, fset, fn.Body); err != nil {
					t.Fatalf("printing %s: %v", fn.Name.Name, err)
				}
				bodies[fn.Name.Name] = b.String()
			}
		}
	}

	// Which field each diversion is held in. SCATTER and DESCENT share the sandbox:
	// one simulation with two temperaments.
	field := map[string]string{
		"SCATTER": "m.sandbox", "DESCENT": "m.sandbox",
		"SAND": "m.sand", "MATRIX": "m.matrix",
		"PRIMES": "m.primes", "PRAISE": "m.praise",
		"CORRIDORS":   "m.halls",
		"FOUNTAIN":    "m.fountain",
		"FIREWORKS":   "m.sky",
		"COMPUNCTION": "m.comply",
		"DEPTH":       "m.deep",
		"PERPETUITY":  "m.mark",
		"3D":          "toggleSolid",
		"ESCAPE":      "Refuses",
	}

	// Sites a diversion does not appear in, declared so that "nothing to do here"
	// can be told apart from "somebody forgot".
	nothingToDo := map[string]string{
		// Point clouds projected into whatever panel they are given at draw time, so
		// there is no screen-sized state to refit.
		"resizeDiversion/PRAISE": "holds no screen-sized state",

		// Neither is played with. Both fall through to the default arm, where a press
		// goes back to work.
		"handleDivertingMouse/MATRIX":      "not interactive; a press leaves",
		"handleDivertingMouse/PRAISE":      "not interactive; a press leaves",
		"handleDivertingMouse/CORRIDORS":   "not interactive; a press leaves",
		"handleDivertingMouse/FOUNTAIN":    "not interactive; a press leaves",
		"handleDivertingMouse/FIREWORKS":   "not interactive; a press leaves",
		"handleDivertingMouse/COMPUNCTION": "not interactive; a press leaves",
		"handleDivertingMouse/DEPTH":       "not interactive; a press leaves",
	}

	for _, site := range sites {
		body, ok := bodies[site]
		if !ok {
			t.Fatalf("no function named %q, this test names the dispatch sites and one has moved", site)
		}
		for _, d := range diversions {
			// A mode has no simulation to dispatch to; it is listed so somebody can find
			// it, and only startDiversion needs to know about it.
			if (d.Mode || d.Refuses) && site != "startDiversion" {
				continue
			}
			f, known := field[d.Name]
			if !known {
				t.Errorf("%s is in the menu but this test does not know which field holds it", d.Name)
				continue
			}
			if strings.Contains(body, d.Name) || strings.Contains(body, f) {
				continue
			}
			if why, allowed := nothingToDo[site+"/"+d.Name]; allowed {
				_ = why
				continue
			}
			t.Errorf("%s mentions neither %q nor %s, it will silently do nothing there",
				site, d.Name, f)
		}
	}
}

// Everything else in this region is centred. Asserted against the drawn row,
// which is where it lands.
func TestTheHintLineIsCentred(t *testing.T) {
	for i, d := range diversions {
		if d.Mode || d.Refuses {
			continue // neither is a diversion and neither draws a hint
		}
		t.Run(d.Name, func(t *testing.T) {
			m := ready()
			m.Resize(110, 32)
			typeWord(m, theWord)
			m.divertAt = i
			m.Update(key("enter"))
			arrive(t, m)
			tick(m, 60)

			panel := m.gridPanel()
			hint := m.divertHint()
			if hint == "" {
				t.Skip("this one says nothing")
			}

			row := []rune(drawTo(m).Row(panel.Y + panel.H - 1))
			at := strings.Index(string(row), hint)
			if at < 0 {
				t.Fatalf("the hint is not on the panel's last row: %q", strings.TrimRight(string(row), " "))
			}

			// Whatever is left over should be split evenly, give or take the odd column
			// that cannot be halved.
			left := len([]rune(string(row)[:at]))
			right := panel.W - left - len([]rune(hint))
			if left-right > 1 || right-left > 1 {
				t.Errorf("%d columns to the left of the hint and %d to the right; it "+
					"is not centred in a panel %d wide", left, right, panel.W)
			}
		})
	}
}
