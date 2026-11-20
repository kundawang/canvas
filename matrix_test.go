// Copyright 2026 Frederik Zipp. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package canvas

import (
	"math"
	"testing"
)

func TestMatrixMultiply(t *testing.T) {
	// Translate(10,20) then Scale(2,3): the combined matrix maps the
	// local point (1,1) to canvas (2*1+10, 3*1+20) = (12,23).
	m := translationMatrix(10, 20).multiply(scalingMatrix(2, 3))
	x, y := m.apply(1, 1)
	if !approxEqual(x, 12, 1e-12) || !approxEqual(y, 23, 1e-12) {
		t.Errorf("combined transform: got (%g,%g), want (12,23)", x, y)
	}
}

func TestMatrixInverseRoundTrip(t *testing.T) {
	m := translationMatrix(7, -3).
		multiply(rotationMatrix(0.7)).
		multiply(scalingMatrix(2.5, 1.5))
	inv, ok := m.invert()
	if !ok {
		t.Fatal("expected invertible matrix")
	}
	x, y := 4.2, -9.1
	fx, fy := m.apply(x, y)
	rx, ry := inv.apply(fx, fy)
	if !approxEqual(rx, x, 1e-10) || !approxEqual(ry, y, 1e-10) {
		t.Errorf("inverse round trip: got (%g,%g), want (%g,%g)", rx, ry, x, y)
	}
}

func TestMatrixInverseOfIdentity(t *testing.T) {
	inv, ok := identityMatrix().invert()
	if !ok {
		t.Fatal("identity must be invertible")
	}
	if inv != identityMatrix() {
		t.Errorf("inverse of identity = %+v, want identity", inv)
	}
}

func TestMatrixSingular(t *testing.T) {
	singular := scalingMatrix(0, 5)
	if _, ok := singular.invert(); ok {
		t.Error("singular matrix must not be invertible")
	}
	nan := matrix{a: math.NaN(), d: 1}
	if _, ok := nan.invert(); ok {
		t.Error("NaN matrix must not be invertible")
	}
	if nan.finite() {
		t.Error("matrix with NaN component must report non-finite")
	}
}

func TestMatrixUniformScale(t *testing.T) {
	// A rotation preserves scale: uniformScale of a pure rotation is 1.
	if s := rotationMatrix(1.3).uniformScale(); !approxEqual(s, 1, 1e-12) {
		t.Errorf("rotation scale: got %g, want 1", s)
	}
	// Scale (3,3) has uniform scale 3.
	if s := scalingMatrix(3, 3).uniformScale(); !approxEqual(s, 3, 1e-12) {
		t.Errorf("uniform scale: got %g, want 3", s)
	}
}
