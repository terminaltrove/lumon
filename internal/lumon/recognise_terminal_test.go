package lumon

import (
	"fmt"
	"strings"
	"testing"
)

// Two client keys, as fingerprints. The model only ever sees the fingerprint;
// see Recognise.
const (
	keyMark  = "SHA256:markmarkmarkmarkmarkmarkmarkmarkmarkmarkmar"
	keyOther = "SHA256:otherotherotherotherotherotherotherotherot"
)

// boundMark is a floor where MARK S. has refined from keyMark.
func boundMark() *Rooms {
	rs := NewRooms(1)
	rs.Ledger().Refined("MARK S.", 12)
	rs.Ledger().Bind("MARK S.", keyMark)
	return rs
}

// recognised is a session that arrived with a key, before anything is typed.
func recognised(t *testing.T, rs *Rooms, key string) *Model {
	t.Helper()
	m := NewSession(rs, 1)
	m.Resize(120, 40)
	m.Recognise(key)
	return m
}

func TestAReturningKeyIsWelcomedBackByName(t *testing.T) {
	m := recognised(t, boundMark(), keyMark)

	if m.Name != "MARK S." {
		t.Errorf("a returning key arrived as %q, want MARK S.", m.Name)
	}
	screen := screenOf(m)
	if !strings.Contains(screen, "WELCOME BACK") || !strings.Contains(screen, "12 IN ALL") {
		t.Errorf("a returning key was not welcomed back:\n%s", screen)
	}
	m.Update(key("enter"))
	if m.Screen != ScreenLobby {
		t.Error("a refiner was refused their own name")
	}
}

func TestAnotherKeyCannotTakeABoundName(t *testing.T) {
	for _, k := range []string{keyOther, ""} {
		m := recognised(t, boundMark(), k)
		if m.Name != "" {
			t.Errorf("a stranger's terminal arrived already called %q", m.Name)
		}
		m.Update(key("mark s."))
		if !strings.Contains(screenOf(m), "BELONGS TO ANOTHER REFINER") {
			t.Errorf("typing somebody else's name does not say so:\n%s", screenOf(m))
		}
		m.Update(key("enter"))
		if m.Screen != ScreenName {
			t.Errorf("a terminal with key %q took MARK S.", k)
		}
	}
}

// A keyless refiner's name is protected by nothing.
func TestAnUnboundNameIsStillFirstComeFirstServed(t *testing.T) {
	rs := NewRooms(1)
	rs.Ledger().Refined("HELLY R.", 5)

	m := recognised(t, rs, keyOther)
	m.Update(key("helly r."))
	if !strings.Contains(screenOf(m), "ON RECORD") {
		t.Errorf("an unclaimed name does not show its record:\n%s", screenOf(m))
	}
	m.Update(key("enter"))
	if m.Screen != ScreenLobby {
		t.Error("an unclaimed name was refused")
	}
}

// Otherwise anybody could reserve every name without refining.
func TestANameIsBoundByRefiningUnderIt(t *testing.T) {
	rs := NewRooms(1)
	m := recognised(t, rs, keyMark)
	m.Update(key("irving b."))
	m.Update(key("enter"))
	m.Update(key("enter")) // a desk
	if m.Desk == nil {
		t.Fatal("setup: no desk was taken")
	}
	if s, ok := rs.Ledger().Of("IRVING B."); ok && s.Key != "" {
		t.Fatal("a name was bound before anything was refined under it")
	}

	if !refineOne(t, m) {
		t.Skip("nothing woke up")
	}
	if s, _ := rs.Ledger().Of("IRVING B."); s.Key != keyMark {
		t.Errorf("refining under IRVING B. bound it to %q, want the refiner's key", s.Key)
	}
}

func TestABindingIsNotTakenOverByAnotherKey(t *testing.T) {
	rs := boundMark()
	rs.Ledger().Bind("MARK S.", keyOther)
	if s, _ := rs.Ledger().Of("MARK S."); s.Key != keyMark {
		t.Errorf("MARK S. is bound to %q after another key tried, want the first", s.Key)
	}
}

func TestTheKeyIsNeverDrawn(t *testing.T) {
	m := recognised(t, boundMark(), keyMark)
	m.Update(key("enter"))
	m.Update(key("s"))
	for tab := 0; tab < int(scoreTabs); tab++ {
		if strings.Contains(screenOf(m), "SHA256") {
			t.Fatalf("a key fingerprint is on the screen:\n%s", screenOf(m))
		}
		m.Update(key("right"))
	}
}

func TestAQuarterRecordCarriesNoKey(t *testing.T) {
	l, now := ledgerAt(q4)
	l.Refined("MARK S.", 3)
	l.Bind("MARK S.", keyMark)
	*now = now.AddDate(0, 3, 0)
	l.Refined("HELLY R.", 1)

	for _, q := range l.Past() {
		for _, s := range q.Top {
			if s.Key != "" {
				t.Errorf("the record of %s carries %s's key", q.Quarter, s.Name)
			}
		}
	}
}

func TestOneKeyCanHoldOnlyAFewNames(t *testing.T) {
	l := NewRooms(1).Ledger()
	for i := 0; i < namesPerKey+2; i++ {
		name := "REFINER " + string(rune('A'+i))
		l.Refined(name, 1)
		l.Bind(name, keyMark)
	}
	held := 0
	for _, st := range l.Standings() {
		if st.Key == keyMark {
			held++
		}
	}
	if held != namesPerKey {
		t.Errorf("one key holds %d names, want at most %d", held, namesPerKey)
	}
}

func TestABoundNameIsForgottenLast(t *testing.T) {
	rs := boundMark()
	l := rs.Ledger()
	// Fill the ledger with newer, unbound names until it has to forget some.
	for i := 0; len(l.Standings()) < LedgerLimit; i++ {
		l.Refined(fmt.Sprintf("R%d", i), 1)
	}
	l.Refined("ONE MORE", 1)
	if name, ok := l.NameFor(keyMark); !ok || name != "MARK S." {
		t.Errorf("the oldest name, bound to a key, was forgotten before unbound ones (got %q)", name)
	}
}
