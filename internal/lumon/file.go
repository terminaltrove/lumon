package lumon

import (
	"github.com/terminaltrove/lumon/internal/ease"
	"math/rand"

	"github.com/terminaltrove/lumon/internal/physics"
)

// A macrodata file is what a refiner is assigned to; the bar across the top
// counts it down. Completion is read from the bins: every cluster is assigned
// to a bin when the grid is built, so the file is finished when every cluster
// has been refined into one.

// fillFrequency is how fast the bar chases the true completion. The show's bar
// jumps; this one eases like everything else on screen. Raise it towards
// infinity for the show's behaviour.
const fillFrequency = 2.4

// File is the macrodata file this terminal is refining.
type File struct {
	Name string

	// fill is the drawn completion, easing toward the true one.
	fill *physics.Scalar
}

// NewFile picks a file for a seed. The same seed always draws the same name.
func NewFile(seed int64, g *Grid) *File {
	return NewFileNamed(fileName(rand.New(rand.NewSource(seed))))
}

// NewFileNamed is one player's view of a named file. The name belongs to the
// board; the fill is per player and eases up from zero when somebody sits down.
func NewFileNamed(name string) *File {
	return &File{
		Name: name,
		fill: physics.NewScalar(0, fillFrequency),
	}
}

// Completion is how much of the file is refined, from 0 to 1.
func (f *File) Completion(g *Grid) float64 {
	refined, total := 0, 0
	for _, bin := range g.Bins {
		refined += bin.Refined
		total += bin.Quota
	}
	if total == 0 {
		return 0
	}
	return float64(refined) / float64(total)
}

// Percent is the completion the terminal reports, as a whole number. It reads
// the true value, so it is right the moment a cluster lands while the bar is
// still easing.
func (f *File) Percent(g *Grid) int { return int(f.Completion(g) * 100) }

// Step advances the drawn fill toward the true completion.
func (f *File) Step(dt float64, g *Grid) { f.fill.Step(dt, f.Completion(g)) }

// Fill is how full the bar should currently be drawn, from 0 to 1.
func (f *File) Fill() float64 { return ease.Clamp01(f.fill.Value()) }

// Complete reports whether every cluster has been refined.
func (f *File) Complete(g *Grid) bool { return f.Completion(g) >= 1 }

// fileName draws a file name from the full pool.
func fileName(rng *rand.Rand) string {
	names := FileNames()
	return names[rng.Intn(len(names))]
}

// FileNames is every name a file may be given: the show's, then Bell Labs',
// then our own (easter.go).
func FileNames() []string {
	out := make([]string, 0, len(CompletedFiles)+len(DirectoryFiles)+len(BellLabsFiles)+len(EasterEggFiles))
	out = append(out, CompletedFiles...)
	out = append(out, DirectoryFiles...)
	out = append(out, BellLabsFiles...)
	out = append(out, EasterEggFiles...)
	return out
}

// Every file name is one the show (or its book) put on screen; none is made
// up. A name goes in only with a source: an episode, and what showed it.

// CompletedFiles are the twenty-five files Mark S. finished, numbered in the
// order of his completed-file history on screen in "Chikhai Bardo" (2x07).
// Several are also doors on the testing floor.
var CompletedFiles = []string{
	"Allentown", // 1: his first, the "freshman fluke"; a room, 2x07
	"Trinity",
	"Todos Santos",
	"Astoria", // a room, 2x07
	"Lucknow", // a room, 2x07
	"St. Pierre",
	"Coleman", // a room, 2x10
	"Waynesboro",
	"Cork",
	"Molde",
	"Cairns",
	"Bodo",
	"Zurich",
	"Culpepper", // the show's spelling; the town is Culpeper
	"Bellingham",
	"Billings",
	"Yakima",
	"Loveland",
	"Merida",
	"Sopchoppy",
	"Vilnius",
	"Rhodes",
	"Wellington", // the dentist's room, 2x07
	"Dranesville",
	"Cold Harbor", // 25: the last, 2x10
}

// DirectoryFiles are the other files the show names. Lexington alone is from
// the tie-in book, The Lexington Letter. The lobby's drawer lists every file
// name, these and Mark's alike.
var DirectoryFiles = []string{
	"Siena",       // Helly's first file, 1x02
	"Tumwater",    // Dylan's, 1x01 and 1x02
	"Sunset Park", // asked about in 1x03
	"Eminence",    // Dylan's, 1x07
	"Santa Mira",  // on Helly's screen, 2x01
	"Montauk",     // in Irving's dream, 2x04
	// The directory Mark scrolls to find Siena, 1x02.
	"Kingsport", "Labrador", "Le Mars", "Longbranch", "Moonbeam",
	"Nanning", "Narva", "Ocula", "Pacoima",
	"Lahore",    // a door on the testing floor, 2x07
	"Minsk",     // a door on the testing floor, 2x10
	"Lexington", // the Topeka branch's file, The Lexington Letter
}

// BellLabsFiles are not the show's. The Lumon building is Bell Labs' Holmdel
// complex, so a few files are named for people who worked at Bell Labs: Unix's
// makers at Murray Hill, and at Holmdel itself, Jansky, who began radio
// astronomy, and Penzias and Wilson, whose horn antenna heard the Big Bang.
var BellLabsFiles = []string{
	"Kernighan", "Ritchie", "Thompson", "Pike", "McIlroy", "Ossanna",
	"Jansky", "Penzias", "Wilson",
}
