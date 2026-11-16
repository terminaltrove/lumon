package lumon

import (
	"fmt"
	"sort"
	"time"

	"github.com/terminaltrove/lumon/internal/physics"
	"github.com/terminaltrove/lumon/internal/theme"
)

// FrameMsg is the elapsed wall time since the last frame. The Clock turns it
// into a whole number of fixed 60Hz physics steps.
type FrameMsg time.Duration

// frameRate is how often a frame is requested. Lowering it cuts both CPU and
// bandwidth (fewer frames, fewer diffs sent) without adding input latency,
// since View runs on every message. It costs only the smoothness of ambient
// motion: physics is fixed at DT, so the simulation is identical at any frame
// rate.
var frameRate = time.Second / 60

// MinFPS and MaxFPS bound what SetFrameRate accepts. The ceiling is Bubble
// Tea's renderer maximum; below the floor ambient motion looks broken.
const (
	MinFPS = 15
	MaxFPS = 120
)

// SetFrameRate asks for fps frames a second, server-wide, since frame rate is
// a host budget and not a player's choice. Nothing guards it, so it must be
// called before the first session exists (cmd/lumon calls it while parsing
// flags); frameCmd reads it on every tick.
func SetFrameRate(fps int) error {
	if fps < MinFPS || fps > MaxFPS {
		return fmt.Errorf("frame rate %d outside %d-%d", fps, MinFPS, MaxFPS)
	}
	frameRate = time.Second / time.Duration(fps)
	return nil
}

// FrameRate is what SetFrameRate last set, for anything that reports it.
func FrameRate() time.Duration { return frameRate }

// LinkMsg is one measured round trip to this terminal. See link.go.
type LinkMsg time.Duration

// WatchLink tells this session where its round-trip measurements will arrive.
// cmd/lumon calls it for remote sessions only, so a local run shows no link light.
// It must be called before Init, which first reads the channel.
func (m *Model) WatchLink(ch <-chan time.Duration) { m.links = ch }

// KeepOpen preserves a browser session when the user backs out of a file.
// Native terminal and remote sessions retain their usual quit controls.
func (m *Model) KeepOpen() { m.keepOpen = true }

// UseTouch tells a browser session that it is on a touch screen, so its hint
// lines name taps and drags instead of keys.
func (m *Model) UseTouch() { m.touchScreen = true }

// InitialEffects starts the frame loop and optional link monitor.
func (m *Model) InitialEffects() []Effect {
	effects := []Effect{{Kind: EffectFrame}}
	if m.links != nil {
		effects = append(effects, Effect{Kind: EffectLink})
	}
	return effects
}

// UpdateEvent applies a platform-neutral event and returns work for its host.
// It holds the room's write lock throughout the call, so everything that changes
// the board and this player's animation is stepped against one consistent
// board.
func (m *Model) UpdateEvent(event any) []Effect {
	effects := append(m.updateEvent(event), m.sounds...)
	m.sounds = m.sounds[:0]
	return effects
}

// cue asks the host for a sound, at a volume from 0 to 1, with the effects of
// this update.
func (m *Model) cue(s Sound, volume float64) {
	m.sounds = append(m.sounds, Effect{Kind: EffectSound, Sound: s, Volume: volume})
}

func (m *Model) updateEvent(event any) []Effect {
	// A session in the boot sequence or the lobby has no board to hold.
	if room := m.Room; room != nil {
		room.mu.Lock()
		defer func() {
			room.mu.Unlock()
			if m.leaveForLobby {
				m.leaveForLobby = false
				m.Leave()
				m.fadeFilesUp()
			}
			if m.reseat {
				m.reseat = false
				m.sitAgain()
			}
		}()
	}

	switch msg := event.(type) {
	case WindowSizeMsg:
		m.Resize(msg.Width, msg.Height)

	case KeyMsg:
		m.touched()
		if m.handleKey(msg) {
			return []Effect{{Kind: EffectQuit}}
		}

	case MouseMsg:
		m.touched()
		m.handleMouseEvent(msg)
		if m.Quitting {
			return []Effect{{Kind: EffectQuit}}
		}

	case AgentMsg:
		m.agentNote = agentNotice(msg)

	case AgentsMsg:
		m.agents = msg.Agents

	case LinkMsg:
		m.link.Sample(time.Duration(msg))
		return []Effect{{Kind: EffectLink}}

	case FrameMsg:
		// Taken before the work, so the next frame's duration includes this
		// one's drawing.
		started := time.Now()
		if m.leftFor(time.Duration(msg)) {
			m.Quitting = true
			return []Effect{{Kind: EffectQuit}}
		}
		m.advance(time.Duration(msg))
		return []Effect{{Kind: EffectFrame, Since: started}}
	}
	return nil
}

// Stepped reports whether the last frame advanced the simulation at all. A
// frame can run no steps (vsync jitter, or a 120Hz display ticking twice per
// step); with no input either, the browser host skips drawing it.
func (m *Model) Stepped() bool { return m.lastSteps > 0 }

// advance runs the simulation for the elapsed wall time.
func (m *Model) advance(elapsed time.Duration) {
	m.lastSteps = m.Clock.Tick(elapsed, func(dt float64) {
		m.sessionAge += dt
		m.stepAmbience(dt)
		m.stopFinaleMusic()

		// The board advances whatever screen this player is on, so one
		// person idling does not freeze a shared board. A lobby session has
		// no board yet.
		if m.Room != nil {
			m.stepBoard(dt)
			m.followFile()
		}

		switch m.Screen {
		case ScreenIntro:
			if m.Intro == nil {
				m.finishIntro()
				break
			}
			before := m.Intro.Stage()
			blank := before == stageBoot && len(m.Intro.Lines()) == 0
			m.Intro.Step(dt)
			if blank && len(m.Intro.Lines()) > 0 {
				m.cue(SoundBeep, 1) // the log's first lines are drawn
			}
			if before == stageBoot && m.Intro.Stage() == stageLogo {
				m.cue(SoundStartup, 1)
			}
			if m.Intro.Done() {
				m.finishIntro()
			}
		case ScreenLobby:
			// The drawer fades up and the reel turns. Through rolodex(), so the
			// drawer exists from the first tick and its fade starts then.
			if m.rooms != nil {
				r := m.rolodex()
				was := r.Want()
				r.Step(dt)
				m.flickCard(was != r.Want(), dt)
			}
		case ScreenName, ScreenScores, ScreenOff:
			// Nothing of this player's to simulate; the board was stepped above.
		case ScreenIdle:
			if m.idleMark != nil {
				m.idleMark.Step(dt)
				if m.idleMark.Cornered() {
					m.celebrateCorner()
				}
			}
		default:
			if m.Room == nil {
				break
			}
			// A diversion is stepped in place of the view; the board was
			// stepped above.
			if m.Diverting() {
				m.stepDiversion(dt)
				m.stepChrome(dt)
				break
			}
			m.stepView(dt)
		}
	})
	m.popOpened()
	m.chimeProgress()
}

// stepBoard advances what every player must agree on. Agitation is stepped
// here so its noise phase stays in step between players.
func (m *Model) stepBoard(dt float64) {
	m.publish()

	// Through this session's own board clock, so a room does not age once per
	// person watching it. See Room.StepTo.
	m.boardTime += dt
	m.Room.StepTo(m.boardTime)
	m.takeSeatIfOffered()
	m.followBots(dt)

	// Only seated refiners decide what wakes: the countdown is decremented once
	// per asking session, and a watcher should not steer it by scrolling.
	// StepTo is not gated, so a watcher can keep the board moving.
	if m.Desk != nil {
		m.Room.MaybeActivate(m.viewports())
	}

	// Before syncAgitation, which releases the bodies these flights need. See
	// catchDeposits.
	m.catchDeposits()
	m.syncAgitation()

	m.driveAgitation()
	m.Agitation.Step(dt)
}

// takeSeatIfOffered sits a watcher down when their turn comes. The session
// pulls the seat from inside its own Update, which already holds the room's
// write lock, so it uses the room's unlocked PromoteIfNext: calling Seat would
// lock again, and RWMutex is not reentrant. The room must not push a seat into
// another session's Model, which may be drawing at the time. Nothing else about
// the view is reset.
func (m *Model) takeSeatIfOffered() {
	if m.Desk != nil || m.watch == nil {
		return
	}
	if d := m.Room.PromoteIfNext(m.watch, m.Name); d != nil {
		m.Desk, m.watch = d, nil
	}
}

// Watching reports whether this session is at a board it has no desk at.
func (m *Model) Watching() bool { return m.Room != nil && m.Desk == nil }

// stepView advances this player's own animation of the board.
func (m *Model) stepView(dt float64) {
	m.stepArrival(dt)
	m.stepZoom(dt)
	m.Drift.Sync(m.Layout)
	m.Drift.Step(dt)
	m.ripple.Sync(m.Layout)
	m.catchBlasts()
	m.ripple.Step(dt)
	m.trackLens()
	m.trackRemoteLenses()
	m.Lens.Step(dt)
	for _, l := range m.remoteLens {
		l.Step(dt)
	}
	m.Magnify.Update(dt, m.glyphTargets())
	m.Flights.Step(dt)
	for _, mine := range m.Flights.TakeHalfway() {
		m.cue(SoundRefine, volumeFor(mine))
	}
	m.Regen.Step(dt)
	m.collectLanded()
	if m.solid != nil {
		cx, cy := m.solidCentre()
		m.solid.Step(dt, cx, cy, m.transit == nil)
	}
	m.stepTints(dt)
	m.stepChrome(dt)
}

// noticeFrequency is how fast the completion notice opens, in rad/s (critically
// damped, about 0.6s). noticeShows is how far open counts as fully open, since
// a critically damped spring never quite reaches one.
const (
	noticeFrequency = 11.0
	noticeShows     = 0.99
)

// stepFinale runs the display a finished file puts on, and opens the notice.
// The fireworks fire from this terminal's own bins (BinFill's lids, as in the
// FIREWORKS diversion), seeded from drawnFrame(). It does not run during a
// diversion, which owns the panel and the lids.
func (m *Model) stepFinale(dt float64) {
	if m.Room == nil || !m.Room.Complete() || m.Diverting() {
		m.dropFinale()
		return
	}
	if m.noticeOpen == nil {
		m.noticeOpen = physics.NewScalar(0, noticeFrequency)
		// The music, from its start, for a file finished just now; one
		// arrived at partway through is held for less than it lasts.
		if m.Room.Held() < finaleLate {
			m.cue(SoundFinale, finaleVolume)
			m.finaleHeard = true
		}
	}
	m.noticeOpen.Step(dt, 1)

	if m.finale == nil {
		m.finale = NewFireworks(m, m.gridPanel().Play(), m.viewRng)
	}
	m.finale.Resize(m.gridPanel().Play())
	m.finale.Aim(m.Layout)
	m.finale.Step(dt, m.BinFill)
}

// dropFinale lets go of the celebration and closes the notice. It is called
// every frame the file is not being held, so the Fireworks (a particle per
// digit) cannot outlive its file.
func (m *Model) dropFinale() {
	if m.finale != nil && m.BinFill != nil {
		// Release the lids, or they stay held open into the next file.
		for b := 0; b < BinCount; b++ {
			m.BinFill.SetHold(b, false)
		}
	}
	m.finale, m.noticeOpen = nil, nil
}

// stepTints tells every visible digit what colour it should be and advances the
// ones that are crossing to a new one. It runs on the tick, not in the draw,
// because View also runs on every input message and would speed up the fades
// while somebody types.
func (m *Model) stepTints(dt float64) {
	l := m.Layout
	m.tint.Sync(l)
	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			n := m.Grid.At(gx, gy)
			if n == nil {
				continue
			}
			m.tint.Toward(gx, gy, m.cellTarget(n, gx, gy))
		}
	}
	m.tint.Step(dt)
}

// stepChrome advances the file bar along the top and the bins along the
// bottom. It is separate from stepView because it also runs during a
// diversion, which replaces only the grid.
func (m *Model) stepChrome(dt float64) {
	// Re-check the selected bin every step: bins fill without a keypress
	// (credit lands after creditDelay, and other refiners fill bins too).
	m.SelectedBin = m.nearestOpenBin(m.SelectedBin)

	m.File.Step(dt, m.Grid)

	// The drawers first, so a bin's credit raises its drawer before its lid,
	// held open by it, would shut. A diversion owns the panel, and a finished
	// file has its own display.
	raise := m.Room != nil && !m.Room.Complete() && !m.Diverting()
	m.Drawer.Step(dt, m.Grid, raise)
	for _, i := range m.Drawer.Rose() {
		m.cue(SoundBinRise, volumeFor(m.BinFill.Mine(i))*binRiseVolume)
	}
	for _, i := range m.Drawer.Blinked() {
		m.cue(SoundBlink, volumeFor(m.BinFill.Mine(i)))
	}

	m.BinFill.Step(dt, m.Grid, m.Flights.Active(), m.Drawer.Up)
	for _, s := range m.BinFill.Swings() {
		sound := SoundBinClose
		if s.Open {
			sound = SoundBinOpen
		}
		m.cue(sound, volumeFor(s.Mine))
	}
	m.hearBlasts()

	// Here, not in stepView, because stepView does not run during a diversion,
	// and a finale left unstepped would never release the lids it holds.
	m.stepFinale(dt)
}

// Breathing bounds, in rows. A woken cluster swells to its crest, eases back to
// its trough, and swells again. The swell is a hint that something is there and
// how long it has been ignored; it never grows large enough to read. Pointing
// at or ringfencing a cluster is what takes it to full size.
const (
	// BreatheHigh and BreatheLow are the band a fully escalated cluster uses:
	// under four rows, against MaxGlyphHeight for a reached cluster, so it reads
	// as a blob and not yet as a digit.
	BreatheHigh = 3.8
	BreatheLow  = 2.6

	// BreatheHighCalm and BreatheLowCalm are the band it starts with: the size
	// of an ordinary digit. Starting the band at PlainHeight makes the growth
	// follow the smoothstep, which starts flat, so a cluster stays a plain
	// character for its first twenty-odd seconds. A higher starting band lets
	// the critically damped spring, fastest at the start, reach glyph size
	// within seconds.
	BreatheHighCalm = PlainHeight
	BreatheLowCalm  = PlainHeight

	// HintSeconds is how long the swell takes to reach its ceiling, in seconds.
	// It is shorter than EscalateSeconds: over the full escalation a digit only
	// crosses the glyph threshold after about 62s. Over 45s it is plain for
	// about 28s and a small glyph after that. Colour and shake carry the
	// urgency after size has peaked.
	HintSeconds = 45.0

	// breatheArrival is how close, in rows, the cluster must get to its target
	// before the cycle turns around, so the springs set the rhythm.
	breatheArrival = 0.35

	// breatheFollowFrequency is how tightly a cluster member tracks the shared
	// swell. Fast, because the swell is already slow; slow cells would never
	// reach the crest.
	breatheFollowFrequency = 4.0
)

// EscalateSeconds is how long a cluster takes to reach full fury. It is longer
// than theme.ScaryRedSeconds, so the colour saturates first and the shake keeps
// building after it.
const EscalateSeconds = 100.0

// driveAgitation reapplies each woken cluster's shake every frame, since
// escalation is a function of time.
func (m *Model) driveAgitation() {
	if m.Room == nil {
		return
	}
	for _, c := range m.actives {
		id := c.grp.ID
		e := m.escalation(id)
		m.Agitation.SetEscalation(id, e)

		want := activeIntensity + e*(furiousIntensity-activeIntensity)
		if m.hoveredGroup == id {
			want = superActiveIntensity
		}
		m.Agitation.SetIntensity(id, want)
	}
}

// mineSeat marks a reach as this session's own in reachSeat. It is negative
// because seats are 0-based and a local game has no desk at all.
const mineSeat = -1

// glyphTargets is the height every growing digit should be heading for. There
// are two sources, both corruption: a woken cluster's breathe cycle, and a
// cluster somebody is reaching for, which goes to MaxGlyphHeight. Where they
// overlap the larger wins. Any other cell is plain, so size always means
// corruption; the cursor lens bulges the grid but magnifies nothing.
func (m *Model) glyphTargets() map[int]float64 {
	clear(m.targets)

	// Cells follow the shared swell closely. Reaching is a separate source, not
	// a multiplier on the swell.
	clear(m.reachSeat)

	for _, c := range m.actives {
		want := c.breathe.Value()
		freq := breatheFollowFrequency

		// Pointing at it or holding it, by anybody. Two checks: m.actives still
		// lists a cluster that was put back to sleep, and Grid.Group catches one
		// that has detonated (Retire does not touch the room's awake list).
		if seat, reached := m.reachedBy(c.grp.ID); reached {
			if m.Grid.Group(c.grp.ID) != nil {
				want, freq = MaxGlyphHeight, reachGrowFrequency
				m.reachSeat[c.grp.ID] = seat
			}
		}

		for _, id := range c.grp.NumberIDs {
			if m.Flights.InFlight(id) {
				continue
			}
			if want > m.targets[id] {
				m.targets[id] = want
				m.Magnify.SetFrequency(id, freq)
			}
		}
	}

	return m.targets
}

// reachedBy reports whether anybody at this board is pointing at or holding a
// cluster, and whose seat it is; mineSeat means this session's own.
//
// Your own reach comes from your own fields, not your desk's published Reach,
// so it does not depend on publish having run and works in a local game with
// no desk. This session wins a tie, so your own gesture shows in your colour.
func (m *Model) reachedBy(groupID int) (seat int, reached bool) {
	if groupID == m.hoveredGroup || m.held[groupID] {
		return mineSeat, true
	}
	if m.Room == nil {
		return 0, false
	}
	for _, d := range m.Room.desk {
		if d == nil || d == m.Desk {
			continue
		}
		for _, id := range d.Reach {
			if id == groupID {
				return d.Seat, true
			}
		}
	}
	return 0, false
}

// collectLanded starts the regen animation in each cell a landed digit
// vacated. The replacement digit was already written when the cluster was
// refined; this touches only the view.
func (m *Model) collectLanded() {
	for _, f := range m.Flights.TakeLanded() {
		m.Regen.Begin(f.ID, m.Grid.Cells[f.ID].Value)
	}
}

// viewports is the windows onto the board that MaybeActivate should wake a
// cluster in. Each session passes its own, so a cluster wakes where somebody
// can see it.
func (m *Model) viewports() []Viewport {
	l := m.Layout
	return []Viewport{{X: l.ViewportX, Y: l.ViewportY, W: l.ViewportW, H: l.ViewportH}}
}

// backOut closes the outermost thing the refiner is inside, and reports whether
// there was one; the caller quits when there was not. The order is: an overlay,
// the drawer, a diversion, the third dimension, and last the lobby back to the
// name prompt.
func (m *Model) backOut() bool {
	switch {
	case m.agentNote != "":
		m.agentNote = ""
	case m.Escaped:
		m.Escaped = false
	case m.Helping:
		m.Helping = false
	case m.About:
		m.About = false
	case m.calloutOpen:
		m.calloutOpen = false
	case m.divertOpen:
		m.divertOpen = false
	case m.Diverting():
		m.endDiversion()
	case m.solid != nil:
		m.toggleSolid()
	// Back to the name prompt, only for a session that was asked for a name.
	// A local session never saw the prompt, so q from its lobby quits.
	case m.Screen == ScreenLobby && m.askName:
		m.Screen = ScreenName
	default:
		return false
	}
	return true
}

func (m *Model) handleKey(msg KeyMsg) bool {
	// keys_test.go finds the keys a handler claims by walking its case clauses
	// and requires each to be documented. So documented keys are cases, and
	// keys that must stay out of the controls (esc, debug, the hidden words,
	// the camera) are `if`s.
	//
	// ctrl+c always quits. "?" works everywhere, including the name prompt,
	// since a name cannot contain it. "q" backs out one level at a time (see
	// backOut) and quits only when nothing is left; it is ignored on the name
	// prompt, where it is a letter.
	//
	// Escape quits nothing: it shows the Lumon notice. See escape.go.
	//
	// A LAN request comes first: it shows over every screen, so its keys work on
	// every screen.
	if m.answerLANRequest(msg.String()) {
		return false
	}
	if m.lanUI.step != lanClosed && msg.String() != "ctrl+c" {
		return m.handleLANKey(msg)
	}
	if m.Screen == ScreenIdle && m.handleIdleKey(msg.String()) {
		return false
	}
	// A terminal that is only the screensaver has nothing else to show, not even
	// the controls, so every other key ends it.
	if m.screensaverOnly {
		m.wakeScreensaver()
		return true
	}
	if msg.String() == "esc" {
		m.Escaped = true
		return false
	}

	// The completion debug key, an `if` so it stays out of the controls. Behind
	// the flag it does not exist.
	if CompletionDebug && msg.String() == completionDebugKey &&
		m.Screen == ScreenMain && m.Room != nil && !m.Room.Complete() {
		m.finishFileNow()
		return false
	}

	if m.exitOpen && msg.String() != "ctrl+c" {
		switch msg.String() {
		case "q":
			m.exitOpen = false
			m.Quitting = true
			return true
		case "up", "down", "left", "right", "tab":
			m.exitQuit = !m.exitQuit
		case "enter":
			m.exitOpen = false
			if m.exitQuit {
				m.Quitting = true
				return true
			}
			m.Screen = ScreenLobby
			m.leaveForLobby = true
		default:
			m.exitOpen = false
		}
		return false
	}
	if m.calloutOpen && msg.String() != "ctrl+c" && msg.String() != "q" {
		switch msg.String() {
		case "up", "k":
			m.calloutAt = (m.calloutAt + len(callouts) - 1) % len(callouts)
		case "down", "j":
			m.calloutAt = (m.calloutAt + 1) % len(callouts)
		case "enter":
			m.sayCallout()
			m.calloutOpen = false
		default:
			m.calloutOpen = false
		}
		return false
	}

	switch k := msg.String(); k {
	case "ctrl+c":
		if m.keepOpen {
			return false
		}
		m.Quitting = true
		return true
	case "?":
		m.Helping = true
		return false
	case "a":
		// Only on the board and in the lobby, since "a" can be typed in a name.
		if m.Screen == ScreenMain || m.Screen == ScreenLobby {
			m.About = !m.About
			return false
		}
	case "q":
		if m.Screen == ScreenName {
			break
		}
		if m.backOut() {
			return false
		}
		if m.Screen == ScreenMain && m.rooms != nil {
			if m.keepOpen {
				m.Screen = ScreenLobby
				m.leaveForLobby = true
				return false
			}
			m.exitOpen, m.exitQuit = true, false
			return false
		}
		if !m.keepOpen {
			m.Quitting = true
			return true
		}
		return false
	}

	// An open overlay takes every key, and any key dismisses it, topmost
	// first. Esc itself never reaches here, so pressing it again leaves the
	// escape notice up.
	if m.agentNote != "" {
		m.agentNote = ""
		// The agent wants the refiner back, so the screensaver ends with it.
		if m.Screen == ScreenIdle {
			m.wakeScreensaver()
		}
		return false
	}
	if m.Escaped {
		m.Escaped = false
		return false
	}
	if m.Helping {
		m.Helping = false
		return false
	}
	if m.About {
		m.About = false
		return false
	}

	// The diversions are modes of the board, so they take the keyboard before
	// the grid does.
	if m.divertOpen {
		m.handleDiversionsKey(msg.String())
		return false
	}
	if m.Diverting() {
		m.handleDivertingKey(msg.String())
		return false
	}

	// Any key switches the set on, skips the boot sequence, or wakes the
	// screensaver.
	if m.Screen == ScreenOff {
		m.switchOn()
		return false
	}
	if m.Screen == ScreenIntro {
		m.finishIntro()
		return false
	}
	if m.Screen == ScreenName {
		m.handleNameKey(msg)
		return false
	}
	if m.Screen == ScreenLobby {
		m.handleLobbyKey(msg.String())
		return false
	}
	if m.Screen == ScreenScores {
		m.handleScoresKey(msg.String())
		return false
	}
	if m.Screen == ScreenIdle {
		m.wakeScreensaver()
		return false
	}

	// The hidden word, checked before the grid's keys so typing it does not move
	// the cursor, and only reachable from the board.
	if swallowed, opened := m.noteKey(msg.String()); swallowed {
		// An if, not a switch, so keys_test.go's case scan does not see the
		// words.
		if opened == theWord {
			m.divertOpen, m.divertAt = true, 0
		} else if opened != "" {
			m.toggleSolid()
		}
		return false
	}

	// While a finished file is on its notice, the grid's keys do nothing.
	if m.Room != nil && m.Room.Complete() {
		return false
	}

	// The 3D camera's keys, an `if` so they stay out of the controls (the
	// footer names them; see rig.go). After the typed word, so 3D still closes
	// the view.
	if m.solid != nil && m.solid.Rig.Key(msg.String()) {
		return false
	}

	switch msg.String() {
	case "up", "k":
		m.moveCursor(0, -1)
		m.touchRig()
	case "down", "j":
		m.moveCursor(0, 1)
		m.touchRig()
	case "left", "h":
		m.moveCursor(-1, 0)
		m.touchRig()
	case "right", "l":
		m.moveCursor(1, 0)
		m.touchRig()
	// Shift+arrows navigate within a commitment, and so do the capitals: many
	// terminal emulators swallow shift+arrow for scrollback or panes and send
	// nothing. A lone K reaches here because noteKey never swallows a word's
	// first letter.
	case "shift+up", "K":
		m.moveCursorSweeping(0, -1)
		m.touchRig()
	case "shift+down", "J":
		m.moveCursorSweeping(0, 1)
		m.touchRig()
	case "shift+left", "H":
		m.moveCursorSweeping(-1, 0)
		m.touchRig()
	case "shift+right", "L":
		m.moveCursorSweeping(1, 0)
		m.touchRig()
	case "f":
		m.takeUnderCursor()
		m.touchRig()
	case "+", "=":
		m.Zoom(1)
		m.touchRig()
	case "-", "_":
		m.Zoom(-1)
		m.touchRig()
	case "1", "2", "3", "4", "5":
		if m.spectating {
			m.follow(msg.String())
			break
		}
		m.SelectedBin = m.nearestOpenBin(int(msg.String()[0] - '1'))
		m.touchRig()
	case "0", "tab":
		if m.spectating {
			m.follow(msg.String())
		}
	case "o":
		m.BinFill.Hold(m.SelectedBin)
		m.touchRig()
	case "enter", " ":
		m.refineAtCursor()
		m.touchRig()
	case "s":
		m.openScores(ScreenMain)
	case "t":
		m.Theme = theme.NextTheme(m.Theme)
	case "i":
		m.StartScreensaver()
	case "n":
		m.newsHidden = !m.newsHidden
	case "m":
		if m.lan != nil {
			m.openLAN()
		}
	case "b":
		if m.Desk != nil {
			m.calloutAt = 0
			m.calloutOpen = true
		}
	case "c":
		if m.allowBots && m.Room != nil {
			m.cue(SoundClick, clickVolume)
		}
		m.toggleCompany()
	case "w":
		m.toggleWatching()
	}
	return false
}

// wheelStep is how many rows one notch of the wheel moves. More than one,
// because a grid row is two terminal rows and a one-row scroll feels stuck.
const wheelStep = 3

// handleMouseEvent takes a mouse event, routed by screen as handleKey is. An
// unrouted screen would fall through to the grid path and dereference a nil
// *Room, and motion reporting means merely moving the pointer would do it.
func (m *Model) handleMouseEvent(msg MouseMsg) {
	press := msg.Button == MouseLeft && msg.Action == MousePress
	if press {
		m.cue(SoundClick, clickVolume)
	}

	// The LAN panel is keys only, and nothing behind it takes the mouse.
	if m.lanUI.step != lanClosed {
		return
	}

	// A click dismisses an overlay; motion does not.
	if m.agentNote != "" {
		if press {
			m.agentNote = ""
			if m.Screen == ScreenIdle {
				m.wakeScreensaver()
			}
		}
		return
	}
	if m.exitOpen {
		if press {
			m.exitOpen = false
		}
		return
	}
	if m.calloutOpen {
		if press {
			m.calloutOpen = false
		}
		return
	}
	if m.Helping {
		if press {
			m.Helping = false
		}
		return
	}
	if m.About {
		if press {
			m.About = false
		}
		return
	}
	if m.divertOpen {
		if press {
			m.divertOpen = false
		}
		return
	}
	if m.Diverting() {
		m.handleDivertingMouse(msg.X, msg.Y, press,
			msg.Action == MouseRelease,
			msg.Action == MouseMotion)
		return
	}

	// The wheel scrolls the grid (as the arrow keys do), the lobby and the
	// scoreboard.
	if up, down := msg.Button == MouseWheelUp,
		msg.Button == MouseWheelDown; up || down {
		switch {
		case m.Screen == ScreenMain && m.Room != nil && !m.Room.Complete():
			if up {
				m.moveCursor(0, -wheelStep)
			} else {
				m.moveCursor(0, wheelStep)
			}
			return
		case m.Screen == ScreenLobby:
			by := 1
			if up {
				by = -1
			}
			m.rolodexTurn(by)
			return
		case m.Screen == ScreenScores:
			m.scrollScores(up)
			return
		}
	}

	switch m.Screen {
	case ScreenOff:
		// A click switches the set on, as any key does.
		if press {
			m.switchOn()
		}
		return

	case ScreenIntro:
		// A click skips the boot sequence, as any key does.
		if press {
			m.finishIntro()
		}
		return

	case ScreenName:
		// A typed field has nothing for the mouse to do.
		return

	case ScreenLobby:
		m.handleLobbyMouse(msg.X, msg.Y, press)
		return

	case ScreenScores:
		// Must be routed; see the doc comment.
		if press {
			m.Screen = m.scoresFrom
		}
		return

	case ScreenIdle:
		m.handleIdleMouse(msg.X, msg.Y, press, msg.Action == MouseRelease, msg.Action == MouseMotion)
		return
	}

	// The link light in the footer is clickable whatever the board is doing.
	// Checked before the grid so a click on it does not reach a cell.
	if press && m.clickedLink(msg.X, msg.Y) {
		return
	}

	// While a finished file is on its notice the grid is not drawn, so its
	// mouse handling (including motion) does nothing.
	if m.Room != nil && m.Room.Complete() {
		return
	}

	// The grid. The pointer aims the lens, whether or not it is over a digit.
	m.Lens.LookAt(float64(msg.X), float64(msg.Y))
	_, _, m.pointerOnNumbers = m.HitTest(msg.X, msg.Y)

	// A press starts a sweep. A release on the same cell is a click, which
	// refines.
	if press {
		if gx, gy, ok := m.HitTest(msg.X, msg.Y); ok {
			m.CursorX, m.CursorY = gx, gy
			m.lastCursorX, m.lastCursorY = gx, gy
			clear(m.held)
			m.sweep = &sweepBox{ax: gx, ay: gy, bx: gx, by: gy}
			return
		}
		if idx, ok := m.Layout.BinAt(msg.X, msg.Y); ok {
			m.SelectedBin = m.nearestOpenBin(idx)
		}
		return
	}

	// Dragging: the ringfence follows the pointer and gathers as it goes.
	if m.sweep != nil && msg.Button == MouseLeft &&
		msg.Action == MouseMotion {
		if gx, gy, ok := m.HitTest(msg.X, msg.Y); ok {
			m.CursorX, m.CursorY = gx, gy
			m.lastCursorX, m.lastCursorY = gx, gy
			m.sweep.bx, m.sweep.by = gx, gy
			m.updateHover(gx, gy)
			m.takeSweep()
		}
		return
	}

	// Letting go. A sweep that never left its own cell was a click and refines.
	if msg.Action == MouseRelease {
		if s := m.sweep; s != nil {
			m.sweep = nil
			if s.ax == s.bx && s.ay == s.by {
				clear(m.held)
				m.refineAtCursor()
			}
		}
		return
	}

	// Motion with nothing held down: pointing at a cluster brings it up, and
	// makes it thrash harder.
	if gx, gy, ok := m.HitTest(msg.X, msg.Y); ok {
		m.CursorX, m.CursorY = gx, gy
		m.lastCursorX, m.lastCursorY = gx, gy
		m.updateHover(gx, gy)
	}
}

// finishIntro ends the boot sequence and drops the Intro, so it is no longer
// stepped and its decoded frames can be collected.
func (m *Model) finishIntro() {
	if m.Intro != nil && m.Intro.Stage() == stageLogo {
		m.cue(SoundHush, 1)
	}
	// A session with a board goes to it; one that must be asked its name goes
	// to the prompt; anyone else goes to the files. Checking for a board first
	// avoids sending a session with neither to the grid.
	switch {
	case m.Room != nil:
		// A local run. Join ran before the boot sequence, so start the arrival
		// animation now.
		m.arrive()
		m.Screen = ScreenMain
	case m.askName || m.rooms == nil:
		m.Screen = ScreenName
	default:
		m.Screen = ScreenLobby
		m.fadeFilesUp()
	}
	m.Intro = nil
}

// trackLens hands the lens to the keyboard when the cursor has moved, so
// whichever of the two inputs was used last is the one the lens follows.
func (m *Model) trackLens() {
	if m.CursorX == m.lastCursorX && m.CursorY == m.lastCursorY {
		return
	}
	m.lastCursorX, m.lastCursorY = m.CursorX, m.CursorY
	sx, sy := m.Layout.ScreenOf(m.CursorX, m.CursorY)
	m.Lens.LookAt(float64(sx), float64(sy))
}

// sweepBox is a ringfence being drawn: the cell it started from and the cell it
// has reached.
type sweepBox struct{ ax, ay, bx, by int }

// bounds is the rectangle it covers, in grid cells, either way round.
func (s sweepBox) bounds() (x0, y0, x1, y1 int) {
	return mini(s.ax, s.bx), mini(s.ay, s.by), maxi(s.ax, s.bx), maxi(s.ay, s.by)
}

// moveCursor moves within the grid and scrolls the viewport to follow. Plain
// movement releases the ringfence.
func (m *Model) moveCursor(dx, dy int) {
	clear(m.held)
	m.sweep = nil

	m.CursorX = clampInt(m.CursorX+dx, 0, GridSize-1)
	m.CursorY = clampInt(m.CursorY+dy, 0, GridSize-1)
	m.scrollToCursor()
	m.updateHover(m.CursorX, m.CursorY)
	if m.spectating {
		m.lookAround = lookAroundFor
	}
}

// moveCursorSweeping moves while drawing a ringfence (shift+arrow or the
// capitals), taking in whatever the rectangle now covers. A terminal reports no
// key release, so the sweep is committed as it grows.
func (m *Model) moveCursorSweeping(dx, dy int) {
	if m.sweep == nil {
		m.sweep = &sweepBox{ax: m.CursorX, ay: m.CursorY}
	}
	m.CursorX = clampInt(m.CursorX+dx, 0, GridSize-1)
	m.CursorY = clampInt(m.CursorY+dy, 0, GridSize-1)
	m.sweep.bx, m.sweep.by = m.CursorX, m.CursorY

	m.scrollToCursor()
	m.updateHover(m.CursorX, m.CursorY)
	m.takeSweep()
}

// takeSweep ringfences every awake cluster the current sweep touches. One cell
// inside the rectangle takes all of the cluster.
func (m *Model) takeSweep() {
	if m.Room == nil || m.sweep == nil {
		return
	}
	x0, y0, x1, y1 := m.sweep.bounds()
	for gy := y0; gy <= y1; gy++ {
		for gx := x0; gx <= x1; gx++ {
			if n := m.Grid.At(gx, gy); n != nil && m.IsActive(n.BadGroupID) {
				m.held[n.BadGroupID] = true
			}
		}
	}
}

// takeUnderCursor ringfences the cluster the cursor is on, if it is on one. A
// cell belonging to no awake cluster leaves the ringfence as it was.
func (m *Model) takeUnderCursor() {
	if m.Room == nil {
		return
	}
	if n := m.Grid.At(m.CursorX, m.CursorY); n != nil && m.IsActive(n.BadGroupID) {
		m.held[n.BadGroupID] = true
	}
}

// heldGroups is what is ringfenced, skipping anything that has gone, sorted by
// ID. The order decides which clusters fit when a bin fills, so map order would
// make the same sweep behave differently each time. lensList is sorted for the
// same reason.
func (m *Model) heldGroups() []*BadGroup {
	if len(m.held) == 0 {
		return nil
	}
	ids := make([]int, 0, len(m.held))
	for id := range m.held {
		if m.Grid.Group(id) != nil && m.IsActive(id) {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)

	out := make([]*BadGroup, 0, len(ids))
	for _, id := range ids {
		out = append(out, m.Grid.Group(id))
	}
	return out
}

func (m *Model) scrollToCursor() {
	l := m.Layout
	x, y := l.ViewportX, l.ViewportY
	if m.CursorX < x {
		x = m.CursorX
	}
	if m.CursorX >= x+l.ViewportW {
		x = m.CursorX - l.ViewportW + 1
	}
	if m.CursorY < y {
		y = m.CursorY
	}
	if m.CursorY >= y+l.ViewportH {
		y = m.CursorY - l.ViewportH + 1
	}
	if x == l.ViewportX && y == l.ViewportY {
		return
	}
	l.ScrollTo(x, y)

	// The board moved under everybody else's cursor, so re-project their lenses,
	// or they spring across the screen on every scroll.
	m.reprojectLenses()
}

// updateHover raises the active cluster to "super active" when the cursor is on
// it, matching the C++ reference's superActive state.
func (m *Model) updateHover(gx, gy int) {
	if m.Room == nil {
		return
	}
	want := -1
	if n := m.Grid.At(gx, gy); n != nil && m.IsActive(n.BadGroupID) {
		want = n.BadGroupID
	}
	if want == m.hoveredGroup {
		return
	}
	m.hoveredGroup = want
	// driveAgitation sets the level, including escalation; setting a bare
	// hover level here would lose the escalation until the next frame.
	m.driveAgitation()
}

// refineAtCursor refines the cluster under the cursor, if it is the active one.
func (m *Model) refineAtCursor() {
	if m.Room == nil {
		return
	}
	// A watcher cannot refine, by keyboard or mouse. Everything below gates
	// only on the room, and Room.Claim accepts a nil desk without attribution.
	if m.Desk == nil {
		return
	}
	// A finished file is being held on screen; there is nothing left to refine.
	if m.Room.Complete() {
		return
	}

	// Whatever is ringfenced is refined, instead of the cluster under the
	// cursor.
	if held := m.heldGroups(); len(held) > 0 {
		for _, grp := range held {
			// Checked per cluster, so a sweep that would overfill the bin
			// refines what fits and leaves the rest ringfenced.
			if m.Grid.Bins[m.SelectedBin].Full() {
				break
			}
			delete(m.held, grp.ID)
			m.refine(grp)
		}
		m.sweep = nil
		return
	}

	n := m.Grid.At(m.CursorX, m.CursorY)
	if n == nil {
		return
	}
	// The cluster may have just been refined by somebody else. If so, this
	// reach still counts.
	if n.BadGroupID < 0 {
		if m.Room.CoRefine(GridIndex(m.CursorX, m.CursorY), m.Desk) {
			m.bindName()
		}
		return
	}
	grp := m.Grid.Group(n.BadGroupID)
	if grp == nil || !m.IsActive(grp.ID) {
		return
	}
	// A bin that has reached its quota takes no more.
	if m.Grid.Bins[m.SelectedBin].Full() {
		return
	}

	// Release it, or its cells, now ordinary digits, would stay lifted.
	delete(m.held, grp.ID)
	m.refine(grp)
}

// refine releases a cluster. Every digit leaves at once, at the size it already
// had, and shrinks and straightens on the way. Shrinking first would leave it
// sitting still in its cell for about a quarter of a second.
func (m *Model) refine(grp *BadGroup) {
	ids, digits, seq := m.Room.refineInto(grp, m.SelectedBin, m.Desk)
	m.launchDigits(m.SelectedBin, ids, digits, true)
	m.bindName()

	// Marked as seen, since this view already launched it.
	m.depositSeen = seq
	m.syncAgitation()
}

// CompletionDebug enables a key that finishes a file at once, for testing the
// completion. It is set once from --completion-debug at startup, like
// canvas.NoBold, and read everywhere after; it is process-wide, so every remote
// session agrees.
var CompletionDebug bool

// completionDebugKey is what finishes the file when the flag is on.
const completionDebugKey = "ctrl+f"

// finishFileNow refines every cluster left on the board, through refine, so
// everything a normal finish does (flights, delayed credit, the drawer,
// deposits, the notice) actually runs.
//
// It moves to another bin as each fills. Credit lands after creditDelay, so
// Bin.Refined has not changed yet during this loop; it counts what it has
// handed out alongside what has landed.
func (m *Model) finishFileNow() {
	if m.Room == nil {
		return
	}
	var handed [BinCount]int
	for _, grp := range append([]*BadGroup(nil), m.Grid.Groups...) {
		bin := -1
		for i, b := range m.Grid.Bins {
			if b.Refined+handed[i] < b.Quota {
				bin = i
				break
			}
		}
		if bin < 0 {
			return // every bin has taken what it is owed
		}
		m.SelectedBin = bin
		handed[bin]++
		delete(m.held, grp.ID)
		m.refine(grp)
	}
}

// nearestOpenBin is the bin nearest to want that still has room, ties going to
// the lower index, or want itself if every bin is full. A full bin refuses
// refines, so leaving it selected would make the refine key silently do
// nothing.
func (m *Model) nearestOpenBin(want int) int {
	if m.Room == nil || want < 0 || want >= BinCount {
		return want
	}
	if !m.Grid.Bins[want].Full() {
		return want
	}
	for d := 1; d < BinCount; d++ {
		// Left first, so an equal distance either side takes the lower index.
		if i := want - d; i >= 0 && !m.Grid.Bins[i].Full() {
			return i
		}
		if i := want + d; i < BinCount && !m.Grid.Bins[i].Full() {
			return i
		}
	}
	return want
}

// launchDigits puts a cluster's digits in the air toward a bin. The refiner and
// every view replaying the deposit share it, so they animate a refinement the
// same way. It touches only this terminal's flights. mine says whether it is
// this refiner's own. Its sound plays halfway, apart from the lid's.
func (m *Model) launchDigits(bin int, ids []int, digits []rune, mine bool) {
	l := m.Layout

	// In 3D, where the volume draws each digit, so flights leave from there.
	solidAt := m.solidAnchors()

	target := physics.Vec2{
		X: float64(l.BinCenterX[bin]),
		Y: float64(l.BinTop),
	}

	cluster := make([]Departure, 0, len(ids))
	for i, id := range ids {
		gx, gy := id%GridSize, id/GridSize

		// Each member leans its own way, deterministically from its index and
		// position, so the cluster comes apart raggedly.
		tilt := MaxCollapseTilt * (0.45 + 0.55*float64((i*7+gx+gy)%5)/4)
		if (i+gx)%2 == 0 {
			tilt = -tilt
		}

		// Read the digit's displacement now: once agitation is released the
		// body is gone, and it would hold at its bare cell centre.
		ox, oy := l.Screen(m.Agitation.Offset(id))

		// Launch from where the digit appears, agitation included.
		sx, sy := l.ScreenOf(gx, gy)
		from := physics.Vec2{
			X: float64(sx) + ox,
			Y: float64(sy) + oy,
		}
		if at, ok := solidAt[id]; ok {
			// The projection already includes agitation and drift, so it
			// replaces the flat position.
			from = at
		}

		cluster = append(cluster, Departure{
			GridID: id,
			Digit:  digits[i],
			From:   from,
			To:     target,
			BinIdx: bin,
			Height: m.Magnify.Height(gx, gy),
			Tilt:   tilt,
			Mine:   mine,
		})
		m.Magnify.Drop(id)
	}
	m.Flights.Launch(cluster...)
}

// catchDeposits flies every refinement this view has not flown yet.
//
// It runs in stepBoard, not stepView, for two reasons. It must run before
// syncAgitation releases the retired cluster's bodies, or Agitation.Velocity
// returns nothing and every digit leaves from rest. And stepView does not run
// during a diversion or on the scoreboard, so stale deposits would pile up and
// fly all at once on return.
//
// A view not looking at the grid marks deposits seen without flying them:
// Flights.Step runs only in stepView, so a flight launched then would hang in
// the air and hold the bin's lid open. Ripple.Missed does the same.
func (m *Model) catchDeposits() {
	if m.Room == nil {
		return
	}
	for i := range m.deposits {
		d := &m.deposits[i]
		if d.Seq <= m.depositSeen {
			continue
		}
		m.depositSeen = d.Seq
		if m.Screen != ScreenMain || m.Diverting() {
			continue
		}
		m.launchDigits(d.Bin, d.IDs, d.Digits, false)
	}
}

// clusterCentroid is the average grid position of a cluster's members.
func clusterCentroid(grp *BadGroup) physics.Vec2 { return centroidOf(grp.NumberIDs) }

// centroidOf is the average grid position of a run of cells. It takes IDs
// because a view replaying a deposit no longer has the group: Retire removed it.
func centroidOf(ids []int) physics.Vec2 {
	var sum physics.Vec2
	for _, id := range ids {
		sum = sum.Add(physics.Vec2{X: float64(id % GridSize), Y: float64(id / GridSize)})
	}
	if len(ids) == 0 {
		return sum
	}
	return sum.Scale(1 / float64(len(ids)))
}

// stepArrival advances the board coming up, and drops it when done. cellAttr
// checks it for every visible digit each frame, so a settled board should pay
// only a nil check.
func (m *Model) stepArrival(dt float64) {
	if m.arrival == nil {
		return
	}
	m.arrival.Step(dt)
	if m.arrival.Done() {
		m.arrival = nil
	}
}
