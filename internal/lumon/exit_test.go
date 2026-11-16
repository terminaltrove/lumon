package lumon

import (
	"strings"
	"testing"
)

func sessionOnFile(t *testing.T) *Model {
	t.Helper()
	rs := NewRooms(7)
	rs.EnsureOpen(2)
	m := NewLocalSession(rs, 1, "mark")
	m.Resize(120, 40)
	m.Update(key("enter"))
	if m.Room == nil || m.Desk == nil {
		t.Fatal("the session did not join a file")
	}
	return m
}

func TestLeavingAFileOffersBothDestinationsAndFreesTheDesk(t *testing.T) {
	m := sessionOnFile(t)
	room := m.Room
	m.Update(key("q"))
	if !m.exitOpen || m.Quitting {
		t.Fatal("q did not open the leave choice")
	}
	for _, want := range []string{"Return to files", "Quit application"} {
		if !strings.Contains(m.View(), want) {
			t.Errorf("leave choice does not show %q", want)
		}
	}

	m.Update(key("enter"))
	if m.Screen != ScreenLobby || m.Room != nil || m.Desk != nil || m.Quitting {
		t.Fatalf("returning to files left screen=%v room=%v desk=%v quitting=%v",
			m.Screen, m.Room != nil, m.Desk != nil, m.Quitting)
	}
	if got := room.Seated(); got != 0 {
		t.Errorf("the old file still holds %d desks", got)
	}
	if m.Name != "MARK" {
		t.Errorf("returning to files lost the refiner's name: %q", m.Name)
	}
	m.Update(key("enter"))
	if m.Room == nil || m.Desk == nil {
		t.Error("the refiner could not join another file")
	}
}

func TestLeavingAFileCanQuitOrCancel(t *testing.T) {
	m := sessionOnFile(t)
	m.Update(key("q"))
	m.Update(key("backspace"))
	if m.exitOpen || m.Quitting || m.Room == nil {
		t.Fatal("cancel did not keep the refiner on the file")
	}

	m.Update(key("q"))
	m.Update(key("down"))
	_, cmd := m.Update(key("enter"))
	if !m.Quitting || !quits(cmd) {
		t.Fatal("choosing quit did not end the application")
	}
}

func TestEmbeddedSessionReturnsToFilesWithoutQuitting(t *testing.T) {
	m := sessionOnFile(t)
	m.KeepOpen()
	room := m.Room
	m.Update(key("?"))
	if view := m.View(); strings.Contains(view, "quit") || strings.Contains(view, "ctrl+c") {
		t.Fatal("browser help still advertises application quit")
	}
	m.Update(key("q")) // back out of help first
	if m.Room == nil || m.Helping {
		t.Fatal("q did not dismiss help while retaining the current file")
	}
	m.Update(key("q"))
	if m.Screen != ScreenLobby || m.Room != nil || m.Quitting || m.exitOpen || room.Seated() != 0 {
		t.Fatal("browser q must return to files and release the desk")
	}
	for _, input := range []string{"q", "q", "ctrl+c"} {
		_, cmd := m.Update(key(input))
		if m.Quitting || quits(cmd) {
			t.Fatalf("%s terminated an embedded session", input)
		}
	}
	if strings.Contains(m.View(), "q quit") {
		t.Fatal("browser lobby still advertises quit")
	}
	m.Update(key("enter"))
	if m.Room == nil || m.Desk == nil {
		t.Fatal("browser session could not rejoin a file")
	}
}
