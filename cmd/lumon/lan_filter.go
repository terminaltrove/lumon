package main

import (
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// A LAN host draws on the joiner's terminal, and anybody on the network can pose
// as a host. So what it sends goes through this first: the sequences a game of
// lumon actually draws with get through, and nothing else does. That rules out
// the ones that write the clipboard, set the title, change how the keyboard
// reports, or make the terminal answer, since its answers would go back to the
// host as input.
//
// Only the game's own two kinds of string get through, its pictures, and each
// must have exactly the header the game sends.

const (
	lanCSILimit  = 128
	lanHeadLimit = 256

	// lanImageLimit is the longest picture, far more than the mark needs.
	lanImageLimit = 8 << 20
)

// lanCSIFinals are the plain CSI sequences allowed: moving the cursor, erasing,
// inserting and deleting, scrolling, margins and colour. None makes a terminal
// answer.
const lanCSIFinals = "ABCDEFGHJKLMPSTXdfmr@"

// lanModes are the terminal modes a game may set: autowrap, the cursor, the
// mouse, focus reports, the alternate screen, bracketed paste and synchronised
// output.
var lanModes = []int{7, 25, 1000, 1002, 1003, 1004, 1006, 1049, 2004, 2026}

// lanInlineHead is the header of the game's iTerm2 picture, which leaves no room
// for a file to be downloaded or named.
var lanInlineHead = regexp.MustCompile(`^1337;File=inline=1;size=[0-9]+;width=[0-9]+;height=[0-9]+;preserveAspectRatio=0$`)

// lanKittyHead is the header of a kitty picture: G, then key=value pairs.
var lanKittyHead = regexp.MustCompile(`^G[a-zA-Z]=[-0-9a-zA-Z]+(,[a-zA-Z]=[-0-9a-zA-Z]+)*$`)

type lanScreenState int

const (
	lanGround lanScreenState = iota
	lanEsc
	lanEscSkip
	lanC2
	lanCSI
	lanCSIDrop
	lanHead
	lanHeadEsc
	lanPass
	lanPassEsc
	lanDrop
	lanDropEsc
)

// lanScreen filters a host's output on its way to the terminal. A sequence can
// be split across writes, so it keeps its place between them.
type lanScreen struct {
	w     io.Writer
	state lanScreenState
	csi   []byte
	kind  byte
	head  []byte
	sent  int
	out   []byte
}

func newLANScreen(w io.Writer) *lanScreen { return &lanScreen{w: w} }

// Write implements io.Writer.
func (s *lanScreen) Write(p []byte) (int, error) {
	s.out = s.out[:0]
	for _, b := range p {
		s.step(b)
	}
	if len(s.out) > 0 {
		if _, err := s.w.Write(s.out); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (s *lanScreen) step(b byte) {
	switch s.state {
	case lanGround:
		switch {
		case b == 0x1b:
			s.state = lanEsc
		case b == '\r', b == '\n', b == '\t', b == '\b':
			s.out = append(s.out, b)
		case b < 0x20, b == 0x7f:
		case b == 0xc2:
			s.state = lanC2
		default:
			s.out = append(s.out, b)
		}
	case lanC2:
		// U+0080 to U+009F are the eight-bit forms of ESC [ and the rest.
		s.state = lanGround
		if b < 0x80 || b > 0x9f {
			s.out = append(s.out, 0xc2)
			s.step(b)
		}
	case lanEsc:
		switch {
		case b == '[':
			s.state, s.csi = lanCSI, s.csi[:0]
		case b == ']', b == '_':
			s.state, s.kind, s.head = lanHead, b, s.head[:0]
		case b == 'P', b == 'X', b == '^':
			s.state = lanDrop
		case b == '7', b == '8', b == '=', b == '>', b == 'M':
			s.out = append(s.out, 0x1b, b)
			s.state = lanGround
		case b == 0x1b:
		case b >= 0x20 && b <= 0x2f:
			s.state = lanEscSkip
		default:
			s.state = lanGround
		}
	case lanEscSkip:
		switch {
		case b == 0x1b:
			s.state = lanEsc
		case b < 0x20, b >= 0x30:
			s.state = lanGround
		}
	case lanCSI, lanCSIDrop:
		switch {
		case b >= 0x20 && b <= 0x3f:
			if s.state == lanCSI && len(s.csi) >= lanCSILimit {
				s.state = lanCSIDrop
			} else if s.state == lanCSI {
				s.csi = append(s.csi, b)
			}
		case b >= 0x40 && b <= 0x7e:
			if s.state == lanCSI && lanCSIAllowed(string(s.csi), b) {
				s.out = append(s.out, 0x1b, '[')
				s.out = append(s.out, s.csi...)
				s.out = append(s.out, b)
			}
			s.state = lanGround
		case b == 0x1b:
			s.state = lanEsc
		default:
			s.state = lanGround
		}
	case lanHead:
		switch {
		case b == 0x07:
			s.endHead()
		case b == 0x1b:
			s.state = lanHeadEsc
		case (s.kind == ']' && b == ':') || (s.kind == '_' && b == ';'):
			if lanHeadAllowed(s.kind, string(s.head)) {
				s.out = append(s.out, 0x1b, s.kind)
				s.out = append(s.out, s.head...)
				s.out = append(s.out, b)
				s.state, s.sent = lanPass, 0
			} else {
				s.state = lanDrop
			}
		case len(s.head) >= lanHeadLimit:
			s.state = lanDrop
		default:
			s.head = append(s.head, b)
		}
	case lanHeadEsc:
		if b == '\\' {
			s.endHead()
		} else {
			s.state = lanEsc
			s.step(b)
		}
	case lanPass:
		switch {
		case b == 0x07 && s.kind == ']':
			s.out = append(s.out, b)
			s.state = lanGround
		case b == 0x1b:
			s.state = lanPassEsc
		case !lanIsBase64(b) || s.sent >= lanImageLimit:
			s.closeString()
			s.state = lanDrop
		default:
			s.out = append(s.out, b)
			s.sent++
		}
	case lanPassEsc:
		s.closeString()
		s.state = lanGround
		if b != '\\' {
			s.state = lanEsc
			s.step(b)
		}
	case lanDrop:
		switch b {
		case 0x07, 0x18, 0x1a:
			s.state = lanGround
		case 0x1b:
			s.state = lanDropEsc
		}
	case lanDropEsc:
		s.state = lanGround
		if b != '\\' {
			s.state = lanEsc
			s.step(b)
		}
	}
}

// endHead is a string ending with no payload. Only a kitty command does.
func (s *lanScreen) endHead() {
	s.state = lanGround
	if s.kind == '_' && lanHeadAllowed(s.kind, string(s.head)) {
		s.out = append(s.out, 0x1b, s.kind)
		s.out = append(s.out, s.head...)
		s.closeString()
	}
}

// closeString ends a string that was let through.
func (s *lanScreen) closeString() {
	if s.kind == ']' {
		s.out = append(s.out, 0x07)
		return
	}
	s.out = append(s.out, 0x1b, '\\')
}

func lanIsBase64(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '+' || b == '/' || b == '='
}

func lanCSIAllowed(params string, final byte) bool {
	if rest, private := strings.CutPrefix(params, "?"); private {
		if final != 'h' && final != 'l' {
			return false
		}
		for _, p := range strings.Split(rest, ";") {
			n, err := strconv.Atoi(p)
			if err != nil || !slices.Contains(lanModes, n) {
				return false
			}
		}
		return true
	}
	for i := range len(params) {
		if c := params[i]; (c < '0' || c > '9') && c != ';' && c != ':' {
			return false
		}
	}
	return strings.IndexByte(lanCSIFinals, final) >= 0
}

// lanHeadAllowed decides on a picture by its header. A kitty picture may only
// come in the sequence itself: t=f, t=t and t=s would have the terminal read
// files, and a=q makes it answer.
func lanHeadAllowed(kind byte, head string) bool {
	if kind == ']' {
		return lanInlineHead.MatchString(head)
	}
	if !lanKittyHead.MatchString(head) {
		return false
	}
	for _, kv := range strings.Split(head[1:], ",") {
		if (strings.HasPrefix(kv, "t=") && kv != "t=d") || kv == "a=q" {
			return false
		}
	}
	return true
}

// lanPasteFilter takes the start of a bracketed paste out of a joiner's input.
// Nothing in the game takes a paste, and Bubble Tea holds one in memory for as
// long as it stays open. The sequence can be split across frames, so it holds a
// partial match, but not a lone Esc, which is a key and must not wait for the
// next one. Having passed an Esc on, it still drops what would complete the
// sequence behind it.
type lanPasteFilter struct {
	pos     int
	escSent bool
	out     []byte
}

const lanPasteStart = "\x1b[200~"

func (f *lanPasteFilter) filter(p []byte) []byte {
	f.out = f.out[:0]
	for _, b := range p {
		f.step(b)
	}
	if f.pos == 1 && !f.escSent {
		f.out = append(f.out, 0x1b)
		f.escSent = true
	}
	return f.out
}

func (f *lanPasteFilter) step(b byte) {
	if b == lanPasteStart[f.pos] {
		f.pos++
		if f.pos == len(lanPasteStart) {
			f.pos = 0
			if f.escSent {
				f.pos = 1
			}
		}
		return
	}
	start := 0
	if f.escSent {
		start = 1
	}
	f.out = append(f.out, lanPasteStart[start:f.pos]...)
	f.pos, f.escSent = 0, false
	if b == lanPasteStart[0] {
		f.pos = 1
		return
	}
	f.out = append(f.out, b)
}
