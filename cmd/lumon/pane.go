package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/terminaltrove/lumon/internal/agent"
	"github.com/terminaltrove/lumon/internal/lumon"
)

// The game in a Claude Code pane (plugins/claude-code's inline mod). The mod runs
// `lumon --pane FILE` and draws what it prints; it writes the pane's size and
// the player's keys to FILE, which the game reads each frame.
//
// Out, one line per changed frame: "F <columns> <rows> <cells>", the cells as
// a Raster takes them (canvas.AppendRaster), base64.
//
// In, FILE rewritten whole by the mod, one event per line, numbered so the
// game takes each once: "<n> size <columns> <rows>", "<n> key <name>" (the key
// as Bubble Tea names it) and "<n> quit".

// frameEvery is the pane's frame interval: 30 a second is smooth enough and
// keeps what crosses into Claude Code small.
const frameEvery = time.Second / 30

// paneHost drives one game for a pane.
type paneHost struct {
	m     *lumon.Model
	input string
	out   *bufio.Writer

	seen   int    // the last event number taken
	sized  bool   // whether the pane's size has arrived
	last   []byte // the last frame sent
	cells  []byte
	encode []byte
}

func newPaneHost(m *lumon.Model, input string, out io.Writer) *paneHost {
	return &paneHost{m: m, input: input, out: bufio.NewWriterSize(out, 1<<16)}
}

// runPane plays until the mod asks the game to stop or stops reading. With
// agents, it shows the coding agent's news as the terminal game does.
func runPane(m *lumon.Model, input string, agents agent.Dir) error {
	p := newPaneHost(m, input, os.Stdout)
	tick := time.NewTicker(frameEvery)
	defer tick.Stop()
	w := newAgentWatch()
	var sinceAgents time.Duration
	last := time.Now()
	for now := range tick.C {
		elapsed := now.Sub(last)
		last = now
		if sinceAgents += elapsed; agents != "" && sinceAgents >= agentPoll {
			sinceAgents = 0
			for _, msg := range w.next(now, agents.Read()) {
				m.UpdateEvent(msg)
			}
		}
		quit, err := p.step(elapsed)
		if quit || err != nil {
			return err
		}
	}
	return nil
}

// step takes the new input, advances the game by elapsed and sends the frame
// if it changed. It reports whether the game is over.
func (p *paneHost) step(elapsed time.Duration) (quit bool, err error) {
	if p.takeInput() {
		return true, nil
	}
	for _, e := range p.m.UpdateEvent(lumon.FrameMsg(elapsed)) {
		if e.Kind == lumon.EffectQuit {
			return true, nil
		}
	}
	if !p.sized {
		return false, nil
	}
	buf := p.m.ViewCells()
	p.cells = buf.AppendRaster(p.cells[:0])
	if bytes.Equal(p.cells, p.last) {
		return false, nil
	}
	p.last = append(p.last[:0], p.cells...)
	w, h := buf.Size()
	p.encode = base64.StdEncoding.AppendEncode(p.encode[:0], p.cells)
	fmt.Fprintf(p.out, "F %d %d ", w, h)
	p.out.Write(p.encode)
	p.out.WriteByte('\n')
	return false, p.out.Flush()
}

// inputMax bounds what is read of the input file each frame.
const inputMax = 64 << 10

// takeInput applies the events not yet taken, and reports a quit.
func (p *paneHost) takeInput() (quit bool) {
	f, err := os.Open(p.input)
	if err != nil {
		return false
	}
	defer f.Close()
	data, _ := io.ReadAll(io.LimitReader(f, inputMax))
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		n, err := strconv.Atoi(fields[0])
		if err != nil || n <= p.seen {
			continue
		}
		p.seen = n
		switch {
		case fields[1] == "quit":
			return true
		case fields[1] == "size" && len(fields) == 4:
			w, errW := strconv.Atoi(fields[2])
			h, errH := strconv.Atoi(fields[3])
			if errW == nil && errH == nil && w > 0 && h > 0 && w <= 512 && h <= 256 {
				p.m.UpdateEvent(lumon.WindowSizeMsg{Width: w, Height: h})
				p.sized = true
			}
		case fields[1] == "key" && len(fields) >= 3:
			name := strings.Join(fields[2:], " ")
			if fields[2] == "space" {
				name = " "
			}
			msg := lumon.KeyMsg{Key: name}
			if r := []rune(name); len(r) == 1 {
				msg.Runes = r
			}
			for _, e := range p.m.UpdateEvent(msg) {
				if e.Kind == lumon.EffectQuit {
					return true
				}
			}
		}
	}
	return false
}
