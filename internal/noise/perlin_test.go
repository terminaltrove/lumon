package noise

import (
	"math"
	"testing"
)

// These golden values were produced by v2's perlin.go, which is itself a
// verified port of siv::PerlinNoise from LumonMDR. They exist so this package's
// copy can never silently drift: the bad-number map IS the game, and changing it
// changes which digits a player is asked to refine.

func TestMT19937MatchesTheReferenceStream(t *testing.T) {
	want := []uint32{3653376905, 3874707356, 1256454805, 272500964, 958157939}

	mt := NewMT19937(505)
	for i, w := range want {
		if got := mt.Uint32(); got != w {
			t.Fatalf("output %d = %d, want %d", i, got, w)
		}
	}
}

func TestNoise2D01MatchesGoldenValues(t *testing.T) {
	cases := []struct {
		x, y int
		want float64
	}{
		{0, 0, 0.42524773074782796},
		{1, 0, 0.57486805756993198},
		{0, 1, 0.59973960826367656},
		{3, 7, 0.52148622166229219},
		{12, 5, 0.79850336639602149},
		{50, 50, 0.57475226925217204},
		{99, 99, 0.82352184986820354},
		{17, 42, 0.40981258416835753},
	}

	for _, c := range cases {
		got := PerlinBadNumbers.Noise2D_01(float64(c.x)*BadScale, float64(c.y)*BadScale)
		if math.Abs(got-c.want) > 1e-15 {
			t.Errorf("Noise2D_01 at (%d,%d) = %.17g, want %.17g", c.x, c.y, got, c.want)
		}
	}
}

func TestNoise3DMatchesGoldenValues(t *testing.T) {
	cases := []struct {
		x, y int
		time float64
		want float64
	}{
		// Gradient noise is zero at integer lattice points, so t=0 and t=1000
		// (1000*0.004 = 4.0) both land exactly on zero.
		{0, 0, 0, 0},
		{0, 0, 100, -0.19046400000000002},
		{0, 0, 1000, 0},
		{3, 7, 0, 0},
		{3, 7, 100, 0.27302399999999999},
		{3, 7, 1000, 0},
		{50, 50, 0, 0},
		{50, 50, 100, 0.46348800000000001},
		{50, 50, 1000, 0},
	}

	for _, c := range cases {
		got := PerlinAnimation.Noise3D(float64(c.x)*NoiseScale, float64(c.y)*NoiseScale, c.time*NoiseSpeed)
		if math.Abs(got-c.want) > 1e-15 {
			t.Errorf("Noise3D at (%d,%d,t=%g) = %.17g, want %.17g", c.x, c.y, c.time, got, c.want)
		}
	}
}

func TestConstantsMatchTheCppReference(t *testing.T) {
	if BadScale != 0.4 {
		t.Errorf("BadScale = %v, want 0.4 (NumberGrid.cpp)", BadScale)
	}
	if BadThreshold != 0.7 {
		t.Errorf("BadThreshold = %v, want 0.7 (NumberGrid.cpp)", BadThreshold)
	}
	if NoiseScale != 1.0 || NoiseSpeed != 0.004 || NoiseOffset != 15.0 {
		t.Errorf("animation constants = %v/%v/%v, want 1.0/0.004/15.0 (NumbersPanel.cpp)",
			NoiseScale, NoiseSpeed, NoiseOffset)
	}
}

// gridSize is the board this package is sampled over.
//
// Declared here, not imported, and it must stay equal to
// lumon.GridSize. This package cannot import lumon, lumon imports it, and the
// whole reason noise is its own package is that it depends on nothing. The
// duplication is safe because the number is not a choice: it is the 100x100 grid
// of the C++ reference's NumberGrid, which is also what the golden count below
// was taken against. Change one and the other is wrong, and the golden value is
// wrong with it.
const gridSize = 100

// The whole-map assertions. If any of these three move, the port changed which
// numbers are bad.

func TestBadNumberCountMatchesV2(t *testing.T) {
	count := 0
	for y := 0; y < gridSize; y++ {
		for x := 0; x < gridSize; x++ {
			if IsBadNumber(x, y) {
				count++
			}
		}
	}

	if count != 808 {
		t.Errorf("bad numbers in a %dx%d grid = %d, want 808", gridSize, gridSize, count)
	}
}

func TestFirstBadCoordinatesInScanOrderMatchV2(t *testing.T) {
	want := [][2]int{
		{7, 0}, {25, 0}, {43, 0}, {80, 0}, {81, 0}, {85, 0},
		{97, 0}, {6, 1}, {7, 1}, {50, 1}, {85, 1}, {6, 2},
	}

	var got [][2]int
	for y := 0; y < gridSize && len(got) < len(want); y++ {
		for x := 0; x < gridSize && len(got) < len(want); x++ {
			if IsBadNumber(x, y) {
				got = append(got, [2]int{x, y})
			}
		}
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("bad coord %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// A hash over the entire map catches any difference the spot checks miss.
func TestFullBadMapFingerprintMatchesV2(t *testing.T) {
	var h uint64 = 14695981039346656037
	for y := 0; y < gridSize; y++ {
		for x := 0; x < gridSize; x++ {
			var b byte
			if IsBadNumber(x, y) {
				b = 1
			}
			h ^= uint64(b)
			h *= 1099511628211
		}
	}

	if h != 6214508363073316245 {
		t.Errorf("bad-map FNV-1a = %d, want 6214508363073316245", h)
	}
}
