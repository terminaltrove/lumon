package lumon

import (
	"math"
	"math/bits"
	"math/rand"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/terminaltrove/lumon/internal/canvas"
)

// soundsOf is the sounds among effects.
func soundsOf(effects []Effect) []Sound {
	var out []Sound
	for _, e := range effects {
		if e.Kind == EffectSound {
			out = append(out, e.Sound)
		}
	}
	return out
}

// frameSounds runs one frame and returns the sounds it cued.
func frameSounds(m *Model) []Sound { return soundsOf(m.UpdateEvent(FrameMsg(frameInterval))) }

// bootSounds runs m's boot sequence to its end and returns what each frame
// cued, and the stage the sequence was in before that frame.
func bootSounds(m *Model) (heard []Sound, stages []introStage) {
	for i := 0; i < 60*30 && m.Screen == ScreenIntro; i++ {
		stage := m.Intro.Stage()
		for _, s := range frameSounds(m) {
			heard, stages = append(heard, s), append(stages, stage)
		}
	}
	return heard, stages
}

func TestTheStartupSoundBeginsAsTheLogoDoes(t *testing.T) {
	heard, stages := bootSounds(introReady())

	if !slices.Equal(heard, []Sound{SoundBeep, SoundStartup}) {
		t.Fatalf("a whole boot sequence cued %v, want a beep and the startup sound", heard)
	}
	if stages[1] != stageBoot {
		t.Error("the startup sound was not cued on the frame the logo started")
	}
}

func TestCuttingTheLogoShortHushesItsSound(t *testing.T) {
	m := introReady()
	for i := 0; i < 60*30 && m.Intro.Stage() == stageBoot; i++ {
		frameSounds(m)
	}
	frameSounds(m)

	got := soundsOf(m.UpdateEvent(KeyMsg{Key: "x", Runes: []rune{'x'}}))

	if !slices.Equal(got, []Sound{SoundHush}) {
		t.Errorf("skipping the logo cued %v, want a hush", got)
	}
}

func TestSkippingTheLogLeavesNothingToHush(t *testing.T) {
	m := introReady()
	frameSounds(m)

	if got := soundsOf(m.UpdateEvent(KeyMsg{Key: "x", Runes: []rune{'x'}})); len(got) > 0 {
		t.Errorf("skipping the log before the logo cued %v", got)
	}
}

func TestAFinishedBootLetsItsSoundRunOut(t *testing.T) {
	heard, _ := bootSounds(introReady())
	if slices.Contains(heard, SoundHush) {
		t.Error("a boot sequence that ran to the end hushed its own sound")
	}
}

// darkSet is a browser's model before the refiner has switched it on.
func darkSet() *Model {
	m := NewModel(1)
	m.Resize(120, 40)
	m.StartOff()
	return m
}

func TestTheDarkSetWaitsSilentlyToBeSwitchedOn(t *testing.T) {
	m := darkSet()
	for range 60 * 5 {
		if got := frameSounds(m); len(got) > 0 {
			t.Fatalf("the dark set cued %v", got)
		}
	}
	if m.Screen != ScreenOff {
		t.Fatalf("the set switched itself on to %v", m.Screen)
	}
	if !strings.Contains(screenText(m), "PRESS ANY KEY TO POWER ON") {
		t.Error("the dark set does not say how to switch it on")
	}
}

func TestAnyKeyOrClickSwitchesTheSetOn(t *testing.T) {
	m := darkSet()
	m.UpdateEvent(KeyMsg{Key: "x", Runes: []rune{'x'}})
	if m.Screen != ScreenIntro {
		t.Errorf("a key left the set on %v, not the boot sequence", m.Screen)
	}

	m = darkSet()
	m.UpdateEvent(MouseMsg{X: 3, Y: 3, Button: MouseLeft, Action: MousePress})
	if m.Screen != ScreenIntro {
		t.Errorf("a click left the set on %v, not the boot sequence", m.Screen)
	}
}

func TestASetSwitchedOnBeepsAsItsLogStartsThenPlaysItsStartupSound(t *testing.T) {
	m := darkSet()
	frameSounds(m)
	if got := soundsOf(m.UpdateEvent(KeyMsg{Key: "x", Runes: []rune{'x'}})); len(got) > 0 {
		t.Errorf("the press that switched the set on cued %v", got)
	}
	heard, stages := bootSounds(m)
	if !slices.Equal(heard, []Sound{SoundBeep, SoundStartup}) {
		t.Fatalf("the boot cued %v, want a beep and then the startup sound", heard)
	}
	if stages[0] != stageBoot {
		t.Errorf("the beep came in stage %v, not as the log started", stages[0])
	}
}

// The log's first lines are drawn on the frame that beeps, not before.
func TestTheBeepComesWithTheLogsFirstLines(t *testing.T) {
	m := darkSet()
	m.UpdateEvent(KeyMsg{Key: "x", Runes: []rune{'x'}})
	for range 60 * 3 {
		before := screenText(m)
		if slices.Contains(frameSounds(m), SoundBeep) {
			if strings.TrimSpace(before) != "" {
				t.Errorf("the log was showing before the beep:\n%s", before)
			}
			if strings.TrimSpace(screenText(m)) == "" {
				t.Error("the beep came on a blank screen")
			}
			return
		}
	}
	t.Fatal("the set never beeped")
}

func TestTheDarkSetsCursorSitsASpaceAfterItsPrompt(t *testing.T) {
	m := darkSet()
	buf := canvas.NewBuffer(120, 40)
	m.Draw(buf)
	for y := range 40 {
		i := strings.Index(buf.Row(y), "POWER ON")
		if i < 0 {
			continue
		}
		end := i + len("POWER ON")
		if buf.AttrAt(end, y) == m.Theme.BootCursor || buf.AttrAt(end+1, y) != m.Theme.BootCursor {
			t.Errorf("the cursor is not a space after the prompt: %q", buf.Row(y))
		}
		return
	}
	t.Fatal("no prompt on the dark set")
}

// The boot's first cursor, and the log after it, sit off the screen's edge.
func TestTheBootsFirstCursorIsKeptOffTheEdge(t *testing.T) {
	m := darkSet()
	m.UpdateEvent(KeyMsg{Key: "x", Runes: []rune{'x'}})
	for range 60 * 3 {
		frameSounds(m)
		buf := canvas.NewBuffer(120, 40)
		m.Draw(buf)
		for y := range 40 {
			for x := range 120 {
				if buf.AttrAt(x, y) != m.Theme.BootCursor {
					continue
				}
				if x < 1 || y < 1 {
					t.Errorf("the boot's first cursor is at column %d, row %d, touching the edge", x, y)
				}
				return
			}
		}
	}
	t.Fatal("the boot never showed its cursor")
}

// The browser hears the numbers while they show, louder with the pointer over
// them.
func TestTheNumbersAreHeardWhileTheyShowAndLouderUnderThePointer(t *testing.T) {
	if darkSet().NumbersShowing() {
		t.Error("the dark set shows numbers")
	}
	m := ready()
	if !m.NumbersShowing() || m.PointerOnNumbers() {
		t.Fatalf("on the board: showing %v, pointer on them %v", m.NumbersShowing(), m.PointerOnNumbers())
	}
	l := m.Layout
	x, y := l.ScreenOf(l.ViewportX+l.ViewportW/2, l.ViewportY+l.ViewportH/2)
	m.UpdateEvent(MouseMsg{X: x, Y: y, Action: MouseMotion})
	if !m.PointerOnNumbers() {
		t.Error("the pointer is over the numbers and the game does not know it")
	}
	m.PointerLeft()
	if m.PointerOnNumbers() {
		t.Error("the pointer left the screen and is still over the numbers")
	}
	m.UpdateEvent(MouseMsg{X: x, Y: y, Action: MouseMotion})
	m.UpdateEvent(MouseMsg{X: x, Y: l.BinTop + 1, Action: MouseMotion})
	if m.PointerOnNumbers() {
		t.Error("the pointer moved onto the bins and is still over the numbers")
	}
}

// Where computer refiners are allowed, c clicks as it calls one.
func TestCallingCompanyClicks(t *testing.T) {
	m := ready()
	if volumeOf(cuesOf(m.UpdateEvent(KeyMsg{Key: "c", Runes: []rune{'c'}})), SoundClick) >= 0 {
		t.Error("c clicked where there is no company to call")
	}
	m = ready()
	m.AllowBots(0)
	if v := volumeOf(cuesOf(m.UpdateEvent(KeyMsg{Key: "c", Runes: []rune{'c'}})), SoundClick); v <= 0 || v > 0.7 {
		t.Errorf("c was cued at %v", v)
	}
}

// A detonation is heard once, quietly, by everyone at the board.
func TestADetonationIsHeardOnce(t *testing.T) {
	_, a, b := twoInARoom(t)
	grp := activateFirstVisibleGroup(a)
	if grp == nil {
		t.Skip("no cluster to detonate")
	}
	a.Room.mu.Lock()
	a.Room.Detonate(grp, rand.New(rand.NewSource(1)).Intn)
	a.Room.mu.Unlock()
	for _, m := range []*Model{a, b} {
		cues := heard(m, 30)
		n := 0
		for _, c := range cues {
			if c.Sound == SoundBlast {
				n++
				if c.Volume > 0.6 {
					t.Errorf("a detonation sounded at %v", c.Volume)
				}
			}
		}
		if n != 1 {
			t.Errorf("a detonation was heard %d times", n)
		}
	}
}

// A click sounds, quietly, wherever it lands; a key or the mouse moving
// does not.
func TestAClickClicks(t *testing.T) {
	click := MouseMsg{X: 3, Y: 3, Button: MouseLeft, Action: MousePress}
	for name, m := range map[string]*Model{"the dark set": darkSet(), "the board": ready()} {
		if v := volumeOf(cuesOf(m.UpdateEvent(click)), SoundClick); v <= 0 || v > 0.7 {
			t.Errorf("a click on %s was cued at %v", name, v)
		}
	}
	m := ready()
	for _, msg := range []any{
		KeyMsg{Key: "x", Runes: []rune{'x'}},
		MouseMsg{X: 5, Y: 5, Action: MouseMotion},
		MouseMsg{X: 5, Y: 5, Button: MouseLeft, Action: MouseRelease},
	} {
		if volumeOf(cuesOf(m.UpdateEvent(msg)), SoundClick) >= 0 {
			t.Errorf("%+v clicked", msg)
		}
	}
}

// The chime rings once each time the header's percent rises, and not for a
// file just opened.
func TestTheChimeRingsWhenTheFileGetsFurther(t *testing.T) {
	m := ready()
	if slices.Contains(frameSounds(m), SoundChime) {
		t.Fatal("a file just opened chimed")
	}
	m.Room.mu.Lock()
	m.Grid.Bins[0].Refined += m.Grid.Bins[0].Quota / 2
	m.Room.mu.Unlock()
	if v := volumeOf(heard(m, 1), SoundChime); v < 0 || v > 0.6 {
		t.Errorf("the percent rose and the chime was cued at %v", v)
	}
	if slices.Contains(frameSounds(m), SoundChime) {
		t.Error("one rise chimed twice")
	}
}

func TestNullChimesOnlyWhenItIsDone(t *testing.T) {
	m := onFile(t, nullFile)
	frameSounds(m)
	m.Room.mu.Lock()
	m.Grid.Bins[0].Refined += m.Grid.Bins[0].Quota / 2
	m.Room.mu.Unlock()
	if slices.Contains(frameSounds(m), SoundChime) {
		t.Error("Null chimed while its header read 0%")
	}
}

// ambience runs the set's own sounds for seconds and returns when each of
// s was cued, and its loudest volume.
func ambience(m *Model, s Sound, seconds float64) (at []float64, loudest float64) {
	const dt = 0.1
	for t := 0.0; t < seconds; t += dt {
		m.stepAmbience(dt)
		if v := volumeOf(m.sounds, s); v >= 0 {
			at, loudest = append(at, t), max(loudest, v)
		}
		m.sounds = m.sounds[:0]
	}
	return at, loudest
}

// beepTimes runs m's beeps for seconds and returns when each sounded.
func beepTimes(m *Model, seconds float64) []float64 {
	at, _ := ambience(m, SoundBeep, seconds)
	return at
}

func TestTheSetIsFaintlyHeardWorkingVeryRarely(t *testing.T) {
	at, loudest := ambience(ready(), SoundProcess, 24*3600)
	if n := len(at); n < 60 || n > 200 {
		t.Errorf("a day brought the set's working %d times, want about once in %.0fs", n, processMean)
	}
	if loudest > 0.2 {
		t.Errorf("the set's working was cued at %v, want it faint", loudest)
	}
	for _, m := range []*Model{darkSet(), introReady()} {
		if at, _ := ambience(m, SoundProcess, 24*3600); len(at) > 0 {
			t.Errorf("a set on %v was heard working", m.Screen)
		}
	}
}

func TestABoxOpeningOverTheScreenPops(t *testing.T) {
	m := ready()
	heard(m, 2)
	m.UpdateEvent(KeyMsg{Key: "?", Runes: []rune{'?'}})
	if !m.Helping {
		t.Fatal("? did not open the controls")
	}
	if got := soundsOf(heard(m, 1)); !slices.Equal(got, []Sound{SoundPop}) {
		t.Errorf("the controls opening cued %v, want a pop", got)
	}
	if got := heard(m, 30); len(got) > 0 {
		t.Errorf("the open controls went on to cue %v", soundsOf(got))
	}
	m.UpdateEvent(KeyMsg{Key: "?", Runes: []rune{'?'}})
	if got := heard(m, 30); len(got) > 0 {
		t.Errorf("closing the controls cued %v", soundsOf(got))
	}
}

func TestTheSetBeepsRarelyAndNeverSoonAfterItsBoot(t *testing.T) {
	m := ready()
	at := beepTimes(m, 6*3600)

	if len(at) < 10 || len(at) > 60 {
		t.Errorf("six hours brought %d beeps, want a handful an hour", len(at))
	}
	last := 0.0
	gaps := map[int]bool{}
	for _, t0 := range at {
		gap := t0 - last
		if gap < beepFirst || gap > beepLongest+1 {
			t.Errorf("a beep came %.0fs after the last, want between %.0fs and %.0fs", gap, beepFirst, beepLongest)
		}
		gaps[int(gap/30)] = true
		last = t0
	}
	if len(gaps) < 5 {
		t.Error("the beeps come at a steady beat, not at random")
	}
}

func TestTheDarkSetAndTheBootNeverBeepOnTheirOwn(t *testing.T) {
	for _, m := range []*Model{darkSet(), introReady()} {
		if at := beepTimes(m, 3600); len(at) > 0 {
			t.Errorf("a set on %v beeped at %v", m.Screen, at)
		}
	}
}

func TestEachCardTheDrawerPassesFlicks(t *testing.T) {
	m, _ := lobbyAt(t, 120, 40, viewRolodex)
	if m.rolodex().Len() < 2 {
		t.Skip("one file in the drawer")
	}
	if got := heard(m, 60); len(got) > 0 {
		t.Fatalf("a still drawer cued %v", soundsOf(got))
	}
	m.UpdateEvent(KeyMsg{Key: "down"})
	got := soundsOf(heard(m, 60))
	if len(got) == 0 || slices.ContainsFunc(got, func(s Sound) bool { return s != SoundCard }) {
		t.Errorf("turning the drawer a card cued %v, want card flicks", got)
	}
}

// logoSeconds is how long a boot sequence's logo stage runs, until the mark's
// last frame shows.
func logoSeconds(m *Model) float64 {
	steps := 0
	for i := 0; i < 60*60 && m.Screen == ScreenIntro; i++ {
		if m.Intro.Stage() == stageLogo && m.Intro.Frame() < len(logoFrames())-1 {
			steps++
		}
		frameSounds(m)
	}
	return float64(steps) / 60
}

func TestAFittedLogoLastsAsLongAsItsSound(t *testing.T) {
	m := NewModel(1)
	m.Resize(120, 40)
	m.FitIntroTo(6500 * time.Millisecond)
	m.StartIntro()

	got := logoSeconds(m)
	want := 6.5 * float64(len(logoFrames())-1) / float64(len(logoFrames()))
	if math.Abs(got-want) > .1 {
		t.Errorf("a logo fitted to 6.5s animates for %.2fs, want about %.2fs", got, want)
	}
	if plain := logoSeconds(introReady()); math.Abs(plain-want) < 1 {
		t.Errorf("an unfitted logo also takes %.2fs; only the browser's should slow", plain)
	}
}

func TestAMorphMovesBetweenFramesADotAtATime(t *testing.T) {
	i := 20
	from, to := LogoFrame(i), LogoFrame(i+1)
	if !slices.Equal(LogoMorph(i, 0), from) || !slices.Equal(LogoMorph(i, 1), to) {
		t.Fatal("a morph does not start at its frame and end at the next")
	}
	half := LogoMorph(i, .5)
	differ, switched := 0, 0
	for c := range half {
		a, b, h := from[c]-'⠀', to[c]-'⠀', half[c]-'⠀'
		if h&(a&b) != a&b || h&^(a|b) != 0 {
			t.Fatalf("cell %d halfway shows dots neither frame has, or drops ones both do", c)
		}
		differ += bits.OnesCount32(uint32(a ^ b))
		switched += bits.OnesCount32(uint32((a ^ h) & (a ^ b)))
	}
	if differ == 0 {
		t.Fatal("the two frames are the same; pick another pair")
	}
	if share := float64(switched) / float64(differ); share < .3 || share > .7 {
		t.Errorf("halfway through, %.0f%% of the changing dots have changed", 100*share)
	}
}

// cuesOf is the sound effects among effects.
func cuesOf(effects []Effect) []Effect {
	var out []Effect
	for _, e := range effects {
		if e.Kind == EffectSound {
			out = append(out, e)
		}
	}
	return out
}

// heard runs n frames and returns every sound they cued.
func heard(m *Model, n int) []Effect {
	var out []Effect
	for range n {
		out = append(out, cuesOf(m.UpdateEvent(FrameMsg(frameInterval)))...)
	}
	return out
}

// volumeOf is how loud s was cued among cues, or -1 if it was not.
func volumeOf(cues []Effect, s Sound) float64 {
	for _, c := range cues {
		if c.Sound == s {
			return c.Volume
		}
	}
	return -1
}

// refineWithSound refines the first cluster m can reach into bin, and returns
// the sounds the key press cued.
func refineWithSound(t *testing.T, m *Model, bin int) []Effect {
	t.Helper()
	grp := activateFirstVisibleGroup(m)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	id := grp.NumberIDs[0]
	m.CursorX, m.CursorY = id%GridSize, id/GridSize
	m.SelectedBin = bin
	return cuesOf(m.UpdateEvent(KeyMsg{Key: "enter"}))
}

// A refined cluster sounds halfway to its bin, frames after the lid opens
// for it, so the two are heard apart.
func TestARefinedClusterSoundsHalfwayToItsBin(t *testing.T) {
	_, a, b := twoInARoom(t)

	if v := volumeOf(refineWithSound(t, a, 2), SoundRefine); v >= 0 {
		t.Error("the cluster sounded as it left")
	}
	type ear struct {
		who             string
		m               *Model
		want            func(float64) bool
		opened, sounded int
	}
	ears := []*ear{
		{"the refiner", a, func(v float64) bool { return v == 1 }, -1, -1},
		{"the other refiner", b, func(v float64) bool { return v > 0 && v < 1 }, -1, -1},
	}
	for frame := range 60 * 3 {
		for _, e := range ears {
			cues := heard(e.m, 1)
			if e.opened < 0 && volumeOf(cues, SoundBinOpen) >= 0 {
				e.opened = frame
			}
			if v := volumeOf(cues, SoundRefine); v >= 0 {
				if e.sounded >= 0 {
					t.Errorf("%s heard one cluster twice", e.who)
				}
				e.sounded = frame
				if !e.want(v) {
					t.Errorf("%s heard the cluster at volume %v", e.who, v)
				}
			}
		}
	}
	for _, e := range ears {
		if e.sounded < 0 || e.opened < 0 || e.sounded-e.opened < 10 {
			t.Errorf("%s: the lid opened on frame %d and the cluster sounded on %d", e.who, e.opened, e.sounded)
		}
	}
}

func TestALidSoundsAsItSwingsOpenAndShut(t *testing.T) {
	_, a, b := twoInARoom(t)
	refineWithSound(t, a, 1)

	var mine, theirs []Effect
	for range 60 * 6 {
		mine = append(mine, heard(a, 1)...)
		theirs = append(theirs, heard(b, 1)...)
	}
	for _, c := range []struct {
		who  string
		cues []Effect
		want float64
	}{{"the refiner", mine, 1}, {"the other refiner", theirs, farVolume}} {
		var lids []Sound
		for _, cue := range c.cues {
			if cue.Sound == SoundBinOpen || cue.Sound == SoundBinClose {
				lids = append(lids, cue.Sound)
				if cue.Volume != c.want {
					t.Errorf("%s heard a lid at volume %v, want %v", c.who, cue.Volume, c.want)
				}
			}
		}
		if !slices.Equal(lids, []Sound{SoundBinOpen, SoundBinClose}) {
			t.Errorf("%s heard the lid %v, want it open and then shut", c.who, lids)
		}
	}
}

func TestALidOpenedByHandSoundsBothWays(t *testing.T) {
	m := ready()
	m.UpdateEvent(KeyMsg{Key: "o", Runes: []rune{'o'}})
	if v := volumeOf(heard(m, 1), SoundBinOpen); v != 1 {
		t.Errorf("opening a lid by hand sounded at %v, want 1", v)
	}
	// It shuts a moment after.
	m.UpdateEvent(KeyMsg{Key: "o", Runes: []rune{'o'}})
	if v := volumeOf(heard(m, 90), SoundBinClose); v != 1 {
		t.Errorf("shutting it by hand sounded at %v, want 1", v)
	}
	if got := heard(m, 60); len(got) > 0 {
		t.Errorf("a shut lid went on to cue %v", soundsOf(got))
	}
}
