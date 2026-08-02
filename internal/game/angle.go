package game

import "math"

// Angle is the PS1 binary's 12-bit circle convention: 0-4095 spans a full
// turn, confirmed via bn-psx's maybe_GetSin/maybe_GetCos (0x8007fa38/
// 0x8007fb40 in SLES_003.27). Ship pitch/yaw/roll ([ship+0x70]/[+0x72]/
// [+0x74] in the original struct) and the AI autopilot's synthesized
// steering deltas are all expressed in this unit throughout the original
// binary, so keeping it as the interchange type here avoids constant
// unit-juggling against the reverse-engineered logic it's ported from.
type Angle uint16

// AngleFullTurn is one full revolution in Angle units.
const AngleFullTurn Angle = 4096

// Wrapped normalizes a into 0-4095, matching the original's `&0xfff` masking
// (seen throughout the physics/AI code whenever an angle is accumulated).
func (a Angle) Wrapped() Angle {
	return a % AngleFullTurn
}

func (a Angle) Radians() float64 {
	return float64(a) / float64(AngleFullTurn) * 2 * math.Pi
}

// Sin and Cos use Go's math library rather than porting the original's
// quarter-wave lookup table (0x800923ac) -- consistent with this project's
// float32-throughout decision (see TODO.md). The table's quantization is a
// PS1 hardware/ROM-size artifact, not gameplay-relevant behavior worth
// replicating; revisit only if bit-exact ship handling ever turns out to
// depend on it.
func (a Angle) Sin() float32 {
	return float32(math.Sin(a.Radians()))
}

func (a Angle) Cos() float32 {
	return float32(math.Cos(a.Radians()))
}
