package theme

import (
	"strings"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// A theme is held on each Model, never in package state: remotely every
// session shares this package, so a global palette would let one refiner
// restyle everybody. A Palette names roles (the colour a woken cluster is
// drawn in), never hues, so themes can differ in more than hue without
// touching the renderer.

// Palette is the handful of colours a theme is built from.
type Palette struct {
	// Name is how the theme is chosen and stored. Do not store an index
	// instead: inserting a theme would move existing users onto another one.
	Name string

	// Primary is the chrome: rules, headers, the boot log.
	Primary canvas.Color

	// Dim is a bin nobody is feeding, and other chrome that is present but not
	// being addressed.
	Dim canvas.Color

	// Live is a woken cluster before it starts to redden.
	Live canvas.Color

	// Glow is the brightest chrome: the bin you have selected.
	Glow canvas.Color

	// Ink is drawn on a bright background, such as the boot cursor. It must
	// contrast with Primary.
	Ink canvas.Color

	// Peak is the digit under the cursor, the brightest single thing on screen.
	Peak canvas.Color

	// Backdrop is the resting grid. It sits well back so a waking cluster
	// catches the eye.
	Backdrop canvas.Color

	// Lurk is a dormant corrupt digit, a shade off an ordinary one until its
	// cluster activates.
	Lurk canvas.Color

	// Urgent is where the escalation ramp ends, for a cluster left unrefined.
	Urgent canvas.Color

	// Paper is the colour of the screen itself. Unset means the terminal's own
	// background, which the dark themes want. A pale theme must name it, or
	// the terminal's background shows through most of the screen.
	Paper canvas.Color

	// Desks tell refiners apart, tinted to stay close to the rest of the
	// palette. A slice, so the palette does not depend on the board's
	// MaxDesks; DeskColour wraps the seat.
	Desks []canvas.Color

	// Tempers are the four bars in a bin's drawer: Woe, Frolic, Dread, Malice.
	// They are saturated hues that stay the same across themes, as in the
	// show, because they say what was refined. Unset entries take
	// defaultTempers.
	Tempers []canvas.Color

	// Signal is the link light in the footer: good, fair, poor. Like the
	// tempers it keeps fixed saturated hues, since a bad link must look bad in
	// any theme. Unset entries take defaultSignal.
	Signal []canvas.Color
}

// defaultTempers are the four bars' colours where a palette does not name its
// own: green, yellow, magenta, blue. Dread is magenta to match the reference
// frame, though one published account of the show says red.
var defaultTempers = []canvas.Color{
	hex("#5BD97E"), // Woe
	hex("#E8DC63"), // Frolic
	hex("#D970C4"), // Dread
	hex("#5B7DE8"), // Malice
}

// defaultSignal are the link light's three colours where a palette does not name
// its own: a traffic light of green (Woe's), amber and red. Amber, not Frolic's
// yellow, so it reads as a warning on its own.
var defaultSignal = []canvas.Color{
	hex("#5BD97E"), // good
	hex("#E8A63C"), // fair
	hex("#E85B5B"), // poor
}

// Theme is a Palette resolved, once, into the attributes the renderer asks
// for.
type Theme struct {
	Palette

	Chrome canvas.Attr
	Header canvas.Attr
	Number canvas.Attr

	// Digit is a resting digit on the board. Bad is a dormant corrupt one, a
	// shade off it.
	Digit   canvas.Attr
	Bad     canvas.Attr
	BadLive canvas.Attr
	Cursor  canvas.Attr
	BinOn   canvas.Attr
	BinOff  canvas.Attr
	// Footer is text that matters less: descriptions, captions, a file not
	// chosen. It sits between Dim and Peak, so it recedes but can still be read.
	// Number is the colour for something that is only there as decoration.
	Footer   canvas.Attr
	Selected canvas.Attr

	// Hint is the line of keys along the bottom, including a diversion's own.
	// It uses Peak, the brightest text there is, for the one line that tells a
	// new refiner the keys.
	Hint canvas.Attr

	// Plate is the background a ringfenced cluster's enlarged digits sit on.
	// Only a background, since the digits keep their escalation colour (see
	// glyphAttr).
	Plate   canvas.Color
	Swollen canvas.Attr

	FileFill     canvas.Attr
	FileEdge     canvas.Attr
	FileFillText canvas.Attr

	// BinFull is the fill behind a bin's readout: Ink on Primary.
	BinFull canvas.Attr

	// BinFullText is a character on that fill, with the same background so
	// the bar is unbroken.
	BinFullText canvas.Attr

	Boot       canvas.Attr
	BootCursor canvas.Attr
	Hovered    canvas.Attr

	// Page is what the screen is cleared to, and Surface what an overlay clears
	// behind itself. Both are currently the paper colour.
	Page    canvas.Attr
	Surface canvas.Attr
}

// cursorPlateMix is how much Primary is blended into Ink for the cursor's
// plate: visible when looked for, never competing with a woken cluster.
const cursorPlateMix = 0.2

// cursorPlate is what the cursor cell is drawn on: as in the show, a
// rectangle barely lifted off the page, with the digit over it in Peak (Ink
// would vanish against it). It is derived from two colours every palette has,
// so it needs no palette role.
func cursorPlate(p Palette) canvas.Color {
	return BlendColors(p.Ink, p.Primary, cursorPlateMix)
}

// footerLift is how far Footer goes from Dim toward Peak. Less than this falls
// under 4.5:1 on some themes.
const footerLift = 0.4

// digitLift is how far a resting digit sits from Backdrop toward Live: about
// half brightness, as the show's are.
const digitLift = 0.4

// NewTheme resolves a palette. When the palette names Paper, every attribute
// without its own background gets it: a foreground-only attribute replaces the
// cell's background with the terminal's own.
func NewTheme(p Palette) *Theme {
	t := &Theme{
		Palette: p,

		Chrome:   canvas.Attr{FG: p.Primary},
		Header:   canvas.Attr{FG: p.Primary, Bold: true},
		Number:   canvas.Attr{FG: p.Backdrop},
		Digit:    canvas.Attr{FG: BlendColors(p.Backdrop, p.Live, digitLift), Bold: true},
		Bad:      canvas.Attr{FG: BlendColors(p.Lurk, p.Live, digitLift), Bold: true},
		BadLive:  canvas.Attr{FG: p.Live, Bold: true},
		Cursor:   canvas.Attr{FG: p.Peak, BG: cursorPlate(p), Bold: true},
		BinOn:    canvas.Attr{FG: p.Glow, Bold: true},
		BinOff:   canvas.Attr{FG: p.Dim},
		Footer:   canvas.Attr{FG: BlendColors(p.Dim, p.Peak, footerLift)},
		Hint:     canvas.Attr{FG: p.Peak},
		Selected: canvas.Attr{FG: p.Ink, BG: p.Live},
		Plate:    p.Backdrop,
		Swollen:  canvas.Attr{FG: p.Primary},

		FileFill:     canvas.Attr{BG: p.Backdrop},
		FileEdge:     canvas.Attr{FG: p.Backdrop},
		FileFillText: canvas.Attr{FG: p.Glow, BG: p.Backdrop, Bold: true},

		BinFull:     canvas.Attr{FG: p.Ink, BG: p.Primary, Bold: true},
		BinFullText: canvas.Attr{FG: p.Ink, BG: p.Primary, Bold: true},

		Boot:       canvas.Attr{FG: p.Primary},
		BootCursor: canvas.Attr{FG: p.Ink, BG: p.Primary},
		Hovered:    canvas.Attr{FG: p.Peak, Bold: true},
	}

	// Fill out a full-length copy: a palette that names no tempers has nil.
	tempers := make([]canvas.Color, len(defaultTempers))
	copy(tempers, t.Tempers)
	for i, c := range tempers {
		if !c.Set() {
			tempers[i] = defaultTempers[i]
		}
	}
	t.Tempers = tempers

	// Copy, never write into t.Signal: remotely the caller's palette is
	// shared. See dimEach.
	signal := make([]canvas.Color, len(defaultSignal))
	copy(signal, t.Signal)
	for i, c := range signal {
		if !c.Set() {
			signal[i] = defaultSignal[i]
		}
	}
	t.Signal = signal

	// Without paper, Page and Surface stay the zero attribute, which canvas
	// writes as bare runes. Do not give Page a foreground: it would cost an
	// escape sequence in every blank cell.
	if p.Paper.Set() {
		t.Page = canvas.Attr{FG: p.Backdrop, BG: p.Paper}
		t.Surface = canvas.Attr{FG: p.Backdrop, BG: p.Paper}

		for _, a := range []*canvas.Attr{
			&t.Chrome, &t.Header, &t.Number, &t.Digit, &t.Bad, &t.BadLive, &t.BinOn,
			&t.BinOff, &t.Footer, &t.Hint, &t.Swollen, &t.FileEdge, &t.Boot,
			&t.Hovered,
		} {
			a.BG = p.Paper
		}
	}
	return t
}

// dimEach returns a new slice with f applied to each colour, leaving the
// original alone. See Dimmed.
func dimEach(cs []canvas.Color, f func(canvas.Color) canvas.Color) []canvas.Color {
	out := make([]canvas.Color, len(cs))
	for i, c := range cs {
		out[i] = f(c)
	}
	return out
}

// On is an ad-hoc attribute in this theme: a foreground of the caller's
// choosing, on the theme's own paper. Use it for attributes no role covers,
// such as a nameplate, or they show the terminal's background on a pale theme.
func (t *Theme) On(fg canvas.Color, bold bool) canvas.Attr {
	return canvas.Attr{FG: fg, BG: t.Paper, Bold: bold}
}

// SignalColour is the link light's shade for a band: 0 good, 1 fair, 2 poor.
// It clamps the band: wrapping would report a bad link as good.
func (t *Theme) SignalColour(band int) canvas.Color {
	if len(t.Signal) == 0 {
		return t.Primary
	}
	if band < 0 {
		band = 0
	}
	if band >= len(t.Signal) {
		band = len(t.Signal) - 1
	}
	return t.Signal[band]
}

// DeskColour is the shade a seat's cursor and nameplate are drawn in. It is
// asked of the viewer's theme, since a Desk belongs to the shared board. The
// seat wraps, so a fifth refiner reuses the first colour.
func (t *Theme) DeskColour(seat int) canvas.Color {
	if len(t.Desks) == 0 {
		return t.Primary
	}
	return t.Desks[((seat%len(t.Desks))+len(t.Desks))%len(t.Desks)]
}

// DeskCursor is how a refiner's cursor and name tag are drawn: Ink on their
// seat colour, so the cursor, tag and plate match exactly. Bold, because on a
// sixteen-colour terminal bold selects the bright half of the pair.
func (t *Theme) DeskCursor(seat int) canvas.Attr {
	return canvas.Attr{FG: t.Ink, BG: t.DeskColour(seat), Bold: true}
}

// ScaryColour is the colour a woken cluster is drawn in, given how many seconds
// it has been awake: the theme's own primary, ramping to its own urgent.
func (t *Theme) ScaryColour(elapsed float64) canvas.Color {
	return BlendColors(t.Primary, t.Urgent, elapsed/ScaryRedSeconds)
}

// themes are the themes that ship. The first is the show's terminal and the
// default; then two old-CRT themes, then ports of common terminal schemes.
var themes = []*Theme{
	NewTheme(Palette{
		Name:     "lumon",
		Primary:  hex("#00FFFF"),
		Dim:      hex("#3A8A9A"),
		Live:     hex("#7DE3FF"),
		Glow:     hex("#CFF6FF"),
		Ink:      hex("#1A1A2E"),
		Peak:     hex("#FFFFFF"),
		Backdrop: hex("#1B4149"),
		Lurk:     hex("#215059"),
		Urgent:   hex("#FF3020"),
		Desks: []canvas.Color{
			hex("#7DE3FF"), // ice
			hex("#8AE6B0"), // green
			hex("#E8C97A"), // amber
			hex("#D89AE0"), // orchid
		},
	}),

	// P3 phosphor green.
	NewTheme(Palette{
		Name:     "phosphor",
		Primary:  hex("#33FF66"),
		Dim:      hex("#2E7D4F"),
		Live:     hex("#8CFFAE"),
		Glow:     hex("#D2FFE0"),
		Ink:      hex("#04160B"),
		Peak:     hex("#FFFFFF"),
		Backdrop: hex("#1B4429"),
		Lurk:     hex("#225232"),
		Urgent:   hex("#FF5A2B"),
		Desks: []canvas.Color{
			hex("#8CFFAE"),
			hex("#7DE3FF"),
			hex("#E8C97A"),
			hex("#D89AE0"),
		},
	}),

	// Amber CRT.
	NewTheme(Palette{
		Name:     "amber",
		Primary:  hex("#FFB000"),
		Dim:      hex("#8A6008"),
		Live:     hex("#FFD37A"),
		Glow:     hex("#FFEFC8"),
		Ink:      hex("#1A0F00"),
		Peak:     hex("#FFFFFF"),
		Backdrop: hex("#4A3208"),
		Lurk:     hex("#5A3D0A"),
		Urgent:   hex("#FF3020"),
		Desks: []canvas.Color{
			hex("#FFD37A"),
			hex("#8AE6B0"),
			hex("#7DE3FF"),
			hex("#D89AE0"),
		},
	}),

	NewTheme(Palette{
		Name:     "dracula",
		Primary:  hex("#BD93F9"),
		Dim:      hex("#6272A4"),
		Live:     hex("#8BE9FD"),
		Glow:     hex("#F8F8F2"),
		Ink:      hex("#282A36"),
		Peak:     hex("#FFFFFF"),
		Backdrop: hex("#44475A"),
		Lurk:     hex("#4E5168"),
		Urgent:   hex("#FF5555"),
		Desks: []canvas.Color{
			hex("#8BE9FD"),
			hex("#50FA7B"),
			hex("#FFB86C"),
			hex("#FF79C6"),
		},
	}),

	NewTheme(Palette{
		Name:     "nord",
		Primary:  hex("#88C0D0"),
		Dim:      hex("#4C566A"),
		Live:     hex("#8FBCBB"),
		Glow:     hex("#ECEFF4"),
		Ink:      hex("#2E3440"),
		Peak:     hex("#FFFFFF"),
		Backdrop: hex("#3B4252"),
		Lurk:     hex("#434C5E"),
		Urgent:   hex("#BF616A"),
		Desks: []canvas.Color{
			hex("#88C0D0"),
			hex("#A3BE8C"),
			hex("#EBCB8B"),
			hex("#B48EAD"),
		},
	}),

	NewTheme(Palette{
		Name:     "gruvbox",
		Primary:  hex("#83A598"),
		Dim:      hex("#665C54"),
		Live:     hex("#8EC07C"),
		Glow:     hex("#FBF1C7"),
		Ink:      hex("#282828"),
		Peak:     hex("#FFFFFF"),
		Backdrop: hex("#3C3836"),
		Lurk:     hex("#504945"),
		Urgent:   hex("#FB4934"),
		Desks: []canvas.Color{
			hex("#83A598"),
			hex("#B8BB26"),
			hex("#FABD2F"),
			hex("#D3869B"),
		},
	}),

	NewTheme(Palette{
		Name:     "catppuccin",
		Primary:  hex("#89B4FA"),
		Dim:      hex("#6C7086"),
		Live:     hex("#94E2D5"),
		Glow:     hex("#CDD6F4"),
		Ink:      hex("#1E1E2E"),
		Peak:     hex("#FFFFFF"),
		Backdrop: hex("#313244"),
		Lurk:     hex("#45475A"),
		Urgent:   hex("#F38BA8"),
		Desks: []canvas.Color{
			hex("#89B4FA"),
			hex("#A6E3A1"),
			hex("#F9E2AF"),
			hex("#CBA6F7"),
		},
	}),

	// Dark numbers on pale blue, as the show's terminal appears on screen, and
	// the only theme that names Paper. Roles keep their meaning: Backdrop and
	// Lurk recede, so they are lighter than the ink; Peak is the darkest
	// colour; Ink is the paper itself.
	NewTheme(Palette{
		Name:     "inverted",
		Paper:    hex("#D6E4F0"),
		Primary:  hex("#1B4C8C"),
		Dim:      hex("#6E8CB0"),
		Live:     hex("#0A2E63"),
		Glow:     hex("#08203F"),
		Ink:      hex("#D6E4F0"),
		Peak:     hex("#04142B"),
		Backdrop: hex("#8FA9C6"),
		Lurk:     hex("#89A3C0"),
		Urgent:   hex("#B3251A"),
		Desks: []canvas.Color{
			hex("#1B4C8C"),
			hex("#1F6B4A"),
			hex("#8A5A10"),
			hex("#6B2D75"),
		},
	}),

	// The publisher's house colours.
	NewTheme(Palette{
		Name:     "trove",
		Primary:  hex("#B9FFAC"),
		Dim:      hex("#4F7A48"),
		Live:     hex("#D7FFCF"),
		Glow:     hex("#EEFFEA"),
		Ink:      hex("#0B1409"),
		Peak:     hex("#FFFFFF"),
		Backdrop: hex("#2A4426"),
		Lurk:     hex("#33512E"),
		Urgent:   hex("#FF6B4A"),
		Desks: []canvas.Color{
			hex("#B9FFAC"),
			hex("#7DE3FF"),
			hex("#E8C97A"),
			hex("#D89AE0"),
		},
	}),
}

// DefaultTheme is the show's terminal, and what every session starts with.
var DefaultTheme = themes[0]

// Themes lists the themes that ship, in order.
func Themes() []*Theme { return themes }

// ThemeNames lists their names, in the same order.
func ThemeNames() []string {
	out := make([]string, 0, len(themes))
	for _, t := range themes {
		out = append(out, t.Name)
	}
	return out
}

// FindTheme looks a theme up by name, case-insensitively. The second result
// reports whether it was found, so a typo in a flag is not silently the
// default.
func FindTheme(name string) (*Theme, bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	for _, t := range themes {
		if t.Name == want {
			return t, true
		}
	}
	return nil, false
}

// NextTheme is the one after cur, wrapping. A nil or unknown theme gives the
// first.
func NextTheme(cur *Theme) *Theme {
	for i, t := range themes {
		if t == cur {
			return themes[(i+1)%len(themes)]
		}
	}
	return themes[0]
}

// Dimmed is this theme with every colour pulled toward Backdrop, for a board
// that is still arriving; level 0 is all Backdrop. Rebuilding through NewTheme
// dims everything drawn without each draw function knowing about arrivals.
// Paper is left alone. It costs a NewTheme per frame, so callers use it only
// while Model.arrival is set.
func (t *Theme) Dimmed(level float64) *Theme {
	p := t.Palette
	to := func(c canvas.Color) canvas.Color { return BlendColors(p.Backdrop, c, level) }

	p.Primary = to(p.Primary)
	p.Dim = to(p.Dim)
	p.Live = to(p.Live)
	p.Glow = to(p.Glow)
	p.Ink = to(p.Ink)
	p.Peak = to(p.Peak)
	p.Lurk = to(p.Lurk)
	p.Urgent = to(p.Urgent)

	// Clone the slices, never dim in place: copying the struct copies only
	// slice headers, and writing through them would permanently fade the
	// palette every session shares. TestEachRefinerGetsTheirOwnDesk guards it.
	p.Desks = dimEach(p.Desks, to)
	p.Tempers = dimEach(p.Tempers, to)
	p.Signal = dimEach(p.Signal, to)
	return NewTheme(p)
}
