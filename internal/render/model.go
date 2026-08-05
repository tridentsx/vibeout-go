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
// and Right orientation rows. Textured primitives retain their decoded color
// modulation but currently use flat color until the ship texture-page mapping
// is connected.
func DrawShipPerspective(frame *Frame, camera Camera, ship *game.Ship, object *assets.Object, width, height float32) {
	if frame == nil || ship == nil || object == nil {
		return
	}
	const near = float32(1)
	focalX := psxProjectionDistance * width / 320
	focalY := psxProjectionDistance * height / 240
	up := cross(ship.Forward, ship.Right)
	for _, polygon := range object.Polygons {
		if len(polygon.Indices) < 3 {
			continue
		}
		vertices := make([]perspectiveVertex, 0, len(polygon.Indices)+2)
		valid := true
		for _, index := range polygon.Indices {
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
			vertices = append(vertices, perspectiveVertex{position: camera.WorldToCamera(world)})
		}
		if !valid {
			continue
		}
		vertices = clipPerspectiveNearPlane(vertices, near)
		if len(vertices) < 3 {
			continue
		}
		color := polygonColor(polygon)
		for i := 1; i+1 < len(vertices); i++ {
			corners := [3]perspectiveVertex{vertices[0], vertices[i], vertices[i+1]}
			submitScreenTriangle(frame, corners, focalX, focalY, width, height, nil, color, true)
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
