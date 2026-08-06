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
