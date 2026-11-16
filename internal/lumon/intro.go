package lumon

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"math"
	"math/rand"
	"strings"

	"github.com/terminaltrove/lumon/internal/ease"
	"sync"
)

// The boot sequence that plays before the grid appears: a fake POST log that
// scrolls past, then the Lumon logo drawing itself on the braille layer.
// It runs on the fixed-step Clock, so it takes the same number of steps
// whatever the terminal's frame rate.
type introStage int

const (
	stageBoot introStage = iota
	stageLogo
	stageDone
)

// Intro timing, in seconds.
const (
	// bootLineRate is how fast the log fills, in lines per second: quick
	// enough to be brisk, slow enough to read as a sequence.
	bootLineRate = 135.0

	// bootTypeRate is how fast the one typewritten line types, in characters
	// per second. It moves with bootLineRate.
	bootTypeRate = 135.0

	// bootHold is how long the finished log sits before the logo takes over.
	bootHold = 0.25

	// logoFrameTime is how long each frame of the logo animation shows. It is
	// slower than the source GIF's 40ms (the header's delay is read but not
	// used), so the ninety frames take four and a half seconds instead of
	// three and a half.
	logoFrameTime = 0.05

	// cursorBlinkRate is how long each half of the cursor blink lasts.
	cursorBlinkRate = 0.12

	// wakeTime is how long a set just switched on shows only a cursor in the
	// corner before the log starts, and wakeBlink each half of its blink.
	wakeTime  = 1.0
	wakeBlink = 0.25

	// logoFadeIn is how long the mark's ink takes to come up out of the dark.
	// It dims the ink, not the frames, so it overlaps the animation drawing
	// itself.
	logoFadeIn = 0.6

	// logoHold is how long the screen stays dark after the mark has gone. The
	// last dozen frames dissolve the mark and frame 89 is blank; without the
	// hold the file drawer appears the instant the last dot goes out.
	logoHold = 0.75
)

// authorizeLine is the one log line that types out instead of appearing whole.
const authorizeLine = "> AUTHORIZE TERMINAL ACCESS"

// Intro is the boot sequence's state.
type Intro struct {
	lines []string

	revealed float64 // lines revealed so far, fractional
	typed    float64 // characters typed of the authorize line
	held     float64 // seconds the finished log has been held
	wake     float64 // seconds of cursor alone left before the log starts

	// frameTime is how long each frame of the logo shows; morph, whether the
	// logo moves from one frame to the next dot by dot instead of at once.
	frameTime float64
	morph     bool
	dark      float64 // seconds the screen has been black after the mark
	elapsed   float64 // seconds in the current stage

	stage introStage
	frame int

	// authorizeAt is where the typewritten line sits in the log, or -1 if it
	// is missing. The log reveals several lines a frame, so it needs to know
	// where to stop.
	authorizeAt int

	// markAt is where the mark's first line sits in the log, or -1 if it is
	// missing. The log has the mark as five braille lines; a terminal that can
	// draw the picture as an image placement needs to know which rows those are.
	markAt int
}

// NewIntro builds a boot sequence. The rng fixes which optional lines appear, so
// a seeded run always boots the same way.
func NewIntro(rng *rand.Rand) *Intro {
	in := &Intro{lines: bootLog(rng), authorizeAt: -1, markAt: -1, frameTime: logoFrameTime}
	for i, line := range in.lines {
		if line == authorizeLine && in.authorizeAt < 0 {
			in.authorizeAt = i
		}
		if line == SmallLogo[0] && in.markAt < 0 {
			in.markAt = i
		}
	}
	return in
}

// FitLogo spreads the logo's animation over seconds, morphing between its
// frames so a slower animation stays smooth.
func (in *Intro) FitLogo(seconds float64) {
	if n := len(logoFrames()); n > 0 && seconds > 0 {
		in.frameTime = seconds / float64(n)
		in.morph = true
	}
}

// Between is how far the logo is from its frame to the next, from 0 to 1,
// and whether it morphs at all.
func (in *Intro) Between() (float64, bool) {
	if !in.morph || in.stage != stageLogo {
		return 0, false
	}
	t := in.elapsed/in.frameTime - float64(in.frame)
	return math.Max(0, math.Min(1, t)), true
}

// WakeFirst holds the log back for wakeTime, showing only a blinking cursor
// in the corner, as a set just switched on does.
func (in *Intro) WakeFirst() { in.wake = wakeTime }

// Waking reports whether the set is still waking, and so whether its cursor
// is lit.
func (in *Intro) Waking() (waking, lit bool) {
	if in.stage != stageBoot || in.wake <= 0 {
		return false, false
	}
	return true, int((wakeTime-in.wake)/wakeBlink)%2 == 0
}

// MarkAt is which log line the mark starts on, or -1 if it is not in the log.
func (in *Intro) MarkAt() int { return in.markAt }

// Done reports whether the intro has finished and the grid should take over.
func (in *Intro) Done() bool { return in.stage == stageDone }

// Skip ends the intro at once.
func (in *Intro) Skip() { in.stage = stageDone }

// Stage reports which part of the sequence is playing.
func (in *Intro) Stage() introStage { return in.stage }

// Lines returns the log lines revealed so far, the last of them possibly
// half-typed.
func (in *Intro) Lines() []string {
	n := int(in.revealed)
	if n > len(in.lines) {
		n = len(in.lines)
	}
	out := in.lines[:n]

	// The authorize line is appended separately while it is being typed.
	if in.Typing() {
		typed := int(in.typed)
		if typed > len(authorizeLine) {
			typed = len(authorizeLine)
		}
		out = append(append([]string{}, out...), authorizeLine[:typed])
	}
	return out
}

// Typing reports whether the authorize line is mid-type, which is the only time
// the cursor shows.
func (in *Intro) Typing() bool {
	return in.authorizeAt >= 0 && int(in.revealed) == in.authorizeAt
}

// CursorOn reports whether the block cursor is currently lit.
func (in *Intro) CursorOn() bool {
	return int(in.elapsed/cursorBlinkRate)%2 == 0
}

// Frame is which frame of the logo animation is showing.
func (in *Intro) Frame() int { return in.frame }

// LogoFade is how far up the mark's brightness has come, from 0 to 1. It is 1
// outside the logo stage, so a caller can always multiply by it.
func (in *Intro) LogoFade() float64 {
	if in.stage != stageLogo {
		return 1
	}
	return ease.Smoothstep(ease.Clamp01(in.elapsed / logoFadeIn))
}

// Step advances the sequence.
func (in *Intro) Step(dt float64) {
	in.elapsed += dt

	switch in.stage {
	case stageBoot:
		if in.wake > 0 {
			in.wake -= dt
			return
		}
		if in.Typing() {
			in.typed += dt * bootTypeRate
			if int(in.typed) >= len(authorizeLine) {
				in.revealed++
				in.typed = 0
			}
			return
		}

		next := in.revealed + dt*bootLineRate

		// Stop exactly on the authorize line.
		if at := float64(in.authorizeAt); in.authorizeAt >= 0 && in.revealed < at && next > at {
			next = at
		}
		in.revealed = next

		if int(in.revealed) < len(in.lines) {
			return
		}
		in.revealed = float64(len(in.lines))
		in.held += dt
		if in.held >= bootHold {
			in.stage = stageLogo
			in.elapsed = 0
		}

	case stageLogo:
		n := len(logoFrames())
		if n == 0 {
			in.stage = stageDone
			return
		}
		if f := int(in.elapsed / in.frameTime); f != in.frame {
			in.frame = f
		}
		if in.frame >= n {
			// The last frame is blank, so holding on it is holding on black.
			in.frame = n - 1
			in.dark += dt
			if in.dark >= logoHold {
				in.stage = stageDone
			}
		}
	}
}

// The logo animation, embedded: 90 frames of braille, gzipped (about 29KB).
//
// Regenerating it from `lumon_2.gif` (1462x752, 90 frames at 40ms):
//
//	magick lumon_2.gif -coalesce frames/%03d.png
//	for f in frames/*.png; do uniart "$f" -m braille -b -f -w 96; done
//
// then join each frame's 24 rows into one line, prepend a `90 96 24 40` header
// (count, width, height, delay in ms), and gzip. Notes on the pipeline:
//   - uniart plays a GIF instead of dumping it (and panics without a
//     terminal), so the frames are extracted first.
//   - -coalesce is required: GIF frames are delta-encoded.
//   - The height is whatever uniart's flooring gives for the width (24 at 96,
//     41 at 160).
//   - Pass the frames at full resolution and let uniart dither. Pre-scaling
//     and thresholding (as SmallLogo does) turns the thin strokes solid.
//
// The width is fixed at 96 cells on every terminal; one too narrow gets
// SmallLogo. At 32 cells uniart's dithering turns to noise, so there is no
// smaller animated size.
//
//go:embed assets/lumon.braille.gz
var logoAsset []byte

// logoW and logoH are the animation's size in cells, checked against the
// asset's own header on load.
const (
	logoW = 96
	logoH = 24
)

var (
	logoOnce   sync.Once
	logoCache  []string
	logoErrStr string
)

// logoFrames decodes the animation on first use. On failure it returns nil
// and the intro falls back to SmallLogo.
func logoFrames() []string {
	logoOnce.Do(func() {
		zr, err := gzip.NewReader(bytes.NewReader(logoAsset))
		if err != nil {
			logoErrStr = err.Error()
			return
		}
		defer zr.Close()

		raw, err := io.ReadAll(zr)
		if err != nil {
			logoErrStr = err.Error()
			return
		}

		lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
		if len(lines) < 2 {
			logoErrStr = "asset is empty"
			return
		}

		var count, w, h, delayMs int
		if _, err := fmt.Sscanf(lines[0], "%d %d %d %d", &count, &w, &h, &delayMs); err != nil {
			logoErrStr = "bad header: " + err.Error()
			return
		}
		if count != len(lines)-1 {
			logoErrStr = fmt.Sprintf("header says %d frames, file has %d", count, len(lines)-1)
			return
		}
		// The constants must agree with the header, or the mark is laid out at
		// one size and drawn at another.
		if w != logoW || h != logoH {
			logoErrStr = fmt.Sprintf("asset is %dx%d, the code says %dx%d", w, h, logoW, logoH)
			return
		}
		logoCache = lines[1:]
	})
	return logoCache
}

// LogoSize is the animation's size in cells, and whether it decoded at all.
func LogoSize() (w, h int, ok bool) {
	return logoW, logoH, len(logoFrames()) > 0
}

// LogoMorph returns frame i on its way to frame i+1, t of the way there: each
// dot the two frames do not share switches at its own moment, so the change
// is spread over the step. Nil if i is out of range.
func LogoMorph(i int, t float64) []rune {
	from := LogoFrame(i)
	to := LogoFrame(i + 1)
	if from == nil || to == nil || t <= 0 {
		return from
	}
	cut := morphCuts()
	for c := range from {
		a, b := from[c]-'⠀', to[c]-'⠀'
		if from[c] < '⠀' || from[c] > '⣿' || to[c] < '⠀' || to[c] > '⣿' || a == b {
			continue
		}
		dots := a & b
		for d := range 8 {
			bit := rune(1) << d
			switch {
			case b&bit != 0 && a&bit == 0 && t >= cut[c*8+d]:
				dots |= bit // arriving
			case a&bit != 0 && b&bit == 0 && t < cut[c*8+d]:
				dots |= bit // not yet gone
			}
		}
		from[c] = '⠀' + dots
	}
	return from
}

var (
	morphOnce    sync.Once
	morphCutsVal []float64
)

// morphCuts is when, from 0 to 1 of a step, each dot of each cell switches:
// fixed, so a morph never flickers, and scattered, so it reads as a dissolve.
func morphCuts() []float64 {
	morphOnce.Do(func() {
		rng := rand.New(rand.NewSource(1))
		morphCutsVal = make([]float64, logoW*logoH*8)
		for i := range morphCutsVal {
			morphCutsVal[i] = rng.Float64()
		}
	})
	return morphCutsVal
}

// LogoFrame returns one frame's runes, or nil if it is out of range.
func LogoFrame(i int) []rune {
	frames := logoFrames()
	if i < 0 || i >= len(frames) {
		return nil
	}
	return []rune(frames[i])
}

// SmallLogo is the compact Lumon mark, for terminals too small for the
// animation. Five rows of twenty braille cells, padded with U+2800 (not
// spaces) so the block stays rectangular whatever follows it.
//
// Regenerating it:
//
//	magick lumon_223.png -colorspace gray -resize 40x20! -threshold 15% small.png
//	uniart small.png -m braille -b -w 20
//
// The resize is exactly 40x20 dots, so one pixel maps to one dot and uniart
// has nothing to dither (at this size its dithering returns scattered dots).
// At a 25% threshold the ellipse breaks up; at 5% the meridians blur into the
// outline. The wordmark is too small to read at five rows (it needs about ten).
var SmallLogo = []string{
	"⠀⠀⣀⡤⠔⢒⡩⣿⠟⠛⠛⠻⡿⢭⡒⠢⢄⣀⠀⠀",
	"⣠⠊⠉⡉⠹⢋⠘⠉⠉⠉⠉⠉⣉⣃⢉⠏⢉⠉⠑⣄",
	"⡇⠀⠸⣧⣤⣼⣤⣼⢿⢧⠾⣿⣯⣽⣿⠓⢾⠇⠀⢸",
	"⠙⢤⡤⠤⠶⣤⢶⡤⠤⠤⠤⠤⢤⡦⢤⠶⠤⢤⡤⠋",
	"⠀⠀⠉⠒⠢⠬⣓⣷⣤⣤⣤⣴⣾⣞⠥⠔⠒⠁⠀⠀",
}
