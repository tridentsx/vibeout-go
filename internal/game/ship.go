package game

// Vector3 is a runtime game-state vector (float32, per this project's
// float32-throughout decision), distinct from psx.Vector3 which is a raw
// int32 PRM-file field -- these represent different things even though the
// original binary uses fixed-point ints for both.
type Vector3 struct {
	X, Y, Z float32
}

// ControlSource selects which system drives a ship's physics each frame.
// Session 7 found two structurally parallel functions and couldn't resolve
// which one dispatches to a real player at runtime: maybe_RunShipAutopilot
// (synthesizes pad-shaped input from track curvature, explicitly skips the
// ship whose [+0xac]&0x8000 flag is set) and
// maybe_IntegrateShipPhysicsFromPadInput (real pad input, no such skip).
// Session 9 resolved this with a live breakpoint on
// maybe_IntegrateShipPhysicsFromPadInput's entry during active real-controller
// steering: it never fired. It's dead code, not an untraceable dispatch --
// maybe_RunShipAutopilot is confirmed to be the one real per-ship control
// path, for both AI and human input (whichever padInputState its caller
// passes in). This enum remains this project's own explicit stand-in for
// the (now-irrelevant) dispatch question, kept for the AI/network/local
// distinction itself, which is still useful even though the "which
// function" mystery is closed.
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

	// Yaw, Pitch, Roll ([ship+0x70]/[+0x72]/[+0x74]) -- the session 7
	// breakthrough: confirmed as the actual per-ship orientation integration
	// site, ending a multi-session search. Both physics-integrator functions
	// write all three every frame.
	//
	// Correction (session 10): session 5's original offset assignment
	// ("+0x70/+0x72/+0x74 = Pitch/Yaw/Roll") had pitch and yaw swapped --
	// it was an explicitly uncertain guess at the time ("open for
	// pitch/yaw"). Confirmed this session by tracing exactly which offset
	// each ported formula writes to: both IntegrateYawFromSteering's
	// steeringRate-derived term and the air-brake-differential term
	// (physics.go) write to [ship+0x70] -- that's Yaw. The bank-rate decay
	// term writes to [ship+0x72] -- Pitch. The roll-rate accumulator
	// writes to [ship+0x74] -- Roll, as already established. Field
	// declaration order below now matches the real offset order.
	Yaw, Pitch, Roll Angle

	// Flags ([ship+0xc]), a bitfield tested throughout the collision,
	// retire, and AI-reaction code. Only the bits with a well-established
	// meaning across multiple independent confirmations are named below;
	// the rest are preserved verbatim but not yet individually decoded.
	Flags uint32

	// SectionID ([ship+0xc8]/[+0xca]) -- current track section index, used
	// for spatial queries (maybe_BuildSectionSpatialIndex and others) and AI
	// waypoint navigation (maybe_RunShipAutopilot). Correction (session 8,
	// bn-psx/docs/wipeout2097_ship_physics_hunt.md): earlier sessions
	// mislabeled this as [ship+0x98], conflating it with the *section*
	// struct's own self-index field at the section's own +0x98 (reached via
	// [ship+4], a pointer to the ship's current TrackSection node) --
	// [ship+0x98] directly is a completely different field, see Speed below.
	SectionID int16
	// PreviousSectionID ([ship+0xca]) receives the section index that was
	// current before the per-frame nearest-section graph search.
	PreviousSectionID int16

	// Speed and MaxSpeed ([ship+0x98]/[+0x9a]) -- current speed, ramped
	// toward a throttle-derived target at a fixed rate (see UpdateThrottle),
	// and its per-ship-class cap (loaded from a difficulty-branched stat
	// table at race setup). Confirmed session 8. Wired into Velocity/Position
	// via IntegrateShipPhysics (sessions 8-9, see physics.go).
	Speed, MaxSpeed float32

	// Forward ([ship+0x10]/[+0x14]/[+0x18]) -- a fixed-point forward-facing
	// unit vector, distinct from the render rotation matrix. Confirmed to
	// feed the thrust formula in IntegrateShipPhysics (session 8), but how
	// the original keeps it in sync with Yaw/steering was never found (the
	// write site doesn't exist anywhere IntegrateShipPhysics's callers were
	// traced) -- IntegrateShipPhysics derives it from Yaw via cos/sin as an
	// explicit, flagged engineering assumption, not a ported fact.
	Forward Vector3

	// Right ([ship+0x20]/[+0x24]/[+0x28]) -- the second Q12 orientation
	// vector written beside Forward by UpdateShipOrientationVectorsAndTrackSide
	// (0x8003214c-0x80032218). At zero rotation it is (1,0,0). Wall collision
	// uses Right/16 together with Forward/16 to construct the ship's corner
	// probes. The runtime port stores the corresponding unit float vector.
	Right Vector3

	// AirBrakeLeft, AirBrakeRight ([ship+0x90]/[+0x92]) -- per-side air-brake
	// ramp values, confirmed session 9 (sub_800662fc): +38/frame while held,
	// -38/frame while released, floor at 0 (no ceiling found in the writer).
	// Their sum feeds IntegrateShipPhysics's spring-accel denominator
	// (braking trades responsiveness for control); their difference feeds a
	// speed-scaled yaw term (differential-brake steering assist).
	AirBrakeLeft, AirBrakeRight float32

	// BoostState ([ship+0x9c]) -- selects the thrust boost multiplier in
	// IntegrateShipPhysics: 0 -> 1x, 1-2 -> 3x, >=3 -> 6x (confirmed via
	// disassembly, session 8). What sets this value (boost pad? weapon
	// pickup?) was never reconciled against the pickup system -- only the
	// multiplier mapping itself is confirmed.
	BoostState int32

	// InertiaFactor ([ship+0x7e]) and DragCoefficient ([ship+0xa4]) --
	// per-ship-class stats loaded from the same difficulty-branched spec
	// tables as MaxSpeed/TurnAccel/TurnRate (session 4/8), used respectively
	// as the divisor for thrust's contribution to acceleration and as a
	// scale factor in an air-brake-modulated velocity decay term in
	// IntegrateShipPhysics (session 9). Their mathematical roles are
	// confirmed from disassembly; "inertia"/"drag" are this project's own
	// plausible-but-unconfirmed physical labels for what the original's own
	// docs never individually named (session 8: "not yet individually
	// identified").
	InertiaFactor, DragCoefficient float32

	// GroundedSpring ([ship+0xa6]) is a per-class stat used only by the
	// grounded branch's post-contact velocity redirect. The live denominator
	// is GroundedSpring + (AirBrakeLeft+AirBrakeRight)/4.
	GroundedSpring float32

	// SpeedMagnitude ([ship+0x94]) -- half of the current velocity vector's
	// magnitude, recomputed each IntegrateShipPhysics call (session 8-9).
	// Feeds the air-brake differential yaw term. The original gates this
	// recompute on an unconfirmed flag ([ship+0xc2]); this port always
	// recomputes it, the common case.
	SpeedMagnitude float32

	// SteeringRate ([ship+0x76]) -- a ramped turn-rate accumulator, and
	// TurnAccel/TurnRate ([ship+0xa0]/[+0xa2]), its per-ship-class ramp
	// rate and clamp (loaded from the same difficulty-branched spec tables
	// as MaxSpeed). Confirmed session 8. Sign convention (this port's own
	// choice, not literally the original's -- see steering.go's file-level
	// comment): positive = left, negative = right. Ported via
	// UpdateSteeringDigital (plain button input) and UpdateSteeringTwist
	// (NegCon twist input, session 9), both feeding IntegrateYawFromSteering.
	SteeringRate, TurnAccel, TurnRate float32

	// PitchRate ([ship+0x78], renamed from an earlier working name of
	// "BankRate" -- session 10 confirmed via real-world game knowledge
	// that WipEout 2097 has no dedicated bank button at all; this is a
	// separate nose-pitch control) -- a ramped accumulator feeding Pitch
	// each frame. Raw control-flow verification at 0x80031374 and
	// 0x80031964 shows that grounded contact applies quarter damping, while
	// only the airborne branch applies the `(pitchRate-60)*3/4` bias; the
	// branches rejoin before `pitch += pitchRate/16` at 0x80031a1c. Ramped +/-0x24 per
	// frame by maybe_RunShipAutopilot based on padInputState[2] bits
	// 0x40/0x10, matching D-pad Down/Up (Up dips the nose down, Down
	// pulls it up, per the user's real-world knowledge session 10) --
	// ported via UpdatePitchInput. The raw-bit-to-button mapping itself
	// (which of 0x40/0x10 is which D-pad direction) rests on one earlier
	// hand-traced LLIL reading plus the visual-direction description, not
	// an independent live verification the way the air-brake and steering
	// bits got -- treat as plausible, not confirmed to the same standard
	// as the rest of this file.
	//
	// RollRate ([ship+0x7a]) -- feeds Roll each frame (`roll += rollRate`,
	// then a self-decay on the sum: `roll -= roll>>3`). Confirmed session
	// 10: RollRate itself is driven directly by SteeringRate
	// (`rollRate += steeringRate/32`, then `rollRate -= rollRate/2`) --
	// this is WipEout 2097's real banking mechanic: turning the ship rolls
	// it into the turn, matching the "steering + airbrakes" combined
	// effect described in session 10 (the airbrake half is the
	// air-brake-differential yaw term in physics.go, not this field).
	// Resolves what was initially misread mid-session as "no writer found"
	// (a mistraced register turned out to be SteeringRate, not an
	// independent field). Not an externally-driven input like PitchRate;
	// IntegratePitchAndRoll both sets and integrates it.
	PitchRate, RollRate float32

	// TwistMargin, TwistDivisor -- NegCon twist-steering calibration
	// constants consumed by UpdateSteeringTwist (session 9). In the
	// original these are free-standing globals (data_80094d3c-derived
	// center fixed at 128, a margin table indexed by a per-ship-class
	// "curve" selection, and a divisor byte), not per-ship-struct fields --
	// modeled as Ship fields here for API consistency with
	// InertiaFactor/DragCoefficient. Confirmed live for the default case:
	// margin=6, divisor=255.
	TwistMargin, TwistDivisor float32

	// TrackProgress ([ship+0xb0]/[+0xb4]) -- cumulative distance-along-track
	// accumulator, written by maybe_UpdateShipRaceRankAndAI from a
	// per-section curve-distance table; used for race rank/position.
	TrackProgress int32
	LapDistance   int16

	// RankCounter ([ship+0xc6]/[+0xc7]) -- signed relative-position counters
	// between ship pairs, maintained by maybe_UpdateShipRaceRankAndAI.
	RankCounter [2]int8

	// RecoveryTimer ([ship+0xe0]) is set to 500 when the far-from-track
	// centerline test enters the original recovery callback state.
	RecoveryTimer int32
}

// ShipFlag bits with a well-established meaning, confirmed across multiple
// independently-examined functions (bn-psx/docs/wipeout2097_ship_physics_hunt.md).
// Bits not listed here exist and are tested throughout the original code but
// haven't been individually pinned down yet -- Flags preserves them all,
// this is deliberately not an exhaustive enum.
const (
	// ShipFlagCockpitCamera ([ship+0xc] bit 0x4) records the active camera
	// callback. ToggleShipCameraView (SLES_003.27 0x800663cc) sets it while
	// installing UpdateCockpitCameraView (0x800208fc), and clears it while
	// installing UpdateChaseCameraFollow (0x80020608).
	ShipFlagCockpitCamera uint32 = 0x4

	// ShipFlagFarFromTrackSection ([ship+0xc] bit 0x10) is set when the
	// weighted distance to the nearest searched TrackSection is at least
	// 3701, and cleared below that threshold.
	ShipFlagFarFromTrackSection uint32 = 0x10

	// ShipFlagTrackFaceSide ([ship+0xc] bit 0x20) is recomputed every frame
	// by UpdateShipOrientationVectorsAndTrackSide (0x800320d8). The physics
	// integrator uses it to choose between a section's paired driving faces.
	ShipFlagTrackFaceSide uint32 = 0x20

	// ShipFlagRetired marks a ship that has retired/reset -- set alongside
	// the effect-callback-chain reset pattern seen in
	// maybe_CheckShipRetireStatus, maybe_UpdateShipRaceRankAndAI, and
	// maybe_IntegrateShipPhysicsFromPadInput's out-of-bounds recovery path.
	ShipFlagRetired uint32 = 0x40

	// ShipFlagRecoveryState is the exact combined mask ORed into ship+0xc by
	// the 32001-unit far-from-track recovery path.
	ShipFlagRecoveryState uint32 = 0x401
)
