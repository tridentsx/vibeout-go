package physics

import "math"

// UpdatePhysics advances a ship's velocity and position by one frame,
// porting the confirmed core of maybe_IntegrateShipPhysicsFromPadInput
// (SLES_003.27 0x80066c7c, bn-psx/docs/wipeout2097_ship_physics_hunt.md
// session 7): each axis of velocity decays by 1/16 per frame (the original:
// `v -= v>>4`, a fixed-point right-shift; the float32 equivalent is a
// straight multiply, since there's no truncation semantics worth
// replicating), then position accumulates velocity scaled by 1/32 (the
// original: `position += velocity>>5`).
//
// Correction (session 8): this exact decay+integrate shape is the
// *unconditional tail* of maybe_IntegrateShipPhysicsFromPadInput, used
// there as a fallback/coasting path -- it is NOT the same as
// maybe_IntegrateShipPhysics's (0x80030784) main thrust-driven integrator,
// which uses a different, spring-like acceleration term and integrates
// position at velocity>>6, not >>5. The two aren't interchangeable
// constants for the same mechanism; which one a real ship actually uses
// moment-to-moment is still open (see UpdateThrottle's doc and the hunt
// doc's session 8 section).
//
// This is deliberately only the confirmed drag-and-integrate core, not the
// full function -- steering input application, banking/lean, and the
// out-of-bounds recovery path are not yet decoded closely enough to port.
// Both the AI (maybe_RunShipAutopilot) and human (this function's namesake)
// paths funnel into this same shape; how a Ship's steering/thrust inputs
// should feed in here is still open pending further reverse engineering.
//
// Superseded for the primary path (sessions 8-9): IntegrateShipPhysics below
// now ports maybe_IntegrateShipPhysics's own velocity/position update, which
// is the one confirmed to be reachable per-ship-per-frame (via
// maybe_RunShipAutopilot, for both AI and real pad input -- see ship.go's
// ControlSource doc comment). This function is kept as-is: still a distinct,
// separately-confirmed shape from a different original function, not dead
// code, but callers wanting the real thrust/spring/drag model should use
// IntegrateShipPhysics instead.
func UpdatePhysics(s *Ship) {
	const decay = 15.0 / 16.0
	const integrationScale = 1.0 / 32.0

	s.Velocity.X *= decay
	s.Velocity.Y *= decay
	s.Velocity.Z *= decay

	s.Position.X += s.Velocity.X * integrationScale
	s.Position.Y += s.Velocity.Y * integrationScale
	s.Position.Z += s.Velocity.Z * integrationScale
}

// airBrakeRampStep is 0x26 (38), confirmed via raw llil (not just decompile
// -- see UpdateAirBrakes's doc comment) as sub_800662fc's per-frame ramp
// rate for both ship+0x90 and ship+0x92.
const airBrakeRampStep = 38

// UpdateAirBrakes ramps AirBrakeLeft/AirBrakeRight by airBrakeRampStep per
// frame based on held state, porting sub_800662fc (SLES_003.27 0x800662fc,
// bn-psx/docs/wipeout2097_ship_physics_hunt.md session 9). In the original
// this is gated on padInputState[2] bits 0x800 (left, feeds ship+0x90) and
// 0x400 (right, feeds ship+0x92); which physical PS1 pad button maps to
// which bit was never pinned down (session 9's "not resolved" note), so
// this takes pre-decoded bools rather than a raw pad word, consistent with
// UpdateThrottle/UpdateSteeringDigital's existing style.
//
// The original's "clamp to 0" branch is dead code: confirmed via raw llil
// (0x80066338-0x80066350) that the intermediate comparison result is always
// a 0/1 boolean, so the >=0 test guarding that branch is always true and the
// branch itself never executes. There is therefore no ceiling on these
// values anywhere in the original function -- this port doesn't invent one
// either. Landing exactly on 0 (always true when only ever modified in
// airBrakeRampStep increments starting from 0, as this function does) does
// act as a floor: the decrement only fires while > 0, and at exactly 0 the
// value is left unchanged rather than going negative.
func UpdateAirBrakes(s *Ship, leftHeld, rightHeld bool) {
	switch {
	case leftHeld:
		s.AirBrakeLeft += airBrakeRampStep
	case s.AirBrakeLeft > 0:
		s.AirBrakeLeft -= airBrakeRampStep
	}

	switch {
	case rightHeld:
		s.AirBrakeRight += airBrakeRampStep
	case s.AirBrakeRight > 0:
		s.AirBrakeRight -= airBrakeRampStep
	}
}

// IntegrateShipPhysics ports the confirmed `ship+0xc & 0x10` branch of
// maybe_IntegrateShipPhysics (SLES_003.27 0x80030784,
// bn-psx/docs/wipeout2097_ship_physics_hunt.md sessions 8-9): thrust from
// Speed/Forward/boost, a spring term that redirects velocity's own
// magnitude toward Forward (moderated by the air brakes' combined ramp),
// a thrust/InertiaFactor acceleration term with a Y-only gravity bias, a
// DragCoefficient-and-air-brake-modulated velocity decay, position
// integration, and an air-brake-differential yaw nudge.
//
// # Fixed-point-to-float32 scale corrections (read before touching this)
//
// The original stores Forward as a Q12 fixed-point unit vector (component
// range roughly [-4096,4096] for [-1,1]) and compensates for that scale with
// right-shifts wherever Forward is multiplied into something else. This
// port's Forward is a true float32 unit vector (no Q12), so every shift
// that existed purely to cancel Forward's Q12 scale has been adjusted by
// -12 bits relative to the original's literal shift amount:
//   - thrust = (speed * forwardQ12 * boost) >> 6 in the original becomes
//     speed * forward * boost * 64 here (>>6 - 12 = <<6, i.e. *64) --
//     dropping the shift entirely (as an earlier draft of this port did)
//     would silently be off by a factor of 64.
//   - the velocity-magnitude-redirect term uses (|velocity| * forwardQ12)
//     >> 12 in the original, which becomes a direct multiply here (>>12 -
//     12 = >>0, no shift) since both the -12 shift and Forward's +12 scale
//     cancel exactly.
//
// All other shifts in this function (the air-brake sum/difference terms,
// the drag divisor, position integration) involve no Q12 quantities and are
// ported as straight divisions by 2^n, matching this project's existing
// convention (see UpdatePhysics, IntegrateYawFromSteering) of not
// replicating the original's round-toward-zero truncation semantics.
//
// # Deliberately not ported (scope boundaries, not oversights)
//
//   - The function's *other* branch (`ship+0xc & 0x10 == 0`), which replaces
//     the air-brake spring term with a raycast against nearby track-surface
//     geometry not modeled in this project yet.
//   - The out-of-bounds distance check and speed-reset recovery path at the
//     top of the branch (built on sub_80031e8c).
//   - A track-section-boundary Y-position clip correction mid-branch.
//   - The pitch/bank/roll continuation at the function's tail (session 8:
//     "not confirmed cleanly enough to port yet", entangled with un-traced
//     collision-avoidance lean code). Yaw is still fully covered: by
//     IntegrateYawFromSteering (unchanged) plus the air-brake-differential
//     yaw term this function adds.
//
// # Confirmed since session 14 (was an engineering assumption before)
//
//   - Forward is built from BOTH Yaw and Pitch (session 8 originally derived
//     it from Yaw alone, flagged as an unconfirmed assumption, since the
//     original's own write site was never found). Session 14's systematic
//     sweep found it: `maybe_UpdateShipOrientationVectorsAndTrackSide`
//     (SLES_003.27 0x800320d8), called every frame per ship from
//     `maybe_DispatchShipDrawFunction`, writes `Forward.X = -sin(Yaw)*cos(Pitch)`,
//     `Forward.Y = -sin(Pitch)`, `Forward.Z = cos(Yaw)*cos(Pitch)` -- a true
//     unit vector for arbitrary Yaw/Pitch (verified numerically), reducing to
//     (0,0,1) at zero angles, matching camera.go's own "Yaw=0 faces +Z"
//     convention. Roll does not enter this vector -- it only appears in a
//     second matrix row the original writes (`ship+0x20/+0x24/+0x28`,
//     likely an Up or Right vector) that has no consumer in this port yet.
//
// # Engineering assumptions (not ported facts, still flagged)
//
//   - The `ship+0xc2 == 0` gate on updating SpeedMagnitude isn't decoded, so
//     this always recomputes it -- the common case per the original's own
//     logic.
func IntegrateShipPhysics(s *Ship) {
	const frameRateScale = 60.0 / 50.0 // PAL(50Hz)->NTSC(60Hz) normalization, confirmed elsewhere in this function
	const gravityBiasY = 79488         // 0x13880, added to Y-axis thrust only, before the InertiaFactor division

	sinYaw, cosYaw := s.Yaw.Sin(), s.Yaw.Cos()
	sinPitch, cosPitch := s.Pitch.Sin(), s.Pitch.Cos()
	s.Forward.X = -sinYaw * cosPitch
	s.Forward.Y = -sinPitch
	s.Forward.Z = cosYaw * cosPitch

	boostMul := float32(1)
	if s.BoostState != 0 {
		if s.BoostState < 3 {
			boostMul = 3
		} else {
			boostMul = 6
		}
	}

	thrust := Vector3{
		X: s.Speed * s.Forward.X * boostMul * 64,
		Y: s.Speed * s.Forward.Y * boostMul * 64,
		Z: s.Speed * s.Forward.Z * boostMul * 64,
	}

	velocityMagnitude := float32(math.Sqrt(float64(
		s.Velocity.X*s.Velocity.X + s.Velocity.Y*s.Velocity.Y + s.Velocity.Z*s.Velocity.Z,
	)))
	s.SpeedMagnitude = velocityMagnitude / 2

	redirectTarget := Vector3{
		X: velocityMagnitude * s.Forward.X,
		Y: velocityMagnitude * s.Forward.Y,
		Z: velocityMagnitude * s.Forward.Z,
	}

	brakeSum := s.AirBrakeLeft + s.AirBrakeRight
	springDenom := 4*brakeSum + 20 // 0x14

	accel := Vector3{
		X: (redirectTarget.X-s.Velocity.X)/springDenom + thrust.X/s.InertiaFactor,
		Y: (redirectTarget.Y-s.Velocity.Y)/springDenom + (thrust.Y+gravityBiasY)/s.InertiaFactor,
		Z: (redirectTarget.Z-s.Velocity.Z)/springDenom + thrust.Z/s.InertiaFactor,
	}

	s.Velocity.X += accel.X * frameRateScale
	s.Velocity.Y += accel.Y * frameRateScale
	s.Velocity.Z += accel.Z * frameRateScale

	dragDivisor := s.DragCoefficient * (74 - brakeSum/8) / 128 // 0x4a=74, >>3, >>7 in the original
	s.Velocity.X -= s.Velocity.X / dragDivisor
	s.Velocity.Y -= s.Velocity.Y / dragDivisor
	s.Velocity.Z -= s.Velocity.Z / dragDivisor

	s.Position.X += s.Velocity.X / 64
	s.Position.Y += s.Velocity.Y / 64
	s.Position.Z += s.Velocity.Z / 64

	brakeDiff := s.AirBrakeLeft - s.AirBrakeRight
	s.Yaw += Angle(int32(brakeDiff / 8 * s.SpeedMagnitude / 32768))
}
