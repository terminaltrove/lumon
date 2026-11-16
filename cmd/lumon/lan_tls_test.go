package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"
)

// Tests keep their LAN keys in a directory of their own, never this user's.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "lumon-lan-")
	if err != nil {
		panic(err)
	}
	lanConfigDir = func() (string, error) { return filepath.Join(dir, "lan"), nil }
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// freshLANDir gives a test a key directory of its own, so a host started in it
// has a key no other test's host has.
func freshLANDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "lan")
	was := lanConfigDir
	lanConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { lanConfigDir = was })
	return dir
}

// wiretap passes connections through to a host and keeps everything sent
// either way.
type wiretap struct {
	ln net.Listener
	mu sync.Mutex
	up bytes.Buffer // joiner to host
	dn bytes.Buffer // host to joiner
}

func tapHost(t *testing.T, h *lanHost) *wiretap {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	w := &wiretap{ln: ln}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			in, err := ln.Accept()
			if err != nil {
				return
			}
			out, err := net.Dial("tcp", hostAddr(h))
			if err != nil {
				_ = in.Close()
				return
			}
			pass := func(dst, src net.Conn, keep *bytes.Buffer) {
				buf := make([]byte, 4096)
				for {
					n, err := src.Read(buf)
					w.mu.Lock()
					keep.Write(buf[:n])
					w.mu.Unlock()
					if _, werr := dst.Write(buf[:n]); err != nil || werr != nil {
						_ = dst.Close()
						_ = src.Close()
						return
					}
				}
			}
			go pass(out, in, &w.up)
			go pass(in, out, &w.dn)
		}
	}()
	return w
}

func (w *wiretap) addr() string { return w.ln.Addr().String() }

func (w *wiretap) seen() (up, dn []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return bytes.Clone(w.up.Bytes()), bytes.Clone(w.dn.Bytes())
}

// Somebody watching the network sees TLS from the first byte: not the name in
// the hello, not a key pressed, not the game drawn.
func TestNothingCrossesTheNetworkInPlainText(t *testing.T) {
	h := startTestHost(t)
	tap := tapHost(t, h)
	known := lanKnown{tap.addr(): h.ident.key}
	c, br, err := lanDial(tap.addr(), testHello("WIRETAPPED"), known)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	h.answer(waitingOn(t, h, 1)[0].ID, true)
	if err := lanAwait(c, br); err != nil {
		t.Fatal(err)
	}
	if err := writeLANFrame(c, lanFrameInput, []byte("zebra")); err != nil {
		t.Fatal(err)
	}
	screen := make([]byte, 64<<10)
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	got := 0
	for got < 4096 {
		n, err := br.Read(screen[got:])
		got += n
		if err != nil {
			break
		}
	}
	if got < 64 {
		t.Fatal("no game came through")
	}
	time.Sleep(100 * time.Millisecond)

	up, dn := tap.seen()
	if len(up) < 3 || up[0] != 0x16 || up[1] != 0x03 {
		t.Fatalf("the joiner's first bytes are not a TLS handshake: % x", up[:min(len(up), 8)])
	}
	if len(dn) < 3 || dn[0] != 0x16 || dn[1] != 0x03 {
		t.Fatalf("the host's first bytes are not a TLS handshake: % x", dn[:min(len(dn), 8)])
	}
	for _, plain := range []string{"WIRETAPPED", `"wire"`, "zebra", "WAIT", "OK\n"} {
		if bytes.Contains(up, []byte(plain)) || bytes.Contains(dn, []byte(plain)) {
			t.Errorf("%q crossed the network in plain text", plain)
		}
	}
	if bytes.Contains(dn, screen[:64]) {
		t.Error("the game's screen crossed the network in plain text")
	}
}

// The first time, a host's key is put to the player and nothing is said to the
// host. Trusted, it is remembered, and the next join asks nothing.
func TestAFirstJoinAsksAboutTheKeyAndTheNextDoesNot(t *testing.T) {
	freshLANDir(t)
	h := startTestHost(t)
	known, err := loadLANKnown()
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = lanDial(hostAddr(h), testHello("NEW"), known)
	var doubt *lanDoubt
	if !errors.As(err, &doubt) || doubt.was != "" || doubt.key != h.ident.key || doubt.addr != hostAddr(h) {
		t.Fatalf("err = %v (%+v)", err, doubt)
	}
	if n := len(h.status().Waiting); n != 0 {
		t.Fatalf("the host heard from somebody who had not trusted it: %d waiting", n)
	}

	if err := known.trust(doubt.addr, doubt.key); err != nil {
		t.Fatal(err)
	}
	again, err := loadLANKnown()
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := lanDial(hostAddr(h), testHello("NEW"), again)
	if err != nil {
		t.Fatalf("a remembered key was not trusted: %v", err)
	}
	_ = c.Close()
}

// Another key at an address already trusted is refused, and nothing reaches
// that host, not even a name. Only replacing the key lets the join through.
func TestAChangedKeyIsRefusedUntilItIsReplaced(t *testing.T) {
	real := startTestHost(t)
	freshLANDir(t)
	impostor := startTestHost(t)
	if real.ident.key == impostor.ident.key {
		t.Fatal("both hosts have one key")
	}
	known := lanKnown{hostAddr(impostor): real.ident.key}

	_, _, err := lanDial(hostAddr(impostor), testHello("TARGET"), known)
	var doubt *lanDoubt
	if !errors.As(err, &doubt) || doubt.was != real.ident.key || doubt.key != impostor.ident.key {
		t.Fatalf("err = %v (%+v)", err, doubt)
	}
	if !strings.Contains(err.Error(), "changed") {
		t.Fatalf("err = %v", err)
	}
	if n := len(impostor.status().Waiting); n != 0 {
		t.Fatal("the name reached a host whose key had changed")
	}

	if err := known.trust(doubt.addr, doubt.key); err != nil {
		t.Fatal(err)
	}
	c, _, err := lanDial(hostAddr(impostor), testHello("TARGET"), known)
	if err != nil {
		t.Fatalf("a replaced key was refused: %v", err)
	}
	_ = c.Close()
}

// A key is only ever trusted for the address it was trusted at. A machine that
// answers for another game, by discovery or by its address, is one more doubt.
func TestATrustedKeyDoesNotVouchForAnotherAddress(t *testing.T) {
	h := startTestHost(t)
	tap := tapHost(t, h)
	known := lanKnown{hostAddr(h): h.ident.key}
	_, _, err := lanDial(tap.addr(), testHello("JOE"), known)
	var doubt *lanDoubt
	if !errors.As(err, &doubt) || doubt.was != "" {
		t.Fatalf("err = %v", err)
	}
}

// An older lumon speaks before any TLS, and is told to update.
func TestAnOlderJoinerIsToldToUpdate(t *testing.T) {
	h := startTestHost(t)
	c, err := net.Dial("tcp", hostAddr(h))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, _ = io.WriteString(c, `{"wire":2,"name":"OLD","cols":80,"rows":24}`+"\n")
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, _ := bufio.NewReader(c).ReadString('\n')
	if !strings.HasPrefix(line, "NO update lumon") {
		t.Fatalf("reply %q", line)
	}
	if len(h.status().Waiting) != 0 {
		t.Fatal("an unencrypted joiner got onto the list")
	}
}

// An older host answers TLS with its own complaint, in plain text. That is said,
// and nothing is sent unencrypted.
func TestJoiningAnOlderHostSaysSo(t *testing.T) {
	for _, reply := range []string{"NO no hello\n", "NO that is not lumon\n"} {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		got := make(chan []byte, 1)
		go func() {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer c.Close()
			buf := make([]byte, 4096)
			n, _ := c.Read(buf)
			got <- buf[:n]
			_, _ = io.WriteString(c, reply)
		}()
		_, _, err = lanDial(ln.Addr().String(), testHello("NEWER"), lanKnown{})
		_ = ln.Close()
		if err == nil || !strings.Contains(err.Error(), "older lumon") {
			t.Fatalf("%q: err = %v", reply, err)
		}
		if sent := <-got; bytes.Contains(sent, []byte("NEWER")) || len(sent) == 0 || sent[0] != 0x16 {
			t.Fatalf("%q: sent % x", reply, sent[:min(len(sent), 8)])
		}
	}
}

// Refusals made at the door, before any handshake, are still read and said.
func TestADoorRefusalIsSaidBeforeAnyHandshake(t *testing.T) {
	h := startTestHost(t)
	h.mu.Lock()
	h.playing = lanMaxConns
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		h.playing = 0
		h.mu.Unlock()
	}()
	if _, _, err := lanDial(hostAddr(h), testHello("LATE"), trusting(h)); err == nil || err.Error() != "that game is full" {
		t.Fatalf("err = %v", err)
	}
}

// A connection that never finishes its handshake is dropped, and its place at
// the door comes back.
func TestAStalledHandshakeIsDropped(t *testing.T) {
	h := startTestHost(t)
	h.mu.Lock()
	h.helloWait = 200 * time.Millisecond
	h.mu.Unlock()
	c, err := net.Dial("tcp", hostAddr(h))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Read(make([]byte, 1)); err == nil {
		t.Fatal("a silent connection was kept")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		h.mu.Lock()
		pending := h.pending
		h.mu.Unlock()
		if pending == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("pending = %d after the handshake was dropped", pending)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// However many handshakes one machine starts, only lanPendingPerAddr run; the
// rest are turned away at the door, in plain text, before costing any crypto.
func TestAFloodOfHandshakesIsHeldAtTheDoor(t *testing.T) {
	h := startTestHost(t)
	var conns []net.Conn
	defer func() {
		for _, c := range conns {
			_ = c.Close()
		}
	}()
	refused := 0
	for range 20 {
		c, err := net.Dial("tcp", hostAddr(h))
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
		_ = c.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		line, _ := bufio.NewReader(c).ReadString('\n')
		if strings.HasPrefix(line, "NO too many people are waiting") {
			refused++
		}
	}
	if refused != 20-lanPendingPerAddr {
		t.Fatalf("%d refused, want %d", refused, 20-lanPendingPerAddr)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.pending > lanPendingPerAddr {
		t.Fatalf("pending = %d", h.pending)
	}
}

// Hosting again keeps the same key, so the people who trusted it are not asked
// again.
func TestTheHostKeyIsKept(t *testing.T) {
	freshLANDir(t)
	a, err := loadLANIdentity()
	if err != nil {
		t.Fatal(err)
	}
	b, err := loadLANIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if a.key != b.key {
		t.Fatal("the key changed between two hostings")
	}
}

// The key and the trusted list are this user's alone, and a key file others can
// read is refused rather than used.
func TestTheKeysAreKeptPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissions are not modes on Windows")
	}
	dir := freshLANDir(t)
	if _, err := loadLANIdentity(); err != nil {
		t.Fatal(err)
	}
	known, err := loadLANKnown()
	if err != nil {
		t.Fatal(err)
	}
	if err := known.trust("192.168.1.5:2424", strings.Repeat("ab", 32)); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]os.FileMode{".": 0o700, lanKeyFile: 0o600, lanKnownFile: 0o600} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s is %o, want %o", name, got, want)
		}
	}
	if err := os.Chmod(filepath.Join(dir, lanKeyFile), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadLANIdentity(); err == nil || !strings.Contains(err.Error(), "chmod 600") {
		t.Fatalf("a readable key was used: %v", err)
	}
}

// The trusted list survives a round trip, and lines that are not an address and
// a whole fingerprint are ignored.
func TestTheTrustedListReadsBackWhatWasWritten(t *testing.T) {
	dir := freshLANDir(t)
	known := lanKnown{}
	key := strings.Repeat("0f", 32)
	if err := known.trust("[fd00::5]:2424", key); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, lanKnownFile), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(f, "garbage\n10.0.0.1:2424 short\n10.0.0.2:2424 "+strings.Repeat("zz", 32)+"\n")
	_ = f.Close()
	back, err := loadLANKnown()
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != 1 || back["[fd00::5]:2424"] != key {
		t.Fatalf("read back %v", back)
	}
}

// The host's panel shows the start of its key in groups of four, which is what
// a joiner is asked to compare.
func TestTheHostShowsItsKeyAsTheJoinerSeesIt(t *testing.T) {
	h := startTestHost(t)
	got := h.status().Key
	if !regexp.MustCompile(`^[0-9A-F]{4}( [0-9A-F]{4}){4}$`).MatchString(got) {
		t.Fatalf("key shown as %q", got)
	}
	if got != lanShowKey(h.ident.key) {
		t.Fatalf("host shows %q, joiner would see %q", got, lanShowKey(h.ident.key))
	}
}

// y trusts a new key and r replaces a changed one. A changed key ignores y, so
// it cannot be waved through by habit; n and Esc give up.
func TestTheKeyQuestionTakesOnlyItsAnswers(t *testing.T) {
	size := func() (int, int, bool) { return 80, 24, true }
	key, was := strings.Repeat("4f", 32), strings.Repeat("9b", 32)
	for _, tc := range []struct {
		name, keys string
		was        string
		ok         bool
	}{
		{"new, yes", "x;#y", "", true},
		{"new, no", "n", "", false},
		{"new, esc", "\x1b", "", false},
		{"changed, yes is not enough", "yyy", was, false},
		{"changed, replace", "yR", was, true},
		{"changed, esc", "\x1b", was, false},
	} {
		var out bytes.Buffer
		d := &lanDoubt{addr: "192.168.1.5:2424", key: key, was: tc.was}
		err := lanAskTrust(iotest.OneByteReader(strings.NewReader(tc.keys)), &out, size, d)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err = %v", tc.name, err)
		}
		if !strings.Contains(out.String(), lanShowKey(key)) {
			t.Errorf("%s: the key was not shown", tc.name)
		}
		if tc.was != "" && !strings.Contains(out.String(), lanShowKey(was)) {
			t.Errorf("%s: the trusted key was not shown", tc.name)
		}
	}
}

// Two people join one encrypted game, and one leaves and comes back.
func TestTwoJoinersPlayOverTLSAndOneRejoins(t *testing.T) {
	h := startTestHost(t)
	a := join(t, h, "ANA")
	a.waitFor(t, "new file")
	b := join(t, h, "BEN")
	b.waitFor(t, "new file")
	if got := len(h.status().Players); got != 2 {
		t.Fatalf("%d playing", got)
	}
	if _, ok := b.c.(*tls.Conn); !ok {
		t.Fatalf("a joiner's connection is %T", b.c)
	}
	_ = b.c.Close()
	deadline := time.Now().Add(5 * time.Second)
	for len(h.status().Players) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("the one who left is still listed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	join(t, h, "BEN").waitFor(t, "new file")
	if got := len(h.status().Players); got != 2 {
		t.Fatalf("%d playing after rejoining", got)
	}
}

// Encrypting costs next to nothing beside drawing: the stream a joiner gets,
// through TLS and through plain TCP.
func BenchmarkLANStream(b *testing.B) {
	id, err := loadLANIdentity()
	if err != nil {
		b.Fatal(err)
	}
	frame := bytes.Repeat([]byte("\x1b[38;2;4;233;233m7\x1b[0m"), 10<<10/24)
	for _, secure := range []bool{false, true} {
		b.Run(fmt.Sprintf("tls=%v", secure), func(b *testing.B) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				b.Fatal(err)
			}
			defer ln.Close()
			go func() {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				if secure {
					c = tls.Server(c, id.serverConfig())
				}
				_, _ = io.Copy(io.Discard, c)
			}()
			c, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				b.Fatal(err)
			}
			if secure {
				c = tls.Client(c, lanKnown{ln.Addr().String(): id.key}.clientConfig(ln.Addr().String()))
			}
			defer c.Close()
			b.SetBytes(int64(len(frame)))
			b.ResetTimer()
			for b.Loop() {
				if _, err := c.Write(frame); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
