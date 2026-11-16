package lumon

import "time"

// WindowSizeMsg updates the model's terminal-cell viewport.
type WindowSizeMsg struct{ Width, Height int }

// KeyMsg is the platform-neutral key event consumed by the game model.
type KeyMsg struct {
	Key   string
	Runes []rune
	Alt   bool
}

// String returns the key's name.
func (m KeyMsg) String() string { return m.Key }

// MouseAction is what a mouse event did: press, release or motion.
type MouseAction uint8

const (
	MousePress MouseAction = iota
	MouseRelease
	MouseMotion
)

// MouseButton is the button or wheel direction of a mouse event.
type MouseButton uint8

const (
	MouseNone MouseButton = iota
	MouseLeft
	MouseWheelUp
	MouseWheelDown
)

// MouseMsg uses zero-based cell coordinates, like the native terminal model.
type MouseMsg struct {
	X, Y   int
	Button MouseButton
	Action MouseAction
}

// EffectKind describes work the host event loop must schedule.
type EffectKind uint8

const (
	EffectFrame EffectKind = iota
	EffectLink
	EffectQuit
	EffectSound // play or stop Effect.Sound, if the host has sound
)

// Effect is the small platform-neutral boundary between the game and its host.
type Effect struct {
	Kind  EffectKind
	Since time.Time
	Sound Sound

	// Volume is how loud to play Sound, from 0 to 1.
	Volume float64
}

// Sound is a sound the game asks its host to play. Only the browser plays any.
type Sound uint8

const (
	SoundStartup  Sound = iota + 1 // the boot sequence's log has finished and its logo starts
	SoundHush                      // the boot sequence was cut short: stop its sound
	SoundBinOpen                   // a bin's lid swings open
	SoundBinClose                  // a bin's lid swings shut
	SoundRefine                    // a refined cluster's digits leave for their bin
	SoundBeep                      // the set beeps: as it is switched on, and rarely after
	SoundProcess                   // the set works at something, faintly, very rarely
	SoundCard                      // a card flicks past in the file drawer
	SoundPop                       // a box opens over the screen: help, a menu, a notice
	SoundChime                     // the file's header reads a higher percent
	SoundClick                     // the mouse is clicked
	SoundBinRise                   // a bin reads a higher percent
	SoundBlink                     // a bin's drawer blinks its four bars
	SoundBlast                     // a cluster left awake detonates
	SoundFinale                    // the music for a finished file, as long as it is held
)

// farVolume is how loud a sound of another refiner's doing plays (a
// computer's, in the browser), against one of this refiner's own.
const farVolume = 0.4

// volumeFor is the volume of a sound that is this refiner's doing, or not.
func volumeFor(mine bool) float64 {
	if mine {
		return 1
	}
	return farVolume
}

// AgentMsg tells the refiner that a coding agent they are waiting on has
// finished, or needs their approval. Project names the agent's working
// directory and must already be plain printable text.
type AgentMsg struct {
	Project       string
	NeedsApproval bool

	// A finished turn's summary, each zero when the agent could not say: how
	// long it took, its steps (tool calls) and the files it edited.
	Took   time.Duration
	Steps  int
	Edited int
}

// AgentState is what one agent on the agents strip is doing.
type AgentState int

const (
	AgentWorking AgentState = iota
	AgentWaiting            // for the refiner's approval
	AgentDone
)

// AgentLine is one coding agent on the strip in the file bar's lower edge.
// Name and Doing must already be plain printable text.
type AgentLine struct {
	Name  string
	State AgentState
	Doing string        // the step it is on, while working
	For   time.Duration // how long it has been on its prompt
}

// AgentsMsg is every coding agent the refiner is running, replacing the last
// such message. Empty clears the strip.
type AgentsMsg struct {
	Agents []AgentLine
}
