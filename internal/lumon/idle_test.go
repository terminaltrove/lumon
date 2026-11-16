package lumon

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// wait advances m by d of wall time, a second at a time, and returns the last
// command, which is where a tea.Quit would be.
func wait(m *Model, d time.Duration) tea.Cmd {
	var cmd tea.Cmd
	for d > 0 {
		step := time.Second
		if d < step {
			step = d
		}
		_, cmd = m.Update(FrameMsg(step))
		d -= step
		if m.Quitting {
			return cmd
		}
	}
	return cmd
}

func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// The transport's idle timeout never fires while a client is connected: every
// frame and every link-light keepalive pushes its deadline back. Only the model
// can tell a person from a redraw.
func TestASessionNobodyTouchesIsSentHome(t *testing.T) {
	m := session(t, NewRooms(1), "mark")
	m.SendHomeAfter(time.Minute, 5*time.Minute)
	m.Update(key("enter")) // past the name, into the drawer

	if cmd := wait(m, 4*time.Minute); m.Quitting || quits(cmd) {
		t.Fatal("a session was sent home before its time was up")
	}
	cmd := wait(m, 2*time.Minute)
	if !m.Quitting || !quits(cmd) {
		t.Fatal("a session left alone past its limit is still connected")
	}
	if f := m.Farewell(); !strings.Contains(f, "UNATTENDED") {
		t.Errorf("the farewell does not say why: %q", f)
	}
}

func TestAnyKeyOrMouseResetsTheIdleClock(t *testing.T) {
	m := session(t, NewRooms(1), "mark")
	m.SendHomeAfter(time.Minute, 5*time.Minute)
	m.Update(key("enter"))

	wait(m, 4*time.Minute)
	m.Update(key("down"))
	wait(m, 4*time.Minute)
	m.Update(press(10, 10))
	wait(m, 4*time.Minute)
	if m.Quitting {
		t.Fatal("a session somebody kept touching was sent home")
	}

	wait(m, 2*time.Minute)
	if !m.Quitting {
		t.Error("the clock never ran out after the last touch")
	}
}

// Bots that ask for a terminal end up here. They press nothing, or only enter,
// and a generic username leaves the field empty, so enter does not get them
// through.
func TestTheNameScreenGivesUpSooner(t *testing.T) {
	m := session(t, NewRooms(1), "mark")
	m.SendHomeAfter(time.Minute, 5*time.Minute)

	cmd := wait(m, time.Minute+time.Second)
	if !m.Quitting || !quits(cmd) {
		t.Fatal("a session that never got past the name screen is still connected")
	}
	if f := m.Farewell(); !strings.Contains(f, "NO REFINER WAS IDENTIFIED") {
		t.Errorf("the farewell at the door does not say why: %q", f)
	}
}

// The screensaver means nobody is at the keyboard, yet it holds a desk and
// redraws the screen, so it gets the same limit as any screen past the door.
func TestTheScreensaverIsNotAnExemption(t *testing.T) {
	rs := NewRooms(1)
	m := session(t, rs, "mark")
	m.SendHomeAfter(time.Minute, 5*time.Minute)
	m.Update(key("enter"))
	m.Update(key("enter")) // take a desk
	if m.Desk == nil {
		t.Fatal("setup: no desk was taken")
	}
	m.StartScreensaver()

	wait(m, 5*time.Minute+time.Second)
	if !m.Quitting {
		t.Error("a screensaver nobody touched kept its desk past the limit")
	}
}

func TestALocalSessionIsNeverSentHome(t *testing.T) {
	m := NewLocalSession(NewRooms(1), 1, "mark")
	m.Resize(120, 40)

	wait(m, 2*time.Hour)
	if m.Quitting {
		t.Error("a local session was sent home")
	}
	if f := m.Farewell(); f != "" {
		t.Errorf("a session that was never sent home has a farewell: %q", f)
	}
}

func TestKeysHoldTheDoorOnlySoLong(t *testing.T) {
	m := NewSession(NewRooms(1), 1)
	m.Screen = ScreenName
	m.SendHomeAfter(time.Minute, time.Hour)
	// A key every 59 seconds never reaches the idle limit, so only the door's own
	// limit can end this.
	const gap = 59 * time.Second
	allowed := time.Minute * doorPatience
	for at := gap; at < allowed; at += gap {
		m.touched()
		if m.leftFor(gap) {
			t.Fatalf("sent home after %v of busy keys, before the %v allowed", at, allowed)
		}
	}
	m.touched()
	if !m.leftFor(gap) {
		t.Fatal("a terminal stayed at the door past its limit by pressing keys")
	}
	if m.Farewell() == "" {
		t.Error("sent home with no farewell")
	}
}

func TestTheDoorLimitDoesNotFollowARefinerInside(t *testing.T) {
	m := NewSession(NewRooms(1), 1)
	m.SendHomeAfter(time.Minute, time.Hour)
	m.Screen = ScreenName
	for i := 0; i < doorPatience-1; i++ {
		m.touched()
		if m.leftFor(59 * time.Second) {
			t.Fatal("sent home before the door's limit")
		}
	}
	// Inside, no time at the door counts, and coming back starts it again.
	m.Screen = ScreenLobby
	m.touched()
	if m.leftFor(30 * time.Minute) {
		t.Fatal("a refiner past the door was sent home by the door's limit")
	}
	if m.idle.atDoor != 0 {
		t.Errorf("the door's clock kept %v after leaving it", m.idle.atDoor)
	}
}
