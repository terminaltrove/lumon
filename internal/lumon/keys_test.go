package lumon

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// boundKeys is every key string the package matches, parsed out of the key
// handlers. Bindings are strings in switches, so reflection cannot find them.
func boundKeys(t *testing.T) map[string]bool {
	t.Helper()

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", nil, 0)
	if err != nil {
		t.Fatalf("parsing the package: %v", err)
	}

	// The functions that dispatch on a key string. Hand-kept: a new key handler
	// must be added here, or its bindings go unchecked. handleDiversionsKey is left
	// out because what it opens is meant to be found, and its menu uses the
	// documented up/down/enter.
	handlers := map[string]bool{
		"handleKey": true, "handleNameKey": true, "handleLobbyKey": true,
		"handleScoresKey": true,
	}

	out := map[string]bool{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || !handlers[fn.Name.Name] {
					continue
				}
				ast.Inspect(fn, func(n ast.Node) bool {
					cc, ok := n.(*ast.CaseClause)
					if !ok {
						return true
					}
					for _, e := range cc.List {
						lit, ok := e.(*ast.BasicLit)
						if !ok || lit.Kind != token.STRING {
							continue
						}
						if s, err := strconv.Unquote(lit.Value); err == nil {
							out[s] = true
						}
					}
					return true
				})
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("found no key bindings at all; the parser is looking in the wrong place")
	}
	return out
}

func TestEveryDocumentedKeyIsBound(t *testing.T) {
	bound := boundKeys(t)

	for _, g := range bindingTable {
		for _, b := range g.Bindings {
			for _, k := range b.Bound {
				if !bound[k] {
					t.Errorf("%s: the controls promise %q (%s) but nothing handles it",
						g.Title, k, b.What)
				}
			}
		}
	}
}

// Otherwise a control could be added and left undiscoverable.
func TestEveryBoundKeyIsDocumented(t *testing.T) {
	documented := map[string]bool{}
	for _, g := range bindingTable {
		for _, b := range g.Bindings {
			for _, k := range b.Bound {
				documented[k] = true
			}
		}
	}

	// Keys that are not in the controls table, with the reason.
	exempt := map[string]string{
		"backspace": "editing your name; the field is self-evident",
	}

	for k := range boundKeys(t) {
		if documented[k] || exempt[k] != "" {
			continue
		}
		t.Errorf("%q is bound but appears in no controls list; a key nobody can "+
			"discover may as well not exist", k)
	}
}

// The footer is built from the table, but it must still fit, and drop
// entries instead of truncating mid-word.
func TestTheFooterHintFitsWhateverTheWidth(t *testing.T) {
	for _, w := range []int{200, 80, 60, 40, 24, 12, 6, 1, 0} {
		got := footerHint(w)
		if len([]rune(got)) > w {
			t.Errorf("at width %d the hint is %d wide: %q", w, len([]rune(got)), got)
		}
		if strings.HasSuffix(got, "·") || strings.HasSuffix(got, " ") {
			t.Errorf("at width %d the hint ends mid-separator: %q", w, got)
		}
	}
	// A wide terminal gets the lot.
	if !strings.Contains(footerHint(200), "? help") {
		t.Error("a wide footer does not mention the controls key")
	}
	// A narrow one keeps what you cannot play without.
	if got := footerHint(24); got != "" && !strings.Contains(got, "move") {
		t.Errorf("a narrow footer dropped movement first: %q", got)
	}
}

// They are an overlay so they can open wherever somebody is stuck.
func TestTheControlsOpenFromAnywhere(t *testing.T) {
	for _, c := range []struct {
		name  string
		build func() *Model
	}{
		{"the grid", func() *Model { return ready() }},
		{"the name prompt", func() *Model {
			m := named(NewSession(NewRooms(1), 1), "mark")
			m.Resize(120, 40)
			return m
		}},
		{"the lobby", func() *Model {
			rs := NewRooms(1)
			rs.Open()
			m := named(NewSession(rs, 1), "mark")
			m.Resize(120, 40)
			m.Update(key("enter"))
			return m
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := c.build()
			was := m.Screen

			m.Update(key("?"))
			if !m.Helping {
				t.Fatal("? did not open the controls")
			}
			if m.Screen != was {
				t.Errorf("opening the controls moved off %v to %v", was, m.Screen)
			}

			got := screenOf(m)
			if !strings.Contains(got, "LUMON MDR HELP") {
				t.Errorf("the controls are not on screen:\n%s", got)
			}

			// Any key puts them away and leaves you where you were.
			m.Update(key("x"))
			if m.Helping {
				t.Error("a key did not dismiss the controls")
			}
			if m.Screen != was {
				t.Errorf("dismissing the controls moved off %v to %v", was, m.Screen)
			}
		})
	}
}

func TestNothingActsBehindTheControls(t *testing.T) {
	m := ready()
	m.Update(key("?"))

	x, y, bin := m.CursorX, m.CursorY, m.SelectedBin
	flights := len(m.Flights.Active())

	m.Update(key("down"))
	if m.Helping {
		t.Fatal("the first key should have dismissed the overlay")
	}
	if m.CursorX != x || m.CursorY != y {
		t.Errorf("the cursor moved to (%d,%d) behind the controls", m.CursorX, m.CursorY)
	}

	m.Update(key("?"))
	m.Update(key("enter"))
	if m.SelectedBin != bin {
		t.Error("the bin changed behind the controls")
	}
	if len(m.Flights.Active()) != flights {
		t.Error("enter refined something behind the controls")
	}
}

func TestTheMouseDoesNotReachBehindTheControls(t *testing.T) {
	m := ready()
	m.Update(key("?"))

	x, y := overTheGrid(m)
	m.Update(mouseMotion(x, y))
	if !m.Helping {
		t.Error("moving the pointer dismissed the controls; only a click should")
	}
	before := len(m.Flights.Active())
	m.Update(click(x, y))
	if m.Helping {
		t.Error("a click did not dismiss the controls")
	}
	if len(m.Flights.Active()) != before {
		t.Error("a click refined something behind the controls")
	}
}

// It must say something legible at any size, not paint an empty box.
func TestTheControlsDegradeRatherThanBreak(t *testing.T) {
	for _, wh := range [][2]int{{200, 60}, {100, 30}, {80, 24}, {40, 12}, {20, 6}, {8, 3}} {
		m := ready()
		m.Resize(wh[0], wh[1])
		m.Helping = true

		buf := drawTo(m)
		w, h := buf.Size()
		for y := 0; y < h; y++ {
			if n := len([]rune(buf.Row(y))); n > w {
				t.Fatalf("at %dx%d row %d is %d wide", wh[0], wh[1], y, n)
			}
		}
	}
}

// The hint grows with the controls table and shares the footer with the banner
// and the roster, so it must give way to them.
func TestTheFooterHintDoesNotRunUnderTheBanner(t *testing.T) {
	for _, w := range []int{200, 140, 110, 90, 76, 60} {
		m := ready()
		m.Resize(w, 30)
		buf := drawTo(m)
		row := buf.Row(m.Layout.FooterY())

		// The banner's own text must survive intact wherever it is drawn.
		if strings.Contains(row, "LUMON INDUSTRIES") &&
			!strings.Contains(row, "LUMON INDUSTRIES, MDR") {
			t.Errorf("at width %d the banner is broken by the hint: %q", w, row)
		}
	}
}

// The README and docs/controls.md are checked against the handlers like the
// other places the controls are written down. They need not list every key, but
// every key they promise must be handled.
func TestTheReadmePromisesNoKeyThatDoesNotExist(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Skipf("no README to check: %v", err)
	}

	// The controls section only. The rest of the file has backticked flags, file
	// names and shell, none of which are keys.
	body := string(raw)
	start := strings.Index(body, "### TUI Controls")
	if start < 0 {
		t.Fatal("the README has no TUI Controls section")
	}
	end := strings.Index(body[start+1:], "\n## ")
	if end < 0 {
		end = len(body) - start - 1
	}
	promisesOnlyBoundKeys(t, "the README", body[start:start+1+end])
}

func TestTheControlsDocPromisesNoKeyThatDoesNotExist(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "controls.md"))
	if err != nil {
		t.Fatalf("the full list of controls is missing: %v", err)
	}
	promisesOnlyBoundKeys(t, "docs/controls.md", string(raw))
}

// promisesOnlyBoundKeys fails for every backticked key in text that no handler
// matches.
func promisesOnlyBoundKeys(t *testing.T, where, text string) {
	t.Helper()
	bound := boundKeys(t)

	// How the docs write a key against how a handler matches one. Arrows are
	// drawn as arrows, and the space bar as a word because a backticked blank is
	// unreadable; Bubble Tea's String for it is " ", which the handlers match.
	alias := map[string]string{
		"space": " ",
		"↑":     "up", "↓": "down", "←": "left", "→": "right",
		"H": "shift+up", "J": "shift+down", "K": "shift+left", "L": "shift+right",
	}
	// Backticked words that are not keys: the forms a range or a modifier is written
	// in, each covered by the keys around it.
	notKeys := map[string]bool{
		"shift": true, "3d": true,
	}

	for _, m := range regexp.MustCompile("`([^`\n]+)`").FindAllStringSubmatch(text, -1) {
		key := m[1]
		if notKeys[key] {
			continue
		}
		if a, ok := alias[key]; ok {
			key = a
		}
		if bound[key] {
			continue
		}
		// A range like 1-5 stands for each of its ends and everything between.
		if len(key) == 3 && key[1] == '-' {
			for c := key[0]; c <= key[2]; c++ {
				if !bound[string(c)] {
					t.Errorf("%s promises %q, and %q is not bound", where, m[1], string(c))
				}
			}
			continue
		}
		t.Errorf("%s promises %q but nothing handles it", where, m[1])
	}
}
