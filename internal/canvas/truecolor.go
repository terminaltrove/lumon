package canvas

import "strconv"

// trueColorWrap is an attribute's sequences for a truecolour terminal, written
// without lipgloss. The browser build uses it for every attribute.
// TestTrueColorWrapMatchesLipgloss keeps it byte-identical to lipgloss's
// truecolour output.
func trueColorWrap(a Attr) wrap {
	body := make([]byte, 0, 40)
	part := func(p ...byte) {
		if len(body) > 0 {
			body = append(body, ';')
		}
		body = append(body, p...)
	}
	if a.Bold && !NoBold {
		part('1')
	}
	if a.Faint {
		part('2')
	}
	if a.FG.Set() {
		body = colourParams(body, a.FG, false, len(body) > 0)
	}
	if a.BG.Set() {
		body = colourParams(body, a.BG, true, len(body) > 0)
	}
	if len(body) == 0 {
		return wrap{}
	}
	return withOver(wrap{prefix: "\x1b[" + string(body) + "m", suffix: "\x1b[0m"}, a)
}

// overline is SGR 53, which lipgloss has no option for.
const overline = "\x1b[53m"

// withOver adds the overline to a wrap that styles anything, after the rest,
// so both builds write it the same way.
func withOver(w wrap, a Attr) wrap {
	if a.Over && w.prefix != "" {
		w.prefix += overline
	}
	return w
}

// colourParams appends the SGR parameters that select c, as termenv writes
// them for a truecolour terminal: 30-37/90-97 for the sixteen, 38;5;n for the
// rest of the palette and 38;2;r;g;b for a true colour (40s, 100s and 48 for a
// background).
func colourParams(body []byte, c Color, background, sep bool) []byte {
	if sep {
		body = append(body, ';')
	}
	if r, g, b, ok := c.Parts(); ok {
		if background {
			body = append(body, "48;2;"...)
		} else {
			body = append(body, "38;2;"...)
		}
		body = strconv.AppendUint(body, uint64(r), 10)
		body = append(body, ';')
		body = strconv.AppendUint(body, uint64(g), 10)
		body = append(body, ';')
		return strconv.AppendUint(body, uint64(b), 10)
	}
	n := int(c & 0xFF)
	switch {
	case n < 8 && background:
		return strconv.AppendInt(body, int64(40+n), 10)
	case n < 8:
		return strconv.AppendInt(body, int64(30+n), 10)
	case n < 16 && background:
		return strconv.AppendInt(body, int64(100+n-8), 10)
	case n < 16:
		return strconv.AppendInt(body, int64(90+n-8), 10)
	case background:
		body = append(body, "48;5;"...)
	default:
		body = append(body, "38;5;"...)
	}
	return strconv.AppendInt(body, int64(n), 10)
}
