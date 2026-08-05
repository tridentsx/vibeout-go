package render

import (
	"fmt"

	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/game"
)

// TrackRenderer owns GPU resources and presentation-only transforms for a
// loaded track. It does not read files or advance simulation state.
type TrackRenderer struct {
	track                                        *assets.Track
	device                                        *Device
	tileTextures                                 []*sdl.GPUTexture
	tileColors                                   [][3]uint8
	sceneryTextures                               []*sdl.GPUTexture
	skyTextures                                   []*sdl.GPUTexture
	scale, offsetX, offsetZ                      float32
	sceneryScale, sceneryOffsetX, sceneryOffsetZ float32
}

const psxProjectionDistance = float32(1000) // InitGTEProjectionState, 0x8008008c.

type perspectiveVertex struct {
	position game.Vector3
	uv       sdl.FPoint
}

// DrawPerspective submits the track through the original camera coordinate
// system. InitGTEProjectionState configures the retail executable's GTE with
// H=1000 for a 320-pixel-wide display; scaling H with the output width keeps
// that horizontal field of view when the presentation window is enlarged.
//
// Every face is submitted unconditionally, every frame -- confirmed against
// both reference projects (wipeout.js and wipeout-rewrite): neither reads
// TRACK.VEW at all, they just draw the whole track and let the renderer's
// own clipping handle the rest. An earlier version of this function tried to
// replicate the PS1's own TRACK.VEW-based partial-visibility scheme (needed
// on 1994 hardware, not on a modern GPU rendering ~1400 quads per frame) and
// it was never a perfect stand-in: it produced real, hard-to-fully-close
// gaps wherever its neighbor-distance/VEW heuristic didn't happen to cover
// the camera's actual view. Depth testing (see internal/render/gpu.go)
// already makes draw order irrelevant, so there's no correctness reason to
// cull at all here.
func (t *TrackRenderer) DrawPerspective(frame *Frame, camera Camera, width, height float32) {
	if t == nil {
		return
	}

	// near is in world units (thousands-scale track geometry): a threshold of
	// 1 lets faces with a near-zero camera-space Z through, and dividing by
	// that near-zero Z during projection stretches them into screen-filling
	// slivers. A few hundred units keeps that singularity out of range.
	const near = float32(200)
	focalX := psxProjectionDistance * width / 320
	focalY := psxProjectionDistance * height / 240
	uv := [4]sdl.FPoint{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}}
	for _, face := range t.track.Faces {
		n := 4
		if face.Indices[2] == face.Indices[3] {
			n = 3
		}
		polygon := make([]perspectiveVertex, 0, n+2)
		valid := true
		for i := 0; i < n; i++ {
			index := int(face.Indices[i])
			if index >= len(t.track.Vertices) {
				valid = false
				break
			}
			v := t.track.Vertices[index]
			polygon = append(polygon, perspectiveVertex{
				position: camera.WorldToCamera(game.Vector3{X: float32(v.X), Y: float32(v.Y), Z: float32(v.Z)}),
				uv:       uv[i],
			})
		}
		if !valid {
			continue
		}
		polygon = clipPerspectiveNearPlane(polygon, near)
		if len(polygon) < 3 {
			continue
		}

		texture, color := t.facePresentation(face.Tile)
		for i := 1; i+1 < len(polygon); i++ {
			corners := [3]perspectiveVertex{polygon[0], polygon[i], polygon[i+1]}
			submitScreenTriangle(frame, corners, focalX, focalY, width, height, texture, color, false)
		}
	}
}

func (t *TrackRenderer) facePresentation(tile uint8) (*sdl.GPUTexture, sdl.FColor) {
	color := sdl.FColor{R: 1, G: 90.0 / 255, B: 200.0 / 255, A: 1}
	if int(tile) < len(t.tileTextures) && t.tileTextures[tile] != nil {
		return t.tileTextures[tile], sdl.FColor{R: 1, G: 1, B: 1, A: 1}
	}
	if int(tile) < len(t.tileColors) {
		average := t.tileColors[tile]
		if average != ([3]uint8{}) {
			color = sdl.FColor{R: float32(average[0]) / 255, G: float32(average[1]) / 255, B: float32(average[2]) / 255, A: 1}
		}
	}
	return nil, color
}

func clipPerspectiveNearPlane(polygon []perspectiveVertex, near float32) []perspectiveVertex {
	if len(polygon) == 0 {
		return nil
	}
	result := make([]perspectiveVertex, 0, len(polygon)+2)
	previous := polygon[len(polygon)-1]
	previousInside := previous.position.Z >= near
	for _, current := range polygon {
		currentInside := current.position.Z >= near
		if currentInside != previousInside {
			ratio := (near - previous.position.Z) / (current.position.Z - previous.position.Z)
			result = append(result, perspectiveVertex{
				position: add(previous.position, mul(sub(current.position, previous.position), ratio)),
				uv: sdl.FPoint{
					X: previous.uv.X + (current.uv.X-previous.uv.X)*ratio,
					Y: previous.uv.Y + (current.uv.Y-previous.uv.Y)*ratio,
				},
			})
		}
		if currentInside {
			result = append(result, current)
		}
		previous, previousInside = current, currentInside
	}
	return result
}

func NewTrackRenderer(device *Device, track *assets.Track, width, height float32) (*TrackRenderer, error) {
	if track == nil || len(track.Vertices) == 0 {
		return nil, fmt.Errorf("render: track has no vertices")
	}
	result := &TrackRenderer{track: track, device: device}
	minX, maxX := track.Vertices[0].X, track.Vertices[0].X
	minZ, maxZ := track.Vertices[0].Z, track.Vertices[0].Z
	for _, v := range track.Vertices[1:] {
		if v.X < minX {
			minX = v.X
		}
		if v.X > maxX {
			maxX = v.X
		}
		if v.Z < minZ {
			minZ = v.Z
		}
		if v.Z > maxZ {
			maxZ = v.Z
		}
	}
	const margin float32 = 40
	sx, sz := (width-2*margin)/(float32(maxX-minX)+1), (height-2*margin)/(float32(maxZ-minZ)+1)
	result.scale = sx
	if sz < sx {
		result.scale = sz
	}
	result.offsetX = -float32(minX)
	result.offsetZ = -float32(minZ)
	if len(track.Scenery) > 0 {
		minX, maxX = track.Scenery[0].Header.Position.X, track.Scenery[0].Header.Position.X
		minZ, maxZ = track.Scenery[0].Header.Position.Z, track.Scenery[0].Header.Position.Z
		for _, obj := range track.Scenery[1:] {
			x, z := obj.Header.Position.X, obj.Header.Position.Z
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if z < minZ {
				minZ = z
			}
			if z > maxZ {
				maxZ = z
			}
		}
		sx = (width - 2*margin) / (float32(maxX-minX) + 1)
		sz = (height - 2*margin) / (float32(maxZ-minZ) + 1)
		result.sceneryScale = sx
		if sz < sx {
			result.sceneryScale = sz
		}
		result.sceneryOffsetX = -float32(minX)
		result.sceneryOffsetZ = -float32(minZ)
	}
	result.tileTextures = make([]*sdl.GPUTexture, len(track.Tiles))
	result.tileColors = make([][3]uint8, len(track.Tiles))
	for i, img := range track.Tiles {
		if img == nil {
			continue
		}
		result.tileColors[i] = averageColor(img)
		tex, err := device.NewTexture(int(img.Width), int(img.Height), img.Pixels)
		if err != nil {
			continue
		}
		result.tileTextures[i] = tex
	}
	result.sceneryTextures = make([]*sdl.GPUTexture, len(track.SceneryTiles))
	for i, img := range track.SceneryTiles {
		if img == nil {
			continue
		}
		tex, err := device.NewTexture(int(img.Width), int(img.Height), img.Pixels)
		if err != nil {
			continue
		}
		result.sceneryTextures[i] = tex
	}
	result.skyTextures = make([]*sdl.GPUTexture, len(track.SkyTiles))
	for i, img := range track.SkyTiles {
		if img == nil {
			continue
		}
		tex, err := device.NewTexture(int(img.Width), int(img.Height), img.Pixels)
		if err != nil {
			continue
		}
		result.skyTextures[i] = tex
	}
	return result, nil
}

func averageColor(img *assets.Image) [3]uint8 {
	var r, g, b, n uint64
	for p := 0; p+3 < len(img.Pixels); p += 4 {
		if img.Pixels[p+3] == 0 {
			continue
		}
		r += uint64(img.Pixels[p])
		g += uint64(img.Pixels[p+1])
		b += uint64(img.Pixels[p+2])
		n++
	}
	if n == 0 {
		return [3]uint8{}
	}
	return [3]uint8{uint8(r / n), uint8(g / n), uint8(b / n)}
}

func (t *TrackRenderer) Destroy() {
	if t == nil {
		return
	}
	for _, texture := range t.tileTextures {
		if texture != nil && t.device != nil {
			t.device.ReleaseTexture(texture)
		}
	}
	for _, texture := range t.sceneryTextures {
		if texture != nil && t.device != nil {
			t.device.ReleaseTexture(texture)
		}
	}
	for _, texture := range t.skyTextures {
		if texture != nil && t.device != nil {
			t.device.ReleaseTexture(texture)
		}
	}
}

// DrawSceneryPerspective submits SCENE.PRM's decorative track-side objects
// (billboards, structures) through the same camera coordinate system as
// DrawPerspective. Scenery objects carry no rotation in ObjectHeader --
// their vertices are placed directly at Header.Position in world axes, the
// same assumption the existing DrawScenery top-down helper already makes.
// SCENE.CMP's textures (up to 128x128, real color) are the source of most of
// the track's visual color; TRACK.CMP's own tiles are uniformly small 32x32
// grayscale metal-floor material.
// DrawSceneryPerspective submits SCENE.PRM's decorative track-side objects
// (billboards, structures) at their authored scale (1x).
func (t *TrackRenderer) DrawSceneryPerspective(frame *Frame, camera Camera, width, height float32) {
	drawObjectsPerspective(frame, camera, t.track.Scenery, t.sceneryTextures, 1, width, height)
}

// DrawSkyPerspective submits SKY.PRM's horizon backdrop. Confirmed against
// wipeout.js's loadTrack/createScene: sky objects are authored small and
// placed via createScene(files, {scale: 48}), i.e. their local vertex
// offsets (not their Header.Position, which Three.js keeps as a separate,
// unscaled translation) are enlarged 48x to form a distant-looking dome.
// Without this, there was a real gap between the near track/scenery geometry
// and the horizon -- confirmed with a magenta clear-color diagnostic showing
// the clear color through that gap, i.e. genuinely no geometry there.
func (t *TrackRenderer) DrawSkyPerspective(frame *Frame, camera Camera, width, height float32) {
	const skyScale = 48
	drawObjectsPerspective(frame, camera, t.track.Sky, t.skyTextures, skyScale, width, height)
}

// drawObjectsPerspective submits a flat list of PRM objects (scenery or sky)
// through the camera. objectScale enlarges each vertex's offset from its
// object's Position before placing it in world space (see DrawSkyPerspective
// for why sky needs this and scenery doesn't).
func drawObjectsPerspective(frame *Frame, camera Camera, objects []assets.Object, textures []*sdl.GPUTexture, objectScale, width, height float32) {
	if frame == nil {
		return
	}
	const near = float32(200)
	focalX := psxProjectionDistance * width / 320
	focalY := psxProjectionDistance * height / 240
	for _, obj := range objects {
		for _, polygon := range obj.Polygons {
			if len(polygon.Indices) < 3 {
				continue
			}
			vertices := make([]perspectiveVertex, 0, len(polygon.Indices)+2)
			valid := true
			for i, index := range polygon.Indices {
				if int(index) >= len(obj.Vertices) {
					valid = false
					break
				}
				v := obj.Vertices[index]
				// Header.Origin equals Header.Position for both scenery and sky
				// objects (confirmed by direct inspection), so subtracting it here --
				// as DrawShipPerspective does for craft, where Origin and Position
				// are independent -- would cancel Position out entirely and leave
				// vertices at raw local-space coordinates near the world origin. The
				// existing 2D DrawScenery helper already gets this right: only
				// Position offsets the vertex.
				world := game.Vector3{
					X: float32(obj.Header.Position.X) + float32(v.X)*objectScale,
					Y: float32(obj.Header.Position.Y) + float32(v.Y)*objectScale,
					Z: float32(obj.Header.Position.Z) + float32(v.Z)*objectScale,
				}
				var uv sdl.FPoint
				if i < len(polygon.UV) {
					uv = sdl.FPoint{X: float32(polygon.UV[i].U) / 255, Y: float32(polygon.UV[i].V) / 255}
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
				submitScreenTriangle(frame, corners, focalX, focalY, width, height, texture, color, false)
			}
		}
	}
}

func objectPresentation(textures []*sdl.GPUTexture, polygon assets.Polygon) (*sdl.GPUTexture, sdl.FColor) {
	if polygon.Texture != nil {
		index := int(*polygon.Texture)
		if index >= 0 && index < len(textures) && textures[index] != nil {
			return textures[index], sdl.FColor{R: 1, G: 1, B: 1, A: 1}
		}
	}
	return nil, polygonColor(polygon)
}

func (t *TrackRenderer) point(x, z int32) (float32, float32) {
	const margin float32 = 40
	return (float32(x)+t.offsetX)*t.scale + margin, (float32(z)+t.offsetZ)*t.scale + margin
}

func (t *TrackRenderer) DrawScenery(renderer *sdl.Renderer) {
	if t == nil {
		return
	}
	const margin float32 = 40
	for _, obj := range t.track.Scenery {
		ox := (float32(obj.Header.Position.X)+t.sceneryOffsetX)*t.sceneryScale + margin
		oz := (float32(obj.Header.Position.Z)+t.sceneryOffsetZ)*t.sceneryScale + margin
		for _, poly := range obj.Polygons {
			for i := range poly.Indices {
				a := obj.Vertices[poly.Indices[i]]
				b := obj.Vertices[poly.Indices[(i+1)%len(poly.Indices)]]
				renderer.RenderLine(ox+float32(a.X)*t.sceneryScale, oz+float32(a.Z)*t.sceneryScale, ox+float32(b.X)*t.sceneryScale, oz+float32(b.Z)*t.sceneryScale)
			}
		}
	}
}

func (t *TrackRenderer) DrawSections(renderer *sdl.Renderer) {
	if t == nil {
		return
	}
	for _, section := range t.track.Sections {
		if section.Next < 0 || int(section.Next) >= len(t.track.Sections) {
			continue
		}
		next := t.track.Sections[section.Next]
		ax, az := t.point(section.X, section.Z)
		bx, bz := t.point(next.X, next.Z)
		renderer.RenderLine(ax, az, bx, bz)
	}
}

func DrawShipsTopDown(renderer *sdl.Renderer, camera Camera, ships []*game.Ship, originX, originY, worldScale float32) {
	const half float32 = 4
	for _, ship := range ships {
		x, z := camera.ProjectTopDown(ship.Position)
		px, py := originX+x*worldScale, originY-z*worldScale
		renderer.RenderFillRect(&sdl.FRect{X: px - half, Y: py - half, W: half * 2, H: half * 2})
	}
}
