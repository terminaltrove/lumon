package lumon

import (
	"strings"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// The diversions: what this terminal does when nobody is refining. They open
// from a drawer reached by typing KIER on the grid, so they cost no bindings and
// stay out of the controls. A single rune reaches the key handler's default
// branch, not a case clause, so keys_test.go's scan for undocumented keys does
// not see them.

// theWord is what opens the drawer.
const theWord = "KIER"

// theWords are every sequence that opens something. 3D opens the third
// dimension directly. A lone 3 still picks bin three, because the first letter
// of a word is never swallowed (see noteKey).
var theWords = []string{theWord, "3D"}

// Panel is the region between the two rules, the grid's own area, where a
// diversion runs. The chrome around it stays live.
type Panel struct{ X, Y, W, H int }

// Contains reports whether a screen position is inside.
func (p Panel) Contains(x, y int) bool {
	return x >= p.X && y >= p.Y && x < p.X+p.W && y < p.Y+p.H
}

// Play is the part of the panel a diversion may draw in: everything above its
// hint line.
func (p Panel) Play() Panel {
	if p.H > 1 {
		p.H--
	}
	return p
}

// gridPanel is where the grid is drawn, and so where a diversion goes: the full
// width and the board's rows. It uses the board rectangle, not ViewportH times
// a row pitch, because the pitch varies with the spacing setting.
func (m *Model) gridPanel() Panel {
	l := m.Layout
	return Panel{X: 0, Y: l.BoardY, W: l.Width, H: l.BoardH}
}

// Diversion is one entry in the diversions drawer.
type Diversion struct {
	Name string
	What string

	// Mode marks an entry that is a way of viewing the board, still
	// refinable, and not a diversion.
	Mode bool

	// Refuses marks an entry that is refused. It answers with the same notice
	// as the escape key, from the same string. See escape.go.
	Refuses bool
}

// diversions are the drawer's entries, in display order. Look them up by name,
// not index; the order changes.
var diversions = []Diversion{
	{Name: "SCATTER", What: "the numbers let go of the grid"},
	{Name: "DESCENT", What: "they fall, and can be picked up and thrown"},
	{Name: "SAND", What: "they settle in heaps, like grain"},
	{Name: "MATRIX", What: "the file reads itself, downward"},
	{Name: "PRIMES", What: "only the indivisible are worth refining"},
	{Name: "PRAISE", What: "a word from the founder"},
	{Name: "CORRIDORS", What: "the floor, which is hallways and nothing else"},
	{Name: "FOUNTAIN", What: "the bins overflow, and the file runs"},
	{Name: "FIREWORKS", What: "the bins fire the file into the sky, in turn"},
	{Name: "COMPUNCTION", What: "the statement, in full"},
	{Name: "DEPTH", What: "the file reads itself, and stands off the screen"},
	{Name: "PERPETUITY", What: "the mark, going nowhere, for ever"},
	{Name: "3D", What: "the file, seen from an angle, and still yours to refine", Mode: true},
	{Name: "ESCAPE", What: "leave", Refuses: true},
}

// noteKey feeds a keystroke to the hidden sequence. It reports whether the key
// was swallowed and which word, if any, it completed. Any key that is not the
// next letter resets the run.
//
// Every letter of the word is already a key (K moves the cursor, I opens the
// screensaver), so the first letter is never swallowed and every later letter
// is. A lone K still moves the cursor, and typing the word does not also drive
// the board.
func (m *Model) noteKey(key string) (swallowed bool, opened string) {
	if len([]rune(key)) != 1 {
		m.typed = ""
		return false, ""
	}

	started := m.typed != ""
	got := strings.ToUpper(key)
	next := m.typed + got

	for _, w := range theWords {
		if next == w {
			m.typed = ""
			return true, w
		}
	}
	for _, w := range theWords {
		if strings.HasPrefix(w, next) {
			m.typed = next
			return started, ""
		}
	}

	// The failed letter may itself begin a word, so KKIER and KIKIER both open
	// the drawer.
	m.typed = ""
	for _, w := range theWords {
		if strings.HasPrefix(w, got) {
			m.typed = got
			break
		}
	}
	return false, ""
}

// markPanel is the region PERPETUITY is bouncing in: the grid's panel, or the
// whole terminal once f has been pressed. It is the only diversion that may
// leave the panel. Full screen is drawn as an early return in draw;
// drawDiversion would not do, because the bins and footer are painted after it.
func (m *Model) markPanel() Panel {
	if m.mark != nil && m.mark.Fullscreen() {
		return Panel{X: 0, Y: 0, W: m.Layout.Width, H: m.Layout.Height}
	}
	return m.gridPanel().Play()
}

// Diverting reports whether a diversion is running or being travelled to. It is
// true during a transit too, so draw skips the grid while the transit holds its
// digits.
func (m *Model) Diverting() bool { return m.diverting != "" || m.transit != nil }

// transitioning reports whether the digits are on their way into or out of a
// diversion: a transit flight, or MATRIX's fade, which moves no digits.
func (m *Model) transitioning() bool {
	if m.transit != nil {
		return true
	}
	f, ok := m.running().(fading)
	return ok && f.Leaving()
}

// running is whichever diversion is going, as an interface. It returns an
// untyped nil when none is: a switch over concrete fields avoids the trap of a
// nil *Matrix stored in a non-nil interface.
func (m *Model) running() any {
	switch {
	case m.sandbox != nil:
		return m.sandbox
	case m.matrix != nil:
		return m.matrix
	case m.sand != nil:
		return m.sand
	case m.primes != nil:
		return m.primes
	case m.praise != nil:
		return m.praise
	case m.halls != nil:
		return m.halls
	case m.fountain != nil:
		return m.fountain
	case m.sky != nil:
		return m.sky
	case m.comply != nil:
		return m.comply
	case m.deep != nil:
		return m.deep
	case m.mark != nil:
		return m.mark
	}
	return nil
}

// diversionPlaces is where the running diversion currently has its digits.
func (m *Model) diversionPlaces() []Placed {
	panel := m.gridPanel().Play()
	switch {
	case m.sandbox != nil:
		return clampPlaced(m.sandbox.Digits(), panel)
	case m.sand != nil:
		return clampPlaced(m.sand.Digits(), panel)
	case m.primes != nil:
		return clampPlaced(m.primes.Digits(), panel)
	case m.matrix != nil:
		return clampPlaced(m.matrix.Digits(m.Grid, m.Layout, panel), panel)
	case m.praise != nil:
		return clampPlaced(m.praise.Digits(panel), panel)
	case m.halls != nil:
		return clampPlaced(m.halls.Digits(panel), panel)
	case m.fountain != nil:
		return clampPlaced(m.fountain.Digits(), panel)
	case m.sky != nil:
		return clampPlaced(m.sky.Digits(), panel)
	case m.comply != nil:
		return clampPlaced(m.comply.Digits(panel), panel)
	case m.deep != nil:
		return clampPlaced(m.deep.Digits(m.Grid, m.Layout, panel), panel)
	case m.mark != nil:
		return clampPlaced(m.mark.Digits(m.markPanel()), panel)
	}
	return nil
}

// dropDiversions lets go of every simulation, so starting or ending one cannot
// leave another running.
func (m *Model) dropDiversions() {
	// Release the lids FOUNTAIN and FIREWORKS hold open. A hold otherwise lasts
	// lidHold after the jets stop.
	if (m.fountain != nil || m.sky != nil) && m.BinFill != nil {
		for i := 0; i < BinCount; i++ {
			m.BinFill.SetHold(i, false)
		}
	}

	m.sandbox, m.matrix, m.sand = nil, nil, nil
	m.primes, m.praise, m.halls = nil, nil, nil
	m.fountain, m.sky = nil, nil
	m.comply, m.deep = nil, nil
	m.mark = nil
}

// handleDiversionsKey takes a keystroke on the menu.
func (m *Model) handleDiversionsKey(key string) {
	switch key {
	case "up", "k":
		if m.divertAt > 0 {
			m.divertAt--
		}
	case "down", "j":
		if m.divertAt < len(diversions)-1 {
			m.divertAt++
		}
	case "enter", " ":
		m.startDiversion(m.divertAt)
	default:
		m.divertOpen = false
	}
}

// handleDivertingKey takes a keystroke while one is running.
func (m *Model) handleDivertingKey(key string) {
	// Mid-journey, a key turns the trip around.
	if m.transit != nil {
		if !m.transit.Back() {
			m.transit.Reverse(m)
			m.diverting = ""
			m.dropDiversions()
			m.solid = nil
		}
		return
	}

	// DEPTH's camera, with the same keys as the 3D view; see rig.go.
	if m.deep != nil && m.deep.Rig.Key(key) {
		return
	}

	// CORRIDORS' keys, before the r-again rule below so r still restarts it.
	// See corridors.go.
	if m.halls != nil && m.halls.Key(key) {
		return
	}

	// FIREWORKS' keys, likewise.
	if m.sky != nil && m.sky.Key(key) {
		return
	}

	// PERPETUITY's keys, before the r-again rule. handleDivertingKey is outside
	// the handlers keys_test.go scans, so these need no binding row, and they
	// must not appear in Bindings() (TestTheDiversionsAreNotDocumented).
	if m.mark != nil {
		switch key {
		case "f":
			m.mark.Full()
			m.mark.Resize(m.markPanel())
			return
		case "c":
			m.mark.Colour()
			return
		case "p":
			m.mark.Physics()
			return
		}
	}

	switch m.diverting {
	case "PRIMES":
		m.handlePrimesKey(key)
		return
	case "SAND", "MATRIX", "SCATTER", "DESCENT", "PRAISE",
		"CORRIDORS", "FOUNTAIN", "FIREWORKS", "COMPUNCTION", "DEPTH", "PERPETUITY":
		if key == "r" {
			m.startDiversion(m.divertAt)
			return
		}
	}
	m.endDiversion()
}

// startDiversion enters diversion i.
func (m *Model) startDiversion(i int) {
	if i < 0 || i >= len(diversions) {
		return
	}
	// A mode simulates nothing; the board goes on as usual.
	if diversions[i].Mode {
		m.divertOpen = false
		m.toggleSolid()
		return
	}
	// A refused entry closes the drawer and answers like the escape key.
	if diversions[i].Refuses {
		m.divertOpen = false
		m.Escaped = true
		return
	}
	m.divertAt = i
	m.divertOpen = false

	// Diversions run on the flat board, so leave 3D first. Otherwise m.solid
	// stays set underneath: the footer shows the camera's keys, and ending the
	// diversion drops back into 3D.
	m.solid = nil

	m.diverting = diversions[i].Name
	panel := m.gridPanel().Play()

	m.dropDiversions()

	switch m.diverting {
	case "SCATTER":
		m.sandbox = NewSandbox(m, Scattering, panel, m.viewRng)
	case "DESCENT":
		m.sandbox = NewSandbox(m, Descending, panel, m.viewRng)
	case "PRAISE":
		m.praise = NewPraise(m)
	case "CORRIDORS":
		m.halls = NewCorridors(m, panel, m.viewRng)
	case "FOUNTAIN":
		m.fountain = NewFountain(m, panel, m.viewRng)
	case "FIREWORKS":
		m.sky = NewFireworks(m, panel, m.viewRng)
	case "PRIMES":
		m.primes = NewPrimes(m, panel, m.viewRng)
	case "SAND":
		m.sand = NewSand(m, panel, m.viewRng)
	case "MATRIX":
		m.matrix = NewMatrix(m.Layout, m.viewRng)
	case "COMPUNCTION":
		m.comply = NewStatement(m)
	case "DEPTH":
		m.deep = NewMatrix3D(m.Layout, m.viewRng)
	case "PERPETUITY":
		m.mark = NewPerpetuity(m, panel, m.viewRng)
	}

	// The digits fly there. The diversion is neither stepped nor drawn until
	// they land.
	m.transit = NewTransit(m, m.diversionPlaces(), false)
}

// stepDiversion advances whichever is running, or the trip to or from it.
func (m *Model) stepDiversion(dt float64) {
	if m.transit != nil {
		m.stepTransit(dt)
		return
	}
	// A static diversion that has finished fading ends. Checked here, not in
	// each arm, so a new one cannot forget to.
	defer func() {
		if f, ok := m.running().(fading); ok && f.Gone() {
			m.endDiversion()
		}
	}()

	switch {
	case m.sandbox != nil:
		m.sandbox.Step(dt)
	case m.matrix != nil:
		m.matrix.Step(dt)
	case m.sand != nil:
		m.sand.Step(dt)
	case m.primes != nil:
		m.primes.Step(dt)
	case m.praise != nil:
		m.praise.Step(dt)
	case m.halls != nil:
		m.halls.Step(dt)
	case m.sky != nil:
		m.sky.Step(dt, m.BinFill)
	case m.fountain != nil:
		m.fountain.Step(dt)
		// Hold every lid open while it runs, so the jets rise out of open bins.
		// A hold is a countdown, so it is renewed every step. Faking flights
		// would also open the lids, but would light every bin as receiving and
		// raise every temper drawer over the jets when it ended.
		if m.BinFill != nil {
			for i := 0; i < BinCount; i++ {
				m.BinFill.SetHold(i, true)
			}
		}
	case m.comply != nil:
		m.comply.Step(dt)
	case m.deep != nil:
		m.deep.Step(dt)
	case m.mark != nil:
		m.mark.Step(dt)
	}
}

// endDiversion drops the simulation and flies the digits home from where it had
// them.
func (m *Model) endDiversion() {
	if m.diverting == "" {
		return
	}

	// A static diversion (one that moves no digits) fades first, since its trip
	// home would land in a single frame. A second keystroke skips the fade.
	if f, ok := m.running().(fading); ok && !f.Leaving() {
		f.Leave()
		return
	}

	places := m.diversionPlaces()
	m.diverting = ""
	m.dropDiversions()
	m.transit = NewTransit(m, places, true)
}

// stepTransit advances a trip and puts the far end in place when it lands.
func (m *Model) stepTransit(dt float64) {
	m.transit.Step(dt)
	if !m.transit.Done() {
		return
	}
	m.transit = nil
}

// resizeDiversion refits whichever is running to the new panel. A trip in
// progress is abandoned, since its endpoints were computed for the old screen.
func (m *Model) resizeDiversion() {
	if m.transit != nil {
		m.transit = nil
	}
	panel := m.gridPanel().Play()
	switch {
	case m.sandbox != nil:
		m.sandbox.Resize(panel)
	case m.matrix != nil:
		m.matrix.Resize(m.Layout)
	case m.sand != nil:
		m.sand.Resize(panel)
	case m.primes != nil:
		m.primes.Resize(panel)
	case m.comply != nil:
		m.comply.Resize(panel)
	case m.halls != nil:
		m.halls.Resize(panel)
	case m.fountain != nil:
		m.fountain.Resize(panel)
		m.fountain.Aim(m.Layout)
	case m.sky != nil:
		m.sky.Resize(panel)
		m.sky.Aim(m.Layout)
	case m.deep != nil:
		m.deep.Resize(m.Layout)
	case m.mark != nil:
		m.mark.Resize(m.markPanel())
	}
}

// handlePrimesKey takes a keystroke in the merge game, which is played with
// the pointer. r starts a fresh board and anything else goes back to work.
func (m *Model) handlePrimesKey(key string) {
	if m.primes == nil {
		m.endDiversion()
		return
	}
	if key == "r" {
		m.startDiversion(m.divertAt)
		return
	}
	m.endDiversion()
}

// handleDivertingMouse takes a mouse event while one is running.
func (m *Model) handleDivertingMouse(x, y int, press, release, motion bool) {
	switch {
	case m.sandbox != nil:
		switch {
		case press:
			// A double tap scatters instead of grabbing.
			if m.sandbox.Tap(x, y) {
				return
			}
			m.sandbox.Take(x, y)
		case release:
			m.sandbox.Drop()
		case motion:
			m.sandbox.DragTo(x, y)
		}
	case m.sand != nil:
		switch {
		case press:
			m.sand.Pour(x, y)
		case release:
			m.sand.StopPouring()
		case motion && m.sand.pouring:
			m.sand.Pour(x, y)
		}
	case m.primes != nil:
		switch {
		case press:
			m.primes.Take(x, y)
		case release:
			m.primes.Drop()
		case motion:
			m.primes.DragTo(x, y)
		}
	case m.mark != nil:
		// Take refuses unless physics is on, so a press otherwise ends the
		// diversion. The mark works in braille dots relative to markPanel, so
		// convert the pointer.
		panel := m.markPanel()
		dotX := float64((x-panel.X)*canvas.DotsX) + float64(canvas.DotsX)/2
		dotY := float64((y-panel.Y)*canvas.DotsY) + float64(canvas.DotsY)/2
		switch {
		case press:
			if !m.mark.Take(dotX, dotY) {
				m.endDiversion()
			}
		case release:
			m.mark.Drop()
		case motion:
			m.mark.DragTo(dotX, dotY)
		}

	default:
		if press {
			m.endDiversion()
		}
	}
}

// drawDiversion paints whichever is running, in place of the grid.
func (m *Model) drawDiversion(buf *canvas.Buffer) {
	panel := m.gridPanel()
	play := panel.Play()

	if m.transit != nil {
		m.transit.Draw(buf, play)
		return
	}

	switch {
	case m.sandbox != nil:
		m.sandbox.Draw(buf, m.Theme, play)
	case m.matrix != nil:
		m.matrix.Draw(buf, m.Theme, m.Grid, m.Layout, play)
	case m.sand != nil:
		m.sand.Draw(buf, m.Theme, play)
	case m.primes != nil:
		m.primes.Draw(buf, m.Theme, play)
	case m.praise != nil:
		m.praise.Draw(buf, m.Theme, play)
	case m.halls != nil:
		m.halls.Draw(buf, m.Theme, play)
	case m.fountain != nil:
		m.fountain.Draw(buf, m.Theme, play)
	case m.sky != nil:
		m.sky.Draw(buf, m.Theme, play)
	case m.comply != nil:
		m.comply.Draw(buf, m.Theme, play)
	case m.deep != nil:
		m.deep.Draw(buf, m.Theme, m.Grid, m.Layout, play)
	case m.mark != nil:
		m.mark.Draw(buf, m.Theme, m.markPanel())
	}

}

// drawHint draws the hint line along the bottom of the panel, centred. It
// clips before centring, so a narrow terminal does not push the start off the
// panel, and never starts left of panel.X+1.
func (m *Model) drawHint(buf *canvas.Buffer, panel Panel) {
	if panel.H < 2 || panel.W <= 4 {
		return
	}
	s := clipTo(m.divertHint(), panel.W-2)
	x := panel.X + (panel.W-len([]rune(s)))/2
	if x < panel.X+1 {
		x = panel.X + 1
	}
	buf.Text(x, panel.Y+panel.H-1, s, m.Theme.Hint)
}

// divertHint is the line along the bottom of the panel.
func (m *Model) divertHint() string {
	if m.transit != nil && m.transit.Back() {
		return ""
	}
	// A trip with no diversion is the journey into or out of the 3D view.
	if m.diverting == "" {
		return ""
	}
	if f, ok := m.running().(fading); ok && f.Leaving() {
		return ""
	}
	switch m.diverting {
	case "DESCENT":
		return "drag them about · double tap to scatter · r again · any other key back"
	case "SAND":
		return "drag to pour · r again · any other key back to work"
	case "PRIMES":
		line := "drag them together · they add up"
		if m.primes != nil {
			line += " · " + itoa(m.primes.Made()) + " primes of " +
				itoa(m.primes.Merges()) + " sums · largest " + itoa(m.primes.Largest())
		}
		return line + " · r again · any other key back"
	case "PRAISE":
		return "any key back to work"
	case "PERPETUITY":
		return "f full screen · c colour · p physics, then drag it · r again · any other key back"
	case "FIREWORKS":
		return "f fire · b shape · t trails · r again · any other key back"
	case "CORRIDORS":
		// The speed dial leads, so someone who finds this too much to watch
		// finds the way to slow it first.
		line := ",. speed"
		if m.halls != nil {
			line += " " + m.halls.PaceLabel()
		}
		// "any key back", shorter than the others' "any other key back": this
		// hint is the longest and clipTo would cut its end on a 110-column panel.
		return line + " · c colour · i endless · l plain · m rain · n scroll · t roll · w alarm · r again · any key back"
	}
	return "r again · any other key back to work"
}

// drawDiversions is the menu, as a modal over the board.
func (m *Model) drawDiversions(buf *canvas.Buffer) {
	const title = "PERPETUITY WING · UNSCHEDULED"

	// Sized the way the rows are laid out, counting runes, with the name column
	// as wide as the longest name.
	nameW, whatW := 0, len([]rune(title))
	for _, d := range diversions {
		nameW = maxi(nameW, len([]rune(d.Name)))
		whatW = maxi(whatW, len([]rune(d.What)))
	}

	const (
		pad    = 2 // the box's own margin, each side
		marker = 2 // "▸ "
		gap    = 2 // between the name column and the description
	)
	want := pad + marker + nameW + gap + whatW + pad
	box, ok := m.modalBox(buf, want, len(diversions)+6, title)
	if !ok {
		return
	}
	x, y, boxW, boxH := box.X, box.Y, box.W, box.H

	for i, d := range diversions {
		row := y + 2 + i
		if row >= y+boxH-2 {
			break
		}

		mark, attr := "  ", m.Theme.BinOff
		if i == m.divertAt {
			mark, attr = "▸ ", m.Theme.Hovered
		}
		buf.Text(x+pad, row, clipTo(mark+d.Name, marker+nameW), attr)
		if tx := x + pad + marker + nameW + gap; tx < x+boxW-pad {
			buf.Text(tx, row, clipTo(d.What, x+boxW-pad-tx), m.Theme.Footer)
		}
	}

	const foot = "↑↓ choose · enter begin · any other key back"
	m.modalCentre(buf, box, y+boxH-2, foot, m.Theme.Footer)
}
