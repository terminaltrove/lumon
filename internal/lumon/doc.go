// Package lumon is the Macrodata Refinement terminal: the board, one player's
// view of it, and everything that happens between them.
//
// # Room and Model
//
// Room is the board: the grid, the bins, the awake clusters, the desks and the
// file being refined. Up to MaxDesks refiners share one. Model is one
// terminal's view of a Room and holds the animation state that belongs to that
// player alone: the lens, the flights, the agitation, the drawn size of every
// digit.
//
// Model embeds *Room, so view code reads the board by promotion (m.Grid,
// m.Actives(), m.Awake(id)). View code may read the board, but only Room's own
// methods may change it. Go reports an ambiguity only when names collide at the
// same depth, so a method declared on both would silently shadow the other;
// TestModelAndRoomHaveNoNamesInCommon guards against that.
//
// They share a package because separating them would mean exporting the seam
// or duplicating the test harness.
//
// # Other packages
//
// Code belongs outside this package if it can be described without mentioning
// a board or a screen:
//
//   - noise decides which numbers are corrupt.
//   - glyph draws a digit as strokes, rasterised to coverage and quantised to
//     characters.
//   - theme resolves a palette of roles into attributes.
//   - ease has the easing helpers the board and the renderer share.
//   - canvas and physics are the cell buffer and the solver.
//
// The diversions stay here because their tests reach into many unexported
// fields; moving them would mean exporting a simulation's velocities and worlds.
//
// # Frames
//
// Update holds the board's write lock for its whole call and View holds the
// read lock, so a frame is always drawn against the board it was just simulated
// from. physics.Clock turns elapsed time into a whole number of fixed 1/60s
// steps, bounded so a heavy frame slows down instead of freezing input.
// Behaviour does not depend on how fast a terminal draws.
package lumon
