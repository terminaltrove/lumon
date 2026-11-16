// Package noise is the Perlin field that decides which numbers are corrupt.
//
// It is a deterministic port of siv::PerlinNoise as LumonMDR's NumberGrid.cpp
// uses it, MT19937 included, seeded 505 for placement and 555 for the
// animation field. Two implementations of the same seed must agree byte for
// byte or they produce different boards, so the golden values in
// perlin_test.go are the specification. Do not change them.
//
// The constants BadScale 0.4, BadThreshold 0.7, NoiseSpeed 0.004 and
// NoiseOffset 15.0 come from the C++.
package noise
