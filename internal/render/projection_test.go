package render

import "testing"

// The two focal lengths must be equal, or the image is stretched on any window that is not
// 4:3. They were previously scaled by width/320 and height/240 independently, which on 16:9
// magnified everything horizontally by a third.
func TestProjectionHasSquarePixels(t *testing.T) {
	for _, size := range [][2]float32{{320, 240}, {640, 480}, {1280, 720}, {1920, 1080}} {
		x, y := projectionFocals(size[0], size[1])
		if x != y {
			t.Errorf("%vx%v: focal lengths %v and %v differ, so pixels are not square",
				size[0], size[1], x, y)
		}
	}
	// The vertical field of view must not depend on window width.
	_, tall := projectionFocals(640, 480)
	_, wide := projectionFocals(1280, 480)
	if tall != wide {
		t.Errorf("vertical focal length changed with width: %v vs %v", tall, wide)
	}
	// A 4:3 window keeps retail's geometry exactly.
	x, _ := projectionFocals(320, 240)
	if x != psxProjectionDistance {
		t.Errorf("at 320x240 the focal length is %v, want retail's %v", x, psxProjectionDistance)
	}
}
