package lumon

import (
	"fmt"
	"math"
	"strings"

	"github.com/terminaltrove/lumon/internal/canvas"
	"github.com/terminaltrove/lumon/internal/ease"
	"github.com/terminaltrove/lumon/internal/glyph"
	"github.com/terminaltrove/lumon/internal/theme"
)

// The two screens between the boot sequence and the grid: who you are, and
// which file you are going to work. A remote session has no board until it picks
// one, so the Model has a nil Room through both.

// NameLimit is the longest a refiner's name may be, so that four nameplates and
// the controls hint fit in the footer.
const NameLimit = 12

// NewSession builds a terminal that will go through the lobby. The name field
// starts empty and enter does nothing until something is typed, so scanners that
// press enter cannot get through, and an idle terminal is sent home (see
// idle.go). Whatever login name it came with is not offered: it is usually root or ubuntu.
func NewSession(rooms *Rooms, seed int64) *Model {
	m := newView(seed)
	m.rooms = rooms
	m.askName = true
	m.Screen = ScreenName
	return m
}

// NewLocalSession is the same, for somebody at their own terminal. It skips the
// name prompt and goes straight to the files.
func NewLocalSession(rooms *Rooms, seed int64, name string) *Model {
	m := newView(seed)
	m.rooms = rooms
	m.Name = tidyName(name)
	if m.Name == "" {
		m.Name = "REFINER"
	}
	m.Screen = ScreenLobby
	return m
}

// multiplayerAvailable follows the host's wiring: LAN is offered only by
// eligible native builds; askName and joined identify remote and LAN sessions.
// Browser sessions never share their floor, even when computer refiners play.
func (m *Model) multiplayerAvailable() bool {
	return !m.keepOpen && (m.lan != nil || m.askName || m.joined)
}

// NewScreensaver builds a terminal for the bouncing mark and nothing else. It has
// no files and takes no desk, and any key or click ends the program.
func NewScreensaver(seed int64) *Model {
	m := newView(seed)
	m.screensaverOnly = true
	m.StartScreensaver()
	return m
}

// tidyName puts a name into the house style: upper case, at most NameLimit
// long, only letters, digits, '.' and '-', with runs of spaces collapsed to one.
// It is also the refiner's scoreboard identity (refinerID), so it is the
// sanitiser for anything a client types as a name.
func tidyName(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToUpper(strings.TrimSpace(s)) {
		switch {
		case r == ' ':
			space = true
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-':
			if space && b.Len() > 0 && b.Len() < NameLimit {
				b.WriteRune(' ')
			}
			space = false
			if b.Len() < NameLimit {
				b.WriteRune(r)
			}
		}
	}
	// A space that took the last place. Left on, this would be a different
	// name from what tidying it again gives.
	return strings.TrimRight(b.String(), " ")
}

// handleNameKey takes a keystroke on the name screen. It reads msg.Runes, not
// String: Bubble Tea delivers the space bar as its own key, and a paste or a run
// of characters as one message.
func (m *Model) handleNameKey(msg KeyMsg) {
	switch msg.String() {
	case "enter":
		m.Name = tidyName(m.Name)
		if m.Name == "" {
			return // a nameplate has to say something
		}
		if !m.claimName() {
			return // somebody on the floor already has it; claimName says so
		}
		m.Screen = ScreenLobby
	case "backspace":
		if r := []rune(m.Name); len(r) > 0 {
			m.Name = string(r[:len(r)-1])
			m.noteName()
		}
	default:
		// An alt chord is a shortcut meant for something else, not a letter.
		if !msg.Alt {
			before := m.Name
			m.typeName(msg.Runes)
			if m.Name != before {
				m.noteName()
			}
		}
	}
}

// typeName adds what was typed to the name. It does not use tidyName, which
// trims, so a space typed after MARK survives until the S arrives; enter tidies
// any trailing space.
func (m *Model) typeName(typed []rune) {
	name := []rune(m.Name)
	for _, r := range strings.ToUpper(string(typed)) {
		if len(name) >= NameLimit {
			break
		}
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-':
			name = append(name, r)
		case r == ' ' && len(name) > 0 && name[len(name)-1] != ' ':
			name = append(name, r)
		}
	}
	m.Name = string(name)
}

// lobbyView is one of three ways of drawing the drawer. They are modes of one
// screen: a new screen needs wiring in draw, advance, the key handler and
// handleMouse, and a missed mouse arm falls through to the grid path, which
// dereferences the nil *Room. All three share the reel's selection, so enter
// means the same thing in each and switching keeps your place.
type lobbyView int

const (
	viewRolodex lobbyView = iota
	viewList
	viewGrid
	lobbyViews // how many there are
)

// WipeRequested reports that the refiner confirmed wiping this browser's saved
// progress. Only an embedded browser session can ask.
func (m *Model) WipeRequested() bool { return m.wipe }

// handleLobbyKey takes a keystroke on the lobby screen.
func (m *Model) handleLobbyKey(key string) {
	if m.rooms == nil {
		return
	}
	r := m.rolodex()

	// The next key answers the wipe question: y wipes, anything else cancels
	// and does nothing more.
	if m.wipeAsk {
		m.wipeAsk, m.notice = false, ""
		m.wipe = key == "y"
		return
	}

	switch key {
	case "up", "k":
		if m.lobbyView == viewGrid {
			m.gridMove(r, -m.gridCols())
			return
		}
		r.Turn(-1)
	case "down", "j":
		if m.lobbyView == viewGrid {
			m.gridMove(r, m.gridCols())
			return
		}
		r.Turn(1)
	case "left":
		// In the grid, sideways is the next tile; in the reel and the list it is
		// the next letter.
		if m.lobbyView == viewGrid {
			m.gridMove(r, -1)
			return
		}
		r.TurnToLetter(-1)
	case "right":
		if m.lobbyView == viewGrid {
			m.gridMove(r, 1)
			return
		}
		r.TurnToLetter(1)
	// Only shared floors have other players' files to navigate by occupancy.
	case "+", "=":
		if m.multiplayerAvailable() {
			m.seekBusier(r, 1)
		}
	case "-", "_":
		if m.multiplayerAvailable() {
			m.seekBusier(r, -1)
		}
	case "v":
		m.lobbyView = (m.lobbyView + 1) % lobbyViews
	case "n":
		m.tryJoin(m.rooms.Open())
	case "c":
		if m.allowBots {
			m.company = (m.company + 1) % MaxDesks
			m.notice = companyNotice(m.company)
		}
	case "enter", " ":
		m.takeCard()
	case "l":
		if m.lan != nil {
			m.openLAN()
		}
	case "s":
		m.openScores(ScreenLobby)
	case "t":
		m.Theme = theme.NextTheme(m.Theme)
	case "r":
		// Browser only: play the boot sequence again. It finishes back here,
		// since a session with no file goes to the files.
		if m.keepOpen {
			m.StartIntro()
		}
	case "x":
		// Browser only: wipe the progress this browser has saved. The host
		// clears it and reloads, so it starts again from the boot sequence.
		if m.keepOpen {
			m.wipeAsk, m.notice = true, "wipe all saved progress and start again? y / n"
		}
	}
	// q, through backOut, returns to the name screen.
}

// gridMove steps the selection by whole tiles. It sets a destination with GoTo:
// Turn adds momentum, which could carry a keypress two tiles.
func (m *Model) gridMove(r *Rolodex, by int) {
	r.GoTo(r.Want() + by)
}

// seekBusier sends the drawer up or down the occupancy ranking. It uses + and -
// because they mean "more" and "less" elsewhere (zoom, the 3D dolly).
func (m *Model) seekBusier(r *Rolodex, by int) {
	if r.SeekBusier(by) {
		m.notice = ""
		return
	}
	m.notice = "nobody is refining anything just now"
}

// tryJoin joins a file, or sets a notice saying why it could not.
func (m *Model) tryJoin(room *Room) {
	if room == nil {
		m.watchOffer = ""
		m.notice = "that file is no longer open"
		return
	}

	// Choosing a full file a second time accepts the offer to watch it. It
	// takes two presses so nobody ends up watching when they meant to work.
	file := room.CurrentFile()
	if m.watchOffer == file {
		m.watchOffer = ""
		if m.Watch(room) {
			m.notice = ""
			return
		}
		m.notice = fmt.Sprintf("%s has nobody watching it and no room to wait", file)
		return
	}

	if m.Join(room) {
		m.notice, m.watchOffer = "", ""
		return
	}

	m.watchOffer = file
	m.notice = fmt.Sprintf("%s is full, enter again to watch", file)
}

// topRefiner is the title over a finished file's leading refiner. The
// quarter's honour is separate; see quarter.go.
const topRefiner = "TOP REFINER"

// cursorBlink is how long, in seconds, the name-screen cursor spends lit, and
// then dark.
const cursorBlink = 0.5

// cursorOn reports whether the block cursor is showing this instant.
func (m *Model) cursorOn() bool {
	return int(m.sessionAge/cursorBlink)%2 == 0
}

// drawName asks who is sitting down.
func (m *Model) drawName(buf *canvas.Buffer) {
	w, h := buf.Size()
	if w < 30 || h < 8 {
		return
	}

	rows := []struct {
		text string
		attr canvas.Attr
	}{
		{"LUMON INDUSTRIES", m.Theme.Header},
		{"MACRODATA REFINEMENT", m.Theme.Footer},
		{"", m.Theme.Footer},
		{"PLEASE IDENTIFY YOURSELF", m.Theme.Boot},
	}

	top := h/2 - 4
	for i, row := range rows {
		buf.Text((w-len([]rune(row.text)))/2, top+i, row.text, row.attr)
	}

	// The field, with a block cursor on the end. The blink runs off the
	// session clock, since m.Intro is nil by the time this screen shows.
	field := m.Name
	if m.cursorOn() {
		field += "█"
	}
	box := NameLimit + 4
	x := (w - box) / 2
	buf.Box(x, top+6, box, 3, m.Theme.Chrome)
	buf.Text(x+2, top+7, field, m.Theme.Hovered)

	// Beneath it, a welcome back for a known name, or why a name is refused.
	if note := m.nameNote; note != "" {
		attr := m.Theme.Footer
		if m.nameRefused {
			attr = m.Theme.BadLive
		}
		buf.Text((w-len([]rune(note)))/2, top+9, note, attr)
	}

	const hint = "enter to continue"
	buf.Text((w-len(hint))/2, top+10, hint, m.Theme.Footer)
}

// lobbyTop is the first row of the drawer (the same as cardTop), and
// lobbyBottom how many rows are kept clear beneath it: a blank, the notice, the
// hint, and one more.
const (
	lobbyTop    = 2
	lobbyBottom = 4

	// lobbyHeadRows is the list's column headings and the blank under them.
	lobbyHeadRows = 2
)

// lobbyWindow is which slice of the list is on screen, and how much of it. The
// renderer and the mouse handler both call it, so a click resolves to the row
// that is drawn. The cursor is passed in: it is the rolodex's index, which all
// three views share.
func (m *Model) lobbyWindow(height, n, at int) (first, visible int) {
	visible = height - lobbyTop - lobbyBottom - lobbyHeadRows
	if visible > n {
		visible = n
	}
	if visible < 0 {
		visible = 0
	}
	if at >= visible && visible > 0 {
		first = at - visible + 1
	}
	return first, visible
}

// handleLobbyMouse takes a click on the drawer. A press selects, and a press on
// the current selection joins it, so a stray click in the list or grid cannot
// join a file. In the rolodex the only card under the pointer is the selection,
// so one press joins. The wheel is routed separately.
func (m *Model) handleLobbyMouse(x, y int, press bool) {
	if m.rooms == nil || !press {
		return
	}
	r := m.rolodex()

	switch m.lobbyView {
	case viewList:
		if i, ok := m.lobbyRowAt(y, r); ok {
			m.takeOrSelect(r, i)
		}
	case viewGrid:
		if i, ok := m.lobbyTileAt(x, y, r); ok {
			m.takeOrSelect(r, i)
		}
	default:
		box, ok := cardFits(m.Layout.Width, m.Layout.Height)
		if !ok {
			return
		}
		// The band reaches up by the lift, since a presented card stands that
		// far out of the drawer.
		if y >= box.hinge-box.spec.up-1-cardLiftRows && y <= box.hinge+box.spec.down+1 {
			m.takeCard()
		}
	}
}

// takeOrSelect moves to a file, or joins it if it is where you already were.
func (m *Model) takeOrSelect(r *Rolodex, i int) {
	if i == r.Want() {
		m.takeCard()
		return
	}
	r.GoTo(i)
}

// lobbyRowAt is which file the pointer is over in the list view. It uses
// lobbyWindow, as the renderer does, so clicks land on the drawn row.
func (m *Model) lobbyRowAt(y int, r *Rolodex) (int, bool) {
	first, visible := m.lobbyWindow(m.Layout.Height, r.Len(), r.Want())
	top := lobbyListTop(m.Layout.Height, visible) + lobbyHeadRows
	i := first + (y - top)
	if y < top || i < first || i >= first+visible || i >= r.Len() {
		return 0, false
	}
	return i, true
}

// lobbyTileAt is which file the pointer is over in the grid view.
func (m *Model) lobbyTileAt(x, y int, r *Rolodex) (int, bool) {
	w, h := m.Layout.Width, m.Layout.Height
	cols := m.gridCols()
	rows := lobbyContentRows(h) / lobbyTileH
	if rows < 1 {
		return 0, false
	}

	top := lobbyGridTop(h, r.Len(), cols)
	left := (w - cols*lobbyTileW) / 2
	col := (x - left) / lobbyTileW
	row := (y - top) / lobbyTileH
	if x < left || col >= cols || y < top || row >= rows {
		return 0, false
	}

	perPage := cols * rows
	i := (r.Want()/perPage)*perPage + row*cols + col
	if i >= r.Len() {
		return 0, false
	}
	return i, true
}

// rolodexTurn moves the reel, for the wheel.
func (m *Model) rolodexTurn(by int) {
	if m.rooms == nil {
		return
	}
	m.rolodex().Turn(by)
}

// takeCard opens the file on the card the reel is showing, if nobody has, and
// sits down at it. It goes through tryJoin, so a full file is still refused with
// a notice.
func (m *Model) takeCard() {
	r := m.rolodex()
	// On its way somewhere, the card is the one it was going to.
	r.Arrive()
	e, ok := r.Selected()
	if !ok {
		m.tryJoin(m.rooms.Open())
		return
	}
	m.tryJoin(m.rooms.OpenNamed(e.Name))
}

// fadeFilesUp has the drawer fade up out of the dark again, riffling onto the
// file last opened, as it first did, for a refiner coming back to the files
// from the boot sequence or a file.
func (m *Model) fadeFilesUp() {
	if m.files != nil {
		m.files.Fade = NewFade()
		m.files.Riffle(m.filesHome(m.files.Files()))
	}
}

// filesHome is the card the drawer comes up on: the file this refiner last
// opened, or the first.
func (m *Model) filesHome(files []Entry) int {
	for i, e := range files {
		if e.Name == m.lastFile {
			return i
		}
	}
	return 0
}

// rolodex is this session's drawer, built on first use and kept in step with the
// registry after that.
func (m *Model) rolodex() *Rolodex {
	files := m.rooms.Directory()
	if m.files == nil {
		m.files = NewRolodex(files, 0)
		m.files.Riffle(m.filesHome(files))
		return m.files
	}
	m.files.Sync(files)
	return m.files
}

// drawLobby is the drawer of macrodata files: the card being read, the rest of
// the drawer edge-on either side, and the tab it is filed behind. See card.go
// for the drawing and rolodex.go for the turning.
func (m *Model) drawLobby(buf *canvas.Buffer) {
	w, h := buf.Size()
	if m.rooms == nil {
		return
	}
	r := m.rolodex()

	if m.lobbyView != viewRolodex {
		if r.Len() == 0 {
			buf.Text(2, lobbyTop, "no files", m.Theme.Footer)
		} else if m.lobbyView == viewList {
			m.drawLobbyNotice(buf, m.drawLobbyList(buf, r))
		} else {
			m.drawLobbyNotice(buf, m.drawLobbyGrid(buf, r))
		}
		m.drawLobbyHint(buf)
		return
	}

	box, ok := cardFits(w, h)
	if !ok {
		// A card does not shrink. Below its size, describe the selection in
		// words.
		if e, on := r.Selected(); on {
			buf.Text(2, lobbyTop, e.Name, m.Theme.Hovered)
			buf.Text(2, lobbyTop+1, cardStatus(e, w-4), m.Theme.Footer)
		}
		m.drawLobbyHint(buf)
		return
	}

	if r.Len() == 0 {
		buf.Text(2, lobbyTop, "no files", m.Theme.Footer)
		m.drawLobbyHint(buf)
		return
	}

	// The drawer fades up from the backdrop after the boot sequence.
	m.drawLobbyNotice(buf, box.hinge+box.spec.down+1)

	lit := m.Theme.Primary
	ink := canvas.Attr{FG: theme.BlendColors(m.Theme.Backdrop, lit, r.Level())}
	dim := canvas.Attr{FG: theme.BlendColors(m.Theme.Backdrop, m.Theme.Dim, r.Level())}

	// The stack's second tone, a shade further back, gives the bars depth.
	back := canvas.Attr{FG: theme.BlendColors(m.Theme.Backdrop, m.Theme.Dim, r.Level()*0.55)}
	drawCardCombs(buf, box, dim, back)
	if e, scale, on := r.Showing(); on {
		bf, af := r.Behind(cardStackDepth)
		drawCardStack(buf, box, cardInitial(e.Name), bf, af, back)

		// The card lifts past the combs and tabs, which stay put. A card draws
		// only its edges, so it is cleared at the raised position to hide the
		// tabs behind it.
		if by := int(math.Round(r.Lift() * cardLiftRows)); by > 0 {
			box.hinge -= by
			clearCard(buf, box, m.Theme.Page)
		}

		// The file's status goes in the card's lower panel.
		drawCard(buf, box, scale, e, cardStatus(e, box.spec.w-6), ink)
	}

	m.drawLobbyHint(buf)
}

// drawLobbyNotice says what just happened, centred on the row midway between the
// last row of the drawer's contents and the controls.
func (m *Model) drawLobbyNotice(buf *canvas.Buffer, last int) {
	if m.notice == "" {
		return
	}
	w, h := buf.Size()
	line := clipTo(m.notice, w-2)
	buf.Text((w-len([]rune(line)))/2, (last+h-2+1)/2, line, m.Theme.BadLive)
}

// drawLobbyHint is the controls line along the bottom of the drawer, centred
// like the card, in the board's hint colour.
func (m *Model) drawLobbyHint(buf *canvas.Buffer) {
	w, h := buf.Size()
	line := m.lobbyHint(w - 4)
	buf.Text((w-len([]rune(line)))/2, h-2, line, m.Theme.Hint)
}

// completeGlyphHeights are the heights the "100%" may be drawn at, in rows,
// tallest first, so a small terminal can shrink it. Seven is the smallest at
// which the percent's rings stay open; smaller sizes are a fallback for
// terminals that cannot fit seven.
var completeGlyphHeights = []float64{7, 6, 5}

// completeSolidAt is the coverage a cell needs to be drawn, solid, in the
// "100%". The usual shading ramp would grey the edges and put a dot in each
// zero's counter. At 0.35 the zeros fill in and go square; at 0.65 the curves
// break up.
const completeSolidAt = 0.5

// drawComplete is the notice a finished file is held on for its HoldSeconds,
// as a modal over the board. The board behind it is still, because advanceFile
// puts every cluster to sleep when the file finishes. The "100%" is set large
// with the same rasteriser as the board's enlarged digits.
func (m *Model) drawComplete(buf *canvas.Buffer) {
	rank := m.Room.FileStandings()
	credited := len(rank) > 0 && rank[0].Refined > 0

	// The room's share, on one line, which is the widest thing in the box.
	var share string
	if credited {
		var parts []string
		for _, st := range rank {
			parts = append(parts, fmt.Sprintf("%s %d", st.Name, st.Refined))
		}
		share = strings.Join(parts, " · ")
	}

	line := fileLine(m.FileName)

	widest := len(topRefiner)
	for _, s := range []string{m.FileName, share, "THE WORK IS MYSTERIOUS AND IMPORTANT", line} {
		if n := len([]rune(s)); n > widest {
			widest = n
		}
	}

	// Two borders, the file name, a blank, the hundred, its caption, a blank,
	// three lines of who did it, a blank and the countdown; then, each with a
	// blank above, the file's own line and the files completed.
	extra, said := 0, 0
	if line != "" {
		said = 2
	}
	extra += said
	if len(m.Room.History) > 0 {
		extra += 2
	}

	// The tallest "100%" this terminal can hold, in both height and width.
	// Everything below sits at fixed offsets from the box's top, so a number
	// too tall would push the countdown through the border, and
	// drawCompleteNumber draws nothing if the number is too wide.
	screenW, screenH := buf.Size()
	boxW := mini(widest+8, screenW-2)

	glyphH := completeGlyphHeights[len(completeGlyphHeights)-1]
	for _, h := range completeGlyphHeights {
		mark := len(completeNumber)*int(math.Ceil(h*glyph.CellAspect)) + len(completeNumber) - 1
		if int(h)+11+extra <= screenH-2 && mark <= boxW-2 {
			glyphH = h
			break
		}
	}

	rows := int(glyphH) + 11 + extra

	// The box grows open in height (per viewer, a physics.Scalar). Growing the
	// width too would slide every centred line outward as it opened.
	open := 1.0
	if m.noticeOpen != nil {
		open = ease.Clamp01(m.noticeOpen.Value())
	}
	shown := int(math.Round(float64(rows) * open))
	if shown > rows {
		shown = rows
	}

	// Decide whether the box fits from its finished height, not the opening
	// one, or every notice would start by flashing the fallback line.
	w, h := buf.Size()
	if mini(widest+8, w-2) < modalMinW || mini(rows, h-2) < modalMinH {
		buf.Text(0, h/2, clipTo("100% COMPLETE", w), m.Theme.Hovered)
		return
	}
	if shown < 1 {
		return
	}

	box, ok := m.modalBox(buf, widest+8, shown, "")
	if !ok {
		return // still opening, and too short to draw a frame around yet
	}

	// Nothing inside until the box is its full height, since every line sits
	// at a fixed offset from the top or bottom and would overlap.
	if shown < rows {
		return
	}

	y := box.Y + 1
	m.modalCentre(buf, box, y, m.FileName, m.Theme.Header)

	y += 2
	m.drawCompleteNumber(buf, box, y, glyphH)
	y += int(glyphH)
	// Just the word, since the number above has its own percent. The fallback
	// line keeps "100% COMPLETE" because it has no number over it.
	m.modalCentre(buf, box, y, "COMPLETE", m.Theme.Hovered)

	// Who did it. The tally is read while the file is held, before rotate
	// folds it away.
	y += 2
	if credited {
		m.modalCentre(buf, box, y, topRefiner, m.Theme.Footer)
		m.modalCentre(buf, box, y+1, rank[0].Name, m.Theme.BinOn)
		m.modalCentre(buf, box, y+2, share, m.Theme.BinOff)
	} else {
		// Nobody credited (a room somebody left): show the house line.
		m.modalCentre(buf, box, y+1, "THE WORK IS MYSTERIOUS AND IMPORTANT", m.Theme.Boot)
	}

	if line != "" {
		m.modalCentre(buf, box, y+4, line, m.Theme.Hovered)
	}
	y += said

	if done := m.Room.History; len(done) > 0 && y+5 < box.Y+box.H-2 {
		m.modalCentre(buf, box, y+4, fmt.Sprintf("FILES COMPLETED: %d", len(done)), m.Theme.Footer)

		// The most recent few, newest last, as much as the width holds.
		recent := done
		if len(recent) > 5 {
			recent = recent[len(recent)-5:]
		}
		m.modalCentre(buf, box, y+5, strings.Join(recent, " · "), m.Theme.BinOff)
	}

	// A countdown to the next file.
	left := int(m.Room.HoldSeconds()-m.Room.Held()) + 1
	if left < 1 {
		left = 1
	}
	m.modalCentre(buf, box, box.Y+box.H-2, fmt.Sprintf("next file in %d", left), m.Theme.Footer)
}

// completeNumber is what the notice sets large.
const completeNumber = "100%"

// drawCompleteNumber sets the hundred across the modal, in the board's own
// digits.
func (m *Model) drawCompleteNumber(buf *canvas.Buffer, box modal, y int, high float64) {
	// Measured first: glyphs rasterise to different widths (a 1 is narrower
	// than a 0), so a fixed advance would leave the number off centre.
	covs := make([]*glyph.Coverage, 0, len(completeNumber))
	wide := 0
	for _, r := range completeNumber {
		// By rune, so the percent rasterises like the digits.
		cov := glyph.RasterizeRuneFor(r, high*glyph.CellAspect, high, 0, true)
		if cov == nil {
			return
		}
		covs = append(covs, cov)
		wide += cov.W + 1
	}
	wide--

	x := box.X + (box.W-wide)/2
	if x < box.X+1 {
		return // it would run off its own box; the caption still says it
	}
	for _, cov := range covs {
		// Solid, and never the ramp. See completeSolidAt.
		for row := 0; row < cov.H; row++ {
			for col := 0; col < cov.W; col++ {
				if cov.At(col, row) >= completeSolidAt {
					buf.Set(x+col, y+row, glyph.GlyphSolid, m.Theme.Hovered)
				}
			}
		}
		x += cov.W + 1
	}
}

// lobbyContentRows is how many rows the drawer's contents may take.
func lobbyContentRows(height int) int { return height - lobbyTop - lobbyBottom }

// lobbyCentred is the top row of something rows tall, centred in that space. A
// block that fills it, or more, starts at the top as usual.
func lobbyCentred(height, rows int) int {
	return lobbyTop + maxi((lobbyContentRows(height)-rows)/2, 0)
}

// lobbyListTop is the row of the list's headings. The renderer and the mouse
// handler both call it, so a click resolves to the row that is drawn.
func lobbyListTop(height, visible int) int {
	return lobbyCentred(height, lobbyHeadRows+visible)
}

// drawLobbyList is the drawer as one row per file, for scanning. Its columns
// follow drawScores: fixed numeric columns, and the name takes what is left. It
// returns the last row it used.
func (m *Model) drawLobbyList(buf *canvas.Buffer, r *Rolodex) int {
	w, h := buf.Size()
	files := r.Files()
	at := r.Want()

	const (
		colPercent = 10
		colDesks   = 8
	)
	x := 4
	nameW := w - x - colPercent - colDesks - 4
	whoW := 0
	if nameW > lobbyListNameW+8 {
		whoW = mini(nameW-lobbyListNameW, lobbyListWhoW)
		nameW = lobbyListNameW
	}
	if nameW < 8 {
		nameW = 8
	}
	// With room to spare the block sits in the middle, or it hugs the margin.
	x = maxi(x, (w-nameW-colPercent-colDesks-whoW)/2)

	head := func(y int, a canvas.Attr) {
		cx := x
		buf.Text(cx, y, clipTo("FILE", nameW), a)
		cx += nameW
		buf.Text(cx, y, "COMPLETE", a)
		cx += colPercent
		buf.Text(cx, y, "DESKS", a)
		cx += colDesks
		if whoW > 0 {
			buf.Text(cx, y, clipTo("REFINERS", whoW), a)
		}
	}
	first, visible := m.lobbyWindow(h, len(files), at)
	top := lobbyListTop(h, visible)
	head(top, m.Theme.BinOff)
	for i := 0; i < visible; i++ {
		e := files[first+i]
		y := top + lobbyHeadRows + i

		attr := m.Theme.Footer
		switch {
		case first+i == at:
			attr = m.Theme.Hovered
		case e.Full():
			attr = m.Theme.BinOff
		case e.Open:
			attr = m.Theme.Header
		}
		if first+i == at {
			buf.Text(x-2, y, "▸", attr)
		}

		cx := x
		buf.Text(cx, y, clipTo(e.Name, nameW), attr)
		cx += nameW
		if e.Open {
			buf.Text(cx, y, itoa(e.Percent)+"%", attr)
		} else {
			buf.Text(cx, y, "-", attr)
		}
		cx += colPercent
		buf.Text(cx, y, itoa(e.Occupied)+"/"+itoa(e.Desks), attr)
		cx += colDesks
		if whoW > 0 {
			buf.Text(cx, y, clipTo(refinerNames(e, whoW), whoW), attr)
		}
	}
	return top + lobbyHeadRows + visible - 1
}

// lobbyListNameW is how much of a row the file name may take before the
// refiners get the rest. The longest name in the pool is 14.
const lobbyListNameW = 16

// lobbyListWhoW is the most the refiners get. It holds three ordinary names, and
// refinerNames goes to initials for more. A fixed width keeps the list from
// sliding as people sit down.
const lobbyListWhoW = 28

// refinerNames is who is on a file, as one string that fits.
func refinerNames(e Entry, width int) string {
	if len(e.Refiners) == 0 {
		return ""
	}
	for _, form := range []func(Occupant) string{
		func(o Occupant) string { return o.Name },
		func(o Occupant) string { return initials(o.Name) },
	} {
		names := make([]string, 0, len(e.Refiners))
		for _, o := range e.Refiners {
			names = append(names, form(o))
		}
		if s := strings.Join(names, ", "); len([]rune(s)) <= width {
			return s
		}
	}
	return itoa(e.Occupied) + "/" + itoa(e.Desks)
}

// The grid's tile. Three rows of content plus a blank between rows, and wide
// enough for the longest file name with room either side.
const (
	lobbyTileW = 22
	lobbyTileH = 4
)

// gridCols is how many tiles fit across, at least one. The key handler uses it
// too, since ↑↓ in the grid moves a whole row.
func (m *Model) gridCols() int {
	cols := (m.Layout.Width - 4) / lobbyTileW
	if cols < 1 {
		return 1
	}
	return cols
}

// lobbyGridRows is how many rows of tiles are on screen: all of them if they fit,
// or a full page. It counts every file, not this page's, so turning the page
// does not move the grid.
func lobbyGridRows(height, files, cols int) int {
	return mini(lobbyContentRows(height)/lobbyTileH, (files+cols-1)/cols)
}

// lobbyGridTop is the grid's first row. The renderer and the mouse handler both
// call it, so a click lands on the tile that is drawn. The last tile's blank row
// is not counted as part of the block.
func lobbyGridTop(height, files, cols int) int {
	return lobbyCentred(height, lobbyGridRows(height, files, cols)*lobbyTileH-1)
}

// drawLobbyGrid is the floor at a glance: every file in the directory as a
// tile, with how far through it is and who is sitting at it. Unopened files are
// included so they can be chosen. It returns the last row it used.
func (m *Model) drawLobbyGrid(buf *canvas.Buffer, r *Rolodex) int {
	w, h := buf.Size()
	files := r.Files()
	at := r.Want()

	cols := m.gridCols()
	rows := lobbyContentRows(h) / lobbyTileH
	if rows < 1 {
		return lobbyTop
	}

	// The window is by whole rows, so a tile never straddles the top edge.
	perPage := cols * rows
	page := 0
	if perPage > 0 {
		page = (at / perPage) * perPage
	}
	left := (w - cols*lobbyTileW) / 2
	top := lobbyGridTop(h, len(files), cols)

	for i := 0; i < perPage && page+i < len(files); i++ {
		e := files[page+i]
		x := left + (i%cols)*lobbyTileW
		y := top + (i/cols)*lobbyTileH
		m.drawLobbyTile(buf, x, y, e, page+i == at)
	}
	return top + lobbyGridRows(h, len(files), cols)*lobbyTileH - 2
}

// drawLobbyTile is one file in the grid.
func (m *Model) drawLobbyTile(buf *canvas.Buffer, x, y int, e Entry, on bool) {
	const inner = lobbyTileW - 3

	name := m.Theme.Footer
	switch {
	case on:
		name = m.Theme.Hovered
	case e.Full():
		name = m.Theme.BinOff
	case e.Open:
		name = m.Theme.Header
	}
	if on {
		buf.Text(x, y, "▸", name)
	}
	buf.Text(x+2, y, clipTo(e.Name, inner), name)

	if !e.Open {
		buf.Text(x+2, y+1, "UNOPENED", m.Theme.BinOff)
		return
	}

	// The completion, as a number and as a bar.
	pct := itoa(e.Percent) + "%"
	buf.Text(x+2, y+1, pct, m.Theme.Footer)
	barX := x + 2 + len([]rune(pct)) + 1
	barW := inner - len([]rune(pct)) - 1
	if barW > 0 {
		lit := barW * e.Percent / 100
		buf.HLine(barX, y+1, barW, '┄', m.Theme.BinOff)
		if lit > 0 {
			buf.HLine(barX, y+1, lit, '━', m.Theme.Header)
		}
	}

	// Who is on it, each in their seat's colour, as on their nameplates.
	if len(e.Refiners) == 0 {
		buf.Text(x+2, y+2, "empty", m.Theme.BinOff)
		return
	}
	cx := x + 2
	for i, o := range e.Refiners {
		s := initials(o.Name)
		if i > 0 {
			s = " " + s
		}
		if cx-x-2+len([]rune(s)) > inner {
			break
		}
		buf.Text(cx, y+2, s, m.Theme.On(m.Theme.DeskColour(o.Seat), false))
		cx += len([]rune(s))
	}
}
