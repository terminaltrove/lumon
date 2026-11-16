package lumon

import "github.com/terminaltrove/lumon/internal/canvas"

// drawExit offers both ways to leave a board without losing the current name.
func (m *Model) drawExit(buf *canvas.Buffer) {
	// The title, then every line a blank row from the next: the two choices,
	// what cancels, and the keys.
	box, ok := m.modalBox(buf, 44, 10, "LEAVE THIS FILE?")
	if !ok {
		w, h := buf.Size()
		if w > 0 && h > 0 {
			buf.Text(0, h/2, clipTo("q quit · enter return to files", w), m.Theme.BinOn)
		}
		return
	}
	choices := []string{"Return to files", "Quit application"}
	for i, choice := range choices {
		row := pickRow{text: choice, attr: m.Theme.Footer}
		if (i == 1) == m.exitQuit {
			row.attr, row.picked = m.Theme.BinOn, true
		}
		m.drawPickRow(buf, box, box.Y+2+i*2, row)
	}
	m.modalCentre(buf, box, box.Y+box.H-4, "Any other key cancels", m.Theme.Footer)
	m.modalCentre(buf, box, box.Y+box.H-2, "arrows choose · enter confirm · q quit", m.Theme.Footer)
}
