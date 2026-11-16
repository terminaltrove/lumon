package lumon

// Deposits share a refinement across a room the way blast.go shares a blast:
// the board records which digits went to which bin, and every terminal flies
// them across its own screen. Nothing about the animation is exchanged.

// Deposit is one cluster's departure: which bin, which cells, and what they left
// with.
type Deposit struct {
	Bin int

	// IDs are the cells that emptied and Digits what each held, in the same
	// order. The digits must be recorded because refine has already overwritten
	// the cells by the time another view sees the deposit. Everything else a
	// flight needs (position, shake, size, target bin mouth) is the view's own.
	IDs    []int
	Digits []rune

	// At is the board's clock, shared with blasts, and Seq the order. A view
	// remembers the last Seq it flew, so pruning the list loses nothing.
	At  float64
	Seq int
}

// depositLife is how long a departure is remembered, in seconds: long enough
// for every view to have drawn a frame, as with blastLife.
const depositLife = 2.0

// RecordDeposit remembers a cluster's departure and returns its sequence. The
// caller holds the lock. The refiner uses the sequence to mark their own
// deposit as already flown, since replaying it would send every digit twice.
// That is why a deposit has no owner.
func (r *Room) RecordDeposit(bin int, ids []int, digits []rune) int {
	r.depositSeq++
	r.deposits = append(r.deposits, Deposit{
		Bin:    bin,
		IDs:    ids,
		Digits: digits,
		At:     r.blastTime,
		Seq:    r.depositSeq,
	})
	return r.depositSeq
}

// stepDeposits forgets departures every view has already flown. The caller
// holds the lock. It does not advance the clock: stepBlasts owns blastTime.
func (r *Room) stepDeposits() {
	if len(r.deposits) == 0 {
		return
	}
	kept := r.deposits[:0]
	for _, d := range r.deposits {
		if r.blastTime-d.At <= depositLife {
			kept = append(kept, d)
		}
	}
	r.deposits = kept
}
