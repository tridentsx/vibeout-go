package game

// UpdatePhysics advances a ship's velocity and position by one frame,
// porting the confirmed core of maybe_IntegrateShipPhysicsFromPadInput
// (SLES_003.27 0x80066c7c, bn-psx/docs/wipeout2097_ship_physics_hunt.md
// session 7): each axis of velocity decays by 1/16 per frame (the original:
// `v -= v>>4`, a fixed-point right-shift; the float32 equivalent is a
// straight multiply, since there's no truncation semantics worth
// replicating), then position accumulates velocity scaled by 1/32 (the
// original: `position += velocity>>5`).
//
// This is deliberately only the confirmed drag-and-integrate core, not the
// full function -- steering input application, banking/lean, and the
// out-of-bounds recovery path are not yet decoded closely enough to port.
// Both the AI (maybe_RunShipAutopilot) and human (this function's namesake)
// paths funnel into this same shape; how a Ship's steering/thrust inputs
// should feed in here is still open pending further reverse engineering.
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
