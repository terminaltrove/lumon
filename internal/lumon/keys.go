package lumon

import "strings"

// The controls, written down once. The ? overlay, the grid footer and --help
// all render from this table, and TestEveryKeyIsDocumented parses the key
// handlers to check it against what is bound. Update it whenever you bind a
// key.

// Binding is one row of the controls table: the keys, and what they do.
type Binding struct {
	// Keys as the player reads them: "arrows / hjkl", not "up".
	Keys string

	// Bound is the key strings the handler matches, checked by
	// TestEveryKeyIsDocumented. Empty for rows that are not a key, such as the
	// mouse.
	Bound []string

	// What it does.
	What string

	// Short is the compact form for the footer. Empty leaves the row out.
	Short string

	// Browser marks a control that exists only in the browser. Bindings leaves
	// it out for terminal and remote players.
	Browser bool

	// Company marks a control for computer refiners, which exist only where
	// the host allows them (a local terminal and the browser, never a remote session).
	Company bool

	// LAN marks a control for hosting and joining, which only somebody at their
	// own terminal is offered.
	LAN bool

	// Multiplayer marks controls that need a shared human floor over a network.
	Multiplayer bool
}

// BindingGroup is the controls for one screen.
type BindingGroup struct {
	Title    string
	Bindings []Binding
}

// Bindings is every terminal control, grouped by where it applies. It returns
// the shared table, built once because the footer reads it every frame; callers
// must not modify it.
func Bindings() []BindingGroup { return terminalBindings }

// terminalBindings is the table without its browser-only rows or the
// computer refiners'; companyBindings keeps the computer refiners'.
var (
	terminalBindings    = keepBindings(func(b Binding) bool { return !b.Browser && !b.Company && !b.LAN })
	companyBindings     = keepBindings(func(b Binding) bool { return !b.Browser && !b.LAN })
	lanBindings         = keepBindings(func(b Binding) bool { return !b.Browser && !b.Company })
	companyLANBindings  = keepBindings(func(b Binding) bool { return !b.Browser })
	soloBindings        = keepBindings(func(b Binding) bool { return !b.Browser && !b.Company && !b.LAN && !b.Multiplayer })
	soloCompanyBindings = keepBindings(func(b Binding) bool { return !b.Browser && !b.LAN && !b.Multiplayer })
)

func keepBindings(keep func(Binding) bool) []BindingGroup {
	groups := make([]BindingGroup, 0, len(bindingTable))
	for _, group := range bindingTable {
		kept := BindingGroup{Title: group.Title}
		for _, binding := range group.Bindings {
			if keep(binding) {
				kept.Bindings = append(kept.Bindings, binding)
			}
		}
		groups = append(groups, kept)
	}
	return groups
}

var bindingTable = []BindingGroup{
	{Title: "REFINING", Bindings: []Binding{
		{Keys: "arrows / hjkl", Bound: []string{"up", "k", "down", "j", "left", "h", "right", "l"},
			What: "move the cursor", Short: "↑↓←→ move"},
		{Keys: "f / shift+arrows / HJKL", Bound: []string{"f", "shift+up",
			"shift+down", "shift+left", "shift+right", "H", "J", "K", "L"},
			What:  "take the cluster under the cursor, and keep it while you move",
			Short: "f focus"},
		{Keys: "1-5", Bound: []string{"1", "2", "3", "4", "5"},
			What: "choose a bin", Short: "1-5 bin"},
		{Keys: "o", Bound: []string{"o"}, What: "open the selected bin"},
		{Keys: "enter / space", Bound: []string{"enter", " "},
			What: "refine the cluster under the cursor", Short: "enter refine"},
		{Keys: "mouse", What: "click a digit to refine it, or a bin to select it"},
		{Keys: "wheel", What: "scroll the grid"},
	}},
	{Title: "THE TERMINAL", Bindings: []Binding{
		{Keys: "s", Bound: []string{"s"}, What: "who has refined what", Short: "s scores"},
		{Keys: "tab / ←→", Bound: []string{"tab"}, What: "on the scores: this quarter, lifetime, the record"},
		{Keys: "+ / -", Bound: []string{"+", "=", "-", "_"},
			What:  "spread the digits further apart, or bring them back in",
			Short: "+- zoom"},
		{Keys: "t", Bound: []string{"t"}, What: "next colour theme", Short: "t theme"},
		{Keys: "?", Bound: []string{"?"}, What: "these controls", Short: "? help"},
		{Keys: "a", Bound: []string{"a"}, What: "who made this"},
		{Keys: "i", Bound: []string{"i"}, What: "screensaver"},
		{Keys: "n", Bound: []string{"n"}, What: "hide or show the floor news"},
		{Keys: "m", Bound: []string{"m"}, What: "host a LAN game, or join one", LAN: true},
		{Keys: "b", Bound: []string{"b"}, What: "broadcast a Lumon phrase over your cursor", Short: "b phrases"},
		// Escape is never documented: pressing it reports you (escape.go). It
		// is bound with an `if`, not a case clause, so the scan in keys_test.go
		// does not require a row for it.
		{Keys: "q", Bound: []string{"q"},
			What: "back out, or choose another file or quit", Short: "q quit/files"},
		{Keys: "ctrl+c", Bound: []string{"ctrl+c"}, What: "quit from anywhere, even mid-name"},
		// Last, so the footer drops them first.
		{Keys: "c", Bound: []string{"c"}, What: "a computer refiner joins you; with every desk full, they all go",
			Short: "c company", Company: true},
		{Keys: "w", Bound: []string{"w"}, What: "get up and watch the computer refiners, or sit back down",
			Short: "w watch", Company: true},
		{Keys: "1-4 / tab / 0", Bound: []string{"0", "tab"},
			What: "while watching: follow that desk's refiner, the next, or any", Company: true},
	}},
	{Title: "CHOOSING A FILE", Bindings: []Binding{
		{Keys: "↑↓ / jk", Bound: []string{"up", "k", "down", "j"},
			What: "the next file, or the next row in the grid"},
		// The scan would pass without this row because REFINING declares
		// left/right, but they mean something else here.
		{Keys: "←→", Bound: []string{"left", "right"},
			What: "jump a letter, or a tile in the grid"},
		{Keys: "wheel", What: "scroll the drawer"},
		// Already documented under THE TERMINAL; this row says what they do on
		// this screen.
		// Only where human multiplayer is available.
		{Keys: "+ / -", Bound: []string{"+", "=", "-", "_"}, What: busiestWhat, Multiplayer: true},
		{Keys: "v", Bound: []string{"v"}, What: "the rolodex, the list, or the grid of files"},
		{Keys: "enter", Bound: []string{"enter", " "}, What: "join the file you are on"},
		{Keys: "n", Bound: []string{"n"}, What: "open a new file"},
		{Keys: "l", Bound: []string{"l"}, What: "host a LAN game, or join one", LAN: true},
		{Keys: "c", Bound: []string{"c"}, What: "how many computer refiners join you", Company: true},
		// Remotely the boot sequence plays once per connection.
		{Keys: "r", Bound: []string{"r"}, What: "replay the boot sequence", Browser: true},
		// Wipes the browser's saved progress after a y to confirm.
		{Keys: "x", Bound: []string{"x"}, What: "wipe saved progress and start again", Browser: true},
	}},
}

// busiestWhat is what + and - do on the files.
const busiestWhat = "the file with the most refiners on it, or the fewest"

var browserBindings = func() []BindingGroup {
	groups := make([]BindingGroup, 0, len(bindingTable))
	for _, group := range bindingTable {
		copy := BindingGroup{Title: group.Title}
		for _, binding := range group.Bindings {
			if binding.Keys == "ctrl+c" || binding.LAN || binding.Multiplayer {
				continue
			}
			if binding.Keys == "q" {
				binding.What = "back out, or return to the files"
				binding.Short = "q files"
			}
			copy.Bindings = append(copy.Bindings, binding)
		}
		groups = append(groups, copy)
	}
	return groups
}()

func browserHints(rungs []string) []string {
	out := make([]string, len(rungs))
	for i, rung := range rungs {
		out[i] = strings.TrimSuffix(strings.ReplaceAll(rung, "q quit/files", "q files"), " · q quit")
	}
	return out
}

var browserFooterLadder = browserHints(companyFooterLadder)

// The browser's lobby lines are their own, not derived: they drop q, which has
// nothing to go back to, and + and -, since nobody else is on its files to be
// the busiest, and add r and x, which only the browser binds.
var browserLobbyLadder = ladder([]string{
	"↑↓ choose", "enter join", "v view", "n new file", "s scores", "t theme", "r intro", "x reset", "? help", "c company",
})
var browserGridLadder = ladder([]string{
	"←→↑↓ choose", "enter join", "v view", "n new file", "s scores", "t theme", "r intro", "x reset", "? help", "c company",
})

// The touch screen's lines name what the page's gestures and toolbar do (see
// the page); the toolbar's own buttons need no hint.
var touchFooterLadder = ladder([]string{
	"tap to refine", "drag to sweep", "tap a bin", "two fingers to scroll",
})
var touchLobbyLadder = ladder([]string{
	"tap a file to join", "two fingers to scroll",
})

// footerHint is the controls line along the bottom of the grid, at most width
// columns wide. It is generated from the table's Short fields; rows drop from
// the right as the terminal narrows, so the essentials come first.
func footerHint(width int) string { return pick(footerLadder, width) }

// footerLadder is every length of the footer hint, longest first, joined once
// at startup so the draw path only chooses one. companyFooterLadder adds the
// computer refiners' keys.
var (
	footerLadder        = ladder(footerShorts(false))
	companyFooterLadder = ladder(footerShorts(true))
)

func footerShorts(company bool) []string {
	var short []string
	for _, g := range bindingTable {
		if g.Title == "CHOOSING A FILE" {
			continue // the lobby has its own line
		}
		for _, b := range g.Bindings {
			if b.Short != "" && (company || !b.Company) {
				short = append(short, b.Short)
			}
		}
	}
	return short
}

// ladder joins a list of parts at every length, longest first.
func ladder(parts []string) []string {
	out := make([]string, 0, len(parts))
	for n := len(parts); n > 0; n-- {
		out = append(out, strings.Join(parts[:n], " · "))
	}
	return out
}

// pick is the longest rung that fits.
func pick(rungs []string, width int) string {
	for _, s := range rungs {
		if len([]rune(s)) <= width {
			return s
		}
	}
	return ""
}

// lobbyHint is the same, for the drawer. The arrows mean different things in
// the grid view (tiles and rows) and the rolodex or list (files and letters),
// so each has its own ladder.
//
// These ladders are hand-written and no test checks them against bindingTable,
// so a key added to the lobby must be added here too. q goes back to the name
// screen remotely and quits at a local terminal.
func (m *Model) lobbyHint(width int) string {
	switch {
	case m.touchScreen:
		return pick(touchLobbyLadder, width)
	case m.keepOpen && m.lobbyView == viewGrid:
		return pick(browserGridLadder, width)
	case m.keepOpen:
		return pick(browserLobbyLadder, width)
	case m.lan != nil && m.lobbyView == viewGrid:
		return pick(gridLANLadder, width)
	case m.lan != nil:
		return pick(lobbyLANLadder, width)
	case m.lobbyView == viewGrid && (m.askName || m.joined):
		return pick(gridBackLadder, width)
	case m.lobbyView == viewGrid:
		return pick(gridLadder, width)
	case m.askName || m.joined:
		return pick(lobbyBackLadder, width)
	}
	return pick(lobbyLadder, width)
}

// A local lobby always allows computer refiners; a remote lobby is the Back
// ladders, which never offer them.
var lobbyLadder = ladder([]string{
	"↑↓ choose", "enter join", "v view", "n new file", "s scores", "t theme", "? help", "q quit", "c company",
})

// A local lobby on a build with LAN games says so.
var lobbyLANLadder = ladder([]string{
	"↑↓ choose", "enter join", "+- busiest", "v view", "n new file", "l LAN", "s scores", "t theme", "? help",
})

var gridLANLadder = ladder([]string{
	"←→↑↓ choose", "enter join", "+- busiest", "v view", "n new file", "l LAN", "s scores", "t theme", "? help",
})

var gridLadder = ladder([]string{
	"←→↑↓ choose", "enter join", "v view", "n new file", "s scores", "t theme", "? help", "q quit", "c company",
})

var lobbyBackLadder = ladder([]string{
	"↑↓ choose", "enter join", "+- busiest", "v view", "n new file", "s scores", "t theme", "? help", "q back",
})

var gridBackLadder = ladder([]string{
	"←→↑↓ choose", "enter join", "+- busiest", "v view", "n new file", "s scores", "t theme", "? help", "q back",
})
