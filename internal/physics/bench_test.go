package physics

import "testing"

// A lumon-scale cluster: a 5x5 block of digits bound as one wobbling body.
func benchCluster(n int) *World {
	w := NewWorld()
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			w.Add(Vec2{float64(x), float64(y)}, 1)
		}
	}
	at := func(x, y int) int { return y*n + x }
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if x+1 < n {
				w.AddConstraint(NewDistance(at(x, y), at(x+1, y), 1, 1))
			}
			if y+1 < n {
				w.AddConstraint(NewDistance(at(x, y), at(x, y+1), 1, 1))
			}
		}
	}
	return w
}

func BenchmarkStepCluster5x5(b *testing.B) {
	w := benchCluster(5)
	g := Vec2{0, -9.81}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Step(DT, g)
	}
}

func BenchmarkStepCluster20x20(b *testing.B) {
	w := benchCluster(20)
	g := Vec2{0, -9.81}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Step(DT, g)
	}
}

func BenchmarkIntegrate1000Particles(b *testing.B) {
	w := NewWorld()
	for i := 0; i < 1000; i++ {
		w.Add(Vec2{float64(i % 50), float64(i / 50)}, 1)
	}
	g := Vec2{0, -9.81}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Integrate(DT, g)
	}
}

// The idle starfield: O(n^2), so this is the one to watch.
func BenchmarkBruteForce200Bodies(b *testing.B) {
	bodies := make([]Body, 200)
	for i := range bodies {
		bodies[i] = Body{Pos: Vec2{float64(i % 20), float64(i / 20)}, Mass: 1}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BruteForce(bodies, 1, 0.25)
	}
}
