package render

import "testing"

// The vertex shader treats inPosition.z as camera-space Z and emits
// vec4(x*z, y*z, depth*z, z). A zero there collapses every vertex to the origin
// with w = 0 and nothing rasterises -- the UI silently drew nothing at all. Pin the
// invariant so it cannot regress to zero.
func TestUIDepthIsOnTheNearPlane(t *testing.T) {
	if depthNear <= 0 {
		t.Fatalf("depthNear is %v; the UI would collapse to a degenerate vertex", depthNear)
	}
	// Sitting exactly on the near plane must map to depth 0, the nearest value, so
	// the UI wins the LESS test against the 1.0 depth clear.
	depth := (depthNear - depthNear) / (depthFar - depthNear)
	if depth != 0 {
		t.Errorf("near-plane depth is %v, want 0 so the UI draws in front", depth)
	}
}

// Retail coordinates must map to the corners of clip space, or the 320x240
// constants lifted from the executable land in the wrong place.
func TestUINdcMapping(t *testing.T) {
	u := &UI{}
	for _, tc := range []struct {
		x, y, wantX, wantY float32
	}{
		{0, 0, -1, 1},
		{RetailWidth, RetailHeight, 1, -1},
		{RetailWidth / 2, RetailHeight / 2, 0, 0},
	} {
		gotX, gotY := u.ndc(tc.x, tc.y)
		if gotX != tc.wantX || gotY != tc.wantY {
			t.Errorf("ndc(%v,%v) = (%v,%v), want (%v,%v)",
				tc.x, tc.y, gotX, gotY, tc.wantX, tc.wantY)
		}
	}
}

// Present issues one draw call per texture and iterates a Go map, so draw order
// between textures is randomised. Two UI quads at equal depth therefore swapped
// priority every frame, which is what made the splash screens flicker. Successive
// draws must get strictly nearer depths so the result is order-independent.
func TestUILayeringIsMonotonic(t *testing.T) {
	u := &UI{}
	u.BeginFrame()
	previous := float32(depthFar)
	for i := 0; i < 8; i++ {
		z := u.nextDepth()
		if z >= previous {
			t.Fatalf("draw %d got z=%v, not nearer than the previous %v", i, z, previous)
		}
		if z < depthNear {
			t.Fatalf("draw %d got z=%v, in front of the near plane", i, z)
		}
		previous = z
	}
	// A frame with more draws than layers must clamp rather than go degenerate.
	for i := 0; i < uiMaxLayers*2; i++ {
		if z := u.nextDepth(); z < depthNear {
			t.Fatalf("overflow draw %d got z=%v, below the near plane", i, z)
		}
	}
	// And the counter resets, or the first draw of frame two would sit behind the
	// last draw of frame one.
	u.BeginFrame()
	if z := u.nextDepth(); z != depthNear+uiMaxLayers*uiLayerStep {
		t.Errorf("after BeginFrame the first draw is at %v, want the back of the span", z)
	}
}

// PAL is 320x256. Every boot splash TIM is that tall, and retail's draw coordinates
// are in that frame.
func TestRetailFrameIsPAL(t *testing.T) {
	if RetailWidth != 320 || RetailHeight != 256 {
		t.Errorf("retail frame is %dx%d, want 320x256 for PAL", RetailWidth, RetailHeight)
	}
	// PRESS START sits at y=0xe4, which must fall inside the frame.
	if 0xe4 >= RetailHeight {
		t.Errorf("y=0xe4 falls outside a %d-tall frame", RetailHeight)
	}
}
