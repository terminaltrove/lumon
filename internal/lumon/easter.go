package lumon

// Easter eggs. The show's file names (file.go) are all its own; these are
// ours, and say so on the way out.

// EasterEggFiles are files of our own: one about the game, one about
// Terminal Trove, and Null, after Null Island, the point at 0°, 0° where
// bad map data lands.
var EasterEggFiles = []string{"Localhost", "Trove", nullFile}

// nullFile's board is all zeroes, and it reads 0% until it is done.
const nullFile = "Null"

// fileLines is the extra line a finished file's notice gives some files.
var fileLines = map[string]string{
	"Localhost": "There's no place like 127.0.0.1.",
	"Trove":     "This file was curated by Terminal Trove.",
	"Kernighan": "hello, world",
	"Ritchie":   "hello, world",
	"Thompson":  "When in doubt, use brute force.",
	"Pike":      "Simplicity is complicated.",
	"McIlroy":   "Do one thing and do it well.",
	"Jansky":    "The hiss was the Milky Way.",
	"Penzias":   "It wasn't the pigeons.",
	"Wilson":    "It wasn't the pigeons.",
}

// fileLine is the extra line for a finished file, or "".
func fileLine(name string) string { return fileLines[name] }

// digitFor is the digit a cell of the named file shows, given the one drawn
// for it.
func digitFor(name string, drawn int) int {
	if name == nullFile {
		return 0
	}
	return drawn
}

// dressGrid gives a freshly made board its file's digits.
func (r *Room) dressGrid() {
	if r.FileName != nullFile {
		return
	}
	for i := range r.Grid.Cells {
		r.Grid.Cells[i].Value = 0
	}
}

// shownPercent is the completion a file's header reads.
func shownPercent(name string, percent int) int {
	if name == nullFile && percent < 100 {
		return 0
	}
	return percent
}

// celebrateCorner is the screensaver's mark landing exactly in a corner: the
// floor hears of it, and the set beeps.
func (m *Model) celebrateCorner() {
	if m.rooms != nil {
		m.rooms.news.Post("THE MARK HIT THE CORNER")
	}
	m.cue(SoundBeep, 1)
}
