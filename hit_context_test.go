// Copyright 2026 Frederik Zipp. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package canvas

import (
	"math"
	"testing"
)

// drain reads one flushed message from the draws channel.
func drain(t *testing.T, draws chan []byte) []byte {
	t.Helper()
	select {
	case msg := <-draws:
		return msg
	default:
		t.Fatal("expected a flushed message")
		return nil
	}
}

func TestDisabledKeepsProtocolAndEvents(t *testing.T) {
	draws := make(chan []byte, 1)
	ctx := newContext(draws, nil, nil) // hit regions off
	if ctx.HitRegionsEnabled() {
		t.Fatal("hit regions should be disabled by default")
	}
	ctx.BeginPath()
	ctx.Rect(0, 0, 10, 10)
	ctx.FillRect(1, 2, 3, 4)
	ctx.Flush()
	msg := drain(t, draws)
	// No hit-region opcodes may appear in the wire data when disabled.
	hitOpcodes := map[byte]bool{
		bEnableHitRegions: true,
		bAddHitRegion:     true,
		bRemoveHitRegion:  true,
		bClearHitRegions:  true,
	}
	for _, b := range msg {
		if hitOpcodes[b] {
			t.Errorf("disabled context emitted hit opcode %d", b)
		}
	}
	// API calls return ErrHitRegionsDisabled and emit nothing.
	if err := ctx.AddFillHitRegion("x"); err != ErrHitRegionsDisabled {
		t.Errorf("AddFillHitRegion: got %v, want ErrHitRegionsDisabled", err)
	}
	if _, err := ctx.RemoveHitRegion("x"); err != ErrHitRegionsDisabled {
		t.Errorf("RemoveHitRegion: got %v, want ErrHitRegionsDisabled", err)
	}
	if err := ctx.ClearHitRegions(); err != ErrHitRegionsDisabled {
		t.Errorf("ClearHitRegions: got %v, want ErrHitRegionsDisabled", err)
	}
	if ctx.HitRegionCount() != 0 || ctx.HitTest(1, 1) != nil {
		t.Error("disabled context must report no regions and no hits")
	}
}

func TestEnabledHandshakeAndRegionWireFormat(t *testing.T) {
	draws := make(chan []byte, 1)
	ctx := newContext(draws, nil, &Options{HitRegions: true})

	ctx.BeginPath()
	ctx.Rect(0, 0, 20, 20)
	if err := ctx.AddFillHitRegion("box"); err != nil {
		t.Fatalf("AddFillHitRegion: %v", err)
	}
	ctx.Flush()
	msg := drain(t, draws)
	// First byte is the one-time enable handshake.
	if msg[0] != bEnableHitRegions {
		t.Fatalf("first opcode = %d, want bEnableHitRegions(%d)", msg[0], bEnableHitRegions)
	}
	// The region opcode appears exactly once.
	var regionOps int
	for _, b := range msg {
		if b == bAddHitRegion {
			regionOps++
		}
	}
	if regionOps != 1 {
		t.Errorf("add-region opcode count = %d, want 1", regionOps)
	}
	if ctx.HitRegionCount() != 1 {
		t.Fatalf("region count = %d, want 1", ctx.HitRegionCount())
	}

	// A second flush must not repeat the handshake.
	ctx.FillRect(0, 0, 1, 1)
	ctx.Flush()
	msg2 := drain(t, draws)
	for _, b := range msg2 {
		if b == bEnableHitRegions {
			t.Error("enable handshake sent more than once")
		}
	}
}

func TestContextHitTestAndLocalCoords(t *testing.T) {
	draws := make(chan []byte, 1)
	ctx := newContext(draws, nil, &Options{HitRegions: true})
	ctx.Translate(100, 200)
	ctx.BeginPath()
	ctx.Rect(0, 0, 50, 50)
	if err := ctx.AddFillHitRegion("moved"); err != nil {
		t.Fatal(err)
	}
	// Canvas (110,220) -> local (10,20).
	info := ctx.HitTest(110, 220)
	if info == nil || info.Region != "moved" {
		t.Fatalf("hit: got %v", info)
	}
	if !approxEqual(info.X, 10, 1e-9) || !approxEqual(info.Y, 20, 1e-9) {
		t.Errorf("local coords: got (%g,%g), want (10,20)", info.X, info.Y)
	}
	if ctx.HitTest(5, 5) != nil {
		t.Error("point outside translated region must miss")
	}
}

func TestEnrichMouseAndTouchEvents(t *testing.T) {
	hit := newHitTester(0)
	hit.cursor = rectPath(0, 0, 100, 100)
	if _, err := hit.add("zone", HitFill); err != nil {
		t.Fatal(err)
	}

	// Mouse event inside the region gets a Hit attached.
	mouse := MouseDownEvent{MouseEvent: MouseEvent{X: 50, Y: 60}}
	enriched := enrichEventWithHit(mouse, hit).(MouseDownEvent)
	if enriched.Hit == nil || enriched.Hit.Region != "zone" {
		t.Fatalf("mouse enrichment: got %v", enriched.Hit)
	}
	// Outside the region: Hit is nil but the event is otherwise intact.
	outside := MouseMoveEvent{MouseEvent: MouseEvent{X: 200, Y: 200}}
	enrichedOut := enrichEventWithHit(outside, hit).(MouseMoveEvent)
	if enrichedOut.Hit != nil {
		t.Errorf("outside mouse hit: got %v, want nil", enrichedOut.Hit)
	}
	if enrichedOut.X != 200 {
		t.Error("enrichment must not alter original event fields")
	}

	// Multi-touch: two simultaneous contacts on and off the region are each
	// resolved independently.
	touch := TouchStartEvent{TouchEvent: TouchEvent{
		Touches: TouchList{
			{Identifier: 0, X: 10, Y: 10},   // on region
			{Identifier: 1, X: 150, Y: 150}, // off region
		},
	}}
	et := enrichEventWithHit(touch, hit).(TouchStartEvent)
	if et.Touches[0].Hit == nil || et.Touches[0].Hit.Region != "zone" {
		t.Errorf("touch 0 should hit zone, got %v", et.Touches[0].Hit)
	}
	if et.Touches[1].Hit != nil {
		t.Errorf("touch 1 should miss, got %v", et.Touches[1].Hit)
	}

	// A nil tester (feature off) leaves events untouched.
	plain := enrichEventWithHit(mouse, nil).(MouseDownEvent)
	if plain.Hit != nil {
		t.Error("nil tester must not attach hit info")
	}

	// Non-pointer events pass through unchanged.
	key := KeyDownEvent{KeyboardEvent: KeyboardEvent{Key: "a"}}
	if got := enrichEventWithHit(key, hit); got != key {
		t.Error("key event must pass through unchanged")
	}

	// Wheel and every touch event subtype are enriched too.
	wheel := WheelEvent{MouseEvent: MouseEvent{X: 10, Y: 10}}
	if got := enrichEventWithHit(wheel, hit).(WheelEvent); got.Hit == nil ||
		got.Hit.Region != "zone" {
		t.Errorf("wheel enrichment: got %v", got.Hit)
	}
	for _, ev := range []Event{
		TouchMoveEvent{TouchEvent: TouchEvent{Touches: TouchList{{X: 10, Y: 10}}}},
		TouchEndEvent{TouchEvent: TouchEvent{ChangedTouches: TouchList{{X: 10, Y: 10}}}},
		TouchCancelEvent{TouchEvent: TouchEvent{TargetTouches: TouchList{{X: 10, Y: 10}}}},
	} {
		got := enrichEventWithHit(ev, hit)
		var lists []TouchList
		switch e := got.(type) {
		case TouchMoveEvent:
			lists = []TouchList{e.Touches}
		case TouchEndEvent:
			lists = []TouchList{e.ChangedTouches}
		case TouchCancelEvent:
			lists = []TouchList{e.TargetTouches}
		}
		if lists[0][0].Hit == nil || lists[0][0].Hit.Region != "zone" {
			t.Errorf("%T enrichment missing hit", got)
		}
	}
}

func TestFreshContextResetsRegionState(t *testing.T) {
	// Simulates a client reconnect: each connection builds a new Context,
	// so region state and IDs must start fresh rather than leak across
	// connections.
	mk := func() *Context {
		return newContext(make(chan []byte, 1), nil, &Options{HitRegions: true})
	}
	first := mk()
	first.BeginPath()
	first.Rect(0, 0, 10, 10)
	if err := first.AddFillHitRegion("r"); err != nil {
		t.Fatal(err)
	}
	if first.HitRegionCount() != 1 {
		t.Fatalf("first context count = %d", first.HitRegionCount())
	}
	second := mk()
	if second.HitRegionCount() != 0 {
		t.Errorf("reconnected context must start with no regions, got %d", second.HitRegionCount())
	}
	if second.HitTest(5, 5) != nil {
		t.Error("reconnected context must have no region hits")
	}
}

func TestArcRegionGeometry(t *testing.T) {
	draws := make(chan []byte, 1)
	ctx := newContext(draws, nil, &Options{HitRegions: true})
	// A full circle of radius 50 centred at (100,100).
	ctx.BeginPath()
	ctx.Arc(100, 100, 50, 0, 2*math.Pi, false)
	if err := ctx.AddFillHitRegion("circle"); err != nil {
		t.Fatal(err)
	}
	// Centre is inside.
	if ctx.HitTest(100, 100) == nil {
		t.Error("circle centre must hit")
	}
	// A point 40 units from centre is inside.
	if ctx.HitTest(140, 100) == nil {
		t.Error("point inside circle must hit")
	}
	// A point 60 units from centre is outside.
	if ctx.HitTest(160, 100) != nil {
		t.Error("point outside circle must miss")
	}
}

func TestRegionRemoveClearWireOps(t *testing.T) {
	draws := make(chan []byte, 1)
	ctx := newContext(draws, nil, &Options{HitRegions: true})
	ctx.BeginPath()
	ctx.Rect(0, 0, 10, 10)
	_ = ctx.AddFillHitRegion("a")
	_ = ctx.AddStrokeHitRegion("b")

	ok, err := ctx.RemoveHitRegion("a")
	if err != nil || !ok {
		t.Fatalf("remove: ok=%v err=%v", ok, err)
	}
	ok2, _ := ctx.RemoveHitRegion("ghost")
	if ok2 {
		t.Error("removing a missing region must report false and emit no opcode")
	}
	if err := ctx.ClearHitRegions(); err != nil {
		t.Fatal(err)
	}
	ctx.Flush()
	msg := drain(t, draws)
	var removes, clears int
	for _, b := range msg {
		switch b {
		case bRemoveHitRegion:
			removes++
		case bClearHitRegions:
			clears++
		}
	}
	if removes != 1 {
		t.Errorf("remove opcodes = %d, want 1 (missing name emits none)", removes)
	}
	if clears != 1 {
		t.Errorf("clear opcodes = %d, want 1", clears)
	}
	if ctx.HitRegionCount() != 0 {
		t.Errorf("count after clear = %d, want 0", ctx.HitRegionCount())
	}
}

func TestRotatedRegionsThroughContext(t *testing.T) {
	// End-to-end through the public Context API: two rotated, overlapping
	// rectangles, verifying topmost selection and local coordinates.
	draws := make(chan []byte, 1)
	ctx := newContext(draws, nil, &Options{HitRegions: true})

	ctx.SetTransform(1, 0, 0, 1, 30, 45)
	ctx.BeginPath()
	ctx.Rect(0, 0, 40, 10)
	if err := ctx.AddFillHitRegion("a"); err != nil {
		t.Fatal(err)
	}

	ctx.SetTransform(1, 0, 0, 1, 50, 50)
	ctx.Rotate(math.Pi / 2)
	ctx.Translate(-20, -5)
	ctx.BeginPath()
	ctx.Rect(0, 0, 40, 10)
	if err := ctx.AddFillHitRegion("b"); err != nil {
		t.Fatal(err)
	}

	if info := ctx.HitTest(50, 50); info == nil || info.Region != "b" {
		t.Fatalf("overlap: got %v, want b", info)
	}
	if info := ctx.HitTest(35, 47); info == nil || info.Region != "a" {
		t.Fatalf("a-only: got %v, want a", info)
	}
}

func TestStrokeRegionThroughContext(t *testing.T) {
	draws := make(chan []byte, 1)
	ctx := newContext(draws, nil, &Options{HitRegions: true})
	ctx.SetLineWidth(6)
	ctx.BeginPath()
	ctx.Rect(0, 0, 100, 100)
	if err := ctx.AddStrokeHitRegion("outline"); err != nil {
		t.Fatal(err)
	}
	if ctx.HitTest(50, 3) == nil {
		t.Error("point in stroke band should hit")
	}
	if ctx.HitTest(50, 50) != nil {
		t.Error("interior of stroke-only region should miss")
	}
}
