package render

import (
	"sort"

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
func DrawShipPerspective(renderer *sdl.Renderer, camera Camera, ship *game.Ship, object *assets.Object, width, height float32) {
	if renderer == nil || ship == nil || object == nil {
		return
	}
	const near = float32(1)
	focalX := psxProjectionDistance * width / 320
	focalY := psxProjectionDistance * height / 240
	up := cross(ship.Forward, ship.Right)
	triangles := make([]perspectiveTriangle, 0, len(object.Polygons)*2)
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
			triangle := perspectiveTriangle{}
			for j, corner := range corners {
				triangle.vertices[j] = sdl.Vertex{
					Position: sdl.FPoint{X: width/2 + corner.position.X*focalX/corner.position.Z, Y: height/2 + corner.position.Y*focalY/corner.position.Z},
					Color:    color,
				}
				triangle.depth += corner.position.Z / 3
			}
			if backFacing(triangle.vertices) {
				continue
			}
			triangles = append(triangles, triangle)
		}
	}
	sort.Slice(triangles, func(i, j int) bool { return triangles[i].depth > triangles[j].depth })
	for _, triangle := range triangles {
		renderer.RenderGeometry(nil, triangle.vertices[:], nil)
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

// backFacing reports whether a screen-space triangle winds away from the
// viewer, using the signed area of its projected 2D vertices. PS1 PRM/TRV
// data is authored with a fixed winding convention; this is a standard
// screen-space backface test, independent of any 3D normal convention.
func backFacing(v [3]sdl.Vertex) bool {
	area := (v[1].Position.X-v[0].Position.X)*(v[2].Position.Y-v[0].Position.Y) -
		(v[2].Position.X-v[0].Position.X)*(v[1].Position.Y-v[0].Position.Y)
	return area > 0
}

func cross(a, b game.Vector3) game.Vector3 {
	return game.Vector3{X: a.Y*b.Z - a.Z*b.Y, Y: a.Z*b.X - a.X*b.Z, Z: a.X*b.Y - a.Y*b.X}
}
