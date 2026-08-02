package game

import "math"

// DistanceSquared and Distance port the distance computation shared by all
// four maybe_ShipReactionHandler{A,B,C,D} variants (SLES_003.27, session 6):
// sum of squared per-axis differences, then maybe_ISqrt (0x80080504, a
// fixed-point LZCS/LZCR fast integer sqrt) -- ported here as plain
// math.Sqrt per this project's float32-throughout decision, since the
// original's fast-inverse-style approximation was a PS1 CPU-budget
// necessity, not gameplay-relevant behavior.
func DistanceSquared(a, b Vector3) float32 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	dz := a.Z - b.Z
	return dx*dx + dy*dy + dz*dz
}

func Distance(a, b Vector3) float32 {
	return float32(math.Sqrt(float64(DistanceSquared(a, b))))
}

// TooClose reports whether two ships are within the given avoidance radius
// of each other, the trigger condition each maybe_ShipReactionHandler
// variant's caller (maybe_DispatchShipReactionHandler) checks before
// invoking one of the four deflection reactions. The original selects which
// variant (swerve left/right, brake, none) semi-randomly via maybe_Rand --
// not ported here yet; this is only the confirmed trigger-distance check.
func TooClose(a, b *Ship, radius float32) bool {
	return DistanceSquared(a.Position, b.Position) < radius*radius
}
