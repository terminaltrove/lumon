package glyph

// The alphabet as centreline strokes, in the digits' unit box. raster.go
// computes coverage as distance to a centreline, so a font's filled contours
// would need a second, even-odd fill path.

// LetterStrokes is the outline of one letter, or nil if it has none. A space
// returns an empty, non-nil slice so the layout advances without drawing.
func LetterStrokes(r rune) []Stroke {
	if r == ' ' {
		return []Stroke{}
	}
	return LetterOutlines[r]
}

const (
	// letterTop and letterBottom are the cap height. The letters sit slightly
	// inside the box so neighbours do not touch when they are set close.
	letterTop    = 0.14
	letterBottom = 0.86

	// letterLeft and letterRight are the side bearings.
	letterLeft  = 0.14
	letterRight = 0.80
)

var LetterOutlines = map[rune][]Stroke{
	// P, a stem and a bowl closing back onto it.
	'P': {
		Line(Point{letterLeft, letterBottom}, Point{letterLeft, letterTop}),
		append(
			Stroke{{X: letterLeft, Y: letterTop}},
			append(
				Arc(0.40, 0.31, 0.34, 0.17, -90, 90),
				Point{X: letterLeft, Y: 0.48},
			)...,
		),
	},

	// R, P, with a leg.
	'R': {
		Line(Point{letterLeft, letterBottom}, Point{letterLeft, letterTop}),
		append(
			Stroke{{X: letterLeft, Y: letterTop}},
			append(
				Arc(0.38, 0.31, 0.32, 0.17, -90, 90),
				Point{X: letterLeft, Y: 0.48},
			)...,
		),
		Line(Point{0.40, 0.48}, Point{letterRight, letterBottom}),
	},

	// A, two legs and a crossbar.
	'A': {
		Line(Point{letterLeft, letterBottom}, Point{0.47, letterTop}, Point{letterRight, letterBottom}),
		Line(Point{0.25, 0.62}, Point{0.69, 0.62}),
	},

	// I, a stem, with serifs so it is not mistaken for a stray rule.
	'I': {
		Line(Point{0.47, letterTop}, Point{0.47, letterBottom}),
		Line(Point{0.28, letterTop}, Point{0.66, letterTop}),
		Line(Point{0.28, letterBottom}, Point{0.66, letterBottom}),
	},

	// S, two opposed bowls joined at the waist. One stroke: the segment the
	// append draws between the arcs is the spine, so each arc must end at the
	// waist or the join slashes through the letter.
	'S': {
		append(
			Arc(0.50, 0.32, 0.27, 0.19, 20, -250),
			Arc(0.50, 0.68, 0.27, 0.19, -70, 160)...,
		),
	},

	// E, a stem and three arms.
	'E': {
		Line(Point{letterLeft, letterTop}, Point{letterLeft, letterBottom}),
		Line(Point{letterLeft, letterTop}, Point{letterRight, letterTop}),
		Line(Point{letterLeft, 0.50}, Point{0.66, 0.50}),
		Line(Point{letterLeft, letterBottom}, Point{letterRight, letterBottom}),
	},

	// K, a stem and two diagonals meeting on it.
	'K': {
		Line(Point{letterLeft, letterTop}, Point{letterLeft, letterBottom}),
		Line(Point{letterRight, letterTop}, Point{letterLeft, 0.53}),
		Line(Point{0.30, 0.44}, Point{letterRight, letterBottom}),
	},

	// B, a stem and two bowls, the lower one wider.
	'B': {
		Line(Point{letterLeft, letterTop}, Point{letterLeft, letterBottom}),
		append(
			Stroke{{X: letterLeft, Y: letterTop}},
			append(Arc(0.38, 0.32, 0.30, 0.18, -90, 90), Point{X: letterLeft, Y: 0.50})...,
		),
		append(
			Stroke{{X: letterLeft, Y: 0.50}},
			append(Arc(0.38, 0.68, 0.34, 0.18, -90, 90), Point{X: letterLeft, Y: letterBottom})...,
		),
	},

	// C, one open bowl with the gap facing right.
	'C': {
		Arc(0.50, 0.50, 0.33, 0.36, -55, -305),
	},

	// D, a stem and one big bowl.
	'D': {
		Line(Point{letterLeft, letterTop}, Point{letterLeft, letterBottom}),
		append(
			Stroke{{X: letterLeft, Y: letterTop}},
			append(Arc(0.36, 0.50, 0.38, 0.36, -90, 90), Point{X: letterLeft, Y: letterBottom})...,
		),
	},

	// F, E without its foot.
	'F': {
		Line(Point{letterLeft, letterTop}, Point{letterLeft, letterBottom}),
		Line(Point{letterLeft, letterTop}, Point{letterRight, letterTop}),
		Line(Point{letterLeft, 0.50}, Point{0.64, 0.50}),
	},

	// G, C with a bar turning back in.
	'G': {
		Arc(0.50, 0.50, 0.33, 0.36, -55, -305),
		Line(Point{letterRight + 0.03, 0.50}, Point{0.56, 0.50}),
		Line(Point{letterRight + 0.03, 0.50}, Point{letterRight + 0.03, 0.72}),
	},

	// H, two stems and a crossbar.
	'H': {
		Line(Point{letterLeft, letterTop}, Point{letterLeft, letterBottom}),
		Line(Point{letterRight, letterTop}, Point{letterRight, letterBottom}),
		Line(Point{letterLeft, 0.50}, Point{letterRight, 0.50}),
	},

	// L, a stem and a foot.
	'L': {
		Line(Point{letterLeft, letterTop}, Point{letterLeft, letterBottom}, Point{letterRight, letterBottom}),
	},

	// M, two stems and a vee between them. One stroke, because the vee has to
	// meet both stems and drawing it separately leaves a gap at this size.
	'M': {
		Line(
			Point{letterLeft, letterBottom}, Point{letterLeft, letterTop},
			Point{0.47, 0.60}, Point{letterRight, letterTop},
			Point{letterRight, letterBottom},
		),
	},

	// N, two stems and a diagonal.
	'N': {
		Line(
			Point{letterLeft, letterBottom}, Point{letterLeft, letterTop},
			Point{letterRight, letterBottom}, Point{letterRight, letterTop},
		),
	},

	// O, a closed ring.
	'O': {
		Arc(0.47, 0.50, 0.33, 0.36, 0, 360),
	},

	// T, a stem hung from a bar.
	'T': {
		Line(Point{letterLeft, letterTop}, Point{letterRight, letterTop}),
		Line(Point{0.47, letterTop}, Point{0.47, letterBottom}),
	},

	// U, two stems joined by a bowl underneath. y is down, so the bowl sweeps
	// 180 to 0 through 90. Three strokes, since appending would draw a stray
	// join; the arc's ends meet the stems.
	'U': {
		Line(Point{letterLeft, letterTop}, Point{letterLeft, 0.62}),
		Arc(0.47, 0.62, 0.33, 0.24, 180, 0),
		Line(Point{letterRight, 0.62}, Point{letterRight, letterTop}),
	},

	// V, two diagonals meeting at a point.
	'V': {
		Line(Point{letterLeft, letterTop}, Point{0.47, letterBottom}, Point{letterRight, letterTop}),
	},

	// W, two vees. The middle peak stops short of the cap, or it reads as two
	// Vs.
	'W': {
		Line(
			Point{letterLeft, letterTop}, Point{0.31, letterBottom},
			Point{0.47, 0.40}, Point{0.63, letterBottom},
			Point{letterRight, letterTop},
		),
	},

	// Y, a vee on a stem, meeting above the middle.
	'Y': {
		Line(Point{letterLeft, letterTop}, Point{0.47, 0.52}, Point{letterRight, letterTop}),
		Line(Point{0.47, 0.52}, Point{0.47, letterBottom}),
	},

	// Z, two bars and the diagonal between them.
	'Z': {
		Line(
			Point{letterLeft, letterTop}, Point{letterRight, letterTop},
			Point{letterLeft, letterBottom}, Point{letterRight, letterBottom},
		),
	},

	// The two marks the statement needs. Both sit low, and the comma has a tail
	// so it is not a full stop at this resolution.
	'.': {
		Line(Point{0.45, letterBottom}, Point{0.52, letterBottom}),
	},
	',': {
		Line(Point{0.50, 0.78}, Point{0.50, letterBottom}, Point{0.42, 0.95}),
	},

	// %, a diagonal between two rings. It is set to the digits' height
	// (0.08-0.92), not the letters' cap height, because it is drawn only beside
	// digits. It is here because StrokesFor sends anything outside 0-9 to
	// LetterStrokes. The rings are ellipses with counters, so they do not fill
	// in when the pen thickens at small sizes.
	'%': {
		Line(Point{0.22, 0.92}, Point{0.78, 0.08}),
		Arc(0.30, 0.25, 0.13, 0.15, 0, 360),
		Arc(0.70, 0.75, 0.13, 0.15, 0, 360),
	},
}
