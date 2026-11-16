package lumon

import (
	"strings"
	"testing"
)

// It opens as the familiar flat wave so the player sees what is being pulled
// apart.
func TestDepthOpensFlatBeforeItPullsBack(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "DEPTH")

	// Flat, and identical to what MATRIX draws, for a good second.
	tick(m, 60)
	if d := m.deep.Depth(); d > depthArrived {
		t.Errorf("a second in the view is already %.2f deep", d)
	}
	flat := screenRows(m)

	tick(m, 60*3)
	mid := m.deep.Depth()
	if mid <= depthArrived {
		t.Fatalf("four seconds in the view is still flat at %.2f", mid)
	}
	tick(m, 60*4)
	if late := m.deep.Depth(); late <= mid {
		t.Errorf("the view stopped opening at %.2f", late)
	}
	if late := m.deep.Depth(); late < 0.9 {
		t.Errorf("eight seconds in the view has only reached %.2f", late)
	}

	if deep := screenRows(m); deep == flat {
		t.Error("the deep view draws exactly what the flat one did")
	}
}

func screenRows(m *Model) string {
	buf := drawTo(m)
	p := m.gridPanel()
	var b strings.Builder
	for y := p.Y; y < p.Y+p.H; y++ {
		b.WriteString(buf.Row(y))
		b.WriteByte('\n')
	}
	return b.String()
}

// Dropping from deep straight to the grid would be an abrupt cut.
func TestDepthComesBackToFlatBeforeGoingHome(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "DEPTH")
	tick(m, 60*8)
	if m.deep.Depth() < 0.9 {
		t.Fatal("the view never opened")
	}

	m.Update(key("x"))
	if m.deep == nil {
		t.Fatal("leaving dropped the view instead of closing it")
	}
	if !m.deep.Leaving() {
		t.Fatal("leaving did not turn the camera round")
	}
	if m.transit != nil {
		t.Error("the digits set off while the view was still deep")
	}

	// It flattens first, and only then does anything fly.
	frames := 0
	for ; frames < 60*8 && m.deep != nil && m.deep.Depth() > depthArrived; frames++ {
		if m.transit != nil {
			t.Fatalf("the digits set off at depth %.2f, on frame %d", m.deep.Depth(), frames)
		}
		tick(m, 1)
	}
	if frames < 8 {
		t.Errorf("the view came back in %d frames; that is a cut", frames)
	}

	for f := 0; f < 60*8 && m.Diverting(); f++ {
		tick(m, 1)
	}
	if m.Diverting() {
		t.Error("it never got back to the grid")
	}
}

// The camera's target is nought both before the pull starts and after it turns
// round, so "leaving" cannot be read off the target alone, or a diversion left
// in its first seconds could never end.
func TestDepthCanBeLeftBeforeItEverOpens(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "DEPTH")
	tick(m, 30)
	if m.deep.Leaving() {
		t.Fatal("it reported itself leaving half a second after it started")
	}

	m.Update(key("x"))
	for f := 0; f < 60*6 && m.Diverting(); f++ {
		tick(m, 1)
	}
	if m.Diverting() {
		t.Error("leaving before the pull began never got back to the grid")
	}
}

// It keeps its own place in the menu so the player recognises it.
func TestTheFlatWaveIsUntouched(t *testing.T) {
	names := map[string]bool{}
	for _, d := range diversions {
		names[d.Name] = true
	}
	if !names["MATRIX"] {
		t.Error("the flat wave lost its place in the menu")
	}
	if !names["DEPTH"] {
		t.Error("DEPTH is not in the menu")
	}

	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "DEPTH")
	if m.deep.flat == nil {
		t.Fatal("DEPTH built no wave")
	}
	// The inner fade is put straight to full: it has a fade of its own, and the two
	// rising together would multiply and halve the opening's brightness.
	if lvl := m.deep.flat.Level(); lvl < 0.99 {
		t.Errorf("the inner wave's own fade is at %.2f; the two are multiplying", lvl)
	}
}

// Flat, the digits go home from the grid's cells; opened, from their projected
// positions.
func TestDepthGivesBackWhereverItHasTheDigits(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "DEPTH")
	panel := m.gridPanel().Play()

	tick(m, 30)
	flat := m.deep.Digits(m.Grid, m.Layout, panel)
	if len(flat) == 0 {
		t.Fatal("the flat view gave back no digits")
	}
	l := m.Layout
	for _, p := range flat {
		if _, _, ok := l.CellAt(int(p.X), int(p.Y)); !ok {
			t.Fatalf("a flat digit is at (%.0f,%.0f), which is not a grid cell", p.X, p.Y)
		}
	}

	tick(m, 60*8)
	deep := m.deep.Digits(m.Grid, m.Layout, panel)
	if len(deep) == 0 {
		t.Fatal("the deep view gave back no digits")
	}
	same := 0
	for i := range deep {
		if i < len(flat) && deep[i] == flat[i] {
			same++
		}
	}
	if same == len(deep) {
		t.Error("the deep view has its digits in exactly the flat positions")
	}
	for _, p := range deep {
		if !panel.Contains(int(p.X), int(p.Y)) {
			t.Errorf("a deep digit is at (%.0f,%.0f), outside the panel", p.X, p.Y)
		}
	}
}

// Otherwise the pull back adds no depth.
func TestTheColumnsSitAtDifferentDepths(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "DEPTH")
	tick(m, 60*8)

	shots := m.deep.shots(m.Grid, m.Layout, m.gridPanel().Play())
	if len(shots) == 0 {
		t.Fatal("nothing was projected")
	}
	near, far := shots[len(shots)-1].Depth, shots[0].Depth
	if near-far < depthSpread*0.5 {
		t.Errorf("the nearest column is %.2f and the furthest %.2f; that is a sheet",
			near, far)
	}
}
