# Third-party notices

## inertia

`internal/physics/` is a Go port of the solver core of [inertia](https://github.com/aclfe/inertia)
by Kairav Mittal, a terminal physics sandbox written in Rust. Ported pieces:

| lumon file | inertia source |
|---|---|
| `internal/physics/world.go` (Verlet integration, substep/iterate loop) | `src/cloth.rs`, `integrate`, `Cloth::step` |
| `internal/physics/constraint.go` (distance constraints, pinning) | `src/cloth.rs`, `build_constraints`, `solve_constraints` |
| `internal/physics/attract.go` (`BruteForce`, `PointAttractor`) | `src/nbody.rs`, `brute_force` |
| `internal/physics/clock.go` (fixed timestep, catch-up cap, wall-clock budget) | `src/app.rs`, `App::tick` |
| `internal/physics/fluid.go` (SPH: density, pressure, viscosity) | `src/fluid.rs`, `compute_density_pressure`, `compute_forces` |
| `internal/physics/fluid.go` (emitters) | `src/fluid.rs`, `Emitter`, `add_emitter`, `emit`, `push_particle`, `too_crowded` |

The emitter is a segment where upstream's is a disc, since in two dimensions the
plane of the disc is a line, and it round-robins between mouths where inertia
returns out of all of them at the cap. Its defining property is ported exactly:
there is no rate, and a mouth emits only as fast as its own output clears it.

The fluid is the solver core only, in two dimensions. inertia's `fluid.rs` also
has thermodynamics, emitters, and coupling to rigid bodies and to the cloth,
none of which is ported. The kernels' normalising constants differ because they
are normalised over an area here, not a volume.

`internal/physics/attract.go`'s `Seek` and `internal/physics/hash.go` are
original to lumon, not ported.

inertia is dual-licensed MIT OR Apache-2.0; both texts are alongside this file.
