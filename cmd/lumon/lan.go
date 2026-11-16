package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
)

// LAN games. One player hosts and the others join from their own lumon.
//
// The host runs everything, a Model and a Bubble Tea program per joiner, all in
// its one Rooms. A joiner is a thin terminal: its keys and size go up, and the
// drawn screen comes back. Nothing about the board crosses the network, so the
// game itself needs no change to be played this way.
//
// The conversation over TCP, all of it inside TLS 1.3 (see lan_tls.go):
//
//	joiner  one line of JSON, a lanHello
//	host    "WAIT\n" while the host decides, or "NO <reason>\n" and it closes
//	host    "OK\n" once accepted, or "NO <reason>\n" and it closes
//	host    from then on, the raw terminal stream
//	joiner  from then on, frames: a type byte, a two-byte length, the payload
//
// The host may refuse a connection with "NO <reason>\n" before TLS begins, when it
// is full or the address is turned away; nothing else is ever sent in the clear.
//
// Only addresses on the local network are served or dialled, which is what makes
// this a LAN game and not a way to host on the internet. The host accepts or
// denies each person, and anybody who joins is trusted to the same degree as
// somebody at the next desk: see docs/lan.md.

const (
	// lanPort is the TCP port games are played on and the UDP port they are
	// found on.
	lanPort = 2424

	// lanWire is this conversation's version. The host runs the game, so a
	// joiner's game version does not matter, only that they speak the same
	// protocol.
	lanWire = 3

	// lanMaxConns is the most people playing. People not yet let in have their
	// own limits, so they cannot use up these places: lanMaxPending in all and
	// lanPendingPerAddr from any one address.
	lanMaxConns       = 8
	lanMaxPending     = 6
	lanPendingPerAddr = 2

	// lanHelloLimit is the longest hello line, and lanFrameLimit the largest
	// frame payload.
	lanHelloLimit = 1 << 10
	lanFrameLimit = 4 << 10

	// lanFrameInput carries terminal bytes, and lanFrameSize two 16-bit numbers:
	// columns, then rows.
	lanFrameInput = 0
	lanFrameSize  = 1
)

// lanHello is what a joiner says first.
type lanHello struct {
	Wire      int    `json:"wire"`
	Name      string `json:"name"`
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
	Term      string `json:"term"`
	ColorTerm string `json:"colorterm"`
}

// lanCGNAT is the shared address space Tailscale uses. It is not on the public
// internet, and a group playing over a tailnet is a LAN for this purpose.
var lanCGNAT = netip.MustParsePrefix("100.64.0.0/10")

// errNotLocal is what joining an address off the local network says.
var errNotLocal = errors.New("that address is not on your local network")

// isLANAddr reports whether an address is on the local network.
func isLANAddr(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || lanCGNAT.Contains(a)
}

// lanBanKey is what an address is known by when counting against it. Somebody on
// IPv6 has a whole /64 of addresses to hand, so that is one address here.
func lanBanKey(a netip.Addr) netip.Addr {
	a = a.Unmap().WithZone("")
	if a.Is6() {
		if p, err := a.Prefix(64); err == nil {
			return p.Addr()
		}
	}
	return a
}

// lanPeer is the address at the far end of a connection.
func lanPeer(c net.Conn) (netip.Addr, bool) {
	tcp, ok := c.RemoteAddr().(*net.TCPAddr)
	if !ok {
		return netip.Addr{}, false
	}
	return tcp.AddrPort().Addr().Unmap(), true
}

// writeLANFrame writes one frame.
func writeLANFrame(w io.Writer, kind byte, payload []byte) error {
	if len(payload) > lanFrameLimit {
		return fmt.Errorf("frame of %d bytes", len(payload))
	}
	head := [3]byte{kind}
	binary.BigEndian.PutUint16(head[1:], uint16(len(payload)))
	if _, err := w.Write(append(head[:], payload...)); err != nil {
		return err
	}
	return nil
}

// readLANFrame reads one frame into buf, which holds lanFrameLimit bytes.
func readLANFrame(r io.Reader, buf []byte) (kind byte, payload []byte, err error) {
	var head [3]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return 0, nil, err
	}
	n := int(binary.BigEndian.Uint16(head[1:]))
	if n > lanFrameLimit {
		return 0, nil, fmt.Errorf("frame of %d bytes", n)
	}
	if _, err := io.ReadFull(r, buf[:n]); err != nil {
		return 0, nil, err
	}
	return head[0], buf[:n], nil
}

// sizeFrame is a lanFrameSize payload.
func sizeFrame(cols, rows int) []byte {
	var b [4]byte
	binary.BigEndian.PutUint16(b[0:], uint16(cols))
	binary.BigEndian.PutUint16(b[2:], uint16(rows))
	return b[:]
}

// marshalHello is the hello line.
func marshalHello(h lanHello) ([]byte, error) {
	line, err := json.Marshal(h)
	if err != nil {
		return nil, err
	}
	if len(line) >= lanHelloLimit {
		return nil, errors.New("hello too long")
	}
	return append(line, '\n'), nil
}
