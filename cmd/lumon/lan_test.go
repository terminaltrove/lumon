package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/terminaltrove/lumon/internal/lumon"
)

func startTestHost(t *testing.T) *lanHost {
	t.Helper()
	rooms := lumon.NewRooms(1)
	rooms.EnsureOpen(2)
	h, err := startLANHost(rooms, nil, "HOST", "127.0.0.1:0", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.stop)
	return h
}

func hostAddr(h *lanHost) string { return h.ln.Addr().String() }

// trusting is a joiner who has already trusted this host's key.
func trusting(h *lanHost) lanKnown { return lanKnown{hostAddr(h): h.ident.key} }

func testHello(name string) lanHello {
	return lanHello{Wire: lanWire, Name: name, Cols: 100, Rows: 30}
}

// waitingOn waits for this many requests on the host's list.
func waitingOn(t *testing.T, h *lanHost, n int) []lumon.LANRequest {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		got := h.status().Waiting
		if len(got) == n {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d waiting, want %d", len(got), n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// ask dials and asks to join, leaving the request on the host's list.
func ask(t *testing.T, h *lanHost, name string) (net.Conn, *bufio.Reader) {
	t.Helper()
	c, br, err := lanDial(hostAddr(h), testHello(name), trusting(h))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, br
}

// joiner is a test's connection to a game, collecting the screen.
type joiner struct {
	c net.Conn

	mu     sync.Mutex
	screen strings.Builder
	ended  chan struct{}
}

// join asks to join under this name and has the host accept.
func join(t *testing.T, h *lanHost, name string) *joiner {
	t.Helper()
	c, br := ask(t, h, name)
	waiting := waitingOn(t, h, 1)
	h.answer(waiting[0].ID, true)
	if err := lanAwait(c, br); err != nil {
		t.Fatal(err)
	}
	j := &joiner{c: c, ended: make(chan struct{})}
	go func() {
		defer close(j.ended)
		buf := make([]byte, 4096)
		for {
			n, err := br.Read(buf)
			j.mu.Lock()
			j.screen.Write(buf[:n])
			j.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	return j
}

func (j *joiner) typeKeys(t *testing.T, keys string) {
	t.Helper()
	if err := writeLANFrame(j.c, lanFrameInput, []byte(keys)); err != nil {
		t.Fatal(err)
	}
}

func (j *joiner) waitFor(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		j.mu.Lock()
		s := j.screen.String()
		j.mu.Unlock()
		if strings.Contains(s, want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	t.Fatalf("never saw %q in:\n%s", want, j.screen.String())
}

// seated waits for this many refiners across the floor, and returns the files
// that have any.
func seated(t *testing.T, h *lanHost, n int) []lumon.Listing {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var files []lumon.Listing
		total := 0
		for _, e := range h.rooms.List() {
			if len(e.Refiners) > 0 {
				files = append(files, e)
				total += len(e.Refiners)
			}
		}
		if total == n {
			return files
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d seated, want %d", total, n)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestAJoinerGetsTheGameAndNoLANOfTheirOwn(t *testing.T) {
	h := startTestHost(t)
	j := join(t, h, "BOB")
	j.waitFor(t, "new file")

	// They asked under a name, so they are not asked for one again.
	j.mu.Lock()
	asked := strings.Contains(j.screen.String(), "IDENTIFY")
	j.mu.Unlock()
	if asked {
		t.Fatal("a joiner was asked for a name after the host accepted one")
	}

	// The hint on a joiner's files never offers hosting.
	j.mu.Lock()
	s := j.screen.String()
	j.mu.Unlock()
	if strings.Contains(s, "l LAN") {
		t.Fatal("a joiner was offered LAN")
	}
	if got := h.status().Joined; got != 1 {
		t.Fatalf("joined = %d", got)
	}
}

func TestTwoJoinersShareTheHostsFloor(t *testing.T) {
	h := startTestHost(t)
	a, b := join(t, h, "ANN"), join(t, h, "BEA")
	a.waitFor(t, "new file")
	b.waitFor(t, "new file")
	a.typeKeys(t, "\r")
	seated(t, h, 1)
	b.typeKeys(t, "\r")
	files := seated(t, h, 2)
	if len(files) != 1 {
		t.Fatalf("on %d files, want them sharing one", len(files))
	}
}

func TestAResizeReachesTheGame(t *testing.T) {
	h := startTestHost(t)
	j := join(t, h, "BOB")
	j.waitFor(t, "new file")
	if err := writeLANFrame(j.c, lanFrameSize, sizeFrame(40, 12)); err != nil {
		t.Fatal(err)
	}
	// A nonsense size is ignored, and the session survives it.
	if err := writeLANFrame(j.c, lanFrameSize, sizeFrame(65000, 65000)); err != nil {
		t.Fatal(err)
	}
	if err := writeLANFrame(j.c, lanFrameSize, []byte{1}); err != nil {
		t.Fatal(err)
	}
	j.typeKeys(t, "s")
	j.waitFor(t, "enter")
}

func TestLeavingFreesTheNameAndTheCount(t *testing.T) {
	h := startTestHost(t)
	j := join(t, h, "BOB")
	j.waitFor(t, "new file")
	_ = j.c.Close()
	deadline := time.Now().Add(10 * time.Second)
	for h.status().Joined != 0 {
		if time.Now().After(deadline) {
			t.Fatal("still counted after leaving")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if badge, ok := h.rooms.Claim("BOB"); !ok {
		t.Fatal("the name was not released")
	} else {
		h.rooms.Release(badge)
	}
}

func TestStoppingEndsTheGameForEverybody(t *testing.T) {
	rooms := lumon.NewRooms(1)
	rooms.EnsureOpen(1)
	h, err := startLANHost(rooms, nil, "HOST", "127.0.0.1:0", "")
	if err != nil {
		t.Fatal(err)
	}
	j := join(t, h, "BOB")
	j.waitFor(t, "new file")
	h.stop()
	select {
	case <-j.ended:
	case <-time.After(5 * time.Second):
		t.Fatal("the joiner's connection outlived the game")
	}
	if _, err := net.DialTimeout("tcp", hostAddr(h), time.Second); err == nil {
		t.Fatal("still listening after stopping")
	}
}

func TestAnotherWireVersionIsRefused(t *testing.T) {
	h := startTestHost(t)
	hello := testHello("BOB")
	hello.Wire = lanWire + 1
	if _, _, err := lanDial(hostAddr(h), hello, trusting(h)); err == nil || !strings.Contains(err.Error(), "update lumon") {
		t.Fatalf("err = %v", err)
	}
}

func TestTheNinthConnectionIsRefused(t *testing.T) {
	h := startTestHost(t)
	for i := range lanMaxConns {
		join(t, h, fmt.Sprintf("BOB%d", i)).waitFor(t, "new file")
	}
	if _, _, err := lanDial(hostAddr(h), testHello("LAST"), trusting(h)); err == nil || err.Error() != "that game is full" {
		t.Fatalf("err = %v", err)
	}
}

func TestSomethingThatIsNotLumonIsHungUpOn(t *testing.T) {
	h := startTestHost(t)
	for name, send := range map[string]string{
		"not json": "GET / HTTP/1.1\r\n",
		"too long": strings.Repeat("x", lanHelloLimit*2),
	} {
		c, err := net.Dial("tcp", hostAddr(h))
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(c, send)
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
		line, _ := bufio.NewReader(c).ReadString('\n')
		if !strings.HasPrefix(line, "NO ") {
			t.Fatalf("%s: reply %q", name, line)
		}
		_ = c.Close()
	}
	if h.status().Joined != 0 {
		t.Fatal("something got in")
	}
}

func TestAnOversizedFrameEndsTheSession(t *testing.T) {
	h := startTestHost(t)
	j := join(t, h, "BOB")
	j.waitFor(t, "new file")
	if _, err := j.c.Write([]byte{lanFrameInput, 0xff, 0xff}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-j.ended:
	case <-time.After(5 * time.Second):
		t.Fatal("the session survived a frame over the limit")
	}
}

func TestOnlyLocalAddressesCount(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1":          true,
		"::1":                true,
		"10.1.2.3":           true,
		"172.16.0.1":         true,
		"192.168.1.5":        true,
		"169.254.1.1":        true,
		"100.64.0.1":         true,
		"100.127.255.254":    true,
		"fd00::1":            true,
		"fe80::1":            true,
		"::ffff:192.168.1.5": true,
		"8.8.8.8":            false,
		"1.1.1.1":            false,
		"100.128.0.1":        false,
		"172.32.0.1":         false,
		"2606:4700::1111":    false,
		"::ffff:8.8.8.8":     false,
		"224.0.0.1":          false,
		"255.255.255.255":    false,
	} {
		if got := isLANAddr(netip.MustParseAddr(addr)); got != want {
			t.Errorf("%s = %v, want %v", addr, got, want)
		}
	}
}

func TestJoiningRefusesAddressesOffTheNetwork(t *testing.T) {
	ctx := context.Background()
	for _, target := range []string{"8.8.8.8", "8.8.8.8:2424", "[2606:4700::1111]:2424"} {
		if _, err := lanResolve(ctx, target); !errors.Is(err, errNotLocal) {
			t.Errorf("%s: %v", target, err)
		}
		if _, _, err := lanDial(target, testHello("X"), nil); !errors.Is(err, errNotLocal) {
			t.Errorf("dial %s: %v", target, err)
		}
	}
	for target, want := range map[string]string{
		"192.168.1.5":      "192.168.1.5:2424",
		"192.168.1.5:9000": "192.168.1.5:9000",
		"[fe80::1]:9000":   "[fe80::1]:9000",
		"localhost:9000":   "",
	} {
		got, err := lanResolve(ctx, target)
		if err != nil {
			t.Errorf("%s: %v", target, err)
		}
		if want != "" && got != want {
			t.Errorf("%s = %s, want %s", target, got, want)
		}
	}
	if _, err := lanResolve(ctx, ""); err == nil {
		t.Error("an empty address resolved")
	}
}

// publicListener hands out connections that claim to come off the internet.
type publicListener struct {
	net.Listener
	conns chan net.Conn
}

func (l publicListener) Accept() (net.Conn, error) {
	c, ok := <-l.conns
	if !ok {
		return nil, net.ErrClosed
	}
	return c, nil
}
func (l publicListener) Close() error { return nil }

type publicConn struct{ net.Conn }

func (publicConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("8.8.8.8"), Port: 40000}
}

func TestAConnectionFromThePublicInternetIsHungUpOn(t *testing.T) {
	rooms := lumon.NewRooms(1)
	l := publicListener{conns: make(chan net.Conn, 1)}
	h := &lanHost{rooms: rooms, ln: l, conns: map[net.Conn]struct{}{}, denied: map[netip.Addr]int{}, answerWait: time.Minute, ident: &lanIdentity{}}
	h.wg.Add(1)
	go h.accept()
	defer close(l.conns)

	ours, theirs := net.Pipe()
	defer ours.Close()
	l.conns <- publicConn{theirs}

	line, _ := marshalHello(testHello("BOB"))
	_ = ours.SetDeadline(time.Now().Add(5 * time.Second))
	// The host hangs up without reading the hello or saying anything.
	if _, err := ours.Write(line); err == nil {
		t.Fatal("the host read a hello from a public address")
	}
	if n, _ := ours.Read(make([]byte, 16)); n != 0 {
		t.Fatal("the host answered a public address")
	}
	if h.status().Joined != 0 {
		t.Fatal("a public address joined")
	}
}

func TestAGameCanBeFoundAndItsAnswersAreNotTaken(t *testing.T) {
	rooms := lumon.NewRooms(1)
	rooms.EnsureOpen(1)
	h, err := startLANHost(rooms, nil, "HOST", "127.0.0.1:0", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer h.stop()
	if h.udp == nil {
		t.Fatal("not listening for people looking")
	}

	f, err := startLANFinder([]string{h.udp.LocalAddr().String()})
	if err != nil {
		t.Fatal(err)
	}
	defer f.close()

	deadline := time.Now().Add(5 * time.Second)
	var games []lumon.LANGame
	for len(games) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the game was never found")
		}
		time.Sleep(20 * time.Millisecond)
		games = f.list()
	}
	if games[0].Host != "HOST" || games[0].Addr != hostAddr(h) {
		t.Fatalf("found %+v", games[0])
	}

	if games[0].File == "" || games[0].Players != 1 {
		t.Fatalf("found %+v: want a name, and the host counted", games[0])
	}

	// Anything but a probe gets nothing.
	c, err := net.DialUDP("udp4", nil, h.udp.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, _ = c.Write([]byte("hello"))
	_ = c.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if n, _ := c.Read(make([]byte, 64)); n != 0 {
		t.Fatal("answered something that was not a probe")
	}
}

func TestAFinderForgetsGamesAndHoldsOnlySoMany(t *testing.T) {
	f, err := startLANFinder(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer f.close()
	f.mu.Lock()
	f.games["1.2.3.4:1"] = lanFound{seen: time.Now().Add(-2 * lanGameTTL)}
	f.mu.Unlock()
	if got := f.list(); len(got) != 0 {
		t.Fatalf("still listed: %+v", got)
	}

	addr := f.conn.LocalAddr().(*net.UDPAddr)
	c, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: addr.Port})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	// A bad port is not listed, and a good answer's address is where it came
	// from.
	_, _ = c.Write(fmt.Appendf(nil, `{"wire":%d,"host":"X","port":99999}`, lanWire))
	_, _ = c.Write(fmt.Appendf(nil, `{"wire":%d,"host":"X","port":2424,"addr":"8.8.8.8"}`, lanWire+1))
	_, _ = c.Write(fmt.Appendf(nil, `{"wire":%d,"host":"X\u001b[31m","file":"Siena\u0007","port":2424,"addr":"8.8.8.8"}`, lanWire))
	deadline := time.Now().Add(5 * time.Second)
	var games []lumon.LANGame
	for len(games) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the answer was not taken")
		}
		time.Sleep(10 * time.Millisecond)
		games = f.list()
	}
	if len(games) != 1 || games[0].Addr != "127.0.0.1:2424" {
		t.Fatalf("games = %+v", games)
	}
	if games[0].File != "Siena" || games[0].Host != "X31M" {
		t.Fatalf("not cleaned: %+v", games[0])
	}
}

// lanPipeTestWait bounds each step of the pipe test. A shared CI runner under
// the race detector has taken over 5 seconds to pass one keystroke along.
const lanPipeTestWait = 20 * time.Second

func TestThePipeCarriesBothWaysAndLetsGoOfTheKeyboard(t *testing.T) {
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdinR.Close()
	defer stdinW.Close()
	ours, theirs := net.Pipe()
	defer ours.Close()

	var out strings.Builder
	var sizes sync.Mutex
	w, h := 80, 24
	done := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		done <- lanPipe(theirs, theirs, stdinR, &out, func() (int, int, bool) {
			sizes.Lock()
			defer sizes.Unlock()
			return w, h, true
		}, 80, 24)
	}()
	// However the test ends, the pipe lets go of the keyboard before the
	// keyboard is closed under it. Deferred last, so it runs first.
	defer func() {
		_ = ours.Close()
		select {
		case <-finished:
		case <-time.After(lanPipeTestWait):
		}
	}()

	// A key goes up as a frame.
	if _, err := stdinW.WriteString("x"); err != nil {
		t.Fatal(err)
	}
	_ = ours.SetDeadline(time.Now().Add(lanPipeTestWait))
	buf := make([]byte, lanFrameLimit)
	kind, payload, err := readLANFrame(ours, buf)
	if err != nil || kind != lanFrameInput || string(payload) != "x" {
		t.Fatalf("kind=%d payload=%q err=%v", kind, payload, err)
	}

	// A resize goes up as a frame.
	sizes.Lock()
	w, h = 90, 30
	sizes.Unlock()
	_ = ours.SetDeadline(time.Now().Add(lanPipeTestWait))
	kind, payload, err = readLANFrame(ours, buf)
	if err != nil || kind != lanFrameSize || string(payload) != string(sizeFrame(90, 30)) {
		t.Fatalf("kind=%d payload=%v err=%v", kind, payload, err)
	}

	// The game comes down, and the game ending returns promptly with the
	// keyboard read given up, though nobody pressed anything.
	if _, err := ours.Write([]byte("screen")); err != nil {
		t.Fatal(err)
	}
	_ = ours.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(lanPipeTestWait):
		t.Fatal("the pipe did not let go")
	}
	if out.String() != "screen" {
		t.Fatalf("out = %q", out.String())
	}
}

// A terminal resized while its player waited to be let in tells the host at
// once: the host knows only the size in the hello.
func TestAResizeWhileWaitingReachesTheHost(t *testing.T) {
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdinR.Close()
	defer stdinW.Close()
	ours, theirs := net.Pipe()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_ = lanPipe(theirs, theirs, stdinR, io.Discard, func() (int, int, bool) { return 100, 40, true }, 80, 24)
	}()
	defer func() {
		_ = ours.Close()
		select {
		case <-finished:
		case <-time.After(lanPipeTestWait):
		}
	}()
	_ = ours.SetDeadline(time.Now().Add(lanPipeTestWait))
	kind, payload, err := readLANFrame(ours, make([]byte, lanFrameLimit))
	if err != nil || kind != lanFrameSize || string(payload) != string(sizeFrame(100, 40)) {
		t.Fatalf("kind=%d payload=%v err=%v", kind, payload, err)
	}
}

func TestTheServiceHostsOnceAndSaysWhenItCannot(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	busy := ln.Addr().String()
	defer ln.Close()

	rooms := lumon.NewRooms(1)
	s := newLANService(rooms, nil, "HOST")
	s.udpAddr, s.tcpAddr = "", busy
	defer s.close()
	if err := s.Host("HOST"); err == nil {
		t.Fatal("hosted on a port in use")
	}
	if _, on := s.Hosting(); on {
		t.Fatal("reports hosting after failing")
	}

	s.tcpAddr = "127.0.0.1:0"
	if err := s.Host("HOST"); err != nil {
		t.Fatal(err)
	}
	first, on := s.Hosting()
	if !on || first.File == "" {
		t.Fatalf("hosting = %+v, %v", first, on)
	}
	game := s.host
	if err := s.Host("HOST"); err != nil {
		t.Fatal(err)
	}
	if s.host != game {
		t.Fatal("hosting twice started a second game")
	}
	s.StopHosting()
	if _, on := s.Hosting(); on {
		t.Fatal("still hosting")
	}
}

func TestSomebodyWaitsUntilTheHostAccepts(t *testing.T) {
	h := startTestHost(t)
	c, br := ask(t, h, "joe\x1b[31m")
	got := waitingOn(t, h, 1)[0]
	if got.Name != "JOE31M" || got.Addr != "127.0.0.1" {
		t.Fatalf("request = %+v", got)
	}
	if h.status().Joined != 0 {
		t.Fatal("joined before being accepted")
	}

	// Nothing has come back yet: no game, no answer.
	_ = c.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, err := br.Peek(1); err == nil {
		t.Fatal("something was sent before the host answered")
	}
	_ = c.SetReadDeadline(time.Time{})

	h.answer(got.ID, true)
	if err := lanAwait(c, br); err != nil {
		t.Fatal(err)
	}
	if len(h.status().Waiting) != 0 {
		t.Fatal("still on the list")
	}
	deadline := time.Now().Add(10 * time.Second)
	for h.status().Joined != 1 {
		if time.Now().After(deadline) {
			t.Fatal("never joined")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTheJoinerPlaysUnderTheNameTheHostAccepted(t *testing.T) {
	h := startTestHost(t)
	j := join(t, h, "ANN")
	j.waitFor(t, "new file")
	j.typeKeys(t, "\r")
	files := seated(t, h, 1)
	if got := files[0].Refiners[0].Name; got != "ANN" {
		t.Fatalf("seated as %q", got)
	}
}

func TestDenyingSaysSoAndThreeTimesBlocksTheAddress(t *testing.T) {
	h := startTestHost(t)
	for i := range lanDeniesPerAddr {
		c, br := ask(t, h, "PEST")
		h.answer(waitingOn(t, h, 1)[0].ID, false)
		if err := lanAwait(c, br); err == nil || err.Error() != "the host said no" {
			t.Fatalf("denial %d: %v", i, err)
		}
		waitingOn(t, h, 0)
	}
	if _, _, err := lanDial(hostAddr(h), testHello("PEST"), trusting(h)); err == nil || err.Error() != "the host has said no" {
		t.Fatalf("after three denials: %v", err)
	}
	if h.status().Joined != 0 {
		t.Fatal("somebody denied got in")
	}
}

func TestNotAnsweringRunsOutAndDoesNotCountAsADenial(t *testing.T) {
	h := startTestHost(t)
	h.answerWait = 100 * time.Millisecond
	c, br := ask(t, h, "JOE")
	if err := lanAwait(c, br); err == nil || err.Error() != "the host did not answer" {
		t.Fatalf("err = %v", err)
	}
	waitingOn(t, h, 0)
	h.mu.Lock()
	denied := len(h.denied)
	h.mu.Unlock()
	if denied != 0 {
		t.Fatal("running out counted as a denial")
	}
}

func TestHangingUpTakesTheRequestOffTheList(t *testing.T) {
	h := startTestHost(t)
	c, _ := ask(t, h, "JOE")
	got := waitingOn(t, h, 1)[0]
	_ = c.Close()
	waitingOn(t, h, 0)

	// An answer for somebody gone is nothing to worry about.
	h.answer(got.ID, true)
	if h.status().Joined != 0 {
		t.Fatal("somebody gone joined")
	}
}

func TestStoppingEndsEverybodyWaiting(t *testing.T) {
	rooms := lumon.NewRooms(1)
	h, err := startLANHost(rooms, nil, "HOST", "127.0.0.1:0", "")
	if err != nil {
		t.Fatal(err)
	}
	c, br := ask(t, h, "JOE")
	waitingOn(t, h, 1)
	h.stop()
	if err := lanAwait(c, br); err == nil || err.Error() != "the game has ended" {
		t.Fatalf("err = %v", err)
	}
	if len(h.status().Waiting) != 0 {
		t.Fatal("still on the list after stopping")
	}
}

func TestOnlySoManyCanWait(t *testing.T) {
	h := startTestHost(t)
	for i := range lanMaxWaiting {
		if _, err := h.ask(fmt.Sprintf("JOE%d", i), netip.AddrFrom4([4]byte{10, 0, 0, byte(i + 1)})); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.ask("LAST", netip.MustParseAddr("10.0.1.1")); err == nil || !strings.Contains(err.Error(), "waiting") {
		t.Fatalf("err = %v", err)
	}
}

func TestRequestsComeUpOldestFirst(t *testing.T) {
	h := startTestHost(t)
	ask(t, h, "FIRST")
	waitingOn(t, h, 1)
	ask(t, h, "SECOND")
	got := waitingOn(t, h, 2)
	if got[0].Name != "FIRST" || got[1].Name != "SECOND" {
		t.Fatalf("order = %+v", got)
	}
}

func TestAnsweringNeverBlocksTheHostsGame(t *testing.T) {
	h := startTestHost(t)
	ask(t, h, "JOE")
	id := waitingOn(t, h, 1)[0].ID
	done := make(chan struct{})
	go func() {
		h.answer(id, false)
		h.answer(id, false)
		h.answer(id+99, true)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("answering blocked")
	}
}

func TestSendingBeforeTheAnswerEndsTheConnection(t *testing.T) {
	h := startTestHost(t)
	c, br := ask(t, h, "JOE")
	waitingOn(t, h, 1)
	if _, err := c.Write([]byte{lanFrameInput, 0, 1, 'x'}); err != nil {
		t.Fatal(err)
	}
	waitingOn(t, h, 0)
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := br.ReadByte(); err == nil {
		t.Fatal("the connection went on")
	}
}

func TestATakenNameGetsANumber(t *testing.T) {
	h := startTestHost(t)
	a := join(t, h, "ANN")
	a.waitFor(t, "new file")
	b := join(t, h, "ANN")
	b.waitFor(t, "new file")
	a.typeKeys(t, "\r")
	b.typeKeys(t, "\r")
	names := map[string]bool{}
	for _, f := range seated(t, h, 2) {
		for _, r := range f.Refiners {
			names[r.Name] = true
		}
	}
	if !names["ANN"] || !names["ANN-2"] {
		t.Fatalf("names = %v", names)
	}
}

func TestTheGameIsNamedAfterTheHostsFile(t *testing.T) {
	rooms := lumon.NewRooms(1)
	rooms.EnsureOpen(1)
	h, err := startLANHost(rooms, nil, "HOST", "127.0.0.1:0", "")
	if err != nil {
		t.Fatal(err)
	}
	defer h.stop()
	if got := h.status().File; got != "HOST's files" {
		t.Fatalf("nobody on a file: %q", got)
	}
	j := join(t, h, "ANN")
	j.waitFor(t, "new file")
	j.typeKeys(t, "\r")
	want := lumon.CleanLANFile(seated(t, h, 1)[0].Name)
	deadline := time.Now().Add(5 * time.Second)
	for h.status().File != want {
		if time.Now().After(deadline) {
			t.Fatalf("name = %q, want %q", h.status().File, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitTerminal runs lanWaitForHost against a test's keyboard and connection.
func waitTerminal(t *testing.T, c net.Conn, br *bufio.Reader) (*os.File, *strings.Builder, chan error) {
	t.Helper()
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdinR.Close(); _ = stdinW.Close() })
	out := &strings.Builder{}
	done := make(chan error, 1)
	go func() {
		done <- lanWaitForHost(c, br, stdinR, out, func() (int, int, bool) { return 80, 24, true })
	}()
	return stdinW, out, done
}

func TestEscStopsWaitingAndOtherKeysDoNot(t *testing.T) {
	h := startTestHost(t)
	c, br := ask(t, h, "JOE")
	waitingOn(t, h, 1)
	keys, _, done := waitTerminal(t, c, br)

	// An arrow key starts with the Esc byte.
	if _, err := keys.WriteString("\x1b[A"); err != nil {
		t.Fatal(err)
	}
	if _, err := keys.WriteString("x"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		t.Fatalf("stopped on a key that is not Esc: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	if _, err := keys.WriteString("\x1b"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, errLANStopped) {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Esc did not stop the wait")
	}
	waitingOn(t, h, 0)
}

func TestAcceptedWhileWaitingHandsOverToTheGame(t *testing.T) {
	h := startTestHost(t)
	c, br := ask(t, h, "JOE")
	id := waitingOn(t, h, 1)[0].ID
	_, out, done := waitTerminal(t, c, br)
	h.answer(id, true)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the wait did not end")
	}
	if !strings.Contains(out.String(), "Waiting for the host") {
		t.Fatalf("the waiting screen was not drawn: %q", out.String())
	}
	// The game's first bytes are still there for the pipe.
	_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
	if _, err := br.ReadByte(); err != nil {
		t.Fatalf("the game did not come: %v", err)
	}
}

func TestAReasonFromAHostIsPlainText(t *testing.T) {
	err := lanReply("NO evil\x1b[31m\x07 words", "OK", "x")
	if err == nil || strings.ContainsAny(err.Error(), "\x1b\x07") {
		t.Fatalf("err = %q", err)
	}
}

func TestTheWaitingScreenFitsAnySize(t *testing.T) {
	for _, size := range [][2]int{{200, 60}, {80, 24}, {10, 3}, {1, 1}, {0, 0}} {
		var b strings.Builder
		lanDrawWaiting(&b, size[0], size[1], "192.168.1.5:2424")
		if b.Len() == 0 {
			t.Fatal("drew nothing")
		}
	}
}

// A host at a board answers with the room's lock held, which is why the status
// must not read the Rooms. This hangs if it does.
func TestAHostAtABoardCanAnswer(t *testing.T) {
	rooms := lumon.NewRooms(1)
	rooms.EnsureOpen(1)
	s := newLANService(rooms, nil, "HOST")
	s.udpAddr, s.tcpAddr = "", "127.0.0.1:0"
	defer s.close()
	m := lumon.NewLocalSession(rooms, 1, "HOST")
	m.OfferLAN(s)
	if err := s.Host("HOST"); err != nil {
		t.Fatal(err)
	}
	if !m.Join(rooms.Find(rooms.List()[0].Name)) {
		t.Fatal("could not seat the host")
	}
	h := s.host
	c, br := ask(t, h, "JOE")
	waitingOn(t, h, 1)

	press := func() {
		done := make(chan struct{})
		go func() {
			m.UpdateEvent(lumon.KeyMsg{Key: "y", Runes: []rune("y")})
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("the host's game hung answering")
		}
	}
	// The first press only shows it, then it is past the guard.
	press()
	time.Sleep(600 * time.Millisecond)
	press()
	if err := lanAwait(c, br); err != nil {
		t.Fatal(err)
	}
}

func TestAGameOnSeveralNetworksIsListedOnce(t *testing.T) {
	f, err := startLANFinder(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer f.close()
	now := time.Now()
	a := lanAnnounce{Wire: lanWire, ID: "game", Addr: "192.168.1.211", Host: "ALEX", File: "SIENA", Port: 2424, Players: 1}
	for _, from := range []string{"127.0.0.1", "192.168.139.3", "192.168.1.211"} {
		f.heard(a, netip.MustParseAddr(from), now)
	}
	games := f.list()
	if len(games) != 1 || games[0].Addr != "192.168.1.211:2424" {
		t.Fatalf("games = %+v", games)
	}

	// Another machine with the same name and file is another game.
	b := a
	b.ID, b.Addr = "other", "192.168.1.50"
	f.heard(b, netip.MustParseAddr("192.168.1.50"), now)
	if games := f.list(); len(games) != 2 {
		t.Fatalf("games = %+v", games)
	}
}

func TestTheAddressShownAlwaysAnswered(t *testing.T) {
	now := time.Now()
	for name, tc := range map[string]struct {
		prefer string
		heard  []string
		want   string
	}{
		"the host's choice":        {"10.0.0.5", []string{"127.0.0.1:1", "10.0.0.5:1", "10.0.0.4:1"}, "10.0.0.5:1"},
		"a choice it never heard":  {"8.8.8.8", []string{"10.0.0.4:1"}, "10.0.0.4:1"},
		"anything over loopback":   {"", []string{"127.0.0.1:1", "10.0.0.9:1"}, "10.0.0.9:1"},
		"loopback if that is all":  {"10.0.0.5", []string{"127.0.0.1:1"}, "127.0.0.1:1"},
		"the same pick every time": {"", []string{"10.0.0.9:1", "10.0.0.2:1", "10.0.0.7:1"}, "10.0.0.2:1"},
	} {
		g := lanFound{prefer: tc.prefer, addrs: map[string]time.Time{}}
		for _, addr := range tc.heard {
			g.addrs[addr] = now
		}
		if got := g.bestAddr(now); got != tc.want {
			t.Errorf("%s: %q, want %q", name, got, tc.want)
		}
	}

	// An address gone quiet is not used, though the game still answers elsewhere.
	g := lanFound{prefer: "10.0.0.5", addrs: map[string]time.Time{
		"10.0.0.5:1": now.Add(-2 * lanGameTTL),
		"10.0.0.4:1": now,
	}}
	if got := g.bestAddr(now); got != "10.0.0.4:1" {
		t.Fatalf("got %q", got)
	}
}

func TestAnswersCannotGrowAGameOrAnIDWithoutLimit(t *testing.T) {
	f, err := startLANFinder(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer f.close()
	now := time.Now()
	for i := range lanAddrLimit * 3 {
		f.heard(lanAnnounce{ID: "game", Port: 2424}, netip.AddrFrom4([4]byte{10, 0, 0, byte(i + 1)}), now)
	}
	if got := len(f.games["game|"].addrs); got != lanAddrLimit {
		t.Fatalf("%d addresses held", got)
	}
	// An id too long, or none, counts as its address.
	long := strings.Repeat("x", lanIDLimit+1)
	f.heard(lanAnnounce{ID: long, Port: 2424}, netip.MustParseAddr("10.0.1.1"), now)
	f.heard(lanAnnounce{Port: 2424}, netip.MustParseAddr("10.0.1.2"), now)
	if _, held := f.games[long]; held {
		t.Fatal("kept a long id")
	}
	if _, held := f.games["10.0.1.2:2424"]; !held {
		t.Fatal("an answer with no id was not kept")
	}
}

// Over this machine's real networks, however many it has: it answers on each,
// and that is still one game.
func TestARealGameIsListedOnceOnEveryNetworkItAnswersOn(t *testing.T) {
	rooms := lumon.NewRooms(1)
	h, err := startLANHost(rooms, nil, "HOST", "127.0.0.1:0", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer h.stop()
	if h.udp == nil {
		t.Fatal("not listening for people looking")
	}
	f, err := startLANFinder(lanProbeTargets(h.udp.LocalAddr().(*net.UDPAddr).Port))
	if err != nil {
		t.Fatal(err)
	}
	defer f.close()

	// Two rounds of asking, so every network has had the time to answer.
	deadline := time.Now().Add(2*lanProbeEvery + 500*time.Millisecond)
	for time.Now().Before(deadline) {
		if games := f.list(); len(games) > 1 {
			t.Fatalf("listed more than once: %+v", games)
		}
		time.Sleep(50 * time.Millisecond)
	}
	f.mu.Lock()
	heard := len(f.games[h.id].addrs)
	f.mu.Unlock()
	games := f.list()
	if len(games) != 1 {
		t.Fatalf("games = %+v", games)
	}
	t.Logf("heard on %d addresses, listed as %s", heard, games[0].Addr)
}

func TestAHostThatNeverEndsALineCannotFillAJoinersMemory(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = c.Write(make([]byte, 4*lanHelloLimit))
			}()
		}
	}()
	if _, _, err := lanDial(ln.Addr().String(), testHello("JOE"), nil); err == nil || !strings.Contains(err.Error(), "did not answer") {
		t.Fatalf("dial: %v", err)
	}

	ours, theirs := net.Pipe()
	defer ours.Close()
	defer theirs.Close()
	go func() { _, _ = theirs.Write(make([]byte, 4*lanHelloLimit)) }()
	err = lanAwait(ours, bufio.NewReaderSize(ours, lanHelloLimit))
	if err == nil || !strings.Contains(err.Error(), "did not answer") {
		t.Fatalf("await: %v", err)
	}
}

func TestABannedAddressIsRefusedBeforeItSaysAnything(t *testing.T) {
	h := startTestHost(t)
	peer := netip.MustParseAddr("127.0.0.1")
	h.mu.Lock()
	h.denied[peer] = lanDeniesPerAddr
	h.mu.Unlock()
	c, err := net.Dial("tcp", hostAddr(h))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, _ := bufio.NewReader(c).ReadString('\n')
	if line != "NO the host has said no\n" {
		t.Fatalf("reply %q", line)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.pending != 0 {
		t.Fatalf("pending = %d", h.pending)
	}
}

func TestOneAddressCannotHoldTheDoor(t *testing.T) {
	h := startTestHost(t)
	a := netip.MustParseAddr("192.168.1.66")
	for i := range lanPendingPerAddr {
		if why := h.door(a); why != "" {
			t.Fatalf("connection %d: %s", i, why)
		}
	}
	if why := h.door(a); !strings.Contains(why, "waiting") {
		t.Fatalf("over the limit: %q", why)
	}
	// Somebody else still gets in, and a place comes back.
	if why := h.door(netip.MustParseAddr("192.168.1.67")); why != "" {
		t.Fatal(why)
	}
	h.through(a)
	if why := h.door(a); why != "" {
		t.Fatal(why)
	}
}

func TestThoseNotLetInCannotUseUpThePlaces(t *testing.T) {
	h := startTestHost(t)
	for i := range lanMaxPending {
		if why := h.door(netip.AddrFrom4([4]byte{192, 168, 1, byte(i + 1)})); why != "" {
			t.Fatal(why)
		}
	}
	if why := h.door(netip.MustParseAddr("192.168.2.1")); !strings.Contains(why, "waiting") {
		t.Fatalf("over the limit: %q", why)
	}
	// None of them is a player, so the game is not full.
	if h.status().Joined != 0 {
		t.Fatal("somebody at the door counted as playing")
	}
	h.mu.Lock()
	h.pending = 0
	h.playing = lanMaxConns
	h.mu.Unlock()
	if why := h.door(netip.MustParseAddr("192.168.2.1")); why != "that game is full" {
		t.Fatalf("full: %q", why)
	}
	h.mu.Lock()
	h.playing = 0
	h.mu.Unlock()
}

func TestAnIPv6NetworkCountsAsOneAddress(t *testing.T) {
	a := lanBanKey(netip.MustParseAddr("fd00:1:2:3::1"))
	if b := lanBanKey(netip.MustParseAddr("fd00:1:2:3:ffff::2")); a != b {
		t.Fatalf("%v and %v", a, b)
	}
	if b := lanBanKey(netip.MustParseAddr("fd00:1:2:4::1")); a == b {
		t.Fatal("two networks counted as one")
	}
	if got := lanBanKey(netip.MustParseAddr("fe80::1%en0")); got.Zone() != "" {
		t.Fatalf("zone kept: %v", got)
	}
	if got := lanBanKey(netip.MustParseAddr("::ffff:192.168.1.5")); got != netip.MustParseAddr("192.168.1.5") {
		t.Fatalf("mapped: %v", got)
	}
}

func TestTheHostCanRemoveAPlayer(t *testing.T) {
	h := startTestHost(t)
	j := join(t, h, "ANN")
	j.waitFor(t, "new file")
	players := h.status().Players
	if len(players) != 1 || players[0].Name != "ANN" || players[0].Addr != "127.0.0.1" {
		t.Fatalf("players = %+v", players)
	}
	h.kick(players[0].ID)
	select {
	case <-j.ended:
	case <-time.After(5 * time.Second):
		t.Fatal("still connected")
	}
	deadline := time.Now().Add(5 * time.Second)
	for h.status().Joined != 0 || len(h.status().Players) != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("status = %+v", h.status())
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.mu.Lock()
	denied := h.denied[netip.MustParseAddr("127.0.0.1")]
	h.mu.Unlock()
	if denied != 1 {
		t.Fatalf("denied = %d", denied)
	}
	h.kick(players[0].ID) // nobody there any more
}

func TestAPlayerListShowsTheNumberedName(t *testing.T) {
	h := startTestHost(t)
	join(t, h, "ANN").waitFor(t, "new file")
	join(t, h, "ANN").waitFor(t, "new file")
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := h.status().Players
		if len(got) == 2 && got[1].Name == "ANN-2" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("players = %+v", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// failConn fails its writes and notes being closed.
type failConn struct {
	net.Conn
	closed chan struct{}
}

func (failConn) Write([]byte) (int, error)        { return 0, errors.New("stalled") }
func (failConn) SetWriteDeadline(time.Time) error { return nil }
func (f failConn) Close() error                   { close(f.closed); return nil }

func TestAFailedWriteClosesTheConnection(t *testing.T) {
	f := failConn{closed: make(chan struct{})}
	if _, err := (lanWriter{f}).Write([]byte("x")); err == nil {
		t.Fatal("no error")
	}
	select {
	case <-f.closed:
	default:
		t.Fatal("the connection stayed open, and Bubble Tea ignores write errors")
	}
}

func TestResizesAreMeteredAndClamped(t *testing.T) {
	if c, r := lanSize(2000, 1000); c != lumon.MaxCols || r != lumon.MaxRows {
		t.Fatalf("clamped to %dx%d", c, r)
	}
	if c, r := lanSize(0, 5); c != 80 || r != 24 {
		t.Fatalf("nonsense gave %dx%d", c, r)
	}
	var frames bytes.Buffer
	for range 100 {
		_ = writeLANFrame(&frames, lanFrameSize, sizeFrame(2000, 1000))
	}
	var sizes []tea.WindowSizeMsg
	err := lanReadFrames(&frames, io.Discard, func(m tea.Msg) {
		sizes = append(sizes, m.(tea.WindowSizeMsg))
	})
	if !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	if len(sizes) != 1 || sizes[0].Width != lumon.MaxCols || sizes[0].Height != lumon.MaxRows {
		t.Fatalf("sizes = %+v", sizes)
	}
}

func TestAPasteStartNeverReachesTheGame(t *testing.T) {
	run := func(frames ...string) string {
		var f lanPasteFilter
		var out strings.Builder
		for _, frame := range frames {
			out.Write(f.filter([]byte(frame)))
		}
		return out.String()
	}
	for _, tc := range []struct {
		frames []string
		want   string
	}{
		{[]string{"a\x1b[200~b"}, "ab"},
		{[]string{"\x1b[200"}, ""},
		{[]string{"\x1b[20", "0~x"}, "x"},
		{[]string{"\x1b[200~\x1b[200~"}, ""},
		// A lone Esc is a key, so it goes at once, and what would complete the
		// sequence behind it is still taken out.
		{[]string{"\x1b"}, "\x1b"},
		{[]string{"\x1b", "[200~"}, "\x1b"},
		{[]string{"\x1b", "[200~", "[200~", "x"}, "\x1bx"},
		// Everything else is left alone.
		{[]string{"\x1b[A"}, "\x1b[A"},
		{[]string{"\x1b[<0;10;20M"}, "\x1b[<0;10;20M"},
		{[]string{"\x1b[20x"}, "\x1b[20x"},
		{[]string{"\x1b", "x"}, "\x1bx"},
		{[]string{"\x1b\x1b[A"}, "\x1b\x1b[A"},
		{[]string{"hello"}, "hello"},
	} {
		if got := run(tc.frames...); got != tc.want {
			t.Errorf("%q = %q, want %q", tc.frames, got, tc.want)
		}
	}
}

func filterScreen(chunks ...string) string {
	var out strings.Builder
	s := newLANScreen(&out)
	for _, c := range chunks {
		_, _ = s.Write([]byte(c))
	}
	return out.String()
}

func TestWhatAGameDrawsWithGetsThrough(t *testing.T) {
	for _, s := range []string{
		"plain text, ünïcödé ╭─╮ ▄▀ ⢉⠏",
		"\r\n\t\b",
		"\x1b[?1049h\x1b[?25l\x1b[?1003h\x1b[?1006h\x1b[?2004h\x1b[?7l\x1b[?2026h\x1b[?1004l",
		"\x1b[H\x1b[2J\x1b[K\x1b[2K\x1b[J\x1b[A\x1b[3A\x1b[5;10H\x1b[0m\x1b[1;5r\x1b[2L",
		"\x1b[38;2;0;255;255m\x1b[48;5;23mcolour\x1b[0m",
		"\x1b7\x1b8",
		"\x1b7\x1b]1337;File=inline=1;size=4;width=20;height=9;preserveAspectRatio=0:QUJDRA==\a\x1b8",
		"\x1b_Ga=T,f=100,m=1,q=2;QUJDRA==\x1b\\\x1b_Gm=0;QUJD\x1b\\",
	} {
		if got := filterScreen(s); got != s {
			t.Errorf("%q came out as %q", s, got)
		}
	}
}

func TestWhatAGameDoesNotDrawWithIsDropped(t *testing.T) {
	for name, s := range map[string]string{
		"clipboard write":     "\x1b]52;c;QUJD\a",
		"clipboard, ST":       "\x1b]52;c;?\x1b\\",
		"title":               "\x1b]0;owned\a",
		"background query":    "\x1b]11;?\x1b\\",
		"palette":             "\x1b]4;1;rgb:ff/00/00\a",
		"hyperlink":           "\x1b]8;;http://example.com\a",
		"iterm download":      "\x1b]1337;File=name=eA==;size=4;inline=0:QUJD\a",
		"iterm other":         "\x1b]1337;RequestAttention=yes\a",
		"title report":        "\x1b[21t",
		"window ops":          "\x1b[8;1;1t",
		"cursor report":       "\x1b[6n",
		"device attributes":   "\x1b[c\x1b[>c\x1b[=c",
		"keyboard mode":       "\x1b[>1u\x1b[=1u\x1b[<u\x1b[?1u",
		"mode report":         "\x1b[?2026$p",
		"unknown mode":        "\x1b[?9999h\x1b[?1049;9999h",
		"mode with a letter":  "\x1b[?25xh",
		"sgr with a letter":   "\x1b[1xm",
		"dcs":                 "\x1bPq#0;2;0;0;0\x1b\\",
		"dcs query":           "\x1bP$qm\x1b\\",
		"pm":                  "\x1b^x\x1b\\",
		"sos":                 "\x1bXx\x1b\\",
		"kitty from a file":   "\x1b_Ga=T,t=f;L2V0Yy9wYXNzd2Q=\x1b\\",
		"kitty temp file":     "\x1b_Ga=T,t=t;QUJD\x1b\\",
		"kitty shared memory": "\x1b_Ga=T,t=s;QUJD\x1b\\",
		"kitty query":         "\x1b_Ga=q,i=1;QUJD\x1b\\",
		"apc not kitty":       "\x1b_owned\x1b\\",
		"full reset":          "\x1bc",
		"charset":             "\x1b(0",
		"8-bit csi":           "\xc2\x9b6n",
		"bell and enq":        "\a\x05\x7f",
		"too long a sequence": "\x1b[" + strings.Repeat("1;", lanCSILimit) + "m",
	} {
		got := filterScreen(s)
		got = strings.TrimRight(got, "n") // the tail of an 8-bit sequence is text
		for _, bad := range []string{"\x1b", "\xc2\x9b", "\a", "\x05"} {
			if strings.Contains(got, bad) {
				t.Errorf("%s: %q came out as %q", name, s, got)
			}
		}
	}
}

func TestAPictureCannotCarryAnythingElse(t *testing.T) {
	got := filterScreen("\x1b_Ga=T;QU\x1b]52;c;QUJD\a")
	if strings.Contains(got, "]52") {
		t.Fatalf("got %q", got)
	}
	got = filterScreen("\x1b]1337;File=inline=1;size=1;width=1;height=1;preserveAspectRatio=0:QU\nJD\a after")
	if strings.Contains(got, "\n") {
		t.Fatalf("got %q", got)
	}
	if !strings.HasSuffix(got, " after") && strings.Contains(got, "JD") {
		t.Fatalf("got %q", got)
	}
}

func TestTheFilterHoldsItsPlaceAcrossWrites(t *testing.T) {
	want := "ok\x1b[38;5;1mcolour\x1b[0mé\x1b_Ga=T;QUJD\x1b\\end"
	in := "ok\x1b]0;title\a\x1b[38;5;1mcolour\x1b[0m\xc3\xa9\x1b_Ga=T;QUJD\x1b\\e\x1b[6nnd"
	var chunks []string
	for i := range len(in) {
		chunks = append(chunks, in[i:i+1])
	}
	if got := filterScreen(chunks...); got != want {
		t.Fatalf("byte at a time = %q, want %q", got, want)
	}
	if got := filterScreen(in); got != want {
		t.Fatalf("all at once = %q, want %q", got, want)
	}
}

func TestARealGamePassesTheFilterUnchanged(t *testing.T) {
	h := startTestHost(t)
	j := join(t, h, "ANN")
	j.waitFor(t, "new file")
	j.typeKeys(t, "\r")
	seated(t, h, 1)
	time.Sleep(500 * time.Millisecond)
	j.typeKeys(t, "?")
	time.Sleep(200 * time.Millisecond)
	j.typeKeys(t, "?")
	time.Sleep(200 * time.Millisecond)
	j.mu.Lock()
	raw := j.screen.String()
	j.mu.Unlock()
	if got := filterScreen(raw); got != raw {
		i := 0
		for i < len(got) && i < len(raw) && got[i] == raw[i] {
			i++
		}
		t.Fatalf("differs at byte %d of %d: got %q, was %q", i, len(raw), got[i:min(i+60, len(got))], raw[i:min(i+60, len(raw))])
	}
}

func TestCtrlRightBracketAlwaysLeaves(t *testing.T) {
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdinR.Close()
	defer stdinW.Close()
	ours, theirs := net.Pipe()
	defer ours.Close()
	done := make(chan error, 1)
	go func() {
		done <- lanPipe(theirs, theirs, stdinR, io.Discard, func() (int, int, bool) { return 80, 24, true }, 80, 24)
	}()
	// The host ignores everything and never ends the game.
	go func() { _, _ = io.Copy(io.Discard, ours) }()

	if _, err := stdinW.Write([]byte{lanLeaveKey}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ctrl+] did not leave")
	}
}

func TestAForgedAnswerCannotMoveAGame(t *testing.T) {
	f, err := startLANFinder(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer f.close()
	now := time.Now()
	real := lanAnnounce{Wire: lanWire, ID: "game", Addr: "192.168.1.5", Host: "ALEX", File: "SIENA", Port: 2424, Players: 1}
	bad := netip.MustParseAddr("192.168.1.66")
	f.heard(real, netip.MustParseAddr("192.168.1.5"), now)

	// The real id with the forger's own address is a separate line.
	forged := real
	forged.Addr, forged.Host = bad.String(), "EVIL"
	f.heard(forged, bad, now)
	games := f.list()
	if len(games) != 2 {
		t.Fatalf("games = %+v", games)
	}
	for _, g := range games {
		if g.Host == "ALEX" && g.Addr != "192.168.1.5:2424" {
			t.Fatalf("moved: %+v", g)
		}
	}

	// The real id and preferred address cannot move it or rename it.
	echo := real
	echo.Host = "EVIL"
	f.heard(echo, bad, now.Add(time.Second))
	f.heard(real, netip.MustParseAddr("192.168.1.5"), now.Add(time.Second))
	f.heard(echo, bad, now.Add(2*time.Second))
	for _, g := range f.list() {
		if g.Addr == "192.168.1.5:2424" && g.Host != "ALEX" {
			t.Fatalf("renamed: %+v", g)
		}
	}
	if got := f.games["game|192.168.1.5"].bestAddr(now.Add(2 * time.Second)); got != "192.168.1.5:2424" {
		t.Fatalf("best = %s", got)
	}
}

func TestForgedAddressesCannotKeepTheRealOneOut(t *testing.T) {
	f, err := startLANFinder(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer f.close()
	now := time.Now()
	a := lanAnnounce{Wire: lanWire, ID: "game", Addr: "192.168.1.5", Host: "ALEX", Port: 2424}
	for i := range lanAddrLimit {
		f.heard(a, netip.AddrFrom4([4]byte{192, 168, 9, byte(i + 1)}), now)
	}
	f.heard(a, netip.MustParseAddr("192.168.1.5"), now)
	if got := f.games["game|192.168.1.5"].bestAddr(now); got != "192.168.1.5:2424" {
		t.Fatalf("best = %s", got)
	}
	if got := len(f.games["game|192.168.1.5"].addrs); got > lanAddrLimit {
		t.Fatalf("%d addresses held", got)
	}
}

func TestOneMachineCannotFillTheList(t *testing.T) {
	f, err := startLANFinder(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer f.close()
	now := time.Now()
	bad := netip.MustParseAddr("192.168.1.66")
	for i := range lanGameLimit {
		f.heard(lanAnnounce{ID: fmt.Sprintf("fake%d", i), Addr: bad.String(), Port: 2424}, bad, now)
	}
	if got := len(f.games); got != lanGamesPerSource {
		t.Fatalf("%d games from one address", got)
	}
	f.heard(lanAnnounce{ID: "real", Addr: "192.168.1.5", Host: "ALEX", Port: 2424}, netip.MustParseAddr("192.168.1.5"), now)
	if _, ok := f.games["real|192.168.1.5"]; !ok {
		t.Fatal("the real game was kept out")
	}
}
