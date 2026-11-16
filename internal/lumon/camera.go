package lumon

import "math"

// A perspective camera for clouds of digits: rotate, divide by depth, sort
// back to front.

// Pt3 is one point of a cloud, in the cloud's own space. y is up, so a caller
// working in box coordinates flips it on the way in, as praise.go does.
type Pt3 struct {
	X, Y, Z float64

	// Digit is what is drawn there. It is fixed on the point so a turning cloud
	// does not shimmer.
	Digit rune

	// Tag is the caller's own label, passed through the projection untouched.
	// The depth sort reorders the cloud, so this is how a caller tells which of
	// its points each shot is.
	Tag int
}

// Shot is one projected point: where it landed and how near it is.
type Shot struct {
	Col, Row int
	Depth    float64
	Digit    rune
	Tag      int

	// Near is the perspective factor the point was drawn at: one at the plane
	// through the origin, above one in front of it, below behind. Callers that
	// size things by distance use it.
	Near float64
}

// Camera is a viewpoint over a cloud.
type Camera struct {
	// Yaw turns the cloud about the vertical and Pitch rocks it toward and away
	// from the viewer, both in degrees.
	Yaw, Pitch float64

	// Dolly is the distance to the eye, in cloud units. Larger is flatter and
	// closer to orthographic.
	Dolly float64

	// Scale is how many columns one world unit takes.
	Scale float64

	// PanX and PanY slide the picture across the panel after projection, in
	// columns and rows. Yaw moves a plane sideways in proportion to its depth
	// (x' = x·cos + z·sin), so a turned cloud is off-centre and needs panning
	// to fit without shrinking.
	PanX, PanY float64

	// shots is reused between frames.
	shots []Shot
}

// NewCamera returns a camera with the perspective PRAISE was built with.
func NewCamera() *Camera { return &Camera{Dolly: 9.0, Scale: 1} }

// Project runs a cloud through the camera and passes each visible point to
// emit, back to front, so nearer points overwrite further ones without a depth
// buffer.
func (c *Camera) Project(pts []Pt3, panel Panel, emit func(Shot)) {
	for _, s := range c.Shots(pts, panel) {
		emit(s)
	}
}

// Shots is the projected cloud, back to front. Valid until the next call.
func (c *Camera) Shots(pts []Pt3, panel Panel) []Shot {
	shots := c.Cast(pts, panel)

	// Insertion sort: callers' clouds are nearly sorted, since turning shuffles
	// points only locally.
	for i := 1; i < len(shots); i++ {
		for j := i; j > 0 && shots[j].Depth < shots[j-1].Depth; j-- {
			shots[j], shots[j-1] = shots[j-1], shots[j]
		}
	}
	return shots
}

// Cast is the projected cloud in the order it was given, with no depth sort.
// It is for callers that already draw in painter's order, such as the volume,
// built plane by plane from the back. Yaw mixes x into z, so a turned volume's
// planes interleave and Shots' insertion sort would go quadratic over thousands
// of points.
func (c *Camera) Cast(pts []Pt3, panel Panel) []Shot {
	w, h := panel.W, panel.H
	if w < 1 || h < 1 || len(pts) == 0 {
		return nil
	}

	ry := c.Yaw * math.Pi / 180
	rx := c.Pitch * math.Pi / 180
	sinY, cosY := math.Sin(ry), math.Cos(ry)
	sinX, cosX := math.Sin(rx), math.Cos(rx)

	if cap(c.shots) < len(pts) {
		c.shots = make([]Shot, 0, len(pts))
	}
	shots := c.shots[:0]

	for _, pt := range pts {
		// Yaw about the vertical, then pitch (Rx·Ry, no roll).
		x := pt.X*cosY + pt.Z*sinY
		z := -pt.X*sinY + pt.Z*cosY
		y := pt.Y*cosX - z*sinX
		z = pt.Y*sinX + z*cosX

		// Perspective divide, eye down the +z axis. A point at or behind the
		// eye is dropped, since a negative divisor would mirror it.
		den := c.Dolly - z
		if den <= 0.001 {
			continue
		}
		d := c.Dolly / den

		col := panel.X + int(math.Round(float64(w)/2+c.PanX+x*c.Scale*d))
		row := panel.Y + int(math.Round(float64(h)/2+c.PanY-y*c.Scale*d/RowAspect))
		if !panel.Contains(col, row) {
			continue
		}
		shots = append(shots, Shot{
			Col: col, Row: row, Depth: z, Digit: pt.Digit, Tag: pt.Tag, Near: d,
		})
	}

	c.shots = shots
	return shots
}
