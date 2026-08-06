package game

import "testing"

// fakeTrack is a straight run of sections with a settable path node, so the waypoint
// search and the seek can be exercised without loading a circuit.
type fakeTrack struct {
	count    int
	pathNode int // -1 for none, as Talon's Reach and Valparaiso have
	spacing  float32
}

func (f fakeTrack) SectionCount() int { return f.count }
func (f fakeTrack) SectionCenter(i int) Vector3 {
	return Vector3{X: 0, Y: 500, Z: float32(i) * f.spacing}
}
func (f fakeTrack) SectionNext(i int) int { return (i + 1) % f.count }
func (f fakeTrack) IsPathNode(i int) bool { return f.pathNode >= 0 && i == f.pathNode }

// The search walks Next from section zero and stops at the first flagged section.
func TestFindFirstWaypointStopsAtTheFlag(t *testing.T) {
	track := fakeTrack{count: 100, pathNode: 42, spacing: 1000}
	if got := FindFirstWaypoint(track); got != 42 {
		t.Errorf("found waypoint %d, want 42", got)
	}
}

// With no flagged section the walk runs to its bound rather than looping forever. That is
// retail behaviour on Talon's Reach and Valparaiso, which carry none.
func TestFindFirstWaypointTerminatesWithoutAFlag(t *testing.T) {
	track := fakeTrack{count: 50, pathNode: -1, spacing: 1000}
	got := FindFirstWaypoint(track)
	if got < 0 || got >= track.count {
		t.Errorf("returned %d, outside the section range", got)
	}
}

// An object starts above the ship, by the recovered 0xc8.
func TestSpawnPlacesTheObjectAboveTheShip(t *testing.T) {
	track := fakeTrack{count: 20, pathNode: 5, spacing: 1000}
	ship := &Ship{Position: Vector3{X: 100, Y: 500, Z: 200}}
	object := SpawnMovingObject(track, ship, 0x1e)
	if object.Position.X != 100 || object.Position.Z != 200 {
		t.Errorf("horizontal position is (%v,%v), want the ship's", object.Position.X, object.Position.Z)
	}
	// Negative Y is up, so above means a smaller Y.
	if object.Position.Y != 500-MovingObjectSpawnHeight {
		t.Errorf("Y is %v, want %v", object.Position.Y, 500-MovingObjectSpawnHeight)
	}
	if object.Timer != MovingObjectTimerStart {
		t.Errorf("timer is %d, want %#x", object.Timer, MovingObjectTimerStart)
	}
	if object.State != MovingObjectHoverLaunch {
		t.Error("must start in the hover state")
	}
	if object.PoolSlot != 0x1e {
		t.Errorf("pool slot is %d, want 0x1e", object.PoolSlot)
	}
}

// The hover state tracks the ship's heading until the climb begins.
func TestHoverTracksTheShipHeading(t *testing.T) {
	track := fakeTrack{count: 20, pathNode: 5, spacing: 1000}
	ship := &Ship{Position: Vector3{Y: 500}, Yaw: 1234}
	object := SpawnMovingObject(track, ship, 0x1e)
	object.Advance(track, ship)
	if object.Yaw.Wrapped() != Angle(1234).Wrapped() {
		t.Errorf("yaw is %d, want the ship's 1234", object.Yaw)
	}
	// Once past the climb threshold the heading is frozen, so changing the ship's yaw
	// must no longer move it.
	object.Timer = MovingObjectClimbBelow
	ship.Yaw = 2000
	object.Advance(track, ship)
	if object.Yaw.Wrapped() == Angle(2000).Wrapped() {
		t.Error("the climb phase must not track the ship")
	}
}

// The one-off impulse lands on exactly one tick.
func TestImpulseAppliesOnce(t *testing.T) {
	track := fakeTrack{count: 20, pathNode: 5, spacing: 1000}
	ship := &Ship{Position: Vector3{Y: 500}}
	object := SpawnMovingObject(track, ship, 0x1e)
	hits := 0
	for object.Timer > MovingObjectImpulseTick-3 && object.State == MovingObjectHoverLaunch {
		before := object.Acceleration.Y
		object.Advance(track, ship)
		if object.Acceleration.Y < before {
			hits++
		}
	}
	if hits != 1 {
		t.Errorf("the impulse applied %d times, want once", hits)
	}
}

// The climb has a negative vertical acceleration, which is upward, and a steady turn.
func TestClimbRisesAndTurns(t *testing.T) {
	track := fakeTrack{count: 20, pathNode: 5, spacing: 1000}
	ship := &Ship{Position: Vector3{Y: 500}}
	object := SpawnMovingObject(track, ship, 0x1e)
	object.Timer = MovingObjectClimbBelow
	startY := object.Position.Y
	startYaw := object.Yaw
	for i := 0; i < 40; i++ {
		object.Advance(track, ship)
	}
	if object.Position.Y >= startY {
		t.Errorf("Y went from %v to %v; negative Y is up so it must decrease", startY, object.Position.Y)
	}
	if object.Yaw == startYaw {
		t.Error("the yaw rate must turn the object")
	}
	if object.Acceleration.Y != MovingObjectClimbAccel {
		t.Errorf("climb acceleration is %v, want %d", object.Acceleration.Y, MovingObjectClimbAccel)
	}
}

// Handover resets the timer, switches state, and snaps above the waypoint. The snap is
// never visible in play because the hover state outlasts a race countdown.
func TestHandoverSnapsAboveTheWaypoint(t *testing.T) {
	track := fakeTrack{count: 20, pathNode: 5, spacing: 1000}
	ship := &Ship{Position: Vector3{Y: 500}}
	object := SpawnMovingObject(track, ship, 0x1e)
	object.Timer = MovingObjectHandoverAt
	object.Advance(track, ship)
	if object.State != MovingObjectSeekWaypoint {
		t.Fatalf("state is %v, want seeking", object.State)
	}
	if object.Timer != MovingObjectTimerStart {
		t.Errorf("timer is %d, want it reset to %#x", object.Timer, MovingObjectTimerStart)
	}
	centre := track.SectionCenter(object.Waypoint)
	// The integrator runs after the snap, so allow for one tick of drift.
	if got := object.Position.Y; got > centre.Y-MovingObjectSnapHeight+50 {
		t.Errorf("Y is %v, want about %v", got, centre.Y-MovingObjectSnapHeight)
	}
}

// The hover state must last longer than a race countdown, which is what keeps the snap
// off screen.
func TestHoverOutlastsTheCountdown(t *testing.T) {
	ticks := MovingObjectTimerStart - MovingObjectHandoverAt
	if ticks <= CountdownStart {
		t.Errorf("the hover lasts %d ticks against a %d-tick countdown; the snap would be visible",
			ticks, CountdownStart)
	}
	if seconds := float64(ticks) / TicksPerSecond; seconds < 10 {
		t.Errorf("the hover lasts %.1f s, expected about twelve", seconds)
	}
}

// Seeking steers the object toward its waypoint height, approaching it monotonically.
//
// The vertical term is a plain proportional controller, `accel.y = delta.y >> 6`, against
// the integrator's 7/8 decay. Steady state is v = 7a, so the approach is asymptotic and
// deliberately unhurried -- it does not arrive within a few hundred ticks, and asserting
// arrival would be asserting the wrong thing. What matters is that it closes the gap and
// never overshoots away from the target.
func TestSeekApproachesTheWaypointHeight(t *testing.T) {
	track := fakeTrack{count: 20, pathNode: 5, spacing: 1000}
	object := &MovingObject{
		State:    MovingObjectSeekWaypoint,
		Timer:    MovingObjectTimerStart,
		Waypoint: 5,
		Position: Vector3{X: 0, Y: 500, Z: 5000},
	}
	want := track.SectionCenter(5).Y - MovingObjectSeekHeight
	previous := object.Position.Y - want
	if previous < 0 {
		previous = -previous
	}
	for i := 0; i < 600; i++ {
		object.Advance(track, nil)
		gap := object.Position.Y - want
		if gap < 0 {
			gap = -gap
		}
		if gap > previous+1 {
			t.Fatalf("tick %d: the gap grew from %v to %v", i, previous, gap)
		}
		previous = gap
	}
	// It should have closed a substantial part of the distance.
	if previous > 2000 {
		t.Errorf("after 600 ticks the gap is still %v; the controller is too weak", previous)
	}
}

// The release trigger takes the object out of the seeking state.
func TestReleaseLeavesSeeking(t *testing.T) {
	object := &MovingObject{State: MovingObjectSeekWaypoint}
	object.Release()
	if object.State != MovingObjectIdle {
		t.Errorf("state is %v after release", object.State)
	}
	// It must do nothing in any other state.
	hover := &MovingObject{State: MovingObjectHoverLaunch}
	hover.Release()
	if hover.State != MovingObjectHoverLaunch {
		t.Error("release must only apply while seeking")
	}
}

// The integrator's decay must remove energy, or an object never settles.
func TestIntegratorDecays(t *testing.T) {
	object := &MovingObject{Velocity: Vector3{X: 1000, Y: 1000, Z: 1000}}
	first := object.Velocity.X
	object.integrate()
	second := object.Velocity.X
	if second >= first {
		t.Fatalf("velocity went %v -> %v, it must decay", first, second)
	}
	// 7/8 per tick, from `v -= v >> 3`.
	if ratio := second / first; ratio < 0.87 || ratio > 0.88 {
		t.Errorf("decay ratio is %v, want 7/8", ratio)
	}
}
