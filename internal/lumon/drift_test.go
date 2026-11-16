package lumon

import (
	"math"
	"testing"
)

func driftFor(l *Layout) *Drift {
	d := NewDrift()
	d.Sync(l)
	return d
}

func TestDriftCoversEveryVisibleCell(t *testing.T) {
	l := NewLayout(120, 40)
	d := driftFor(l)

	if got := d.Len(); got != l.ViewportW*l.ViewportH {
		t.Errorf("drift tracks %d cells, want %d", got, l.ViewportW*l.ViewportH)
	}
}

func TestOrdinaryNumbersDrift(t *testing.T) {
	l := NewLayout(120, 40)
	d := driftFor(l)

	for i := 0; i < 240; i++ {
		d.Step(1.0 / 60)
	}

	moved := 0
	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			if dx, dy := d.Offset(gx, gy); dx != 0 || dy != 0 {
				moved++
			}
		}
	}
	if moved == 0 {
		t.Fatal("no background number drifted")
	}
}

// Drift must stay subtle enough that a waking cluster still catches the eye.
func TestDriftStaysSubtle(t *testing.T) {
	l := NewLayout(120, 40)
	d := driftFor(l)

	for i := 0; i < 3000; i++ {
		d.Step(1.0 / 60)
		for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
			for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
				dx, dy := d.Offset(gx, gy)
				if m := math.Hypot(dx, dy); m > MaxDriftOffset+1e-9 {
					t.Fatalf("cell (%d,%d) drifted %v cells, want at most %v", gx, gy, m, MaxDriftOffset)
				}
			}
		}
	}
}

func TestNeighbouringNumbersDriftCoherently(t *testing.T) {
	l := NewLayout(120, 40)
	d := driftFor(l)
	for i := 0; i < 240; i++ {
		d.Step(1.0 / 60)
	}

	agree, total := 0, 0
	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW-1; gx++ {
			a, _ := d.Offset(gx, gy)
			b, _ := d.Offset(gx+1, gy)
			if a == 0 || b == 0 {
				continue
			}
			total++
			if (a < 0) == (b < 0) {
				agree++
			}
		}
	}
	if total == 0 {
		t.Skip("nothing drifted yet")
	}
	if float64(agree)/float64(total) < 0.6 {
		t.Errorf("only %d of %d neighbour pairs drift the same way; want a coherent field", agree, total)
	}
}

func TestOffsetOutsideTheViewportIsZero(t *testing.T) {
	l := NewLayout(120, 40)
	d := driftFor(l)
	for i := 0; i < 60; i++ {
		d.Step(1.0 / 60)
	}

	if dx, dy := d.Offset(l.ViewportX+l.ViewportW+5, l.ViewportY); dx != 0 || dy != 0 {
		t.Errorf("a cell right of the viewport reported (%v,%v)", dx, dy)
	}
	if dx, dy := d.Offset(-1, -1); dx != 0 || dy != 0 {
		t.Errorf("a negative cell reported (%v,%v)", dx, dy)
	}
}

func TestScrollingRebasesTheDriftPool(t *testing.T) {
	l := NewLayout(120, 40)
	d := driftFor(l)
	for i := 0; i < 240; i++ {
		d.Step(1.0 / 60)
	}

	l.ScrollTo(l.ViewportX+10, l.ViewportY)
	d.Sync(l)

	for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
		for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
			if dx, dy := d.Offset(gx, gy); math.Hypot(dx, dy) > MaxDriftOffset+1e-9 {
				t.Fatalf("cell (%d,%d) is out of bounds right after a scroll", gx, gy)
			}
		}
	}
}

func TestDriftIsDeterministic(t *testing.T) {
	run := func() []float64 {
		l := NewLayout(120, 40)
		d := driftFor(l)
		for i := 0; i < 180; i++ {
			d.Step(1.0 / 60)
		}
		var out []float64
		for gx := l.ViewportX; gx < l.ViewportX+5; gx++ {
			dx, dy := d.Offset(gx, l.ViewportY)
			out = append(out, dx, dy)
		}
		return out
	}

	a, b := run(), run()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("value %d differs: %v vs %v", i, a[i], b[i])
		}
	}
}

func TestResizingGrowsThePool(t *testing.T) {
	l := NewLayout(60, 20)
	d := driftFor(l)
	small := d.Len()

	big := NewLayout(160, 50)
	d.Sync(big)

	if d.Len() <= small {
		t.Errorf("pool went from %d to %d cells, want it to grow", small, d.Len())
	}
	if d.Len() != big.ViewportW*big.ViewportH {
		t.Errorf("pool is %d, want %d", d.Len(), big.ViewportW*big.ViewportH)
	}
}

// Over fifteen seconds a background digit changes column a handful of times.
// Plain rounding of the same motion changes column every few frames whenever
// the offset lingers near a boundary.
func TestBackgroundDigitsChangeColumnRarely(t *testing.T) {
	l := NewLayout(120, 40)
	d := driftFor(l)

	const frames = 900 // fifteen seconds
	last := map[int]int{}
	flips := map[int]int{}

	for i := 0; i < frames; i++ {
		d.Step(1.0 / 60)
		for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH; gy++ {
			for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
				id := GridIndex(gx, gy)
				c := d.ColumnOffset(NewLayout(120, 40), gx, gy)
				if prev, seen := last[id]; seen && prev != c {
					flips[id]++
				}
				last[id] = c
			}
		}
	}

	worst, total := 0, 0
	for _, n := range flips {
		if n > worst {
			worst = n
		}
		total += n
	}
	if worst > 14 {
		t.Errorf("a digit changed column %d times in 15s; want a calm drift", worst)
	}
	if total == 0 {
		t.Error("no digit ever changed column; the drift is dead")
	}
}

func TestBackgroundDigitsDoReachAnAdjacentColumn(t *testing.T) {
	l := NewLayout(120, 40)
	d := driftFor(l)

	reached := false
	for i := 0; i < 1800 && !reached; i++ {
		d.Step(1.0 / 60)
		for gy := l.ViewportY; gy < l.ViewportY+l.ViewportH && !reached; gy++ {
			for gx := l.ViewportX; gx < l.ViewportX+l.ViewportW; gx++ {
				if d.ColumnOffset(NewLayout(120, 40), gx, gy) != 0 {
					reached = true
					break
				}
			}
		}
	}
	if !reached {
		t.Error("no digit ever left its home column; the dead band has swallowed the drift")
	}
}

func TestColumnOffsetIsZeroOutsideTheViewport(t *testing.T) {
	l := NewLayout(120, 40)
	d := driftFor(l)
	for i := 0; i < 300; i++ {
		d.Step(1.0 / 60)
	}

	if got := d.ColumnOffset(NewLayout(120, 40), -5, -5); got != 0 {
		t.Errorf("ColumnOffset off-grid = %d, want 0", got)
	}
}
