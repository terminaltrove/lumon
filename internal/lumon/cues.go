package lumon

import "math"

// After the boot sequence the set beeps now and then, at random: each beep
// comes beepFirst seconds plus a wait averaging beepMean after the end of the
// boot sequence or the last beep, and at most beepLongest after it.
const (
	beepFirst   = 120.0
	beepMean    = 360.0
	beepLongest = 1200.0
	beepVolume  = 0.7
)

// Now and then, very rarely, the set is faintly heard working at something:
// on average once every processMean seconds after the boot sequence.
const (
	processMean   = 700.0
	processVolume = 0.12
)

// finaleVolume keeps the music for a finished file under the room's other
// sounds, and finaleLate is how far into the hold, in seconds, a view can
// arrive and still hear it from the start.
const (
	finaleVolume = 0.5
	finaleLate   = 1.0
)

// stopFinaleMusic hushes the music for a finished file once this view is no
// longer on it: it left, a diversion took over, or the next file came. The
// music fades down by itself by the time the next file comes.
func (m *Model) stopFinaleMusic() {
	if m.finaleHeard && (m.Room == nil || !m.Room.Complete() || m.Diverting()) {
		m.finaleHeard = false
		m.cue(SoundHush, 1)
	}
}

// binRiseVolume keeps a bin's percent ticking up, which happens with every
// cluster, quiet.
const binRiseVolume = 0.5

// blastVolume is a detonation's, which is never this refiner's doing.
const blastVolume = 0.5

// hearBlasts sounds every detonation since the last, once.
func (m *Model) hearBlasts() {
	if m.Room != nil && m.Room.blastSeq != m.blastHeard {
		m.blastHeard = m.Room.blastSeq
		m.cue(SoundBlast, blastVolume)
	}
}

// clickVolume keeps a click, the commonest sound, under the rest.
const clickVolume = 0.6

// cardEvery is the least time between two cards' flicks, in seconds: a
// drawer at full speed passes a card about every 40ms.
const cardEvery = 0.05

// stepAmbience plays the set's own sounds once it is past its boot sequence:
// its beeps, counted down, and its faint working, by chance.
func (m *Model) stepAmbience(dt float64) {
	if m.Screen == ScreenOff || m.Screen == ScreenIntro {
		return
	}
	if m.soundRng.Float64() < dt/processMean {
		m.cue(SoundProcess, processVolume)
	}
	if m.beepIn <= 0 {
		m.beepIn = beepFirst + math.Min(m.soundRng.ExpFloat64()*beepMean, beepLongest-beepFirst)
		return
	}
	if m.beepIn -= dt; m.beepIn <= 0 {
		m.cue(SoundBeep, beepVolume)
	}
}

// flickCard sounds a card when the drawer has moved onto another, unless one
// sounded within cardEvery.
func (m *Model) flickCard(moved bool, dt float64) {
	m.cardIn -= dt
	if moved && m.cardIn <= 0 {
		m.cue(SoundCard, 1)
		m.cardIn = cardEvery
	}
}

// overlaysShowing is a bit for each box drawn over the screen (see Draw).
func (m *Model) overlaysShowing() uint16 {
	var b uint16
	for i, on := range [...]bool{
		m.divertOpen, m.About, m.Helping, m.exitOpen, m.calloutOpen,
		m.lanUI.step != lanClosed, m.Escaped, m.agentNote != "",
		m.Room != nil && m.Screen == ScreenMain && m.Room.Complete(),
	} {
		if on {
			b |= 1 << i
		}
	}
	return b
}

// popOpened sounds a pop when a box has opened over the screen since the
// last frame.
func (m *Model) popOpened() {
	now := m.overlaysShowing()
	if now&^m.overlays != 0 {
		m.cue(SoundPop, 1)
	}
	m.overlays = now
}

// chimeVolume keeps the chime well under the refine it follows.
const chimeVolume = 0.5

// chimeProgress chimes when the header's percent has risen since the last
// frame on the board. A file first seen, or another, sets where it starts.
func (m *Model) chimeProgress() {
	if m.Screen != ScreenMain || m.Room == nil {
		m.heardRoom = nil
		return
	}
	pct := shownPercent(m.File.Name, m.File.Percent(m.Grid))
	if m.heardRoom == m.Room && pct > m.heardPercent {
		m.cue(SoundChime, chimeVolume)
	}
	m.heardRoom, m.heardPercent = m.Room, pct
}
