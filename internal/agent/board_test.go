package agent

import (
	"os"
	"testing"
	"time"
)

// The board lists the agents oldest prompt first, numbers two in one folder,
// and drops the ones gone quiet.
func TestTheBoardShowsTheAgentsStillWorthShowing(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	board := Board(now, []Status{
		{Session: "b", State: Waiting, Project: "api", At: ago(time.Minute), Since: ago(2 * time.Minute)},
		{Session: "a", State: Working, Project: "lumon", At: ago(time.Second), Since: ago(3*time.Minute + 20*time.Second), Doing: "Editing view.go"},
		{Session: "c", State: Working, Project: "lumon", At: ago(time.Second), Since: ago(time.Minute)},
		{Session: "d", State: Done, Project: "web", At: ago(5 * time.Minute), Since: ago(9 * time.Minute)},
		{Session: "e", State: Done, Project: "old", At: ago(11 * time.Minute)},
		{Session: "f", State: Working, Project: "dead", At: ago(3 * time.Hour), Since: ago(3 * time.Hour)},
	})
	want := []Line{
		{Session: "d", Name: "web", State: Done},
		{Session: "a", Name: "lumon", State: Working, Doing: "Editing view.go", For: 3 * time.Minute},
		{Session: "b", Name: "api", State: Waiting, For: 2 * time.Minute},
		{Session: "c", Name: "lumon 2", State: Working, For: time.Minute},
	}
	if len(board) != len(want) {
		t.Fatalf("board %+v", board)
	}
	for i := range want {
		if board[i] != want[i] {
			t.Errorf("line %d: %+v, want %+v", i, board[i], want[i])
		}
	}
}

// A record written by an older release, without the turn's fields, still
// reads, and has no summary.
func TestARecordFromAnOlderReleaseStillReads(t *testing.T) {
	d := testDir(t)
	os.WriteFile(d.file("s1"), []byte(`{"state":"done","project":"lumon","at":1800000000000000000}`), 0o600)
	s, ok := d.Get("s1")
	if !ok || s.State != Done || s.Took() != 0 || s.Steps != 0 {
		t.Fatalf("got %+v, %v", s, ok)
	}
}

// What the step says can never carry an escape onto the screen.
func TestAStepCannotCarryEscapes(t *testing.T) {
	d := testDir(t)
	if err := d.Write(Status{Session: "s1", State: Working, At: time.Now(), Doing: "Editing \x1b]0;pwned\x07a.go"}); err != nil {
		t.Fatal(err)
	}
	s, _ := d.Get("s1")
	for _, r := range s.Doing {
		if r < 0x20 || r > 0x7e {
			t.Fatalf("the step keeps %q", s.Doing)
		}
	}
}
