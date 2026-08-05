package physics

import (
	"fmt"
	"math"

	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/game"
)

// StepGroundedShipTrackPhysics ports the confirmed flag-0x10-clear path of
// IntegrateShipPhysicsAndTrackContact (0x80030a48-0x80031390). The executable
// computes thrust and SpeedMagnitude first, resolves the selected wall run,
// samples and applies driving-surface contact, integrates acceleration/drag/
// position, and only then samples 128 units ahead for pitch alignment.
//
// ship+0x9e is initialized to zero and has no other aligned writer in the
// retail executable, so its pitch-response bypass is dormant.
func StepGroundedShipTrackPhysics(ship *Ship, track *assets.Track) (int, error) {
	if ship == nil || track == nil {
		return 0, fmt.Errorf("physics: grounded track step requires a ship and track")
	}
	if ship.InertiaFactor == 0 || ship.DragCoefficient == 0 {
		return 0, fmt.Errorf("physics: grounded track step requires nonzero inertia and drag")
	}

	if err := prepareShipTrackFrame(ship, track); err != nil {
		return 0, err
	}
	return stepGroundedShipTrackPhysicsPrepared(ship, track)
}

func stepGroundedShipTrackPhysicsPrepared(ship *Ship, track *assets.Track) (int, error) {

	thrust, redirectTarget := prepareAirborneForces(ship)
	_, firstFace, err := shipSectionAndFirstDrivingFace(ship, track)
	if err != nil {
		return 0, err
	}
	responses, ok := ResolveShipWallSensorCollisions(ship, int(ship.SectionID), track.Faces[firstFace], track.Sections, track.Faces, track.Vertices)
	if !ok {
		return responses, fmt.Errorf("physics: section %d wall collision geometry invalid", ship.SectionID)
	}

	sample, err := SampleShipTrackContact(ship, track)
	if err != nil {
		return responses, err
	}
	contactDistance := sample.CenterDistance
	if contactDistance < -300 {
		contactDistance = -300
	}
	ApplyTrackSurfaceContactImpulse(ship, sample.Normal, contactDistance)
	springDenominator := ship.GroundedSpring + (ship.AirBrakeLeft+ship.AirBrakeRight)/4
	if springDenominator == 0 {
		return responses, fmt.Errorf("physics: grounded track step requires nonzero spring divisor")
	}
	spring := Vector3{
		X: (redirectTarget.X - ship.Velocity.X) / springDenominator,
		Y: (redirectTarget.Y - ship.Velocity.Y) / springDenominator,
		Z: (redirectTarget.Z - ship.Velocity.Z) / springDenominator,
	}
	force := TrackSurfaceForce(sample.Normal, contactDistance, sample.SectionY, ship.Position.Y)
	ApplyTrackSectionHeightCorrection(ship, sample.SectionY)
	acceleration := Vector3{
		X: spring.X + (force.X+thrust.X)/ship.InertiaFactor,
		Y: spring.Y + (force.Y+thrust.Y)/ship.InertiaFactor,
		Z: spring.Z + (force.Z+thrust.Z)/ship.InertiaFactor,
	}
	applyShipAccelerationDragAndPosition(ship, acceleration)

	face := track.Faces[sample.FaceIndex]
	probe := Vector3{
		X: ship.Position.X + ship.Forward.X*128,
		Y: ship.Position.Y + ship.Forward.Y*128,
		Z: ship.Position.Z + ship.Forward.Z*128,
	}
	forwardDistance := PlaneDistance(probe, faceVertex0(face, track.Vertices), sample.Normal)
	UpdateTrackPitchAlignment(ship, contactDistance, forwardDistance)
	return responses, nil
}

// StepShipTrackPhysics dispatches the two confirmed branches selected by
// ship flag 0x10. Ordinary sections preserve that state; jump sections may
// set it when the ship projected onto the selected driving-face plane is
// outside the polygon.
func StepShipTrackPhysics(ship *Ship, track *assets.Track) (int, error) {
	if ship == nil || track == nil {
		return 0, fmt.Errorf("physics: track step requires a ship and track")
	}
	if ship.InertiaFactor == 0 || ship.DragCoefficient == 0 {
		return 0, fmt.Errorf("physics: track step requires nonzero inertia and drag")
	}
	if err := prepareShipTrackFrame(ship, track); err != nil {
		return 0, err
	}
	if ship.Flags&game.ShipFlagFarFromTrackSection == 0 {
		return stepGroundedShipTrackPhysicsPrepared(ship, track)
	}

	thrust, redirectTarget := prepareAirborneForces(ship)
	section, firstFace, err := shipSectionAndFirstDrivingFace(ship, track)
	if err != nil {
		return 0, err
	}
	responses := 0
	if ship.Velocity.Y < float32(section.Y) {
		var ok bool
		responses, ok = ResolveShipWallSensorCollisions(ship, int(ship.SectionID), track.Faces[firstFace], track.Sections, track.Faces, track.Vertices)
		if !ok {
			return responses, fmt.Errorf("physics: section %d wall collision geometry invalid", ship.SectionID)
		}
	}
	if distance, err := FarTrackRecoveryDistance(ship, track); err != nil {
		return responses, err
	} else if distance >= 32001 {
		ship.RecoveryTimer = 500
		ship.Speed = 0
		ship.Flags |= game.ShipFlagRecoveryState
	}
	integrateAirborneShipPhysics(ship, thrust, redirectTarget)
	return responses, nil
}

// FarTrackRecoveryDistance ports the centerline projection and asymmetric
// weighting at 0x80031444-0x80031528. For a projection above the ship
// (deltaY<=0), every delta is multiplied by eight. Otherwise only deltaY is
// divided by 128. The resulting magnitude is compared with 32001.
func FarTrackRecoveryDistance(ship *Ship, track *assets.Track) (float32, error) {
	if ship == nil || track == nil || !validSectionIndex(int(ship.SectionID), track.Sections) {
		return 0, fmt.Errorf("physics: recovery distance requires a valid current section")
	}
	section := track.Sections[ship.SectionID]
	nextIndex := int(section.Next)
	if !validSectionIndex(nextIndex, track.Sections) {
		return 0, fmt.Errorf("physics: section %d has invalid next link %d", ship.SectionID, nextIndex)
	}
	start := Vector3{X: float32(section.X), Y: float32(section.Y), Z: float32(section.Z)}
	next := track.Sections[nextIndex]
	end := Vector3{X: float32(next.X), Y: float32(next.Y), Z: float32(next.Z)}
	line := Vector3{X: end.X - start.X, Y: end.Y - start.Y, Z: end.Z - start.Z}
	denominator := line.X*line.X + line.Y*line.Y + line.Z*line.Z
	projected := start
	if denominator != 0 {
		fromStart := Vector3{X: ship.Position.X - start.X, Y: ship.Position.Y - start.Y, Z: ship.Position.Z - start.Z}
		t := dotProduct(fromStart, line) / denominator
		projected = Vector3{X: start.X + line.X*t, Y: start.Y + line.Y*t, Z: start.Z + line.Z*t}
	}
	delta := Vector3{X: projected.X - ship.Position.X, Y: projected.Y - ship.Position.Y, Z: projected.Z - ship.Position.Z}
	if delta.Y <= 0 {
		delta.X *= 8
		delta.Y *= 8
		delta.Z *= 8
	} else {
		delta.Y /= 128
	}
	return vectorMagnitude(delta), nil
}

func prepareShipTrackFrame(ship *Ship, track *assets.Track) error {
	UpdateShipOrientationVectors(ship)
	if _, err := UpdateShipTrackSection(ship, track); err != nil {
		return err
	}
	if err := UpdateShipTrackFaceSide(ship, track); err != nil {
		return err
	}
	section := track.Sections[ship.SectionID]
	if section.Flags&assets.TrackSectionJump != 0 {
		sample, err := SampleShipTrackContact(ship, track)
		if err != nil {
			return err
		}
		projected := Vector3{
			X: ship.Position.X - sample.Normal.X*sample.CenterDistance,
			Y: ship.Position.Y - sample.Normal.Y*sample.CenterDistance,
			Z: ship.Position.Z - sample.Normal.Z*sample.CenterDistance,
		}
		if !PointInsideFace(projected, track.Faces[sample.FaceIndex], track.Vertices) {
			ship.Flags |= game.ShipFlagFarFromTrackSection
		}
	}
	return nil
}

// ApplyTrackSectionHeightCorrection ports 0x80030f80-0x80030fcc. When the
// current section center is at least 705 units below the ship, the ship class
// whose InertiaFactor is exactly 110 moves down by 80. Other classes halve an
// upward (negative-Y) velocity and move down by 16.
func ApplyTrackSectionHeightCorrection(ship *Ship, sectionY float32) {
	if sectionY-ship.Position.Y < 705 {
		return
	}
	if ship.InertiaFactor == 110 {
		ship.Position.Y += 80
		return
	}
	if ship.Velocity.Y < 0 {
		ship.Velocity.Y /= 2
	}
	ship.Position.Y += 16
}

func shipThrust(ship *Ship) Vector3 {
	boost := float32(1)
	if ship.BoostState != 0 {
		boost = 3
		if ship.BoostState >= 3 {
			boost = 6
		}
	}
	scale := ship.Speed * boost * 64
	return Vector3{X: ship.Forward.X * scale, Y: ship.Forward.Y * scale, Z: ship.Forward.Z * scale}
}

func vectorMagnitude(vector Vector3) float32 {
	return float32(math.Sqrt(float64(vector.X*vector.X + vector.Y*vector.Y + vector.Z*vector.Z)))
}

func applyShipAccelerationDragAndPosition(ship *Ship, acceleration Vector3) {
	const frameRateScale = 60.0 / 50.0
	ship.Velocity.X += acceleration.X * frameRateScale
	ship.Velocity.Y += acceleration.Y * frameRateScale
	ship.Velocity.Z += acceleration.Z * frameRateScale

	brakeSum := ship.AirBrakeLeft + ship.AirBrakeRight
	dragDivisor := ship.DragCoefficient * (74 - brakeSum/8) / 128
	ship.Velocity.X -= ship.Velocity.X / dragDivisor
	ship.Velocity.Y -= ship.Velocity.Y / dragDivisor
	ship.Velocity.Z -= ship.Velocity.Z / dragDivisor
	ship.Position.X += ship.Velocity.X / 64
	ship.Position.Y += ship.Velocity.Y / 64
	ship.Position.Z += ship.Velocity.Z / 64

	brakeDifference := ship.AirBrakeLeft - ship.AirBrakeRight
	ship.Yaw = (ship.Yaw + Angle(int32(brakeDifference/8*ship.SpeedMagnitude/32768))).Wrapped()
}

// trackSurfaceMinimumDistance is the literal 75-unit divisor floor at
// IntegrateShipPhysicsAndTrackContact 0x80030e70. It prevents the surface
// spring from becoming singular when the ship reaches or crosses the face.
const trackSurfaceMinimumDistance = float32(75)

// trackSurfaceZeroNormalForceDistance follows from 16384/256 == 64. It is
// only the algebraic zero of the normal term, not a claimed hover height:
// TRACK01's grid face has NormalY=-4096 and its authentic start position
// produces PlaneDistance=+300.
const trackSurfaceZeroNormalForceDistance = float32(256)

// TrackContactSample is the pair of surface probes consumed by the original
// suspension and pitch-alignment paths.
type TrackContactSample struct {
	FaceIndex       int
	Normal          Vector3
	CenterDistance  float32
	ForwardDistance float32
	SectionY        float32
}

// UpdateShipTrackFaceSide ports the track-side half of
// UpdateShipOrientationVectorsAndTrackSide (0x80032234-0x800323bc).
func UpdateShipTrackFaceSide(ship *Ship, track *assets.Track) error {
	section, firstFace, err := shipSectionAndFirstDrivingFace(ship, track)
	if err != nil {
		return err
	}
	face := track.Faces[firstFace]
	firstIndex, secondIndex := int(face.Indices[0]), int(face.Indices[1])
	if firstIndex >= len(track.Vertices) || secondIndex >= len(track.Vertices) {
		return fmt.Errorf("physics: driving face %d has invalid edge vertices", firstFace)
	}
	first, second := track.Vertices[firstIndex], track.Vertices[secondIndex]
	edge := Vector3{X: float32(first.X - second.X), Y: float32(first.Y - second.Y), Z: float32(first.Z - second.Z)}
	toSectionCenter := Vector3{X: float32(section.X) - ship.Position.X, Y: float32(section.Y) - ship.Position.Y, Z: float32(section.Z) - ship.Position.Z}
	if dotProduct(toSectionCenter, edge) > 0 {
		ship.Flags |= game.ShipFlagTrackFaceSide
	} else {
		ship.Flags &^= game.ShipFlagTrackFaceSide
	}
	return nil
}

// SampleShipTrackContact reproduces the paired-face selection and two plane
// samples used by IntegrateShipPhysicsAndTrackContact. Forward is a true unit
// vector, so multiplying it by 128 matches the executable's Q12 Forward/32
// probe position.
func SampleShipTrackContact(ship *Ship, track *assets.Track) (TrackContactSample, error) {
	section, firstFace, err := shipSectionAndFirstDrivingFace(ship, track)
	if err != nil {
		return TrackContactSample{}, err
	}
	faceIndex := firstFace + 1
	if ship.Flags&game.ShipFlagTrackFaceSide != 0 {
		faceIndex = firstFace
	}
	sectionEnd := int(section.FirstFace) + int(section.NumFaces)
	if faceIndex >= sectionEnd || faceIndex >= len(track.Faces) {
		return TrackContactSample{}, fmt.Errorf("physics: paired driving face %d outside section", faceIndex)
	}
	face := track.Faces[faceIndex]
	if int(face.Indices[0]) >= len(track.Vertices) {
		return TrackContactSample{}, fmt.Errorf("physics: driving face %d has invalid first vertex", faceIndex)
	}
	normal := FaceNormal(face)
	planePoint := faceVertex0(face, track.Vertices)
	forwardProbe := Vector3{X: ship.Position.X + ship.Forward.X*128, Y: ship.Position.Y + ship.Forward.Y*128, Z: ship.Position.Z + ship.Forward.Z*128}
	return TrackContactSample{
		FaceIndex:       faceIndex,
		Normal:          normal,
		CenterDistance:  PlaneDistance(ship.Position, planePoint, normal),
		ForwardDistance: PlaneDistance(forwardProbe, planePoint, normal),
		SectionY:        float32(section.Y),
	}, nil
}

func shipSectionAndFirstDrivingFace(ship *Ship, track *assets.Track) (assets.TrackSection, int, error) {
	if ship == nil || track == nil {
		return assets.TrackSection{}, 0, fmt.Errorf("physics: track contact requires ship and track")
	}
	sectionIndex := int(ship.SectionID)
	if sectionIndex < 0 || sectionIndex >= len(track.Sections) {
		return assets.TrackSection{}, 0, fmt.Errorf("physics: section %d out of range", sectionIndex)
	}
	section := track.Sections[sectionIndex]
	begin, end := int(section.FirstFace), int(section.FirstFace)+int(section.NumFaces)
	if begin >= len(track.Faces) || end > len(track.Faces) {
		return assets.TrackSection{}, 0, fmt.Errorf("physics: section %d face range invalid", sectionIndex)
	}
	for index := begin; index < end; index++ {
		if track.Faces[index].Flags&assets.TrackFaceTrack != 0 {
			return section, index, nil
		}
	}
	return assets.TrackSection{}, 0, fmt.Errorf("physics: section %d has no driving face", sectionIndex)
}

func dotProduct(a, b Vector3) float32 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }

// TrackSurfaceForce ports the driving-face contact-force numerator from
// IntegrateShipPhysicsAndTrackContact (SLES_003.27 0x80030e70-0x80030fdc).
// The caller still has to add thrust and divide by the ship's inertia field,
// as the original does immediately afterward.
//
// faceNormal is a true unit vector. The executable uses Q12 normals, so the
// q12Scale factor preserves the magnitude of its integer expression:
//
//	normalQ12 * 16384 / max(faceDistance, 75) - normalQ12 * 64
//
// sectionY is the current TrackSection's center Y, loaded through ship+4 at
// 0x80030f80. WipEout's world Y points downward; the separate +30000 term is
// present literally before the section-center correction is added.
func TrackSurfaceForce(faceNormal Vector3, faceDistance, sectionY, shipY float32) Vector3 {
	divisor := faceDistance
	if divisor < trackSurfaceMinimumDistance {
		divisor = trackSurfaceMinimumDistance
	}
	normalScale := q12Scale * (16384/divisor - 64)
	return Vector3{
		X: faceNormal.X * normalScale,
		Y: faceNormal.Y*normalScale + 30000 + (sectionY-shipY)*64,
		Z: faceNormal.Z * normalScale,
	}
}

// ApplyTrackSurfaceContactImpulse ports the physical core of
// ApplyTrackSurfaceContactImpulse (0x800337c0). Physics clamps distance to
// -300 before calling it. Distances >=31 produce no impulse; shallow positive
// distances add the base normal impulse, while non-positive contact also
// damps velocity and scales the impulse by penetration depth.
func ApplyTrackSurfaceContactImpulse(ship *Ship, faceNormal Vector3, distance float32) {
	if distance >= 31 {
		return
	}
	baseImpulse := float32(q12Scale * (60.0 / 50.0))
	if distance > 0 {
		ship.Velocity.X += faceNormal.X * baseImpulse
		ship.Velocity.Y += faceNormal.Y * baseImpulse
		ship.Velocity.Z += faceNormal.Z * baseImpulse
		return
	}
	ship.Velocity.X -= ship.Velocity.X / 8
	ship.Velocity.Y -= ship.Velocity.Y / 8
	ship.Velocity.Z -= ship.Velocity.Z / 8
	penetrationScale := baseImpulse * (1 - distance/16)
	ship.Velocity.X += faceNormal.X * penetrationScale
	ship.Velocity.Y += faceNormal.Y * penetrationScale
	ship.Velocity.Z += faceNormal.Z * penetrationScale
}

// UpdateTrackPitchAlignment ports the two-probe pitch-rate response at
// 0x8003133c-0x80031390. centerDistance is the previously measured driving
// face distance; forwardDistance is measured again from position plus the
// ship's Q12 forward vector /32, i.e. a probe 128 units ahead.
func UpdateTrackPitchAlignment(ship *Ship, centerDistance, forwardDistance float32) {
	if forwardDistance < 600 {
		ship.PitchRate += 5 + centerDistance - forwardDistance
	} else {
		ship.PitchRate -= 50
	}
	ship.PitchRate -= ship.PitchRate / 4
}
