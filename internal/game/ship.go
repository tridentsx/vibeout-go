package game

// Vector3 is a runtime game-state vector (float32, per this project's
// float32-throughout decision), distinct from psx.Vector3 which is a raw
// int32 PRM-file field -- these represent different things even though the
// original binary uses fixed-point ints for both.
type Vector3 struct {
	X, Y, Z float32
}

// ControlSource selects which system drives a ship's physics each frame.
// In the original binary this maps to two structurally parallel but
// separate functions with no static call site found linking either to the
// per-frame loop (bn-psx/docs/wipeout2097_ship_physics_hunt.md, session 7):
// maybe_RunShipAutopilot (synthesizes pad-shaped input from track curvature,
// explicitly skips the ship whose [+0xac]&0x8000 flag is set) and
// maybe_IntegrateShipPhysicsFromPadInput (real pad input, no such skip).
// The exact runtime dispatch was never resolved from the static binary --
// this enum is this project's own explicit stand-in for that missing piece.
type ControlSource int

const (
	ControlAI ControlSource = iota
	ControlLocalPlayer
	ControlNetwork
)

// Ship holds the confirmed subset of the original 240-byte ship struct's
// fields. Offsets below are the original struct's byte offsets, cited for
// anyone cross-referencing bn-psx's decompilation -- this Go struct does
// not need to match the original's layout or size, only its confirmed
// semantics.
type Ship struct {
	ControlSource ControlSource

	// Position ([ship+0x40]/[+0x44]/[+0x48]) and Velocity ([ship+0x50]/
	// [+0x54]/[+0x58]) -- confirmed written together by both
	// maybe_IntegrateShipPhysics and maybe_IntegrateShipPhysicsFromPadInput.
	Position Vector3
	Velocity Vector3

	// Pitch, Yaw, Roll ([ship+0x70]/[+0x72]/[+0x74]) -- the session 7
	// breakthrough: confirmed as the actual per-ship orientation integration
	// site, ending a multi-session search. Both physics-integrator functions
	// write all three every frame.
	Pitch, Yaw, Roll Angle

	// Flags ([ship+0xc]), a bitfield tested throughout the collision,
	// retire, and AI-reaction code. Only the bits with a well-established
	// meaning across multiple independent confirmations are named below;
	// the rest are preserved verbatim but not yet individually decoded.
	Flags uint32

	// SectionID ([ship+0x98]) -- current track section index, used for
	// spatial queries (maybe_BuildSectionSpatialIndex and others) and AI
	// waypoint navigation (maybe_RunShipAutopilot).
	SectionID int16

	// TrackProgress ([ship+0xb0]/[+0xb4]) -- cumulative distance-along-track
	// accumulator, written by maybe_UpdateShipRaceRankAndAI from a
	// per-section curve-distance table; used for race rank/position.
	TrackProgress int32
	LapDistance   int16

	// RankCounter ([ship+0xc6]/[+0xc7]) -- signed relative-position counters
	// between ship pairs, maintained by maybe_UpdateShipRaceRankAndAI.
	RankCounter [2]int8
}

// ShipFlag bits with a well-established meaning, confirmed across multiple
// independently-examined functions (bn-psx/docs/wipeout2097_ship_physics_hunt.md).
// Bits not listed here exist and are tested throughout the original code but
// haven't been individually pinned down yet -- Flags preserves them all,
// this is deliberately not an exhaustive enum.
const (
	// ShipFlagRetired marks a ship that has retired/reset -- set alongside
	// the effect-callback-chain reset pattern seen in
	// maybe_CheckShipRetireStatus, maybe_UpdateShipRaceRankAndAI, and
	// maybe_IntegrateShipPhysicsFromPadInput's out-of-bounds recovery path.
	ShipFlagRetired uint32 = 0x40
)
