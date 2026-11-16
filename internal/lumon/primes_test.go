package lumon

import (
	"testing"
)

// The numbers grow past a single digit, so a table of 2, 3, 5 and 7 would make
// the board go dark above nine.
func TestPrimalityHoldsPastOneDigit(t *testing.T) {
	for _, n := range []int{2, 3, 5, 7, 11, 13, 17, 19, 23, 29, 31, 97, 101, 9973} {
		if !prime(n) {
			t.Errorf("prime(%d) = false", n)
		}
	}
	for _, n := range []int{-3, 0, 1, 4, 6, 8, 9, 15, 21, 25, 49, 100, 9999} {
		if prime(n) {
			t.Errorf("prime(%d) = true", n)
		}
	}
}

// Folding a sum back to one digit would cap the board at nine.
func TestSumsGrowPastOneDigit(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "PRIMES")
	p := m.primes
	if p.Len() < 4 {
		t.Skip("too few digits")
	}

	p.value[0], p.value[1] = 8, 7
	p.deaf[0], p.deaf[1] = 0, 0
	p.pending = append(p.pending, [2]int{0, 1})
	p.settle()

	if p.value[0] != 15 {
		t.Errorf("8 and 7 made %d, want 15", p.value[0])
	}

	p.value[1] = 9
	p.deaf[0], p.deaf[1] = 0, 0
	p.pending = append(p.pending, [2]int{0, 1})
	p.settle()
	if p.value[0] != 24 {
		t.Errorf("15 and 9 made %d, want 24", p.value[0])
	}
}

// Left alone a number climbs without limit and grows wider than the screen. At
// the ceiling it is banked and sent back to a digit.
func TestANumberIsBankedAtTheCeiling(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "PRIMES")
	p := m.primes
	if p.Len() < 4 {
		t.Skip("too few digits")
	}

	p.value[0], p.value[1] = primeCeiling, 5
	p.deaf[0], p.deaf[1] = 0, 0
	banked := p.banked
	p.pending = append(p.pending, [2]int{0, 1})
	p.settle()

	if p.banked != banked+1 {
		t.Error("passing the ceiling did not bank the number")
	}
	if p.value[0] > 9 || p.value[0] < 1 {
		t.Errorf("a banked number came back as %d, want a single digit", p.value[0])
	}
}

func TestThePrimesBoardDrifts(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "PRIMES")
	p := m.primes

	before := make([]float64, p.world.Len())
	for i := range before {
		before[i] = p.world.Pos(i).X
	}
	tick(m, 60)

	moved := 0
	for i := range before {
		if p.world.Pos(i).X != before[i] {
			moved++
		}
	}
	if moved < len(before)/2 {
		t.Errorf("only %d of %d digits moved in a second", moved, len(before))
	}
}

// A board that drained would end the game a minute in.
func TestTouchingDigitsAddUpAndTheBoardNeverDrains(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "PRIMES")
	p := m.primes

	n := p.Len()
	if n < 10 {
		t.Skip("too few digits")
	}

	p.value[0], p.value[1] = 8, 7
	p.world.SetPos(1, p.world.Pos(0))
	p.deaf[0], p.deaf[1] = 0, 0

	tick(m, 3)

	if p.Merges() == 0 {
		t.Fatal("two digits sitting on each other did not add up")
	}
	if p.value[0] != 8+7 {
		t.Errorf("8 and 7 made %d, want %d", p.value[0], 8+7)
	}
	if p.Len() != n {
		t.Errorf("the board went from %d digits to %d; a merge must replace, not remove",
			n, p.Len())
	}
}

// The hook fires once per pair per relaxation pass, forty-eight times a step, so
// without a cooldown one touch would run the pair through every value in a
// frame. It calls settle directly because on a live board other digits collide
// too and the count would not be this pair's.
func TestAPairInContactMergesOnceNotFortyEightTimes(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "PRIMES")
	p := m.primes
	if p.Len() < 4 {
		t.Skip("too few digits")
	}

	p.value[0], p.value[1] = 4, 3
	p.deaf[0], p.deaf[1] = 0, 0

	// What one step of sustained contact delivers to settle.
	for k := 0; k < 48; k++ {
		p.pending = append(p.pending, [2]int{0, 1})
	}
	before := p.Merges()
	p.settle()

	if got := p.Merges() - before; got != 1 {
		t.Errorf("forty-eight reports of one pair produced %d merges, want 1", got)
	}
	if p.value[0] != 4+3 {
		t.Errorf("4 and 3 made %d, want %d", p.value[0], 4+3)
	}

	// They stay deaf until the cooldown runs out.
	for k := 0; k < 48; k++ {
		p.pending = append(p.pending, [2]int{0, 1})
	}
	p.settle()
	if got := p.Merges() - before; got != 1 {
		t.Errorf("the pair merged again inside its cooldown: %d merges total", got)
	}

	p.deaf[0], p.deaf[1] = 0, 0
	p.pending = append(p.pending, [2]int{0, 1})
	p.settle()
	if got := p.Merges() - before; got != 2 {
		t.Errorf("after the cooldown the pair would not merge again: %d merges total", got)
	}
}

func TestOnlyPrimesGlow(t *testing.T) {
	m := ready()
	m.Resize(110, 30)
	startDiversionNamed(t, m, "PRIMES")
	p := m.primes
	if p.Len() < 4 {
		t.Skip("too few digits")
	}

	// One of each, well apart so neither is held or merging.
	p.value[0], p.value[1] = 7, 8
	tick(m, 1)

	buf := drawTo(m)
	panel := m.gridPanel().Play()
	at := func(i int) (int, int) {
		pos := p.world.Pos(i)
		return int(pos.X + 0.5), int(pos.Y/RowAspect + 0.5)
	}

	px, py := at(0)
	cx, cy := at(1)
	if !panel.Contains(px, py) || !panel.Contains(cx, cy) {
		t.Skip("the sample digits are off the panel")
	}
	if got := buf.AttrAt(px, py); got != m.Theme.BadLive {
		t.Errorf("the prime 7 is drawn %v, want the lit %v", got, m.Theme.BadLive)
	}
	if got := buf.AttrAt(cx, cy); got != m.Theme.Digit {
		t.Errorf("the composite 8 is drawn %v, want the ordinary %v", got, m.Theme.Digit)
	}
}

// It is played on a copy, so it must neither empty the shared board nor count.
func TestPlayingPrimesDoesNotTouchTheRealBoard(t *testing.T) {
	m := ready()
	m.Resize(120, 34)
	m.Name = "MARK S."
	m.Desk.Name = m.Name

	before := corruptCells(m.Grid)
	beforeBins := [BinCount]int{}
	for i := range m.Grid.Bins {
		beforeBins[i] = m.Grid.Bins[i].Refined
	}

	startDiversionNamed(t, m, "PRIMES")
	tick(m, 60*5)

	if after := corruptCells(m.Grid); len(after) != len(before) {
		t.Errorf("the board went from %d corrupt cells to %d", len(before), len(after))
	}
	for i := range m.Grid.Bins {
		if m.Grid.Bins[i].Refined != beforeBins[i] {
			t.Errorf("bin %d changed while somebody was playing", i+1)
		}
	}
	if st, ok := m.Ledger().Of("MARK S."); ok && st.Refined > 0 {
		t.Errorf("playing the game credited %d clusters", st.Refined)
	}
}
