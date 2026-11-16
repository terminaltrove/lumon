package lumon

import (
	"github.com/terminaltrove/lumon/internal/canvas"
	"github.com/terminaltrove/lumon/internal/glyph"
	"github.com/terminaltrove/lumon/internal/noise"
	"github.com/terminaltrove/lumon/internal/physics"
	"github.com/terminaltrove/lumon/internal/theme"
	"math"
)

// The file as a volume, and the refinement happening inside it.
//
// This is not a diversion: diversions play on a copy and never touch the file,
// and this is a view you refine in. It is a render mode of ScreenMain instead:
// drawGrid branches, and the cursor, bins, refining, flights, credit and
// persistence all run as usual.
//
// The live board is the front face of a lattice of digit planes; a woken
// cluster stands in front of it. The camera is flown (rig.go). The mouse works
// through the anchor index the renderer builds while drawing.

const (
	// solidTilt is how far the file is rocked away from the viewer and solidYaw
	// how far it is turned, both in degrees: where the camera starts (rig.go).
	//
	// Pitch turns depth into height, of which the panel has little (about
	// twenty rows), so it is kept small; a larger pitch forces the fit to shrink
	// the digits until they are unreadable. Yaw spends depth on width, where
	// there is room. The refiner can tip it further with , and .
	solidTilt = 7.0
	solidYaw  = 20.0

	// solidLayers is how many planes of digits the file is built from and
	// solidLayerGap how far apart they are, in visual columns.
	//
	// Every site on every plane is occupied, so each face has straight edges,
	// which is what makes it read as a volume. More planes, closer together,
	// project to less than a cell apart and interleave into a moiré. Four gaps
	// of 26 give a depth of 104, about the width of a nineteen-cell block, so
	// the volume is roughly square in plan (the plane the camera turns in).
	solidLayers   = 5
	solidLayerGap = 26.0

	// solidMargin is how many cells of mass are laid down beyond the viewport on
	// every side. Depth alone never reaches the panel edge (a prism's back face
	// projects inside its front face), so without a margin the volume looks
	// like an object in a frame. Three cells reaches the edge on a 120-column
	// terminal without drawing much that is then clipped.
	solidMargin = 3

	// solidSway is how far each plane of the mass wanders, in visual columns,
	// and solidSwayScale and solidSwayRate the field it is read from. Per
	// plane, because per digit would cost thousands of Perlin lookups a frame.
	solidSway      = 0.35
	solidSwayScale = 3.7
	solidSwayRate  = 0.09

	// solidDim is how far the mass is blended from the resting grid's colour
	// (Backdrop) toward the page's ink, front plane to back. Darker than the
	// board so it does not compete with it, and short of the ink so the
	// deepest plane is faint, not gone.
	solidDim = 0.82

	// solidSpread is how fast the planes rush back from the board on arrival.
	// On the way out Transit takes over, so there is nothing to collapse.
	solidSpread = 4.5

	// solidRowGapCells is how far apart the file's rows are set in the volume,
	// as a fraction of the cell's width, so the aspect survives a zoom.
	// Squatter than the flat grid, because the panel is short.
	solidRowGapCells = 0.5

	// solidLift is how far an awake cluster stands in front of the frontmost
	// plane, in visual columns. It must be in front of the whole file to read
	// as nearer, and small against the eye distance (solidDolly), or
	// perspective magnifies the cluster off the panel.
	solidLift = solidLayerGap

	// solidFront is the depth of the frontmost plane, which is where the live
	// board sits and what the lift is measured from.
	solidFront = float64(solidLayers-1) / 2 * solidLayerGap

	// solidShake is how far a shaking cluster travels in depth, in visual
	// columns, and solidShakeScale the noise field it is sampled from.
	solidShake      = 10.0
	solidShakeScale = 0.31

	// solidDolly is the distance to the eye standing still, in visual columns,
	// and solidFill how much of the panel the live face takes at that
	// distance. At this distance a cluster one plane proud is about a third
	// larger than the plane behind. The zoom moves the eye and leaves the
	// scale (solidFit), so flying in overflows the panel.
	solidDolly = 260.0
	solidFill  = 0.90

	// solidNearest is the most a digit may be magnified by being near.
	// Unbounded, every near cluster hits MaxGlyphHeight and they all look the
	// same size whatever their distance.
	solidNearest = 1.8

	// solidFollow is how fast the aim slides after the visible block, in
	// radians a second, turning a viewport scroll's whole-cell step into a
	// glide.
	solidFollow = 5.5
)

// Solid is the file as a volume.
type Solid struct {
	cam *Camera

	// Rig is the camera the refiner flies: the zoom, the turn, the tip, and the
	// slow tour it takes on its own while nobody is working.
	Rig *Rig

	// atX and atY are the point the view is centred on, in grid cells, eased
	// toward solidCentre.
	atX, atY *physics.Scalar

	// spread is how far back the mass has travelled from the board, 0 to 1.
	spread *physics.Scalar

	cloud []Pt3
	mass  []Pt3

	// shade is one attribute per plane and shadeOf the theme they were built
	// for. The theme can change (remotely it is the viewer's choice).
	shade   []canvas.Attr
	shadeOf *theme.Theme

	// fit is the scale the face is drawn at and fitFor the viewport and panel
	// sizes it was measured for. Cached because solidFit sweeps two dozen
	// angles and aim runs twice a frame.
	fit    float64
	fitFor [4]int

	// t is the clock the sway and depth shake are sampled over, in seconds.
	t float64
}

// NewSolid opens the view on a cursor.
func NewSolid(m *Model) *Solid {
	cx, cy := m.solidCentre()
	return &Solid{
		cam:    NewCamera(),
		Rig:    NewRig(solidYaw, solidTilt),
		atX:    physics.NewScalar(cx, solidFollow),
		atY:    physics.NewScalar(cy, solidFollow),
		spread: physics.NewScalar(0, solidSpread),
	}
}

// solidCentre is where the view is aimed: the middle of the visible block, in
// grid cells. Aiming at (or leaning toward) the cursor pushes part of the
// block off the panel, hiding clusters near the far edge; the player flies the
// camera instead (rig.go).
func (m *Model) solidCentre() (x, y float64) {
	l := m.Layout
	return float64(l.ViewportX) + float64(l.ViewportW-1)/2,
		float64(l.ViewportY) + float64(l.ViewportH-1)/2
}

// Step moves the view's aim toward x, y (grid cells). landed says the trip in
// is over; the mass does not spread until then, or the spring would finish
// before the volume is first drawn.
func (s *Solid) Step(dt float64, x, y float64, landed bool) {
	s.t += dt
	s.Rig.Step(dt)
	s.atX.Step(dt, x)
	s.atY.Step(dt, y)
	if landed {
		s.spread.Step(dt, 1)
	}
}

// solidFit is the scale at which the live face (not the whole volume; the mass
// runs off the edges) fits inside the panel at the starting dolly.
//
// It fits the face's corners in both axes, taken at the lifted depth (see
// solidFaceCorners), at every yaw the idle tour reaches: the face is widest
// square-on, so a fit taken at the resting yaw overflows later in the tour.
// The dolly is not swept, so flying in still overflows.
func solidFit(l *Layout, panel Panel) float64 {
	probe := NewCamera()
	probe.Dolly, probe.Pitch, probe.Scale = solidDolly, solidTilt, 1

	fit := math.Inf(1)
	for turn := 0; turn < 360; turn += 15 {
		probe.Yaw = solidYaw + float64(turn)
		loX, hiX, loY, hiY := solidFaceBox(probe, l)
		if math.IsInf(loX, 1) {
			continue
		}

		// Half the span, not the furthest corner from the middle: yaw moves the
		// face off centre, and aim re-centres it with a pan.
		widest := math.Max((hiX-loX)/2, 1)
		tallest := math.Max((hiY-loY)/2, 1)

		fit = math.Min(fit, math.Min(
			float64(panel.W)/2*solidFill/widest,
			float64(panel.H)/2*solidFill/tallest,
		))
	}
	if math.IsInf(fit, 1) {
		return 1
	}
	return fit
}

// solidPlaneZ is the depth of one plane of the mass (plane 0 at the back),
// with the arrival spring applied. Measured back from solidFront, so the live
// face holds still while the mass springs away; at spread 0 every plane is on
// the board.
func solidPlaneZ(plane int, spread float64) float64 {
	return solidFront - float64(solidLayers-1-plane)*solidLayerGap*spread
}

// solidDigit is the digit at one site of the mass. It is an integer hash, not
// Perlin noise, which is smooth and would give visible streaks of similar
// digits. It is a pure function of the file (seed and epoch) and the site, so
// refiners at one board see the same mass.
func solidDigit(seed int64, epoch, gx, gy, plane int) rune {
	h := uint64(seed)*0x9E3779B97F4A7C15 +
		uint64(int64(gx))*0xBF58476D1CE4E5B9 +
		uint64(int64(gy))*0x94D049BB133111EB +
		uint64(int64(plane))*0xD6E8FEB86659FD93 +
		uint64(int64(epoch))*0xA24BAED4963EE407
	h ^= h >> 33
	h *= 0xFF51AFD7ED558CCD
	h ^= h >> 29
	return rune('0' + h%10)
}

// sway is how far one plane of the mass has slid, in cells. See solidSway.
func (s *Solid) sway(plane int) (dx, dy float64) {
	at := float64(plane) * solidSwayScale
	t := s.t * solidSwayRate
	return noise.PerlinAnimation.Noise3D(at, 0, t) * solidSway,
		noise.PerlinAnimation.Noise3D(at, 41.5, t) * solidSway
}

// shades is the attribute each plane is drawn in, rebuilt only when the theme
// changes.
func (s *Solid) shades(t *theme.Theme) []canvas.Attr {
	if s.shadeOf == t && len(s.shade) == solidLayers {
		return s.shade
	}
	s.shade = make([]canvas.Attr, solidLayers)
	for i := range s.shade {
		// Counted over planes, not gaps, so even the frontmost plane is a
		// step darker than Backdrop and the ring around the board is
		// distinguishable from it.
		back := float64(solidLayers-i) / float64(solidLayers)
		s.shade[i] = t.On(theme.BlendColors(t.Backdrop, t.Ink, back*solidDim), false)
	}
	s.shadeOf = t
	return s.shade
}

// aim points the camera and sets the scale. Both builders call it, since
// shots is also reached from solidPlaces and the tests; within a frame it
// always gives the same camera.
func (s *Solid) aim(m *Model, panel Panel) {
	l := m.Layout
	if key := [4]int{l.ViewportW, l.ViewportH, panel.W, panel.H}; key != s.fitFor {
		s.fit, s.fitFor = solidFit(l, panel), key
	}
	s.cam.Scale = s.fit

	// The zoom moves the eye, not the scale.
	s.Rig.Aim(s.cam, solidDolly)

	// Last, through this frame's camera: re-centring on the face each frame
	// keeps the board on the panel as the view turns.
	s.cam.PanX, s.cam.PanY = 0, 0
	s.cam.PanX, s.cam.PanY = solidCentreOn(s.cam, m.Layout, panel)
}

// solidFaceCorners are the four corners of the live face, taken at the depth
// corruption is lifted to: the nearest anything that matters is drawn. The
// pitch turns nearness into vertical offset, so a fit at the face's own depth
// would push woken clusters off the bottom of the panel.
func solidFaceCorners(l *Layout) []Pt3 {
	across := float64(maxi(l.ViewportW-1, 1)) * l.CellW / 2
	down := float64(maxi(l.ViewportH-1, 1)) * l.solidRowGap() / 2

	out := make([]Pt3, 0, 4)
	for _, x := range [2]float64{-across, across} {
		for _, y := range [2]float64{-down, down} {
			out = append(out, Pt3{X: x, Y: y, Z: solidFront + solidLift, Digit: '0'})
		}
	}
	return out
}

// solidFaceBox is where the face lands, in columns and rows either side of the
// middle of the panel, through the given camera. It projects onto a huge panel
// so nothing is clipped.
func solidFaceBox(c *Camera, l *Layout) (loX, hiX, loY, hiY float64) {
	wide := Panel{X: 0, Y: 0, W: 1 << 12, H: 1 << 12}
	loX, hiX = math.Inf(1), math.Inf(-1)
	loY, hiY = math.Inf(1), math.Inf(-1)
	for _, s := range c.Shots(solidFaceCorners(l), wide) {
		x := float64(s.Col) - float64(wide.W)/2
		y := float64(s.Row) - float64(wide.H)/2
		loX, hiX = math.Min(loX, x), math.Max(hiX, x)
		loY, hiY = math.Min(loY, y), math.Max(hiY, y)
	}
	return loX, hiX, loY, hiY
}

// solidCentreOn is the pan that puts the face in the middle of the panel.
func solidCentreOn(c *Camera, l *Layout, panel Panel) (panX, panY float64) {
	loX, hiX, loY, hiY := solidFaceBox(c, l)
	if math.IsInf(loX, 1) {
		return 0, 0
	}
	return -(loX + hiX) / 2, -(loY + hiY) / 2
}

// shots builds the live face into a cloud and projects it, back to front. One
// point per visible cell on the front plane, tagged with its grid ID, because
// the depth sort shuffles the cloud.
func (s *Solid) shots(m *Model, panel Panel) []Shot {
	l := m.Layout
	s.aim(m, panel)
	s.cloud = s.cloud[:0]

	atX, atY := s.atX.Value(), s.atY.Value()
	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			n := m.Grid.At(gx, gy)
			if n == nil || m.Flights.InFlight(n.ID) {
				continue
			}

			// The same offsets as the flat board (ScreenAnchor): agitation
			// takes precedence over ambient drift.
			dx, dy := m.Agitation.Offset(n.ID)
			if dx == 0 && dy == 0 {
				dx, dy = m.Drift.Offset(gx, gy)
			}

			// And the shockwave on top, also as ScreenAnchor does.
			rx, ry := m.rippleOffset(gx, gy)
			dx, dy = dx+rx, dy+ry

			z := solidFront
			if m.IsActive(n.BadGroupID) {
				z = solidFront + solidLift
				z += noise.PerlinAnimation.Noise3D(
					float64(gx)*solidShakeScale,
					float64(gy)*solidShakeScale,
					s.t,
				) * solidShake
			}

			// Visual columns at the file's own scale, centred on the view's aim:
			// a cell is CellW across and a grid row solidRowGap down.
			s.cloud = append(s.cloud, Pt3{
				X:     (float64(gx) - atX + dx) * l.CellW,
				Y:     (atY - float64(gy) - dy) * l.solidRowGap(),
				Z:     z,
				Digit: rune('0' + n.Value),
				Tag:   n.ID,
			})
		}
	}
	if len(s.cloud) == 0 {
		return nil
	}
	return s.cam.Shots(s.cloud, panel)
}

// drawSolid paints the file from the camera, in place of drawGrid.
//
// A digit Magnify has enlarged is drawn as a rasterised glyph, sized by its
// height and by perspective; everything else is one character. Both go into
// the anchor index: a glyph over its whole box, a plain digit at its single
// cell (not the flat grid's CellWidth by RowPitch footprint, which here would
// claim cells another digit is standing in).
func (m *Model) drawSolid(buf *canvas.Buffer) {
	panel := m.gridPanel()
	top, bottom := panel.Y, panel.Y+panel.H-1

	// The mass, then the board in front of it. The mass is all at or behind
	// the front plane, so the two clouds never need sorting against each other.
	m.drawSolidMass(buf, panel)

	for _, s := range m.solid.shots(m, panel) {
		n := &m.Grid.Cells[s.Tag]
		gx, gy := s.Tag%GridSize, s.Tag/GridSize

		// See solidNearest.
		near := clampF(s.Near, 1/solidNearest, solidNearest)
		if h := m.Magnify.Height(gx, gy) * near; glyphOn(h, false) {
			solid := m.IsActive(n.BadGroupID)
			if cov := glyph.RasterizeFor(n.Value, h*glyph.CellAspect, h, m.glyphTilt(n), solid); cov != nil {
				x, y := s.Col-cov.W/2, s.Row-cov.H/2
				paintCoverage(buf, x, y, cov, s.Digit, solid,
					m.glyphAttr(s.Tag), top, bottom)
				m.markAnchorBox(n.ID, x, y, cov)
				continue
			}
		}

		ch := s.Digit
		if v, spinning := m.Regen.Display(n.ID); spinning {
			ch = glyph.EmergeRune(rune('0'+v), m.Regen.Density(n.ID))
		}
		buf.Set(s.Col, s.Row, ch, m.cellAttr(n, gx, gy))
		m.markAnchorAt(n.ID, s.Col, s.Row)
	}
}

// drawSolidMass paints the mass behind the board: every site on every plane,
// reaching solidMargin cells past the viewport.
//
// The cloud is built back to front and goes through Cast unsorted. Within a
// plane every digit has the same attribute and size, so overlap order does not
// show; sorting thousands of points a frame would be too expensive.
func (m *Model) drawSolidMass(buf *canvas.Buffer, panel Panel) {
	s := m.solid
	l := m.Layout
	s.aim(m, panel)
	shade := s.shades(m.Theme)
	spread := s.spread.Value()
	atX, atY := s.atX.Value(), s.atY.Value()

	x0, x1 := l.ViewportX-solidMargin, l.ViewportX+l.ViewportW+solidMargin
	y0, y1 := l.ViewportY-solidMargin, l.ViewportY+l.ViewportH+solidMargin

	s.mass = s.mass[:0]
	for plane := 0; plane < solidLayers; plane++ {
		z := solidPlaneZ(plane, spread)
		dx, dy := s.sway(plane)

		// The frontmost plane is the board's own, so only the ring around the
		// viewport is filled. Inside it, a cell whose digit is in flight to a
		// bin would show a hash digit and seem never to have left.
		face := plane == solidLayers-1

		for gy := y0; gy < y1; gy++ {
			for gx := x0; gx < x1; gx++ {
				if face && gx >= l.ViewportX && gx < l.ViewportX+l.ViewportW &&
					gy >= l.ViewportY && gy < l.ViewportY+l.ViewportH {
					continue
				}
				s.mass = append(s.mass, Pt3{
					X:     (float64(gx) - atX + dx) * l.CellW,
					Y:     (atY - float64(gy) - dy) * l.solidRowGap(),
					Z:     z,
					Digit: solidDigit(m.gridSeed, m.epoch, gx, gy, plane),
					Tag:   plane,
				})
			}
		}
	}

	for _, sh := range s.cam.Cast(s.mass, panel) {
		buf.Set(sh.Col, sh.Row, sh.Digit, shade[sh.Tag])
	}
}

// solidPlaces is where the view has each digit, for the trip in and out.
func (m *Model) solidPlaces() []Placed {
	if m.solid == nil {
		return nil
	}
	panel := m.gridPanel()

	// The live face only: Transit has one particle per visible cell.
	out := make([]Placed, 0, 256)
	for _, s := range m.solid.shots(m, panel) {
		out = append(out, Placed{X: float64(s.Col), Y: float64(s.Row), Digit: s.Digit})
	}
	return out
}

// toggleSolid turns the view on or off. The digits fly between the flat grid
// and the projection through Transit, as they do into a diversion; a trip
// already under way is reversed.
func (m *Model) toggleSolid() {
	if m.Room == nil || m.Screen != ScreenMain || m.Diverting() {
		return
	}
	if m.transit != nil {
		m.transit.Reverse(m)
		m.solid = nil
		return
	}

	if m.solid != nil {
		places := m.solidPlaces()
		m.solid = nil
		m.dropIndex()
		m.transit = NewTransit(m, places, true)
		return
	}

	m.solid = NewSolid(m)
	m.dropIndex()
	m.transit = NewTransit(m, m.solidPlaces(), false)
}

// Solid reports whether the file is being seen as a volume.
func (m *Model) Solid() bool { return m.solid != nil }

// touchRig tells the camera the refiner is working, so the idle tour pauses.
// It is called from the game's keys only, not the camera keys, so steering the
// camera does not stop the tour. Safe to call with no volume.
func (m *Model) touchRig() {
	if m.solid != nil {
		m.solid.Rig.Touch()
	}
}

// solidAnchors is where the volume has each visible digit this frame, by grid
// id, or nil when the board is flat. A refinement uses it so flights leave
// from where the digits appear.
func (m *Model) solidAnchors() map[int]physics.Vec2 {
	if m.solid == nil {
		return nil
	}
	panel := m.gridPanel()
	shots := m.solid.shots(m, panel)
	out := make(map[int]physics.Vec2, len(shots))
	for _, s := range shots {
		out[s.Tag] = physics.Vec2{X: float64(s.Col), Y: float64(s.Row)}
	}
	return out
}
