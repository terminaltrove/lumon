// Package physics is a small 2D solver in which every motion in lumon is
// written.
//
// Each effect is an arrangement of the same primitives. A digit's size is a
// Scalar, a cluster's shake is a World of particles under Distance
// constraints, a digit flying to a bin is a Seek, and a bin's lid is an
// underdamped Scalar that overshoots and knocks shut.
//
// It depends only on the standard library, so it can be tested on its own.
// Most of it is a 2D port of inertia's 3D solver (github.com/aclfe/inertia,
// MIT OR Apache-2.0): Verlet integration and constraints (cloth.rs), n-body
// attraction (nbody.rs), SPH fluid (fluid.rs) and the fixed-timestep Clock
// (app.rs). Its numbers are kept exactly: Substeps 4, Iterations 12,
// DefaultDamping 0.99. See licenses/README.md for what came from where.
//
// Seek, Spin and Sparks are original to lumon: a critically damped pull toward
// a point, momentum with per-second drag for a flicked reel, and particles that
// are thrown, dragged, and then die. Sparks is the only one that removes
// anything, so its arrays are exported and its caller does the culling.
package physics
