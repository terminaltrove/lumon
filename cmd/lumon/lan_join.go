package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/muesli/cancelreader"
	"github.com/terminaltrove/lumon/internal/lumon"
	"github.com/terminaltrove/lumon/internal/theme"
)

const (
	// lanDialWait bounds finding and connecting to a host.
	lanDialWait = 5 * time.Second

	// lanAwaitSlack is how much longer than the host's own limit a joiner waits
	// for its answer, so that the host is the one to say so.
	lanAwaitSlack = 15 * time.Second

	// lanProbeEvery is how often the network is asked for games, and lanGameTTL
	// how long a game stays listed after it last answered.
	lanProbeEvery = 1500 * time.Millisecond
	lanGameTTL    = 5 * time.Second

	// lanGameLimit is the most games remembered, lanAddrLimit the most addresses
	// for each, lanIDLimit the longest id, and lanGamesPerSource how many games
	// one address can add. Together they are the most that answers can make us
	// hold, and they stop one machine filling the list.
	lanGameLimit      = 32
	lanAddrLimit      = 8
	lanIDLimit        = 32
	lanGamesPerSource = 4

	// lanSizePoll is how often the terminal's size is checked. Polling works on
	// every platform; SIGWINCH does not exist on Windows.
	lanSizePoll = 250 * time.Millisecond

	// lanTerminalReset puts a terminal back if a host goes without doing it:
	// no focus or mouse reports, no bracketed paste, no synchronised output, no
	// margins, wrapping on, the cursor, plain colour, and the main screen.
	lanTerminalReset = "\x1b[?1004l\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?2004l\x1b[?2026l\x1b[r\x1b[?7h\x1b[?25h\x1b[0m\x1b[?1049l"

	// lanLeaveKey is ctrl+], which ends a game from this side whatever the host
	// is doing. Nothing else is sure to, since every other key goes to the host.
	lanLeaveKey = 0x1d
)

// lanResolve turns what somebody typed into an address to dial. A bare host gets
// the usual port. Anything that is not on the local network is refused, so
// joining cannot reach further than hosting can.
func lanResolve(ctx context.Context, target string) (string, error) {
	target = strings.TrimSpace(target)
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		host, port = strings.Trim(target, "[]"), fmt.Sprint(lanPort)
	}
	if host == "" {
		return "", errors.New("no address")
	}
	if a, err := netip.ParseAddr(host); err == nil {
		if !isLANAddr(a) {
			return "", errNotLocal
		}
		return net.JoinHostPort(a.String(), port), nil
	}
	found, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return "", fmt.Errorf("could not find %s", host)
	}
	for _, a := range found {
		if isLANAddr(a) {
			return net.JoinHostPort(a.Unmap().String(), port), nil
		}
	}
	return "", errNotLocal
}

// errLANStopped is somebody giving up on waiting for the host.
var errLANStopped = errors.New("you stopped waiting")

// lanReadLine reads one line from a host, no longer than a hello can be, so that
// something that is not lumon cannot fill this machine's memory.
func lanReadLine(br *bufio.Reader) (string, error) {
	line, err := br.ReadSlice('\n')
	return string(line), err
}

// lanReply makes sense of a line from a host, or reports why it did not say want.
// A reason comes off the network and is shown on this terminal, so only plain
// text gets through.
func lanReply(reply, want, addr string) error {
	reply = strings.TrimSpace(reply)
	if reply == want {
		return nil
	}
	if why, ok := strings.CutPrefix(reply, "NO "); ok {
		return errors.New(strings.Map(func(r rune) rune {
			if r < ' ' || r > '~' {
				return -1
			}
			return r
		}, why))
	}
	return fmt.Errorf("%s did not answer as a lumon game", addr)
}

// lanDial connects, says hello and sees that the host is willing to ask. On
// success the connection is waiting for the host's answer: see lanAwait.
//
// Nothing is said until TLS is up and the host has shown the key this user
// trusts for that address. A key not trusted yet is a *lanDoubt, and nothing is
// ever sent without encryption.
func lanDial(target string, hello lanHello, known lanKnown) (net.Conn, *bufio.Reader, error) {
	ctx, cancel := context.WithTimeout(context.Background(), lanDialWait)
	defer cancel()
	addr, err := lanResolve(ctx, target)
	if err != nil {
		return nil, nil, err
	}
	var d net.Dialer
	raw, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("could not reach %s", addr)
	}

	// An older host only answers once its own wait for a hello runs out, so this
	// waits a little longer than that to hear why.
	first := &lanFirstBytes{Conn: raw}
	c := tls.Client(first, known.clientConfig(addr))
	hctx, hcancel := context.WithTimeout(context.Background(), lanHelloWait+time.Second)
	err = c.HandshakeContext(hctx)
	hcancel()
	if err != nil {
		defer raw.Close()
		var doubt *lanDoubt
		if errors.As(err, &doubt) {
			return nil, nil, doubt
		}
		if line, ok := first.refusal(); ok {
			why := lanReply(line, "", addr)
			if s := why.Error(); s == "no hello" || s == "that is not lumon" {
				return nil, nil, fmt.Errorf("%s runs an older lumon, which cannot be joined", addr)
			}
			return nil, nil, why
		}
		return nil, nil, fmt.Errorf("%s did not answer as a lumon game", addr)
	}

	line, err := marshalHello(hello)
	if err != nil {
		_ = c.Close()
		return nil, nil, err
	}
	_ = c.SetDeadline(time.Now().Add(lanDialWait))
	if _, err := c.Write(line); err != nil {
		_ = c.Close()
		return nil, nil, fmt.Errorf("could not reach %s", addr)
	}
	br := bufio.NewReaderSize(c, lanHelloLimit)
	reply, err := lanReadLine(br)
	if err != nil {
		_ = c.Close()
		return nil, nil, fmt.Errorf("%s did not answer as a lumon game", addr)
	}
	if err := lanReply(reply, "WAIT", addr); err != nil {
		_ = c.Close()
		return nil, nil, err
	}
	_ = c.SetDeadline(time.Time{})
	return c, br, nil
}

// lanAwait waits for the host's answer. On success the reader holds the start of
// the game's output.
func lanAwait(c net.Conn, br *bufio.Reader) error {
	_ = c.SetReadDeadline(time.Now().Add(lanAnswerWait + lanAwaitSlack))
	reply, err := lanReadLine(br)
	if errors.Is(err, bufio.ErrBufferFull) {
		return fmt.Errorf("%s did not answer as a lumon game", c.RemoteAddr())
	}
	if err != nil {
		return errors.New("the game has ended")
	}
	if err := lanReply(reply, "OK", c.RemoteAddr().String()); err != nil {
		return err
	}
	_ = c.SetReadDeadline(time.Time{})
	return nil
}

// lanWaitForHost shows somebody they are waiting, until the host answers or they
// give up. It takes the keyboard meanwhile and gives it back before returning,
// so the game gets the next key. Anything but Esc or ctrl+c is dropped. An arrow
// key starts with the same byte as Esc but arrives as more than one.
func lanWaitForHost(c net.Conn, br *bufio.Reader, stdin *os.File, stdout io.Writer, size func() (int, int, bool)) error {
	reader, err := cancelreader.NewReader(stdin)
	if err != nil {
		return err
	}

	draw := func() {
		w, h, ok := size()
		if !ok {
			w, h = 80, 24
		}
		lanDrawWaiting(stdout, w, h, c.RemoteAddr().String())
	}
	draw()

	var stopped atomic.Bool
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		buf := make([]byte, 64)
		for {
			n, err := reader.Read(buf)
			if n == 1 && (buf[0] == 0x1b || buf[0] == 0x03) {
				stopped.Store(true)
				_ = c.Close()
				return
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		lastW, lastH, _ := size()
		t := time.NewTicker(lanSizePoll)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if w, h, ok := size(); ok && (w != lastW || h != lastH) {
					lastW, lastH = w, h
					draw()
				}
			}
		}
	}()

	err = lanAwait(c, br)
	close(stop)
	reader.Cancel()
	wg.Wait()
	_ = reader.Close()
	if stopped.Load() {
		return errLANStopped
	}
	if err == nil {
		_, _ = io.WriteString(stdout, "\x1b[2J\x1b[H")
	}
	return err
}

// lanLine is one centred line of a screen drawn while no game is running.
type lanLine struct{ text, style string }

// lanDrawWaiting draws the waiting screen. addr is the address dialled, never
// anything the host said.
func lanDrawWaiting(out io.Writer, w, h int, addr string) {
	lanDrawLines(out, w, h, []lanLine{
		{"Waiting for the host to let you in", "\x1b[1;36m"},
		{addr, "\x1b[36m"},
		{"", ""},
		{"esc to cancel", "\x1b[36m"},
		{"in a game, ctrl+] always leaves", "\x1b[36m"},
	})
}

// lanDrawLines draws lines centred on a cleared screen.
func lanDrawLines(out io.Writer, w, h int, lines []lanLine) {
	var b strings.Builder
	b.WriteString("\x1b[?1049h\x1b[?25l\x1b[2J")
	top := max((h-len(lines))/2, 0) + 1
	for i, l := range lines {
		text := l.text
		if len(text) > w {
			text = text[:max(w, 0)]
		}
		fmt.Fprintf(&b, "\x1b[%d;%dH%s%s\x1b[0m", top+i, max((w-len(text))/2, 0)+1, l.style, text)
	}
	_, _ = io.WriteString(out, b.String())
}

// joinLAN asks to join a game and, if the host accepts, plays it on this terminal
// until it ends. The terminal must be free: no Bubble Tea program is running.
func joinLAN(target, name string) error {
	in, out := os.Stdin.Fd(), os.Stdout.Fd()
	if !term.IsTerminal(in) || !term.IsTerminal(out) {
		return errors.New("joining needs a terminal")
	}
	cols, rows, err := term.GetSize(out)
	if err != nil {
		cols, rows = 80, 24
	}
	size := func() (int, int, bool) {
		w, h, err := term.GetSize(out)
		return w, h, err == nil
	}
	known, err := loadLANKnown()
	if err != nil {
		return err
	}
	hello := lanHello{
		Wire:      lanWire,
		Name:      lumon.LANName(name),
		Cols:      cols,
		Rows:      rows,
		Term:      os.Getenv("TERM"),
		ColorTerm: os.Getenv("COLORTERM"),
	}
	c, br, err := lanDial(target, hello, known)

	// A key not trusted yet is put to the player, and once they trust it the
	// game is dialled again, so the key is checked the same way as next time.
	var doubt *lanDoubt
	if errors.As(err, &doubt) {
		if err := lanConfirmKey(in, size, doubt); err != nil {
			return err
		}
		if err := known.trust(doubt.addr, doubt.key); err != nil {
			return fmt.Errorf("could not remember the host's key: %w", err)
		}
		c, br, err = lanDial(target, hello, known)
	}
	if err != nil {
		return err
	}
	defer c.Close()

	state, err := term.MakeRaw(in)
	if err != nil {
		return fmt.Errorf("could not take the terminal: %w", err)
	}
	defer func() {
		_, _ = io.WriteString(os.Stdout, lanTerminalReset)
		_ = term.Restore(in, state)
	}()

	if err := lanWaitForHost(c, br, os.Stdin, os.Stdout, size); err != nil {
		return err
	}
	return lanPipe(c, br, os.Stdin, os.Stdout, size, hello.Cols, hello.Rows)
}

// lanConfirmKey takes the terminal to ask about a key, and gives it back.
func lanConfirmKey(in uintptr, size func() (int, int, bool), d *lanDoubt) error {
	state, err := term.MakeRaw(in)
	if err != nil {
		return fmt.Errorf("could not take the terminal: %w", err)
	}
	defer func() {
		_, _ = io.WriteString(os.Stdout, lanTerminalReset)
		_ = term.Restore(in, state)
	}()
	return lanAskTrust(os.Stdin, os.Stdout, size, d)
}

// lanAskTrust shows a host's key and waits for the player's answer: y trusts a
// new key, r replaces a changed one, and n, Esc or ctrl+c give up. A changed key
// takes r, a key nobody presses by accident. Both fingerprints were worked out
// here, never said by the host.
func lanAskTrust(in io.Reader, out io.Writer, size func() (int, int, bool), d *lanDoubt) error {
	lines := []lanLine{
		{"First time joining " + d.addr, "\x1b[1;36m"},
		{"", ""},
		{"Check the host's LAN panel shows this key:", "\x1b[36m"},
		{"", ""},
		{lanShowKey(d.key), "\x1b[1m"},
		{"", ""},
		{"y it matches, join · n cancel", "\x1b[36m"},
	}
	yes := byte('y')
	if d.was != "" {
		lines = []lanLine{
			{"The key of " + d.addr + " has changed", "\x1b[1;31m"},
			{"", ""},
			{"Trusted  " + lanShowKey(d.was), "\x1b[36m"},
			{"Now      " + lanShowKey(d.key), "\x1b[1m"},
			{"", ""},
			{"Somebody may be pretending to be the host.", "\x1b[31m"},
			{"Replace it only if the host's LAN panel shows the new key.", "\x1b[36m"},
			{"", ""},
			{"r replace and join · n cancel", "\x1b[36m"},
		}
		yes = 'r'
	}
	draw := func() {
		w, h, ok := size()
		if !ok {
			w, h = 80, 24
		}
		lanDrawLines(out, w, h, lines)
	}
	draw()

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		lastW, lastH, _ := size()
		t := time.NewTicker(lanSizePoll)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if w, h, ok := size(); ok && (w != lastW || h != lastH) {
					lastW, lastH = w, h
					draw()
				}
			}
		}
	}()
	defer func() { close(stop); <-done }()

	buf := make([]byte, 64)
	for {
		n, err := in.Read(buf)
		if n == 1 {
			k := buf[0]
			if k >= 'A' && k <= 'Z' {
				k += 'a' - 'A'
			}
			switch k {
			case yes:
				return nil
			case 'n', 0x1b, 0x03:
				if d.was != "" {
					return errors.New("the host's key has changed, so you did not join")
				}
				return errors.New("you did not trust that host")
			}
		}
		if err != nil {
			return errLANStopped
		}
	}
}

// lanPipe joins a terminal to a game until the game ends. The input is read
// through a cancelable reader: a read left blocked here would eat the next key
// pressed in the local game the player returns to.
//
// cols and rows are the size the host was told in the hello. The terminal may
// have been resized while waiting to be let in, so the size is compared with
// those at once, not with whatever it is when the game starts.
func lanPipe(c net.Conn, game io.Reader, stdin *os.File, stdout io.Writer, size func() (int, int, bool), cols, rows int) error {
	reader, err := cancelreader.NewReader(stdin)
	if err != nil {
		return err
	}

	var mu sync.Mutex
	send := func(kind byte, payload []byte) error {
		mu.Lock()
		defer mu.Unlock()
		return writeLANFrame(c, kind, payload)
	}

	var left atomic.Bool
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		buf := make([]byte, 1024)
		for {
			n, err := reader.Read(buf)
			if bytes.IndexByte(buf[:n], lanLeaveKey) >= 0 {
				left.Store(true)
				_ = c.Close()
				return
			}
			if n > 0 && send(lanFrameInput, buf[:n]) != nil {
				return
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		lastW, lastH := cols, rows
		t := time.NewTicker(lanSizePoll)
		defer t.Stop()
		for {
			w, h, ok := size()
			if ok && (w != lastW || h != lastH) {
				lastW, lastH = w, h
				if send(lanFrameSize, sizeFrame(w, h)) != nil {
					return
				}
			}
			select {
			case <-stop:
				return
			case <-t.C:
			}
		}
	}()

	_, copyErr := io.Copy(newLANScreen(stdout), game)
	close(stop)
	_ = c.Close()
	reader.Cancel()
	wg.Wait()
	_ = reader.Close()

	if copyErr != nil && !left.Load() && !errors.Is(copyErr, net.ErrClosed) && !errors.Is(copyErr, io.EOF) {
		return errors.New("the connection to the game was lost")
	}
	return nil
}

// lanFinder looks for games on the network.
type lanFinder struct {
	conn    *net.UDPConn
	targets []string

	mu    sync.Mutex
	games map[string]lanFound
	stop  chan struct{}
	wg    sync.WaitGroup
}

// lanFound is one game, which answers once for each network the two machines
// share. Answers cannot be authenticated, so one that claims to be this game has
// to name the same preferred address to be counted with it, and only the address
// that game prefers, or the one first heard from, can change what it is called.
type lanFound struct {
	game    lumon.LANGame
	seen    time.Time
	prefer  string
	namedBy string
	addrs   map[string]time.Time
}

// bestAddr is the address to show and dial, of those still answering: the one
// the host prefers, or else any but this machine talking to itself. Every one of
// them answered, so a host's preference can only choose among them.
func (g lanFound) bestAddr(now time.Time) string {
	best := ""
	rank := func(addr string) int {
		host, _, _ := net.SplitHostPort(addr)
		a, err := netip.ParseAddr(host)
		switch {
		case host == g.prefer:
			return 2
		case err == nil && a.IsLoopback():
			return 0
		}
		return 1
	}
	for addr, seen := range g.addrs {
		if now.Sub(seen) > lanGameTTL {
			continue
		}
		if best == "" || rank(addr) > rank(best) || (rank(addr) == rank(best) && addr < best) {
			best = addr
		}
	}
	return best
}

// startLANFinder asks each target, repeatedly, what is being hosted.
func startLANFinder(targets []string) (*lanFinder, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	f := &lanFinder{conn: conn, targets: targets, games: map[string]lanFound{}, stop: make(chan struct{})}
	f.wg.Add(2)
	go f.listen()
	go f.probe()
	return f, nil
}

func (f *lanFinder) close() {
	close(f.stop)
	_ = f.conn.Close()
	f.wg.Wait()
}

func (f *lanFinder) probe() {
	defer f.wg.Done()
	t := time.NewTicker(lanProbeEvery)
	defer t.Stop()
	for {
		for _, target := range f.targets {
			if a, err := net.ResolveUDPAddr("udp4", target); err == nil {
				_, _ = f.conn.WriteToUDP([]byte(lanProbe), a)
			}
		}
		select {
		case <-f.stop:
			return
		case <-t.C:
		}
	}
}

func (f *lanFinder) listen() {
	defer f.wg.Done()
	buf := make([]byte, 512)
	for {
		n, from, err := f.conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			return
		}
		if !isLANAddr(from.Addr()) {
			continue
		}
		var a lanAnnounce
		if json.Unmarshal(buf[:n], &a) != nil || a.Wire != lanWire || a.Port < 1 || a.Port > 65535 {
			continue
		}
		f.heard(a, from.Addr().Unmap(), time.Now())
	}
}

// heard takes an answer. The address is where the answer came from, never what
// it claims.
func (f *lanFinder) heard(a lanAnnounce, from netip.Addr, now time.Time) {
	addr := net.JoinHostPort(from.String(), fmt.Sprint(a.Port))
	prefer := a.Addr
	if _, err := netip.ParseAddr(prefer); err != nil {
		prefer = ""
	}
	key := a.ID + "|" + prefer
	if a.ID == "" || len(a.ID) > lanIDLimit {
		key, prefer = addr, ""
	}
	source := from.String()

	f.mu.Lock()
	defer f.mu.Unlock()
	found, known := f.games[key]
	if !known && (len(f.games) >= lanGameLimit || f.gamesFrom(from) >= lanGamesPerSource) {
		return
	}
	if !known {
		found.addrs = map[string]time.Time{}
		found.prefer, found.namedBy = prefer, source
	}
	_, seen := found.addrs[addr]
	if !seen && len(found.addrs) >= lanAddrLimit && source == prefer {
		// The address the game names has room made for it, or forged ones could
		// keep it out.
		for other := range found.addrs {
			delete(found.addrs, other)
			break
		}
	}
	if seen || len(found.addrs) < lanAddrLimit {
		found.addrs[addr] = now
	}
	if source == prefer {
		found.namedBy = source
	}
	if source == found.namedBy {
		found.game = lumon.LANGame{File: lumon.CleanLANFile(a.File), Host: lumon.LANName(a.Host), Players: a.Players}
	}
	found.seen = now
	f.games[key] = found
}

// gamesFrom counts the games an address has answered for. It takes the lock's
// holder.
func (f *lanFinder) gamesFrom(from netip.Addr) int {
	key := lanBanKey(from)
	n := 0
	for _, found := range f.games {
		for addr := range found.addrs {
			if ap, err := netip.ParseAddrPort(addr); err == nil && lanBanKey(ap.Addr()) == key {
				n++
				break
			}
		}
	}
	return n
}

// list is the games still answering, in a steady order.
func (f *lanFinder) list() []lumon.LANGame {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	games := make([]lumon.LANGame, 0, len(f.games))
	for key, found := range f.games {
		best := found.bestAddr(now)
		if now.Sub(found.seen) > lanGameTTL || best == "" {
			delete(f.games, key)
			continue
		}
		found.game.Addr = best
		games = append(games, found.game)
	}
	sort.Slice(games, func(i, j int) bool { return games[i].Addr < games[j].Addr })
	return games
}

// lanProbeTargets is where to ask: every network this machine is on, and itself.
func lanProbeTargets(port int) []string {
	p := fmt.Sprint(port)
	targets := []string{net.JoinHostPort("127.0.0.1", p), net.JoinHostPort("255.255.255.255", p)}
	ifaces, err := net.Interfaces()
	if err != nil {
		return targets
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := n.IP.To4()
			if ip == nil || len(n.Mask) != net.IPv4len {
				continue
			}
			broadcast := make(net.IP, net.IPv4len)
			for i := range ip {
				broadcast[i] = ip[i] | ^n.Mask[i]
			}
			targets = append(targets, net.JoinHostPort(broadcast.String(), p))
		}
	}
	return targets
}

// lanService is the network side of the panel in the files screen.
type lanService struct {
	rooms   *lumon.Rooms
	theme   *theme.Theme
	name    string
	tcpAddr string
	udpAddr string
	targets func() []string

	mu     sync.Mutex
	host   *lanHost
	finder *lanFinder
}

func newLANService(rooms *lumon.Rooms, th *theme.Theme, name string) *lanService {
	return &lanService{
		rooms:   rooms,
		theme:   th,
		name:    name,
		tcpAddr: fmt.Sprintf(":%d", lanPort),
		udpAddr: fmt.Sprintf(":%d", lanPort),
		targets: func() []string { return lanProbeTargets(lanPort) },
	}
}

// Host implements lumon.LAN. A game already running keeps the name it started
// with.
func (s *lanService) Host(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.host != nil {
		return nil
	}
	s.name = name
	h, err := startLANHost(s.rooms, s.theme, s.name, s.tcpAddr, s.udpAddr)
	if err != nil {
		return errors.New("could not host: is another lumon hosting here?")
	}
	s.host = h
	return nil
}

// StopHosting implements lumon.LAN.
func (s *lanService) StopHosting() {
	s.mu.Lock()
	h := s.host
	s.host = nil
	s.mu.Unlock()
	if h != nil {
		h.stop()
	}
}

// Hosting implements lumon.LAN.
func (s *lanService) Hosting() (lumon.LANHosting, bool) {
	s.mu.Lock()
	h := s.host
	s.mu.Unlock()
	if h == nil {
		return lumon.LANHosting{}, false
	}
	return h.status(), true
}

// Answer implements lumon.LAN.
func (s *lanService) Answer(id uint64, accept bool) {
	s.mu.Lock()
	h := s.host
	s.mu.Unlock()
	if h != nil {
		h.answer(id, accept)
	}
}

// Kick implements lumon.LAN.
func (s *lanService) Kick(id uint64) {
	s.mu.Lock()
	h := s.host
	s.mu.Unlock()
	if h != nil {
		h.kick(id)
	}
}

// Games implements lumon.LAN.
func (s *lanService) Games() []lumon.LANGame {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finder == nil {
		f, err := startLANFinder(s.targets())
		if err != nil {
			return nil
		}
		s.finder = f
	}
	return s.finder.list()
}

// StopLooking implements lumon.LAN.
func (s *lanService) StopLooking() {
	s.mu.Lock()
	f := s.finder
	s.finder = nil
	s.mu.Unlock()
	if f != nil {
		f.close()
	}
}

// close ends everything, when the program is leaving.
func (s *lanService) close() {
	s.StopLooking()
	s.StopHosting()
}
