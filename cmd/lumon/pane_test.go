package main

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/terminaltrove/lumon/internal/lumon"
)

func newTestPane(t *testing.T) (*paneHost, *bytes.Buffer, string) {
	t.Helper()
	dir := t.TempDir()
	rs := lumon.NewRooms(7)
	rs.EnsureOpen(1)
	m := lumon.JoinRoom(rs.Open(), 7)
	var out bytes.Buffer
	return newPaneHost(m, filepath.Join(dir, "input"), &out), &out, filepath.Join(dir, "input")
}

func writeInput(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// frames is every frame line the host wrote: its size and its cells.
func frames(t *testing.T, out *bytes.Buffer) (sizes [][2]int, cells [][]byte) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		f := strings.Fields(line)
		if len(f) != 4 || f[0] != "F" {
			t.Fatalf("not a frame line: %.60q", line)
		}
		w, _ := strconv.Atoi(f[1])
		h, _ := strconv.Atoi(f[2])
		raw, err := base64.StdEncoding.DecodeString(f[3])
		if err != nil {
			t.Fatal(err)
		}
		sizes = append(sizes, [2]int{w, h})
		cells = append(cells, raw)
	}
	return sizes, cells
}

// The pane's size arrives through the input file, and a frame of exactly that
// many cells comes back.
func TestThePaneGetsFramesOfItsOwnSize(t *testing.T) {
	p, out, input := newTestPane(t)
	writeInput(t, input, "1 size 90 30")
	if quit, err := p.step(frameEvery); quit || err != nil {
		t.Fatalf("step: quit %v, %v", quit, err)
	}
	sizes, cells := frames(t, out)
	if len(sizes) != 1 || sizes[0] != [2]int{90, 30} {
		t.Fatalf("frames %v", sizes)
	}
	if len(cells[0]) != 90*30*12 {
		t.Fatalf("%d bytes of cells for 90x30", len(cells[0]))
	}
}

// A frame that has not changed is not sent again; a key that changes the board
// sends one.
func TestThePaneSendsOnlyFramesThatChanged(t *testing.T) {
	p, out, input := newTestPane(t)
	writeInput(t, input, "1 size 90 30")
	p.step(frameEvery)
	sent := strings.Count(out.String(), "\n")
	writeInput(t, input, "1 size 90 30", "2 key ?")
	p.step(frameEvery)
	if strings.Count(out.String(), "\n") == sent {
		t.Fatal("the help opened and no frame was sent")
	}
	// The same input again is old news.
	before := out.Len()
	for range 3 {
		p.step(0)
	}
	if out.Len() != before {
		t.Error("an unchanged frame was sent again")
	}
}

// The plugin asks the game to stop with a quit line; a malformed line is
// ignored.
func TestThePaneStopsWhenAsked(t *testing.T) {
	p, _, input := newTestPane(t)
	writeInput(t, input, "1 size 90 30", "nonsense", "2 key", "3 quit")
	quit, err := p.step(frameEvery)
	if err != nil || !quit {
		t.Fatalf("quit %v, err %v", quit, err)
	}
}
