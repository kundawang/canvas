// Copyright 2026 Frederik Zipp. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package canvas

import (
	"math"
	"testing"
)

func approxEqual(got, want, tol float64) bool {
	return math.Abs(got-want) <= tol
}

func hitEqual(got *HitInfo, wantName string, wantX, wantY float64) bool {
	if got == nil {
		return false
	}
	if got.Region != wantName {
		return false
	}
	const tol = 1e-9
	return approxEqual(got.X, wantX, tol) && approxEqual(got.Y, wantY, tol)
}

func newTestTester() *hitTester {
	return newHitTester(0)
}

// rectPath builds a unit-style rectangle path in local coordinates.
func rectPath(x, y, w, h float64) path {
	var p path
	p.rect(x, y, w, h)
	return p
}

func TestHitFillBasic(t *testing.T) {
	h := newTestTester()
	h.cursor = rectPath(0, 0, 10, 10)
	if _, err := h.add("r", HitFill); err != nil {
		t.Fatalf("add: %v", err)
	}
	if info := h.hitAt(5, 5); !hitEqual(info, "r", 5, 5) {
		t.Errorf("inside: got %v, want region r at (5,5)", info)
	}
	if info := h.hitAt(11, 11); info != nil {
		t.Errorf("outside: got %v, want nil", info)
	}
}

// TestRotatedOverlappingRectangles is the core geometry fixture: two
// identical rectangles rotated 90 degrees relative to each other around a
// shared centre, overlapping in the middle, with a defined stacking order.
func TestRotatedOverlappingRectangles(t *testing.T) {
	h := newTestTester()

	// Region "a": local rect [0,0]-[40,10], translated to (30,45):
	// canvas centre (50,50), no rotation.
	h.setTransform(1, 0, 0, 1, 30, 45)
	h.cursor = rectPath(0, 0, 40, 10)
	if _, err := h.add("a", HitFill); err != nil {
		t.Fatalf("add a: %v", err)
	}

	// Region "b": same local rect, then rotated 90 degrees about the
	// shared centre (50,50). The local rect centre is (20,5), so moving
	// the origin by (-20,-5) puts that centre on the rotation centre.
	h.setTransform(1, 0, 0, 1, 50, 50)
	h.rotate(math.Pi / 2)
	h.translate(-20, -5)
	h.cursor = rectPath(0, 0, 40, 10)
	if _, err := h.add("b", HitFill); err != nil {
		t.Fatalf("add b: %v", err)
	}

	// (50,50) lies inside both. "b" was registered last, so it is on top.
	info := h.hitAt(50, 50)
	if info == nil || info.Region != "b" {
		t.Fatalf("overlap centre: got %v, want top region b", info)
	}
	// Local coords of (50,50) inside b: CTM rotates 90 then shifts;
	// the shared centre maps back to the local rect centre (20,5).
	if !approxEqual(info.X, 20, 1e-9) || !approxEqual(info.Y, 5, 1e-9) {
		t.Errorf("b local coords: got (%v,%v), want (20,5)", info.X, info.Y)
	}

	// A point only inside "a" (the horizontal bar), away from the overlap.
	// (35,47) is inside a's canvas rect x[30,70] y[45,55].
	info = h.hitAt(35, 47)
	if info == nil || info.Region != "a" {
		t.Fatalf("a-only point: got %v, want a", info)
	}
	if !approxEqual(info.X, 5, 1e-9) || !approxEqual(info.Y, 2, 1e-9) {
		t.Errorf("a local coords: got (%v,%v), want (5,2)", info.X, info.Y)
	}

	// A point only inside "b" (the vertical bar): b's canvas footprint is
	// x[45,55] y[30,70]. (53,65) is inside b but outside a.
	info = h.hitAt(53, 65)
	if info == nil || info.Region != "b" {
		t.Fatalf("b-only point: got %v, want b", info)
	}

	// A point inside neither.
	if info := h.hitAt(10, 10); info != nil {
		t.Errorf("empty area: got %v, want nil", info)
	}
}

func TestLayerOrderAndReAdd(t *testing.T) {
	h := newTestTester()
	h.cursor = rectPath(0, 0, 100, 100)
	if _, err := h.add("bottom", HitFill); err != nil {
		t.Fatal(err)
	}
	if _, err := h.add("top", HitFill); err != nil {
		t.Fatal(err)
	}
	if info := h.hitAt(50, 50); info == nil || info.Region != "top" {
		t.Fatalf("top wins: got %v", info)
	}

	// Re-adding "bottom" replaces it and moves it to the top.
	if _, err := h.add("bottom", HitFill); err != nil {
		t.Fatal(err)
	}
	if h.RegionCount() != 2 {
		t.Fatalf("re-add must not duplicate name, count=%d", h.RegionCount())
	}
	if info := h.hitAt(50, 50); info == nil || info.Region != "bottom" {
		t.Fatalf("re-added region on top: got %v", info)
	}
}

func TestRemoveAndClearUpdateIndex(t *testing.T) {
	h := newTestTester()
	h.cursor = rectPath(0, 0, 10, 10)
	_, _ = h.add("a", HitFill)
	_, _ = h.add("b", HitFill)
	_, _ = h.add("c", HitFill)

	if !h.remove("b") {
		t.Fatal("remove b should report true")
	}
	if h.remove("b") {
		t.Fatal("removing b twice should report false")
	}
	if h.RegionCount() != 2 {
		t.Fatalf("count after remove: %d", h.RegionCount())
	}
	// "c" is still the top region; index updated correctly.
	if info := h.hitAt(5, 5); info == nil || info.Region != "c" {
		t.Errorf("after remove, top = %v, want c", info)
	}

	h.clear()
	if h.RegionCount() != 0 {
		t.Fatalf("clear count = %d", h.RegionCount())
	}
	if info := h.hitAt(5, 5); info != nil {
		t.Errorf("after clear: got %v, want nil", info)
	}
}

func TestStrokeHit(t *testing.T) {
	h := newTestTester()
	h.setLineWidth(4)
	h.cursor = rectPath(0, 0, 100, 100)
	if _, err := h.add("frame", HitStroke); err != nil {
		t.Fatal(err)
	}

	// On the top edge centreline.
	if info := h.hitAt(50, 0); info == nil || info.Region != "frame" {
		t.Errorf("edge centreline: got %v, want frame", info)
	}
	// Two units away (half line width = 2) is still inside the band.
	if info := h.hitAt(50, 2); info == nil || info.Region != "frame" {
		t.Errorf("inside stroke band: got %v, want frame", info)
	}
	// Well inside the rectangle interior: stroke mode must not hit.
	if info := h.hitAt(50, 50); info != nil {
		t.Errorf("interior of stroke-only region: got %v, want nil", info)
	}
	// Just outside the band.
	if info := h.hitAt(50, 3); info != nil {
		t.Errorf("outside stroke band: got %v, want nil", info)
	}
}

func TestFillAndStrokeHit(t *testing.T) {
	h := newTestTester()
	h.setLineWidth(10)
	h.cursor = rectPath(0, 0, 50, 50)
	if _, err := h.add("both", HitFillAndStroke); err != nil {
		t.Fatal(err)
	}
	// Interior hits via fill.
	if info := h.hitAt(25, 25); info == nil || info.Region != "both" {
		t.Errorf("interior: got %v", info)
	}
	// Outside the fill but within the stroke band hits via stroke.
	if info := h.hitAt(25, 54); info == nil || info.Region != "both" {
		t.Errorf("stroke band outside fill: got %v", info)
	}
}

// TestStrokeHitUnderScale checks the stroke band in local space. The line
// width is a local-space quantity; after the point is inverse-mapped into
// local space the band must be lineWidth/2 regardless of the matrix scale.
func TestStrokeHitUnderScale(t *testing.T) {
	h := newTestTester()
	// Scale the local space by 2 and shift the origin. A local stroke
	// width of 4 stays 4 local units (a 2-unit half band in local space).
	h.setTransform(2, 0, 0, 2, 100, 100)
	h.setLineWidth(4)
	h.cursor = rectPath(0, 0, 50, 50)
	if _, err := h.add("scaled-frame", HitStroke); err != nil {
		t.Fatal(err)
	}
	// Canvas (200,101) -> local (50,0.5): within the 2-unit local band of
	// the top edge (y=0).
	if info := h.hitAt(200, 101); info == nil || info.Region != "scaled-frame" {
		t.Errorf("scaled stroke band: got %v, want scaled-frame", info)
	}
	// Canvas (200,105) -> local (50,2.5): outside the top-edge band, but
	// it lies within 2 units of the right edge (x=50), so it still hits.
	if info := h.hitAt(200, 105); info == nil || info.Region != "scaled-frame" {
		t.Errorf("right-edge stroke band: got %v, want scaled-frame", info)
	}
	// Canvas (150,150) -> local (25,25): deep in the interior, far from
	// every edge, so a stroke-only region must not hit.
	if info := h.hitAt(150, 150); info != nil {
		t.Errorf("interior of stroke-only region: got %v, want nil", info)
	}
}

func TestSelfIntersectingPath(t *testing.T) {
	// A bowtie drawn as one self-intersecting sub-path through the
	// midpoint (50,50). Under the non-zero winding rule the two lobes are
	// filled while the central overlap, crossed in opposite directions,
	// has a winding of zero and stays empty. The point-in-path test must
	// reproduce this rather than treat the whole bounding box as filled.
	var p path
	p.moveTo(0, 0)
	p.lineTo(100, 100)
	p.lineTo(100, 0)
	p.lineTo(0, 100)
	p.closePath()
	h := newTestTester()
	h.cursor = p
	if _, err := h.add("bowtie", HitFill); err != nil {
		t.Fatal(err)
	}
	// (80,30) lies inside one triangular lobe only: winding is non-zero.
	if info := h.hitAt(80, 30); info == nil || info.Region != "bowtie" {
		t.Errorf("filled lobe (80,30): got %v, want bowtie", info)
	}
	// A point inside the central overlap, away from every edge: the two
	// crossing triangles contribute opposite windings, so the non-zero
	// rule leaves it unfilled.
	if info := h.hitAt(40, 40); info != nil {
		t.Errorf("winding-cancelled overlap (40,40): got %v, want nil", info)
	}
	// (50,90) sits below both lobes, outside the shape.
	if info := h.hitAt(50, 90); info != nil {
		t.Errorf("outside shape (50,90): got %v, want nil", info)
	}
}

func TestZeroSizedAndEmptyPath(t *testing.T) {
	h := newTestTester()

	// Empty path (no segments).
	h.beginPath()
	if _, err := h.add("empty", HitFill); err != ErrEmptyHitRegion {
		t.Errorf("empty path: got %v, want ErrEmptyHitRegion", err)
	}

	// Zero-sized path: all segments collapse to one point.
	var p path
	p.moveTo(7, 7)
	p.lineTo(7, 7)
	h.cursor = p
	if _, err := h.add("point", HitFill); err != ErrEmptyHitRegion {
		t.Errorf("zero-sized path: got %v, want ErrEmptyHitRegion", err)
	}
	if h.RegionCount() != 0 {
		t.Errorf("failed registrations must not add regions, count=%d", h.RegionCount())
	}
}

func TestNonFiniteAndSingularMatrix(t *testing.T) {
	h := newTestTester()
	h.cursor = rectPath(0, 0, 10, 10)

	h.setTransform(math.NaN(), 0, 0, 1, 0, 0)
	if _, err := h.add("nan", HitFill); err != ErrNonFiniteTransform {
		t.Errorf("NaN matrix: got %v, want ErrNonFiniteTransform", err)
	}

	// Singular (zero scale) matrix cannot be inverted.
	h.setTransform(0, 0, 0, 0, 5, 5)
	if _, err := h.add("singular", HitFill); err != ErrNonFiniteTransform {
		t.Errorf("singular matrix: got %v, want ErrNonFiniteTransform", err)
	}
}

func TestTooManyRegions(t *testing.T) {
	h := newHitTester(3)
	for i := 0; i < 3; i++ {
		h.cursor = rectPath(0, 0, 10, 10)
		name := string(rune('a' + i))
		if _, err := h.add(name, HitFill); err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
	}
	h.cursor = rectPath(0, 0, 10, 10)
	if _, err := h.add("overflow", HitFill); err != ErrTooManyHitRegions {
		t.Errorf("overflow: got %v, want ErrTooManyHitRegions", err)
	}
}

// TestClippedCurve uses a quadratic Bézier clip fixture: a large region is
// registered inside a curved clip, and only the clipped portion can be hit.
func TestClippedCurve(t *testing.T) {
	h := newTestTester()

	// Build a circular-ish clip from four quadratic curves covering the
	// box around (0,0) with control points at distance 100.
	h.beginPath()
	h.moveTo(0, -100)
	h.quadraticTo(100, -100, 100, 0)
	h.quadraticTo(100, 100, 0, 100)
	h.quadraticTo(-100, 100, -100, 0)
	h.quadraticTo(-100, -100, 0, -100)
	h.closePath()
	h.clip()

	// Register a region covering a wide area; the clip should restrict it.
	h.cursor = rectPath(-200, -200, 400, 400)
	if _, err := h.add("clipped", HitFill); err != nil {
		t.Fatalf("add: %v", err)
	}

	// The origin is well inside the curved clip.
	if info := h.hitAt(0, 0); info == nil || info.Region != "clipped" {
		t.Errorf("inside clip: got %v, want clipped", info)
	}
	// A corner of the wide region is outside the rounded clip.
	if info := h.hitAt(150, 150); info != nil {
		t.Errorf("outside curved clip: got %v, want nil", info)
	}
}

// TestClipAcrossDifferentTransforms verifies clip-space conversion when the
// clip and the region use different transformation matrices.
func TestClipAcrossDifferentTransforms(t *testing.T) {
	h := newTestTester()

	// Clip under a translated matrix: clip rect in that space covers
	// canvas x[10,30] y[0,20].
	h.setTransform(1, 0, 0, 1, 10, 0)
	h.beginPath()
	h.cursor = rectPath(0, 0, 20, 20)
	h.clip()

	// Register the region under a different matrix (shift by 100). The
	// local rect is wide enough to cover the clip's canvas footprint.
	h.setTransform(1, 0, 0, 1, 100, 0)
	h.cursor = rectPath(-200, -200, 400, 400)
	if _, err := h.add("r", HitFill); err != nil {
		t.Fatal(err)
	}

	// Canvas point (15,10): inside the clip and the region.
	if info := h.hitAt(15, 10); info == nil || info.Region != "r" {
		t.Errorf("inside transformed clip: got %v, want r", info)
	}
	// Canvas point (50,10): inside region, outside the clip (clip ends at
	// canvas x=30).
	if info := h.hitAt(50, 10); info != nil {
		t.Errorf("outside transformed clip: got %v, want nil", info)
	}
}

func TestSaveRestoreState(t *testing.T) {
	h := newTestTester()
	h.setTransform(1, 0, 0, 1, 100, 100)
	h.setLineWidth(8)
	h.save()
	h.setTransform(2, 0, 0, 2, 0, 0)
	h.setLineWidth(20)
	h.restore()

	// Matrix and line width restored.
	if h.state.ctm.e != 100 || h.state.ctm.f != 100 {
		t.Errorf("ctm not restored: %+v", h.state.ctm)
	}
	if h.state.lineWidth != 8 {
		t.Errorf("lineWidth not restored: got %v", h.state.lineWidth)
	}

	// Restore with empty stack is a no-op (mirrors canvas behaviour).
	h.restore()
}

func TestLocalCoordinatesUnderScale(t *testing.T) {
	h := newTestTester()
	h.setTransform(2, 0, 0, 2, 0, 0)
	h.cursor = rectPath(0, 0, 50, 50) // canvas footprint 100x100
	if _, err := h.add("scaled", HitFill); err != nil {
		t.Fatal(err)
	}
	info := h.hitAt(40, 60)
	if info == nil || info.Region != "scaled" {
		t.Fatalf("hit: got %v", info)
	}
	// Canvas (40,60) maps to local (20,30).
	if !approxEqual(info.X, 20, 1e-9) || !approxEqual(info.Y, 30, 1e-9) {
		t.Errorf("local coords: got (%v,%v), want (20,30)", info.X, info.Y)
	}
}

func TestNonFiniteHitPoint(t *testing.T) {
	h := newTestTester()
	h.cursor = rectPath(0, 0, 10, 10)
	_, _ = h.add("r", HitFill)
	if info := h.hitAt(math.NaN(), 5); info != nil {
		t.Errorf("NaN point: got %v, want nil", info)
	}
	if info := h.hitAt(math.Inf(1), 5); info != nil {
		t.Errorf("+Inf point: got %v, want nil", info)
	}
}
