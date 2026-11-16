package lumon

import (
	"github.com/terminaltrove/lumon/internal/glyph"
	"strings"
	"testing"
)

// An enlarged digit is a shape spread over many cells. Drawn in copies of
// itself, figure and ground are the same character and there is no silhouette
// to read.
func TestNoGlyphCharacterIsADigit(t *testing.T) {
	for _, r := range glyph.GlyphRamp + string(glyph.GlyphSolid) {
		if r >= '0' && r <= '9' {
			t.Errorf("the glyph ramp contains %q, which is a digit", r)
		}
	}
	for h := MinGlyphHeight; h <= MaxGlyphHeight; h += 0.25 {
		for d := 0; d <= 9; d++ {
			cov := glyph.Rasterize(d, h*glyph.CellAspect, h)
			for _, c := range cov.Cells {
				if c <= 0 {
					continue
				}
				if r := glyph.RampRune(c); r >= '0' && r <= '9' {
					t.Fatalf("a %d at %.2f rows drew %q", d, h, r)
				}
			}
		}
	}
}

// A small glyph takes its identity from the character and a large one from its
// shape. At three rows a solid silhouette cannot tell a nought from an eight,
// while an outline in copies of its own digit is readable. Above about five rows
// a mosaic of little digits becomes a texture with no figure.
func TestASmallGlyphIsDrawnInItsOwnDigit(t *testing.T) {
	for h := MinGlyphHeight; h <= MaxGlyphHeight; h += 0.25 {
		for d := 0; d <= 9; d++ {
			cov := glyph.Rasterize(d, h*glyph.CellAspect, h)
			digit := rune('0' + d)
			found := false
			for _, c := range cov.Cells {
				if c > 0 && glyph.EmergeRune(digit, c) == digit {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("a %d at %.2f rows has no cell drawn as itself", d, h)
			}
		}
	}
}

// Corruption speaks in blocks and everything else in characters. No height
// threshold can express this: a 5.5-row lens stays in characters and a
// 5.75-row cluster draws in blocks.
func TestTheVoiceIsAFactAboutTheDigit(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	for i := 0; i < 60*40 && m.FirstActive() == nil; i++ {
		tick(m, 1)
	}
	grp := m.FirstActive()
	if grp == nil {
		t.Skip("nothing woke up")
	}
	tick(m, 60*40)
	drawTo(m)

	// Awake: every glyph on the board is a cluster member, and every one is a shape.
	live := 0
	for _, b := range m.boxes {
		n := &m.Grid.Cells[b.id]
		if !m.IsActive(n.BadGroupID) {
			t.Errorf("an ordinary digit was drawn large at all, at %.2f rows", b.height)
			continue
		}
		live++
		if !b.solid {
			t.Errorf("a corrupt digit at %.2f rows was drawn in characters", b.height)
		}
	}
	if live == 0 {
		t.Skip("no cluster member was on screen")
	}

	// The moment the cluster sleeps its digits speak in characters, at the same
	// heights, on the same frame.
	m.Room.Sleep(grp.ID)
	tick(m, 2)
	drawTo(m)

	member := map[int]bool{}
	for _, id := range grp.NumberIDs {
		member[id] = true
	}
	spoke := 0
	for _, b := range m.boxes {
		if !member[b.id] || b.height < MinGlyphHeight {
			continue
		}
		spoke++
		if b.solid {
			t.Errorf("a digit that is no longer corrupt was still drawn as a shape "+
				"at %.2f rows", b.height)
		}
	}
	if spoke == 0 {
		t.Error("nothing was still large enough to draw a frame after being put to sleep")
	}
}

// A refilling cell is one character resolving into one digit, so it must end as
// the digit.
func TestARefillingCellEndsAsItsDigit(t *testing.T) {
	if got := glyph.EmergeRune('7', 1); got != '7' {
		t.Errorf("a fully emerged cell drew %q, want '7'", got)
	}
	if got := glyph.EmergeRune('7', 0); got == '7' {
		t.Error("a cell at no density already showed its digit")
	}
}

// Otherwise the shading has fewer steps than it claims.
func TestTheWholeGlyphRampIsUsed(t *testing.T) {
	seen := map[rune]bool{}
	for c := 0.0; c <= 1.0; c += 0.001 {
		seen[glyph.RampRune(c)] = true
	}
	for _, r := range glyph.GlyphRamp + string(glyph.GlyphSolid) {
		if !seen[r] {
			t.Errorf("no coverage produces %q", r)
		}
	}
}

// The character ramp is also the small-glyph renderer, so a digit-shaped shade
// would put 0s inside a three-row 4.
func TestNeitherRampIsDigitShaped(t *testing.T) {
	const digitish = "0123456789OSXIlB"
	for _, name := range []struct {
		what string
		ramp string
	}{
		{"the block ramp", glyph.GlyphRamp + string(glyph.GlyphSolid)},
		{"the character ramp", glyph.EmergeRamp},
	} {
		for _, r := range name.ramp {
			if strings.ContainsRune(digitish, r) {
				t.Errorf("%s contains %q, which reads as a number", name.what, r)
			}
		}
	}
}

// In this rendering the character identifies the digit, and a stroke too thin
// to reach full coverage loses it.
func TestASmallGlyphIsMostlyItsOwnDigit(t *testing.T) {
	for _, h := range []float64{MinGlyphHeight + glyphBand, 4.0, 6.0, BreatheHigh} {
		for d := 0; d <= 9; d++ {
			cov := glyph.Rasterize(d, h*glyph.CellAspect, h)
			digit := rune('0' + d)
			inked, own := 0, 0
			for _, c := range cov.Cells {
				if c <= 0 {
					continue
				}
				inked++
				if glyph.EmergeRune(digit, c) == digit {
					own++
				}
			}
			if inked == 0 {
				t.Fatalf("a %d at %.2f rows has no ink at all", d, h)
			}
			if float64(own)/float64(inked) < 0.4 {
				t.Errorf("a %d at %.2f rows is %d of %d cells its own digit; the rest is shading",
					d, h, own, inked)
			}
		}
	}
}
