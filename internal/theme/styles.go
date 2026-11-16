package theme

import (
	"github.com/terminaltrove/lumon/internal/canvas"
)

// The default theme's colours under short names, for tests. The renderer
// draws from each Model's own Theme, never from these.
var (
	Cyan       = DefaultTheme.Primary
	CyanFaded  = DefaultTheme.Dim
	BrightBlue = DefaultTheme.Live
	Glow       = DefaultTheme.Glow
	Dark       = DefaultTheme.Ink
	White      = DefaultTheme.Peak

	// The background grid sits well back so a waking cluster catches the eye.
	CyanBackdrop = DefaultTheme.Backdrop
	// A dormant corrupt number is a shade off ordinary, hidden until its
	// cluster activates.
	CyanLurk = DefaultTheme.Lurk
)

// Cell attributes of the default theme, for tests. A Model draws from
// m.Theme, never from these.
var (
	AttrChrome   = DefaultTheme.Chrome
	AttrHeader   = DefaultTheme.Header
	AttrNumber   = DefaultTheme.Number
	AttrDigit    = DefaultTheme.Digit
	AttrBad      = DefaultTheme.Bad
	AttrBadLive  = DefaultTheme.BadLive
	AttrCursor   = DefaultTheme.Cursor
	AttrBinOn    = DefaultTheme.BinOn
	AttrBinOff   = DefaultTheme.BinOff
	AttrFooter   = DefaultTheme.Footer
	AttrSelected = DefaultTheme.Selected
	AttrSwollen  = DefaultTheme.Swollen

	AttrFileFill     = DefaultTheme.FileFill
	AttrFileEdge     = DefaultTheme.FileEdge
	AttrFileFillText = DefaultTheme.FileFillText

	AttrBoot       = DefaultTheme.Boot
	AttrBootCursor = DefaultTheme.BootCursor
	AttrHovered    = DefaultTheme.Hovered
)

// The aliases must keep their canvas types.
var _ canvas.Color = Cyan
var _ canvas.Attr = AttrChrome
