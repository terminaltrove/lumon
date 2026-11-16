package glyph

import "math"

// Digits are stroke outlines in a unit box, rasterised at any fractional size.
// The box runs x from 0 (left) to 1 (right) and y from 0 (top) to 1 (bottom),
// as on screen.

// Point is a position in the unit box.
type Point struct{ X, Y float64 }

// Stroke is a polyline. Curves are approximated by short segments.
type Stroke []Point

// StrokeHalfWidth is a stroke's half-width at the largest sizes, as a
// fraction of the box, so it scales with the glyph like a font weight.
const StrokeHalfWidth = 0.085

const (
	// penRows is the stroke thickness, in rows, a glyph drawn as a silhouette
	// aims for, and penRowsAscii the same for one drawn in copies of its own
	// digit. A silhouette wants a lean pen, or its counters close up. A glyph
	// drawn in characters shows the digit only where coverage is full, so it
	// wants a heavy pen or it comes out as shading. The choice follows the
	// rendering, not the height: the five-row lens is drawn in characters.
	//
	// Small glyphs get relatively heavier, as hinted screen fonts do: a fixed
	// 0.085 at four rows is two thirds of a cell and never reaches full
	// coverage.
	penRows      = 0.85
	penRowsAscii = 1.40

	// penMax is the thickest half-width allowed. Past it a digit's counters
	// (the hole in an 8, the gap in a 6) close up; at 0.30 an 8 and a 0 render
	// identically.
	penMax = 0.25
)

// strokeHalfWidthAt is the pen for a glyph h rows tall, drawn as a silhouette
// (solid) or in copies of its own digit. It keeps the stroke about a fixed
// number of rows thick, between StrokeHalfWidth and penMax.
func strokeHalfWidthAt(h float64, solid bool) float64 {
	if h <= 0 {
		return StrokeHalfWidth
	}
	rows := penRowsAscii
	if solid {
		rows = penRows
	}
	half := rows / 2 / h
	if half < StrokeHalfWidth {
		return StrokeHalfWidth
	}
	if half > penMax {
		return penMax
	}
	return half
}

// DigitStrokes returns the outline of a digit, or nil if it is out of range.
func DigitStrokes(digit int) []Stroke {
	if digit < 0 || digit > 9 {
		return nil
	}
	return digitOutlines[digit]
}

// Arc samples an elliptical arc. Angles are in degrees, measured with y running
// downward: 0 is right of centre, 90 below, 180 left, 270 above. from may be
// greater than to, which sweeps the other way.
func Arc(cx, cy, rx, ry, from, to float64) Stroke {
	const steps = 20
	s := make(Stroke, 0, steps+1)
	for i := 0; i <= steps; i++ {
		deg := from + (to-from)*float64(i)/steps
		r := deg * math.Pi / 180
		s = append(s, Point{cx + rx*math.Cos(r), cy + ry*math.Sin(r)})
	}
	return s
}

// Line builds a polyline through the given points.
func Line(pts ...Point) Stroke { return Stroke(pts) }

var digitOutlines = [10][]Stroke{
	// 0, a single closed ellipse.
	{Arc(0.50, 0.50, 0.30, 0.42, 0, 360)},

	// 1, flag, stem, foot.
	{
		Line(Point{0.28, 0.24}, Point{0.50, 0.08}),
		Line(Point{0.50, 0.08}, Point{0.50, 0.92}),
		Line(Point{0.26, 0.92}, Point{0.74, 0.92}),
	},

	// 2, top bowl sweeping right, diagonal down, foot.
	{
		append(
			Arc(0.50, 0.30, 0.30, 0.21, 180, 380),
			Point{0.72, 0.47}, Point{0.20, 0.90},
		),
		Line(Point{0.16, 0.90}, Point{0.86, 0.90}),
	},

	// 3, two right-facing bowls, open on the left.
	{
		Arc(0.50, 0.29, 0.28, 0.21, 200, 450),
		Arc(0.50, 0.71, 0.30, 0.21, 270, 520),
	},

	// 4, stem with a diagonal shoulder and a crossbar.
	{
		Line(Point{0.70, 0.06}, Point{0.70, 0.92}),
		Line(Point{0.70, 0.06}, Point{0.14, 0.66}, Point{0.90, 0.66}),
	},

	// 5, flat top, left shoulder, lower bowl.
	{
		Line(Point{0.80, 0.10}, Point{0.26, 0.10}, Point{0.30, 0.42}),
		Arc(0.50, 0.66, 0.30, 0.26, 235, 520),
	},

	// 6, a sweep from upper right down the left, closing into a bottom loop.
	{
		Arc(0.50, 0.50, 0.30, 0.42, 300, 150),
		Arc(0.50, 0.68, 0.30, 0.24, 0, 360),
	},

	// 7, flat top and a long diagonal.
	{Line(Point{0.14, 0.10}, Point{0.86, 0.10}, Point{0.40, 0.92})},

	// 8, two stacked loops, the lower one wider.
	{
		Arc(0.50, 0.30, 0.26, 0.22, 0, 360),
		Arc(0.50, 0.71, 0.30, 0.21, 0, 360),
	},

	// 9, the mirror of 6: a top loop with a tail down the right.
	{
		Arc(0.50, 0.32, 0.30, 0.24, 0, 360),
		Arc(0.50, 0.55, 0.30, 0.37, -20, 100),
	},
}
