package lumon

import (
	"github.com/terminaltrove/lumon/internal/ease"
	"math"
	"sort"

	"github.com/terminaltrove/lumon/internal/physics"
)

// The drawer of macrodata files, and turning through it.
//
// One card shows at a time, with the rest as dividers down its flanks. Turning
// the reel flips the card away and brings the next one up. The whole reel is
// one physics.Spin holding a fractional card index, so it coasts with
// momentum; each card's pose follows from its index against that one number.

const (
	// reelDrag is the fraction of the drawer's speed left after a second
	// (five per cent): a flick carries a few cards and a held key winds it up.
	reelDrag = 0.05

	// reelKick is what one press adds to the speed, in cards a second. Pushes
	// accumulate, so a held key keeps adding while the drawer coasts.
	reelKick = 6.0

	// reelTop is as fast as the drawer will go, in cards a second. Faster, the
	// cards flip quicker than the eye resolves.
	reelTop = 26.0

	// reelDetent is the speed, in cards a second, below which the drawer is
	// pulled to the nearest card, and reelSnap the stiffness of that pull (ω²,
	// per second squared). Without a detent the drawer can stop between two
	// cards; with one always on, it could not coast.
	reelDetent = 4.0
	reelSnap   = 120.0

	// reelArrived is how near a card the drawer has to be before it counts as
	// settled, in cards.
	reelArrived = 0.01
	reelStill   = 0.05

	// reelSeek is the stiffness (ω², per second squared) of the critically
	// damped pull toward a card the drawer has been sent to. Softer than
	// reelSnap, so the journey is watched.
	//
	// A critically damped spring takes about the same time whatever the
	// distance, so a long jump would pass through the cards very fast; reelTop
	// clips that, giving a run at full speed and a deceleration over the last
	// few cards. The value is tuned by eye against reelDrag, which also slows
	// the journey: it crosses the drawer in about a second.
	reelSeek = 90.0

	// liftFrequency is how fast a card presented to you rises out of the drawer,
	// and settles back into it when you take the wheel yourself.
	liftFrequency = 6.5

	// A riffle, as the show's drawer does as it comes up, runs at a speed set
	// by how many cards it has left, in cards a second: up to riffleTop, slowing
	// by riffleFall a second for every card nearer, until the last three, which
	// go one at a time at three, then two, then one riffleLast, the last card
	// easing to riffleRest of that as it settles.
	riffleTop  = 60.0
	riffleFall = 3.0
	riffleLast = 1.25
	riffleRest = 0.4
)

// riffle is the drawer running onto a card on its own: fast at first, slowing
// onto the card.
type riffle struct{ to float64 }

// riffleSpeed is how fast a riffle runs with left cards still to go.
func riffleSpeed(left float64) float64 {
	switch {
	case left >= 3:
		return math.Min(riffleTop, 3*riffleLast+(left-3)*riffleFall)
	case left >= 2:
		return 3 * riffleLast
	case left >= 1:
		return 2 * riffleLast
	}
	return riffleLast * (riffleRest + (1-riffleRest)*left)
}

// Rolodex is the file selector: the directory, and where in it the reel is.
type Rolodex struct {
	Fade

	files []Entry

	// at is the drawer's position in cards, fractional while it is turning.
	// It has momentum and no target; the selected card is read off it.
	at *physics.Spin

	// seekTo is the card the drawer has been sent to, or -1. See Seek.
	seekTo int

	// riffling is the riffle the drawer is running, or nil. See Riffle.
	riffling *riffle

	// lift is how far the card is standing out of the drawer, and presenting
	// whether it should be. A card the drawer was sent to (Seek) stands out;
	// it settles back as soon as the player turns the drawer.
	lift       *physics.Scalar
	presenting bool
}

// NewRolodex opens the drawer at a given card.
func NewRolodex(files []Entry, at int) *Rolodex {
	at = clampInt(at, 0, maxi(len(files)-1, 0))
	return &Rolodex{
		Fade:   NewFade(),
		files:  files,
		at:     physics.NewSpin(float64(at), reelDrag),
		seekTo: -1,
		lift:   physics.NewScalar(0, liftFrequency),
	}
}

// Sync takes a fresh directory without losing the refiner's place. The
// listing is rebuilt every frame and a file opening elsewhere shifts the ones
// after it, so the place is kept by name.
func (r *Rolodex) Sync(files []Entry) {
	if len(files) == 0 {
		r.files = nil
		return
	}
	was := ""
	if at := r.Want(); at >= 0 && at < len(r.files) {
		was = r.files[at].Name
	}
	r.files = files
	if was == "" {
		return
	}
	for i, e := range files {
		if e.Name == was {
			// Shift by as many cards as the listing moved, keeping the
			// velocity, so the drawer does not appear to turn.
			if by := float64(i - r.Want()); by != 0 {
				v := r.at.Velocity()
				r.at.Set(r.at.Value() + by)
				r.at.Push(v)
			}
			return
		}
	}
}

// Step lets the drawer coast, and brings it up out of the dark. The detent is
// applied before the step, so a nearly-home card is caught on the frame it
// would otherwise drift past.
func (r *Rolodex) Step(dt float64) {
	r.StepFade(dt)
	if len(r.files) == 0 {
		return
	}

	// A riffle moves the drawer itself; seeking and the detent never run with
	// it or together: the detent pulls toward the card under the drawer and
	// would fight the journey.
	if f := r.riffling; f != nil {
		at := r.at.Value()
		left := math.Abs(f.to - at)
		if step := riffleSpeed(left) * dt; step >= left {
			r.at.Set(f.to)
			r.riffling = nil
		} else {
			r.at.Set(at + math.Copysign(step, f.to-at))
		}
		r.at.Push(-r.at.Velocity())
	} else if r.seekTo >= 0 {
		d := float64(r.seekTo) - r.at.Value()
		v := r.at.Velocity()
		r.at.Push((d*reelSeek - v*2*math.Sqrt(reelSeek)) * dt)
		r.cap()

		if math.Abs(d) < reelArrived && math.Abs(v) < reelStill {
			// Arrived; the detent takes over.
			r.seekTo = -1
		}
	} else if v := r.at.Velocity(); math.Abs(v) < reelDetent {
		// Critically damped. Without the velocity term the drawer rings
		// around the card for seconds before the drag stops it.
		near := math.Round(r.at.Value())
		pull := (near-r.at.Value())*reelSnap - v*2*math.Sqrt(reelSnap)
		r.at.Push(pull * dt)
	}
	r.at.Step(dt)
	r.at.Bound(0, float64(len(r.files)-1))

	want := 0.0
	if r.presenting {
		want = 1
	}
	r.lift.Step(dt, want)

	// Settle exactly, or the drawer approaches the card for ever.
	if math.Abs(r.at.Velocity()) < reelStill {
		if near := math.Round(r.at.Value()); math.Abs(r.at.Value()-near) < reelArrived {
			r.at.Set(near)
		}
	}
}

// Len is how many files are in the drawer, and Files the drawer itself.
func (r *Rolodex) Len() int       { return len(r.files) }
func (r *Rolodex) Files() []Entry { return r.files }

// Want is the card the drawer is on (the nearest to its position, so it
// changes while the drawer coasts), and At where it has got to.
func (r *Rolodex) Want() int {
	if len(r.files) == 0 {
		return 0
	}
	return clampInt(int(math.Round(r.at.Value())), 0, len(r.files)-1)
}

func (r *Rolodex) At() float64 { return r.at.Value() }

// Turning reports whether the drawer is still moving.
func (r *Rolodex) Turning() bool {
	// A seek counts even when momentarily still on a card, or a caller
	// waiting for the reel to stop would get whichever card it was passing.
	return r.seekTo >= 0 || r.riffling != nil ||
		math.Abs(r.at.Velocity()) > reelStill ||
		math.Abs(r.at.Value()-math.Round(r.at.Value())) > reelArrived
}

// Selected is the file the drawer is on.
func (r *Rolodex) Selected() (Entry, bool) {
	if len(r.files) == 0 {
		return Entry{}, false
	}
	return r.files[r.Want()], true
}

// Turn shoves the drawer, one press at a time. Pushes add up, so holding the
// key winds the drawer up, and letting go leaves it coasting.
func (r *Rolodex) Turn(by int) {
	if len(r.files) == 0 || by == 0 {
		return
	}
	r.takeTheWheel()
	r.at.Push(float64(sign(by)) * reelKick)
	r.cap()
}

// cap holds the drawer's speed to reelTop.
func (r *Rolodex) cap() {
	if v := r.at.Velocity(); math.Abs(v) > reelTop {
		r.at.Push(math.Copysign(reelTop, v) - v)
	}
}

// takeTheWheel cancels a seek or riffle and puts a presented card back in the
// stack.
func (r *Rolodex) takeTheWheel() {
	r.seekTo = -1
	r.riffling = nil
	r.presenting = false
}

// Riffle runs the drawer onto card to from whichever end of it is further,
// fast at first and slowing onto the card, as the show's drawer does as it
// comes up. Turning the drawer takes it over.
func (r *Rolodex) Riffle(to int) {
	n := len(r.files)
	if n == 0 {
		return
	}
	r.takeTheWheel()
	to = clampInt(to, 0, n-1)
	from := 0
	if to < n-1-to {
		from = n - 1
	}
	r.at.Set(float64(from))
	if from != to {
		r.riffling = &riffle{to: float64(to)}
	}
}

// Riffling reports whether the drawer is riffling.
func (r *Rolodex) Riffling() bool { return r.riffling != nil }

// Arrive puts the drawer where its riffle or seek was going, at once.
func (r *Rolodex) Arrive() {
	switch {
	case r.riffling != nil:
		r.at.Set(r.riffling.to)
		r.riffling = nil
	case r.seekTo >= 0:
		r.at.Set(float64(r.seekTo))
		r.seekTo = -1
	default:
		return
	}
	r.at.Push(-r.at.Velocity())
}

// Seek sends the drawer to a card: it winds up, runs through the cards and
// eases onto it (GoTo, by contrast, jumps). The card is presented, standing
// out of the drawer until the player turns it.
func (r *Rolodex) Seek(i int) {
	if len(r.files) == 0 {
		return
	}
	r.seekTo = clampInt(i, 0, len(r.files)-1)
	r.presenting = true
}

// Seeking reports whether the drawer is on its way somewhere it was sent.
func (r *Rolodex) Seeking() bool { return r.seekTo >= 0 }

// Lift is how far the card is standing out of the drawer, from 0 to 1.
func (r *Rolodex) Lift() float64 { return ease.Clamp01(r.lift.Value()) }

// Busiest is the indices of files with somebody on them, fullest first, then
// alphabetically (Rooms.List's order). It ranks indices without resorting the
// directory, because TurnToLetter and Behind need it alphabetical.
func (r *Rolodex) Busiest() []int {
	var out []int
	for i, e := range r.files {
		if e.Open && e.Occupied > 0 {
			out = append(out, i)
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if oa, ob := r.files[out[a]].Occupied, r.files[out[b]].Occupied; oa != ob {
			return oa > ob
		}
		return r.files[out[a]].Name < r.files[out[b]].Name
	})
	return out
}

// SeekBusier sends the drawer up the occupancy ranking, or down it for by < 0.
// The ends clamp, so holding the key parks on the busiest file. It reports
// false when nobody is on any file, so the caller can say so.
func (r *Rolodex) SeekBusier(by int) bool {
	rank := r.Busiest()
	if len(rank) == 0 {
		return false
	}

	// Where the drawer is in the ranking. On a file nobody is on, enter at
	// whichever end the press was heading for.
	at := -1
	for i, f := range rank {
		if f == r.Want() {
			at = i
			break
		}
	}
	switch {
	case at < 0 && by > 0:
		at = len(rank) // one past the least busy, so going up lands on it
	case at < 0:
		at = -1
	}

	// Up the ranking is toward the front of the slice, since it is fullest first.
	r.Seek(rank[clampInt(at-sign(by), 0, len(rank)-1)])
	return true
}

func sign(n int) int {
	if n < 0 {
		return -1
	}
	return 1
}

// GoTo takes the drawer straight to a card, with no travel. TurnToLetter uses
// it.
func (r *Rolodex) GoTo(i int) {
	if len(r.files) == 0 {
		return
	}
	r.takeTheWheel()
	r.at.Set(float64(clampInt(i, 0, len(r.files)-1)))
}

// TurnToLetter jumps to the first file under the next or previous initial.
// The directory is alphabetical, so this walks to where the initial changes.
func (r *Rolodex) TurnToLetter(by int) {
	if len(r.files) == 0 || by == 0 {
		return
	}
	at := r.Want()
	here := initialOf(r.files[at].Name)

	// Off the current letter first, then (going backward) on to the start of
	// the letter landed on, or pressing left twice would visit the same
	// letter's last card and then its first.
	i := at
	for i+by >= 0 && i+by < len(r.files) && initialOf(r.files[i+by].Name) == here {
		i += by
	}
	if i+by < 0 || i+by >= len(r.files) {
		// No next letter: stay put, instead of moving within this letter.
		return
	}
	i += by
	if by < 0 {
		for i-1 >= 0 && initialOf(r.files[i-1].Name) == initialOf(r.files[i].Name) {
			i--
		}
	}

	r.GoTo(i)
}

// Showing is the card on screen and how flat it is: 1 face on, 0 edge on, and
// negative while its back is turned. It is not always the selected card: past
// half way through a turn, the next card is the one coming up.
func (r *Rolodex) Showing() (Entry, float64, bool) {
	if len(r.files) == 0 {
		return Entry{}, 0, false
	}
	pos := clampF(r.at.Value(), 0, float64(len(r.files)-1))
	lo := int(math.Floor(pos))
	t := pos - float64(lo)

	// Past the halfway point the next card is the one facing you, and the angle
	// runs backwards from edge-on so it opens out.
	idx, angle := lo, t*math.Pi
	if t > 0.5 && lo+1 < len(r.files) {
		idx, angle = lo+1, (1-t)*math.Pi
	}
	scale := math.Cos(angle)
	if t > 0.5 && idx == lo {
		// The reel ran past the last card; there is nothing behind it to turn to.
		scale = math.Abs(scale)
	}
	return r.files[clampInt(idx, 0, len(r.files)-1)], scale, true
}

// initialOf is the letter a file is filed behind.
func initialOf(name string) rune {
	r := []rune(name)
	if len(r) == 0 {
		return ' '
	}
	return upperRune(r[0])
}

// Behind is how many lettered dividers (one per initial) stand either side of
// the card on screen, up to n each way. It is a count, not the letters:
// drawCardStack spaces them evenly, and only needs to know that nothing stands
// before the first letter or after the last.
func (r *Rolodex) Behind(n int) (before, after int) {
	if len(r.files) == 0 || n <= 0 {
		return 0, 0
	}
	e, _, _ := r.Showing()
	here := initialOf(e.Name)

	// The drawer is alphabetical, so the distinct initials come out in order by
	// collapsing runs.
	var letters []rune
	for _, f := range r.files {
		if l := initialOf(f.Name); len(letters) == 0 || letters[len(letters)-1] != l {
			letters = append(letters, l)
		}
	}
	at := -1
	for i, l := range letters {
		if l == here {
			at = i
			break
		}
	}
	if at < 0 {
		return 0, 0
	}
	return min(at, n), min(len(letters)-1-at, n)
}
