package lumon

import (
	"github.com/terminaltrove/lumon/internal/ease"
	"github.com/terminaltrove/lumon/internal/noise"
	"math/rand"
	"sort"

	"github.com/terminaltrove/lumon/internal/physics"
)

// Agitation makes an active corrupt cluster shake.
//
// A cluster is one spring-mass body: a particle per digit, bound to its
// neighbours by distance constraints and tethered to its home cell, so the
// cluster wobbles together instead of each digit jittering alone.
//
// All values are in cell units: 1.0 is one grid cell. The renderer scales that
// to columns and rows.
const (
	// MaxAgitationOffset bounds how far a fully escalated digit can stray from
	// its cell, so a thrashing cluster stays legible.
	MaxAgitationOffset = 0.85

	// calmAgitationOffset is the bound for a freshly woken cluster. The limit
	// ramps to MaxAgitationOffset as the cluster is left unrefined; the clamp,
	// not the drive, sets how wide the shake looks.
	calmAgitationOffset = 0.45

	// homeFrequency is the tether pulling a digit back to its cell, in rad/s.
	// Keep it near the drive's frequency: a stiff tether is a low-pass filter
	// and attenuates a faster drive to nothing (as at 7.0).
	homeFrequency = 4.0

	// linkStiffness binds neighbouring digits. Below 1 so the cluster flexes.
	linkStiffness = 0.35

	// shakeAccel is the peak driving acceleration at full intensity.
	shakeAccel = 90.0

	// driveSpeed is how fast the cluster moves through the noise field, in
	// noise units per second. The drive must be correlated in time: a random
	// force each frame is white noise that the tether averages away. LumonMDR
	// drifts its numbers with Perlin noise the same way (NumbersPanel.cpp).
	driveSpeed = 0.5

	// superSpeedBoost is how much faster the noise is traversed at full
	// escalation.
	superSpeedBoost = 1.2

	// shakeEscalation is how much harder a fully escalated cluster is driven.
	// Intensity is clamped to 1 because it also means "how awake", so growth
	// beyond that comes from here.
	shakeEscalation = 2.2
)

type agitatedGroup struct {
	ids       []int // grid IDs, parallel to particles
	particles []int // physics.World indices
	home      []physics.Vec2
	intensity float64

	// escalation is how long the cluster has been left unrefined, 0 to 1. It
	// widens the shake on top of intensity.
	escalation float64
}

// Agitation owns the simulation for every tracked cluster.
type Agitation struct {
	world  *physics.World
	groups map[int]*agitatedGroup
	byID   map[int]int // grid ID -> particle index
	column map[int]int // grid ID -> committed column, for hysteresis
	row    map[int]int // grid ID -> committed row, likewise
	rng    *rand.Rand

	// forces is reused across frames so stepping allocates nothing.
	forces []physics.Vec2

	// t is the position along the noise field's time axis.
	t float64
}

// NewAgitation returns an empty Agitation. The shake is determined by the
// noise field alone; rng is kept for callers that want extra jitter.
func NewAgitation(rng *rand.Rand) *Agitation {
	return &Agitation{
		world:  physics.NewWorld(),
		groups: make(map[int]*agitatedGroup),
		byID:   make(map[int]int),
		column: make(map[int]int),
		row:    make(map[int]int),
		rng:    rng,
	}
}

// Track builds a body for a cluster. Tracking an already-tracked cluster is a
// no-op, so callers can call it every frame without accumulating duplicates.
func (a *Agitation) Track(grp *BadGroup, g *Grid) {
	if _, ok := a.groups[grp.ID]; ok {
		return
	}

	ag := &agitatedGroup{}
	for _, id := range grp.NumberIDs {
		x, y := id%GridSize, id/GridSize
		home := physics.Vec2{X: float64(x), Y: float64(y)}
		p := a.world.Add(home, 1)
		ag.ids = append(ag.ids, id)
		ag.particles = append(ag.particles, p)
		ag.home = append(ag.home, home)
		a.byID[id] = p
	}

	a.link(ag)

	a.groups[grp.ID] = ag
}

// Release forgets a cluster, dropping its digits back to their cells.
//
// The world is rebuilt so the cluster's particles and constraints are removed.
// Deleting only the bookkeeping leaves them integrated every frame, invisible
// and growing with every refine.
func (a *Agitation) Release(groupID int) {
	ag, ok := a.groups[groupID]
	if !ok {
		return
	}
	for _, id := range ag.ids {
		delete(a.byID, id)
		delete(a.column, id)
		delete(a.row, id)
	}
	delete(a.groups, groupID)
	a.rebuild()
}

// rebuild reconstructs the world from the groups still tracked. Particle
// indices change, so byID is rewritten. Position and velocity are kept, so a
// surviving cluster does not twitch.
func (a *Agitation) rebuild() {
	type state struct {
		pos physics.Vec2
		vel physics.Vec2
	}
	saved := make(map[int]state, len(a.byID))
	for _, ag := range a.groups {
		for _, p := range ag.particles {
			saved[p] = state{pos: a.world.Pos(p), vel: a.world.Velocity(p, physics.DT)}
		}
	}

	a.world = physics.NewWorld()
	clear(a.byID)

	for _, ag := range a.groups {
		for i, old := range ag.particles {
			s := saved[old]
			p := a.world.Add(s.pos, 1)
			a.world.SetVelocity(p, s.vel)
			ag.particles[i] = p
			a.byID[ag.ids[i]] = p
		}
		a.link(ag)
	}
}

// link binds each pair of neighbouring digits in a cluster, so it deforms as
// one body.
func (a *Agitation) link(ag *agitatedGroup) {
	for i, idA := range ag.ids {
		ax, ay := idA%GridSize, idA/GridSize
		for j := i + 1; j < len(ag.ids); j++ {
			idB := ag.ids[j]
			bx, by := idB%GridSize, idB/GridSize
			if abs(bx-ax) > 1 || abs(by-ay) > 1 {
				continue
			}
			rest := ag.home[i].Sub(ag.home[j]).Len()
			a.world.AddConstraint(physics.NewDistance(ag.particles[i], ag.particles[j], rest, linkStiffness))
		}
	}
}

// TrackedIDs lists every cluster with a body, sorted so callers reconcile in a
// deterministic order.
func (a *Agitation) TrackedIDs() []int {
	out := make([]int, 0, len(a.groups))
	for id := range a.groups {
		out = append(out, id)
	}
	sort.Ints(out)
	return out
}

// Tracked reports whether a cluster currently has a body.
func (a *Agitation) Tracked(groupID int) bool {
	_, ok := a.groups[groupID]
	return ok
}

// SetIntensity sets how hard a cluster shakes: 0 is dormant, 1 is a hovered
// "super active" cluster.
func (a *Agitation) SetIntensity(groupID int, i float64) {
	if ag, ok := a.groups[groupID]; ok {
		ag.intensity = ease.Clamp01(i)
	}
}

// SetEscalation sets how far a cluster has escalated, 0 to 1, which widens both
// the drive and the bound on how far a digit may stray.
func (a *Agitation) SetEscalation(groupID int, e float64) {
	if ag, ok := a.groups[groupID]; ok {
		ag.escalation = ease.Clamp01(e)
	}
}

// Step advances every tracked cluster by dt, in seconds.
//
// The order is forces, integrate, solve constraints, then clamp to cells.
// Clamping before solving lets the solver undo it; clamping every pass fights
// the constraints and collapses the cluster.
func (a *Agitation) Step(dt float64) {
	n := a.world.Len()
	if cap(a.forces) < n {
		a.forces = make([]physics.Vec2, n)
	}
	a.forces = a.forces[:n]
	for i := range a.forces {
		a.forces[i] = physics.Vec2{}
	}

	for _, ag := range a.groups {
		for i, p := range ag.particles {
			pos := a.world.Pos(p)
			vel := a.world.Velocity(p, physics.DT)

			// The tether returns a calmed cluster home.
			f := physics.NewSeek(ag.home[i], homeFrequency).Force(pos, vel)

			if ag.intensity > 0 {
				k := ag.intensity * shakeAccel * (1 + ag.escalation*(shakeEscalation-1))
				// Sampling at the digit's own coordinates drives neighbours in
				// related but different directions.
				hx, hy := ag.home[i].X, ag.home[i].Y
				f = f.Add(physics.Vec2{
					X: noise.PerlinAnimation.Noise3D(hx, hy, a.t) * k,
					Y: noise.PerlinAnimation.Noise3D(hx+31.7, hy+17.3, a.t) * k * 0.5,
				})
			}
			a.forces[p] = f
		}
	}

	// The noise axis speeds up with the most escalated cluster, never with
	// hover. Hover is per player, so driving the phase from it would put one
	// player's copy out of phase with everyone else's. Hover scales only the
	// amplitude, in the force above.
	peak := 0.0
	for _, ag := range a.groups {
		if ag.escalation > peak {
			peak = ag.escalation
		}
	}
	a.t += dt * driveSpeed * (1 + peak*superSpeedBoost)

	a.world.IntegrateForces(dt, a.forces)
	a.world.SolveN(physics.Iterations)

	for _, ag := range a.groups {
		limit := calmAgitationOffset + ag.escalation*(MaxAgitationOffset-calmAgitationOffset)
		for i, p := range ag.particles {
			a.clampToCell(p, ag.home[i], limit)
		}
	}
}

// clampToCell keeps a digit within limit of its cell. The tether usually does
// this; the clamp guarantees a digit never wanders into a neighbouring column.
func (a *Agitation) clampToCell(p int, home physics.Vec2, limit float64) {
	d := a.world.Pos(p).Sub(home)
	if l := d.Len(); l > limit {
		a.world.SetPos(p, home.Add(d.Scale(limit/l)))
	}
}

// Offset returns a digit's displacement from its cell, in cell units. Untracked
// digits sit exactly on their cell.
func (a *Agitation) Offset(gridID int) (dx, dy float64) {
	p, ok := a.byID[gridID]
	if !ok {
		return 0, 0
	}
	x, y := gridID%GridSize, gridID/GridSize
	d := a.world.Pos(p).Sub(physics.Vec2{X: float64(x), Y: float64(y)})
	return d.X, d.Y
}

// ColumnOffset returns how many whole columns a digit is displaced by, with the
// same hysteresis as the background drift so it does not stutter.
func (a *Agitation) ColumnOffset(l *Layout, gridID int) int {
	dx, _ := a.Offset(gridID)
	if dx == 0 {
		if _, tracked := a.byID[gridID]; !tracked {
			return 0
		}
	}
	cols, _ := l.Screen(dx, 0)
	a.column[gridID] = columnStep(cols, a.column[gridID])
	return a.column[gridID]
}

// RowOffset returns how many whole rows a digit is displaced by. One cell unit
// of vertical travel is a pitch of rows.
//
// The layout is passed in, not stored: Agitation is stepped from stepBoard on
// the board's clock so players stay in phase, while spacing belongs to one
// terminal.
func (a *Agitation) RowOffset(l *Layout, gridID int) int {
	_, dy := a.Offset(gridID)
	if dy == 0 {
		if _, tracked := a.byID[gridID]; !tracked {
			return 0
		}
	}
	_, rows := l.Screen(0, dy)
	a.row[gridID] = rowStep(rows, a.row[gridID], l.MaxRowSag())
	return a.row[gridID]
}

// ResetSteps forgets the whole-cell offsets every digit has committed to. Call
// it after the board is respaced: the offsets are in screen columns and rows,
// keyed by grid ID, and would otherwise survive a resize at the old width.
func (a *Agitation) ResetSteps() {
	clear(a.column)
	clear(a.row)
}

// Velocity returns a digit's current motion, in cell units per second, so a
// refined digit keeps its momentum on the way to its bin.
func (a *Agitation) Velocity(gridID int) physics.Vec2 {
	p, ok := a.byID[gridID]
	if !ok {
		return physics.Vec2{}
	}
	return a.world.Velocity(p, physics.DT)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
