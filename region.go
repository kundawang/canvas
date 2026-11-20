// Copyright 2026 Frederik Zipp. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package canvas

import (
	"errors"
)

// ErrHitRegionsDisabled is returned by the hit-region API methods
// (AddHitRegion, RemoveHitRegion, ClearHitRegions) when hit regions were
// not enabled via Options.HitRegions when the canvas was created.
var ErrHitRegionsDisabled = errors.New("canvas: hit regions are disabled; set Options.HitRegions to enable them")

// ErrTooManyHitRegions is returned by AddHitRegion when the region limit
// (MaxHitRegions) has been reached.
var ErrTooManyHitRegions = errors.New("canvas: too many hit regions")

// ErrNonFiniteTransform is returned by AddHitRegion when the current
// transformation matrix contains NaN or ±Inf components, or is singular
// and cannot be inverted. A point cannot be mapped into the local
// coordinate space of such a region.
var ErrNonFiniteTransform = errors.New("canvas: non-finite or singular transformation matrix; cannot create hit region")

// ErrEmptyHitRegion is returned by AddHitRegion when the current path
// contains no drawable segments, or collapses to a zero-sized point.
// Such a region could never be hit.
var ErrEmptyHitRegion = errors.New("canvas: hit region path is empty or zero-sized")

// defaultMaxHitRegions is the default upper bound on the number of live
// hit regions. It bounds the per-event hit-test cost and the size of the
// region index sent to the client.
const defaultMaxHitRegions = 1000

// HitMode selects which painted parts of a named region participate in
// hit testing.
type HitMode byte

const (
	// HitFill tests only the interior of the region's path using the
	// non-zero winding rule.
	HitFill HitMode = iota
	// HitStroke tests only the stroked outline of the region's path.
	HitStroke
	// HitFillAndStroke tests both the interior and the stroked outline.
	HitFillAndStroke
)

// A hitRegion is a named, drawable area on the canvas. Regions are kept in
// registration order, which is also their stacking (painting) order; the
// last entry paints on top.
type hitRegion struct {
	id         uint32
	name       string
	mode       HitMode
	localPath  path
	ctm        matrix
	clip       []clipPath
	lineWidth  float64
	zeroStroke bool
}

// A clipPath is one entry of the clipping stack captured at region
// registration time. When a region is created every clip entry is
// re-expressed in the region's local coordinate system. A point hits the
// region only if it lies inside all clip paths.
type clipPath struct {
	path path
}

// A stateClip is a clip entry held in the mirrored drawing-state stack.
// The path is stored in the local coordinate system active when Clip was
// called, together with the transformation matrix (ctm) in effect then, so
// it can later be mapped into a region's potentially different space.
type stateClip struct {
	path path
	ctm  matrix
}

// HitInfo describes a successful hit test: the region hit and the hit
// point expressed in the region's local coordinate system, i.e. the
// coordinate system in effect when the region was registered.
type HitInfo struct {
	// Region is the name of the topmost region containing the hit point.
	Region string
	// X and Y are the hit point in the region's local coordinates.
	X, Y float64
}

// hitTester is the server-side hit-testing engine. It mirrors the parts of
// the canvas drawing state needed to register and test regions: the
// current path, the transformation and clipping stacks, and the stroke
// style. It is only allocated when Options.HitRegions is enabled, so a
// disabled context keeps its original behaviour and wire format.
type hitTester struct {
	maxRegions int
	regionIDs  idGenerator
	regions    []*hitRegion

	state  drawState
	stack  []drawState
	cursor path
}

// drawState is the subset of the canvas drawing state that the hit tester
// tracks, mirroring what Save and Restore push and pop on the client.
type drawState struct {
	ctm       matrix
	clip      []stateClip
	lineWidth float64
}

func newHitTester(maxRegions int) *hitTester {
	if maxRegions <= 0 {
		maxRegions = defaultMaxHitRegions
	}
	return &hitTester{
		maxRegions: maxRegions,
		state:      drawState{ctm: identityMatrix(), lineWidth: 1},
	}
}

// copy returns an independent copy of the drawing state. The clip slice is
// deep-copied so that later Clip calls do not mutate saved states.
func (s drawState) copy() drawState {
	clips := make([]stateClip, len(s.clip))
	for i, c := range s.clip {
		clips[i] = stateClip{path: c.path.clone(), ctm: c.ctm}
	}
	return drawState{ctm: s.ctm, clip: clips, lineWidth: s.lineWidth}
}

// --- drawing state mirroring ---

func (h *hitTester) save() {
	h.stack = append(h.stack, h.state.copy())
}

func (h *hitTester) restore() {
	if len(h.stack) == 0 {
		return
	}
	h.state = h.stack[len(h.stack)-1]
	h.stack = h.stack[:len(h.stack)-1]
}

func (h *hitTester) translate(x, y float64) {
	h.state.ctm = h.state.ctm.multiply(translationMatrix(x, y))
}

func (h *hitTester) rotate(angle float64) {
	h.state.ctm = h.state.ctm.multiply(rotationMatrix(angle))
}

func (h *hitTester) scale(x, y float64) {
	h.state.ctm = h.state.ctm.multiply(scalingMatrix(x, y))
}

func (h *hitTester) transform(a, b, c, d, e, f float64) {
	h.state.ctm = h.state.ctm.multiply(transformMatrix(a, b, c, d, e, f))
}

func (h *hitTester) setTransform(a, b, c, d, e, f float64) {
	h.state.ctm = transformMatrix(a, b, c, d, e, f)
}

func (h *hitTester) setLineWidth(width float64) {
	h.state.lineWidth = width
}

func (h *hitTester) beginPath() {
	h.cursor.begin()
}

func (h *hitTester) clip() {
	if h.cursor.isEmpty() {
		return
	}
	h.state.clip = append(h.state.clip, stateClip{
		path: h.cursor.clone(),
		ctm:  h.state.ctm,
	})
}

// --- path mirroring ---

func (h *hitTester) moveTo(x, y float64)               { h.cursor.moveTo(x, y) }
func (h *hitTester) lineTo(x, y float64)               { h.cursor.lineTo(x, y) }
func (h *hitTester) closePath()                        { h.cursor.closePath() }
func (h *hitTester) rect(x, y, w, hh float64)          { h.cursor.rect(x, y, w, hh) }
func (h *hitTester) bezierTo(a, b, c, d, e, f float64) { h.cursor.bezierCurveTo(a, b, c, d, e, f) }
func (h *hitTester) quadraticTo(a, b, c, d float64)    { h.cursor.quadraticCurveTo(a, b, c, d) }
func (h *hitTester) ellipse(x, y, rx, ry, rot, sa, ea float64, acw bool) {
	h.cursor.ellipse(x, y, rx, ry, rot, sa, ea, acw)
}
func (h *hitTester) arc(x, y, r, sa, ea float64, acw bool) {
	h.cursor.arc(x, y, r, sa, ea, acw)
}
func (h *hitTester) arcTo(x1, y1, x2, y2, r float64) { h.cursor.arcTo(x1, y1, x2, y2, r) }

// --- region management ---

// add registers the current path as a named hit region and returns its
// client-side ID. The region captures the current transformation matrix,
// clipping stack, and stroke width.
func (h *hitTester) add(name string, mode HitMode) (uint32, error) {
	if len(h.regions) >= h.maxRegions {
		return 0, ErrTooManyHitRegions
	}
	if !h.state.ctm.finite() {
		return 0, ErrNonFiniteTransform
	}
	if _, ok := h.state.ctm.invert(); !ok {
		return 0, ErrNonFiniteTransform
	}
	if h.cursor.isEmpty() || h.cursor.isZeroSized() {
		return 0, ErrEmptyHitRegion
	}
	// Re-adding an existing name moves that region to the top, mirroring
	// the canvas addHitRegion semantics of replacing an existing region.
	if i := h.indexOfName(name); i >= 0 {
		old := h.regions[i]
		h.regions = append(h.regions[:i], h.regions[i+1:]...)
		oldID := old.id
		region := h.makeRegion(oldID, name, mode)
		h.regions = append(h.regions, region)
		return oldID, nil
	}
	id := h.regionIDs.generateID()
	h.regions = append(h.regions, h.makeRegion(id, name, mode))
	return id, nil
}

func (h *hitTester) makeRegion(id uint32, name string, mode HitMode) *hitRegion {
	strokeWidth := h.state.lineWidth
	zeroStroke := !isFinite(strokeWidth) || strokeWidth <= 0
	region := &hitRegion{
		id:         id,
		name:       name,
		mode:       mode,
		localPath:  h.cursor.clone(),
		ctm:        h.state.ctm,
		lineWidth:  strokeWidth,
		zeroStroke: zeroStroke,
	}
	// Re-express every clipping entry in the region's local coordinate
	// system. A clip entry captured under matrix C stores its path in
	// C-space. If the region is registered under matrix R, applying
	// R⁻¹·C to the clip points converts them into the region's R-space,
	// where the region's own path and the hit point already live.
	if inv, ok := h.state.ctm.invert(); ok {
		region.clip = make([]clipPath, len(h.state.clip))
		for i, c := range h.state.clip {
			region.clip[i] = clipPath{path: c.path.transform(inv.multiply(c.ctm))}
		}
	}
	return region
}

func (h *hitTester) indexOfName(name string) int {
	for i, r := range h.regions {
		if r.name == name {
			return i
		}
	}
	return -1
}

// remove deletes the named region. Removing a region from the middle shifts
// the stacking order of the regions above it, so the index stays correct.
func (h *hitTester) remove(name string) bool {
	i := h.indexOfName(name)
	if i < 0 {
		return false
	}
	h.regions = append(h.regions[:i], h.regions[i+1:]...)
	return true
}

func (h *hitTester) clear() {
	h.regions = nil
}

// RegionCount returns the number of currently registered hit regions.
func (h *hitTester) RegionCount() int {
	return len(h.regions)
}

// clone returns an independent copy of the path.
func (p path) clone() path {
	subs := make([]subPath, len(p.subs))
	for i, sub := range p.subs {
		subs[i] = subPath{
			startX:   sub.startX,
			startY:   sub.startY,
			closed:   sub.closed,
			segments: append([]segment(nil), sub.segments...),
		}
	}
	return path{subs: subs}
}

// transform returns a copy of the path with every point mapped through the
// given matrix. It is used to re-express clipping paths in the local
// coordinate system of a hit region.
func (p path) transform(m matrix) path {
	clone := p.clone()
	for i := range clone.subs {
		sub := &clone.subs[i]
		sub.startX, sub.startY = m.apply(sub.startX, sub.startY)
		for j := range sub.segments {
			seg := &sub.segments[j]
			seg.x1, seg.y1 = m.apply(seg.x1, seg.y1)
			seg.x2, seg.y2 = m.apply(seg.x2, seg.y2)
		}
	}
	return clone
}

// --- hit testing ---

// hitAt resolves the topmost region containing the canvas-space point
// (canvasX, canvasY). It returns nil when no region is hit.
func (h *hitTester) hitAt(canvasX, canvasY float64) *HitInfo {
	for i := len(h.regions) - 1; i >= 0; i-- {
		r := h.regions[i]
		inv, ok := r.ctm.invert()
		if !ok {
			continue
		}
		lx, ly := inv.apply(canvasX, canvasY)
		if !h.passesClip(r, lx, ly) {
			continue
		}
		if !h.pathHit(r, lx, ly) {
			continue
		}
		return &HitInfo{Region: r.name, X: lx, Y: ly}
	}
	return nil
}

// passesClip reports whether the local point (lx, ly) lies inside every
// clip path captured for the region.
func (h *hitTester) passesClip(r *hitRegion, lx, ly float64) bool {
	for _, c := range r.clip {
		if !c.path.contains(lx, ly) {
			return false
		}
	}
	return true
}

// pathHit applies the region's hit mode to its local path.
func (h *hitTester) pathHit(r *hitRegion, lx, ly float64) bool {
	switch r.mode {
	case HitFill:
		return r.localPath.contains(lx, ly)
	case HitStroke:
		return h.strokeHit(r, lx, ly)
	default: // HitFillAndStroke
		return r.localPath.contains(lx, ly) || h.strokeHit(r, lx, ly)
	}
}

// strokeHit tests whether the local point lies within half the stroke
// width of the path's centreline. Hit testing runs in the region's local
// coordinate system, and lineWidth is itself expressed in local units; the
// CTM was already inverted to map the point into that space. The stroke
// band in local space is therefore exactly lineWidth/2 under rotation and
// uniform scaling, and a close approximation under anisotropic scaling.
func (h *hitTester) strokeHit(r *hitRegion, lx, ly float64) bool {
	if r.zeroStroke {
		return false
	}
	halfWidth := r.lineWidth / 2
	return r.localPath.strokeDistance(lx, ly) <= halfWidth
}

// hitAtPoint is a small guard against non-finite event coordinates.
func finitePoint(x, y float64) bool {
	return isFinite(x) && isFinite(y)
}
