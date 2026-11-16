package glyph

import "github.com/terminaltrove/lumon/internal/ease"

// Two ramps turn coverage into a character: GlyphRamp for an enlarged digit,
// which needs a silhouette, and EmergeRamp for a cell fading up into a digit.

// GlyphRamp is the edge shading of an enlarged digit, densest to lightest. It
// has no digit-like characters, or a stroke's antialiased edge reads as more
// numbers. rasterSamples = 3 gives ten coverage levels and the eye separates
// about four, so four steps are enough.
const GlyphRamp = "▓▒░·"

// GlyphSolid is what a fully covered cell shows. It is kept out of GlyphRamp:
// above glyphSnap a cell is inside the stroke, below it the cell is edge
// shading.
const GlyphSolid = '█'

// glyphSnap is the coverage at which a cell stops being an edge and becomes the
// stroke.
const glyphSnap = 0.72

// RampRune returns the character for a glyph cell at a given coverage: solid
// where the stroke fills the cell, lighter blocks along its antialiased edge.
func RampRune(coverage float64) rune {
	c := ease.Smoothstep(ease.Clamp01(coverage))
	if c >= glyphSnap {
		return GlyphSolid
	}
	ramp := []rune(GlyphRamp)
	idx := int(float64(len(ramp)) * (1 - c/glyphSnap))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(ramp) {
		idx = len(ramp) - 1
	}
	return ramp[idx]
}

// IsGlyphRune reports whether a character is GlyphSolid or from GlyphRamp.
func IsGlyphRune(r rune) bool {
	if r == GlyphSolid {
		return true
	}
	for _, c := range GlyphRamp {
		if c == r {
			return true
		}
	}
	return false
}

// EmergeRamp is the shading a digit-drawn glyph and a refilling cell come up
// through, densest to lightest, with the digit itself above it. No character
// in it may look like a digit (0, 8, S, X), or a small glyph's shading reads
// as more numbers.
const EmergeRamp = "%+=-;:,."

// emergeSnap is the density at which the cell stops shading and shows the digit.
const emergeSnap = 0.72

// EmergeRune returns the character a refilling cell shows at a given density.
func EmergeRune(digit rune, density float64) rune {
	c := ease.Smoothstep(ease.Clamp01(density))
	if c >= emergeSnap {
		return digit
	}
	ramp := []rune(EmergeRamp)
	idx := int(float64(len(ramp)-1) * (1 - c/emergeSnap))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(ramp) {
		idx = len(ramp) - 1
	}
	return ramp[idx]
}
