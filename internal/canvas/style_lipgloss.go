package canvas

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The terminal and remote builds take every escape sequence from lipgloss, so a
// client's colour profile decides what it is sent. See style_js.go for the
// browser build.

type termRenderer = lipgloss.Renderer

// NewStyler returns a Styler for one terminal. A nil renderer means lipgloss's
// default, which is what a program with a terminal of its own wants.
func NewStyler(r *lipgloss.Renderer) *Styler {
	return &Styler{renderer: r, cache: map[Attr]wrap{}}
}

func (s *Styler) newStyle() lipgloss.Style {
	if s == nil || s.renderer == nil {
		return lipgloss.NewStyle()
	}
	return s.renderer.NewStyle()
}

func (s *Styler) build(a Attr) wrap { return buildWrap(a, s.newStyle()) }

// buildDefault renders an attribute for the process's own terminal.
func buildDefault(a Attr) wrap { return buildWrap(a, lipgloss.NewStyle()) }

// Lipgloss is the colour as lipgloss wants it.
func (c Color) Lipgloss() lipgloss.Color { return lipgloss.Color(c.String()) }

// buildWrap renders an attribute once to find the sequences it puts either side
// of its content.
func buildWrap(a Attr, style lipgloss.Style) wrap {
	if a.FG.Set() {
		style = style.Foreground(a.FG.Lipgloss())
	}
	if a.BG.Set() {
		style = style.Background(a.BG.Lipgloss())
	}
	if a.Bold && !NoBold {
		style = style.Bold(true)
	}
	if a.Faint {
		style = style.Faint(true)
	}

	rendered := style.Render(sentinel)
	if i := strings.Index(rendered, sentinel); i >= 0 {
		return withOver(wrap{prefix: rendered[:i], suffix: rendered[i+len(sentinel):]}, a)
	}
	// No sentinel means the profile stripped all styling.
	return wrap{}
}
