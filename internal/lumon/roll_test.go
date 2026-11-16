package lumon

import (
	"strings"
	"testing"
)

// Two people under one name would share one nameplate colour, one scoreboard
// row and one tally on a file, and nobody could tell them apart.
func TestANameOnTheFloorCannotBeTakenTwice(t *testing.T) {
	rs := NewRooms(1)
	first := session(t, rs, "mark")
	first.Update(key("enter"))
	if first.Screen != ScreenLobby {
		t.Fatal("setup: the first MARK was not let through")
	}

	second := session(t, rs, "mark")
	second.Update(key("enter"))
	if second.Screen != ScreenName {
		t.Fatal("a second MARK was let onto the floor while the first was still on it")
	}
	if !strings.Contains(screenOf(second), "ALREADY ON THE FLOOR") {
		t.Errorf("the refusal is not on the screen:\n%s", screenOf(second))
	}
}

// Case and spacing are ignored because both forms are the same scoreboard row.
func TestTheSameNameInAnotherSpacingIsTheSameName(t *testing.T) {
	rs := NewRooms(1)
	first := session(t, rs, "Mark S.")
	first.Update(key("enter"))

	second := session(t, rs, "MARK S.")
	second.Update(key("enter"))
	if second.Screen != ScreenName {
		t.Error("MARK S. was taken twice by writing it differently")
	}
}

func TestANameIsFreedWhenItsRefinerLeaves(t *testing.T) {
	rs := NewRooms(1)
	first := session(t, rs, "mark")
	first.Update(key("enter"))
	first.Update(key("enter")) // and take a desk, so leaving has a desk to give up
	first.Close()

	second := session(t, rs, "mark")
	second.Update(key("enter"))
	if second.Screen != ScreenLobby {
		t.Error("MARK is still held by a terminal that has closed")
	}
}

func TestChangingYourNameGivesTheOldOneBack(t *testing.T) {
	rs := NewRooms(1)
	m := session(t, rs, "mark")
	m.Update(key("enter"))
	m.Update(key("q")) // back to the name screen
	if m.Screen != ScreenName {
		t.Fatalf("setup: q at the drawer went to screen %v", m.Screen)
	}
	for range "MARK" {
		m.Update(key("backspace"))
	}
	m.Update(key("helly"))
	m.Update(key("enter"))

	other := session(t, rs, "mark")
	other.Update(key("enter"))
	if other.Screen != ScreenLobby {
		t.Error("MARK is still held by somebody who has since become HELLY")
	}

	// Going back and keeping the same name is not a clash with yourself.
	m.Update(key("q"))
	m.Update(key("enter"))
	if m.Screen != ScreenLobby {
		t.Error("HELLY was refused for already being HELLY")
	}
}

// Anybody may type any name, so the screen shows what is on record under it
// (this quarter and in all) and never claims who is back.
func TestANameOnRecordShowsItsRecord(t *testing.T) {
	rs := NewRooms(1)
	rs.Ledger().Refined("MARK S.", 12)

	m := session(t, rs, "mark s.")
	screen := screenOf(m)
	if !strings.Contains(screen, "ON RECORD · 12 THIS QUARTER · 12 IN ALL") {
		t.Errorf("a name with 12 to it does not show its record:\n%s", screen)
	}
	if strings.Contains(screen, "WELCOME") {
		t.Errorf("the name screen claims to recognise whoever typed the name:\n%s", screen)
	}
}

func TestANameWithNoRecordShowsNone(t *testing.T) {
	rs := NewRooms(1)
	rs.Ledger().Refined("MARK S.", 12)

	m := session(t, rs, "helly")
	if strings.Contains(screenOf(m), "ON RECORD") {
		t.Error("a name nobody has refined under shows a record")
	}
}

// Remotely q goes back to the name; at your own terminal it quits.
func TestTheLobbyHintSaysWhatQDoes(t *testing.T) {
	remote := session(t, NewRooms(1), "mark")
	remote.Update(key("enter"))
	if hint := remote.lobbyHint(200); !strings.Contains(hint, "q back") || strings.Contains(hint, "esc") {
		t.Errorf("a remote drawer's hint is %q, want q back and no esc", hint)
	}

	local := NewLocalSession(NewRooms(1), 1, "mark")
	if hint := local.lobbyHint(200); !strings.Contains(hint, "q quit") || strings.Contains(hint, "esc") {
		t.Errorf("a local drawer's hint is %q, want q quit and no esc", hint)
	}
}
