package lumon

import "github.com/terminaltrove/lumon/internal/physics"

// Magnify is how large each digit is currently drawn.
//
// Only corrupt numbers grow large, so size means "corrupt"; the cursor lens is
// capped well below what a woken cluster reaches.
//
// State is keyed by grid cell, not viewport slot, so a scroll does not reset a
// swollen cluster, and only growing cells are tracked.
//
// Height is in rows and is fractional: the rasterizer draws at fractional
// sizes, so growth is continuous.
const (
	// PlainHeight is a digit drawn as an ordinary single character.
	PlainHeight = 1.0

	// MaxGlyphHeight is the tallest a digit is ever drawn, in rows.
	MaxGlyphHeight = 11.0

	// MinGlyphHeight is the height, in rows, below which a digit is drawn as
	// the plain character; below it there are too few cells to beat it. Above
	// it, whether a digit is drawn solid or in copies of itself does not depend
	// on height; see glyphBox.solid.
	MinGlyphHeight = 2.2

	// magnifyFrequency is how fast a corrupt cluster swells, and the frequency
	// of each cluster's breathe spring (Room.Activate). It is low so the swell
	// is watched: a cluster stays an ordinary character for its first twenty-odd
	// seconds and crests near the end of its life (measured; the crest is not
	// linear in the frequency). physics.Scalar always settles on its target, so
	// changing this changes the pace and no final size.
	magnifyFrequency = 0.5

	// reachGrowFrequency is how fast a cluster comes up when it is pointed at or
	// ringfenced: about a fifth of a second, so the gesture answers without lag
	// and without reading as a cut.
	reachGrowFrequency = 7.0

	// restEpsilon is how close to plain a shrinking cell must get before it is
	// forgotten.
	restEpsilon = 0.01

	// MaxCollapseTilt is how far a refined digit leans on its way to a bin, in
	// degrees. Small: a digit that rotates far stops reading as a digit.
	MaxCollapseTilt = 14.0

	// MaxGlyphTilt is how far a fully escalated cluster member rocks, in
	// degrees. Smaller than MaxCollapseTilt because it is sustained.
	MaxGlyphTilt = 6.0

	// GlyphTiltStep is the angle quantum. Every distinct angle is its own entry
	// in the rasterizer's cache, so a continuously rocking cluster has to be
	// snapped to a handful of them.
	GlyphTiltStep = 1.5
)

// Magnify holds one growth spring per growing cell.
type Magnify struct {
	cells map[int]*physics.Scalar

	// freq is the rate a cell should converge at, kept separately so a caller
	// (the lens) can set it on the frame before the cell has a spring.
	freq map[int]float64
}

// NewMagnify returns a field with nothing growing.
func NewMagnify() *Magnify {
	return &Magnify{
		cells: make(map[int]*physics.Scalar),
		freq:  make(map[int]float64),
	}
}

// Len reports how many cells are currently growing or shrinking.
func (g *Magnify) Len() int { return len(g.cells) }

// Update advances every cell in targets toward its target. Tracked cells
// absent from targets shrink back to plain and are forgotten once they arrive.
func (g *Magnify) Update(dt float64, targets map[int]float64) {
	for id, want := range targets {
		if want < PlainHeight {
			want = PlainHeight
		}
		if want > MaxGlyphHeight {
			want = MaxGlyphHeight
		}
		c, ok := g.cells[id]
		if !ok {
			c = physics.NewScalar(PlainHeight, g.frequencyOf(id))
			g.cells[id] = c
		} else if f, named := g.freq[id]; named {
			c.Frequency = f
		}
		c.Step(dt, want)
	}

	for id, c := range g.cells {
		if _, still := targets[id]; still {
			continue
		}
		c.Step(dt, PlainHeight)
		if c.Value() <= PlainHeight+restEpsilon {
			delete(g.cells, id)
			delete(g.freq, id)
		}
	}
}

// SetFrequency changes how fast a cell converges, e.g. for the cursor lens,
// which should feel attached to the pointer. It is remembered even if the
// cell is not growing yet, so a rate set on the cell's first frame applies.
func (g *Magnify) SetFrequency(gridID int, f float64) {
	g.freq[gridID] = f
	if c, ok := g.cells[gridID]; ok {
		c.Frequency = f
	}
}

// frequencyOf is the rate a cell should converge at, defaulting to
// magnifyFrequency.
func (g *Magnify) frequencyOf(gridID int) float64 {
	if f, ok := g.freq[gridID]; ok {
		return f
	}
	return magnifyFrequency
}

// Height returns how tall a digit should be drawn, in rows.
func (g *Magnify) Height(gridX, gridY int) float64 {
	c, ok := g.cells[GridIndex(gridX, gridY)]
	if !ok {
		return PlainHeight
	}
	h := c.Value()
	if h < PlainHeight {
		return PlainHeight
	}
	if h > MaxGlyphHeight {
		return MaxGlyphHeight
	}
	return h
}

// Drop resets a cell at once, without shrinking.
func (g *Magnify) Drop(gridID int) {
	delete(g.cells, gridID)
	delete(g.freq, gridID)
}
