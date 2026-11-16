package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func testDir(t *testing.T) Dir {
	t.Helper()
	d, err := OpenDir(filepath.Join(t.TempDir(), "status"))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// What one hook writes, the game reads back.
func TestAStatusReadsBackAsWritten(t *testing.T) {
	d := testDir(t)
	at := time.Unix(1_800_000_000, 0)
	want := Status{Session: "abc-123", State: Done, Project: "lumon", At: at}
	if err := d.Write(want); err != nil {
		t.Fatal(err)
	}
	got, ok := d.Get("abc-123")
	if !ok || got.State != Done || got.Project != "lumon" || !got.At.Equal(at) {
		t.Fatalf("read back %+v, %v; wrote %+v", got, ok, want)
	}
	all := d.Read()
	if len(all) != 1 || all[0].Session != "abc-123" {
		t.Fatalf("Read gave %+v", all)
	}
}

// A session id names a file, so one that could leave the directory, or is not
// a plain id, is refused.
func TestASessionIDThatCouldNameAnotherFileIsRefused(t *testing.T) {
	d := testDir(t)
	for _, id := range []string{"", "../x", "a/b", ".", "..", "a b", "a\x00b", strings.Repeat("a", 129)} {
		if ValidSession(id) {
			t.Errorf("%q passed as a session id", id)
		}
		if err := d.Write(Status{Session: id, State: Working}); err == nil {
			t.Errorf("wrote a status for session %q", id)
		}
	}
	for _, id := range []string{"0f8e2c1a-6b1d-4c39-9a57-3b4d1d1a2b3c", "abc_DEF-9"} {
		if !ValidSession(id) {
			t.Errorf("%q refused as a session id", id)
		}
	}
}

// A project name is drawn on the player's terminal, so whatever the directory
// is called, only plain characters reach it, and not too many.
func TestAProjectNameCannotDrawOnTheTerminal(t *testing.T) {
	for cwd, want := range map[string]string{
		"/home/mark/macrodata":      "macrodata",
		"/tmp/\x1b]0;owned\x07evil": "0ownedevil",
		"/tmp/café bar":             "caf bar",
		"/":                         "a project",
		"":                          "a project",
		"/x/\t":                     "a project",
		"/x/abcdefghijklmnopqrstuvwxyz0123456789": "abcdefghijklmnopqrstuvwxyz012345",
	} {
		if got := Label(cwd); got != want {
			t.Errorf("Label(%q) = %q, want %q", cwd, got, want)
		}
	}
}

// A status file someone else put there with a hostile project is cleaned as
// it is read, not only as it is written.
func TestAProjectIsCleanedOnTheWayIn(t *testing.T) {
	d := testDir(t)
	raw := `{"state":"done","project":"\u001b[2Jx","at":1}`
	if err := os.WriteFile(filepath.Join(string(d), "s1.status"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok := d.Get("s1")
	if !ok || got.Project != "2Jx" {
		t.Fatalf("read %+v, %v", got, ok)
	}
}

// The directory holds who is working on what, so it is the player's alone.
func TestTheStatusDirectoryIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no unix permissions")
	}
	d := testDir(t)
	info, err := os.Stat(string(d))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("directory mode %v", info.Mode().Perm())
	}

	open := filepath.Join(t.TempDir(), "open")
	if err := os.Mkdir(open, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(open, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenDir(open); err == nil {
		t.Error("a world-writable directory was accepted")
	}

	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(string(d), link); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenDir(link); err == nil {
		t.Error("a symlink was accepted as the directory")
	}
}

// Sessions that ended without saying so are swept after a day.
func TestOldStatusesAreSwept(t *testing.T) {
	d := testDir(t)
	now := time.Now()
	for _, s := range []Status{
		{Session: "old", State: Working, At: now.Add(-25 * time.Hour)},
		{Session: "new", State: Working, At: now.Add(-time.Hour)},
	} {
		if err := d.Write(s); err != nil {
			t.Fatal(err)
		}
	}
	d.Prune(now, 24*time.Hour)
	if _, ok := d.Get("old"); ok {
		t.Error("a day-old status survived")
	}
	if _, ok := d.Get("new"); !ok {
		t.Error("an hour-old status was swept")
	}
}

// The game hears of each agent once when it finishes or wants approval, and
// again only after it has gone back to work.
func TestTheGameHearsOfEachChangeOnce(t *testing.T) {
	now := time.Now()
	w := NewWatch()
	step := func(states ...Status) []Status {
		for i := range states {
			states[i].At = now
		}
		return w.Next(now, states)
	}

	if got := step(Status{Session: "a", State: Working}); len(got) != 0 {
		t.Fatalf("working was news: %+v", got)
	}
	if got := step(Status{Session: "a", State: Done}); len(got) != 1 || got[0].State != Done {
		t.Fatalf("done was not news: %+v", got)
	}
	if got := step(Status{Session: "a", State: Done}); len(got) != 0 {
		t.Fatalf("done twice was news twice: %+v", got)
	}
	step(Status{Session: "a", State: Working})
	if got := step(Status{Session: "a", State: Waiting}); len(got) != 1 || got[0].State != Waiting {
		t.Fatalf("waiting was not news: %+v", got)
	}
	// An agent that finished just as the game started is news at once; one
	// that finished long before is not.
	if got := NewWatch().Next(now, []Status{{Session: "b", State: Done, At: now.Add(-5 * time.Second)}}); len(got) != 1 {
		t.Fatalf("a just-finished agent at start was not news: %+v", got)
	}
	if got := NewWatch().Next(now, []Status{{Session: "c", State: Done, At: now.Add(-time.Hour)}}); len(got) != 0 {
		t.Fatalf("an agent that finished an hour ago was news: %+v", got)
	}
}

// The game records itself while it runs, so a second is not opened; a record
// left by a game that died does not count.
func TestAGameIsRunningOnlyWhileItsProcessLives(t *testing.T) {
	d := testDir(t)
	if d.GameRunning() {
		t.Fatal("running before any game started")
	}
	done := d.MarkGame(os.Getpid())
	if !d.GameRunning() {
		t.Fatal("this process's game is not running")
	}
	done()
	if d.GameRunning() {
		t.Fatal("still running after the game ended")
	}

	// A pid no process has: the largest a pid file could hold.
	if err := os.WriteFile(filepath.Join(string(d), "game.pid"), []byte("2147483646"), 0o600); err != nil {
		t.Fatal(err)
	}
	if d.GameRunning() {
		t.Fatal("a dead game counts as running")
	}
}
