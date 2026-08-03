package physics

// Sign convention for SteeringRate throughout this file: positive = left,
// negative = right. This matches the confirmed empirical behavior of the
// original's NegCon-twist formula (bn-psx/docs/wipeout2097_ship_physics_hunt.md
// session 9: live-tested with a real NegCon-configured controller,
// leftward twist produced positive SteeringRate, rightward produced
// negative). The original's separate simple digital-accumulator branch
// increments SteeringRate for what session 9 empirically confirmed is the
// "Right" input bit -- i.e. the two branches use *opposite* sign
// conventions for the same physical direction in the original binary, an
// internal inconsistency that never surfaces as a player-visible bug
// there (a player only ever drives one branch at a time). This port
// deliberately does not reproduce that inconsistency: both
// UpdateSteeringDigital and UpdateSteeringTwist below use the twist
// formula's sign convention uniformly, as a conscious engineering choice
// for internal consistency, not a literal translation of both original
// branches' independent signs.

// UpdateSteeringDigital ramps SteeringRate toward turning left or right,
// porting the confirmed digital-input branch of maybe_RunShipAutopilot's
// steering block (SLES_003.27 0x8002495c,
// bn-psx/docs/wipeout2097_ship_physics_hunt.md sessions 8-9,
// 0x80024b90-0x80024c68) for the Right direction specifically (`padState &
// 0x20`, confirmed live session 9 to be digital Right): `steeringRate +=
// turnAccel`, doubled when crossing back through zero from the opposite
// sign.
//
// The original has no symmetric "digital Left" version of this simple
// accumulator -- digital Left instead routes through the same complex
// NegCon-twist-calibration formula as real analog twist input (bit 0x80,
// see UpdateSteeringTwist), which session 9 also found behaves correctly
// with genuine analog data but did not verify end-to-end for a bare
// digital press's specific input shape. Rather than route plain digital
// Left through that analog-calibration machinery (dragging in calibration
// constants that don't conceptually apply to a discrete button), this
// port gives Left the same clean accumulator shape as Right, mirrored --
// matching wipeout-rewrite's structurally symmetric approach (checked
// this session; phoboslab's clean-room reimplementation uses one
// sign-parameterized formula for both directions, no such asymmetry) --
// rather than reproducing what looks like an incomplete, NegCon-oriented
// patch job specific to this original binary. leftHeld takes priority if
// both are held, matching wipeout-rewrite's if/else-if structure.
func UpdateSteeringDigital(s *Ship, leftHeld, rightHeld bool) {
	switch {
	case leftHeld && s.SteeringRate >= 0:
		s.SteeringRate += s.TurnAccel
	case leftHeld:
		s.SteeringRate += s.TurnAccel * 2
	case rightHeld && s.SteeringRate <= 0:
		s.SteeringRate -= s.TurnAccel
	case rightHeld:
		s.SteeringRate -= s.TurnAccel * 2
	case s.SteeringRate > 0:
		s.SteeringRate -= s.TurnAccel
	case s.SteeringRate < 0:
		s.SteeringRate += s.TurnAccel
	}

	if s.SteeringRate > s.TurnRate {
		s.SteeringRate = s.TurnRate
	} else if s.SteeringRate < -s.TurnRate {
		s.SteeringRate = -s.TurnRate
	}
}

// rampToward advances current toward target by at most step per call,
// matching the target-seeking shape of maybe_RunShipAutopilot's NegCon
// branch (session 9): snap directly to target if within one step of it,
// otherwise move by step -- doubled when the *resulting* direction of
// travel opposes the current sign of the value being ramped (a snappier
// reversal, the same shape as the digital accumulator's doubling, just
// expressed as a target-seek instead of a raw increment).
func rampToward(current, target, step float32) float32 {
	switch {
	case current < target:
		s := step
		if current < 0 {
			s *= 2
		}
		if s < target-current {
			return current + s
		}
		return target
	case target < current:
		next := current
		if target-current >= -step {
			next = target
		}
		s := step
		if next > 0 {
			s *= 2
		}
		return next - s
	default:
		return current
	}
}

// UpdateSteeringTwist ports the confirmed NegCon twist-steering formula
// from maybe_RunShipAutopilot's complex branch (SLES_003.27 0x8002495c,
// bit 0x80 of padInputState[2] -- bn-psx/docs/wipeout2097_ship_physics_hunt.md
// session 9). Session 9 initially misread this as containing a real
// left-turn bug in the shipped game (an uninitialized-register branch
// found via raw disassembly), but a live test with a real NegCon-configured
// controller overturned that: fed a genuinely low twist byte via a
// two-separate-axis DuckStation binding (working around what turned out
// to be a single-stick binding config issue, not a game bug), the
// original computed a strongly positive SteeringRate exactly as this
// formula predicts -- the formula is confirmed correct end-to-end with
// real data, not just correct in isolated pieces.
//
// twist is the raw NegCon twist byte (0-255). TwistCenter is fixed at 128
// (confirmed live). TwistMargin and TwistDivisor are per-selected-ship-class
// calibration constants in the original -- confirmed live this session for
// the default case: margin=6 (curve index 1 into a {0,6,12,18,0,0,0,0}
// table at a fixed global), divisor=255. In the original these live as
// free-standing globals (data_80094d3c/data_80094942, set once at
// ship-class selection and shared by whichever ship reads real pad data),
// not per-ship-struct fields the way TurnAccel/TurnRate are -- modeled as
// Ship fields here anyway for API consistency with the rest of this
// codebase, defaulting to zero (caller must set them; zero values would
// make target always 0, i.e. no steering, rather than crashing).
//
// NOT ported: the original's exact byte-level deadzone clamp applied to
// the raw twist value before this formula runs (session 9 found a branch
// in it that appeared to read an uninitialized register in one input
// range -- never fully resolved whether it's a real bug or session 9's
// own mis-trace, since the live NegCon test that settled the *formula's*
// correctness used a byte value that didn't exercise that specific
// branch). This function instead clamps the raw byte's deviation from
// center directly against TwistMargin -- a simpler substitute, confirmed
// equivalent for the tested live input range (twist bytes 13 and 243),
// not proven identical at the extremes.
func UpdateSteeringTwist(s *Ship, twist uint8) {
	const twistCenter = 128

	deviation := float32(twistCenter) - float32(twist)
	switch {
	case deviation > 0 && deviation < s.TwistMargin:
		deviation = 0
	case deviation < 0 && -deviation < s.TwistMargin:
		deviation = 0
	}

	target := deviation * (s.TurnRate + 400) * 2 / s.TwistDivisor
	s.SteeringRate = rampToward(s.SteeringRate, target, s.TurnAccel)

	if s.SteeringRate > s.TurnRate {
		s.SteeringRate = s.TurnRate
	} else if s.SteeringRate < -s.TurnRate {
		s.SteeringRate = -s.TurnRate
	}
}

// IntegrateYawFromSteering applies SteeringRate to Yaw, porting the
// confirmed tail of maybe_IntegrateShipPhysics (0x80030784,
// bn-psx/docs/wipeout2097_ship_physics_hunt.md session 8): the original
// does `yaw += round(steeringRate >> 6)` using its round-toward-zero shift
// helper (sub_800255e8, confirmed this session). The arithmetic is done in
// int32 and truncated to Angle (uint16) at the end rather than converting
// a possibly-negative float32 straight to Angle, since Go only guarantees
// well-defined wraparound for signed-to-unsigned integer conversions, not
// float-to-unsigned ones.
func IntegrateYawFromSteering(s *Ship) {
	delta := int32(s.SteeringRate / 64)
	s.Yaw = Angle(int32(s.Yaw) + delta)
}

// pitchRampStep is 0x24 (36), maybe_RunShipAutopilot's per-frame ramp for
// PitchRate ([ship+0x78]), confirmed session 10.
const pitchRampStep = 36

// UpdatePitchInput ramps PitchRate by pitchRampStep per frame based on
// D-pad Up/Down, porting maybe_RunShipAutopilot's block at
// `padState & 0x40: pitchRate += 0x24; else padState & 0x10: pitchRate -=
// 0x24` (SLES_003.27 0x8002495c, bn-psx/docs/wipeout2097_ship_physics_hunt.md
// session 10).
//
// This isn't "banking" in the lean-into-a-turn sense despite the
// original ship struct offset's earlier working name -- WipEout 2097 has
// no dedicated bank button at all; real banking is an emergent effect of
// steering combined with the air brakes (already captured by the
// air-brake-differential yaw term in physics.go). This is a genuinely
// separate nose-pitch control on the D-pad's vertical axis (confirmed via
// real-world game knowledge, session 10): Up dips the nose down, Down
// pulls it up. Renamed from the earlier BankRate/UpdateBank to match.
//
// dpadUpHeld maps to the confirmed raw-bit chain (Up -> remap output bit
// 0x10 -> `pitchRate -= step`), matching "Up dips the nose down" if a
// decreasing PitchRate means nose-down -- plausible and internally
// consistent, but the sign-to-visual-direction convention itself was not
// independently live-verified this session, only inferred from a single
// confirmed raw-bit mapping plus the user's description. dpadDownHeld is
// bit 0x40 by elimination (the only other original-branch bit), not
// independently confirmed either. Unlike AirBrakeLeft/AirBrakeRight,
// there's no explicit "decay while neither held" step in the original
// here -- PitchRate's decay happens unconditionally every frame in
// IntegratePitchAndRoll instead, so this function only adds the step
// when held and otherwise leaves PitchRate untouched.
func UpdatePitchInput(s *Ship, dpadDownHeld, dpadUpHeld bool) {
	switch {
	case dpadDownHeld:
		s.PitchRate += pitchRampStep
	case dpadUpHeld:
		s.PitchRate -= pitchRampStep
	}
}

// IntegratePitchAndRoll ports the confirmed pitch and steering-rate/roll
// coupling from maybe_IntegrateShipPhysics's tail (SLES_003.27 0x80030784,
// bn-psx/docs/wipeout2097_ship_physics_hunt.md session 10, common to both
// of the function's branches):
//
//	pitchRate = (pitchRate - 60) - (pitchRate - 60) / 4   // compound decay
//	pitch += round(pitchRate / 16)
//
//	rollRate += round(steeringRate / 32)               // steering banks the ship
//	rollRate -= round(rollRate / 2)                    // heavy per-frame damping
//	roll = (roll + rollRate) - (roll + rollRate) / 8   // integrate, then decay the sum
//
// RollRate is entirely internal to this function -- it's not an
// externally-driven input the way PitchRate is (there's no separate
// update function for it; SteeringRate is RollRate's only input,
// confirmed this session after an initial mid-session mistrace suggested
// it might have no writer at all). This is the real banking mechanic:
// turning the ship (SteeringRate) rolls it into the turn, matching the
// "steering + airbrakes" description of WipEout 2097's actual banking
// feel -- the airbrake half of that feel comes from the
// air-brake-differential yaw term in physics.go, not from here. Call this
// once per frame alongside IntegrateYawFromSteering; both read
// SteeringRate but write disjoint fields (Yaw vs Pitch/Roll), so ordering
// between them doesn't matter.
func IntegratePitchAndRoll(s *Ship) {
	s.PitchRate = (s.PitchRate - 60) - (s.PitchRate-60)/4
	s.Pitch = Angle(int32(s.Pitch) + int32(s.PitchRate/16))

	s.RollRate += s.SteeringRate / 32
	s.RollRate -= s.RollRate / 2

	rollSum := int32(s.Roll) + int32(s.RollRate)
	s.Roll = Angle(rollSum - rollSum/8)
}
