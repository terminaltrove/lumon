package lumon

import (
	"strings"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// Escape does not back out of anything. It shows a notice reporting you to
// your supervisor, the same notice as the ESCAPE entry in the drawer. Backing
// out is q.
//
// It is bound with an `if`, never a `case`: keys_test.go walks the case
// clauses of the key handlers and requires every key it finds to appear in
// Bindings(), so a `case "esc":` would put escape into the controls overlay.
// The corridors' effect keys are hidden the same way.

// escapeNotice is shared by the key and the drawer entry so the two cannot
// drift apart.
const escapeNotice = "Error: This is not supported on your MDR Terminal, " +
	"this issue has been reported to your supervisor."

// escapeTitle matches the drawer entry's name.
const escapeTitle = "ESCAPE"

// escapeDismiss must not mention the escape key.
const escapeDismiss = "any key to go back to work"

// wrapNotice folds text to width on word boundaries, never breaking a word,
// and starts a new line at each newline.
func wrapNotice(text string, width int) []string {
	if width < 1 {
		return nil
	}
	var out []string
	for _, para := range strings.Split(text, "\n") {
		cur := ""
		for _, word := range strings.Fields(para) {
			switch {
			case cur == "":
				cur = word
			case len([]rune(cur))+1+len([]rune(word)) <= width:
				cur += " " + word
			default:
				out = append(out, cur)
				cur = word
			}
		}
		if cur != "" {
			out = append(out, cur)
		}
	}
	return out
}

// drawEscape paints the notice as an overlay, so it can open from anywhere,
// including the name prompt and the lobby.
func (m *Model) drawEscape(buf *canvas.Buffer) {
	m.drawNotice(buf, escapeTitle, escapeNotice, "Error: not supported", m.Theme.On(m.Theme.Urgent, true))
}

// drawNotice paints text, in ink, in a titled box with the dismiss line, or only short
// when no legible box fits.
func (m *Model) drawNotice(buf *canvas.Buffer, title, text, short string, ink canvas.Attr) {
	w, h := buf.Size()

	// Wrap to a readable width, not the terminal's.
	want := w - 12
	if want > escapeWrap {
		want = escapeWrap
	}
	lines := wrapNotice(text, want)
	if len(lines) == 0 {
		return
	}

	widest := len([]rune(escapeDismiss))
	for _, s := range lines {
		if n := len([]rune(s)); n > widest {
			widest = n
		}
	}

	// Six rows of chrome: two borders, the gap under the title, the gap above
	// the dismiss line, and the dismiss line. A one-line text is padded to
	// modalMinH; a terminal too short for that gets the bare line below.
	box, ok := m.modalBox(buf, widest+6, max(len(lines)+6, modalMinH), title)
	if !ok {
		if w >= 12 && h >= 1 {
			buf.Text(0, h/2, clipTo(short, w), ink)
		}
		return
	}

	y := box.Y + 2
	for _, s := range lines {
		if y >= box.Y+box.H-2 {
			break
		}
		m.modalCentre(buf, box, y, s, ink)
		y++
	}
	m.modalCentre(buf, box, box.Y+box.H-2, escapeDismiss, m.Theme.Footer)
}

// escapeWrap is how wide the notice is allowed to set, in columns.
const escapeWrap = 56
