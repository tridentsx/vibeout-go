package render

import (
	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/game"
)

// DrawMovingObject draws a path-following object at its current position and heading,
// tinting its primitives from the light animator.
//
// Retail applies the transform through the scene graph: maybe_IntegrateMovingObjectPath
// calls maybe_SetNodeTranslation with the entity's position and builds a rotation matrix
// from its angles, then marks the node dirty. This does the same arithmetic directly,
// since the port has no scene graph.
func DrawMovingObject(frame *Frame, camera Camera, object *assets.Object,
	textures []*sdl.GPUTexture, images []*assets.Image,
	mover *game.MovingObject, glow *game.CraftGlow, width, height float32) {
	if frame == nil || object == nil || mover == nil {
		return
	}
	const near = float32(1)
	focalX, focalY := projectionFocals(width, height)
	sin, cos := mover.Yaw.Sin(), mover.Yaw.Cos()

	var colors [game.CraftPrimitiveCount]game.StartLightRGB
	if glow != nil {
		colors = glow.Colors()
	}

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
			// The handedness has to agree with how the path system derives motion, or the
			// object flies one way and points the other. maybe_MovingObjectFlightStateB
			// sets acceleration to (-sin*cos, ., cos*cos), so its forward axis is
			// (-sin, +cos) and the rotation must use that same sign.
			world := game.Vector3{
				X: mover.Position.X + lx*cos - lz*sin,
				Y: mover.Position.Y + ly,
				Z: mover.Position.Z + lx*sin + lz*cos,
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
		// Retail tints gouraud primitives -- type tags 6 and 8 -- and leaves the rest as
		// authored.
		if glow != nil && (polygon.Type == 6 || polygon.Type == 8) && lamp < game.CraftPrimitiveCount {
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
