package lumon

import (
	"math"
	"testing"
)

func panelOf(w, h int) Panel { return Panel{X: 0, Y: 0, W: w, H: h} }

// Nearest last, so a cloud has a front and a back without a depth buffer
// (painter's order, as inertia uses).
func TestTheCameraHandsBackTheNearestPointLast(t *testing.T) {
	c := NewCamera()
	c.Scale = 4
	pts := []Pt3{
		{X: 0, Y: 0, Z: 0.5, Digit: 'n'},
		{X: 0, Y: 0, Z: -0.5, Digit: 'f'},
		{X: 0, Y: 0, Z: 0, Digit: 'm'},
	}
	shots := c.Shots(pts, panelOf(40, 20))
	if len(shots) != 3 {
		t.Fatalf("%d of 3 points came back", len(shots))
	}
	for i := 1; i < len(shots); i++ {
		if shots[i].Depth < shots[i-1].Depth {
			t.Fatalf("point %d at depth %.2f came after %.2f", i, shots[i].Depth, shots[i-1].Depth)
		}
	}
	if shots[len(shots)-1].Digit != 'n' {
		t.Errorf("the nearest point was drawn %c, not last", shots[len(shots)-1].Digit)
	}
}

// Dividing by a negative depth turns a point inside out and throws it across
// the panel, mirrored and enormous.
func TestAPointBehindTheEyeIsDroppedRatherThanInverted(t *testing.T) {
	c := NewCamera()
	c.Dolly = 5
	c.Scale = 4

	// Well in front of the eye, and not so close that the perspective divide
	// throws it off the panel, which would drop it for a different reason.
	near := c.Shots([]Pt3{{X: 1, Z: 1, Digit: '1'}}, panelOf(80, 24))
	if len(near) != 1 {
		t.Fatal("a point in front of the eye was dropped")
	}
	if near[0].Col <= 40 {
		t.Errorf("a point to the right of centre landed at column %d", near[0].Col)
	}

	for _, z := range []float64{5, 5.001, 9, 40} {
		if got := c.Shots([]Pt3{{X: 1, Z: z, Digit: '1'}}, panelOf(80, 24)); len(got) != 0 {
			t.Errorf("a point at z=%v, at or behind the eye, came back at column %d",
				z, got[0].Col)
		}
	}
}

func TestTheNearSideOfACloudIsBigger(t *testing.T) {
	c := NewCamera()
	c.Scale = 4
	shots := c.Shots([]Pt3{
		{X: 1, Z: 2, Digit: 'n'},
		{X: 1, Z: -2, Digit: 'f'},
	}, panelOf(80, 24))
	if len(shots) != 2 {
		t.Fatalf("%d of 2 points came back", len(shots))
	}
	var near, far Shot
	for _, s := range shots {
		if s.Digit == 'n' {
			near = s
		} else {
			far = s
		}
	}
	if near.Col-40 <= far.Col-40 {
		t.Errorf("the near point is %d columns off centre and the far one %d;"+
			" there is no perspective", near.Col-40, far.Col-40)
	}
}

func TestYawTurnsTheCloudAboutTheVertical(t *testing.T) {
	c := NewCamera()
	c.Scale = 4
	pt := []Pt3{{X: 1, Digit: '1'}}

	at := func(yaw float64) Shot {
		c.Yaw = yaw
		s := c.Shots(pt, panelOf(80, 24))
		if len(s) != 1 {
			t.Fatalf("the point vanished at yaw %v", yaw)
		}
		return s[0]
	}

	front := at(0)
	side := at(90)
	back := at(180)

	if math.Abs(float64(side.Col-40)) > 1 {
		t.Errorf("edge-on the point is %d columns off centre, want none", side.Col-40)
	}
	if front.Col-40 <= 0 || back.Col-40 >= 0 {
		t.Errorf("half a turn left the point at %d, from %d", back.Col-40, front.Col-40)
	}
}

// A caller may draw every shot it is given without checking bounds.
func TestTheCameraHandsBackNothingOffThePanel(t *testing.T) {
	c := NewCamera()
	c.Scale = 40 // far too large for the panel

	pts := make([]Pt3, 0, 64)
	for i := 0; i < 64; i++ {
		pts = append(pts, Pt3{X: float64(i%8) - 4, Y: float64(i/8) - 4, Digit: '7'})
	}
	panel := Panel{X: 3, Y: 2, W: 20, H: 10}
	for _, s := range c.Shots(pts, panel) {
		if !panel.Contains(s.Col, s.Row) {
			t.Fatalf("a point came back at (%d,%d), outside the panel", s.Col, s.Row)
		}
	}
}
