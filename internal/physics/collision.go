package physics

import (
	"math"

	"github.com/tridentsx/wipeout-go/internal/assets"
)

// PlaneDistance ports the confirmed plane-distance primitive `sub_8003598c`
// (SLES_003.27 0x8003598c, bn-psx/docs/wipeout2097_ship_physics_hunt.md
// session 10), used throughout the original's wall/collision detection
// code (maybe_DetectShipWallCollision and friends, plus the air-brake ramp
// system's own collision checks) to test a point against a track face's
// plane.
//
// The original: `dot(point-planePoint, normal) / (-|normal|^2 >> 12)`,
// where normal is a Q12 fixed-point vector (matching WO_TrackFace's raw
// int16 normal components -- a pre-normalized unit vector scaled by
// 4096). For a unit-length normal in Q12, `-|normal|^2 >> 12` is
// approximately -4096, so the division both cancels normal's Q12 scale
// and negates the sign. This port takes a true float32 unit normal (no
// Q12), so the direct equivalent is simply the negated dot product -- no
// scale-correction division needed, consistent with this project's
// established convention of dropping Q12 arithmetic once a vector is a
// true unit float (see physics.go's Forward-vector handling for the same
// pattern).
//
// Positive result means point is on the side the normal points *away*
// from (behind the plane, from the normal's perspective); negative means
// point is out in front of it, in the normal's direction -- confirmed by
// the sign flip in the original's division (dividing by a negative
// number). Not independently verified against which side is "outside the
// track" for a real WO_TrackFace -- that depends on the original data's
// own normal-direction convention, not something this formula alone
// determines.
func PlaneDistance(point, planePoint, normal Vector3) float32 {
	dx := point.X - planePoint.X
	dy := point.Y - planePoint.Y
	dz := point.Z - planePoint.Z
	return -(dx*normal.X + dy*normal.Y + dz*normal.Z)
}

// q12Scale is the fixed-point scale of a WO_TrackFace's raw int16 normal
// components (a pre-normalized unit vector stored as Q12, i.e. *4096) --
// the same convention as every other Q12 quantity ported this session.
const q12Scale = 4096.0

// FaceNormal converts a psx.TrackFace's raw Q12 normal into a true
// float32 unit vector, for use with PlaneDistance.
func FaceNormal(f assets.TrackFace) Vector3 {
	return Vector3{
		X: float32(f.NormalX) / q12Scale,
		Y: float32(f.NormalY) / q12Scale,
		Z: float32(f.NormalZ) / q12Scale,
	}
}

// faceVertex0 returns a TrackFace's first vertex as a game Vector3,
// converting from psx.TrackVertex's raw int32 world-space coordinates.
func faceVertex0(f assets.TrackFace, verts []assets.TrackVertex) Vector3 {
	v := verts[f.Indices[0]]
	return Vector3{X: float32(v.X), Y: float32(v.Y), Z: float32(v.Z)}
}

// isWallFace reports whether f should be treated as a wall/boundary for
// collision purposes. psx.TrackFaceWall is 0 -- not an active bit, but
// the absence of psx.TrackFaceTrack -- matching the original data's own
// convention that non-drivable faces simply aren't flagged as track
// surface, confirmed via psx/track.go's existing flag definitions (ported
// from wipeout.js, independently corroborated by this session's own
// WO_TrackFaceFlags struct read from the binary).
func isWallFace(f assets.TrackFace) bool {
	return f.Flags&assets.TrackFaceTrack == 0
}

// NearestWallDistance finds the minimum PlaneDistance among all
// wall-flagged faces in the given TrackSection's face range
// (FirstFace..FirstFace+NumFaces into faces), testing point against each
// face's plane (its first vertex as the plane point, its normal). Returns
// the winning face's index into faces and its signed distance; ok is
// false if the section has no wall faces or its face range is invalid.
//
// Ports the face-list-walking shape common to
// maybe_DetectShipWallCollision and sub_80033c1c (both iterate a
// section's face range testing each candidate against the ship's
// position), simplified to a single nearest-wall query rather than the
// original's fuller multi-sensor-point sweep (session 8's "front/side
// sensor" ray pattern in sub_80033c1c, using multiple offset test points
// per frame, not just the ship's center) -- that richer sensor logic and
// the resulting bounce-impulse response are NOT ported here. This is
// deliberately just the core "how far is point from the nearest wall
// plane" geometric primitive, a foundation for a later bounce-response
// pass, not a complete collision system.
// WallBounceImpulse ports the confirmed core of `maybe_ApplyWallGrazeImpulse`
// (SLES_003.27 0x800351a4, bn-psx/docs/wipeout2097_ship_physics_hunt.md
// session 11): the lightest of the original's three wall-contact response
// variants, a direct velocity nudge with no position correction.
//
// The original adds the colliding face's raw Q12 normal components straight
// into the ship's int32 velocity (`velocity.axis += rawQ12Normal.axis`), with
// the Y component additionally halved (`(int16)normalY >> 1`, via a
// `zx.d;<<0x10;s>>0x11` compiler idiom that's functionally a sign-extend
// followed by an arithmetic right shift, not a real zero-extend -- verified
// this session by cross-checking the same face-normal offsets, +8/+0xa/+0xc,
// against psx/track.go's independently-ported .TRF decoder). Since this
// port's FaceNormal returns a true unit vector (already divided by Q12's
// 4096 scale), the original's un-shifted raw-Q12 add is reproduced here by
// multiplying back by q12Scale -- dropping that factor would silently be
// off by 4096x, the same class of correction documented in physics.go's
// IntegrateShipPhysics.
//
// Not ported: the original's further branch on `ship+0xac & 2` (an SFX/
// particle trigger vs. an unrelated rate-limiter global) -- unconfirmed
// gameplay-visible effect, not a physics quantity.
func WallBounceImpulse(velocity Vector3, normal Vector3) Vector3 {
	return Vector3{
		X: velocity.X + normal.X*q12Scale,
		Y: velocity.Y + normal.Y*q12Scale/2,
		Z: velocity.Z + normal.Z*q12Scale,
	}
}

// WallCollisionResponse ports the confirmed core of
// `maybe_ApplyWallCollisionResponse` (SLES_003.27 0x80035384,
// bn-psx/docs/wipeout2097_ship_physics_hunt.md session 11): the fuller of
// the original's wall-contact responses, branching on how deep the ship
// penetrated the wall.
//
// `penetrating` selects the branch actually taken by the original's `a3 s>=
// 0` (shallow/grazing) vs. `a3 s< 0` (deep, a genuine wall ram) test. The
// original derives that `a3` from the same `sub_8003598c`/PlaneDistance-
// shaped call this project already ported, but not always as its direct,
// unnegated return value (one call site passes it straight through, another
// negates it first) -- which exact sign convention a caller must reconstruct
// from NearestWallDistance's own output is deliberately left to the caller
// here rather than guessed at, per this project's standing rule against
// porting unconfirmed wiring (see UpdateAirBrakes's button-bit mapping for
// the same kind of explicit gap).
//
// Shallow: velocity gets the same faceNormal add as WallBounceImpulse but at
// double strength on X/Z (Y still only single-strength/2, matching the
// original's asymmetric shift), and position is nudged by velocity/64 -- the
// same integration scale IntegrateShipPhysics itself uses for position.
//
// Deep (a genuine wall ram): velocity is replaced outright, per axis:
// `v_axis = (v_axis / depth) / 32` (the original's `>>5` after a full
// zero-then-add sequence) -- a hard brake-and-redirect. `depth` must be the
// (positive) penetration magnitude; callers should pass e.g. `-distance`
// from NearestWallDistance if that convention holds, though this wasn't
// independently re-verified this session (see the `penetrating` doc above).
//
// Not ported: the wall-impact SFX dispatch, and the original's own
// SteeringRate/SpeedMagnitude interaction (`ship+0x76 +=/-= f(ship+0x94)`,
// a rate constant added to the ship's existing SteeringRate field, scaled by
// its SpeedMagnitude -- i.e. a hard hit also kicks the ship's turn-rate
// accumulator, read as a spin-out effect layered on top of the velocity
// change) -- a real, confirmed part of the original's behavior, left for a
// follow-up pass once SteeringRate's own integration path can absorb an
// external kick cleanly.
func WallCollisionResponse(position, velocity Vector3, normal Vector3, penetrating bool, depth float32) (newPosition, newVelocity Vector3) {
	if !penetrating {
		newVelocity = Vector3{
			X: velocity.X + normal.X*q12Scale*2,
			Y: velocity.Y + normal.Y*q12Scale/2,
			Z: velocity.Z + normal.Z*q12Scale,
		}
		newPosition = Vector3{
			X: position.X + newVelocity.X/64,
			Y: position.Y + newVelocity.Y/64,
			Z: position.Z + newVelocity.Z/64,
		}
		return newPosition, newVelocity
	}

	newVelocity = Vector3{
		X: (velocity.X / depth) / 32,
		Y: (velocity.Y / depth) / 32,
		Z: (velocity.Z / depth) / 32,
	}
	return position, newVelocity
}

func NearestWallDistance(point Vector3, section assets.TrackSection, faces []assets.TrackFace, verts []assets.TrackVertex) (faceIndex int, distance float32, ok bool) {
	start := int(section.FirstFace)
	end := start + int(section.NumFaces)
	if start < 0 || end > len(faces) {
		return 0, 0, false
	}

	bestIdx := -1
	var best float32
	for i := start; i < end; i++ {
		f := faces[i]
		if !isWallFace(f) {
			continue
		}
		d := PlaneDistance(point, faceVertex0(f, verts), FaceNormal(f))
		if bestIdx == -1 || d < best {
			best = d
			bestIdx = i
		}
	}
	if bestIdx == -1 {
		return 0, 0, false
	}
	return bestIdx, best, true
}

// AngleBetweenVectors ports the confirmed shape of `maybe_AngleBetweenVectors`
// (SLES_003.27 0x800252e8, bn-psx/docs/wipeout2097_ship_physics_hunt.md
// session 12): `acos(dot(a,b) / (|a|*|b|))`, clamped to a valid cosine range
// (the original clamps its Q12 cosine to [-4096,4096] before an acos-shaped
// call, `sub_800255b0`, never independently traced -- this port uses Go's
// real acos directly rather than replicate that call's exact fixed-point
// table/scale, matching this project's established convention of dropping
// Q12 bookkeeping once a quantity is a true float32).
//
// Returns 0 for a zero-length input rather than the original's signed-divide
// overflow trap (`trap(0x5d)`) -- that trap is a MIPS compiler safety net
// for a case that shouldn't arise given real corner geometry, not a
// deliberate behavior to replicate.
func AngleBetweenVectors(a, b Vector3) float32 {
	dot := a.X*b.X + a.Y*b.Y + a.Z*b.Z
	magA := float32(math.Sqrt(float64(a.X*a.X + a.Y*a.Y + a.Z*a.Z)))
	magB := float32(math.Sqrt(float64(b.X*b.X + b.Y*b.Y + b.Z*b.Z)))
	if magA == 0 || magB == 0 {
		return 0
	}

	cos := dot / (magA * magB)
	switch {
	case cos > 1:
		cos = 1
	case cos < -1:
		cos = -1
	}
	return float32(math.Acos(float64(cos)))
}

// pointInsideFaceTolerance is this port's own slack around a full turn
// (2*Pi), standing in for the original's fixed-point threshold (`0x7531`
// against an unconfirmed full-turn scale in the real `maybe_TestPointInsideFace`)
// -- an engineering choice, not a literally ported constant, the same class
// of gap as PlaneDistance's Q12-to-float scale drop.
const pointInsideFaceTolerance = 0.01 // radians

// PointInsideFace ports the confirmed shape of `maybe_TestPointInsideFace`
// (SLES_003.27 0x80035c28, bn-psx/docs/wipeout2097_ship_physics_hunt.md
// session 12): the real per-face hit test `maybe_ResolveShipWallCollision`
// uses to decide whether a sensor point genuinely touches a candidate wall
// face, not merely its infinite plane -- a real gap `NearestWallDistance`
// alone doesn't cover (it only tests the nearest plane distance, never
// whether the point falls inside that face's actual polygon).
//
// point should already be projected onto the face's plane (see
// PlaneDistance/FaceNormal) before calling this -- the original folds that
// projection into the same function; this port keeps them separate so
// PlaneDistance can be reused as-is for both the nearest-wall query and this
// finer-grained test.
//
// Implements the classic winding-angle point-in-polygon test: sums the angle
// subtended at point by each pair of adjacent corners (in Indices order,
// wrapping around); a point strictly inside the face's corners subtends a
// full turn, a point outside subtends less. Handles triangles
// (psx.TrackFace's Indices[3]==Indices[2] convention) and quads uniformly by
// summing however many distinct adjacent corner pairs the face actually has,
// rather than porting the original's own workaround for the degenerate-
// triangle case (reading a neighboring face record's corner data when its
// own 4th corner repeats the 3rd, per the hunt doc's session 12 notes) --
// summing 3 vs. 4 angles converges on the same "close to a full turn" test
// either way, without needing a neighboring face's data at all.
func PointInsideFace(point Vector3, f assets.TrackFace, verts []assets.TrackVertex) bool {
	corners := f.Indices[:]
	if f.Indices[3] == f.Indices[2] {
		corners = f.Indices[:3]
	}

	var sum float32
	for i := range corners {
		a := verts[corners[i]]
		b := verts[corners[(i+1)%len(corners)]]
		da := Vector3{X: point.X - float32(a.X), Y: point.Y - float32(a.Y), Z: point.Z - float32(a.Z)}
		db := Vector3{X: point.X - float32(b.X), Y: point.Y - float32(b.Y), Z: point.Z - float32(b.Z)}
		sum += AngleBetweenVectors(da, db)
	}

	return sum >= float32(2*math.Pi-pointInsideFaceTolerance)
}

// ResolveShipWallCollision composes NearestWallDistance, PointInsideFace, and
// WallCollisionResponse into a single per-frame check, mirroring what
// `maybe_ResolveShipWallCollision` (SLES_003.27 0x80033c1c) does for real,
// with deliberate simplifications flagged below -- this is a best-effort
// composition of the confirmed primitives, not a full port of the original's
// own dispatch tree (see bn-psx/docs/wipeout2097_ship_physics_hunt.md
// sessions 11-12 for what that tree actually looks like).
//
// Reports whether a collision was resolved (velocity/position updated).
//
// # Simplifications, each a deliberate engineering choice, not an oversight
//
//   - Single sensor point (the ship's own Position) rather than the
//     original's multiple front/side probes across several refinement
//     passes (session 8's "front/side sensor" notes, session 12's repeated
//     shift-scaled offset recomputation blocks) -- porting that sweep needs
//     the exact offset/shift wiring pinned down first, not yet done.
//   - Always uses the fuller WallCollisionResponse, never
//     WallBounceImpulse/maybe_ApplyWallGrazeImpulse or the ship-vs-ship
//     maybe_ApplyShipCollisionResponse -- the original's choice between
//     these three is gated by an unconfirmed section-level flag
//     (`section+0x94 & 0x180000`, session 12) this port doesn't have wired
//     to anything yet.
//   - `penetrating`/`depth` are derived directly from PlaneDistance's own
//     confirmed sign convention (positive = crossed to the wall side of the
//     face, matching PlaneDistanceSign's test) rather than reverse-engineered
//     from the original's specific dispatch-site register wiring (`a3`),
//     which threads through a multi-stage nested refinement this project
//     hasn't fully traced yet (see session 11's same caveat on
//     WallCollisionResponse's own doc comment).
//   - The point-plane projection PointInsideFace needs
//     (`point + normal*distance`) is this port's own geometrically-verified
//     formula for its PlaneDistance sign convention, not a byte-for-byte
//     translation of the original's projection instructions (whose exact
//     input sign wasn't independently re-confirmed this session).
//
// contactDistance is this port's own proximity gate, not a ported constant:
// NearestWallDistance always returns *some* nearest wall and PointInsideFace
// only tests whether the ship's *projection* falls inside that wall's
// footprint, regardless of how far away along the normal the ship actually
// is -- without a proximity check, a ship anywhere on a straight track would
// "collide" with a distant boundary wall just because its perpendicular
// projection lands inside that wall panel. A contact only counts when
// `distance > -contactDistance` (per PlaneDistance's sign convention: more
// negative is further from the wall on the safe side), which also covers
// the already-penetrating case (`distance > 0`) automatically. The original
// almost certainly has its own such threshold (or reaches this code at all
// only for a short list of nearby candidate faces, e.g. via section-local
// spatial binning) but it wasn't traced this session -- callers should treat
// this parameter as a real, uncalibrated gap, not a confirmed value.
func ResolveShipWallCollision(s *Ship, section assets.TrackSection, faces []assets.TrackFace, verts []assets.TrackVertex, contactDistance float32) bool {
	faceIdx, distance, ok := NearestWallDistance(s.Position, section, faces, verts)
	if !ok || distance <= -contactDistance {
		return false
	}

	f := faces[faceIdx]
	normal := FaceNormal(f)
	projected := Vector3{
		X: s.Position.X + normal.X*distance,
		Y: s.Position.Y + normal.Y*distance,
		Z: s.Position.Z + normal.Z*distance,
	}
	if !PointInsideFace(projected, f, verts) {
		return false
	}

	s.Position, s.Velocity = WallCollisionResponse(s.Position, s.Velocity, normal, distance > 0, distance)
	return true
}
