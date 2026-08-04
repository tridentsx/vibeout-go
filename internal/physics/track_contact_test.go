package physics

import (
	"testing"

	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/game"
)

func TestTrackSurfaceForceNormalTermIsZeroAt256(t *testing.T) {
	force := TrackSurfaceForce(Vector3{Y: -1}, trackSurfaceZeroNormalForceDistance, 1000, 1000)
	if force != (Vector3{Y: 30000}) {
		t.Fatalf("force at equilibrium = %+v, want only vertical bias", force)
	}
}

func TestTrackSurfaceForceClampsDistanceTo75(t *testing.T) {
	normal := Vector3{Y: -1}
	clamped := TrackSurfaceForce(normal, 10, 0, 0)
	atFloor := TrackSurfaceForce(normal, trackSurfaceMinimumDistance, 0, 0)
	if clamped != atFloor {
		t.Fatalf("clamped force = %+v, floor force = %+v", clamped, atFloor)
	}
}

func TestTrackSurfaceForceIncludesSectionHeightError(t *testing.T) {
	force := TrackSurfaceForce(Vector3{}, 256, 1200, 1000)
	want := float32(30000 + 200*64)
	if force.Y != want {
		t.Fatalf("Y force = %v, want %v", force.Y, want)
	}
}

func TestUpdateTrackPitchAlignment(t *testing.T) {
	ship := &Ship{}
	UpdateTrackPitchAlignment(ship, 300, 280)
	if ship.PitchRate != 18.75 { // (0 + 5 + 20) * 3/4
		t.Fatalf("near pitch rate = %v, want 18.75", ship.PitchRate)
	}
	UpdateTrackPitchAlignment(ship, 300, 600)
	if ship.PitchRate != -23.4375 { // (18.75 - 50) * 3/4
		t.Fatalf("far pitch rate = %v, want -23.4375", ship.PitchRate)
	}
}

func TestUpdateShipTrackFaceSideAndSample(t *testing.T) {
	track := &assets.Track{
		Vertices: []assets.TrackVertex{
			{X: 100, Y: 0, Z: 0}, {X: -100, Y: 0, Z: 0},
			{X: 100, Y: 0, Z: 200}, {X: -100, Y: 0, Z: 200},
		},
		Faces: []assets.TrackFace{
			{Indices: [4]uint16{0, 1, 3, 2}, NormalY: -4096, Flags: assets.TrackFaceTrack},
			{Indices: [4]uint16{0, 1, 3, 2}, NormalY: -4096},
		},
		Sections: []assets.TrackSection{
			{Next: 1, Y: 500, FirstFace: 0, NumFaces: 2},
			{Next: 0, Y: 700},
		},
	}
	ship := &Ship{Position: Vector3{X: -10, Y: -256, Z: 20}, Forward: Vector3{Z: 1}}
	if err := UpdateShipTrackFaceSide(ship, track); err != nil {
		t.Fatal(err)
	}
	if ship.Flags&game.ShipFlagTrackFaceSide == 0 {
		t.Fatal("positive side did not set track-face flag")
	}
	sample, err := SampleShipTrackContact(ship, track)
	if err != nil {
		t.Fatal(err)
	}
	if sample.FaceIndex != 0 || sample.CenterDistance != 256 || sample.ForwardDistance != 256 || sample.SectionY != 500 {
		t.Fatalf("sample = %+v", sample)
	}

	ship.Position.X = 10
	if err := UpdateShipTrackFaceSide(ship, track); err != nil {
		t.Fatal(err)
	}
	if ship.Flags&game.ShipFlagTrackFaceSide != 0 {
		t.Fatal("negative side did not clear track-face flag")
	}
	sample, err = SampleShipTrackContact(ship, track)
	if err != nil || sample.FaceIndex != 1 {
		t.Fatalf("opposite-side sample = %+v, %v", sample, err)
	}
}

func TestApplyTrackSurfaceContactImpulse(t *testing.T) {
	ship := &Ship{Velocity: Vector3{X: 80}}
	ApplyTrackSurfaceContactImpulse(ship, Vector3{Y: -1}, 31)
	if ship.Velocity != (Vector3{X: 80}) {
		t.Fatalf("distance 31 changed velocity: %+v", ship.Velocity)
	}
	ApplyTrackSurfaceContactImpulse(ship, Vector3{Y: -1}, 10)
	if ship.Velocity.Y != -q12Scale*1.2 {
		t.Fatalf("shallow impulse Y = %v", ship.Velocity.Y)
	}

	ship.Velocity = Vector3{X: 80}
	ApplyTrackSurfaceContactImpulse(ship, Vector3{Y: -1}, -16)
	if ship.Velocity.X != 70 || ship.Velocity.Y != -q12Scale*2.4 {
		t.Fatalf("penetrating impulse velocity = %+v", ship.Velocity)
	}
}

func TestStepGroundedShipTrackPhysicsTRACK01PreservesConfirmedOrdering(t *testing.T) {
	track, err := (assets.Loader{Root: "../../assets/WIPEOUT2"}).LoadTrack("TRACK01")
	if err != nil {
		t.Fatal(err)
	}
	ship := &Ship{InertiaFactor: 100, DragCoefficient: 100, GroundedSpring: 10}
	if err := game.PlaceShipOnStartingGrid(ship, track, 0, 0); err != nil {
		t.Fatal(err)
	}

	responses, err := StepGroundedShipTrackPhysics(ship, track)
	if err != nil {
		t.Fatal(err)
	}
	if responses != 0 {
		t.Fatalf("wall responses = %d, want 0 before surface integration", responses)
	}
	if ship.Position == (Vector3{X: -39125, Y: 199, Z: 4123}) {
		t.Fatal("surface acceleration was not integrated into position")
	}
	if ship.PitchRate == 0 {
		t.Fatal("post-integration forward probe did not update pitch rate")
	}
}

func TestStepGroundedShipTrackPhysicsRejectsZeroDivisors(t *testing.T) {
	if _, err := StepGroundedShipTrackPhysics(&Ship{}, &assets.Track{}); err == nil {
		t.Fatal("zero inertia and drag accepted")
	}
}

func TestApplyTrackSectionHeightCorrection(t *testing.T) {
	special := &Ship{Position: Vector3{Y: 100}, Velocity: Vector3{Y: -40}, InertiaFactor: 110}
	ApplyTrackSectionHeightCorrection(special, 805)
	if special.Position.Y != 180 || special.Velocity.Y != -40 {
		t.Fatalf("special class correction = position %v velocity %v", special.Position.Y, special.Velocity.Y)
	}

	ordinary := &Ship{Position: Vector3{Y: 100}, Velocity: Vector3{Y: -40}, InertiaFactor: 100}
	ApplyTrackSectionHeightCorrection(ordinary, 805)
	if ordinary.Position.Y != 116 || ordinary.Velocity.Y != -20 {
		t.Fatalf("ordinary correction = position %v velocity %v", ordinary.Position.Y, ordinary.Velocity.Y)
	}

	untouched := &Ship{Position: Vector3{Y: 101}, Velocity: Vector3{Y: -40}, InertiaFactor: 100}
	ApplyTrackSectionHeightCorrection(untouched, 805)
	if untouched.Position.Y != 101 || untouched.Velocity.Y != -40 {
		t.Fatalf("sub-threshold ship changed: %+v", untouched)
	}
}

func TestStepShipTrackPhysicsDispatchesFarFromTrackBranch(t *testing.T) {
	track, err := (assets.Loader{Root: "../../assets/WIPEOUT2"}).LoadTrack("TRACK01")
	if err != nil {
		t.Fatal(err)
	}
	ship := &Ship{InertiaFactor: 155, DragCoefficient: 135, Velocity: Vector3{Y: 100000}, Flags: game.ShipFlagFarFromTrackSection}
	if err := game.PlaceShipOnStartingGrid(ship, track, 0, 0); err != nil {
		t.Fatal(err)
	}
	ship.Position.Y -= 20000
	ship.Position.X += 20000

	responses, err := StepShipTrackPhysics(ship, track)
	if err != nil {
		t.Fatal(err)
	}
	if ship.Flags&game.ShipFlagFarFromTrackSection == 0 {
		t.Fatal("far ship did not enter flag-0x10 branch")
	}
	_ = responses // Geometry determines whether the unconditional sweep hits.
	if ship.PitchRate != 0 {
		t.Fatalf("airborne branch applied grounded pitch alignment: %v", ship.PitchRate)
	}
}

func TestFarTrackRecoveryDistanceUsesAsymmetricVerticalWeight(t *testing.T) {
	track := &assets.Track{Sections: []assets.TrackSection{
		{Next: 1},
		{Next: 0, Z: 1000},
	}}
	ship := &Ship{SectionID: 0, Position: Vector3{X: 5000, Y: 100}}
	distance, err := FarTrackRecoveryDistance(ship, track)
	if err != nil {
		t.Fatal(err)
	}
	if distance <= 32001 {
		t.Fatalf("above-line weighted distance = %v, want recovery", distance)
	}

	ship.Position.Y = -100
	distance, err = FarTrackRecoveryDistance(ship, track)
	if err != nil {
		t.Fatal(err)
	}
	if distance < 4999 || distance > 5001 {
		t.Fatalf("below-line weighted distance = %v, want about 5000", distance)
	}
}
