package lumon

import (
	"math"

	"github.com/terminaltrove/lumon/internal/theme"
)

// What happens if you leave a cluster awake.
//
// Once its colour has saturated and it has sat red for FuseWait, a fuse
// lights: the cluster flashes white, faster as time runs out, holds white for
// the last FuseHold, and detonates, sending a shockwave through the digits and
// taking progress off the file bar.
//
// The clock is the room's, so every refiner agrees on when a cluster goes; the
// flash and blast are drawn per view.

const (
	// FuseWait is how long, in seconds, a cluster sits fully red before the
	// fuse lights, so the steady red reads as a warning distinct from the
	// countdown.
	FuseWait = 10.0

	// FuseSeconds is how long a cluster burns once the fuse is lit, in seconds.
	// The colour saturates at theme.ScaryRedSeconds, so the time from waking to
	// detonation is theme.ScaryRedSeconds + FuseWait + FuseSeconds (90, 10 and
	// 20).
	FuseSeconds = 20.0

	// FuseHold is how long, in seconds, the cluster is held solid white before
	// it goes. Without it the last flicker and the blast land on the same frame
	// and read as a glitch.
	FuseHold = 1.0

	// fuseSlow and fuseFast are the flash rates, in flashes a second, at the
	// start and end of the fuse. The rate ramps continuously between them.
	fuseSlow = 1.2
	fuseFast = 11.0
)

// Fusing reports whether a cluster is counting down, and how long it has left.
// It reads the same awake clock as the colour and swell, so there is no state
// to keep in step.
func (r *Room) Fusing(groupID int) (left float64, lit bool) {
	awake := r.Awake(groupID)
	if awake <= theme.ScaryRedSeconds+FuseWait {
		return 0, false
	}
	left = theme.ScaryRedSeconds + FuseWait + FuseSeconds - awake
	if left < 0 {
		left = 0
	}
	return left, true
}

// FuseFlash is how white a fusing cluster is drawn, from 0 (its own red) to 1,
// given the seconds left. The last FuseHold seconds are solid white. It depends
// only on the cluster's clock, so every refiner sees the same flicker.
func FuseFlash(left float64) float64 {
	if left <= FuseHold {
		return 1
	}

	// Phase is the integral of the rising rate. rate*time would re-time the
	// whole history at each new rate, and the flashing would stutter.
	burnt := FuseSeconds - left
	rate := fuseSlow + (fuseFast-fuseSlow)*burnt/FuseSeconds
	phase := (fuseSlow + rate) / 2 * burnt

	return 0.5 - 0.5*math.Cos(2*math.Pi*phase)
}

// detonated is every cluster whose fuse has run out, oldest first.
func (r *Room) detonated() []*BadGroup {
	var out []*BadGroup
	for _, c := range r.actives {
		if c.elapsed >= theme.ScaryRedSeconds+FuseWait+FuseSeconds {
			out = append(out, c.grp)
		}
	}
	return out
}

// Detonate destroys a cluster: it is taken off the board, its share of the
// work is written off, and one refinement's credit goes with it as a penalty.
//
// Completion is ΣRefined/ΣQuota. Removing only the quota would raise the bar;
// removing a credit as well gives (R-1)/(Q-1) < R/Q for every R < Q, so the
// bar always falls, and the file stays finishable.
//
// Clusters are not bound to bins, so the quota comes off the bin with the most
// room left (keeping shares even) and the credit off the bin with the most in
// it. The caller holds the lock.
func (r *Room) Detonate(grp *BadGroup, rng func(int) int) {
	if grp == nil || r.Grid.Group(grp.ID) == nil {
		return
	}

	// Reset both value and membership, as a refinement does. A stale
	// BadGroupID would draw the cell as dormant-corrupt for ever.
	for _, id := range grp.NumberIDs {
		r.Grid.Cells[id].Value = digitFor(r.FileName, rng(10))
		r.Grid.Cells[id].BadGroupID = -1
	}

	r.writeOff(grp.ID)
	r.blastSeq++
	r.blasts = append(r.blasts, newBlast(grp, r.blastTime, r.blastSeq))

	r.Sleep(grp.ID)
	r.Retire(grp)
}

// writeOff takes a lost cluster's share off the bin with the most room and a
// credit off the bin with the most in it. The caller holds the lock.
func (r *Room) writeOff(id int) {
	r.writeOffQuota()

	// The penalty, off whichever bin can pay it.
	pay, most := -1, 0
	for i := range r.Grid.Bins {
		if r.Grid.Bins[i].Refined > most {
			pay, most = i, r.Grid.Bins[i].Refined
		}
	}
	if pay >= 0 {
		r.Grid.Bins[pay].Refined--

		// And that refinement's quota share. Every bin keeps Quota == clusters
		// still out there + Refined; without this the file ends one short for
		// ever.
		r.writeOffQuota()
	}

	r.lost = append(r.lost, id)
}

// reconcileQuotas brings the bins back in line with the board: what they are
// waiting for must equal the clusters still out there. Restoring a save
// rebuilds quotas from the seed, so it calls this to drop the detonated ones.
func (r *Room) reconcileQuotas() {
	want := len(r.Grid.Groups)
	have := 0
	for i := range r.Grid.Bins {
		have += r.Grid.Bins[i].Quota - r.Grid.Bins[i].Refined
	}
	for ; have > want; have-- {
		if !r.writeOffQuota() {
			return
		}
	}
}

// writeOffQuota takes one cluster's share of the work off the bin with the
// furthest to go. It never takes a quota below what the bin already holds,
// since a bin holding more than its quota reads as over-full everywhere.
// Restoring a save reaches this through reconcileQuotas, without the penalty,
// because the docked credits are already in the saved bin counts. It reports
// whether there was anything left to write off.
func (r *Room) writeOffQuota() bool {
	room, at := -1, -1
	for i := range r.Grid.Bins {
		if n := r.Grid.Bins[i].Quota - r.Grid.Bins[i].Refined; n > room {
			room, at = n, i
		}
	}
	if at >= 0 && room > 0 {
		r.Grid.Bins[at].Quota--
		return true
	}
	return false
}
