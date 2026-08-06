package render

import (
	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/game"
)

// FindObject returns the named object from a decoded PRM bundle.
func FindObject(objects []assets.Object, name string) *assets.Object {
	for i := range objects {
		if objects[i].Header.Name == name {
			return &objects[i]
		}
	}
	return nil
}

// DrawShipPerspective draws PRM geometry using the ship's confirmed Forward
// and Right orientation rows. textures/images are the craft model's paired
// CMP texture pages (see assets.Loader.LoadModel), index-aligned and
// consumed the same way as scenery/sky: objectPresentation resolves each
// polygon's Texture index to a GPU texture (falling back to its decoded
// flat color when untextured or unresolvable), and textureDimensions
// normalizes its raw PRM UV bytes against that specific texture's actual
// pixel size -- the same fix DrawSceneryPerspective needed, since TERRY.CMP's
// pages are all well under 256px (e.g. 64x96, 32x64) and a fixed /255 would
// under-divide and sample only their left/top fraction.
func DrawShipPerspective(frame *Frame, camera Camera, ship *game.Ship, object *assets.Object, textures []*sdl.GPUTexture, images []*assets.Image, width, height float32) {
	if frame == nil || ship == nil || object == nil {
		return
	}
	const near = float32(1)
	focalX, focalY := projectionFocals(width, height)
	up := cross(ship.Forward, ship.Right)
	for _, polygon := range object.Polygons {
		if len(polygon.Indices) < 3 {
			continue
		}
		uvWidth, uvHeight := textureDimensions(images, polygon.Texture)
		n := len(polygon.Indices)
		vertices := make([]perspectiveVertex, 0, n+2)
		valid := true
		for slot := 0; slot < n; slot++ {
			i := quadIndexOrder(n, slot)
			index := polygon.Indices[i]
			if int(index) >= len(object.Vertices) {
				valid = false
				break
			}
			v := object.Vertices[index]
			localX := float32(v.X) - float32(object.Header.Origin.X)
			localY := float32(v.Y) - float32(object.Header.Origin.Y)
			localZ := float32(v.Z) - float32(object.Header.Origin.Z)
			world := game.Vector3{
				X: ship.Position.X + ship.Right.X*localX + up.X*localY + ship.Forward.X*localZ,
				Y: ship.Position.Y + ship.Right.Y*localX + up.Y*localY + ship.Forward.Y*localZ,
				Z: ship.Position.Z + ship.Right.Z*localX + up.Z*localY + ship.Forward.Z*localZ,
			}
			var uv sdl.FPoint
			if i < len(polygon.UV) {
				uv = sdl.FPoint{X: float32(polygon.UV[i].U) / uvWidth, Y: float32(polygon.UV[i].V) / uvHeight}
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
		texture, color := objectPresentation(textures, polygon)
		for i := 1; i+1 < len(vertices); i++ {
			corners := [3]perspectiveVertex{vertices[0], vertices[i], vertices[i+1]}
			// cull=false, matching DrawPerspective/drawObjectsPerspective: this
			// pipeline's hardware depth test already hides a closed hull's true
			// backfaces (see gpu.go's EnableDepthTest/EnableDepthWrite), so CPU
			// culling is redundant there and actively wrong for single-sided
			// flat panels like the wingtips -- a quad with no opposing backface
			// polygon in the source data vanished depending on view angle
			// whenever its one winding faced away from the camera. The `true`
			// here predates the depth-tested GPU rewrite (4592e83), when
			// culling stood in for the painter's-algorithm renderer's lack of
			// any real occlusion test; track.go dropped it then, this call site
			// was never revisited.
			submitScreenTriangle(frame, corners, focalX, focalY, width, height, texture, color, false)
		}
	}
}

func polygonColor(polygon assets.Polygon) sdl.FColor {
	color := assets.Color{R: 128, G: 96, B: 32}
	if polygon.Color != nil {
		color = *polygon.Color
	} else if len(polygon.Colors) > 0 {
		var r, g, b int
		for _, vertexColor := range polygon.Colors {
			r += int(vertexColor.R)
			g += int(vertexColor.G)
			b += int(vertexColor.B)
		}
		color = assets.Color{R: uint8(r / len(polygon.Colors)), G: uint8(g / len(polygon.Colors)), B: uint8(b / len(polygon.Colors))}
	}
	return sdl.FColor{R: colorComponent(color.R), G: colorComponent(color.G), B: colorComponent(color.B), A: 1}
}

func colorComponent(value uint8) float32 {
	component := float32(value) / 128
	if component > 1 {
		return 1
	}
	return component
}

func cross(a, b game.Vector3) game.Vector3 {
	return game.Vector3{X: a.Y*b.Z - a.Z*b.Y, Y: a.Z*b.X - a.X*b.Z, Z: a.X*b.Y - a.Y*b.X}
}
