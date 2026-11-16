package lumon

import "github.com/terminaltrove/lumon/internal/ease"

import "github.com/terminaltrove/lumon/internal/physics"

// The bins' own animation: how full each one is drawn, and which are being fed.
// It is per player: two refiners see the same deposit arrive at different
// moments because the digits cross different distances on their screens.

// binFillFrequency is how fast a bin's bar chases its true contents. A little
// slower than the file bar (fillFrequency), so a landing cluster has weight.
const binFillFrequency = 1.8

// LidOpen is how far each flap of a bin's lid swings, in degrees from shut
// (pointing inward along its half of the top edge). Past vertical, so the flaps
// splay into a V. Measured from the C++ reference's bin-open.png: flaps rise
// 67px against a 60px bin and travel 41px outward against a 200px width.
const LidOpen = 122.0

// LidSpan is each flap's length as a fraction of the bin's width, from the same
// sprite (about 78px of 200px). Less than half because the flaps are
// foreshortened.
const LidSpan = 0.39

// lidFrequency is how fast the lid swings, and lidDamping how much it rings.
// Fast enough to be open before the first digit arrives; underdamped so it
// overshoots on the way open and knocks once on the way shut.
const (
	// lidHold is how long a bin stays open when opened by hand, in seconds.
	// Pressing the key again shuts it sooner.
	lidHold = 12.0

	lidFrequency = 9.0
	lidDamping   = 0.55

	// lidLinger is how long, in seconds, a lid stays open after the last digit
	// is in, so a drawer raised by the bin's credit, which lands within a frame
	// or two of the digits, rises from a lid still open.
	lidLinger = 0.1
)

// BinFill is the drawn contents of each bin.
type BinFill struct {
	fill [BinCount]*physics.Scalar

	// lid is how far each bin is open, in degrees: 0 shut, 90 straight up.
	lid [BinCount]*physics.Scalar

	// receiving counts digits in the air toward each bin. A bin is lit while
	// anything is on its way.
	receiving [BinCount]int

	// linger is how much longer each lid stays open after its last digit.
	linger [BinCount]float64

	// held is how much longer each bin has been asked to stay open by hand, in
	// seconds. Step recomputes every lid's target each frame, so writing to
	// b.lid from outside would last one frame; held is OR'd into the target
	// instead, so manual and deposit openings compose.
	held [BinCount]float64

	// open is whether each lid was last set swinging open, and mine whether
	// that was this refiner's doing: by hand, or for their own digits.
	open, mine [BinCount]bool
	swings     []LidSwing
}

// LidSwing is a lid that started to swing open or shut on the last step.
type LidSwing struct {
	Bin        int
	Open, Mine bool
}

// NewBinFill returns bars that all start empty.
func NewBinFill() *BinFill {
	b := &BinFill{}
	for i := range b.fill {
		b.fill[i] = physics.NewScalar(0, binFillFrequency)
		b.lid[i] = physics.NewScalarDamped(0, lidFrequency, lidDamping)
	}
	return b
}

// Step eases every bar toward what its bin holds and recounts what is in the
// air. The flights are passed in because they belong to the view, not the
// board. drawerUp, if not nil, reports a bin's drawer up or on its way, which
// keeps the lid open until the drawer is back in.
func (b *BinFill) Step(dt float64, g *Grid, flights []Flight, drawerUp func(int) bool) {
	for i := range b.fill {
		var target float64
		if g != nil && g.Bins[i] != nil {
			target = g.Bins[i].Percentage()
		}
		b.fill[i].Step(dt, target)
	}

	b.receiving = [BinCount]int{}
	var mine [BinCount]bool
	for _, f := range flights {
		if f.BinIdx >= 0 && f.BinIdx < BinCount {
			b.receiving[f.BinIdx]++
			mine[f.BinIdx] = mine[f.BinIdx] || f.Mine
		}
	}

	// The lid opens the frame a cluster launches and shuts lidLinger after the
	// last digit is in, unless it is held or its drawer is up.
	b.swings = b.swings[:0]
	for i := range b.lid {
		if b.held[i] > 0 {
			b.held[i] -= dt
		}
		if b.receiving[i] > 0 {
			b.linger[i] = lidLinger
		} else if b.linger[i] > 0 {
			b.linger[i] -= dt
		}
		open := b.linger[i] > 0 || b.held[i] > 0 || (drawerUp != nil && drawerUp(i))
		if open != b.open[i] {
			if open {
				b.mine[i] = mine[i] || b.held[i] > 0
			}
			b.open[i] = open
			b.swings = append(b.swings, LidSwing{Bin: i, Open: open, Mine: b.mine[i]})
		}
		target := 0.0
		if open {
			target = LidOpen
		}
		b.lid[i].Step(dt, target)
	}
}

// Swings is every lid that started to swing open or shut on the last Step.
// Valid until the next.
func (b *BinFill) Swings() []LidSwing { return b.swings }

// Hold toggles bin i's lid being held open. Only the lid: the drawer stays
// shut while anything is Receiving.
func (b *BinFill) Hold(i int) { b.SetHold(i, !b.Holding(i)) }

// SetHold sets whether bin i is held open, for callers (FOUNTAIN, FIREWORKS)
// that state the open set every frame. Opening re-arms the lidHold countdown,
// so a caller asking every frame keeps the lid up indefinitely.
func (b *BinFill) SetHold(i int, open bool) {
	if i < 0 || i >= BinCount {
		return
	}
	if !open {
		b.held[i] = 0
		return
	}
	b.held[i] = lidHold
}

// Holding reports whether bin i is being held open by hand.
func (b *BinFill) Holding(i int) bool {
	return i >= 0 && i < BinCount && b.held[i] > 0
}

// Mine reports whether bin i's lid last opened for this refiner: by hand, or
// for their own digits.
func (b *BinFill) Mine(i int) bool { return i >= 0 && i < BinCount && b.mine[i] }

// Lid is how far bin i is open, in degrees.
func (b *BinFill) Lid(i int) float64 {
	if i < 0 || i >= BinCount {
		return 0
	}
	// The spring can ring below shut; a lid cannot open into the bin.
	if v := b.lid[i].Value(); v > 0 {
		return v
	}
	return 0
}

// Ajar reports whether bin i is open far enough to draw as open.
func (b *BinFill) Ajar(i int) bool { return b.Lid(i) > 1.5 }

// Fill is how full bin i is drawn, in [0,1].
func (b *BinFill) Fill(i int) float64 {
	if i < 0 || i >= BinCount {
		return 0
	}
	return ease.Clamp01(b.fill[i].Value())
}

// Receiving reports whether digits are on their way to bin i.
func (b *BinFill) Receiving(i int) bool {
	return i >= 0 && i < BinCount && b.receiving[i] > 0
}

// Reset empties every bar at once, for a view that has moved to a new file, so
// the bins do not ease down as though the work were being undone.
func (b *BinFill) Reset() {
	for i := range b.fill {
		b.fill[i] = physics.NewScalar(0, binFillFrequency)
		b.lid[i] = physics.NewScalarDamped(0, lidFrequency, lidDamping)
	}
	b.receiving, b.linger = [BinCount]int{}, [BinCount]float64{}
	b.open, b.swings = [BinCount]bool{}, b.swings[:0]
}
