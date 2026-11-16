package lumon

// The board records only where and when a detonation happened. Each view turns
// it into a wave in its own screen space (see ripple.go), as with flights.

// Blast is one detonation: where, by the board's clock, and in what order.
type Blast struct {
	X, Y float64
	At   float64

	// Seq counts detonations on this board, so a view can tell which ones it has
	// already reacted to. The list is pruned, so position in it means nothing.
	Seq int
}

// blastLife is how long a detonation is remembered, in seconds: long enough for
// every view to have drawn a frame and picked it up. The wave outlives it.
const blastLife = 2.0

// newBlast records a detonation at a cluster's centre.
func newBlast(grp *BadGroup, at float64, seq int) Blast {
	var sx, sy float64
	for _, id := range grp.NumberIDs {
		sx += float64(id % GridSize)
		sy += float64(id / GridSize)
	}
	n := float64(len(grp.NumberIDs))
	if n == 0 {
		return Blast{At: at, Seq: seq}
	}
	return Blast{X: sx / n, Y: sy / n, At: at, Seq: seq}
}

// stepBlasts advances the board's clock and forgets cold detonations. The
// caller holds the lock.
func (r *Room) stepBlasts(dt float64) {
	r.blastTime += dt
	if len(r.blasts) == 0 {
		return
	}
	kept := r.blasts[:0]
	for _, b := range r.blasts {
		if r.blastTime-b.At <= blastLife {
			kept = append(kept, b)
		}
	}
	r.blasts = kept
}
