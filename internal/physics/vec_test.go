package physics

import (
	"math"
	"testing"
)

const eps = 1e-9

func closeTo(t *testing.T, got, want float64, what string) {
	t.Helper()
	if math.Abs(got-want) > eps {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func closeToVec(t *testing.T, got, want Vec2, what string) {
	t.Helper()
	if math.Abs(got.X-want.X) > eps || math.Abs(got.Y-want.Y) > eps {
		t.Errorf("%s = %+v, want %+v", what, got, want)
	}
}

func TestVec2AddSubtractAndScale(t *testing.T) {
	a := Vec2{3, 4}
	b := Vec2{1, 2}

	closeToVec(t, a.Add(b), Vec2{4, 6}, "Add")
	closeToVec(t, a.Sub(b), Vec2{2, 2}, "Sub")
	closeToVec(t, a.Scale(2), Vec2{6, 8}, "Scale")
}

func TestVec2LenAndLenSq(t *testing.T) {
	v := Vec2{3, 4}

	closeTo(t, v.LenSq(), 25, "LenSq")
	closeTo(t, v.Len(), 5, "Len")
}

func TestVec2NormalizeProducesUnitLength(t *testing.T) {
	v := Vec2{3, 4}.Normalize()

	closeTo(t, v.Len(), 1, "Len after Normalize")
	closeToVec(t, v, Vec2{0.6, 0.8}, "Normalize")
}

// Returning zero keeps the solver from producing NaN when two particles land
// exactly on top of each other.
func TestVec2NormalizeZeroVectorReturnsZero(t *testing.T) {
	closeToVec(t, Vec2{}.Normalize(), Vec2{}, "Normalize of zero")
}

func TestVec2Dot(t *testing.T) {
	closeTo(t, Vec2{3, 4}.Dot(Vec2{2, 1}), 10, "Dot")
	closeTo(t, Vec2{1, 0}.Dot(Vec2{0, 1}), 0, "Dot of perpendicular")
}

func closeToVecTol(t *testing.T, got, want Vec2, tol float64, what string) {
	t.Helper()
	if math.Abs(got.X-want.X) > tol || math.Abs(got.Y-want.Y) > tol {
		t.Errorf("%s = %+v, want %+v (tol %v)", what, got, want, tol)
	}
}
