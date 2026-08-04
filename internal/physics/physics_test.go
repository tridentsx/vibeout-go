package physics

import (
	"math"
	"testing"
)

func TestUpdatePhysicsDecaysAndIntegrates(t *testing.T) {
	s := &Ship{Velocity: Vector3{X: 320, Y: 0, Z: 0}}

	UpdatePhysics(s)

	wantVelX := float32(320 * 15.0 / 16.0)
	if s.Velocity.X != wantVelX {
		t.Errorf("Velocity.X = %v, want %v", s.Velocity.X, wantVelX)
	}
	wantPosX := wantVelX / 32.0
	if s.Position.X != wantPosX {
		t.Errorf("Position.X = %v, want %v", s.Position.X, wantPosX)
	}
}

func TestUpdatePhysicsDecaysTowardZero(t *testing.T) {
	s := &Ship{Velocity: Vector3{X: 1000}}
	for i := 0; i < 500; i++ {
		UpdatePhysics(s)
	}
	if s.Velocity.X > 1 {
		t.Errorf("Velocity.X after 500 frames = %v, expected near zero", s.Velocity.X)
	}
}

func almostEqual(a, b float32) bool {
	const epsilon = 1e-3
	d := a - b
	return d > -epsilon && d < epsilon
}

func TestUpdateAirBrakesRampsUpWhenHeld(t *testing.T) {
	s := &Ship{}
	UpdateAirBrakes(s, true, false)
	if s.AirBrakeLeft != 38 {
		t.Errorf("AirBrakeLeft = %v, want 38", s.AirBrakeLeft)
	}
	if s.AirBrakeRight != 0 {
		t.Errorf("AirBrakeRight = %v, want 0 (not held)", s.AirBrakeRight)
	}
}

func TestUpdateAirBrakesRampsDownWhenReleased(t *testing.T) {
	s := &Ship{AirBrakeLeft: 100, AirBrakeRight: 100}
	UpdateAirBrakes(s, false, false)
	if s.AirBrakeLeft != 62 {
		t.Errorf("AirBrakeLeft = %v, want 62", s.AirBrakeLeft)
	}
	if s.AirBrakeRight != 62 {
		t.Errorf("AirBrakeRight = %v, want 62", s.AirBrakeRight)
	}
}

func TestUpdateAirBrakesFloorsAtZero(t *testing.T) {
	// Reached via the normal ramp path (one held frame puts it exactly at
	// the step size), releasing lands exactly on 0 and stays there -- the
	// >0 guard on the decrement means it does not go negative from here.
	// (An arbitrary non-multiple-of-38 starting value, unreachable through
	// normal ramping, *can* overshoot negative in one frame and then gets
	// stuck there since the <=0 branch makes no further change while not
	// held -- a real, LLIL-confirmed quirk of the original, but not a state
	// UpdateAirBrakes itself ever produces.)
	s := &Ship{AirBrakeLeft: 38, AirBrakeRight: 0}
	UpdateAirBrakes(s, false, false)
	if s.AirBrakeLeft != 0 {
		t.Errorf("AirBrakeLeft = %v, want 0", s.AirBrakeLeft)
	}
	UpdateAirBrakes(s, false, false)
	if s.AirBrakeLeft != 0 {
		t.Errorf("AirBrakeLeft = %v, want to stay at 0 on a second not-held frame", s.AirBrakeLeft)
	}
}

func TestUpdateAirBrakesLeftAndRightIndependent(t *testing.T) {
	s := &Ship{}
	UpdateAirBrakes(s, true, false)
	UpdateAirBrakes(s, true, false)
	if s.AirBrakeLeft != 76 {
		t.Errorf("AirBrakeLeft = %v, want 76 after two held frames", s.AirBrakeLeft)
	}
	if s.AirBrakeRight != 0 {
		t.Errorf("AirBrakeRight = %v, want 0 (never held)", s.AirBrakeRight)
	}
}

func TestIntegrateShipPhysicsForwardMatchesYawConvention(t *testing.T) {
	// Matches camera.go's NewChaseCamera convention: Yaw=0 faces +Z.
	s := &Ship{InertiaFactor: 64, DragCoefficient: 128}
	IntegrateShipPhysics(s)
	if !almostEqual(s.Forward.X, 0) || !almostEqual(s.Forward.Z, 1) {
		t.Errorf("Forward at Yaw=0 = (%v,%v), want (0,1)", s.Forward.X, s.Forward.Z)
	}
	if !almostEqual(s.Forward.Y, 0) {
		t.Errorf("Forward.Y at Pitch=0 = %v, want 0", s.Forward.Y)
	}
}

func TestIntegrateShipPhysicsForwardIsUnitVectorWithPitch(t *testing.T) {
	// Confirmed session 14 (maybe_UpdateShipOrientationVectorsAndTrackSide,
	// SLES_003.27 0x800320d8): Forward.Y = -sin(Pitch), and the whole Forward
	// row is a unit vector for arbitrary Yaw/Pitch, not just Yaw=const.
	s := &Ship{InertiaFactor: 64, DragCoefficient: 128, Yaw: Angle(500), Pitch: Angle(300)}
	IntegrateShipPhysics(s)

	wantY := -s.Pitch.Sin()
	if !almostEqual(s.Forward.Y, wantY) {
		t.Errorf("Forward.Y = %v, want -sin(Pitch) = %v", s.Forward.Y, wantY)
	}

	mag := float32(math.Sqrt(float64(s.Forward.X*s.Forward.X + s.Forward.Y*s.Forward.Y + s.Forward.Z*s.Forward.Z)))
	if !almostEqual(mag, 1) {
		t.Errorf("|Forward| = %v, want 1", mag)
	}
}

func TestIntegrateShipPhysicsRightMatchesConfirmedOrientationRow(t *testing.T) {
	s := &Ship{InertiaFactor: 64, DragCoefficient: 128}
	IntegrateShipPhysics(s)
	if !almostEqual(s.Right.X, 1) || !almostEqual(s.Right.Y, 0) || !almostEqual(s.Right.Z, 0) {
		t.Errorf("Right at zero rotation = %+v, want (1,0,0)", s.Right)
	}

	s = &Ship{InertiaFactor: 64, DragCoefficient: 128, Yaw: Angle(500), Pitch: Angle(300), Roll: Angle(700)}
	IntegrateShipPhysics(s)
	sinYaw, cosYaw := s.Yaw.Sin(), s.Yaw.Cos()
	sinPitch, cosPitch := s.Pitch.Sin(), s.Pitch.Cos()
	sinRoll, cosRoll := s.Roll.Sin(), s.Roll.Cos()
	want := Vector3{
		X: cosYaw*cosRoll + sinYaw*sinRoll*sinPitch,
		Y: -sinRoll * cosPitch,
		Z: sinYaw*cosRoll - cosYaw*sinPitch*sinRoll,
	}
	if !almostEqual(s.Right.X, want.X) || !almostEqual(s.Right.Y, want.Y) || !almostEqual(s.Right.Z, want.Z) {
		t.Errorf("Right = %+v, want %+v", s.Right, want)
	}
}

func TestIntegrateShipPhysicsThrustAcceleratesAlongForward(t *testing.T) {
	s := &Ship{Speed: 100, InertiaFactor: 64, DragCoefficient: 128}
	IntegrateShipPhysics(s)

	// Hand-derived expected value, same arithmetic order as the function:
	// thrust.Z = 100*1*1*64 = 6400; accel.Z = 6400/64 = 100 (spring term is
	// 0 since velocity starts at 0); *frameRateScale(1.2) = 120; then the
	// drag term (dragDivisor = 128*74/128 = 74) subtracts velocity/74.
	wantVelZ := float32(6400) / 64 * (60.0 / 50.0)
	wantVelZ -= wantVelZ / 74
	if !almostEqual(s.Velocity.Z, wantVelZ) {
		t.Errorf("Velocity.Z = %v, want %v", s.Velocity.Z, wantVelZ)
	}
	if s.Velocity.X != 0 {
		t.Errorf("Velocity.X = %v, want 0 (thrust is purely along +Z at Yaw=0)", s.Velocity.X)
	}
	wantPosZ := wantVelZ / 64
	if !almostEqual(s.Position.Z, wantPosZ) {
		t.Errorf("Position.Z = %v, want %v", s.Position.Z, wantPosZ)
	}
}

func TestIntegrateShipPhysicsBoostMultiplier(t *testing.T) {
	base := &Ship{Speed: 100, InertiaFactor: 64, DragCoefficient: 128, BoostState: 0}
	boosted := &Ship{Speed: 100, InertiaFactor: 64, DragCoefficient: 128, BoostState: 1}
	sixX := &Ship{Speed: 100, InertiaFactor: 64, DragCoefficient: 128, BoostState: 3}

	IntegrateShipPhysics(base)
	IntegrateShipPhysics(boosted)
	IntegrateShipPhysics(sixX)

	if !almostEqual(boosted.Velocity.Z, base.Velocity.Z*3) {
		t.Errorf("BoostState 1 gave Velocity.Z=%v, want 3x base (%v)", boosted.Velocity.Z, base.Velocity.Z*3)
	}
	if !almostEqual(sixX.Velocity.Z, base.Velocity.Z*6) {
		t.Errorf("BoostState 3 gave Velocity.Z=%v, want 6x base (%v)", sixX.Velocity.Z, base.Velocity.Z*6)
	}
}

func TestIntegrateShipPhysicsAirBrakeDifferentialTurnsYawAtRestIsZero(t *testing.T) {
	// SpeedMagnitude (the yaw term's scale factor) is derived from the
	// *incoming* velocity -- a ship starting from rest has SpeedMagnitude=0,
	// so no brake-differential yaw regardless of how hard it's braking,
	// matching the original's own ordering (ship.94 read before this
	// frame's own thrust update touches velocity).
	s := &Ship{Speed: 100, InertiaFactor: 64, DragCoefficient: 128, AirBrakeLeft: 80, AirBrakeRight: 0}
	IntegrateShipPhysics(s)
	if s.Yaw != 0 {
		t.Errorf("expected no yaw change at rest, got %v", s.Yaw)
	}
}

func TestIntegrateShipPhysicsAirBrakeDifferentialTurnsYawWhenMoving(t *testing.T) {
	// Seed a large existing velocity directly (rather than building it up
	// over many frames) so SpeedMagnitude is big enough to clear the
	// Angle(int32(...)) truncation at realistic brake-differential values.
	s := &Ship{Speed: 100, InertiaFactor: 64, DragCoefficient: 128,
		AirBrakeLeft: 80, AirBrakeRight: 0, Velocity: Vector3{Z: 10000}}
	IntegrateShipPhysics(s)
	if s.Yaw == 0 {
		t.Error("expected asymmetric air braking to produce a nonzero yaw change while moving")
	}
}

func TestIntegrateShipPhysicsSymmetricAirBrakingDoesNotTurnYaw(t *testing.T) {
	s := &Ship{Speed: 100, InertiaFactor: 64, DragCoefficient: 128,
		AirBrakeLeft: 80, AirBrakeRight: 80, Velocity: Vector3{Z: 10000}}
	IntegrateShipPhysics(s)
	if s.Yaw != 0 {
		t.Errorf("expected symmetric air braking to leave Yaw unchanged, got %v", s.Yaw)
	}
}

func TestIntegrateShipPhysicsAirBrakingIncreasesSpringDenominatorDampensCorrection(t *testing.T) {
	// A ship moving sideways relative to its heading (velocity.X != 0 while
	// Forward is +Z) should have that sideways velocity pulled toward zero
	// by the spring term. More combined air-brake pressure should make that
	// pull weaker (bigger denominator), i.e. more residual sideways velocity
	// survives one frame with brakes on than with brakes off.
	noBrakes := &Ship{Velocity: Vector3{X: 500}, InertiaFactor: 64, DragCoefficient: 128}
	withBrakes := &Ship{Velocity: Vector3{X: 500}, InertiaFactor: 64, DragCoefficient: 128, AirBrakeLeft: 200, AirBrakeRight: 200}

	IntegrateShipPhysics(noBrakes)
	IntegrateShipPhysics(withBrakes)

	if withBrakes.Velocity.X <= noBrakes.Velocity.X {
		t.Errorf("expected more residual sideways Velocity.X with brakes on (%v) than off (%v)",
			withBrakes.Velocity.X, noBrakes.Velocity.X)
	}
}
