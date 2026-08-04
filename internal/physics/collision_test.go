package physics

import (
	"math"
	"slices"
	"testing"

	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/game"
	"github.com/tridentsx/wipeout-go/internal/psx"
)

func TestPlaneDistanceZeroOnThePlane(t *testing.T) {
	d := PlaneDistance(Vector3{X: 5, Y: 0, Z: 0}, Vector3{X: 5, Y: 0, Z: 0}, Vector3{X: 1, Y: 0, Z: 0})
	if d != 0 {
		t.Errorf("PlaneDistance = %v, want 0 on the plane", d)
	}
}

func TestWallSensorEdgeMatchesQ12ShiftedCornerGeometry(t *testing.T) {
	position := Vector3{X: 1000, Y: 2000, Z: 3000}
	forward := Vector3{Z: 1}
	right := Vector3{X: 1}

	rear := WallSensorEdge(position, forward, right, true)
	if want := ([2]Vector3{{X: 744, Y: 2000, Z: 2744}, {X: 1256, Y: 2000, Z: 2744}}); rear != want {
		t.Errorf("subtract-forward sensors = %+v, want %+v", rear, want)
	}

	front := WallSensorEdge(position, forward, right, false)
	if want := ([2]Vector3{{X: 744, Y: 2000, Z: 3256}, {X: 1256, Y: 2000, Z: 3256}}); front != want {
		t.Errorf("add-forward sensors = %+v, want %+v", front, want)
	}
}

func TestSectionWallSweepSelectsWallRunOnEitherSideOfTrack(t *testing.T) {
	verts := []psx.TrackVertex{{X: -10}, {X: 10}}
	drivingFace := psx.TrackFace{Indices: [4]uint16{0, 1, 1, 1}, Flags: psx.TrackFaceTrack}
	faces := []psx.TrackFace{
		{Flags: 0},
		{Flags: psx.TrackFaceTrack},
		{Flags: psx.TrackFaceTrack | psx.TrackFaceFlip},
		{Flags: psx.TrackFaceFlip},
	}
	section := psx.TrackSection{X: 0, FirstFace: 0, NumFaces: 4}

	// vertex0-vertex1 points -X. A ship right of center makes the dot
	// positive and selects the prefix wall run.
	prefix, ok := SectionWallSweep(Vector3{X: 5}, section, drivingFace, faces, verts)
	if !ok || len(prefix) != 1 || prefix[0] != 0 {
		t.Fatalf("positive-side sweep = %v, %v; want [0], true", prefix, ok)
	}

	suffix, ok := SectionWallSweep(Vector3{X: -5}, section, drivingFace, faces, verts)
	if !ok || len(suffix) != 1 || suffix[0] != 3 {
		t.Fatalf("non-positive-side sweep = %v, %v; want [3], true", suffix, ok)
	}
}

func TestSectionWallSweepPreservesMultipleWallFacesInSelectedRun(t *testing.T) {
	verts := []psx.TrackVertex{{X: -10}, {X: 10}}
	drivingFace := psx.TrackFace{Indices: [4]uint16{0, 1, 1, 1}, Flags: psx.TrackFaceTrack}
	faces := []psx.TrackFace{
		{Flags: 0}, {Flags: psx.TrackFaceUnknown},
		{Flags: psx.TrackFaceTrack}, {Flags: psx.TrackFaceTrack},
		{Flags: psx.TrackFaceFlip}, {Flags: 0},
	}
	section := psx.TrackSection{FirstFace: 0, NumFaces: 6}

	prefix, _ := SectionWallSweep(Vector3{X: 5}, section, drivingFace, faces, verts)
	if want := []int{0, 1}; !slices.Equal(prefix, want) {
		t.Errorf("prefix = %v, want %v", prefix, want)
	}
	suffix, _ := SectionWallSweep(Vector3{X: -5}, section, drivingFace, faces, verts)
	if want := []int{4, 5}; !slices.Equal(suffix, want) {
		t.Errorf("suffix = %v, want %v", suffix, want)
	}
}

func TestSampleSectionWallSensorsUsesNoseAndSelectedEdge(t *testing.T) {
	verts := []psx.TrackVertex{{X: -10}, {X: 10}, {Z: -300}}
	drivingFace := psx.TrackFace{Indices: [4]uint16{0, 1, 1, 1}, Flags: psx.TrackFaceTrack}
	faces := []psx.TrackFace{
		{Indices: [4]uint16{2, 2, 2, 2}, NormalZ: -4096},
		{Flags: psx.TrackFaceTrack},
	}
	section := psx.TrackSection{FirstFace: 0, NumFaces: 2}

	samples, ok := SampleSectionWallSensors(
		Vector3{X: 5}, Vector3{Z: 1}, Vector3{X: 1},
		section, drivingFace, faces, verts,
	)
	if !ok || len(samples) != 1 {
		t.Fatalf("samples = %+v, %v; want one sample", samples, ok)
	}
	if samples[0].Nose != -812 {
		t.Errorf("nose distance = %v, want -812", samples[0].Nose)
	}
	if want := ([2]float32{-44, -44}); samples[0].Edge != want {
		t.Errorf("edge distances = %v, want %v", samples[0].Edge, want)
	}
}

func TestSelectPrefixNoseResponseFaceAtJunctionStart(t *testing.T) {
	verts := []psx.TrackVertex{
		{X: 0, Z: 0}, {X: 10, Z: 0}, {X: 10, Z: 10}, {X: 0, Z: 10},
		{X: 100, Z: 0}, {X: 110, Z: 0}, {X: 110, Z: 10}, {X: 100, Z: 10},
	}
	faces := []psx.TrackFace{
		{Indices: [4]uint16{0, 1, 2, 3}},
		{Indices: [4]uint16{4, 5, 6, 7}},
	}
	sections := []psx.TrackSection{
		{Next: 1, Flags: psx.TrackSectionJunctionStart},
		{FirstFace: 1},
	}

	if face, ok := SelectPrefixNoseResponseFace(Vector3{X: 5, Z: 5}, 0, 0, 0, sections, faces, verts); !ok || face != 0 {
		t.Errorf("current containment = %d, %v; want candidate face 0", face, ok)
	}
	// The neighbor only gates acceptance in this branch; the executable
	// still passes the original candidate face to the hard response.
	if face, ok := SelectPrefixNoseResponseFace(Vector3{X: 105, Z: 5}, 0, 0, 0, sections, faces, verts); !ok || face != 0 {
		t.Errorf("next containment = %d, %v; want candidate face 0", face, ok)
	}
	if _, ok := SelectPrefixNoseResponseFace(Vector3{X: 50, Z: 50}, 0, 0, 0, sections, faces, verts); ok {
		t.Error("expected point outside current and next faces to be rejected")
	}
}

func TestSelectPrefixNoseResponseFaceOrdinarySectionDoesNotRequireContainment(t *testing.T) {
	sections := []psx.TrackSection{{Next: -1, Previous: -1}}
	faces := []psx.TrackFace{{}}
	face, ok := SelectPrefixNoseResponseFace(Vector3{X: 999}, 0, 0, 0, sections, faces, nil)
	if !ok || face != 0 {
		t.Errorf("ordinary selector = %d, %v; want candidate face 0", face, ok)
	}
}

func TestSelectSuffixNoseResponseFaceUsesNeighborRightWallSlot(t *testing.T) {
	verts := []psx.TrackVertex{
		{X: 0, Z: 0}, {X: 10, Z: 0}, {X: 10, Z: 10}, {X: 0, Z: 10},
		{X: 100, Z: 0}, {X: 110, Z: 0}, {X: 110, Z: 10}, {X: 100, Z: 10},
	}
	faces := make([]psx.TrackFace, 8)
	faces[0] = psx.TrackFace{Indices: [4]uint16{0, 1, 2, 3}}
	faces[7] = psx.TrackFace{Indices: [4]uint16{4, 5, 6, 7}}
	sections := []psx.TrackSection{
		{Next: 1, Flags: psx.TrackSectionJunctionStart},
		{FirstFace: 4},
	}

	// At JunctionStart the neighbor's FirstFace+3 gates acceptance, while
	// the response continues to use the original candidate.
	face, ok := SelectSuffixNoseResponseFace(Vector3{X: 105, Z: 5}, 0, 0, 0, sections, faces, verts)
	if !ok || face != 0 {
		t.Errorf("suffix JunctionStart selector = %d, %v; want candidate face 0", face, ok)
	}
}

func TestSelectSuffixNoseResponseFaceJunctionEndSubstitutesPreviousRightWall(t *testing.T) {
	verts := []psx.TrackVertex{
		{X: 0, Z: 0}, {X: 10, Z: 0}, {X: 10, Z: 10}, {X: 0, Z: 10},
		{X: 100, Z: 0}, {X: 110, Z: 0}, {X: 110, Z: 10}, {X: 100, Z: 10},
	}
	faces := make([]psx.TrackFace, 8)
	faces[0] = psx.TrackFace{Indices: [4]uint16{0, 1, 2, 3}}
	faces[7] = psx.TrackFace{Indices: [4]uint16{4, 5, 6, 7}}
	sections := []psx.TrackSection{
		{Previous: 1, Flags: psx.TrackSectionJunctionEnd},
		{FirstFace: 4},
	}

	face, ok := SelectSuffixNoseResponseFace(Vector3{X: 105, Z: 5}, 0, 0, 0, sections, faces, verts)
	if !ok || face != 7 {
		t.Errorf("suffix JunctionEnd selector = %d, %v; want previous right-wall face 7", face, ok)
	}
}

func TestSelectWallNoseContactsComposesSweepPlaneAndTopology(t *testing.T) {
	verts := []psx.TrackVertex{
		{X: -10}, {X: 10},
		{Z: -300},
	}
	drivingFace := psx.TrackFace{Indices: [4]uint16{0, 1, 1, 1}, Flags: psx.TrackFaceTrack}
	faces := []psx.TrackFace{
		{Indices: [4]uint16{2, 2, 2, 2}, NormalZ: -4096},
		{Flags: psx.TrackFaceTrack},
	}
	sections := []psx.TrackSection{{FirstFace: 0, NumFaces: 2, Previous: -1, Next: -1}}

	contacts, ok := SelectWallNoseContacts(Vector3{X: 5}, Vector3{Z: 1}, Vector3{X: 1}, 0, drivingFace, sections, faces, verts)
	if !ok || len(contacts) != 1 {
		t.Fatalf("contacts = %+v, %v; want one contact", contacts, ok)
	}
	want := WallNoseContact{CandidateFace: 0, ResponseFace: 0, Distance: -812}
	if contacts[0] != want {
		t.Errorf("contact = %+v, want %+v", contacts[0], want)
	}
}

func TestPlaneDistanceSign(t *testing.T) {
	// Normal points along +X. A point further along +X than the plane
	// point has positive conventional signed distance.
	d := PlaneDistance(Vector3{X: 10, Y: 0, Z: 0}, Vector3{X: 0, Y: 0, Z: 0}, Vector3{X: 1, Y: 0, Z: 0})
	if d != 10 {
		t.Errorf("PlaneDistance = %v, want 10", d)
	}

	d2 := PlaneDistance(Vector3{X: -10, Y: 0, Z: 0}, Vector3{X: 0, Y: 0, Z: 0}, Vector3{X: 1, Y: 0, Z: 0})
	if d2 != -10 {
		t.Errorf("PlaneDistance = %v, want -10", d2)
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
		{Indices: [4]uint16{1, 1, 1, 1}, NormalX: -4096, NormalY: 0, NormalZ: 0, Flags: 0},                 // wall at X=100, normal -X
		{Indices: [4]uint16{0, 0, 0, 0}, NormalX: 4096, NormalY: 0, NormalZ: 0, Flags: psx.TrackFaceTrack}, // not a wall
	}
	section := psx.TrackSection{FirstFace: 0, NumFaces: 3}

	// Inward-facing normals give distances 10 and 90, so the X=0 wall wins.
	idx, dist, ok := NearestWallDistance(Vector3{X: 10, Y: 0, Z: 0}, section, faces, verts)
	if !ok {
		t.Fatal("expected NearestWallDistance to find a wall")
	}
	if idx != 0 {
		t.Errorf("faceIndex = %v, want 0 (the nearer wall)", idx)
	}
	if dist != 10 {
		t.Errorf("distance = %v, want 10", dist)
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

func TestHardWallCollisionResponsePortsSteeringKickAndCommonImpulse(t *testing.T) {
	position, velocity, steering := HardWallCollisionResponse(
		Vector3{}, Vector3{}, Vector3{X: 1, Y: 1, Z: 1},
		100, 800, 1, 0,
	)
	wantVelocity := Vector3{X: 8192, Y: 2048, Z: 8192}
	if velocity != wantVelocity {
		t.Errorf("velocity = %+v, want %+v", velocity, wantVelocity)
	}
	if want := (Vector3{X: 128, Y: 32, Z: 128}); position != want {
		t.Errorf("position = %+v, want %+v", position, want)
	}
	if steering != 700 { // 100 + 800/4 + 400
		t.Errorf("steering = %v, want 700", steering)
	}
}

func TestHardWallCollisionResponseNegativeDistanceBrakesAndKicksOtherDirection(t *testing.T) {
	_, velocity, steering := HardWallCollisionResponse(
		Vector3{}, Vector3{X: 320, Y: 640, Z: 960}, Vector3{},
		100, 800, -1, -10,
	)
	if want := (Vector3{X: 1, Y: 2, Z: 3}); velocity != want {
		t.Errorf("velocity = %+v, want %+v", velocity, want)
	}
	if steering != -500 {
		t.Errorf("steering = %v, want -500", steering)
	}
}

func TestFullWallCollisionResponseUsesGentlerSteeringKick(t *testing.T) {
	_, _, steering := FullWallCollisionResponse(Vector3{}, Vector3{}, Vector3{}, 100, 800, 1, 0)
	if steering != 325 { // 100 + 800/32 + 200
		t.Errorf("steering = %v, want 325", steering)
	}
}

func TestResolveShipWallSensorCollisionsProcessesNoseThenBothCorners(t *testing.T) {
	verts := []psx.TrackVertex{{X: -10}, {X: 10}, {Z: -300}}
	drivingFace := psx.TrackFace{Indices: [4]uint16{0, 1, 1, 1}, Flags: psx.TrackFaceTrack}
	faces := []psx.TrackFace{
		{Indices: [4]uint16{2, 2, 2, 2}, NormalZ: -4096},
		{Flags: psx.TrackFaceTrack},
	}
	sections := []psx.TrackSection{
		{FirstFace: 0, NumFaces: 2, Previous: -1, Next: 1, NextJunction: -1},
		{Z: 100, Previous: 0, Next: 0, NextJunction: -1},
	}
	ship := &Ship{Position: Vector3{X: 5}, Forward: Vector3{Z: 1}, Right: Vector3{X: 1}}

	responses, ok := ResolveShipWallSensorCollisions(ship, 0, drivingFace, sections, faces, verts)
	if !ok || responses != 1 {
		t.Fatalf("responses = %d, ok = %v; want 1, true", responses, ok)
	}
	if ship.SteeringRate != -400 {
		t.Errorf("SteeringRate = %v, want -400 after the nose response moves the later corner probes clear", ship.SteeringRate)
	}
}

func TestResolveShipTrackWallsTRACK01GridSlotAppliesSelectedWallResponse(t *testing.T) {
	track, err := (assets.Loader{Root: "../../assets/WIPEOUT2"}).LoadTrack("TRACK01")
	if err != nil {
		t.Fatal(err)
	}
	ship := &Ship{}
	if err := game.PlaceShipOnStartingGrid(ship, track, 0, 0); err != nil {
		t.Fatal(err)
	}
	responses, err := ResolveShipTrackWalls(ship, track)
	if err != nil {
		t.Fatal(err)
	}
	if responses != 0 {
		t.Errorf("grid-slot wall responses = %d, want 0", responses)
	}
	if ship.Velocity != (Vector3{}) {
		t.Errorf("Velocity = %+v, want no starting-grid wall impulse", ship.Velocity)
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

func TestResolveShipWallCollisionApproximateNoContactWhenFar(t *testing.T) {
	section, faces, verts := wallPanelFixture()
	s := &Ship{Position: Vector3{X: 50, Y: 5, Z: 5}}

	if ResolveShipWallCollisionApproximate(s, section, faces, verts, 2.0) {
		t.Error("expected no collision for a ship far from the wall")
	}
	if s.Position != (Vector3{X: 50, Y: 5, Z: 5}) {
		t.Errorf("position changed unexpectedly: %+v", s.Position)
	}
}

func TestResolveShipWallCollisionApproximateNoContactOutsideFaceFootprint(t *testing.T) {
	section, faces, verts := wallPanelFixture()
	// Close to the wall's plane (X=1) but well outside its Y/Z footprint.
	s := &Ship{Position: Vector3{X: 1, Y: 50, Z: 50}}

	if ResolveShipWallCollisionApproximate(s, section, faces, verts, 2.0) {
		t.Error("expected no collision when the projection falls outside the face's footprint")
	}
}

func TestResolveShipWallCollisionApproximateShallowContact(t *testing.T) {
	section, faces, verts := wallPanelFixture()
	s := &Ship{Position: Vector3{X: 1, Y: 5, Z: 5}}

	if !ResolveShipWallCollisionApproximate(s, section, faces, verts, 2.0) {
		t.Fatal("expected a shallow contact to resolve as a collision")
	}
	wantVel := Vector3{X: q12Scale * 2, Y: 0, Z: 0}
	if s.Velocity != wantVel {
		t.Errorf("velocity = %+v, want %+v", s.Velocity, wantVel)
	}
}

func TestResolveShipWallCollisionApproximateDeepPenetration(t *testing.T) {
	section, faces, verts := wallPanelFixture()
	// X=-5 is behind the inward-facing wall plane: distance=-5.
	s := &Ship{
		Position: Vector3{X: -5, Y: 5, Z: 5},
		Velocity: Vector3{X: 320, Y: 640, Z: 960},
	}

	if !ResolveShipWallCollisionApproximate(s, section, faces, verts, 2.0) {
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

func TestSelectWallResponse(t *testing.T) {
	tests := []struct {
		name                        string
		distance                    float32
		flags                       uint32
		priorCollision, pointInside bool
		want                        WallResponseKind
	}{
		{name: "outside plane", distance: 1, want: WallResponseNone},
		{name: "first ordinary contact", distance: 0, want: WallResponseGraze},
		{name: "later ordinary contact", distance: -1, priorCollision: true, want: WallResponseFull},
		{name: "special section outside polygon", distance: -1, flags: trackSectionGrazeOnlyMask, want: WallResponseNone},
		{name: "special section inside polygon", distance: -1, flags: trackSectionGrazeOnlyMask, pointInside: true, want: WallResponseGraze},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := SelectWallResponse(test.distance, test.flags, test.priorCollision, test.pointInside)
			if got != test.want {
				t.Fatalf("response = %d, want %d", got, test.want)
			}
		})
	}
}
