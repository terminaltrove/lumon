package physics

import "math"

// Smoothed particle hydrodynamics: matter that pours. Ported from inertia's
// fluid.rs (see licenses/README.md), keeping only density, pressure and
// viscosity, in 2D. Each particle has a smoothing kernel of width H; summing
// neighbours' kernels gives a density, the excess over rest density gives a
// pressure, and the pressure gradient pushes particles apart.
//
// It uses forces, unlike the rest of this package: pressure has to cross a
// body of fluid within one frame, and a relaxation pass propagates one
// constraint at a time. ripple.go makes the same choice.

const (
	// DefaultH is the default smoothing radius, in the caller's units: how far
	// a particle's influence reaches. It decides how much room the particles
	// take up, so a caller fits a fluid to its container with NewFluidAt.
	DefaultH = 1.6

	// FluidGas is how hard the fluid resists compression at DefaultH. Too low
	// and it collapses; too high and the integrator overshoots and it
	// explodes.
	FluidGas = 140.0

	// FluidViscosity is how strongly a particle is dragged toward its
	// neighbours' average velocity. At zero the fluid behaves as a gas.
	FluidViscosity = 6.0

	// fluidSpacing is the lattice step the rest density is measured on, as a
	// fraction of the smoothing radius.
	fluidSpacing = 0.5

	// fluidFloor is the lowest measured density, as a fraction of rest
	// density. A lone particle's own kernel is tiny, and the force sum divides
	// by it. inertia uses the same.
	fluidFloor = 0.2

	// fluidMaxSpeed caps a particle's speed at DefaultH. SPH goes unstable if a
	// particle crosses more than a smoothing radius in a step. It bounds
	// distance per step as a fraction of h, so it must scale with the radius;
	// left flat it silently clamps a fast jet into a bubbling puddle.
	fluidMaxSpeed = 14.0
)

// Fluid is a body of particles that behave as a liquid. It keeps explicit
// velocities, since viscosity averages them; a Verlet World has none.
type Fluid struct {
	Pos []Vec2
	Vel []Vec2

	// Gravity is applied to every particle each step.
	Gravity Vec2

	// Viscosity is FluidViscosity by default. Raising it thickens the fluid.
	Viscosity float64

	// Gas is how hard the fluid resists compression, scaled with the
	// smoothing radius. Pressure acceleration goes as Gas/h (gradient 1/h^5,
	// (h-r)^2 gives h^2, density 1/h^2) while gravity does not scale, so a
	// flat FluidGas at a large radius collapses into a line on the floor.
	Gas float64

	// MaxSpeed is the speed clamp, scaled off the radius like Gas. See
	// fluidMaxSpeed.
	MaxSpeed float64

	density  []float64
	pressure []float64
	force    []Vec2

	grid hashGrid

	// poly6, spiky and visc normalise the density kernel, the pressure
	// kernel's gradient and the viscosity kernel in 2D. Pressure uses spiky
	// because poly6's gradient vanishes as two particles coincide, and the
	// fluid would clump.
	poly6, spiky, visc float64

	h, h2 float64
	rest  float64
}

// Spacing is the distance between particles in a body of this fluid at rest,
// for seeding a block.
func (f *Fluid) Spacing() float64 { return f.h * fluidSpacing }

// H is the smoothing radius this fluid was built with.
func (f *Fluid) H() float64 { return f.h }

// NewFluid returns an empty body of fluid with the default smoothing radius.
func NewFluid() *Fluid { return NewFluidAt(DefaultH) }

// NewFluidAt returns an empty body of fluid at a chosen smoothing radius. See
// DefaultH.
func NewFluidAt(h float64) *Fluid {
	if h <= 0 {
		h = DefaultH
	}
	f := &Fluid{
		Viscosity: FluidViscosity,
		Gas:       FluidGas * h / DefaultH,
		MaxSpeed:  fluidMaxSpeed * h / DefaultH,
		h:         h,
		h2:        h * h,
	}

	// 2D normalisations (in 3D: 315/(64*pi*h^9) and -45/(pi*h^6)).
	f.poly6 = 4 / (math.Pi * math.Pow(h, 8))
	f.spiky = -30 / (math.Pi * math.Pow(h, 5))
	f.visc = 40 / (math.Pi * math.Pow(h, 5))

	// The rest density is what the kernel sums to in the middle of a regular
	// lattice at Spacing, so it follows from the radius.
	step := f.Spacing()
	for x := -h; x <= h; x += step {
		for y := -h; y <= h; y += step {
			if r2 := x*x + y*y; r2 < f.h2 {
				d := f.h2 - r2
				f.rest += f.poly6 * d * d * d
			}
		}
	}
	return f
}

// Add puts a particle at a position, at rest.
func (f *Fluid) Add(at Vec2) { f.AddAt(at, Vec2{}) }

// AddAt puts a particle at a position with a velocity, keeping Pos and Vel
// parallel (inertia's push_particle).
func (f *Fluid) AddAt(at, vel Vec2) {
	f.Pos = append(f.Pos, at)
	f.Vel = append(f.Vel, vel)
}

// Len is how many particles there are.
func (f *Fluid) Len() int { return len(f.Pos) }

// RestDensity is what a particle in the body of the fluid measures.
func (f *Fluid) RestDensity() float64 { return f.rest }

// Density is what particle i measured on the last step.
func (f *Fluid) Density(i int) float64 {
	if i < 0 || i >= len(f.density) {
		return 0
	}
	return f.density[i]
}

// MaxParticles caps a body of fluid, since an emitter can add particles
// indefinitely. It is lower than inertia's 12,000 to fit a terminal's frame
// budget.
const MaxParticles = 4000

// Emitter is a mouth that fills the fluid, from inertia's fluid.rs. In 2D
// upstream's disc becomes a segment, such as a bin's throat.
type Emitter struct {
	// At is the middle of the mouth, Vel what a particle leaves with, and Reach
	// how far either side of At the mouth extends.
	At    Vec2
	Vel   Vec2
	Reach float64
}

// Emit fills every emitter's mouth with as many particles as will fit. There
// is no rate: crowded refuses a site too near an existing particle, so a jet
// emits exactly as fast as its output moves away, as in inertia. Call it once
// a frame, not once a substep, or it emits faster.
//
// It builds its own hash: before the first Step the one Step leaves is empty,
// nothing looks crowded, and the mouth fills the panel in one frame
// (TestAMouthEmitsOnlyAsFastAsItClears).
func (f *Fluid) Emit(mouths []Emitter) {
	if len(mouths) == 0 {
		return
	}
	f.grid.build(f.Pos, f.h)
	step := f.Spacing()

	// Round-robin over the mouths, so near the cap the first ones cannot
	// starve the rest.
	for k := 0; ; k++ {
		added := false
		for _, e := range mouths {
			if len(f.Pos) >= MaxParticles {
				return
			}
			n := int(e.Reach/step) + 1
			if k > 2*n {
				continue
			}
			// Out from the middle, so a mouth that fits one particle centres it.
			off := float64((k+1)/2) * step
			if k%2 == 1 {
				off = -off
			}
			if math.Abs(off) > e.Reach {
				continue
			}
			at := Vec2{X: e.At.X + off, Y: e.At.Y}
			if f.crowded(at) {
				continue
			}
			f.AddAt(at, e.Vel)
			added = true
		}
		if !added && k > 0 {
			return
		}
		if k > 64 {
			return // a mouth wider than any panel; stop instead of spinning
		}
	}
}

// crowded reports whether a candidate site is too near an existing particle.
// It is what throttles the jet.
func (f *Fluid) crowded(at Vec2) bool {
	min := spawnSpacing * f.Spacing()
	min2 := min * min
	near := false
	f.grid.near(at, func(j int) {
		if j < len(f.Pos) && f.Pos[j].Sub(at).LenSq() < min2 {
			near = true
		}
	})
	return near
}

// spawnSpacing is how close to an existing particle a new one may be born, as a
// fraction of the lattice spacing. inertia's MIN_SPAWN_SPACING.
const spawnSpacing = 0.75

// Step advances the fluid by dt seconds, keeping every particle inside
// bounds. The walls reflect velocity (Bounds is for a World), or the pressure
// solve keeps pushing a particle through.
func (f *Fluid) Step(dt float64, min, max Vec2, restitution float64) {
	n := len(f.Pos)
	if n == 0 || dt <= 0 {
		return
	}

	// The cell is the smoothing radius, so the nine-cell walk covers
	// everything a kernel can reach.
	f.grid.build(f.Pos, f.h)

	f.densities()
	f.forces()

	for i := range f.Pos {
		a := f.force[i]
		if d := f.density[i]; d > 0 {
			a = a.Scale(1 / d)
		}
		f.Vel[i] = f.Vel[i].Add(a.Add(f.Gravity).Scale(dt))

		if s := f.Vel[i].Len(); s > f.MaxSpeed {
			f.Vel[i] = f.Vel[i].Scale(f.MaxSpeed / s)
		}
		f.Pos[i] = f.Pos[i].Add(f.Vel[i].Scale(dt))
		f.wall(i, min, max, restitution)
	}
}

// densities measures how crowded every particle is, and what that costs it.
func (f *Fluid) densities() {
	n := len(f.Pos)
	f.density = resize(f.density, n)
	f.pressure = resize(f.pressure, n)

	// A particle always measures its own kernel at full strength.
	self := f.poly6 * f.h2 * f.h2 * f.h2

	for i := range f.Pos {
		pi := f.Pos[i]
		d := self
		f.grid.near(pi, func(j int) {
			if j == i {
				return
			}
			r2 := f.Pos[j].Sub(pi).LenSq()
			if r2 < f.h2 {
				k := f.h2 - r2
				d += f.poly6 * k * k * k
			}
		})

		if floor := f.rest * fluidFloor; d < floor {
			d = floor
		}
		f.density[i] = d

		// Never negative: a sparse particle is at the surface, and negative
		// pressure would suck the surface inward.
		if p := f.Gas * (d - f.rest); p > 0 {
			f.pressure[i] = p
		} else {
			f.pressure[i] = 0
		}
	}
}

// forces sums the pressure and viscosity acting on every particle.
func (f *Fluid) forces() {
	f.force = resizeVec(f.force, len(f.Pos))

	for i := range f.Pos {
		pi, vi := f.Pos[i], f.Vel[i]
		var acc Vec2

		f.grid.near(pi, func(j int) {
			if j == i {
				return
			}
			rij := pi.Sub(f.Pos[j])
			r := rij.Len()
			if r < 1e-9 || r >= f.h {
				return
			}
			dir := rij.Scale(1 / r)
			dj := f.density[j]
			if dj <= 0 {
				return
			}

			// Pressure: away from the neighbour, by the average of the two
			// pressures so the pair push equally and momentum is conserved.
			k := f.h - r
			acc = acc.Add(dir.Scale(-(f.pressure[i] + f.pressure[j]) / (2 * dj) * f.spiky * k * k))

			// Viscosity: toward the neighbour's velocity.
			acc = acc.Add(f.Vel[j].Sub(vi).Scale(f.Viscosity / dj * f.visc * k))
		})

		f.force[i] = acc
	}
}

// wall keeps one particle inside the box, reflecting its velocity.
func (f *Fluid) wall(i int, min, max Vec2, restitution float64) {
	p, v := f.Pos[i], f.Vel[i]
	if p.X < min.X {
		p.X, v.X = min.X, -v.X*restitution
	} else if p.X > max.X {
		p.X, v.X = max.X, -v.X*restitution
	}
	if p.Y < min.Y {
		p.Y, v.Y = min.Y, -v.Y*restitution
	} else if p.Y > max.Y {
		p.Y, v.Y = max.Y, -v.Y*restitution
	}
	f.Pos[i], f.Vel[i] = p, v
}

// resize returns a slice of length n, reusing the backing array.
func resize(s []float64, n int) []float64 {
	if cap(s) < n {
		return make([]float64, n)
	}
	return s[:n]
}

func resizeVec(s []Vec2, n int) []Vec2 {
	if cap(s) < n {
		return make([]Vec2, n)
	}
	return s[:n]
}
