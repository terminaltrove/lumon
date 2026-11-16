package lumon

import (
	"math/rand"
	"strings"
	"testing"
	"time"
)

// onFile is a refiner seated at the named file.
func onFile(t *testing.T, name string) *Model {
	t.Helper()
	rs := NewRooms(1)
	m := JoinRoom(rs.OpenNamed(name), 1)
	if m == nil {
		t.Fatalf("no desk at %s", name)
	}
	m.Resize(120, 40)
	return m
}

// zeroes reports whether every digit on m's board is 0.
func zeroes(m *Model) bool {
	for _, c := range m.Grid.Cells {
		if c.Value != 0 {
			return false
		}
	}
	return true
}

func TestTheEasterEggFilesAreInTheDrawer(t *testing.T) {
	have := map[string]bool{}
	for _, e := range NewRooms(1).Directory() {
		have[e.Name] = true
	}
	for _, name := range EasterEggFiles {
		if !have[name] {
			t.Errorf("%s is not in the drawer", name)
		}
	}
}

// Null is all zeroes, from its first board through refines and
// detonations, and is finished like any other file.
func TestNullIsAllZeroesAndStillFinishes(t *testing.T) {
	m := onFile(t, nullFile)
	if !zeroes(m) {
		t.Fatal("Null's board has digits other than 0")
	}

	if !refineOne(t, m) {
		t.Skip("nothing woke up")
	}
	tick(m, 120)
	m.Room.mu.Lock()
	if grp := m.Room.FirstActive(); grp != nil {
		m.Room.Detonate(grp, rand.New(rand.NewSource(1)).Intn)
	}
	m.Room.mu.Unlock()
	if !zeroes(m) {
		t.Error("a refine or a detonation put a digit other than 0 on Null")
	}

	completeTheFile(m)
	openNotice(m)
	if got := screenOf(m); !strings.Contains(got, "100% Complete") {
		t.Errorf("a finished Null does not say so:\n%s", got)
	}
}

func TestNullReadsZeroPercentUntilItIsDone(t *testing.T) {
	m := onFile(t, nullFile)
	m.Room.mu.Lock()
	for i := range m.Grid.Bins {
		m.Grid.Bins[i].Refined = m.Grid.Bins[i].Quota / 2
	}
	m.Room.mu.Unlock()
	tick(m, 1)

	if got := screenOf(m); !strings.Contains(got, " 0% Complete") {
		t.Errorf("half of Null is refined and its header says otherwise:\n%s", got)
	}
	if m.File.Percent(m.Grid) == 0 {
		t.Error("the file itself forgot its progress; only the header should read 0")
	}
}

func TestAFinishedEggFileSaysSomethingExtra(t *testing.T) {
	for name, want := range map[string]string{
		"Localhost": "There's no place like 127.0.0.1.",
		"Trove":     "This file was curated by Terminal Trove.",
		"Kernighan": "hello, world",
	} {
		m := onFile(t, name)
		completeTheFile(m)
		openNotice(m)
		if got := screenOf(m); !strings.Contains(got, want) {
			t.Errorf("finished %s does not say %q:\n%s", name, want, got)
		}
	}

	m := onFile(t, "Siena")
	completeTheFile(m)
	openNotice(m)
	for _, line := range fileLines {
		if strings.Contains(screenOf(m), line) {
			t.Errorf("an ordinary file's notice says %q", line)
		}
	}
}

// cornerOf puts mark one step from the box's top-left corner, heading into it.
func cornerOf(p *Perpetuity) {
	rx, ry := p.reach()
	lox, _ := travel(rx, p.w)
	loy, _ := travel(ry, p.h)
	p.vx, p.vy = -34, -34
	p.x, p.y = lox+0.1, loy+0.1
}

func TestTheScreensaverCelebratesACorner(t *testing.T) {
	rs := NewRooms(1)
	m := NewLocalSession(rs, 1, "mark")
	m.Resize(120, 40)
	if !m.Join(rs.Open()) {
		t.Fatal("no desk")
	}
	m.StartScreensaver()
	cornerOf(m.idleMark)

	cues := heard(m, 2)
	if volumeOf(cues, SoundBeep) < 0 {
		t.Error("the mark hit the corner and the set did not beep")
	}
	if line := m.rooms.news.Line(time.Now()); !strings.Contains(line, "THE MARK HIT THE CORNER") {
		t.Errorf("the floor news is %q", line)
	}

	// One corner, one celebration.
	if volumeOf(heard(m, 30), SoundBeep) >= 0 {
		t.Error("one corner was celebrated twice")
	}
}

func TestTheMarkOnTheBoardDoesNotCelebrate(t *testing.T) {
	m := ready()
	p := NewPerpetuity(m, m.gridPanel().Play(), rand.New(rand.NewSource(1)))
	cornerOf(p)
	for range 4 {
		p.Step(1.0 / 60)
	}
	if p.Cornered() {
		t.Error("the PERPETUITY diversion's mark counted a corner")
	}
}
