package render

import (
	"math"

	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/game"
	"github.com/tridentsx/wipeout-go/internal/physics"
)

// Camera is a world-space view. WipEout's world Y axis points down, so the
// camera basis below deliberately calls its vertical axis Down rather than Up.
type Camera struct {
	Position         game.Vector3
	Yaw, Pitch, Roll game.Angle
}

type CameraView uint8

const (
	CameraExternal CameraView = iota
	CameraCockpit
)

// RaceCamera holds the state that the original keeps at and immediately after
// 0x800bde0c. In particular, the external camera cannot be represented by a
// stateless position formula: UpdateChaseCameraFollow (0x80020608) integrates a
// persistent spring velocity toward the track centerline, plus a second,
// slower-decaying vertical clearance accumulator.
//
// That function was previously unreadable: Binary Ninja placed a function four
// bytes past 0x80020608, so the address resolved into the unrelated preceding
// routine and the clearance logic had to be substituted from wipeout-rewrite.
// The cause was a delay-slot double count in bn-psx's R3000A architecture which
// shifted 147 function boundaries by one instruction; with that fixed the real
// entry is claimed and both terms below are the retail ones.
type RaceCamera struct {
	Camera
	View           CameraView
	Section        int
	SpringVelocity game.Vector3
	// ClearanceBias is the retail vertical-clearance accumulator, kept at
	// gp+128 (0x80094990) and updated at 0x80020850-0x8002088c. It decays at
	// /16 per frame, independently of SpringVelocity's /8, and is subtracted
	// from the anchor's Y after the spring is applied. Steady state is
	// |correction|/8, so it lifts the camera further than folding the term into
	// the spring would.
	ClearanceBias float32
	// RaceStartTimer mirrors ship+0xe0 while the retail start-camera
	// callback is active.  Race setup initializes it to 0xa6 and the callback
	// hands off to the normal view at 0x64.
	RaceStartTimer  int32
	RaceStartActive bool
}

// NewRaceCamera initializes the normal racing camera on the ship's current
// section. Race-start animation is installed separately by BeginRaceStart.
func NewRaceCamera(ship *game.Ship, sections []assets.TrackSection) *RaceCamera {
	c := &RaceCamera{Section: int(ship.SectionID)}
	if c.Section < 0 || c.Section >= len(sections) {
		c.Section = 0
	}
	c.Update(ship, sections)
	return c
}

// BeginRaceStart enters the retail presentation callback state.  Physics and
// input remain the caller's responsibility; this only changes camera output.
func (c *RaceCamera) BeginRaceStart() {
	c.RaceStartTimer = 0xa6
	c.RaceStartActive = true
}

func (c *RaceCamera) ToggleView(ship *game.Ship) {
	if c.View == CameraExternal {
		c.View = CameraCockpit
		ship.Flags |= game.ShipFlagCockpitCamera
		return
	}
	c.View = CameraExternal
	ship.Flags &^= game.ShipFlagCockpitCamera
}

func (c *RaceCamera) Update(ship *game.Ship, sections []assets.TrackSection) {
	if c.RaceStartActive {
		c.updateRaceStart(ship, sections)
		return
	}
	if c.View == CameraCockpit {
		c.updateCockpit(ship)
		return
	}
	c.updateExternal(ship, sections)
}

// AdvanceRaceStart ports the per-frame countdown of the presentation timer.
// The original callback itself performs the camera update and switches at
// exactly 100; keeping decrement separate makes the 25 Hz game-tick caller
// explicit (PAL presentation is field-doubled to 50 Hz).
func (c *RaceCamera) AdvanceRaceStart() {
	if c.RaceStartActive && c.RaceStartTimer > 0 {
		c.RaceStartTimer--
	}
}

func (c *RaceCamera) updateRaceStart(ship *game.Ship, sections []assets.TrackSection) {
	if len(sections) == 0 {
		c.RaceStartActive = false
		c.updateExternal(ship, sections)
		return
	}
	current := int(ship.SectionID)
	if current < 0 || current >= len(sections) {
		current = 0
	}
	// The retail node's +8 link is TrackSection.Next. It starts at the
	// current section's +8 node, then follows that link nine times (the loop
	// in 0x800209f4 runs i=8 through i=0).
	anchorIndex := linkedSectionIndex(sections, int(sections[current].Next), current)
	for i := 8; i >= 0; i-- {
		anchorIndex = linkedSectionIndex(sections, int(sections[anchorIndex].Next), anchorIndex)
	}
	start := sectionPoint(sections[anchorIndex])
	start.Y -= 800
	currentSection := sectionPoint(sections[current])
	target := game.Vector3{X: currentSection.X, Y: ship.Position.Y - 400, Z: currentSection.Z}

	// Exact signed fixed-point easing from 0x800209f8. The position delta is
	// multiplied before the /100 and >>8 operations; shifting the easing term
	// first quantizes the camera into multi-second plateaus.
	v := int64(200 - c.RaceStartTimer)
	ease := int64(0x6400) - (v*0x200 + ((v * v * -5) >> 1))
	anchor := game.Vector3{
		// The binary computes target - (target-start)*ease, i.e. the
		// current/grid point is the interpolation origin and the far
		// reference is the second endpoint.
		X: target.X + float32((int64(target.X-start.X)*-ease/100)>>8),
		Y: target.Y + float32((int64(target.Y-start.Y)*-ease/100)>>8),
		Z: target.Z + float32((int64(target.Z-start.Z)*-ease/100)>>8),
	}
	c.Section = nearestCameraSection(anchor, sections, anchorIndex)
	c.Camera.Position = anchor
	look := sub(ship.Position, anchor)
	horizontal := float64(math.Hypot(float64(look.X), float64(look.Z)))
	if horizontal > 0 {
		c.Camera.Yaw = radiansToAngle(math.Atan2(float64(look.X), float64(look.Z)))
		c.Camera.Pitch = radiansToAngle(math.Atan2(float64(look.Y), horizontal))
	}
	c.Camera.Roll = 0
	if c.RaceStartTimer == 0x64 {
		// The original callback writes the intro pose first, then installs the
		// normal callback for the following frame. Do not overwrite this
		// frame's intro pose with the chase pose here.
		//
		// updateExternal computes its own anchor fresh from the ship's current
		// state every call -- ship.Position - Forward*1024, Y-200 -- with no
		// relation to where this intro swoop just left the camera (that anchor
		// tracks the section's road-center line; the ship's actual grid-slot
		// position sits off that centerline by design). Left alone, the very
		// next frame snaps by however far those two unrelated formulas
		// disagree: simulated from TRACK01's start grid, ~876 world units in a
		// single 1/25s tick, which reads as a jump/spin right as the intro
		// hands off. Priming SpringVelocity so next frame's
		// (freshExternalAnchor + SpringVelocity) equals this frame's intro
		// position removes the pop; the existing per-tick decay then eases it
		// toward the real tracked position exactly as it already does for any
		// other transient error.
		forward := shipForward(ship)
		externalAnchor := game.Vector3{
			X: ship.Position.X - forward.X*1024,
			Y: ship.Position.Y - forward.Y*1024 - 200,
			Z: ship.Position.Z - forward.Z*1024,
		}
		c.SpringVelocity = sub(c.Camera.Position, externalAnchor)
		c.RaceStartActive = false
	}
}

func linkedSectionIndex(sections []assets.TrackSection, index, fallback int) int {
	if index >= 0 && index < len(sections) {
		return index
	}
	return fallback
}

// updateExternal ports UpdateChaseCameraFollow's float-equivalent camera
// math. The original anchor is ship position minus its Q12 forward vector /4,
// i.e. 1024 world units behind, with another -200 on Y. Its 60/50 correction
// is PAL normalization already present literally in the executable.
func (c *RaceCamera) updateExternal(ship *game.Ship, sections []assets.TrackSection) {
	forward := shipForward(ship)
	position := game.Vector3{
		X: ship.Position.X - forward.X*1024,
		Y: ship.Position.Y - forward.Y*1024 - 200,
		Z: ship.Position.Z - forward.Z*1024,
	}

	if len(sections) != 0 {
		c.Section = nearestCameraSection(position, sections, c.Section)
		section := sections[c.Section]
		previous := linkedSection(sections, int(section.Previous), c.Section)
		projected := projectPointToRay(position, sectionPoint(previous), sectionPoint(section))
		errorFromCenter := sub(position, projected)

		// UpdateChaseCameraFollow (0x80020608) scales the centerline error by
		// 60/50 before using it. The literal sequence is err*60 (sll 4, sub,
		// sll 2) then a 0x51eb851f multiply whose mfhi>>4 is /50, at
		// 0x800206fc-0x80020760.
		correction := mul(errorFromCenter, 60.0/50.0)

		c.SpringVelocity.X -= correction.X / 64
		c.SpringVelocity.Y -= correction.Y / 64
		c.SpringVelocity.Z -= correction.Z / 64
		c.SpringVelocity.X -= c.SpringVelocity.X / 8
		c.SpringVelocity.Y -= c.SpringVelocity.Y / 8
		c.SpringVelocity.Z -= c.SpringVelocity.Z / 8
		position = add(position, c.SpringVelocity)

		// Vertical clearance, read from 0x80020850-0x8002088c. This is a
		// *second* accumulator with its own, slower decay -- not a term folded
		// into the spring:
		//
		//   VectorMagnitude3D(correction)      ; the 60/50-scaled error
		//   clearance += magnitude >> 7        ; /128
		//   clearance -= clearance >> 4        ; /16, slower than the spring's /8
		//   anchor.Y  -= clearance
		//
		// An earlier revision here substituted wipeout-rewrite's
		// camera_update_race_external, which folds the boost into the same
		// acceleration vector as X/Z (correction.Y += length*0.5), because
		// 0x80020608 could not be read: Binary Ninja had placed a function four
		// bytes past that address, leaving the real entry unclaimed. That was an
		// architecture bug in bn-psx (a delay-slot double count that shifted 147
		// function boundaries) and is now fixed, so the retail form is used.
		//
		// The reason the separate accumulator misbehaved when it was first tried
		// is the constants, not the structure: the increment is magnitude/128 and
		// the decay is /16. Folding it through SpringVelocity as well double-counts.
		//
		// Retail keeps this at gp+128 (0x80094990), i.e. one global shared by all
		// cameras; per-camera state here differs only in two-player, where the
		// original would have both views feeding one accumulator.
		c.ClearanceBias += length(correction) / 128
		c.ClearanceBias -= c.ClearanceBias / 16
		position.Y -= c.ClearanceBias
	}

	// The camera node stores -shipYaw and -shipPitch at
	// 0x80020898-0x800208b8. Preserve that orientation rather than aiming at
	// the craft: the real chase view keeps the track horizon in view.
	c.Camera = Camera{Position: position, Yaw: -ship.Yaw, Pitch: -ship.Pitch}
}

// updateCockpit ports UpdateCockpitCameraView (0x800208fc). The helper at
// 0x80021a3c produces the ship matrix's local-up column in Q12; >>5 is a
// rigid 128-unit offset. Unlike the external camera, cockpit view inherits
// roll exactly.
func (c *RaceCamera) updateCockpit(ship *game.Ship) {
	up := shipLocalUp(ship)
	c.Section = int(ship.SectionID)
	c.Camera = Camera{
		Position: game.Vector3{
			X: ship.Position.X - up.X*128,
			Y: ship.Position.Y - up.Y*128,
			Z: ship.Position.Z - up.Z*128,
		},
		Yaw: -ship.Yaw, Pitch: -ship.Pitch, Roll: -ship.Roll,
	}
}

func shipForward(ship *game.Ship) game.Vector3 {
	sy, cy := ship.Yaw.Sin(), ship.Yaw.Cos()
	sp, cp := ship.Pitch.Sin(), ship.Pitch.Cos()
	return game.Vector3{X: -sy * cp, Y: -sp, Z: cy * cp}
}

func shipLocalUp(ship *game.Ship) game.Vector3 {
	sy, cy := ship.Yaw.Sin(), ship.Yaw.Cos()
	sp, cp := ship.Pitch.Sin(), ship.Pitch.Cos()
	sr, cr := ship.Roll.Sin(), ship.Roll.Cos()
	return game.Vector3{
		X: cy*sr - sy*sp*cr,
		Y: cp * cr,
		Z: sy*sr + cy*sp*cr,
	}
}

// Basis returns right, down and forward camera axes in world space.
func (c Camera) Basis() (right, down, forward game.Vector3) {
	sy, cy := c.Yaw.Sin(), c.Yaw.Cos()
	sp, cp := c.Pitch.Sin(), c.Pitch.Cos()
	sr, cr := c.Roll.Sin(), c.Roll.Cos()
	// These are the rows written by maybe_BuildRotationMatrixFromEulerAnglesAlt
	// (0x8001e0a8). The camera callbacks pass (pitch, yaw, roll), with the
	// first two angles already negated from the ship. The GTE consumes this
	// node as a view matrix, so its rows are the camera axes.
	right = game.Vector3{
		X: cy * cr,
		Y: -cy * sr,
		Z: -sy,
	}
	down = game.Vector3{
		X: cp*sr - sp*sy*cr,
		Y: cp*cr + sp*sy*sr,
		Z: -sp * cy,
	}
	forward = game.Vector3{
		X: sp*sr + cp*sy*cr,
		Y: sp*cr - cp*sy*sr,
		Z: cp * cy,
	}
	return
}

func (c Camera) WorldToCamera(point game.Vector3) game.Vector3 {
	right, down, forward := c.Basis()
	delta := sub(point, c.Position)
	return game.Vector3{X: dot(delta, right), Y: dot(delta, down), Z: dot(delta, forward)}
}

// NewChaseCamera remains as a compatibility helper for the old map renderer.
// It now uses the confirmed unsmoothed binary anchor rather than hand-picked
// constants; callers requiring faithful motion must retain a RaceCamera.
func NewChaseCamera(ship *game.Ship) Camera {
	forward := shipForward(ship)
	return Camera{
		Position: game.Vector3{
			X: ship.Position.X - forward.X*1024,
			Y: ship.Position.Y - forward.Y*1024 - 200,
			Z: ship.Position.Z - forward.Z*1024,
		},
		Yaw: ship.Yaw, Pitch: ship.Pitch,
	}
}

func (c Camera) ProjectTopDown(p game.Vector3) (x, z float32) {
	dx := p.X - c.Position.X
	dz := p.Z - c.Position.Z
	sin, cos := c.Yaw.Sin(), c.Yaw.Cos()
	return dx*cos - dz*sin, dx*sin + dz*cos
}

// SectionCamera is a deliberately simple track-inspection view. Its position
// and aim points are linked section face centroids, so bends do not get cut by
// a long tangent approximation and imprecise TRS origins cannot push the view
// outside the road. This is presentation-only and is not the race camera.
func SectionCamera(track *assets.Track, index int) Camera {
	if track == nil || len(track.Sections) == 0 {
		return Camera{}
	}
	sections := track.Sections
	index = linkedIndex(sections, index, 0)
	cameraIndex := index
	cameraDistance := float32(0)
	for range len(sections) {
		previousIndex := linkedIndex(sections, int(sections[cameraIndex].Previous), cameraIndex)
		if previousIndex == index || previousIndex == cameraIndex {
			break
		}
		cameraDistance += length(sub(sectionRoadCenter(track, previousIndex), sectionRoadCenter(track, cameraIndex)))
		cameraIndex = previousIndex
		if cameraDistance >= 18000 {
			break
		}
	}
	targetIndex := index
	targetDistance := float32(0)
	for range len(sections) {
		nextIndex := linkedIndex(sections, int(sections[targetIndex].Next), targetIndex)
		if nextIndex == index || nextIndex == targetIndex {
			break
		}
		targetDistance += length(sub(sectionRoadCenter(track, nextIndex), sectionRoadCenter(track, targetIndex)))
		targetIndex = nextIndex
		if targetDistance >= 6000 {
			break
		}
	}
	cameraRoadCenter := sectionRoadCenter(track, cameraIndex)
	position := cameraRoadCenter
	position.Y -= 1800
	target := sectionRoadCenter(track, targetIndex)
	direction := sub(target, position)
	horizontal := float32(math.Hypot(float64(direction.X), float64(direction.Z)))
	if horizontal == 0 {
		direction.Z = 1
		horizontal = 1
	}
	yaw := radiansToAngle(math.Atan2(float64(direction.X), float64(direction.Z)))
	// Remove the 700-unit viewing height from the slope calculation. Otherwise
	// every view tilts down merely because the camera is above the road.
	roadDeltaY := target.Y - cameraRoadCenter.Y
	pitch := radiansToAngle(math.Atan2(float64(roadDeltaY), float64(horizontal)))
	return Camera{Position: position, Yaw: yaw, Pitch: pitch}
}

func sectionRoadCenter(track *assets.Track, sectionIndex int) game.Vector3 {
	section := track.Sections[sectionIndex]
	first, end := int(section.FirstFace), int(section.FirstFace)+int(section.NumFaces)
	if first < 0 || first >= len(track.Faces) || end > len(track.Faces) {
		return sectionPoint(section)
	}
	var center game.Vector3
	vertices := 0
	for _, face := range track.Faces[first:end] {
		if face.Flags&assets.TrackFaceTrack == 0 {
			continue
		}
		n := 4
		if face.Indices[2] == face.Indices[3] {
			n = 3
		}
		for i := 0; i < n; i++ {
			vertexIndex := int(face.Indices[i])
			if vertexIndex >= len(track.Vertices) {
				continue
			}
			vertex := track.Vertices[vertexIndex]
			center = add(center, game.Vector3{X: float32(vertex.X), Y: float32(vertex.Y), Z: float32(vertex.Z)})
			vertices++
		}
	}
	if vertices == 0 {
		return sectionPoint(section)
	}
	return mul(center, 1/float32(vertices))
}

func radiansToAngle(radians float64) game.Angle {
	units := int64(math.Round(radians * float64(game.AngleFullTurn) / (2 * math.Pi)))
	return game.Angle(units)
}

// nearestCameraSection shares physics.NearestTrackSection -- the same
// junction-aware graph search UpdateShipTrackSection uses to place the ship
// -- rather than a camera-only walk that only ever follows .Next/.Previous.
// Track forks (a boost-pad shortcut, say) branch through a section's
// NextJunction link; a search blind to that link tracks whichever branch
// .Next happens to point down regardless of which one the ship actually
// took, so on the branch .Next doesn't follow, the camera's tracked section
// visibly drifted away from the ship's real one across the fork and only
// resynced once the branches rejoined -- by which point the camera had
// drifted ahead of (or behind) the ship it's supposed to be chasing.
func nearestCameraSection(point game.Vector3, sections []assets.TrackSection, current int) int {
	if current < 0 || current >= len(sections) {
		current = 0
	}
	best, _, err := physics.NearestTrackSection(point, sections, current)
	if err != nil {
		return current
	}
	return best
}

func linkedIndex(sections []assets.TrackSection, candidate, fallback int) int {
	if candidate < 0 || candidate >= len(sections) {
		return fallback
	}
	return candidate
}

func linkedSection(sections []assets.TrackSection, candidate, fallback int) assets.TrackSection {
	return sections[linkedIndex(sections, candidate, fallback)]
}

func sectionPoint(s assets.TrackSection) game.Vector3 {
	return game.Vector3{X: float32(s.X), Y: float32(s.Y), Z: float32(s.Z)}
}

func projectPointToRay(point, rayStart, rayEnd game.Vector3) game.Vector3 {
	ray := sub(rayEnd, rayStart)
	denom := dot(ray, ray)
	if denom == 0 {
		return rayStart
	}
	t := dot(sub(point, rayStart), ray) / denom
	return add(rayStart, mul(ray, t))
}

func add(a, b game.Vector3) game.Vector3 {
	return game.Vector3{X: a.X + b.X, Y: a.Y + b.Y, Z: a.Z + b.Z}
}
func sub(a, b game.Vector3) game.Vector3 {
	return game.Vector3{X: a.X - b.X, Y: a.Y - b.Y, Z: a.Z - b.Z}
}
func mul(v game.Vector3, s float32) game.Vector3 {
	return game.Vector3{X: v.X * s, Y: v.Y * s, Z: v.Z * s}
}
func dot(a, b game.Vector3) float32 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func length(v game.Vector3) float32 { return float32(math.Sqrt(float64(dot(v, v)))) }
