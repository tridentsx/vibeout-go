package render

import (
	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/game"
)

// Menu models: the front end shows a rotating craft or track shape rather than a
// text list. The models come from COMMON's PRMs -- VECTO/VENOM/RAPIE/PHANT for the
// classes, TERRY for the teams, WIERD for the animal teams and JUNE for the track
// previews. See bn-psx/docs/wipeout2097_menu_system.md.
//
// This does not reproduce retail's own carousel layout, which has not been reversed;
// it draws one model, centred and spinning, which is the shape of the real screens.

// MenuModelSpinRate is how far the model turns per tick, in the port's Angle units
// (4096 to a full turn). Retail's rate has not been recovered, so this is chosen to
// read well at 25 Hz -- a full revolution takes about six seconds.
const MenuModelSpinRate = 28

// DrawMenuModel draws one object centred on (centerX, centerY) in retail
// coordinates, spun about its vertical axis by angle and scaled so its largest
// dimension fills fillPixels.
//
// Depth comes from each vertex's own camera-space Z, so the model's faces occlude
// each other correctly, unlike the flat per-draw layering the 2D helpers use. The
// distance is chosen to keep the whole model inside UIBandModelNear..UIBandModelFar,
// between the background art and the labels.
func DrawMenuModel(frame *Frame, ui *UI, object *assets.Object, textures []*sdl.GPUTexture,
	images []*assets.Image, centerX, centerY int, fillPixels float32, angle game.Angle) {
	if frame == nil || ui == nil || object == nil || len(object.Polygons) == 0 {
		return
	}

	// Model extent about its origin, so the scale can be derived rather than tuned
	// per model -- the class craft, team craft and track shapes differ by orders of
	// magnitude in native units.
	var maxExtent float32
	for _, v := range object.Vertices {
		for _, c := range [3]float32{
			float32(v.X) - float32(object.Header.Origin.X),
			float32(v.Y) - float32(object.Header.Origin.Y),
			float32(v.Z) - float32(object.Header.Origin.Z),
		} {
			if c < 0 {
				c = -c
			}
			if c > maxExtent {
				maxExtent = c
			}
		}
	}
	if maxExtent == 0 {
		return
	}

	// Place the model mid-band and pick a scale that projects maxExtent to half of
	// fillPixels at that distance.
	const distance = float32((UIBandModelFar + UIBandModelNear) / 2)
	focal := float32(psxProjectionDistance)
	scale := (fillPixels / 2) * distance / (focal * maxExtent)

	sin, cos := angle.Sin(), angle.Cos()

	for _, polygon := range object.Polygons {
		if len(polygon.Indices) < 3 {
			continue
		}
		uvWidth, uvHeight := textureDimensions(images, polygon.Texture)
		n := len(polygon.Indices)
		projected := make([]Vertex, 0, n)
		// objectPresentation resolves the texture and flat colour together, the same
		// way the ship path does, so untextured polygons get their PRM colour.
		texture, colour := objectPresentation(textures, polygon)
		valid := true

		for slot := 0; slot < n; slot++ {
			i := quadIndexOrder(n, slot)
			index := polygon.Indices[i]
			if int(index) >= len(object.Vertices) {
				valid = false
				break
			}
			v := object.Vertices[index]
			lx := (float32(v.X) - float32(object.Header.Origin.X)) * scale
			ly := (float32(v.Y) - float32(object.Header.Origin.Y)) * scale
			lz := (float32(v.Z) - float32(object.Header.Origin.Z)) * scale

			// Spin about Y, then push away from the camera.
			rx := lx*cos + lz*sin
			rz := -lx*sin + lz*cos
			z := distance + rz
			if z < depthNear {
				valid = false
				break
			}

			// Project in retail coordinates, then hand off to the UI's mapping so the
			// model lands in the same frame as the labels. Y is negated because model
			// space has -Y up while screen space grows downwards.
			sx := float32(centerX) + rx*focal/z
			sy := float32(centerY) + ly*focal/z
			ndcX, ndcY := ui.ndc(sx, sy)

			var uv sdl.FPoint
			if i < len(polygon.UV) && uvWidth > 0 && uvHeight > 0 {
				uv = sdl.FPoint{
					X: float32(polygon.UV[i].U) / uvWidth,
					Y: float32(polygon.UV[i].V) / uvHeight,
				}
			}
			projected = append(projected, Vertex{
				X: ndcX, Y: ndcY, Z: z,
				U: uv.X, V: uv.Y,
				R: colour.R, G: colour.G, B: colour.B, A: 1,
			})
		}
		if !valid || len(projected) < 3 {
			continue
		}

		if texture == nil {
			texture = ui.white
		}
		// Fan the polygon into triangles.
		for i := 2; i < len(projected); i++ {
			frame.Submit([3]Vertex{projected[0], projected[i-1], projected[i]}, texture)
		}
	}
}
