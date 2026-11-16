package main

import (
	"bufio"
	"cmp"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/terminaltrove/lumon/internal/lumon"
	"github.com/terminaltrove/lumon/internal/theme"
)

const (
	// lanHelloWait is how long a connection has to say hello.
	lanHelloWait = 5 * time.Second

	// lanWriteWait is how long a joiner's terminal may stop reading before it is
	// disconnected, so one stalled joiner cannot keep their place for ever.
	lanWriteWait = 30 * time.Second

	// lanAnswerWait is how long somebody waits for the host to answer.
	lanAnswerWait = 2 * time.Minute

	// lanMaxWaiting is how many can wait at once. They count as connections too.
	lanMaxWaiting = 4

	// lanDeniesPerAddr is how many times an address can be denied before it may
	// not ask again, so nobody can keep putting the question on the host's
	// screen. lanDeniedTable is how many addresses are remembered.
	lanDeniesPerAddr = 3
	lanDeniedTable   = 1024

	// lanFileEvery is how often the game's name is brought up to date.
	lanFileEvery = time.Second

	// lanInputRate bounds a joiner's input in bytes a second. Mouse motion is
	// about 13 bytes an event, so this only stops a flood, whose every event
	// costs the host's game a turn.
	lanInputRate = 16 << 10

	// lanSizeEvery is the least time between resizes. The joiner's own program
	// checks four times a second, so this only stops a flood.
	lanSizeEvery = 100 * time.Millisecond
)

// lanHost is a game being hosted.
type lanHost struct {
	rooms *lumon.Rooms
	theme *theme.Theme
	name  string
	addr  string

	// id tells this game from others, however many networks it answers on.
	id string
	// ip is the address to prefer of those it answers on, the one in addr.
	ip string

	// answerWait is lanAnswerWait and helloWait lanHelloWait, which tests
	// shorten; helloWait under mu.
	answerWait time.Duration
	helloWait  time.Duration

	// ident is the key every connection is encrypted under, and tls the setup
	// that presents it.
	ident *lanIdentity
	tls   *tls.Config

	ln  net.Listener
	udp *net.UDPConn

	mu      sync.Mutex
	conns   map[net.Conn]struct{}
	playing int
	players map[uint64]*lanPlayer
	file    string

	// pending counts connections not yet let in, and pendingFrom the same by
	// lanBanKey.
	pending     int
	pendingFrom map[netip.Addr]int

	waiting []*lanWaiter
	lastID  uint64
	denied  map[netip.Addr]int
	wg      sync.WaitGroup

	done     chan struct{}
	stopOnce sync.Once
}

// lanWaiter is somebody asking to join. The host's answer goes down answer,
// which has room for it so that answering never blocks the host's game.
type lanWaiter struct {
	req    lumon.LANRequest
	peer   netip.Addr
	answer chan bool
}

// lanPlayer is somebody the host let in.
type lanPlayer struct {
	info lumon.LANPlayer
	peer netip.Addr
	conn net.Conn
}

// startLANHost listens on tcpAddr, and on udpAddr for people looking for games
// unless it is empty.
func startLANHost(rooms *lumon.Rooms, th *theme.Theme, name, tcpAddr, udpAddr string) (*lanHost, error) {
	ident, err := loadLANIdentity()
	if err != nil {
		return nil, fmt.Errorf("could not host: %w", err)
	}
	ln, err := net.Listen("tcp", tcpAddr)
	if err != nil {
		return nil, fmt.Errorf("could not host: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ip := lanLocalAddress()
	h := &lanHost{
		rooms: rooms,
		theme: th,
		name:  name,
		addr:  net.JoinHostPort(ip, fmt.Sprint(port)),
		id:    newLANGameID(),
		ip:    ip,
		ln:    ln,
		conns: map[net.Conn]struct{}{},
		file:  lumon.HostFiles(name),

		players:     map[uint64]*lanPlayer{},
		pendingFrom: map[netip.Addr]int{},

		answerWait: lanAnswerWait,
		helloWait:  lanHelloWait,
		ident:      ident,
		tls:        ident.serverConfig(),
		denied:     map[netip.Addr]int{},
		done:       make(chan struct{}),
	}
	if udpAddr != "" {
		// Without this the game can still be joined by typing its address, so a
		// busy port is no reason to refuse to host.
		if a, err := net.ResolveUDPAddr("udp4", udpAddr); err == nil {
			if u, err := net.ListenUDP("udp4", a); err == nil {
				h.udp = u
				h.wg.Add(1)
				go h.answerProbes(port)
			} else {
				log.Printf("lumon: this game cannot be found on the network: %v", err)
			}
		}
	}
	h.wg.Add(2)
	go h.accept()
	go h.followFile()
	return h, nil
}

// newLANGameID makes a game's id. rand.Read does not fail on a supported platform.
func newLANGameID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// followFile keeps the game's name up to date from outside the host's game. Its
// model asks for the status holding a room's lock, which reading the Rooms there
// would deadlock on, and people looking for games should not cost a walk of them.
func (h *lanHost) followFile() {
	defer h.wg.Done()
	t := time.NewTicker(lanFileEvery)
	defer t.Stop()
	for {
		file := h.rooms.GameName(h.name)
		h.mu.Lock()
		h.file = file
		h.mu.Unlock()
		select {
		case <-h.done:
			return
		case <-t.C:
		}
	}
}

// lanLocalAddress is the address to give people: the one this machine would
// reach the network from, if that is a local one. Dialling UDP sends nothing.
func lanLocalAddress() string {
	if c, err := net.Dial("udp4", "192.0.2.1:9"); err == nil {
		defer c.Close()
		if a, ok := c.LocalAddr().(*net.UDPAddr); ok {
			if ip := a.AddrPort().Addr().Unmap(); isLANAddr(ip) && !ip.IsLoopback() {
				return ip.String()
			}
		}
	}
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok || n.IP.To4() == nil {
				continue
			}
			if ip, ok := netip.AddrFromSlice(n.IP.To4()); ok && ip.IsPrivate() {
				return ip.String()
			}
		}
	}
	return "127.0.0.1"
}

// status is what the panel shows.
func (h *lanHost) status() lumon.LANHosting {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := lumon.LANHosting{File: h.file, Addr: h.addr, Key: lanShowKey(h.ident.key), Joined: h.playing}
	for _, w := range h.waiting {
		s.Waiting = append(s.Waiting, w.req)
	}
	for _, p := range h.players {
		s.Players = append(s.Players, p.info)
	}
	slices.SortFunc(s.Players, func(a, b lumon.LANPlayer) int { return cmp.Compare(a.ID, b.ID) })
	return s
}

// kick removes a player. It counts against their address as a denial would.
func (h *lanHost) kick(id uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if p, ok := h.players[id]; ok {
		h.deny(p.peer)
		_ = p.conn.Close()
	}
}

// deny counts against an address. It takes the lock's holder.
func (h *lanHost) deny(peer netip.Addr) {
	if len(h.denied) >= lanDeniedTable {
		clear(h.denied)
	}
	h.denied[lanBanKey(peer)]++
}

// door decides whether a new connection may come in at all, before it has said
// anything, and counts it if so. Its reason is said to the connection.
func (h *lanHost) door(peer netip.Addr) string {
	key := lanBanKey(peer)
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case h.denied[key] >= lanDeniesPerAddr:
		return "the host has said no"
	case h.playing >= lanMaxConns:
		return "that game is full"
	case h.pending >= lanMaxPending, h.pendingFrom[key] >= lanPendingPerAddr:
		return "too many people are waiting to join"
	}
	h.pending++
	h.pendingFrom[key]++
	return ""
}

// through is a connection no longer waiting at the door, let in or not.
func (h *lanHost) through(peer netip.Addr) {
	key := lanBanKey(peer)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pending--
	if h.pendingFrom[key]--; h.pendingFrom[key] <= 0 {
		delete(h.pendingFrom, key)
	}
}

// answer is the host accepting or denying somebody. If they have gone, there is
// nobody to answer.
func (h *lanHost) answer(id uint64, accept bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, w := range h.waiting {
		if w.req.ID != id {
			continue
		}
		h.waiting = append(h.waiting[:i], h.waiting[i+1:]...)
		if !accept {
			h.deny(w.peer)
		}
		w.answer <- accept
		return
	}
}

// forget takes somebody off the list, however they stopped waiting.
func (h *lanHost) forget(w *lanWaiter) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, other := range h.waiting {
		if other == w {
			h.waiting = append(h.waiting[:i], h.waiting[i+1:]...)
			return
		}
	}
}

// stop ends the game for everybody who joined.
func (h *lanHost) stop() {
	h.stopOnce.Do(func() { close(h.done) })
	_ = h.ln.Close()
	if h.udp != nil {
		_ = h.udp.Close()
	}
	h.mu.Lock()
	for c := range h.conns {
		_ = c.Close()
	}
	h.mu.Unlock()

	done := make(chan struct{})
	go func() { h.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		log.Print("lumon: a LAN session was slow to end")
	}
}

func (h *lanHost) accept() {
	defer h.wg.Done()
	for {
		c, err := h.ln.Accept()
		if err != nil {
			return
		}
		peer, ok := lanPeer(c)
		if !ok || !isLANAddr(peer) {
			_ = c.Close()
			continue
		}
		if why := h.door(peer); why != "" {
			lanRefuse(c, why)
			continue
		}
		h.mu.Lock()
		h.conns[c] = struct{}{}
		h.wg.Add(1)
		h.mu.Unlock()
		go h.serve(c, peer)
	}
}

// lanRefuse says why and hangs up. The reply fits in a socket buffer, so the
// short deadline only matters to somebody not reading.
func lanRefuse(c net.Conn, why string) {
	_ = c.SetWriteDeadline(time.Now().Add(time.Second))
	_, _ = io.WriteString(c, "NO "+why+"\n")
	_ = c.Close()
}

func (h *lanHost) serve(c net.Conn, peer netip.Addr) {
	defer h.wg.Done()
	defer func() {
		_ = c.Close()
		h.mu.Lock()
		delete(h.conns, c)
		h.mu.Unlock()
		if r := recover(); r != nil {
			log.Printf("lumon: a LAN session ended: %v", r)
		}
	}()

	atDoor := true
	defer func() {
		if atDoor {
			h.through(peer)
		}
	}()

	// Everything from here on is encrypted, the hello too. The connection is
	// still counted at the door, which is what bounds how many handshakes run at
	// once. Something that does not begin TLS is an older lumon, or not lumon,
	// and is told so in plain text before it is hung up on.
	h.mu.Lock()
	wait := h.helloWait
	h.mu.Unlock()
	tc := tls.Server(c, h.tls)
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	err := tc.HandshakeContext(ctx)
	cancel()
	if err != nil {
		var plain tls.RecordHeaderError
		if errors.As(err, &plain) && plain.Conn != nil {
			lanRefuse(plain.Conn, "update lumon: your version cannot join this game")
		}
		return
	}

	_ = tc.SetDeadline(time.Now().Add(wait))
	br := bufio.NewReaderSize(tc, lanHelloLimit)
	hello, err := readLANHello(br)
	if err != nil {
		lanRefuse(tc, err.Error())
		return
	}
	w, err := h.ask(lumon.LANName(hello.Name), peer)
	if err != nil {
		lanRefuse(tc, err.Error())
		return
	}
	if _, err := io.WriteString(tc, "WAIT\n"); err != nil {
		h.forget(w)
		return
	}
	_ = tc.SetDeadline(time.Time{})

	accepted, why := h.await(tc, br, w)
	if !accepted {
		if why != "" {
			lanRefuse(tc, why)
		}
		return
	}
	h.through(peer)
	atDoor = false

	// The game may have filled while they waited.
	id := w.req.ID
	h.mu.Lock()
	full := h.playing >= lanMaxConns
	if !full {
		h.playing++
		h.players[id] = &lanPlayer{
			info: lumon.LANPlayer{ID: id, Name: w.req.Name, Addr: w.req.Addr},
			peer: peer,
			conn: c,
		}
	}
	h.mu.Unlock()
	if full {
		lanRefuse(tc, "that game is full")
		return
	}
	defer func() {
		h.mu.Lock()
		h.playing--
		delete(h.players, id)
		h.mu.Unlock()
	}()

	_ = tc.SetDeadline(time.Now().Add(lanHelloWait))
	if _, err := io.WriteString(tc, "OK\n"); err != nil {
		return
	}
	_ = tc.SetDeadline(time.Time{})
	h.play(tc, br, hello, id, w.req.Name)
}

// readLANHello reads the hello. Its errors are said to the joiner.
func readLANHello(br *bufio.Reader) (lanHello, error) {
	var hello lanHello
	line, err := br.ReadSlice('\n')
	if err != nil {
		return hello, errors.New("no hello")
	}
	if err := json.Unmarshal(line, &hello); err != nil {
		return hello, errors.New("that is not lumon")
	}
	if hello.Wire != lanWire {
		return hello, errors.New("update lumon: your version cannot join this game")
	}
	return hello, nil
}

// ask puts somebody on the host's list. Its errors are said to the joiner.
func (h *lanHost) ask(name string, peer netip.Addr) (*lanWaiter, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.denied[lanBanKey(peer)] >= lanDeniesPerAddr {
		return nil, errors.New("the host has said no")
	}
	if len(h.waiting) >= lanMaxWaiting {
		return nil, errors.New("too many people are waiting to join")
	}
	h.lastID++
	w := &lanWaiter{
		req:    lumon.LANRequest{ID: h.lastID, Name: name, Addr: peer.String()},
		peer:   peer,
		answer: make(chan bool, 1),
	}
	h.waiting = append(h.waiting, w)
	return w, nil
}

// await holds a connection until the host answers, the wait runs out or the
// joiner goes. A false with no reason means there is nobody left to tell.
//
// Nothing is due from a joiner yet, so a read that returns means they hung up or
// are not following the conversation. The read is over before await returns,
// and it only peeks, so it can leave nothing owed to the game.
func (h *lanHost) await(c net.Conn, br *bufio.Reader, w *lanWaiter) (bool, string) {
	gone := make(chan struct{})
	go func() {
		defer close(gone)
		_, _ = br.Peek(1)
	}()
	timer := time.NewTimer(h.answerWait)
	defer timer.Stop()

	accepted, why, left := false, "", false
	select {
	case accepted = <-w.answer:
		if !accepted {
			why = "the host said no"
		}
	case <-timer.C:
		why = "the host did not answer"
	case <-gone:
		left = true
	}
	h.forget(w)

	_ = c.SetReadDeadline(time.Now())
	<-gone
	_ = c.SetReadDeadline(time.Time{})
	if left || br.Buffered() > 0 {
		return false, ""
	}
	return accepted, why
}

// lanWriter disconnects a terminal that stops reading. It has to close the
// connection itself, because Bubble Tea takes no notice of a failed write.
type lanWriter struct{ c net.Conn }

func (w lanWriter) Write(b []byte) (int, error) {
	_ = w.c.SetWriteDeadline(time.Now().Add(lanWriteWait))
	n, err := w.c.Write(b)
	if err != nil {
		_ = w.c.Close()
	}
	return n, err
}

// play runs the game for one joiner until they leave or the connection goes.
func (h *lanHost) play(c net.Conn, br *bufio.Reader, hello lanHello, id uint64, name string) {
	out := lanWriter{c}
	m := lumon.NewJoinedSession(h.rooms, time.Now().UnixNano(), name)

	// A taken name got a number.
	h.mu.Lock()
	if p, ok := h.players[id]; ok {
		p.info.Name = m.Name
	}
	h.mu.Unlock()

	if h.theme != nil {
		m.Theme = h.theme
	}
	cols, rows := lanSize(hello.Cols, hello.Rows)
	m.Resize(cols, rows)
	// A host at a board brings their joiners to it, rather than leaving them to
	// find it in the drawer.
	m.SitAt(h.rooms.FileOf(h.name))

	r := lipgloss.NewRenderer(out)
	r.SetColorProfile(profileFor(hello.Term, hello.ColorTerm))
	m.UseRenderer(r)
	m.UseGraphics(lumon.DetectGraphics(hello.Term, nil))

	input, feed := io.Pipe()
	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithMouseAllMotion(),
		tea.WithoutBracketedPaste(),
		tea.WithInput(input),
		tea.WithOutput(out),
		// The host's own program has the terminal, and ctrl+c there ends this.
		tea.WithoutSignalHandler(),
		tea.WithFPS(120),
	)

	go func() {
		err := lanReadFrames(br, feed, p.Send)
		_ = feed.CloseWithError(err)
		p.Quit()
	}()

	// A connection going is how sessions end. Anything else is said, though it
	// lands on the host's game.
	if _, err := p.Run(); err != nil && !errors.Is(err, tea.ErrProgramKilled) &&
		!errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
		log.Printf("lumon: LAN session ended: %v", err)
	}
	p.Kill()
	_ = input.Close()
	// The name too, or nobody else can use it.
	m.Close()
}

// profileFor is the colour depth a terminal claims.
func profileFor(term, colorTerm string) termenv.Profile {
	switch {
	case term == "" || term == "dumb":
		return termenv.Ascii
	case strings.Contains(strings.ToLower(colorTerm), "truecolor"),
		strings.Contains(strings.ToLower(colorTerm), "24bit"):
		return termenv.TrueColor
	case strings.Contains(term, "256color"):
		return termenv.ANSI256
	default:
		return termenv.ANSI
	}
}

// lanSize clamps what a joiner claims about its terminal. Nonsense gets the usual
// size, and the game draws no larger than its own limit, so neither should it be
// asked to.
func lanSize(cols, rows int) (int, int) {
	if cols < 1 || rows < 1 || cols > 2000 || rows > 1000 {
		return 80, 24
	}
	return min(cols, lumon.MaxCols), min(rows, lumon.MaxRows)
}

// lanReadFrames feeds a joiner's frames to its program until the connection
// goes.
func lanReadFrames(r io.Reader, input io.Writer, send func(tea.Msg)) error {
	buf := make([]byte, lanFrameLimit)
	tokens, at := float64(lanInputRate), time.Now()
	var paste lanPasteFilter
	var resized time.Time
	for {
		kind, payload, err := readLANFrame(r, buf)
		if err != nil {
			return err
		}
		switch kind {
		case lanFrameInput:
			now := time.Now()
			tokens = min(tokens+now.Sub(at).Seconds()*lanInputRate, lanInputRate)
			at = now
			tokens -= float64(len(payload))
			if tokens < 0 {
				time.Sleep(time.Duration(-tokens / lanInputRate * float64(time.Second)))
			}
			if out := paste.filter(payload); len(out) > 0 {
				if _, err := input.Write(out); err != nil {
					return err
				}
			}
		case lanFrameSize:
			if len(payload) == 4 {
				cols := int(payload[0])<<8 | int(payload[1])
				rows := int(payload[2])<<8 | int(payload[3])
				now := time.Now()
				if cols >= 1 && rows >= 1 && cols <= 2000 && rows <= 1000 && now.Sub(resized) >= lanSizeEvery {
					resized = now
					cols, rows = lanSize(cols, rows)
					send(tea.WindowSizeMsg{Width: cols, Height: rows})
				}
			}
		}
	}
}

// lanAnnounce is a host's answer to somebody looking for games.
type lanAnnounce struct {
	Wire    int    `json:"wire"`
	ID      string `json:"id"`
	Addr    string `json:"addr"`
	File    string `json:"file"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Players int    `json:"players"`
}

// lanProbe is what somebody looking for games sends. It is only ever answered on
// the local network.
const lanProbe = "LUMON?1\n"

func (h *lanHost) answerProbes(port int) {
	defer h.wg.Done()
	buf := make([]byte, 64)
	for {
		n, from, err := h.udp.ReadFromUDPAddrPort(buf)
		if err != nil {
			return
		}
		if string(buf[:n]) != lanProbe || !isLANAddr(from.Addr()) {
			continue
		}
		status := h.status()
		reply, err := json.Marshal(lanAnnounce{Wire: lanWire, ID: h.id, Addr: h.ip, File: status.File, Host: h.name, Port: port, Players: status.Joined + 1})
		if err != nil {
			continue
		}
		_, _ = h.udp.WriteToUDPAddrPort(reply, from)
	}
}
