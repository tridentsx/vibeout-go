package physics

import (
	"fmt"
	"math"

	"github.com/tridentsx/wipeout-go/internal/assets"
)

const trackSectionGrazeOnlyMask uint32 = 0x180000

const wallSensorOrientationScale = 1.0 / 16.0

// WallSensorEdge returns the two ship-corner probes used by
// ResolveShipWallSensorCollisions. The executable forms them from ship
// position and the Q12 Forward/Right rows at +0x10 and +0x20, each shifted by
// four. In this port those rows are unit vectors, hence the corresponding
// world-space extent is 4096/16 = 256 units.
//
// subtractForward selects the exact sign used by the first half of the
// routine (0x80034410-0x80034610). The direction-mirrored half uses the
// opposite sign (0x80034d30-0x80034f30). Naming this parameter after the
// arithmetic, rather than guessing that either half always means "front",
// keeps the confirmed geometry separate from the still-unported section
// traversal decision.
func WallSensorEdge(position, forward, right Vector3, subtractForward bool) [2]Vector3 {
	extent := float32(q12Scale * wallSensorOrientationScale)
	forwardSign := float32(1)
	if subtractForward {
		forwardSign = -1
	}
	base := Vector3{
		X: position.X + forward.X*extent*forwardSign,
		Y: position.Y + forward.Y*extent*forwardSign,
		Z: position.Z + forward.Z*extent*forwardSign,
	}
	return [2]Vector3{
		{X: base.X - right.X*extent, Y: base.Y - right.Y*extent, Z: base.Z - right.Z*extent},
		{X: base.X + right.X*extent, Y: base.Y + right.Y*extent, Z: base.Z + right.Z*extent},
	}
}

// SectionWallSweep selects the ordered wall-face run used by
// ResolveShipWallSensorCollisions before it evaluates its probes. The face
// list is laid out as a wall run, a driving-surface run (flag bit 1), then a
// second wall run; TRACK01's ordinary sections are the canonical
// [wall, track, track, wall] case.
//
// The executable chooses the run with
// dot(sectionCenter-shipPosition, drivingFace.vertex0-drivingFace.vertex1).
// A positive dot scans the wall prefix up to the first track face
// (0x80033ed8-0x800346e4). A non-positive dot skips the prefix and the
// contiguous track run, then scans the suffix (0x8003471c-0x80034ff0).
// This is also the source of ship flag 0x20 already represented by
// UpdateShipTrackFaceSide.
func SectionWallSweep(position Vector3, section assets.TrackSection, drivingFace assets.TrackFace, faces []assets.TrackFace, verts []assets.TrackVertex) ([]int, bool) {
	start := int(section.FirstFace)
	end := start + int(section.NumFaces)
	if start < 0 || end > len(faces) || int(drivingFace.Indices[0]) >= len(verts) || int(drivingFace.Indices[1]) >= len(verts) {
		return nil, false
	}

	v0 := verts[drivingFace.Indices[0]]
	v1 := verts[drivingFace.Indices[1]]
	sideDot := (float32(section.X)-position.X)*float32(v0.X-v1.X) +
		(float32(section.Y)-position.Y)*float32(v0.Y-v1.Y) +
		(float32(section.Z)-position.Z)*float32(v0.Z-v1.Z)

	if sideDot > 0 {
		stop := start
		for stop < end && isWallFace(faces[stop]) {
			stop++
		}
		return faceIndexRange(start, stop), true
	}

	i := start
	for i < end && isWallFace(faces[i]) {
		i++
	}
	for i < end && !isWallFace(faces[i]) {
		i++
	}
	return faceIndexRange(i, end), true
}

func faceIndexRange(start, end int) []int {
	indices := make([]int, end-start)
	for i := range indices {
		indices[i] = start + i
	}
	return indices
}

// WallFaceSensorSample contains the three signed plane distances for one face
// at a single immutable ship pose. Nose is always 512 units along Forward;
// Edge contains the two corners selected by the ship's side of the driving
// strip. The original recomputes each edge from the possibly response-adjusted
// position, so this type is a detection/debug snapshot, not a substitute for
// the eventual sequential mutating resolver.
type WallFaceSensorSample struct {
	FaceIndex int
	Nose      float32
	Edge      [2]float32
}

// WallNoseContact records one accepted hard-response dispatch from the nose
// phase. CandidateFace supplied PlaneDistance; ResponseFace is the possibly
// substituted junction-neighbor face passed to the response helper.
type WallNoseContact struct {
	CandidateFace int
	ResponseFace  int
	Distance      float32
}

// SampleSectionWallSensors composes the executable's confirmed face-run and
// probe geometry without applying a response. Junction sections can replace
// the response face after containment tests, so response dispatch remains a
// separate step until that fallback tree is completely recovered.
func SampleSectionWallSensors(position, forward, right Vector3, section assets.TrackSection, drivingFace assets.TrackFace, faces []assets.TrackFace, verts []assets.TrackVertex) ([]WallFaceSensorSample, bool) {
	indices, ok := SectionWallSweep(position, section, drivingFace, faces, verts)
	if !ok {
		return nil, false
	}

	v0 := verts[drivingFace.Indices[0]]
	v1 := verts[drivingFace.Indices[1]]
	sideDot := (float32(section.X)-position.X)*float32(v0.X-v1.X) +
		(float32(section.Y)-position.Y)*float32(v0.Y-v1.Y) +
		(float32(section.Z)-position.Z)*float32(v0.Z-v1.Z)
	edge := WallSensorEdge(position, forward, right, sideDot > 0)
	nose := Vector3{X: position.X + forward.X*512, Y: position.Y + forward.Y*512, Z: position.Z + forward.Z*512}

	samples := make([]WallFaceSensorSample, 0, len(indices))
	for _, faceIndex := range indices {
		face := faces[faceIndex]
		for _, vertexIndex := range face.Indices {
			if int(vertexIndex) >= len(verts) {
				return nil, false
			}
		}
		planePoint := faceVertex0(face, verts)
		normal := FaceNormal(face)
		samples = append(samples, WallFaceSensorSample{
			FaceIndex: faceIndex,
			Nose:      PlaneDistance(nose, planePoint, normal),
			Edge: [2]float32{
				PlaneDistance(edge[0], planePoint, normal),
				PlaneDistance(edge[1], planePoint, normal),
			},
		})
	}
	return samples, true
}

// SelectPrefixNoseResponseFace ports the junction-aware containment tree at
// 0x80033fe8-0x80034404. It runs after the prefix-side nose probe has crossed
// a candidate wall plane. The returned index is the face passed to
// ApplyHardWallCollisionResponse; false means this candidate is rejected.
//
// PointInsideTrackFace in the executable receives the original candidate's
// plane distance even when testing a neighboring section's first face. This
// helper intentionally preserves that detail rather than recomputing a new
// distance for the fallback face.
func SelectPrefixNoseResponseFace(point Vector3, planeDistance float32, currentSection, candidateFace int, sections []assets.TrackSection, faces []assets.TrackFace, verts []assets.TrackVertex) (int, bool) {
	if currentSection < 0 || currentSection >= len(sections) || candidateFace < 0 || candidateFace >= len(faces) {
		return 0, false
	}
	c := sections[currentSection]
	inside := func(faceIndex int) bool {
		if faceIndex < 0 || faceIndex >= len(faces) {
			return false
		}
		face := faces[faceIndex]
		for _, vertexIndex := range face.Indices {
			if int(vertexIndex) >= len(verts) {
				return false
			}
		}
		n := FaceNormal(face)
		projected := Vector3{X: point.X - n.X*planeDistance, Y: point.Y - n.Y*planeDistance, Z: point.Z - n.Z*planeDistance}
		return PointInsideFace(projected, face, verts)
	}
	firstFace := func(sectionIndex int32) (int, bool) {
		if sectionIndex < 0 || int(sectionIndex) >= len(sections) {
			return 0, false
		}
		index := int(sections[sectionIndex].FirstFace)
		return index, index >= 0 && index < len(faces)
	}
	sectionFlag := func(sectionIndex int32, flag uint16) bool {
		return sectionIndex >= 0 && int(sectionIndex) < len(sections) && sections[sectionIndex].Flags&flag != 0
	}

	if c.Flags&assets.TrackSectionJunctionStart != 0 {
		if inside(candidateFace) {
			return candidateFace, true
		}
		if nextFace, ok := firstFace(c.Next); ok && inside(nextFace) {
			return candidateFace, true
		}
		return 0, false
	}

	if c.Previous >= 0 && int(c.Previous) < len(sections) {
		previousJunction := sections[c.Previous].NextJunction
		if sectionFlag(previousJunction, assets.TrackSectionJunctionStart) {
			if inside(candidateFace) {
				return candidateFace, true
			}
			if nextFace, ok := firstFace(c.Next); ok && inside(nextFace) {
				return nextFace, true
			}
			if previousFace, ok := firstFace(c.Previous); ok && inside(previousFace) {
				return previousFace, true
			}
			return 0, false
		}
	}

	if c.Next >= 0 && int(c.Next) < len(sections) {
		nextJunction := sections[c.Next].NextJunction
		if sectionFlag(nextJunction, assets.TrackSectionJunctionStart) {
			if inside(candidateFace) {
				return candidateFace, true
			}
			if previousFace, ok := firstFace(c.Previous); ok && inside(previousFace) {
				return previousFace, true
			}
			if junctionFace, ok := firstFace(nextJunction); ok && inside(junctionFace) {
				return junctionFace, true
			}
			return 0, false
		}
	}

	if c.Flags&assets.TrackSectionJunctionEnd != 0 {
		previousFace, hasPreviousFace := firstFace(c.Previous)
		if inside(candidateFace) || hasPreviousFace && inside(previousFace) {
			return candidateFace, true
		}
		return 0, false
	}

	if c.Next < 0 || int(c.Next) >= len(sections) {
		return candidateFace, true
	}
	nextJunction := sections[c.Next].NextJunction
	if !sectionFlag(nextJunction, assets.TrackSectionJunctionEnd) {
		return candidateFace, true
	}
	if inside(candidateFace) {
		return candidateFace, true
	}
	if previousFace, ok := firstFace(c.Previous); ok && inside(previousFace) {
		return previousFace, true
	}
	return 0, false
}

// SelectSuffixNoseResponseFace ports the mirrored containment tree at
// 0x80034870-0x80034d24. Neighboring sections use FirstFace+3 here, matching
// the executable's fixed right-wall slot, rather than the prefix tree's
// FirstFace. The response-face choices are intentionally not forced into
// artificial symmetry: at several junction fallbacks this half passes the
// successful neighbor face to ApplyHardWallCollisionResponse.
func SelectSuffixNoseResponseFace(point Vector3, planeDistance float32, currentSection, candidateFace int, sections []assets.TrackSection, faces []assets.TrackFace, verts []assets.TrackVertex) (int, bool) {
	if currentSection < 0 || currentSection >= len(sections) || candidateFace < 0 || candidateFace >= len(faces) {
		return 0, false
	}
	c := sections[currentSection]
	inside := func(faceIndex int) bool {
		if faceIndex < 0 || faceIndex >= len(faces) {
			return false
		}
		face := faces[faceIndex]
		for _, vertexIndex := range face.Indices {
			if int(vertexIndex) >= len(verts) {
				return false
			}
		}
		n := FaceNormal(face)
		projected := Vector3{X: point.X - n.X*planeDistance, Y: point.Y - n.Y*planeDistance, Z: point.Z - n.Z*planeDistance}
		return PointInsideFace(projected, face, verts)
	}
	rightFace := func(sectionIndex int32) (int, bool) {
		if sectionIndex < 0 || int(sectionIndex) >= len(sections) {
			return 0, false
		}
		index := int(sections[sectionIndex].FirstFace) + 3
		return index, index >= 0 && index < len(faces)
	}
	sectionFlag := func(sectionIndex int32, flag uint16) bool {
		return sectionIndex >= 0 && int(sectionIndex) < len(sections) && sections[sectionIndex].Flags&flag != 0
	}

	if c.Flags&assets.TrackSectionJunctionStart != 0 {
		if inside(candidateFace) {
			return candidateFace, true
		}
		if nextFace, ok := rightFace(c.Next); ok && inside(nextFace) {
			return candidateFace, true
		}
		return 0, false
	}

	if c.Previous >= 0 && int(c.Previous) < len(sections) {
		previousJunction := sections[c.Previous].NextJunction
		if sectionFlag(previousJunction, assets.TrackSectionJunctionStart) {
			if inside(candidateFace) {
				return candidateFace, true
			}
			if previousFace, ok := rightFace(c.Previous); ok && inside(previousFace) {
				return previousFace, true
			}
			if nextFace, ok := rightFace(c.Next); ok && inside(nextFace) {
				return nextFace, true
			}
			return 0, false
		}
	}

	if c.Next >= 0 && int(c.Next) < len(sections) {
		nextJunction := sections[c.Next].NextJunction
		if sectionFlag(nextJunction, assets.TrackSectionJunctionStart) {
			if inside(candidateFace) {
				return candidateFace, true
			}
			if previousFace, ok := rightFace(c.Previous); ok && inside(previousFace) {
				return previousFace, true
			}
			if junctionFace, ok := rightFace(nextJunction); ok && inside(junctionFace) {
				return junctionFace, true
			}
			return 0, false
		}
	}

	if c.Flags&assets.TrackSectionJunctionEnd != 0 {
		if inside(candidateFace) {
			return candidateFace, true
		}
		if previousFace, ok := rightFace(c.Previous); ok && inside(previousFace) {
			return previousFace, true
		}
		return 0, false
	}

	if c.Next < 0 || int(c.Next) >= len(sections) {
		return candidateFace, true
	}
	nextJunction := sections[c.Next].NextJunction
	if !sectionFlag(nextJunction, assets.TrackSectionJunctionEnd) {
		return candidateFace, true
	}
	if inside(candidateFace) {
		return candidateFace, true
	}
	if previousFace, ok := rightFace(c.Previous); ok && inside(previousFace) {
		return previousFace, true
	}
	return 0, false
}

// SelectNoseResponseFace dispatches to the executable's two independently
// recovered junction trees. prefixSide corresponds to a positive wall-run
// selector dot and therefore to SectionWallSweep's prefix result.
func SelectNoseResponseFace(prefixSide bool, point Vector3, planeDistance float32, currentSection, candidateFace int, sections []assets.TrackSection, faces []assets.TrackFace, verts []assets.TrackVertex) (int, bool) {
	if prefixSide {
		return SelectPrefixNoseResponseFace(point, planeDistance, currentSection, candidateFace, sections, faces, verts)
	}
	return SelectSuffixNoseResponseFace(point, planeDistance, currentSection, candidateFace, sections, faces, verts)
}

// SelectWallNoseContacts composes the face sweep, 512-unit nose probe, and
// the appropriate junction-aware response-face tree. It is deliberately
// detection-only: ApplyHardWallCollisionResponse also changes steering and
// emits effects, so mutating live ships waits for that helper's complete
// executable-backed port.
func SelectWallNoseContacts(position, forward, right Vector3, currentSection int, drivingFace assets.TrackFace, sections []assets.TrackSection, faces []assets.TrackFace, verts []assets.TrackVertex) ([]WallNoseContact, bool) {
	if currentSection < 0 || currentSection >= len(sections) {
		return nil, false
	}
	section := sections[currentSection]
	samples, ok := SampleSectionWallSensors(position, forward, right, section, drivingFace, faces, verts)
	if !ok || int(drivingFace.Indices[0]) >= len(verts) || int(drivingFace.Indices[1]) >= len(verts) {
		return nil, false
	}
	v0 := verts[drivingFace.Indices[0]]
	v1 := verts[drivingFace.Indices[1]]
	prefixSide := (float32(section.X)-position.X)*float32(v0.X-v1.X)+
		(float32(section.Y)-position.Y)*float32(v0.Y-v1.Y)+
		(float32(section.Z)-position.Z)*float32(v0.Z-v1.Z) > 0
	nose := Vector3{X: position.X + forward.X*512, Y: position.Y + forward.Y*512, Z: position.Z + forward.Z*512}

	contacts := make([]WallNoseContact, 0, len(samples))
	for _, sample := range samples {
		if sample.Nose > 0 {
			continue
		}
		responseFace, accepted := SelectNoseResponseFace(prefixSide, nose, sample.Nose, currentSection, sample.FaceIndex, sections, faces, verts)
		if accepted {
			contacts = append(contacts, WallNoseContact{CandidateFace: sample.FaceIndex, ResponseFace: responseFace, Distance: sample.Nose})
		}
	}
	return contacts, true
}

type WallResponseKind uint8

const (
	WallResponseNone WallResponseKind = iota
	WallResponseGraze
	WallResponseFull
)

// SelectWallResponse ports the repeated dispatch blocks in
// ResolveShipWallSensorCollisions (0x800344c8-0x800346b8 and mirrored blocks
// at 0x80034de8-0x80034fd8). priorCollision is the routine-local flag set
// after a hard nose response or a special-section contained graze. Ordinary
// graze calls do not set that flag in the executable.
func SelectWallResponse(distance float32, sectionCollisionFlags uint32, priorCollision, pointInside bool) WallResponseKind {
	if distance > 0 {
		return WallResponseNone
	}
	if sectionCollisionFlags&trackSectionGrazeOnlyMask != 0 {
		if pointInside {
			return WallResponseGraze
		}
		return WallResponseNone
	}
	if priorCollision {
		return WallResponseFull
	}
	return WallResponseGraze
}

// PlaneDistance ports the confirmed plane-distance primitive `sub_8003598c`
// (SLES_003.27 0x8003598c, bn-psx/docs/wipeout2097_ship_physics_hunt.md
// session 10), used throughout the original's wall/collision detection
// code (maybe_DetectShipWallCollision and friends, plus the air-brake ramp
// system's own collision checks) to test a point against a track face's
// plane.
//
// The original passes planePoint in registers and point on the stack, then
// computes `dot(planePoint-point, normal) / (-|normal|^2 >> 12)`,
// where normal is a Q12 fixed-point vector (matching WO_TrackFace's raw
// int16 normal components -- a pre-normalized unit vector scaled by
// 4096). For a unit-length normal in Q12, `-|normal|^2 >> 12` is
// approximately -4096, so the two negative signs cancel while the division
// removes the normal's Q12 scale. This port takes a true unit normal, so the
// direct equivalent is the conventional dot product -- no
// scale-correction division needed, consistent with this project's
// established convention of dropping Q12 arithmetic once a vector is a
// true unit float (see physics.go's Forward-vector handling for the same
// pattern).
//
// Positive result is therefore the conventional signed distance in the
// normal's direction. The earlier port mislabeled the two argument groups
// and negated this result, making the authentic +300 starting height look
// like a deep -300 penetration.
func PlaneDistance(point, planePoint, normal Vector3) float32 {
	dx := point.X - planePoint.X
	dy := point.Y - planePoint.Y
	dz := point.Z - planePoint.Z
	return dx*normal.X + dy*normal.Y + dz*normal.Z
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

// HardWallCollisionResponse ports the gameplay-state portion of
// ApplyHardWallCollisionResponse (0x800356b8-0x80035878). A negative plane
// distance first replaces velocity with `(velocity/abs(distance))>>5`.
// Both signs then add the face normal (2x X/Z, half Y), integrate position
// by velocity/64, and kick the signed 16-bit steering accumulator by
// `SpeedMagnitude/4 + 400` in the side selected by sideSign.
//
// Particle and sound calls after 0x80035878 are presentation-only and are
// intentionally outside this pure state transition.
func HardWallCollisionResponse(position, velocity, normal Vector3, steeringRate, speedMagnitude, sideSign, planeDistance float32) (newPosition, newVelocity Vector3, newSteeringRate float32) {
	if planeDistance < 0 {
		depth := -planeDistance
		velocity = Vector3{
			X: (velocity.X / depth) / 32,
			Y: (velocity.Y / depth) / 32,
			Z: (velocity.Z / depth) / 32,
		}
	}

	newVelocity = Vector3{
		X: velocity.X + normal.X*q12Scale*2,
		Y: velocity.Y + normal.Y*q12Scale/2,
		Z: velocity.Z + normal.Z*q12Scale*2,
	}
	newPosition = Vector3{
		X: position.X + newVelocity.X/64,
		Y: position.Y + newVelocity.Y/64,
		Z: position.Z + newVelocity.Z/64,
	}

	kick := int32(uint16(speedMagnitude))/4 + 400
	steering := int32(int16(int32(steeringRate)))
	if sideSign > 0 {
		steering += kick
	} else {
		steering -= kick
	}
	newSteeringRate = float32(int16(uint16(steering)))
	return newPosition, newVelocity, newSteeringRate
}

// FullWallCollisionResponse ports ApplyWallCollisionResponse
// (0x80035384-0x80035544). Its velocity and position transition matches the
// hard response, but its steering kick is the gentler
// `SpeedMagnitude/32 + 200`.
func FullWallCollisionResponse(position, velocity, normal Vector3, steeringRate, speedMagnitude, sideSign, planeDistance float32) (newPosition, newVelocity Vector3, newSteeringRate float32) {
	newPosition, newVelocity = wallCollisionMotion(position, velocity, normal, planeDistance)
	kick := int32(uint16(speedMagnitude))/32 + 200
	steering := int32(int16(int32(steeringRate)))
	if sideSign > 0 {
		steering += kick
	} else {
		steering -= kick
	}
	return newPosition, newVelocity, float32(int16(uint16(steering)))
}

func wallCollisionMotion(position, velocity, normal Vector3, planeDistance float32) (Vector3, Vector3) {
	if planeDistance < 0 {
		depth := -planeDistance
		velocity = Vector3{X: (velocity.X / depth) / 32, Y: (velocity.Y / depth) / 32, Z: (velocity.Z / depth) / 32}
	}
	velocity = Vector3{
		X: velocity.X + normal.X*q12Scale*2,
		Y: velocity.Y + normal.Y*q12Scale/2,
		Z: velocity.Z + normal.Z*q12Scale*2,
	}
	position = Vector3{X: position.X + velocity.X/64, Y: position.Y + velocity.Y/64, Z: position.Z + velocity.Z/64}
	return position, velocity
}

// ResolveShipWallSensorCollisions ports the gameplay-state portion of the
// executable routine at 0x80033c1c. Probe positions are recomputed in the
// original order after each response: fixed nose, negative-Right corner,
// then positive-Right corner. Effects and the retail-dormant final correction
// gate are omitted.
func ResolveShipWallSensorCollisions(ship *Ship, currentSection int, drivingFace assets.TrackFace, sections []assets.TrackSection, faces []assets.TrackFace, verts []assets.TrackVertex) (responses int, ok bool) {
	if currentSection < 0 || currentSection >= len(sections) {
		return 0, false
	}
	section := sections[currentSection]
	faceIndices, ok := SectionWallSweep(ship.Position, section, drivingFace, faces, verts)
	if !ok || section.Next < 0 || int(section.Next) >= len(sections) {
		return 0, false
	}
	v0 := verts[drivingFace.Indices[0]]
	v1 := verts[drivingFace.Indices[1]]
	prefixSide := (float32(section.X)-ship.Position.X)*float32(v0.X-v1.X)+
		(float32(section.Y)-ship.Position.Y)*float32(v0.Y-v1.Y)+
		(float32(section.Z)-ship.Position.Z)*float32(v0.Z-v1.Z) > 0
	next := sections[section.Next]
	directionDot := float32(next.X-section.X)*ship.Forward.X + float32(next.Y-section.Y)*ship.Forward.Y + float32(next.Z-section.Z)*ship.Forward.Z
	sideSign := directionDot
	if prefixSide {
		sideSign = -sideSign
	}
	nose := Vector3{X: ship.Position.X + ship.Forward.X*512, Y: ship.Position.Y + ship.Forward.Y*512, Z: ship.Position.Z + ship.Forward.Z*512}
	priorCollision := false

	for _, faceIndex := range faceIndices {
		face := faces[faceIndex]
		for _, vertexIndex := range face.Indices {
			if int(vertexIndex) >= len(verts) {
				return responses, false
			}
		}
		planePoint := faceVertex0(face, verts)
		normal := FaceNormal(face)
		noseDistance := PlaneDistance(nose, planePoint, normal)
		if noseDistance <= 0 {
			if responseFace, accepted := SelectNoseResponseFace(prefixSide, nose, noseDistance, currentSection, faceIndex, sections, faces, verts); accepted {
				responseNormal := FaceNormal(faces[responseFace])
				ship.Position, ship.Velocity, ship.SteeringRate = HardWallCollisionResponse(ship.Position, ship.Velocity, responseNormal, ship.SteeringRate, ship.SpeedMagnitude, sideSign, noseDistance)
				priorCollision = true
				responses++
			}
		}

		for corner := 0; corner < 2; corner++ {
			edge := WallSensorEdge(ship.Position, ship.Forward, ship.Right, prefixSide)
			point := edge[corner]
			distance := PlaneDistance(point, planePoint, normal)
			pointInside := false
			if distance <= 0 && section.CollisionFlags&trackSectionGrazeOnlyMask != 0 {
				projected := Vector3{X: point.X - normal.X*distance, Y: point.Y - normal.Y*distance, Z: point.Z - normal.Z*distance}
				pointInside = PointInsideFace(projected, face, verts)
			}
			switch SelectWallResponse(distance, section.CollisionFlags, priorCollision, pointInside) {
			case WallResponseGraze:
				ship.Velocity = WallBounceImpulse(ship.Velocity, normal)
				if section.CollisionFlags&trackSectionGrazeOnlyMask != 0 {
					priorCollision = true
				}
				responses++
			case WallResponseFull:
				ship.Position, ship.Velocity, ship.SteeringRate = FullWallCollisionResponse(ship.Position, ship.Velocity, normal, ship.SteeringRate, ship.SpeedMagnitude, sideSign, distance)
				responses++
			}
		}
	}
	return responses, true
}

// ResolveShipTrackWalls is the track-resource adapter for the authentic wall
// resolver. It mirrors the original prerequisites: refresh orientation rows
// and the track-side flag, locate the section's first Track face, then pass
// that face (not the side-selected paired driving face) to the wall sweep.
func ResolveShipTrackWalls(ship *Ship, track *assets.Track) (int, error) {
	UpdateShipOrientationVectors(ship)
	if err := UpdateShipTrackFaceSide(ship, track); err != nil {
		return 0, err
	}
	_, firstDrivingFace, err := shipSectionAndFirstDrivingFace(ship, track)
	if err != nil {
		return 0, err
	}
	responses, ok := ResolveShipWallSensorCollisions(ship, int(ship.SectionID), track.Faces[firstDrivingFace], track.Sections, track.Faces, track.Vertices)
	if !ok {
		return responses, fmt.Errorf("physics: section %d wall collision geometry invalid", ship.SectionID)
	}
	return responses, nil
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

// ResolveShipWallCollisionApproximate composes NearestWallDistance, PointInsideFace, and
// WallCollisionResponse into a single per-frame check, mirroring what
// `maybe_ResolveShipWallCollision` (SLES_003.27 0x80033c1c) does for real,
// with deliberate simplifications flagged below -- this is a best-effort
// composition of the confirmed primitives, not a full port of the original's
// own dispatch tree (see bn-psx/docs/wipeout2097_ship_physics_hunt.md
// sessions 11-12 for what that tree actually looks like).
//
// Reports whether a collision was resolved (velocity/position updated).
//
// # Legacy diagnostic only
//
// The authentic multi-probe path now exists as
// ResolveShipWallSensorCollisions. This older helper remains only for focused
// geometry experiments that want a caller-supplied proximity threshold:
//
//   - It uses one sensor at the ship Position rather than the original's
//     sequential nose and two position-recomputed corner probes.
//   - Always uses the fuller WallCollisionResponse, never
//     WallBounceImpulse/maybe_ApplyWallGrazeImpulse or the ship-vs-ship
//     maybe_ApplyShipCollisionResponse -- the original's choice between
//     these three is gated by an unconfirmed section-level flag
//     (`section+0x94 & 0x180000`, session 12), which the authentic resolver
//     now handles.
//   - `penetrating`/`depth` are derived directly from PlaneDistance's own
//     confirmed sign convention (negative = crossed behind an inward-facing
//     wall plane) rather than reverse-engineered
//     from the original's specific dispatch-site register wiring (`a3`),
//     which threads through a multi-stage nested refinement this project
//     hasn't fully traced yet (see session 11's same caveat on
//     WallCollisionResponse's own doc comment).
//   - The point-plane projection PointInsideFace needs
//     (`point - normal*distance`) is this port's own geometrically-verified
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
// `distance < contactDistance`; positive values are clearance on the inward
// side and negative values are penetration. The original
// almost certainly has its own such threshold (or reaches this code at all
// only for a short list of nearby candidate faces, e.g. via section-local
// spatial binning) but it wasn't traced this session -- callers should treat
// this parameter as a real, uncalibrated gap, not a confirmed value.
func ResolveShipWallCollisionApproximate(s *Ship, section assets.TrackSection, faces []assets.TrackFace, verts []assets.TrackVertex, contactDistance float32) bool {
	faceIdx, distance, ok := NearestWallDistance(s.Position, section, faces, verts)
	if !ok || distance >= contactDistance {
		return false
	}

	f := faces[faceIdx]
	normal := FaceNormal(f)
	projected := Vector3{
		X: s.Position.X - normal.X*distance,
		Y: s.Position.Y - normal.Y*distance,
		Z: s.Position.Z - normal.Z*distance,
	}
	if !PointInsideFace(projected, f, verts) {
		return false
	}

	s.Position, s.Velocity = WallCollisionResponse(s.Position, s.Velocity, normal, distance < 0, -distance)
	return true
}
