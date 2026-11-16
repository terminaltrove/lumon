package lumon

import "github.com/terminaltrove/lumon/internal/ease"

import "github.com/terminaltrove/lumon/internal/physics"

// The drawer: a panel that rises out of a bin each time the bin reaches a
// quarter of its quota (25, 50, 75 and 100%), showing the four tempers it
// holds (Woe, Frolic, Dread and Malice) as one bar each. As in the show, all
// four bars blink, and it goes back down before the lid shuts. The goal is to fill every bin evenly with all four, so these bars are
// the only readout of what has been refined, as opposed to how much.
//
// The C++ reference has no such panel; the name "bin drawer" comes from one of
// the recreations. The counts are the board's and the motion is per-player, as
// with BinFill.

// Temper is which of the four a cluster is. The show abbreviates Frolic as FC,
// and so does this.
type Temper int

const (
	Woe Temper = iota
	Frolic
	Dread
	Malice

	// TemperCount is how many tempers there are. It is fixed by the show.
	TemperCount = 4
)

// temperNames are the column labels.
var temperNames = [TemperCount]string{"WO", "FC", "DR", "MA"}

// TemperOf is which temper a cluster is. It is derived from the file and the
// group, so refiners at one board agree without exchanging anything and the
// save need not store it.
//
// It is a hash, not Perlin noise: noise is smooth, so neighbouring clusters
// would share a temper and the bars would fill one at a time.
func TemperOf(seed int64, epoch, groupID int) Temper {
	h := uint64(seed)*0x9E3779B97F4A7C15 +
		uint64(int64(groupID))*0xBF58476D1CE4E5B9 +
		uint64(int64(epoch))*0xA24BAED4963EE407
	h ^= h >> 33
	h *= 0xFF51AFD7ED558CCD
	h ^= h >> 29
	return Temper(h % TemperCount)
}

const (
	// drawerFrequency is how fast the drawer travels, and drawerDamping how much
	// it rings. The damping is just under critical, so it settles with one
	// small overshoot.
	drawerFrequency = 4.5
	drawerDamping   = 0.8

	// A bin reaching a quarter raises its drawer, which rises and grows its
	// bars for drawerRise, blinks its bars drawerBlinks times, each off and
	// then on for drawerBlink (a blink is the 0.42s of its sound), holds for
	// drawerAfter, and goes down. Seconds.
	drawerRise   = 0.9
	drawerBlink  = 0.21
	drawerBlinks = 2
	drawerAfter  = 0.6

	// temperFrequency is how fast a temper's bar grows to what the bin holds.
	temperFrequency = 2.5

	// drawerOpen is how far out, as a fraction of its travel, the drawer must be
	// before it draws. Like BinFill.Ajar, one threshold decides both whether it
	// draws and whether it takes rows, so the two cannot disagree.
	drawerOpen = 0.02
)

// drawerUp is how long, in seconds, a drawer is held up for one quarter.
const drawerUp = drawerRise + 2*drawerBlink*drawerBlinks + drawerAfter

// Drawer is how far each bin's drawer has slid out, from 0 shut to 1 fully open.
type Drawer struct {
	out [BinCount]*physics.Scalar

	// up is how long each drawer has been up for its quarter, in seconds, or
	// -1 while it is not.
	up [BinCount]float64

	// percent is what each bin held at the last Step, and quarters how many
	// quarters of its quota it had reached.
	percent, quarters [BinCount]int

	// share is each bar as drawn, from 0 empty to 1 full, easing toward what
	// the bin holds.
	share  [BinCount][TemperCount]*physics.Scalar
	primed bool // whether a board has been seen yet

	// rose and blinked are the bins whose percent rose and whose drawer
	// started a blink on the last Step.
	rose, blinked []int
}

// NewDrawer returns drawers that all start shut.
func NewDrawer() *Drawer {
	d := &Drawer{}
	for i := range d.out {
		d.out[i] = physics.NewScalarDamped(0, drawerFrequency, drawerDamping)
		d.up[i] = -1
		for t := range d.share[i] {
			d.share[i][t] = physics.NewScalar(0, temperFrequency)
		}
	}
	return d
}

// Step raises the drawer of every bin that has just reached another quarter,
// if raise allows, runs each through its blinks and back down, and eases each
// bar toward what g's bin holds. What a board held when first seen raises
// nothing.
func (d *Drawer) Step(dt float64, g *Grid, raise bool) {
	d.rose, d.blinked = d.rose[:0], d.blinked[:0]
	for i := range d.out {
		percent := 0
		if g != nil && g.Bins[i] != nil {
			percent = g.Bins[i].PercentInt()
		}
		quarters := percent / 25
		if d.primed && percent > d.percent[i] {
			d.rose = append(d.rose, i)
		}
		if d.primed && raise && quarters > d.quarters[i] {
			d.up[i] = 0
		}
		d.percent[i], d.quarters[i] = percent, quarters

		if d.up[i] >= 0 {
			was := d.up[i]
			d.up[i] += dt
			if blinkAt(d.up[i]) != blinkAt(was) && blinkAt(d.up[i]) >= 0 {
				d.blinked = append(d.blinked, i)
			}
			if d.up[i] >= drawerUp {
				d.up[i] = -1
			}
		}
		target := 0.0
		if d.up[i] >= 0 {
			target = 1
		}
		d.out[i].Step(dt, target)

		for t, s := range d.share[i] {
			want := temperShare(g, i, Temper(t))
			if !d.primed {
				s.Set(want)
			}
			s.Step(dt, want)
		}
	}
	d.primed = g != nil
}

// blinkAt is which blink a drawer up this long is in, from 0, or -1 if none.
func blinkAt(up float64) int {
	if up < drawerRise {
		return -1
	}
	if n := int((up - drawerRise) / (2 * drawerBlink)); n < drawerBlinks {
		return n
	}
	return -1
}

// Rose is every bin whose percent rose on the last Step, and Blinked every bin
// whose drawer started a blink. Valid until the next.
func (d *Drawer) Rose() []int    { return d.rose }
func (d *Drawer) Blinked() []int { return d.blinked }

// Up reports whether bin i's drawer is up for a quarter or not yet back in,
// which keeps its lid open.
func (d *Drawer) Up(i int) bool {
	return i >= 0 && i < BinCount && (d.up[i] >= 0 || d.Showing(i))
}

// Dark reports whether bin i's bars are in the off half of a blink.
func (d *Drawer) Dark(i int) bool {
	if i < 0 || i >= BinCount || d.up[i] < drawerRise {
		return false
	}
	n := int((d.up[i] - drawerRise) / drawerBlink)
	return n < 2*drawerBlinks && n%2 == 0
}

// Share is how much of its share of bin i temper t's bar is drawn holding, in
// [0,1].
func (d *Drawer) Share(i int, t Temper) float64 {
	if i < 0 || i >= BinCount || t < 0 || t >= TemperCount {
		return 0
	}
	return ease.Clamp01(d.share[i][t].Value())
}

// temperShare is how much of its share of bin i (Bin.TemperShare) temper t
// fills. A full bin's are all full, whatever a save or a shrunk quota left.
func temperShare(g *Grid, i int, t Temper) float64 {
	if g == nil || g.Bins[i] == nil || g.Bins[i].Quota <= 0 {
		return 0
	}
	bin := g.Bins[i]
	if bin.Refined >= bin.Quota {
		return 1
	}
	share := bin.TemperShare(t)
	if share <= 0 {
		return 0
	}
	return ease.Clamp01(float64(bin.Tempers[t]) / float64(share))
}

// Out is how far bin i's drawer has slid, clamped to 0..1 (the spring itself
// overshoots).
func (d *Drawer) Out(i int) float64 {
	if i < 0 || i >= BinCount {
		return 0
	}
	return ease.Clamp01(d.out[i].Value())
}

// Showing reports whether bin i's drawer is far enough out to draw.
func (d *Drawer) Showing(i int) bool { return d.Out(i) > drawerOpen }
