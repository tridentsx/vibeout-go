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
	tileTextures                                 []*sdl.Texture
	tileColors                                   [][3]uint8
	scale, offsetX, offsetZ                      float32
	sceneryScale, sceneryOffsetX, sceneryOffsetZ float32
}

func NewTrackRenderer(renderer *sdl.Renderer, track *assets.Track, width, height float32) (*TrackRenderer, error) {
	if track == nil || len(track.Vertices) == 0 {
		return nil, fmt.Errorf("render: track has no vertices")
	}
	result := &TrackRenderer{track: track}
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
	result.tileTextures = make([]*sdl.Texture, len(track.Tiles))
	result.tileColors = make([][3]uint8, len(track.Tiles))
	for i, img := range track.Tiles {
		if img == nil {
			continue
		}
		result.tileColors[i] = averageColor(img)
		tex, err := renderer.CreateTexture(sdl.PIXELFORMAT_RGBA8888, sdl.TEXTUREACCESS_STATIC, img.Width, img.Height)
		if err != nil {
			continue
		}
		if err = tex.Update(nil, img.Pixels, int32(img.Width*4)); err != nil {
			tex.Destroy()
			continue
		}
		_ = tex.SetBlendMode(sdl.BLENDMODE_BLEND)
		result.tileTextures[i] = tex
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
		if texture != nil {
			texture.Destroy()
		}
	}
}

func (t *TrackRenderer) point(x, z int32) (float32, float32) {
	const margin float32 = 40
	return (float32(x)+t.offsetX)*t.scale + margin, (float32(z)+t.offsetZ)*t.scale + margin
}

func (t *TrackRenderer) Draw(renderer *sdl.Renderer) {
	if t == nil {
		return
	}
	uv := [4]sdl.FPoint{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}}
	for _, f := range t.track.Faces {
		var texture *sdl.Texture
		color := sdl.FColor{R: 1, G: 90.0 / 255, B: 200.0 / 255, A: 1}
		if int(f.Tile) < len(t.tileTextures) {
			texture = t.tileTextures[f.Tile]
		}
		if texture != nil {
			color = sdl.FColor{R: 1, G: 1, B: 1, A: 1}
		} else if int(f.Tile) < len(t.tileColors) {
			c := t.tileColors[f.Tile]
			if c != ([3]uint8{}) {
				color = sdl.FColor{R: float32(c[0]) / 255, G: float32(c[1]) / 255, B: float32(c[2]) / 255, A: 1}
			}
		}
		n := 4
		if f.Indices[2] == f.Indices[3] {
			n = 3
		}
		vertices := make([]sdl.Vertex, n)
		for i := 0; i < n; i++ {
			v := t.track.Vertices[f.Indices[i]]
			x, z := t.point(v.X, v.Z)
			vertices[i] = sdl.Vertex{Position: sdl.FPoint{X: x, Y: z}, Color: color, TexCoord: uv[i]}
		}
		indices := []int32{0, 1, 2}
		if n == 4 {
			indices = []int32{0, 1, 2, 0, 2, 3}
		}
		renderer.RenderGeometry(texture, vertices, indices)
	}
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
