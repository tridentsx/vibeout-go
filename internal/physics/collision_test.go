package physics

import (
	"math"
	"testing"

	"github.com/tridentsx/wipeout-go/internal/psx"
)

func TestPlaneDistanceZeroOnThePlane(t *testing.T) {
	d := PlaneDistance(Vector3{X: 5, Y: 0, Z: 0}, Vector3{X: 5, Y: 0, Z: 0}, Vector3{X: 1, Y: 0, Z: 0})
	if d != 0 {
		t.Errorf("PlaneDistance = %v, want 0 on the plane", d)
	}
}

func TestPlaneDistanceSign(t *testing.T) {
	// Normal points along +X. A point further along +X than the plane
	// point should be negative per this port's confirmed sign convention
	// (negated dot product -- see PlaneDistance's doc comment).
	d := PlaneDistance(Vector3{X: 10, Y: 0, Z: 0}, Vector3{X: 0, Y: 0, Z: 0}, Vector3{X: 1, Y: 0, Z: 0})
	if d != -10 {
		t.Errorf("PlaneDistance = %v, want -10", d)
	}

	d2 := PlaneDistance(Vector3{X: -10, Y: 0, Z: 0}, Vector3{X: 0, Y: 0, Z: 0}, Vector3{X: 1, Y: 0, Z: 0})
	if d2 != 10 {
		t.Errorf("PlaneDistance = %v, want 10", d2)
	}
}

func TestFaceNormalConvertsQ12ToUnitFloat(t *testing.T) {
	f := psx.TrackFace{NormalX: 4096, NormalY: 0, NormalZ: 0}
	n := FaceNormal(f)
	if n.X != 1 || n.Y != 0 || n.Z != 0 {
		t.Errorf("FaceNormal = %+v, want unit vector (1,0,0)", n)
	}
}

func TestIsWallFace(t *testing.T) {
	wall := psx.TrackFace{Flags: 0}
	track := psx.TrackFace{Flags: psx.TrackFaceTrack}
	trackWithBoost := psx.TrackFace{Flags: psx.TrackFaceTrack | psx.TrackFaceBoost}

	if !isWallFace(wall) {
		t.Error("expected a face with no flags to be a wall")
	}
	if isWallFace(track) {
		t.Error("expected a TrackFaceTrack-flagged face to not be a wall")
	}
	if isWallFace(trackWithBoost) {
		t.Error("expected a track+boost face to not be a wall")
	}
}

func TestNearestWallDistanceFindsClosestWall(t *testing.T) {
	verts := []psx.TrackVertex{
		{X: 0, Y: 0, Z: 0},   // index 0: near wall's vertex
		{X: 100, Y: 0, Z: 0}, // index 1: far wall's vertex
	}
	faces := []psx.TrackFace{
		{Indices: [4]uint16{0, 0, 0, 0}, NormalX: 4096, NormalY: 0, NormalZ: 0, Flags: 0},                  // wall at X=0, normal +X
		{Indices: [4]uint16{1, 1, 1, 1}, NormalX: 4096, NormalY: 0, NormalZ: 0, Flags: 0},                  // wall at X=100, normal +X
		{Indices: [4]uint16{0, 0, 0, 0}, NormalX: 4096, NormalY: 0, NormalZ: 0, Flags: psx.TrackFaceTrack}, // not a wall
	}
	section := psx.TrackSection{FirstFace: 0, NumFaces: 3}

	// A ship at X=10 is much closer to the wall at X=0 (distance -10) than
	// the one at X=100 (distance 90). Since PlaneDistance's sign convention
	// makes "closer/inside" more negative, the wall at X=0 should win as
	// the minimum.
	idx, dist, ok := NearestWallDistance(Vector3{X: 10, Y: 0, Z: 0}, section, faces, verts)
	if !ok {
		t.Fatal("expected NearestWallDistance to find a wall")
	}
	if idx != 0 {
		t.Errorf("faceIndex = %v, want 0 (the nearer wall)", idx)
	}
	if dist != -10 {
		t.Errorf("distance = %v, want -10", dist)
	}
}

func TestNearestWallDistanceIgnoresTrackFaces(t *testing.T) {
	verts := []psx.TrackVertex{{X: 0, Y: 0, Z: 0}}
	faces := []psx.TrackFace{
		{Indices: [4]uint16{0, 0, 0, 0}, NormalX: 4096, Flags: psx.TrackFaceTrack},
	}
	section := psx.TrackSection{FirstFace: 0, NumFaces: 1}

	_, _, ok := NearestWallDistance(Vector3{}, section, faces, verts)
	if ok {
		t.Error("expected no wall found when the only face is track-flagged")
	}
}

func TestWallBounceImpulseAddsScaledNormal(t *testing.T) {
	v := WallBounceImpulse(Vector3{X: 1, Y: 1, Z: 1}, Vector3{X: 1, Y: 1, Z: 0})
	want := Vector3{X: 1 + q12Scale, Y: 1 + q12Scale/2, Z: 1}
	if v != want {
		t.Errorf("WallBounceImpulse = %+v, want %+v", v, want)
	}
}

func TestWallCollisionResponseShallowNudgesPositionAndVelocity(t *testing.T) {
	pos := Vector3{X: 0, Y: 0, Z: 0}
	vel := Vector3{X: 0, Y: 0, Z: 0}
	normal := Vector3{X: 1, Y: 0, Z: 0}

	newPos, newVel := WallCollisionResponse(pos, vel, normal, false, 0)

	wantVel := Vector3{X: q12Scale * 2, Y: 0, Z: 0}
	if newVel != wantVel {
		t.Errorf("velocity = %+v, want %+v", newVel, wantVel)
	}
	wantPos := Vector3{X: wantVel.X / 64, Y: 0, Z: 0}
	if newPos != wantPos {
		t.Errorf("position = %+v, want %+v", newPos, wantPos)
	}
}

func TestWallCollisionResponseDeepReplacesVelocity(t *testing.T) {
	pos := Vector3{X: 5, Y: 5, Z: 5}
	vel := Vector3{X: 320, Y: 640, Z: 960}

	newPos, newVel := WallCollisionResponse(pos, vel, Vector3{}, true, 10)

	want := Vector3{X: 1, Y: 2, Z: 3}
	if newVel != want {
		t.Errorf("velocity = %+v, want %+v", newVel, want)
	}
	if newPos != pos {
		t.Errorf("position = %+v, want unchanged %+v", newPos, pos)
	}
}

func TestAngleBetweenVectorsPerpendicular(t *testing.T) {
	angle := AngleBetweenVectors(Vector3{X: 1}, Vector3{Y: 1})
	if want := float32(math.Pi / 2); !almostEqual(angle, want) {
		t.Errorf("angle = %v, want %v", angle, want)
	}
}

func TestAngleBetweenVectorsParallel(t *testing.T) {
	angle := AngleBetweenVectors(Vector3{X: 1}, Vector3{X: 5})
	if angle != 0 {
		t.Errorf("angle = %v, want 0", angle)
	}
}

func TestAngleBetweenVectorsOpposite(t *testing.T) {
	angle := AngleBetweenVectors(Vector3{X: 1}, Vector3{X: -1})
	if want := float32(math.Pi); !almostEqual(angle, want) {
		t.Errorf("angle = %v, want %v", angle, want)
	}
}

func TestAngleBetweenVectorsZeroLength(t *testing.T) {
	angle := AngleBetweenVectors(Vector3{}, Vector3{X: 1})
	if angle != 0 {
		t.Errorf("angle = %v, want 0 for a zero-length input", angle)
	}
}

func TestPointInsideFaceQuad(t *testing.T) {
	verts := []psx.TrackVertex{
		{X: 0, Y: 0, Z: 0},
		{X: 10, Y: 0, Z: 0},
		{X: 10, Y: 0, Z: 10},
		{X: 0, Y: 0, Z: 10},
	}
	f := psx.TrackFace{Indices: [4]uint16{0, 1, 2, 3}}

	if !PointInsideFace(Vector3{X: 5, Y: 0, Z: 5}, f, verts) {
		t.Error("expected the square's center to be inside")
	}
	if PointInsideFace(Vector3{X: 100, Y: 0, Z: 100}, f, verts) {
		t.Error("expected a point far outside the square to not be inside")
	}
}

func TestPointInsideFaceTriangle(t *testing.T) {
	verts := []psx.TrackVertex{
		{X: 0, Y: 0, Z: 0},
		{X: 10, Y: 0, Z: 0},
		{X: 0, Y: 0, Z: 10},
	}
	// Indices[3] == Indices[2] is this project's degenerate-triangle
	// convention (see psx.TrackFace's doc comment).
	f := psx.TrackFace{Indices: [4]uint16{0, 1, 2, 2}}

	if !PointInsideFace(Vector3{X: 2, Y: 0, Z: 2}, f, verts) {
		t.Error("expected a point inside the triangle to be inside")
	}
	if PointInsideFace(Vector3{X: 100, Y: 0, Z: 100}, f, verts) {
		t.Error("expected a point far outside the triangle to not be inside")
	}
}

// wallPanelFixture builds a single vertical wall face at X=0, normal +X
// (solid wall on the -X side, track on the +X side per PlaneDistance's
// confirmed sign convention), spanning Y and Z from 0 to 10.
func wallPanelFixture() (psx.TrackSection, []psx.TrackFace, []psx.TrackVertex) {
	verts := []psx.TrackVertex{
		{X: 0, Y: 0, Z: 0},
		{X: 0, Y: 10, Z: 0},
		{X: 0, Y: 10, Z: 10},
		{X: 0, Y: 0, Z: 10},
	}
	faces := []psx.TrackFace{
		{Indices: [4]uint16{0, 1, 2, 3}, NormalX: 4096, Flags: 0},
	}
	section := psx.TrackSection{FirstFace: 0, NumFaces: 1}
	return section, faces, verts
}

func TestResolveShipWallCollisionNoContactWhenFar(t *testing.T) {
	section, faces, verts := wallPanelFixture()
	s := &Ship{Position: Vector3{X: 50, Y: 5, Z: 5}}

	if ResolveShipWallCollision(s, section, faces, verts, 2.0) {
		t.Error("expected no collision for a ship far from the wall")
	}
	if s.Position != (Vector3{X: 50, Y: 5, Z: 5}) {
		t.Errorf("position changed unexpectedly: %+v", s.Position)
	}
}

func TestResolveShipWallCollisionNoContactOutsideFaceFootprint(t *testing.T) {
	section, faces, verts := wallPanelFixture()
	// Close to the wall's plane (X=1) but well outside its Y/Z footprint.
	s := &Ship{Position: Vector3{X: 1, Y: 50, Z: 50}}

	if ResolveShipWallCollision(s, section, faces, verts, 2.0) {
		t.Error("expected no collision when the projection falls outside the face's footprint")
	}
}

func TestResolveShipWallCollisionShallowContact(t *testing.T) {
	section, faces, verts := wallPanelFixture()
	s := &Ship{Position: Vector3{X: 1, Y: 5, Z: 5}}

	if !ResolveShipWallCollision(s, section, faces, verts, 2.0) {
		t.Fatal("expected a shallow contact to resolve as a collision")
	}
	wantVel := Vector3{X: q12Scale * 2, Y: 0, Z: 0}
	if s.Velocity != wantVel {
		t.Errorf("velocity = %+v, want %+v", s.Velocity, wantVel)
	}
}

func TestResolveShipWallCollisionDeepPenetration(t *testing.T) {
	section, faces, verts := wallPanelFixture()
	// X=-5 is behind the wall's plane -- a genuine penetration (distance=5>0).
	s := &Ship{
		Position: Vector3{X: -5, Y: 5, Z: 5},
		Velocity: Vector3{X: 320, Y: 640, Z: 960},
	}

	if !ResolveShipWallCollision(s, section, faces, verts, 2.0) {
		t.Fatal("expected a deep penetration to resolve as a collision")
	}
	want := Vector3{X: 2, Y: 4, Z: 6}
	if s.Velocity != want {
		t.Errorf("velocity = %+v, want %+v", s.Velocity, want)
	}
}

func TestNearestWallDistanceInvalidRange(t *testing.T) {
	verts := []psx.TrackVertex{{X: 0, Y: 0, Z: 0}}
	faces := []psx.TrackFace{{Indices: [4]uint16{0, 0, 0, 0}, NormalX: 4096, Flags: 0}}
	section := psx.TrackSection{FirstFace: 5, NumFaces: 3} // out of range for a 1-face slice

	_, _, ok := NearestWallDistance(Vector3{}, section, faces, verts)
	if ok {
		t.Error("expected an out-of-range section face list to return ok=false")
	}
}
