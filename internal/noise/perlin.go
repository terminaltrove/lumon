package noise

import (
	"math"
)

// PerlinNoise is Perlin noise matching C++ siv::PerlinNoise v3.0.0.
type PerlinNoise struct {
	perm [512]int
}

// MT19937 is the Mersenne Twister PRNG, matching C++ std::mt19937.
type MT19937 struct {
	mt  [624]uint32
	mti int
}

// NewMT19937 returns a Mersenne Twister with the given seed.
func NewMT19937(seed uint32) *MT19937 {
	mt := &MT19937{mti: 625}
	mt.Seed(seed)
	return mt
}

// Seed initialises the generator with a seed.
func (m *MT19937) Seed(seed uint32) {
	m.mt[0] = seed
	for i := 1; i < 624; i++ {
		m.mt[i] = 1812433253*(m.mt[i-1]^(m.mt[i-1]>>30)) + uint32(i)
	}
	m.mti = 624
}

// Uint32 returns the next random uint32.
func (m *MT19937) Uint32() uint32 {
	if m.mti >= 624 {
		m.generateNumbers()
	}
	y := m.mt[m.mti]
	m.mti++

	// Tempering
	y ^= y >> 11
	y ^= (y << 7) & 0x9D2C5680
	y ^= (y << 15) & 0xEFC60000
	y ^= y >> 18

	return y
}

// generateNumbers refills the state with the next 624 numbers.
func (m *MT19937) generateNumbers() {
	const (
		n         = 624
		m_        = 397
		matrixA   = 0x9908B0DF
		upperMask = 0x80000000
		lowerMask = 0x7FFFFFFF
	)

	mag01 := [2]uint32{0, matrixA}

	for i := 0; i < n-m_; i++ {
		y := (m.mt[i] & upperMask) | (m.mt[i+1] & lowerMask)
		m.mt[i] = m.mt[i+m_] ^ (y >> 1) ^ mag01[y&1]
	}
	for i := n - m_; i < n-1; i++ {
		y := (m.mt[i] & upperMask) | (m.mt[i+1] & lowerMask)
		m.mt[i] = m.mt[i+(m_-n)] ^ (y >> 1) ^ mag01[y&1]
	}
	y := (m.mt[n-1] & upperMask) | (m.mt[0] & lowerMask)
	m.mt[n-1] = m.mt[m_-1] ^ (y >> 1) ^ mag01[y&1]

	m.mti = 0
}

// NewPerlinNoise creates a new Perlin noise generator with the specified seed.
func NewPerlinNoise(seed uint32) *PerlinNoise {
	p := &PerlinNoise{}
	p.initPermutation(seed)
	return p
}

// initPermutation shuffles the permutation table with MT19937 as siv does,
// then doubles it.
func (p *PerlinNoise) initPermutation(seed uint32) {
	perm := make([]int, 256)
	for i := range perm {
		perm[i] = i
	}

	mt := NewMT19937(seed)

	for i := 1; i < 256; i++ {
		n := uint64(i)
		j := int(uint64(mt.Uint32()) % (n + 1))
		perm[i], perm[j] = perm[j], perm[i]
	}

	for i := 0; i < 256; i++ {
		p.perm[i] = perm[i]
		p.perm[256+i] = perm[i]
	}
}

// grad is the dot product of a corner's pseudo-random gradient with (x, y, z).
func grad(hash int, x, y, z float64) float64 {
	h := hash & 15
	var u, v float64
	if h < 8 {
		u = x
	} else {
		u = y
	}
	if h < 4 {
		v = y
	} else if h == 12 || h == 14 {
		v = x
	} else {
		v = z
	}
	if h&1 != 0 {
		u = -u
	}
	if h&2 != 0 {
		v = -v
	}
	return u + v
}

// Noise2D computes 2D Perlin noise at the given coordinates.
func (p *PerlinNoise) Noise2D(x, y float64) float64 {
	return p.Noise3D(x, y, 0.34567)
}

// Noise2D_01 computes 2D Perlin noise normalized to [0, 1] range.
func (p *PerlinNoise) Noise2D_01(x, y float64) float64 {
	return p.Noise2D(x, y)*0.5 + 0.5
}

// Noise3D computes 3D Perlin noise at the given coordinates. Fade, lerp and
// gradient calls are inlined.
func (p *PerlinNoise) Noise3D(x, y, z float64) float64 {
	floorX := math.Floor(x)
	floorY := math.Floor(y)
	floorZ := math.Floor(z)

	ix := int(floorX) & 255
	iy := int(floorY) & 255
	iz := int(floorZ) & 255

	fx := x - floorX
	fy := y - floorY
	fz := z - floorZ

	// Fade curves 6t⁵ - 15t⁴ + 10t³.
	u := fx * fx * fx * (fx*(fx*6-15) + 10)
	v := fy * fy * fy * (fy*(fy*6-15) + 10)
	w := fz * fz * fz * (fz*(fz*6-15) + 10)

	perm := &p.perm
	A := (perm[ix] + iy) & 255
	B := (perm[ix+1] + iy) & 255

	AA := (perm[A] + iz) & 255
	AB := (perm[A+1] + iz) & 255
	BA := (perm[B] + iz) & 255
	BB := (perm[B+1] + iz) & 255

	fx1 := fx - 1
	fy1 := fy - 1
	fz1 := fz - 1

	p0 := grad(perm[AA], fx, fy, fz)
	p1 := grad(perm[BA], fx1, fy, fz)
	p2 := grad(perm[AB], fx, fy1, fz)
	p3 := grad(perm[BB], fx1, fy1, fz)
	p4 := grad(perm[AA+1], fx, fy, fz1)
	p5 := grad(perm[BA+1], fx1, fy, fz1)
	p6 := grad(perm[AB+1], fx, fy1, fz1)
	p7 := grad(perm[BB+1], fx1, fy1, fz1)

	// Trilinear interpolation.
	q0 := p0 + u*(p1-p0)
	q1 := p2 + u*(p3-p2)
	q2 := p4 + u*(p5-p4)
	q3 := p6 + u*(p7-p6)

	r0 := q0 + v*(q1-q0)
	r1 := q2 + v*(q3-q2)

	return r0 + w*(r1-r0)
}

// Noise3D_01 computes 3D Perlin noise normalized to [0, 1] range.
func (p *PerlinNoise) Noise3D_01(x, y, z float64) float64 {
	return p.Noise3D(x, y, z)*0.5 + 0.5
}

// Global Perlin noise instances. The seeds match the C++ so the map is
// identical:
//   - NumberGrid.cpp:  siv::PerlinNoise perlinBadNumbers{ 505 }
//   - NumbersPanel.cpp: siv::PerlinNoise perlin{ 555 }
var (
	PerlinBadNumbers = NewPerlinNoise(505)
	PerlinAnimation  = NewPerlinNoise(555)
)

// Constants matching the C++ implementation.
//
// Bad-number detection (NumberGrid.cpp):
//
//	float badScale  = 0.4f;
//	float badThresh = 0.7f;
//	if (perlinBadNumbers.noise2D_01(x*badScale, y*badScale) > badThresh) { ... }
//
// Animation offset (NumbersPanel.cpp):
//
//	perlin.noise3D(x*noiseScale, y*noiseScale, t*noiseSpeed) * noiseScaleOffset
const (
	BadScale     = 0.4
	BadThreshold = 0.7
	NoiseScale   = 1.0
	NoiseSpeed   = 0.004
	NoiseOffset  = 15.0
)

// IsBadNumber reports whether the cell is corrupt on the first file.
func IsBadNumber(gridX, gridY int) bool { return IsBadNumberAt(gridX, gridY, 0) }

// fileDriftX and fileDriftY move the sample point between files, giving each
// file its own layout from the one seeded field, which stays identical to the
// C++ reference. They differ and neither is a multiple of the permutation's
// period (256), so successive files do not drift along a diagonal or wrap
// back onto a used layout.
const (
	fileDriftX = 137.0
	fileDriftY = 89.0
)

// IsBadNumberAt reports whether a cell is corrupt in a given file. Epoch 0 is
// the C++ reference board.
func IsBadNumberAt(gridX, gridY, epoch int) bool {
	x := float64(gridX)*BadScale + float64(epoch)*fileDriftX
	y := float64(gridY)*BadScale + float64(epoch)*fileDriftY
	return PerlinBadNumbers.Noise2D_01(x, y) > BadThreshold
}

// GetAnimationOffset returns a number's noise-driven drift offset.
func GetAnimationOffset(gridX, gridY int, time float64, horizontal bool) float64 {
	noise := PerlinAnimation.Noise3D(
		float64(gridX)*NoiseScale,
		float64(gridY)*NoiseScale,
		time*NoiseSpeed,
	)
	offset := noise * NoiseOffset
	if !horizontal {
		offset = -offset
	}
	return offset
}
