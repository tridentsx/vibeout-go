package game

// The moving-object path system, ported from the cluster at 0x80067xxx in SLES_003.27.
// Retail uses it for the rescue craft; the machinery is generic, so this models it as a
// system with a slice of objects rather than as one craft.
//
// Every object carries its own timer and state function, and steers toward waypoints
// taken from the track's own sections -- those flagged SectionFlagPathStart. See
// bn-psx/docs and docs/open-items-animated-scenery.md for the derivation.
//
// A note on units. Retail works in fixed point: velocity decays with `v -= v >> 3`,
// position advances with `p += v >> 6`, and a unit heading vector arrives as
// `sin * cos >> 15` where sin and cos are 4096-scaled. Those become a 7/8 decay, a
// division by 64, and an acceleration magnitude of 4096*4096/32768 = 512 respectively.
// Keeping them as named constants rather than inlined arithmetic is what makes them
// checkable against the disassembly.

// MovingObjectStateID names the flight states. Retail stores a function pointer at
// entity+0x48; an enum is equivalent and avoids a self-referential type.
type MovingObjectStateID int

const (
	// MovingObjectIdle is not a retail state; it marks an object that has finished.
	MovingObjectIdle MovingObjectStateID = iota
	// MovingObjectHoverLaunch is the state maybe_InitMovingObjectPath installs: hold
	// station on the player, then climb away.
	MovingObjectHoverLaunch
	// MovingObjectSeekWaypoint steers toward the current waypoint.
	MovingObjectSeekWaypoint
)

// Timer values, all from maybe_MovingObjectFlightStateA and its handover.
const (
	// MovingObjectTimerStart is 0x320, loaded on entry and again at each handover.
	MovingObjectTimerStart = 0x320
	// MovingObjectImpulseTick is 0x302, where a one-off vertical impulse is applied.
	MovingObjectImpulseTick = 0x302
	// MovingObjectImpulse is the 0x5a subtracted from vertical acceleration there.
	MovingObjectImpulse = 0x5a
	// MovingObjectClimbBelow is 0x2c6: under this the object climbs and turns.
	MovingObjectClimbBelow = 0x2c6
	// MovingObjectHandoverAt is 0x1f4, where the hover state gives way to seeking.
	MovingObjectHandoverAt = 0x1f4
	// MovingObjectClimbAccel is the -0x8c written to vertical acceleration. Negative Y
	// is up, so this is a climb.
	MovingObjectClimbAccel = -0x8c
	// MovingObjectClimbYawRate is the -8 written to the yaw rate, a steady turn. This is
	// a yaw rate and not a pitch: the integrator applies it as `yaw += rate`. Angle is
	// unsigned here, so the negative is expressed as its wrapped equivalent -- adding
	// 4088 is subtracting 8.
	MovingObjectClimbYawRate = AngleFullTurn - 8
)

// Heights, both relative to a waypoint's own Y. Negative Y is up.
const (
	// MovingObjectSnapHeight is 0x1388, the height the object jumps to at handover.
	MovingObjectSnapHeight = 0x1388
	// MovingObjectSeekHeight is 0xbb8, the height it holds while seeking. The start
	// light gantries use the same offset, so it is the engine's "above the road" height.
	MovingObjectSeekHeight = 0xbb8
	// MovingObjectSpawnHeight is the 0xc8 above the player that
	// maybe_InitMovingObjectPath starts it at.
	MovingObjectSpawnHeight = 0xc8
)

// Integrator constants.
const (
	// movingObjectDecayShift is the 3 in `v -= v >> 3`, so velocity retains 7/8.
	movingObjectDecayShift = 3
	// movingObjectApplyShift is the 6 in `p += v >> 6`.
	movingObjectApplyShift = 6
	// movingObjectAccelSlow and movingObjectAccelFast are the magnitudes the two
	// heading-vector shifts produce: 4096*4096 >> 15 and >> 14.
	movingObjectAccelSlow = 512
	movingObjectAccelFast = 1024
	// movingObjectVerticalGain is the 6 in `accel.y = delta.y >> 6` while seeking.
	movingObjectVerticalGain = 6
)

// MovingObject is one object following a path. The field names follow the entity struct
// at maybe_MovingObjectState (0x800be420), whose offsets are in the docs.
type MovingObject struct {
	Position     Vector3
	Velocity     Vector3
	Acceleration Vector3
	Yaw          Angle
	Pitch        Angle
	Roll         Angle
	// YawRate is entity+0x3e, added to Yaw each update.
	YawRate Angle
	// Timer is entity+0x44.
	Timer int
	// State is entity+0x48.
	State MovingObjectStateID
	// Waypoint indexes the track section being steered toward.
	Waypoint int
	// PoolSlot is which prop-pool entry this drives; the rescue craft is 0x1e. Retail
	// keeps the equivalent at entity+0x04.
	PoolSlot int
}

// MovingObjectPathfinder supplies the track data the system needs, so this package does
// not depend on the asset loader.
type MovingObjectPathfinder interface {
	// SectionCount is how many sections the track has.
	SectionCount() int
	// SectionCenter is a section's centre position.
	SectionCenter(index int) Vector3
	// SectionNext is a section's Next link.
	SectionNext(index int) int
	// IsPathNode reports whether a section carries SectionFlagPathStart.
	IsPathNode(index int) bool
}

// FindFirstWaypoint reproduces maybe_InitMovingObjectPath's search: walk Next from
// section zero until a section carries the path flag, bounded by the section count.
//
// Two circuits -- Talon's Reach and Valparaiso -- have no flagged section at all, so the
// walk runs to its bound there and returns whatever it reached. That is retail
// behaviour, not a failure, and is why the mechanism looked dead when only Talon's Reach
// was examined.
func FindFirstWaypoint(track MovingObjectPathfinder) int {
	if track == nil || track.SectionCount() == 0 {
		return 0
	}
	section := 0
	for i := 0; i <= track.SectionCount(); i++ {
		next := track.SectionNext(section)
		if next < 0 || next >= track.SectionCount() {
			return section
		}
		section = next
		if track.IsPathNode(section) {
			return section
		}
	}
	return section
}

// SpawnMovingObject starts an object above a ship, as maybe_InitMovingObjectPath does.
func SpawnMovingObject(track MovingObjectPathfinder, ship *Ship, poolSlot int) *MovingObject {
	object := &MovingObject{
		Timer:    MovingObjectTimerStart,
		State:    MovingObjectHoverLaunch,
		PoolSlot: poolSlot,
		Waypoint: FindFirstWaypoint(track),
	}
	if ship != nil {
		object.Position = Vector3{
			X: ship.Position.X,
			Y: ship.Position.Y - MovingObjectSpawnHeight,
			Z: ship.Position.Z,
		}
		// Retail copies the ship's yaw and pitch, then immediately clears both, so the
		// object starts axis-aligned. Copying and clearing is preserved as a comment
		// rather than as code, since the net effect is zero.
	}
	return object
}

// Advance runs one tick: the current state sets acceleration, then the integrator
// applies it. Mirrors a flight state calling maybe_IntegrateMovingObjectPath.
func (o *MovingObject) Advance(track MovingObjectPathfinder, ship *Ship) {
	switch o.State {
	case MovingObjectHoverLaunch:
		o.hoverLaunch(track, ship)
	case MovingObjectSeekWaypoint:
		o.seekWaypoint(track)
	default:
		return
	}
	o.integrate()
}

// hoverLaunch is maybe_MovingObjectFlightStateA. The branch below 0x190 is unreachable,
// because the handover at 0x1f4 fires first, so it is not modelled.
func (o *MovingObject) hoverLaunch(track MovingObjectPathfinder, ship *Ship) {
	o.Timer--

	switch {
	case o.Timer == MovingObjectImpulseTick:
		o.Acceleration.Y -= MovingObjectImpulse
	case o.Timer < MovingObjectClimbBelow:
		// Climb and turn. The heading is frozen: this branch does not track the ship.
		sin, cos := o.Yaw.Sin(), o.Yaw.Cos()
		pitchCos := o.Pitch.Cos()
		o.Acceleration.X = -sin * pitchCos * movingObjectAccelSlow
		o.Acceleration.Z = cos * pitchCos * movingObjectAccelSlow
		o.Acceleration.Y = MovingObjectClimbAccel
		o.YawRate = MovingObjectClimbYawRate
	default:
		// Hold station: track the ship's heading.
		if ship != nil {
			o.Yaw = ship.Yaw
		}
	}

	if o.Timer < MovingObjectHandoverAt {
		o.handOverToSeek(track)
	}
}

// handOverToSeek resets the timer and snaps the object above its waypoint.
//
// The snap is never visible in play: the hover state runs 0x320 down to 0x1f4, three
// hundred ticks or twelve seconds, where a race countdown is only 166, so the object has
// left the screen well before this fires.
func (o *MovingObject) handOverToSeek(track MovingObjectPathfinder) {
	o.Timer = MovingObjectTimerStart
	o.State = MovingObjectSeekWaypoint
	if track == nil {
		return
	}
	centre := track.SectionCenter(o.Waypoint)
	o.Position = Vector3{
		X: centre.X,
		Y: centre.Y - MovingObjectSnapHeight,
		Z: centre.Z,
	}
}

// seekWaypoint is maybe_MovingObjectFlightStateB: steer toward the midpoint of the
// current waypoint and its Next, held MovingObjectSeekHeight above the track.
func (o *MovingObject) seekWaypoint(track MovingObjectPathfinder) {
	if track == nil {
		return
	}
	current := track.SectionCenter(o.Waypoint)
	nextIndex := track.SectionNext(o.Waypoint)
	next := current
	if nextIndex >= 0 && nextIndex < track.SectionCount() {
		next = track.SectionCenter(nextIndex)
	}
	target := Vector3{
		X: (current.X + next.X) / 2,
		Y: current.Y - MovingObjectSeekHeight,
		Z: (current.Z + next.Z) / 2,
	}
	delta := Vector3{
		X: target.X - o.Position.X,
		Y: target.Y - o.Position.Y,
		Z: target.Z - o.Position.Z,
	}

	// Retail derives the heading with ratan2 and then picks whichever of two candidate
	// turns is shorter, writing the result as a yaw rate. Steering toward the shorter
	// turn is the whole of that logic.
	// Retail derives the heading with ratan2, then picks whichever of two candidate turns
	// has the smaller magnitude and writes it as the yaw rate. The wrapped difference is
	// that shorter turn: as an unsigned Angle it carries the correct sign through
	// addition, since adding 4088 is subtracting 8.
	want := AngleFromDirection(int32(delta.X), int32(delta.Z))
	o.YawRate = (want - o.Yaw).Wrapped()

	sin, cos := o.Yaw.Sin(), o.Yaw.Cos()
	pitchCos := o.Pitch.Cos()
	o.Acceleration.X = -sin * pitchCos * movingObjectAccelSlow
	o.Acceleration.Z = cos * pitchCos * movingObjectAccelSlow
	// Vertical is a plain proportional term on the height error.
	o.Acceleration.Y = delta.Y / (1 << movingObjectVerticalGain)
}

// Release is the external trigger out of the seeking state: retail tests bit 0x400 of
// the ship's flag word at ship+0xc, and on seeing it resets the timer, plays a sound and
// swaps the camera callback. Only the state change belongs here.
func (o *MovingObject) Release() {
	if o.State != MovingObjectSeekWaypoint {
		return
	}
	o.Timer = MovingObjectTimerStart
	o.State = MovingObjectIdle
}

// integrate is maybe_IntegrateMovingObjectPath: accumulate, decay, apply, and advance the
// yaw by its rate.
func (o *MovingObject) integrate() {
	o.Velocity.X += o.Acceleration.X
	o.Velocity.Y += o.Acceleration.Y
	o.Velocity.Z += o.Acceleration.Z

	const retain = float32((1<<movingObjectDecayShift)-1) / (1 << movingObjectDecayShift)
	o.Velocity.X *= retain
	o.Velocity.Y *= retain
	o.Velocity.Z *= retain

	const apply = float32(1) / (1 << movingObjectApplyShift)
	o.Position.X += o.Velocity.X * apply
	o.Position.Y += o.Velocity.Y * apply
	o.Position.Z += o.Velocity.Z * apply

	o.Yaw = (o.Yaw + o.YawRate).Wrapped()
}
