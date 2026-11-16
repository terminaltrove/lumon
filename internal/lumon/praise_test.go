package lumon

import (
	"github.com/terminaltrove/lumon/internal/glyph"
	"math"
	"testing"
)

// A missing letter draws nothing and the wordmark loses a character.
func TestEveryLetterOfTheWordIsDrawable(t *testing.T) {
	for _, line := range praiseLines {
		for _, r := range line {
			if r == ' ' {
				continue
			}
			if glyph.LetterStrokes(r) == nil {
				t.Errorf("no shape for %q, which the word needs", r)
			}
		}
	}
	if glyph.LetterStrokes('Q') != nil {
		t.Error("a letter that was never authored returned strokes")
	}
	if s := glyph.LetterStrokes(' '); s == nil || len(s) != 0 {
		t.Error("a space should be drawable and empty, so the layout advances")
	}
}

// Otherwise they overlap their neighbours and the raster grid clips them.
func TestTheLettersStayInsideTheirBox(t *testing.T) {
	for r, strokes := range glyph.LetterOutlines {
		for _, st := range strokes {
			for _, p := range st {
				if p.X < 0 || p.X > 1 || p.Y < 0 || p.Y > 1 {
					t.Errorf("%q has a point at (%v,%v), outside the unit box", r, p.X, p.Y)
				}
			}
		}
	}
}

// Two shells deep so it reads as solid when it turns edge-on.
func TestTheWordIsSampledIntoAPointCloud(t *testing.T) {
	m := ready()
	m.Resize(96, 28)
	startDiversionNamed(t, m, "PRAISE")
	if m.praise == nil {
		t.Fatalf("PRAISE did not start; screen is %v", m.Screen)
	}

	p := m.praise
	if len(p.pts) < 200 {
		t.Errorf("the word sampled to %d points; too few to read", len(p.pts))
	}
	// Two shells, so an even count and both depths present.
	var front, back int
	for _, pt := range p.pts {
		if pt.Z > 0 {
			front++
		} else {
			back++
		}
	}
	if front != back {
		t.Errorf("%d points in front and %d behind; the word is not extruded evenly", front, back)
	}
}

func TestTheWordTurnsAndWrapsAround(t *testing.T) {
	m := ready()
	m.Resize(96, 28)
	startDiversionNamed(t, m, "PRAISE")

	tick(m, 30)
	half := m.praise.angle
	if half <= 0 {
		t.Fatal("the word is not turning")
	}
	tick(m, 60*20)
	if a := m.praise.angle; a < 0 || a >= 360 {
		t.Errorf("the angle wandered to %v; it should stay in [0,360)", a)
	}
}

func TestTheWordStaysOnScreenThroughAWholeTurn(t *testing.T) {
	m := ready()
	m.Resize(100, 30)
	startDiversionNamed(t, m, "PRAISE")

	for step := 0; step < 24; step++ {
		tick(m, 21) // a little over fifteen degrees each time
		buf := drawTo(m)

		top, bottom, drawn := math.Inf(1), math.Inf(-1), 0
		for y := 0; y < 29; y++ { // the last row is the hint
			for _, r := range buf.Row(y) {
				if r >= '0' && r <= '9' {
					drawn++
					top = math.Min(top, float64(y))
					bottom = math.Max(bottom, float64(y))
					break
				}
			}
		}
		if drawn == 0 {
			t.Fatalf("at %.0f degrees nothing is drawn", m.praise.angle)
		}
		// Two lines, so it fills a band of rows even when edge-on.
		if bottom-top < 3 {
			t.Errorf("at %.0f degrees the word is %v rows tall", m.praise.angle, bottom-top+1)
		}
	}
}

func TestTheWordDeclinesToDrawOnATinyTerminal(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	startDiversionNamed(t, m, "PRAISE")

	m.Resize(16, 5)
	tick(m, 30)
	buf := drawTo(m)
	for y := 0; y < 4; y++ {
		for _, r := range buf.Row(y) {
			if r >= '0' && r <= '9' {
				t.Fatalf("the word drew digits on a sixteen-column terminal:\n%q", buf.Row(y))
			}
		}
	}
}

func TestSamplingWalksAStrokeRatherThanItsCorners(t *testing.T) {
	s := glyph.Line(glyph.Point{X: 0, Y: 0}, glyph.Point{X: 1, Y: 0})
	got := samplePoints(s, 0.1)
	if len(got) < 10 {
		t.Fatalf("a unit-long stroke sampled at 0.1 gave %d points", len(got))
	}
	for i := 1; i < len(got); i++ {
		d := math.Hypot(got[i].X-got[i-1].X, got[i].Y-got[i-1].Y)
		if math.Abs(d-0.1) > 1e-9 {
			t.Fatalf("points %d and %d are %v apart, want 0.1", i-1, i, d)
		}
	}
	if n := samplePoints(nil, 0.1); n != nil {
		t.Error("an empty stroke sampled to something")
	}
}

// The letters are polylines, so a stroke joining two arcs draws a segment
// between them, as the 2's diagonal and the S's spine do. It is a fault when
// the arcs do not end where that segment should start, and the join then reads
// as a strike-through. This checks the property in all seven letters.
func TestNoLetterHasAStrayJoin(t *testing.T) {
	// Only strokes with an arc are checked. A stem or arm is a two-point
	// glyph.Line() whose segments are long by design (R's stem is 0.72). An arc is
	// sampled into twenty-odd short segments, so a bad join stands out.
	const sampled = 8

	// A third of the box. The legitimate joins are P's and R's bowl closures at
	// 0.26 and the S's spine at 0.185; a stray join is around 0.43.
	const longest = 0.35

	for r, strokes := range glyph.LetterOutlines {
		for si, st := range strokes {
			if len(st) <= sampled {
				continue
			}
			for i := 1; i < len(st); i++ {
				d := math.Hypot(st[i].X-st[i-1].X, st[i].Y-st[i-1].Y)
				if d > longest {
					t.Errorf("%q stroke %d has a %.3f-long segment from (%.2f,%.2f) to (%.2f,%.2f); "+
						"an arc most likely does not end where the join should start",
						r, si, d, st[i-1].X, st[i-1].Y, st[i].X, st[i].Y)
				}
			}
		}
	}
}

// A bowl at the top opening right, a bowl at the bottom opening left, and one
// crossing between them.
func TestTheSIsAnS(t *testing.T) {
	strokes := glyph.LetterStrokes('S')
	if len(strokes) != 1 {
		t.Fatalf("the S is %d strokes; it is meant to be one, with the join as its spine", len(strokes))
	}
	pts := strokes[0]

	// It crosses the vertical centre line exactly three times: over the top bowl,
	// along the spine, and under the bottom bowl. A slash through the middle adds
	// a fourth.
	const mid = 0.50
	crossings := 0
	for i := 1; i < len(pts); i++ {
		if (pts[i-1].X-mid)*(pts[i].X-mid) < 0 {
			crossings++
		}
	}
	if crossings != 3 {
		t.Errorf("the S crosses its own centre line %d times, want 3 "+
			"(over the top, along the spine, under the bottom)", crossings)
	}

	// It starts upper-right and ends lower-left, unlike a Z or a mirrored S.
	if pts[0].X < mid || pts[0].Y > mid {
		t.Errorf("the S starts at (%.2f,%.2f), want the upper right", pts[0].X, pts[0].Y)
	}
	if last := pts[len(pts)-1]; last.X > mid || last.Y < mid {
		t.Errorf("the S ends at (%.2f,%.2f), want the lower left", last.X, last.Y)
	}
}
