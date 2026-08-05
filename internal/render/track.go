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
	device                                       *Device
	tileTextures                                 []*sdl.GPUTexture
	tileColors                                   [][3]uint8
	sceneryTextures                              []*sdl.GPUTexture
	skyTextures                                  []*sdl.GPUTexture
	scale, offsetX, offsetZ                      float32
	sceneryScale, sceneryOffsetX, sceneryOffsetZ float32
}

// psxProjectionDistance is the GTE projection-plane distance h. WipEout's
// documented FOV is 90 degrees horizontal (wipeout-rewrite render_gl.c:492-497
// derives its matching 73.75-degree vertical FOV from that figure), and the
// standard GTE convention for a 90-degree horizontal FOV is h == screenWidth/2
// so that atan((width/2)/h) == 45 degrees. The previous value of 1000 cited
// SLES_003.27 0x8008008c ("InitGTEProjectionState"), but that symbol is a
// phantom entry in the recovered database: Binary Ninja's function_at
// resolves that address into an unrelated function (sub_8007ffc0), so the
// citation cannot be trusted. 1000 produced an ~18-degree horizontal FOV,
// consistent with the reported symptoms (chase camera reads as glued to the
// ship's cockpit, wingtips clipping into view, ship dropping below frame).
const psxProjectionDistance = float32(160)

type perspectiveVertex struct {
	position game.Vector3
	uv       sdl.FPoint
}

// prmQuadPerimeterOrder remaps a PRM quad's four Indices/UV entries from
// their on-disk "Z" order (0=top-left, 1=top-right, 2=bottom-left,
// 3=bottom-right) to boundary/perimeter order, confirmed against
// wipeout.js's createModelFromObject: it always triangulates a quad as
// {indices[2],indices[1],indices[0]} and {indices[2],indices[3],indices[1]},
// i.e. sharing the 1-2 edge, not 0-2. A naive fan pivoting on vertex 0 (0,1,2
// then 0,2,3) is only correct if the four vertices already wind around the
// quad's boundary; PRM's Z order makes 0-2 a side rather than a diagonal, so
// that fan splits the quad along the wrong line and leaves a visible gap
// covering roughly half the square. TRACK.TRF faces are unaffected -- that
// format already stores boundary order (confirmed by its own UV winding and
// the existing, working track floor render).
var prmQuadPerimeterOrder = [4]int{0, 1, 3, 2}

// quadIndexOrder returns the perimeter-order position for slot i of an n-way
// PRM polygon (n==3: unchanged; n==4: see prmQuadPerimeterOrder).
func quadIndexOrder(n, i int) int {
	if n == 4 {
		return prmQuadPerimeterOrder[i]
	}
	return i
}

// DrawPerspective submits the track through the original camera coordinate
// system. See psxProjectionDistance for how the GTE's H parameter is derived;
// scaling it with the output width keeps the same horizontal field of view
// when the presentation window is enlarged.
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
	drawObjectsPerspective(frame, camera, t.track.Scenery, t.sceneryTextures, t.track.SceneryTiles, 1, width, height)
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
	drawObjectsPerspective(frame, camera, t.track.Sky, t.skyTextures, t.track.SkyTiles, skyScale, width, height)
}

// drawObjectsPerspective submits a flat list of PRM objects (scenery or sky)
// through the camera. objectScale enlarges each vertex's offset from its
// object's Position before placing it in world space (see DrawSkyPerspective
// for why sky needs this and scenery doesn't). images is the same-order,
// same-index decoded source for textures (see textureDimensions) so a
// polygon's raw PRM UV bytes can be normalized against its own texture's
// actual pixel size.
func drawObjectsPerspective(frame *Frame, camera Camera, objects []assets.Object, textures []*sdl.GPUTexture, images []*assets.Image, objectScale, width, height float32) {
	if frame == nil {
		return
	}
	const near = float32(200)
	focalX := psxProjectionDistance * width / 320
	focalY := psxProjectionDistance * height / 240
	right, _, _ := camera.Basis()
	right.Y = 0
	if rightLength := length(right); rightLength > 0 {
		right = mul(right, 1/rightLength)
	} else {
		right = game.Vector3{X: 1}
	}
	for _, obj := range objects {
		for _, polygon := range obj.Polygons {
			if polygon.Type == assets.PolygonSpriteTopAnchor || polygon.Type == assets.PolygonSpriteBottomAnchor {
				submitSpritePerspective(frame, camera, right, obj, polygon, textures, objectScale, focalX, focalY, width, height, near)
				continue
			}
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
				submitScreenTriangle(frame, corners, focalX, focalY, width, height, texture, color, false)
			}
		}
	}
}

// textureDimensions returns the pixel width/height of the texture a polygon
// references, so its PRM UV bytes -- raw texel offsets into that specific
// texture, not a fixed 0-255 page (confirmed by internal/model's normUV,
// which already divides by the referenced texture page's own W/H for the
// glTF export path) -- normalize to the correct 0..1 fraction. Getting this
// wrong under-divides any texture narrower than 256px (e.g. TRACK01's 128x128
// SCENE.CMP billboards), sampling only its left/top fraction: the FSOL
// billboard's "FSOL" text rendered as "FS" because /255 only reached u<=0.5
// of a 128px-wide image. Falls back to 255 (a no-op scale, matching the old
// behavior) when the texture index is absent or out of range.
func textureDimensions(images []*assets.Image, texture *uint16) (float32, float32) {
	if texture != nil {
		index := int(*texture)
		if index >= 0 && index < len(images) && images[index] != nil {
			return float32(images[index].Width), float32(images[index].Height)
		}
	}
	return 255, 255
}

// submitSpritePerspective draws a PRM sprite polygon (types 10/11) as a
// camera-facing billboard. Confirmed against wipeout.js's
// createModelFromObject: the sprite's anchor vertex is one of the object's
// own vertices (SpriteIndex), the quad is centered on that anchor with a
// vertical offset of height/2 (toward the object for a bottom anchor, away
// from it for a top anchor -- ceiling fixtures use the bottom anchor so the
// sprite hangs below its mount point), and it only rotates around the world
// Y axis to face the camera (never tilting with camera pitch), which is why
// `right` is computed once, yaw-only, by the caller and passed in rather
// than reading camera.Basis per sprite.
func submitSpritePerspective(frame *Frame, camera Camera, right game.Vector3, obj assets.Object, polygon assets.Polygon, textures []*sdl.GPUTexture, objectScale, focalX, focalY, width, height, near float32) {
	if int(polygon.SpriteIndex) >= len(obj.Vertices) {
		return
	}
	anchor := obj.Vertices[polygon.SpriteIndex]
	center := game.Vector3{
		X: float32(obj.Header.Position.X) + float32(anchor.X)*objectScale,
		Y: float32(obj.Header.Position.Y) + float32(anchor.Y)*objectScale,
		Z: float32(obj.Header.Position.Z) + float32(anchor.Z)*objectScale,
	}
	// wipeout-rewrite's render_push_sprite (render_gl.c:774-780) uses
	// poly.spr->width/height directly as world-space half-extents (size.x *
	// 0.5) with no scaling division, matching the same raw-vertex-unit
	// convention as every other PRM field this renderer already trusts
	// as-is (ship hull, scenery quads, track faces). An earlier version of
	// this function divided by 16 based on a same-texture sample that
	// happened to cluster near that ratio; sampling every sprite in
	// TRACK01's SCENE.PRM shows the ratio actually spans 15x-63x for
	// sprites sharing that identical texture, so it was never a genuine
	// texture-relative constant -- just a coincidence of a small sample.
	halfWidth := float32(polygon.SpriteWidth) / 2 * objectScale
	halfHeight := float32(polygon.SpriteHeight) / 2 * objectScale
	if polygon.Type == assets.PolygonSpriteBottomAnchor {
		center.Y -= halfHeight
	} else {
		center.Y += halfHeight
	}

	corners := [4]game.Vector3{
		{X: center.X - right.X*halfWidth, Y: center.Y - halfHeight, Z: center.Z - right.Z*halfWidth},
		{X: center.X + right.X*halfWidth, Y: center.Y - halfHeight, Z: center.Z + right.Z*halfWidth},
		{X: center.X + right.X*halfWidth, Y: center.Y + halfHeight, Z: center.Z + right.Z*halfWidth},
		{X: center.X - right.X*halfWidth, Y: center.Y + halfHeight, Z: center.Z - right.Z*halfWidth},
	}
	uv := [4]sdl.FPoint{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}}

	vertices := make([]perspectiveVertex, 4)
	for i, corner := range corners {
		vertices[i] = perspectiveVertex{position: camera.WorldToCamera(corner), uv: uv[i]}
	}
	vertices = clipPerspectiveNearPlane(vertices, near)
	if len(vertices) < 3 {
		return
	}
	texture, color := objectPresentation(textures, polygon)
	for i := 1; i+1 < len(vertices); i++ {
		triangle := [3]perspectiveVertex{vertices[0], vertices[i], vertices[i+1]}
		submitScreenTriangle(frame, triangle, focalX, focalY, width, height, texture, color, false)
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
