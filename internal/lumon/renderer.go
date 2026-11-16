package lumon

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// UseRenderer binds this terminal to a client's own lipgloss renderer, so its
// colours are chosen for the client's terminal.
func (m *Model) UseRenderer(r *lipgloss.Renderer) {
	m.styler = canvas.NewStyler(r)
}
