package lumon

import (
	"sync"
	"testing"
	"time"
)

// onRoll reports whether a name is on the floor.
func (rs *Rooms) onRoll(id string) bool {
	rs.roll.mu.Lock()
	defer rs.roll.mu.Unlock()
	_, held := rs.roll.names[id]
	return held
}

// Ownership is checked with the name held. Checked first, the holder could bind
// the name and leave between the check and the claim. The ledger's lock holds the
// claimant at the check, and the name is bound there, as that holder would.
func TestANameBoundDuringAClaimIsStillRefused(t *testing.T) {
	rs := NewRooms(1)
	l := rs.Ledger()
	l.Refined("MARK", 1)
	m := session(t, rs, "mark")
	m.key = "SHA256:latecomer"

	l.mu.Lock()
	finished := make(chan struct{})
	go func() {
		m.Update(key("enter"))
		close(finished)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for !rs.onRoll("MARK") {
		if time.Now().After(deadline) {
			l.mu.Unlock()
			<-finished
			t.Fatal("the name's owner was checked before the name was held")
		}
		time.Sleep(time.Millisecond)
	}
	l.by["MARK"].Key = "SHA256:owner"
	l.mu.Unlock()
	<-finished

	if m.Screen != ScreenName || !m.nameRefused || m.nameNote != nameOwned {
		t.Errorf("screen %v, note %q; a name bound to another key was let through", m.Screen, m.nameNote)
	}
	if rs.onRoll("MARK") {
		t.Error("the refused terminal left the name held")
	}
}

func TestANameHeldAndOwnedBySomebodyElseSaysItIsOwned(t *testing.T) {
	rs := NewRooms(1)
	l := rs.Ledger()
	l.Refined("MARK", 1)
	l.Bind("MARK", "SHA256:owner")
	holder, ok := rs.Claim("MARK")
	if !ok {
		t.Fatal("the owner could not claim the name")
	}
	m := session(t, rs, "mark")
	m.Update(key("enter"))

	if m.Screen != ScreenName || m.nameNote != nameOwned {
		t.Errorf("screen %v, note %q; want refused as owned", m.Screen, m.nameNote)
	}
	if rs.roll.names["MARK"] != holder {
		t.Error("the refusal disturbed the holder's claim")
	}
}

// Coming back through the name prompt under the same name is not a second claim.
func TestPressingEnterAgainKeepsTheNameHeld(t *testing.T) {
	rs := NewRooms(1)
	m := session(t, rs, "mark")
	m.Update(key("enter"))
	m.Update(key("q"))
	if m.Screen != ScreenName {
		t.Fatal("did not back out to the name prompt")
	}
	m.Update(key("enter"))
	if m.Screen != ScreenLobby {
		t.Fatalf("screen %v, note %q; a terminal was refused its own name", m.Screen, m.nameNote)
	}
	m.Close()
	if rs.onRoll("MARK") {
		t.Error("the name was held after its terminal closed")
	}
}

// Changing name frees the old one, and only once the new one is held.
func TestAFailedRenameKeepsTheOldNameAndASuccessfulOneFreesIt(t *testing.T) {
	rs := NewRooms(1)
	if _, ok := rs.Claim("HELLY"); !ok {
		t.Fatal("could not claim")
	}
	m := session(t, rs, "mark")
	m.Update(key("enter"))
	m.Update(key("q"))
	for range "MARK" {
		m.Update(key("backspace"))
	}
	m.Update(key("helly"))
	m.Update(key("enter"))
	if m.Screen != ScreenName || !rs.onRoll("MARK") {
		t.Errorf("after a refused rename: screen %v, old name held %v", m.Screen, rs.onRoll("MARK"))
	}
	for range "HELLY" {
		m.Update(key("backspace"))
	}
	m.Update(key("irving"))
	m.Update(key("enter"))
	if m.Screen != ScreenLobby || rs.onRoll("MARK") || !rs.onRoll("IRVING") {
		t.Errorf("after a rename: screen %v, old held %v, new held %v",
			m.Screen, rs.onRoll("MARK"), rs.onRoll("IRVING"))
	}
}

// Everyone pressing enter on one name at once: one has it, and when that one
// leaves it is free. Run under -race.
func TestOnlyOneOfManyClaimantsGetsAName(t *testing.T) {
	rs := NewRooms(1)
	const claimants = 32
	all := make([]*Model, claimants)
	for i := range all {
		all[i] = session(t, rs, "mark")
	}

	var wg sync.WaitGroup
	for _, m := range all {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.Update(key("enter"))
		}()
	}
	wg.Wait()

	var won *Model
	for _, m := range all {
		if m.Screen != ScreenLobby {
			continue
		}
		if won != nil {
			t.Fatal("two terminals got one name")
		}
		won = m
	}
	if won == nil {
		t.Fatal("nobody got the name")
	}
	won.Close()
	if _, ok := rs.Claim("MARK"); !ok {
		t.Error("the name was not free after its holder left")
	}
}

// However it is typed or pasted, a name has one spelling, so a lookalike outside
// the house alphabet cannot be a second holder of a name or get round its owner.
func TestLookalikesAreTheSameNameOrNoName(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"mark", "MARK"},
		{"MaRk", "MARK"},
		{"  mark\t", "MARK"},
		{"mark\u200b", "MARK"},       // zero width space
		{"\u00a0mark\u00a0", "MARK"}, // no-break space
		{"ma\u0301rk", "MARK"},       // combining accent
		{"\ufeffmark", "MARK"},       // byte order mark
		{"\u202emark", "MARK"},       // right-to-left override
		{"marſ", "MARS"},             // long s, whose capital is S
		{"ıvan", "IVAN"},             // dotless i, whose capital is I
		{"МАРК", ""},                 // Cyrillic
		{"ＭＡＲＫ", ""},                 // full width
		{"m\x1b[31mark", "M31MARK"},  // escape sequences do not survive
	} {
		if got := tidyName(c.in); got != c.want {
			t.Errorf("tidyName(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	rs := NewRooms(1)
	rs.Ledger().Refined("MARK", 1)
	rs.Ledger().Bind("MARK", "SHA256:owner")
	m := session(t, rs, "mаrk\u200bmark"[:0]+"mark\u200b")
	m.Update(key("enter"))
	if m.Screen != ScreenName || m.nameNote != nameOwned {
		t.Errorf("a lookalike got round the owner: screen %v, note %q", m.Screen, m.nameNote)
	}
}

// Every way a name reaches the ledger tidies it, some twice, so tidying twice
// must give the same name.
func TestTidyingANameTwiceChangesNothing(t *testing.T) {
	for _, in := range []string{"mark s.", "abcdefghijk l", "abcdefghijkl m", "a  b", "abcdefghijk  ", "- ."} {
		once := tidyName(in)
		if twice := tidyName(once); twice != once {
			t.Errorf("tidyName(%q) = %q, and tidied again %q", in, once, twice)
		}
	}
	if got := tidyName("abcdefghijk l"); got != "ABCDEFGHIJK" {
		t.Errorf("a space in the last place left %q", got)
	}
}

// The directory offers cards that open files by name, one keypress each, and they
// must stop at the same ceiling as "new file".
func TestOpeningNamedFilesStopsAtTheCeiling(t *testing.T) {
	rs := NewRooms(1)
	for range MaxRooms {
		rs.Open()
	}
	for _, name := range DirectoryFiles {
		if rs.OpenNamed(name) == nil {
			t.Fatalf("no room for %s", name)
		}
	}
	if got := rs.Count(); got > MaxRooms {
		t.Errorf("%d files open, over the ceiling of %d", got, MaxRooms)
	}
	// One already open is still the one returned.
	want := rs.List()[0].Name
	if got := rs.OpenNamed(want).CurrentFile(); got != want {
		t.Errorf("asked for %s, an open file, and got %s", want, got)
	}
}
