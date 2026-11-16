package theme

import (
	"github.com/terminaltrove/lumon/internal/ease"
	"strconv"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// ScaryRedSeconds is how long a woken cluster takes to go from cyan to full
// red. Size signals "corrupt" and colour signals urgency: a cluster grows when
// it wakes and reddens the longer it is left, steadily through the breathe
// cycle. It is paced against magnifyFrequency's swell; FuseWait counts from
// when it ends.
const ScaryRedSeconds = 90.0

// ScaryRed is where the default theme's urgency ramp ends.
var ScaryRed = DefaultTheme.Urgent

// ScaryColorAt returns the colour a woken cluster is drawn in on the default
// theme, given how many seconds it has been awake. The renderer uses its own
// Theme.ScaryColour instead, so each session reddens in its own palette.
func ScaryColorAt(elapsed float64) canvas.Color {
	return DefaultTheme.ScaryColour(elapsed)
}

// BlendColors interpolates between two colours. t is clamped to [0,1], so the
// ends are exact. It runs per cell per frame at worst, so it must not allocate.
func BlendColors(a, b canvas.Color, t float64) canvas.Color {
	return canvas.Blend(a, b, ease.Clamp01(t))
}

// ParseHex reads a "#rrggbb" colour.
func ParseHex(s string) (r, g, b int, ok bool) {
	if len(s) != 7 || s[0] != '#' {
		return 0, 0, 0, false
	}
	component := func(i int) (int, bool) {
		v, err := strconv.ParseInt(s[i:i+2], 16, 32)
		return int(v), err == nil
	}
	var okR, okG, okB bool
	if r, okR = component(1); !okR {
		return 0, 0, 0, false
	}
	if g, okG = component(3); !okG {
		return 0, 0, 0, false
	}
	if b, okB = component(5); !okB {
		return 0, 0, 0, false
	}
	return r, g, b, true
}

func formatHex(r, g, b int) string {
	clamp := func(v int) int {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return v
	}
	hex := func(v int) string {
		s := strconv.FormatInt(int64(clamp(v)), 16)
		if len(s) == 1 {
			return "0" + s
		}
		return s
	}
	return "#" + hex(r) + hex(g) + hex(b)
}

// hex parses a palette's "#rrggbb" literal once, at package init, into a
// packed canvas.Color so the render loop compares integers. A typo comes out
// unset and draws in the terminal's own colour.
func hex(s string) canvas.Color { return canvas.ParseColor(s) }
