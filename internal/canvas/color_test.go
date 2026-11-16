package canvas

import "testing"

// A palette is stored packed, and unpacked again for lipgloss.
func TestColourRoundTrips(t *testing.T) {
	for _, s := range []string{"#00FFFF", "#000000", "#FFFFFF", "#1B4149", "#FF3020"} {
		c := ParseColor(s)
		if !c.Set() {
			t.Errorf("%s did not parse", s)
			continue
		}
		if got := c.String(); got != s {
			t.Errorf("%s came back as %s", s, got)
		}
	}

	// The short form doubles each digit, and an index is not a hex colour.
	if ParseColor("#0AF") != ParseColor("#00AAFF") {
		t.Error("#0AF and #00AAFF are not the same colour")
	}
	if got := ParseColor("5"); got != ANSI(5) {
		t.Errorf("the palette index 5 parsed as %v", got)
	}
	if ParseColor("").Set() || ParseColor("nonsense").Set() {
		t.Error("an empty or unparseable colour is not unset")
	}
}

// Otherwise a blank Attr would draw something.
func TestAnUnsetColourIsTheZeroValue(t *testing.T) {
	var c Color
	if c.Set() {
		t.Error("the zero Color reports itself as set")
	}
	if (Attr{}) != defaultAttr {
		t.Error("a blank Attr is not the default one")
	}
	if s := c.String(); s != "" {
		t.Errorf("an unset colour renders as %q", s)
	}
}

// Rounding by adding a half before truncating goes the wrong way when a
// channel is decreasing.
func TestBlendIsExactAtTheEndsAndRoundsBothWays(t *testing.T) {
	a, b := ParseColor("#000000"), ParseColor("#FFFFFF")

	if got := Blend(a, b, 0); got != a {
		t.Errorf("a blend at 0 gave %v, want the first colour", got)
	}
	if got := Blend(a, b, 1); got != b {
		t.Errorf("a blend at 1 gave %v, want the second", got)
	}

	// Rising and falling across the same pair must meet in the same place.
	up := Blend(a, b, 0.25)
	down := Blend(b, a, 0.75)
	if up != down {
		t.Errorf("blending up gave %v and down gave %v; they should agree", up, down)
	}

	// An indexed colour has no channels to mix, so the nearer end wins.
	idx := ANSI(5)
	if got := Blend(idx, b, 0.4); got != idx {
		t.Errorf("blending a palette index gave %v, want the nearer end", got)
	}
	if got := Blend(idx, b, 0.6); got != b {
		t.Errorf("blending a palette index gave %v, want the nearer end", got)
	}
}

// A fade can produce at most BlendSteps+1 colours.
func TestSteppedBlendingLandsOnlyOnSteps(t *testing.T) {
	BlendSteps = 4
	defer func() { BlendSteps = 0 }()
	a, b := RGB(0, 0, 0), RGB(200, 100, 40)
	seen := map[Color]bool{}
	for i := 0; i <= 1000; i++ {
		seen[Blend(a, b, float64(i)/1000)] = true
	}
	if len(seen) != 5 {
		t.Fatalf("a stepped fade produced %d colours, want 5", len(seen))
	}
	if Blend(a, b, 0.01) != a || Blend(a, b, 0.99) != b || Blend(a, b, 0.5) != RGB(100, 50, 20) {
		t.Fatal("stepped blending moved its ends or its middle")
	}
}
