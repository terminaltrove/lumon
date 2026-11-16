package physics

import "math"

// Vec2 is a 2D vector. It replaces inertia's use of nalgebra's Vector3.
type Vec2 struct {
	X, Y float64
}

// Add returns v + o.
func (v Vec2) Add(o Vec2) Vec2 { return Vec2{v.X + o.X, v.Y + o.Y} }

// Sub returns v - o.
func (v Vec2) Sub(o Vec2) Vec2 { return Vec2{v.X - o.X, v.Y - o.Y} }

// Scale returns v multiplied by scalar s.
func (v Vec2) Scale(s float64) Vec2 { return Vec2{v.X * s, v.Y * s} }

// LenSq returns the squared magnitude, for comparing distances without a
// square root.
func (v Vec2) LenSq() float64 { return v.X*v.X + v.Y*v.Y }

// Len returns the magnitude.
func (v Vec2) Len() float64 { return math.Sqrt(v.LenSq()) }

// Dot returns the dot product of v and o.
func (v Vec2) Dot(o Vec2) float64 { return v.X*o.X + v.Y*o.Y }

// Normalize returns a unit vector in the direction of v, or the zero vector if
// v has no length. The zero case avoids NaN when two particles coincide.
func (v Vec2) Normalize() Vec2 {
	l := v.Len()
	if l == 0 {
		return Vec2{}
	}
	return v.Scale(1 / l)
}
