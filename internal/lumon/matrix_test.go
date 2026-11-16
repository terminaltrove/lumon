package lumon

import "testing"

// The wave lights up the board's own digits where the grid already draws them,
// with no glyphs or columns of its own.
func TestTheMatrixLightsUpTheBoardWithoutMovingIt(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	tick(m, 60)

	l := m.Layout
	type at struct {
		x, y int
		d    rune
	}
	var want []at
	for row := 0; row < l.ViewportH; row++ {
		for col := 0; col < l.ViewportW; col++ {
			n := m.Grid.At(l.ViewportX+col, l.ViewportY+row)
			if n == nil {
				continue
			}
			x, y := l.ScreenOf(l.ViewportX+col, l.ViewportY+row)
			want = append(want, at{x, y, rune('0' + n.Value)})
		}
	}
	if len(want) == 0 {
		t.Fatal("no digits on the board")
	}

	startDiversionNamed(t, m, "MATRIX")
	tick(m, 40)
	buf := drawTo(m)

	for _, w := range want {
		if got := buf.RuneAt(w.x, w.y); got != w.d {
			t.Fatalf("cell (%d,%d) shows %q; the board has %q there and the wave must not move or reroll it",
				w.x, w.y, got, w.d)
		}
	}
}

func TestTheMatrixBrightensSomeDigitsAndNotOthers(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	startDiversionNamed(t, m, "MATRIX")
	tick(m, 20)

	l := m.Layout
	buf := drawTo(m)
	seen := map[string]int{}
	for row := 0; row < l.ViewportH; row++ {
		for col := 0; col < l.ViewportW; col++ {
			x, y := l.ScreenOf(l.ViewportX+col, l.ViewportY+row)
			seen[buf.AttrAt(x, y).FG.String()]++
		}
	}
	if len(seen) < 3 {
		t.Errorf("the board is drawn in %d colours; the wave should have a head, a tail and a rest state",
			len(seen))
	}
	if seen[m.Theme.Digit.FG.String()] == 0 {
		t.Error("nothing is at the resting colour; the whole board is lit")
	}
}

func TestTheMatrixFallsAndItsColumnsDisagree(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	startDiversionNamed(t, m, "MATRIX")

	before := append([]float64(nil), m.matrix.head...)
	tick(m, 60)

	moved := 0
	for i, h := range m.matrix.head {
		if h != before[i] {
			moved++
		}
	}
	if moved < len(before)/2 {
		t.Errorf("only %d of %d columns moved in a second", moved, len(before))
	}

	distinct := map[float64]bool{}
	for _, s := range m.matrix.speed {
		distinct[s] = true
	}
	if len(distinct) < len(m.matrix.speed)/2 {
		t.Errorf("%d distinct speeds across %d columns; the wave is one sheet",
			len(distinct), len(m.matrix.speed))
	}
}

func TestTheMatrixKeepsGoing(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	startDiversionNamed(t, m, "MATRIX")
	tick(m, 60*25)

	l := m.Layout
	buf := drawTo(m)
	lit := 0
	for row := 0; row < l.ViewportH; row++ {
		for col := 0; col < l.ViewportW; col++ {
			x, y := l.ScreenOf(l.ViewportX+col, l.ViewportY+row)
			if buf.AttrAt(x, y).FG != m.Theme.Digit.FG {
				lit++
			}
		}
	}
	if lit == 0 {
		t.Error("after twenty-five seconds nothing is lit; the wave ran out")
	}
}

func TestTheMatrixFollowsTheViewport(t *testing.T) {
	m := ready()
	m.Resize(140, 40)
	startDiversionNamed(t, m, "MATRIX")
	if got := len(m.matrix.head); got != m.Layout.ViewportW {
		t.Errorf("the wave has %d columns for a viewport of %d", got, m.Layout.ViewportW)
	}

	m.Resize(70, 20)
	tick(m, 30)
	if got := len(m.matrix.head); got != m.Layout.ViewportW {
		t.Errorf("after a resize the wave has %d columns for a viewport of %d",
			got, m.Layout.ViewportW)
	}
}

// The wave moves no digit, so the flight in and out that other diversions get
// is over in a frame. Without its own fade the board would change colour
// abruptly at each end.
func TestTheWaveRisesAndFallsRatherThanSwitchingOn(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "MATRIX")

	if lvl := m.matrix.Level(); lvl > 0.2 {
		t.Errorf("the wave arrived at %.2f; it should start dark", lvl)
	}

	// It takes a while to come up, and does not overshoot.
	rose := 0
	for i := 0; i < 60*3 && m.matrix.Level() < 0.9; i++ {
		tick(m, 1)
		rose++
		if l := m.matrix.Level(); l > 1.001 {
			t.Fatalf("the wave rose past full, to %.3f", l)
		}
	}
	if rose < 8 {
		t.Errorf("the wave came up in %d frames; that is a switch", rose)
	}

	// Leaving makes it fall before anything flies home.
	m.Update(key("x"))
	if m.matrix == nil {
		t.Fatal("leaving dropped the wave instead of fading it")
	}
	if !m.matrix.Leaving() {
		t.Fatal("leaving did not ask the wave to fall")
	}
	if m.transit != nil {
		t.Error("the digits set off before the wave had fallen")
	}

	fell := 0
	for ; fell < 60*3 && m.matrix != nil; fell++ {
		tick(m, 1)
	}
	if fell < 8 {
		t.Errorf("the wave went out in %d frames; that is a cut", fell)
	}
}

// At nought the rise begins from the frame that was already there.
func TestADarkWaveIsIndistinguishableFromTheBoard(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "MATRIX")
	m.matrix.SetLevel(0)

	lit := 0
	for col := 0; col < m.Layout.ViewportW; col++ {
		for row := 0; row < m.Layout.ViewportH; row++ {
			if m.matrix.attrAt(m.Theme, col, row) != m.Theme.Digit {
				lit++
			}
		}
	}
	if lit != 0 {
		t.Errorf("%d cells were lit with the wave dark", lit)
	}
}
