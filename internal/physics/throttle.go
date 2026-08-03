package physics

// UpdateThrottle ramps a ship's Speed toward a throttle-derived target,
// porting maybe_IntegrateShipPhysicsFromPadInput's opening block
// (SLES_003.27 0x80066cac-0x80066da8, bn-psx/docs/wipeout2097_ship_physics_hunt.md
// session 8). Confirmed via raw disassembly, not just decompiled pseudo-C --
// the decompiler's pretty-printer flattened two different bit tests of
// padInputState[2] (0x2 and 0x4000) into a single misleadingly-repeated
// "if (v0==0) if (v0==0)", which reads as dead code but isn't.
//
// analogThrottle mirrors the original's padInputState[8] byte (roughly
// 0-200; 0 = no throttle, 200 = full) when analogAvailable is true (bit
// 0x2). When it's false, the original falls back to a purely digital
// ramp gated by bit 0x4000 of padInputState[2] -- digitalAccelerate here
// stands in for that bit, though which real pad button/state sets it was
// never pinned down. Both paths converge on the same fixed per-frame rate
// limit (0x13 = 19 in the original's fixed-point speed units) and the same
// clamp to MaxSpeed.
func UpdateThrottle(s *Ship, analogAvailable bool, analogThrottle float32, digitalAccelerate bool) {
	const rampStep = 19

	switch {
	case analogAvailable:
		target := s.MaxSpeed * analogThrottle / 200
		if target > s.MaxSpeed {
			target = s.MaxSpeed
		}
		switch {
		case analogThrottle <= 0:
			s.Speed -= rampStep
		case target < s.Speed:
			s.Speed -= rampStep
		default:
			step := target - s.Speed
			if step > rampStep {
				step = rampStep
			}
			s.Speed += step
		}
	case digitalAccelerate:
		s.Speed += rampStep
	default:
		s.Speed -= rampStep
	}

	if s.Speed > s.MaxSpeed {
		s.Speed = s.MaxSpeed
	}
}
