// Package ease holds arithmetic shared by packages that should not import each
// other, such as the board and the glyph rasteriser. A helper used in one place
// belongs where it is used.
package ease

// Clamp01 bounds a value to [0, 1]. NaN passes through unchanged.
func Clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// Smoothstep is the cubic 3t² - 2t³, whose slope is zero at 0 and 1 so a
// transition starts and ends gently. It does not clamp, so an out-of-range
// input stays visible; callers wrap it in Clamp01 where needed.
func Smoothstep(t float64) float64 { return t * t * (3 - 2*t) }
