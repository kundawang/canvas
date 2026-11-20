// Copyright 2026 Frederik Zipp. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package canvas

import "math"

// matrix is a 2D affine transformation matrix in the convention used by the
// canvas transformation matrix:
//
//	[ a b 0 ]
//	[ c d 0 ]
//	[ e f 1 ]
//
// A column vector (x, y) is transformed as:
//
//	x' = a*x + c*y + e
//	y' = b*x + d*y + f
type matrix struct {
	a, b, c, d, e, f float64
}

func identityMatrix() matrix {
	return matrix{a: 1, d: 1}
}

// transformMatrix builds a matrix from the arguments of Context.Transform
// and Context.SetTransform.
func transformMatrix(a, b, c, d, e, f float64) matrix {
	return matrix{a: a, b: b, c: c, d: d, e: e, f: f}
}

func translationMatrix(x, y float64) matrix {
	return matrix{a: 1, d: 1, e: x, f: y}
}

func rotationMatrix(angle float64) matrix {
	sin, cos := math.Sincos(angle)
	return matrix{a: cos, b: sin, c: -sin, d: cos}
}

func scalingMatrix(x, y float64) matrix {
	return matrix{a: x, d: y}
}

// multiply returns the product m*n, i.e. the transformation n applied first,
// then m, matching ctx.setTransform(...); ctx.transform(...).
func (m matrix) multiply(n matrix) matrix {
	return matrix{
		a: m.a*n.a + m.b*n.c,
		b: m.a*n.b + m.b*n.d,
		c: m.c*n.a + m.d*n.c,
		d: m.c*n.b + m.d*n.d,
		e: m.a*n.e + m.c*n.f + m.e,
		f: m.b*n.e + m.d*n.f + m.f,
	}
}

// apply transforms the point (x, y).
func (m matrix) apply(x, y float64) (float64, float64) {
	return m.a*x + m.c*y + m.e, m.b*x + m.d*y + m.f
}

// determinant returns the determinant of the 2x2 linear part. A value of
// zero means the matrix is singular and cannot be inverted.
func (m matrix) determinant() float64 {
	return m.a*m.d - m.b*m.c
}

// invert returns the inverse matrix and ok == false if the matrix is
// singular or not finite.
func (m matrix) invert() (matrix, bool) {
	if !m.finite() {
		return matrix{}, false
	}
	det := m.determinant()
	if det == 0 || math.IsNaN(det) || math.IsInf(det, 0) {
		return matrix{}, false
	}
	invDet := 1 / det
	return matrix{
		a: m.d * invDet,
		b: -m.b * invDet,
		c: -m.c * invDet,
		d: m.a * invDet,
		e: (m.c*m.f - m.d*m.e) * invDet,
		f: (m.b*m.e - m.a*m.f) * invDet,
	}, true
}

// finite reports whether all six components are finite numbers. A matrix
// containing NaN or ±Inf values cannot be used for hit testing.
func (m matrix) finite() bool {
	return isFinite(m.a) && isFinite(m.b) && isFinite(m.c) &&
		isFinite(m.d) && isFinite(m.e) && isFinite(m.f)
}

// uniformScale returns an estimate of the scale factor of the matrix,
// defined as the square root of the absolute determinant. It is exact for
// uniform scaling and rotation, and an average for anisotropic scaling.
// It is used to convert a device-space stroke width into local-space units.
func (m matrix) uniformScale() float64 {
	det := math.Abs(m.determinant())
	if det == 0 {
		return 0
	}
	return math.Sqrt(det)
}

func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}
