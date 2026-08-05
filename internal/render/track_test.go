package render

import (
	"testing"

	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/tridentsx/wipeout-go/internal/game"
)

func TestClipPerspectiveNearPlane(t *testing.T) {
	polygon := []perspectiveVertex{
		{position: game.Vector3{X: -2, Z: 2}, uv: sdl.FPoint{X: 0, Y: 0}},
		{position: game.Vector3{X: 0, Z: 0}, uv: sdl.FPoint{X: 0.5, Y: 1}},
		{position: game.Vector3{X: 2, Z: 2}, uv: sdl.FPoint{X: 1, Y: 0}},
	}
	clipped := clipPerspectiveNearPlane(polygon, 1)
	if len(clipped) != 4 {
		t.Fatalf("clipped vertex count = %d, want 4", len(clipped))
	}
	for i, vertex := range clipped {
		if vertex.position.Z < 1 {
			t.Fatalf("vertex %d remains behind near plane: %+v", i, vertex.position)
		}
	}
}

func TestProjectionDistancePreservesNativeHorizontalScale(t *testing.T) {
	const width = float32(320)
	focalLength := psxProjectionDistance * width / 320
	if focalLength != width/2 {
		t.Fatalf("focal length = %v, want %v (native width/2, for a 90-degree horizontal FOV)", focalLength, width/2)
	}
	// A point at the screen's right edge (x == width/2 in NDC-ish screen-space
	// offset) should sit at exactly 45 degrees off the view axis, i.e.
	// worldX/worldZ == 1, when the focal length equals half the screen width.
	edgeOffset := width / 2
	worldXOverZ := edgeOffset / focalLength
	if worldXOverZ != 1 {
		t.Fatalf("worldX/Z at screen edge = %v, want 1 (45 degrees, for a 90-degree horizontal FOV)", worldXOverZ)
	}
}
