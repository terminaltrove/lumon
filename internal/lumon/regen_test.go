package lumon

import "testing"

func TestRegenStartsScramblingImmediately(t *testing.T) {
	r := NewRegen(seededRand(1))

	r.Begin(42, 7)

	if !r.Active(42) {
		t.Error("the cell is not regenerating")
	}
	if _, ok := r.Display(42); !ok {
		t.Error("a regenerating cell shows nothing")
	}
}

func TestUntouchedCellIsNotRegenerating(t *testing.T) {
	r := NewRegen(seededRand(1))

	if r.Active(99) {
		t.Error("an untouched cell reports as regenerating")
	}
	if _, ok := r.Display(99); ok {
		t.Error("an untouched cell offers a display value")
	}
}

func TestRegenScramblesThroughSeveralDigits(t *testing.T) {
	r := NewRegen(seededRand(3))
	r.Begin(42, 7)

	seen := map[int]bool{}
	for i := 0; i < 600 && r.Active(42); i++ {
		if v, ok := r.Display(42); ok {
			seen[v] = true
		}
		r.Step(1.0 / 60)
	}

	if len(seen) < 4 {
		t.Errorf("the cell showed only %d distinct digits while regenerating, want a scramble", len(seen))
	}
}

// Changes are dense early and sparse late.
func TestRegenScrambleSlowsAsItSettles(t *testing.T) {
	r := NewRegen(seededRand(5))
	r.Begin(42, 7)

	var early, late int
	last := -1
	frames := 0
	for r.Active(42) && frames < 900 {
		if v, ok := r.Display(42); ok {
			if v != last {
				if frames < 30 {
					early++
				} else if frames > 60 {
					late++
				}
			}
			last = v
		}
		r.Step(1.0 / 60)
		frames++
	}

	if early == 0 {
		t.Fatal("the cell never changed digit early on")
	}
	if late >= early {
		t.Errorf("the scramble did not slow: %d changes early, %d late", early, late)
	}
}

func TestRegenSettlesOnTheFinalValue(t *testing.T) {
	r := NewRegen(seededRand(7))
	r.Begin(42, 7)

	for i := 0; i < 900 && r.Active(42); i++ {
		r.Step(1.0 / 60)
	}

	if r.Active(42) {
		t.Fatal("the cell never finished regenerating")
	}
	if _, ok := r.Display(42); ok {
		t.Error("a finished cell still offers a scramble value")
	}
}

// Brightness rises with the same easing as the swell.
func TestRegenDensityRisesOverTime(t *testing.T) {
	r := NewRegen(seededRand(9))
	r.Begin(42, 7)

	first := r.Density(42)
	for i := 0; i < 30; i++ {
		r.Step(1.0 / 60)
	}
	mid := r.Density(42)

	if mid <= first {
		t.Errorf("density went %v -> %v, want it to rise", first, mid)
	}
	if first > 0.2 {
		t.Errorf("regeneration started at density %v, want it to fade up from near nothing", first)
	}
}

func TestRegenHandlesManyCellsAtOnce(t *testing.T) {
	r := NewRegen(seededRand(11))
	for id := 0; id < 12; id++ {
		r.Begin(id, id%10)
	}

	for i := 0; i < 900; i++ {
		r.Step(1.0 / 60)
	}

	for id := 0; id < 12; id++ {
		if r.Active(id) {
			t.Fatalf("cell %d never settled", id)
		}
	}
}

func TestRegenIsDeterministic(t *testing.T) {
	run := func() []int {
		r := NewRegen(seededRand(13))
		r.Begin(42, 7)
		var seq []int
		for i := 0; i < 120; i++ {
			if v, ok := r.Display(42); ok {
				seq = append(seq, v)
			}
			r.Step(1.0 / 60)
		}
		return seq
	}

	a, b := run(), run()
	if len(a) != len(b) {
		t.Fatalf("lengths differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("step %d: %d vs %d", i, a[i], b[i])
		}
	}
}
