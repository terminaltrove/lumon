package lumon

// columnDeadBand is how far past a column boundary, in columns, a digit must
// travel before it switches column. Without it, a digit drifting near a
// half-column flips back and forth every few frames.
const columnDeadBand = 0.3

// columnStep converts a continuous horizontal offset, in columns, to a whole
// column. It keeps the current column until the offset clears the dead band
// (a Schmitt trigger), and resolves a large jump in one call.
func columnStep(offset float64, current int) int {
	for offset > float64(current)+0.5+columnDeadBand {
		current++
	}
	for offset < float64(current)-0.5-columnDeadBand {
		current--
	}
	return current
}

// maxRowSag is how far a digit may drop below its own row, in rows, at pitch
// rowH.
//
// Vertical travel is downward only. A digit sagging into the blank rows under
// its grid row stays within {y .. y+pitch-1}, so it can never meet the digit
// below; a symmetric wobble would let the two overdraw on a shared row.
//
// It uses the floor of the eased pitch: mid-zoom the pitch is fractional and
// rowTop gives whole rows, so the shortest grid row is floor(pitch) deep.
// Rounding up lets a digit vanish under its neighbour.
func maxRowSag(rowH float64) int {
	if rowH < 2 {
		return 0
	}
	return int(rowH) - 1
}

// rowStep converts a continuous vertical offset, in rows, to a whole row with
// the same dead band as columnStep, clamped to [0, maxSag].
func rowStep(offset float64, current, maxSag int) int {
	for offset > float64(current)+0.5+columnDeadBand {
		current++
	}
	for offset < float64(current)-0.5-columnDeadBand {
		current--
	}
	if current < 0 {
		current = 0
	}
	if current > maxSag {
		current = maxSag
	}
	return current
}

// glyphBand is how far past MinGlyphHeight a digit must grow before it is drawn
// as a glyph, and how far back below it must fall before it gives up being one.
const glyphBand = 0.3

// glyphOn decides whether a digit is drawn as a multi-cell glyph or as an
// ordinary character, keeping its previous answer within the band, so a digit
// near the threshold does not flip every frame.
func glyphOn(height float64, on bool) bool {
	if on {
		return height > MinGlyphHeight-glyphBand
	}
	return height >= MinGlyphHeight+glyphBand
}
