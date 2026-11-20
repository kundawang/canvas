// Copyright 2026 Frederik Zipp. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package canvas

import "math"

// A path is a server-side mirror of the client-side canvas current path.
// It consists of one or more sub-paths. Curves are flattened into line
// segments so that the same model can be used for fill and stroke hit
// testing, including for self-intersecting paths.
type path struct {
	subs []subPath
}

type subPath struct {
	startX, startY float64
	segments       []segment
	closed         bool
}

// segment is one flattened piece of a sub-path. The straight segment runs
// from (x1, y1) to (x2, y2). For line segments these are the original
// endpoints; for arcs and Bézier curves they are flattening samples.
type segment struct {
	x1, y1, x2, y2 float64
}

// curveFlatteningSegments is the number of straight segments a single
// arc or Bézier curve is approximated with.
const curveFlatteningSegments = 24

func (p *path) begin() {
	p.subs = nil
}

func (p *path) moveTo(x, y float64) {
	p.subs = append(p.subs, subPath{startX: x, startY: y})
}

// ensureSubPath starts a new sub-path at the origin if the path is empty,
// mirroring the canvas behaviour of path methods that implicitly create
// a sub-path.
func (p *path) ensureSubPath() {
	if len(p.subs) == 0 {
		p.moveTo(0, 0)
	}
}

func (p *path) current() *subPath {
	if len(p.subs) == 0 {
		return nil
	}
	return &p.subs[len(p.subs)-1]
}

// currentPoint returns the end point of the last sub-path and false if the
// path has no sub-paths.
func (p *path) currentPoint() (x, y float64, ok bool) {
	sub := p.current()
	if sub == nil {
		return 0, 0, false
	}
	if len(sub.segments) == 0 {
		return sub.startX, sub.startY, true
	}
	last := sub.segments[len(sub.segments)-1]
	return last.x2, last.y2, true
}

func (p *path) lineTo(x, y float64) {
	p.ensureSubPath()
	sub := p.current()
	sx, sy, _ := p.currentPoint()
	sub.segments = append(sub.segments, segment{sx, sy, x, y})
}

func (p *path) closePath() {
	sub := p.current()
	if sub == nil || sub.closed {
		return
	}
	sx, sy, _ := p.currentPoint()
	if sx != sub.startX || sy != sub.startY {
		sub.segments = append(sub.segments, segment{sx, sy, sub.startX, sub.startY})
	}
	sub.closed = true
}

func (p *path) rect(x, y, w, h float64) {
	p.moveTo(x, y)
	p.lineTo(x+w, y)
	p.lineTo(x+w, y+h)
	p.lineTo(x, y+h)
	p.closePath()
}

// bezierCurveTo appends a flattened cubic Bézier curve.
func (p *path) bezierCurveTo(cp1x, cp1y, cp2x, cp2y, x, y float64) {
	p.ensureSubPath()
	sub := p.current()
	sx, sy, _ := p.currentPoint()
	prevX, prevY := sx, sy
	for i := 1; i <= curveFlatteningSegments; i++ {
		t := float64(i) / curveFlatteningSegments
		px, py := cubicBezierPoint(t, sx, sy, cp1x, cp1y, cp2x, cp2y, x, y)
		sub.segments = append(sub.segments, segment{prevX, prevY, px, py})
		prevX, prevY = px, py
	}
}

func cubicBezierPoint(t, x0, y0, x1, y1, x2, y2, x3, y3 float64) (float64, float64) {
	mt := 1 - t
	a := mt * mt * mt
	b := 3 * mt * mt * t
	c := 3 * mt * t * t
	d := t * t * t
	return a*x0 + b*x1 + c*x2 + d*x3,
		a*y0 + b*y1 + c*y2 + d*y3
}

// quadraticCurveTo appends a flattened quadratic Bézier curve.
func (p *path) quadraticCurveTo(cpx, cpy, x, y float64) {
	p.ensureSubPath()
	sub := p.current()
	sx, sy, _ := p.currentPoint()
	prevX, prevY := sx, sy
	for i := 1; i <= curveFlatteningSegments; i++ {
		t := float64(i) / curveFlatteningSegments
		mt := 1 - t
		px := mt*mt*sx + 2*mt*t*cpx + t*t*x
		py := mt*mt*sy + 2*mt*t*cpy + t*t*y
		sub.segments = append(sub.segments, segment{prevX, prevY, px, py})
		prevX, prevY = px, py
	}
}

// ellipse appends a flattened elliptical arc, mirroring Context.Ellipse.
// If the current sub-path is empty the arc starts a new sub-path at the
// arc's first point, matching the canvas behaviour; otherwise a straight
// segment connects the current point to the arc start.
func (p *path) ellipse(x, y, radiusX, radiusY, rotation, startAngle, endAngle float64, anticlockwise bool) {
	delta := normalizeArcSweep(startAngle, endAngle, anticlockwise)
	if delta == 0 {
		return
	}
	sinRot, cosRot := math.Sincos(rotation)
	arcPoint := func(angle float64) (float64, float64) {
		ex := radiusX * math.Cos(angle)
		ey := radiusY * math.Sin(angle)
		return x + cosRot*ex - sinRot*ey,
			y + sinRot*ex + cosRot*ey
	}
	startX, startY := arcPoint(startAngle)
	// An empty current sub-path starts at the arc's first point rather than
	// the implicit origin, so no spurious connecting segment is created.
	if sub := p.current(); sub == nil || len(sub.segments) == 0 {
		if sub == nil {
			p.moveTo(startX, startY)
		} else {
			sub.startX, sub.startY = startX, startY
		}
	}
	sub := p.current()
	prevX, prevY, _ := p.currentPoint()
	for i := 1; i <= curveFlatteningSegments; i++ {
		angle := startAngle + delta*float64(i)/float64(curveFlatteningSegments)
		px, py := arcPoint(angle)
		sub.segments = append(sub.segments, segment{prevX, prevY, px, py})
		prevX, prevY = px, py
	}
}

// arc appends a flattened circular arc, mirroring Context.Arc.
func (p *path) arc(x, y, radius, startAngle, endAngle float64, anticlockwise bool) {
	p.ellipse(x, y, radius, radius, 0, startAngle, endAngle, anticlockwise)
}

// arcTo appends an approximation of the ArcTo tangent arc plus its
// connecting line. The arc centre and angles are derived geometrically.
func (p *path) arcTo(x1, y1, x2, y2, radius float64) {
	p.ensureSubPath()
	x0, y0, _ := p.currentPoint()
	// Parallel tangent points at distance radius from (x1, y1).
	dx0, dy0 := x0-x1, y0-y1
	dx2, dy2 := x2-x1, y2-y1
	len0 := math.Hypot(dx0, dy0)
	len2 := math.Hypot(dx2, dy2)
	if len0 == 0 || len2 == 0 || radius == 0 {
		p.lineTo(x1, y1)
		return
	}
	u0x, u0y := dx0/len0, dy0/len0
	u2x, u2y := dx2/len2, dy2/len2
	dot := u0x*u2x + u0y*u2y
	if dot <= -1+1e-12 {
		// Tangents are opposite: no well-defined arc.
		p.lineTo(x1, y1)
		return
	}
	// Tangent points.
	t0x, t0y := x1+u0x*radius, y1+u0y*radius
	t2x, t2y := x1+u2x*radius, y2+u2y*radius
	p.lineTo(t0x, t0y)
	// Centre: intersection of the lines through the tangent points
	// perpendicular to the tangents.
	cx0, cy0 := t0x-u0y*radius, t0y+u0x*radius
	cx2, cy2 := t2x-u2y*radius, t2y+u2x*radius
	centre := intersectLines(cx0, cy0, cx0+u0x, cy0+u0y, cx2, cy2, cx2+u2x, cy2+u2y)
	startAngle := math.Atan2(t0y-centre.y, t0x-centre.x)
	endAngle := math.Atan2(t2y-centre.y, t2x-centre.x)
	cross := u0x*u2y - u0y*u2x
	anticlockwise := cross > 0
	p.ellipse(centre.x, centre.y, radius, radius, 0, startAngle, endAngle, anticlockwise)
}

type point struct{ x, y float64 }

func intersectLines(ax, ay, bx, by, cx, cy, dx, dy float64) point {
	den := (bx-ax)*(dy-cy) - (by-ay)*(dx-cx)
	if math.Abs(den) < 1e-15 {
		return point{x: cx, y: cy}
	}
	t := ((cx-ax)*(dy-cy) - (cy-ay)*(dx-cx)) / den
	return point{x: ax + t*(bx-ax), y: ay + t*(by-ay)}
}

// normalizeArcSweep returns the signed angular sweep from startAngle to
// endAngle in the requested direction, in the range (-2*pi, 2*pi).
// A full revolution (endAngle-startAngle of a whole multiple of 2*pi) is
// preserved as a full 2*pi sweep rather than collapsing to zero.
func normalizeArcSweep(startAngle, endAngle float64, anticlockwise bool) float64 {
	twoPi := 2 * math.Pi
	raw := endAngle - startAngle
	if math.Mod(math.Abs(raw), twoPi) == 0 && raw != 0 {
		if anticlockwise {
			return -twoPi
		}
		return twoPi
	}
	delta := math.Mod(raw, twoPi)
	if anticlockwise {
		if delta > 0 {
			delta -= twoPi
		}
	} else {
		if delta < 0 {
			delta += twoPi
		}
	}
	return delta
}

// contains reports whether the point (x, y), expressed in the path's local
// coordinate system, lies inside the path according to the non-zero
// winding rule. This rule handles self-intersecting paths correctly.
func (p *path) contains(x, y float64) bool {
	winding := 0
	for _, sub := range p.subs {
		for _, seg := range sub.segments {
			winding += crossingWinding(seg.x1, seg.y1, seg.x2, seg.y2, x, y)
		}
	}
	return winding != 0
}

// crossingWinding contributes +1 or -1 for a segment crossing the
// horizontal ray from (px, py) toward +x, using the non-zero rule.
func crossingWinding(x1, y1, x2, y2, px, py float64) int {
	if y1 <= py {
		if y2 > py && isLeft(x1, y1, x2, y2, px, py) > 0 {
			return 1
		}
		return 0
	}
	if y2 <= py && isLeft(x1, y1, x2, y2, px, py) < 0 {
		return -1
	}
	return 0
}

// isLeft returns > 0 if the point (px, py) is left of the directed line
// from (x1, y1) to (x2, y2), < 0 if right, 0 if on it.
func isLeft(x1, y1, x2, y2, px, py float64) float64 {
	return (x2-x1)*(py-y1) - (px-x1)*(y2-y1)
}

// strokeDistance returns the minimum distance from the point (x, y) to
// the stroked centreline of the path in local coordinates. Open sub-paths
// contribute their segments; closed sub-paths contribute the same
// segments including the closing segment.
func (p *path) strokeDistance(x, y float64) float64 {
	minDist := math.Inf(1)
	for _, sub := range p.subs {
		for _, seg := range sub.segments {
			d := distanceToSegment(x, y, seg.x1, seg.y1, seg.x2, seg.y2)
			if d < minDist {
				minDist = d
			}
		}
	}
	return minDist
}

// isEmpty reports whether the path contains no drawable segments. Such a
// path can never be hit.
func (p *path) isEmpty() bool {
	for _, sub := range p.subs {
		if len(sub.segments) > 0 {
			return false
		}
	}
	return true
}

// isZeroSized reports whether all segments of the path collapse onto a
// single point (zero bounding area). These paths produce no fillable or
// strokeable area under the canvas rules and can never be hit.
func (p *path) isZeroSized() bool {
	var x, y float64
	have := false
	for _, sub := range p.subs {
		for _, seg := range sub.segments {
			if !have {
				x, y = seg.x1, seg.y1
				have = true
			}
			if seg.x1 != x || seg.y1 != y || seg.x2 != x || seg.y2 != y {
				return false
			}
		}
	}
	return true
}

func distanceToSegment(px, py, x1, y1, x2, y2 float64) float64 {
	dx, dy := x2-x1, y2-y1
	lengthSq := dx*dx + dy*dy
	if lengthSq == 0 {
		return math.Hypot(px-x1, py-y1)
	}
	t := ((px-x1)*dx + (py-y1)*dy) / lengthSq
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	cx, cy := x1+t*dx, y1+t*dy
	return math.Hypot(px-cx, py-cy)
}
