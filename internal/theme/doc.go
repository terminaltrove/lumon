// Package theme resolves a palette of roles into the attributes the renderer
// draws with.
//
// A Palette names roles (primary, dim, live, glow, ink, peak, backdrop, lurk,
// urgent, a shade per desk and one per temper), and NewTheme turns them into
// canvas.Attr values. Draw sites ask for a role, so a new palette restyles the
// screen without touching them.
//
// A theme belongs to one terminal, never to the process. Remotely every
// session shares this package, so a palette in a package variable would let
// one refiner pressing `t` restyle everybody. Themes are values held on each
// Model, and everything on screen, including other refiners' cursors and
// nameplates, is drawn in the viewer's palette.
//
// Nothing here knows about a grid, a room or a Model. How long a cluster takes
// to redden belongs to the board; the ramp it reddens along is here.
package theme
