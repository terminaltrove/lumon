package canvas

import (
	"strings"
	"sync"
)

// The emitter moves the terminal from one attribute to the next without a
// reset in between. A grid row is about twenty short digit runs separated by
// blanks, and remotely the frame is the traffic, so a prefix and reset per run
// would be most of a session's bytes.

// carries reports whether a cell can be drawn in the state already set: when
// it asks for that state, or when it is a space with the same background. A
// space shows only its background and an overline; Bold and Faint have no
// stroke to change. If Attr ever gains another field a space can show
// (reverse, underline), revisit this and TestAttrCarriesNothingASpaceCouldShow.
func carries(cur, a Attr, r rune) bool {
	return a == cur || (r == ' ' && a.BG == cur.BG && a.Over == cur.Over)
}

// sgrRun writes several SGR parameters as one sequence, and nothing if every
// part is empty. On a profile that strips styling every part is empty, and a
// bare CSI m is SGR 0, a reset sent to a terminal that asked for no escapes.
type sgrRun struct{ open bool }

func (r *sgrRun) part(sb *strings.Builder, body string) {
	if body == "" {
		return
	}
	if !r.open {
		sb.WriteString("\x1b[")
		r.open = true
	} else {
		sb.WriteByte(';')
	}
	sb.WriteString(body)
}

func (r *sgrRun) close(sb *strings.Builder) {
	if r.open {
		sb.WriteByte('m')
	}
}

// splitBody turns a rendered prefix, "\x1b[38;2;0;255;255m", into the parameter
// body lipgloss put in it, "38;2;0;255;255". ok is false for anything that is
// not exactly one CSI ... m, so an unexpected lipgloss output falls back to
// reset-and-set.
func splitBody(prefix string) (string, bool) {
	if !strings.HasPrefix(prefix, "\x1b[") || !strings.HasSuffix(prefix, "m") {
		return "", false
	}
	body := prefix[2 : len(prefix)-1]
	if body == "" || strings.ContainsAny(body, "\x1b[m") {
		return "", false
	}
	return body, true
}

// seqs are the component sequences for one terminal: what sets a foreground,
// a background or a weight, and what resets to plain. They are keyed by Color,
// not Attr: blended effects create new colours per cell per frame, and keying
// by Attr would square the cache's growth.
type seqs struct {
	mu    sync.RWMutex
	fg    map[Color]string
	bg    map[Color]string
	bold  string
	faint string
	reset string
	ready bool
}

// pkgSeqs backs a nil Styler, as wrapCache does for its wraps.
var pkgSeqs seqs

func (s *Styler) seqs() *seqs {
	if s == nil {
		return &pkgSeqs
	}
	return &s.seq
}

// init builds the parts that do not depend on a colour, once. It is lazy
// because NoBold is set before drawing and the profile is unknown until a
// renderer exists.
func (t *seqs) init(s *Styler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ready {
		return
	}
	t.fg, t.bg = map[Color]string{}, map[Color]string{}

	// Probe with a colour: under NoBold a bold probe comes back empty and the
	// reset would be lost.
	probe := Attr{FG: RGB(0, 255, 255)}
	t.reset = s.build(probe).suffix

	if body, ok := splitBody(s.build(Attr{Bold: true}).prefix); ok {
		t.bold = body
	}
	if body, ok := splitBody(s.build(Attr{Faint: true}).prefix); ok {
		t.faint = body
	}
	t.ready = true
}

// colourBody is the parameter body that sets one colour, or "" if this terminal
// strips it.
func (t *seqs) colourBody(s *Styler, c Color, background bool) string {
	t.mu.RLock()
	m := t.fg
	if background {
		m = t.bg
	}
	body, ok := m[c]
	t.mu.RUnlock()
	if ok {
		return body
	}

	a := Attr{FG: c}
	if background {
		a = Attr{BG: c}
	}
	body, _ = splitBody(s.build(a).prefix)

	t.mu.Lock()
	if background {
		t.bg[c] = body
	} else {
		t.fg[c] = body
	}
	t.mu.Unlock()
	return body
}

// resetSeq is what returns this terminal to plain, or "" where it never left.
func (s *Styler) resetSeq() string {
	t := s.seqs()
	t.init(s)
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.reset
}

// writeDelta writes the shortest sequence that moves a terminal from one
// attribute to another. Anything turned off goes through a reset: lipgloss
// never returns SGR 22, 39 or 49, and writing them directly would bypass its
// colour-profile degradation on sixteen-colour and monochrome terminals.
func (s *Styler) writeDelta(sb *strings.Builder, from, to Attr) {
	t := s.seqs()
	t.init(s)

	off := (from.Bold && !to.Bold) || (from.Faint && !to.Faint) || (from.Over && !to.Over) ||
		(from.FG.Set() && !to.FG.Set()) || (from.BG.Set() && !to.BG.Set())

	if off || from == defaultAttr {
		// One cached write.
		if off {
			sb.WriteString(s.resetSeq())
		}
		sb.WriteString(s.wrap(to).prefix)
		return
	}

	var run sgrRun
	if to.Bold && !from.Bold && !NoBold {
		run.part(sb, t.bold)
	}
	if to.Faint && !from.Faint {
		run.part(sb, t.faint)
	}
	// Foreground then background, the order lipgloss composes them, so a
	// built sequence matches a rendered one byte for byte.
	if to.FG != from.FG {
		run.part(sb, t.colourBody(s, to.FG, false))
	}
	if to.BG != from.BG {
		run.part(sb, t.colourBody(s, to.BG, true))
	}
	run.close(sb)
	// After the colours, as withOver puts it, and only where they are drawn.
	if to.Over && !from.Over && t.reset != "" {
		sb.WriteString(overline)
	}
}
