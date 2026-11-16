// Package agent connects the game to coding agents such as Claude Code. Hooks
// in the agent record whether it is working, waiting for approval or done; a
// game started with --agent-status watches those records and tells the player.
// It is native only: the browser build never imports it.
package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// State is what an agent is doing.
type State string

const (
	Working State = "working"
	Waiting State = "waiting" // for the player's approval
	Done    State = "done"
)

// Status is one agent session's latest state. At is when it last changed,
// which tells one prompt's record from the next.
type Status struct {
	Session string
	State   State
	Project string
	At      time.Time

	// The turn so far: when its prompt was sent, the step it is on, how many
	// steps it has taken, and the files it has edited, kept as hashes so no
	// path is stored.
	Since time.Time
	Doing string
	Steps int
	Edits []uint32
}

// record is a status on disk. Session is the file's name, not a field, so a
// file cannot claim to be another session. The turn's fields are optional, so
// a record from an older release still reads.
type record struct {
	State   State    `json:"state"`
	Project string   `json:"project"`
	At      int64    `json:"at"` // Unix nanoseconds
	Since   int64    `json:"since,omitempty"`
	Doing   string   `json:"doing,omitempty"`
	Steps   int      `json:"steps,omitempty"`
	Edits   []uint32 `json:"edits,omitempty"`
}

// editsMax is the most edited files a turn counts.
const editsMax = 64

// Dir is a private directory of status files, one per session.
type Dir string

const statusExt = ".status"

// OpenDir makes the directory if it is missing and refuses one that others can
// write to, or a symlink, since its files decide when a window opens.
func OpenDir(path string) (Dir, error) {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", path)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return "", fmt.Errorf("%s can be written by others", path)
	}
	return Dir(path), nil
}

// ValidSession reports whether id is safe to name a file: 1 to 128 letters,
// digits, dashes and underscores.
func ValidSession(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// labelMax is the most characters of a project name shown.
const labelMax = 32

// Label is the name to show for a working directory: its last element, with
// only ASCII letters, digits, spaces, dots, dashes and underscores kept, none
// of the dots or spaces at its ends, so a directory name cannot carry terminal
// escapes onto the player's screen.
func Label(cwd string) string {
	return clean(filepath.Base(cwd))
}

func clean(s string) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len() == labelMax {
			break
		}
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			r == ' ' || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if out := strings.Trim(b.String(), " ."); out != "" {
		return out
	}
	return "a project"
}

// doingMax is the most characters of a step shown.
const doingMax = 40

// cleanDoing keeps a step's printable ASCII, so a tool's name or a file's
// cannot carry terminal escapes onto the player's screen.
func cleanDoing(s string) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len() == doingMax {
			break
		}
		if r >= 0x20 && r <= 0x7e {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func unixOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

func timeOrZero(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n)
}

func (d Dir) file(session string) string {
	return filepath.Join(string(d), session+statusExt)
}

// Write records s, replacing the session's previous status in one rename so a
// reader never sees half a file.
func (d Dir) Write(s Status) error {
	if !ValidSession(s.Session) {
		return errors.New("not a session id")
	}
	if len(s.Edits) > editsMax {
		s.Edits = s.Edits[:editsMax]
	}
	data, err := json.Marshal(record{
		State: s.State, Project: clean(s.Project), At: s.At.UnixNano(),
		Since: unixOrZero(s.Since), Doing: cleanDoing(s.Doing), Steps: max(s.Steps, 0), Edits: s.Edits,
	})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(string(d), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), d.file(s.Session)); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// recordMax bounds what is read from one status file.
const recordMax = 4 << 10

// Get reads one session's status.
func (d Dir) Get(session string) (Status, bool) {
	if !ValidSession(session) {
		return Status{}, false
	}
	f, err := os.Open(d.file(session))
	if err != nil {
		return Status{}, false
	}
	defer f.Close()
	var r record
	if err := json.NewDecoder(io.LimitReader(f, recordMax)).Decode(&r); err != nil {
		return Status{}, false
	}
	switch r.State {
	case Working, Waiting, Done:
	default:
		return Status{}, false
	}
	if len(r.Edits) > editsMax {
		r.Edits = r.Edits[:editsMax]
	}
	return Status{
		Session: session, State: r.State, Project: clean(r.Project), At: time.Unix(0, r.At),
		Since: timeOrZero(r.Since), Doing: cleanDoing(r.Doing), Steps: max(r.Steps, 0), Edits: r.Edits,
	}, true
}

// Read is every session's status; unreadable files are skipped.
func (d Dir) Read() []Status {
	entries, err := os.ReadDir(string(d))
	if err != nil {
		return nil
	}
	var out []Status
	for _, e := range entries {
		session, ok := strings.CutSuffix(e.Name(), statusExt)
		if !ok || !e.Type().IsRegular() {
			continue
		}
		if s, ok := d.Get(session); ok {
			out = append(out, s)
		}
	}
	return out
}

// Remove forgets a session.
func (d Dir) Remove(session string) {
	if ValidSession(session) {
		os.Remove(d.file(session))
	}
}

// Prune forgets sessions whose status is older than age, which ended without
// a SessionEnd hook.
func (d Dir) Prune(now time.Time, age time.Duration) {
	for _, s := range d.Read() {
		if now.Sub(s.At) > age {
			d.Remove(s.Session)
		}
	}
}

// Watch turns successive reads into news: an agent that has just finished or
// started waiting for approval.
type Watch struct {
	seen map[string]State
}

// NewWatch starts a watch with nothing seen.
func NewWatch() *Watch { return &Watch{seen: map[string]State{}} }

// freshFor is how recent a finish must be to be news when it is the first
// thing seen of a session: one that finished as the game opened is, and a
// record left from yesterday is not.
const freshFor = time.Minute

// Next takes a fresh read at now and returns the statuses that are news.
func (w *Watch) Next(now time.Time, read []Status) []Status {
	var news []Status
	seen := make(map[string]State, len(read))
	for _, s := range read {
		was, known := w.seen[s.Session]
		if s.State != was && (s.State == Done || s.State == Waiting) &&
			(known || now.Sub(s.At) <= freshFor) {
			news = append(news, s)
		}
		seen[s.Session] = s.State
	}
	// Ended sessions are forgotten with their files.
	w.seen = seen
	return news
}
