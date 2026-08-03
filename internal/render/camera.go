package render

import "github.com/tridentsx/wipeout-go/internal/game"

// Camera is this project's viewpoint for rendering. Position/orientation
// only, no projection math -- how it's applied to a given renderer is that
// renderer's concern.
type Camera struct {
	Position game.Vector3
	Yaw      game.Angle // heading the camera is looking along, XZ plane
}

// PLACEHOLDER CAMERA -- NOT REVERSE-ENGINEERED. See TODO.md "Camera system"
// for the open item. The real WipEout 2097 binary computes a per-object
// camera-relative transform matrix each frame (confirmed via bn-psx:
// maybe_TransformAndSubmitPolygons loads a precomputed matrix from
// [drawObject+0x30] straight into the GTE before gte_rtps() -- a standard
// PS1 pattern of combining camera and object world transforms once per
// object, not per-vertex), populated from a draw-list table
// (SLES_003.27 0x800f6f24) whose own population site was not traced far
// enough to recover the actual camera position/orientation formula (where
// it sits relative to the ship, how roll/pitch influence it, etc.).
//
// NewChaseCamera is a standard third-person "chase cam" stand-in (behind
// and above the ship, looking along its heading) used only so the port has
// *a* camera to render from while that RE work is still open. Do not treat
// any constant or behavior in this function as derived from the original
// game -- replace this function's body, not just its constants, once the
// real formula is found.
func NewChaseCamera(ship *game.Ship) Camera {
	const followDistance = 40.0
	const followHeight = 12.0

	forward := game.Vector3{X: ship.Yaw.Sin(), Z: ship.Yaw.Cos()}
	return Camera{
		Position: game.Vector3{
			X: ship.Position.X - forward.X*followDistance,
			Y: ship.Position.Y + followHeight,
			Z: ship.Position.Z - forward.Z*followDistance,
		},
		Yaw: ship.Yaw,
	}
}

// ProjectTopDown rotates and translates a world point into camera-relative
// XZ space, so the camera's heading always faces "up" (increasing Z)
// regardless of which way it's actually turned. This is the flat top-down
// stand-in used by the current line-wireframe smoke-test renderer -- not
// the real GPU-API perspective projection from TODO.md's rendering
// approach, which will replace this entirely, camera formula or not.
func (c Camera) ProjectTopDown(p game.Vector3) (x, z float32) {
	dx := p.X - c.Position.X
	dz := p.Z - c.Position.Z
	sin, cos := c.Yaw.Sin(), c.Yaw.Cos()
	x = dx*cos - dz*sin
	z = dx*sin + dz*cos
	return x, z
}
