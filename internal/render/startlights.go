package render

import (
	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/game"
)

// The start light gantry, from maybe_UpdateStartLightGantry (SLES_003.27 0x80049ef4).
//
// Retail places three instances of COMMON/LIGHT.PRM's `light1` object and per tick:
//
//	p   = maybe_StartLightPlacements[i]      // a track section
//	ref = *(p + 4)                            // its Previous
//	yaw = ratan2(p->0x0c - ref->0x0c, p->0x14 - ref->0x14)
//	BuildRotationMatrixFromEulerAngles(node, 0x1000 - yaw, 0, 0)
//	SetNodeTranslation(node, p->0x0c, p->0x10 - 0xbb8, p->0x14)
//	node->0x40 = 1                            // forced visible
//
// so each sits at a section's centre raised 0xbb8 and is aimed along the local track
// direction. Which three sections maybe_StartLightPlacements holds is NOT known: it
// lives in .bss and is read as base+i*4, so its writer leaves no reference to find.
// The placement below approximates it from the start line, which is where the gantry
// stands.

// gantryHeight is retail's 0xbb8 vertical offset. Negative Y is up.
const gantryHeight = 3000

// gantrySpacing is how many sections apart the three gantries are placed. This is the
// port's own choice, standing in for the unrecovered placement array.
const gantrySpacing = 2

// DrawStartLightGantries draws the three gantries near the track's start line, tinted
// for the current countdown phase.
func DrawStartLightGantries(frame *Frame, camera Camera, track *assets.Track, trackID uint8,
	object *assets.Object, textures []*sdl.GPUTexture, images []*assets.Image,
	lights *game.StartLightState, width, height float32) {
	if frame == nil || track == nil || object == nil || lights == nil {
		return
	}
	line, ok := game.TrackStartLineSection[trackID]
	if !ok {
		return
	}
	colors := lights.Colors()

	// Walk back from the line so the gantries straddle it the way the real ones do.
	section := line
	for i := 0; i < game.GantryCount; i++ {
		if section < 0 || section >= len(track.Sections) {
			return
		}
		s := track.Sections[section]
		previous := int(s.Previous)
		if previous < 0 || previous >= len(track.Sections) {
			return
		}
		p := track.Sections[previous]

		// Aim along the local track direction, as retail does from the section's
		// Previous.
		yaw := game.AngleFromDirection(s.X-p.X, s.Z-p.Z)
		position := game.Vector3{
			X: float32(s.X),
			Y: float32(s.Y) - gantryHeight,
			Z: float32(s.Z),
		}
		drawGantry(frame, camera, object, textures, images, position, yaw, colors, width, height)

		for step := 0; step < gantrySpacing; step++ {
			next := int(track.Sections[section].Previous)
			if next < 0 || next >= len(track.Sections) {
				return
			}
			section = next
		}
	}
}

// drawGantry draws one gantry, tinting the first game.StartLightCount type-4 polygons
// with the countdown colours and leaving the rest of the model as authored.
func drawGantry(frame *Frame, camera Camera, object *assets.Object,
	textures []*sdl.GPUTexture, images []*assets.Image,
	position game.Vector3, yaw game.Angle, colors [game.StartLightCount]game.StartLightRGB,
	width, height float32) {
	const near = float32(1)
	focalX, focalY := projectionFocals(width, height)
	sin, cos := yaw.Sin(), yaw.Cos()

	lamp := 0
	for _, polygon := range object.Polygons {
		if len(polygon.Indices) < 3 {
			continue
		}
		uvWidth, uvHeight := textureDimensions(images, polygon.Texture)
		vertices := make([]perspectiveVertex, 0, len(polygon.Indices)+2)
		valid := true
		for slot := 0; slot < len(polygon.Indices); slot++ {
			i := quadIndexOrder(len(polygon.Indices), slot)
			index := polygon.Indices[i]
			if int(index) >= len(object.Vertices) {
				valid = false
				break
			}
			v := object.Vertices[index]
			lx := float32(v.X) - float32(object.Header.Origin.X)
			ly := float32(v.Y) - float32(object.Header.Origin.Y)
			lz := float32(v.Z) - float32(object.Header.Origin.Z)
			world := game.Vector3{
				X: position.X + lx*cos + lz*sin,
				Y: position.Y + ly,
				Z: position.Z - lx*sin + lz*cos,
			}
			var uv sdl.FPoint
			if i < len(polygon.UV) && uvWidth > 0 && uvHeight > 0 {
				uv = sdl.FPoint{
					X: float32(polygon.UV[i].U) / uvWidth,
					Y: float32(polygon.UV[i].V) / uvHeight,
				}
			}
			vertices = append(vertices, perspectiveVertex{position: camera.WorldToCamera(world), uv: uv})
		}
		if !valid {
			continue
		}
		vertices = clipPerspectiveNearPlane(vertices, near)
		if len(vertices) < 3 {
			continue
		}

		texture, colour := objectPresentation(textures, polygon)
		// Type 4 is the flat-shaded tag retail's tint loop selects, and only the first
		// eight of them are lamps.
		if polygon.Type == 4 && lamp < game.StartLightCount {
			c := colors[lamp]
			colour = sdl.FColor{
				R: float32(c.R) / 255,
				G: float32(c.G) / 255,
				B: float32(c.B) / 255,
				A: 1,
			}
			texture = nil
			lamp++
		}
		for i := 1; i+1 < len(vertices); i++ {
			corners := [3]perspectiveVertex{vertices[0], vertices[i], vertices[i+1]}
			submitScreenTriangle(frame, corners, focalX, focalY, width, height, texture, colour, false)
		}
	}
}
