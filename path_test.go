// Copyright 2026 Frederik Zipp. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package canvas

import (
	"math"
	"testing"
)

// TestPathFlattening exercises the curve primitives so that flattened
// Bézier and arc geometry is both non-empty and hit-testable.
func TestPathFlattening(t *testing.T) {
	t.Run("cubic bezier", func(t *testing.T) {
		var p path
		p.moveTo(0, 0)
		p.bezierCurveTo(10, 50, 90, 50, 100, 0)
		if p.isEmpty() || p.isZeroSized() {
			t.Fatal("flattened bezier must be non-empty and non-degenerate")
		}
		// A point near the curve's bulge should be inside once closed into
		// a filled shape; primarily assert no panic and sane containment.
		p.closePath()
		if !p.contains(50, 5) {
			t.Error("point under the bezier curve should be inside")
		}
	})

	t.Run("quadratic bezier", func(t *testing.T) {
		var p path
		p.moveTo(0, 0)
		p.quadraticCurveTo(50, 100, 100, 0)
		if p.isEmpty() || p.isZeroSized() {
			t.Fatal("flattened quadratic must be non-empty")
		}
	})

	t.Run("arc to", func(t *testing.T) {
		var p path
		p.moveTo(0, 0)
		p.arcTo(50, 0, 50, 50, 20)
		if p.isEmpty() {
			t.Fatal("arcTo must produce segments")
		}
	})

	t.Run("partial arc anticlockwise", func(t *testing.T) {
		var p path
		p.arc(100, 100, 40, 0, math.Pi, true)
		if p.isEmpty() {
			t.Fatal("partial arc must produce segments")
		}
	})
}

// TestHitTesterCurveWrappers drives the hitTester path-mirroring wrappers
// (the methods Context calls) rather than the path type directly.
func TestHitTesterCurveWrappers(t *testing.T) {
	h := newTestTester()
	h.beginPath()
	h.moveTo(0, 0)
	h.bezierTo(10, 10, 90, 10, 100, 100)
	h.quadraticTo(50, 0, 100, 100)
	h.ellipse(50, 50, 40, 20, 0.5, 0, math.Pi, false)
	h.arcTo(0, 0, 10, 10, 5)
	h.lineTo(0, 0)
	h.closePath()
	if h.cursor.isEmpty() {
		t.Fatal("mirrored curve path must not be empty")
	}
	if _, err := h.add("curves", HitFill); err != nil {
		t.Fatalf("add curve region: %v", err)
	}
}

// TestTransformWrappers covers the remaining CTM mirroring methods and
// confirms they compose without error.
func TestTransformWrappers(t *testing.T) {
	h := newTestTester()
	h.scale(2, 3)
	h.transform(1, 0, 0, 1, 5, 6)
	if !h.state.ctm.finite() {
		t.Fatal("composed matrix should be finite")
	}
	// Save/restore around a clip entry exercises drawState.copy with clips.
	h.save()
	h.beginPath()
	h.rect(0, 0, 10, 10)
	h.clip()
	h.translate(1, 1)
	h.restore()
	if len(h.state.clip) != 0 {
		t.Error("clip pushed inside a saved state must pop on restore")
	}
}

// TestAddFillAndStrokeHelper covers the convenience constructor and
// verifies its wire mode byte.
func TestAddFillAndStrokeHelper(t *testing.T) {
	draws := make(chan []byte, 1)
	ctx := newContext(draws, nil, &Options{HitRegions: true})
	ctx.BeginPath()
	ctx.Rect(0, 0, 10, 10)
	if err := ctx.AddFillAndStrokeHitRegion("both"); err != nil {
		t.Fatal(err)
	}
	got := ctx.hit.regions[0].mode
	if got != HitFillAndStroke {
		t.Errorf("mode = %d, want HitFillAndStroke(%d)", got, HitFillAndStroke)
	}
}
