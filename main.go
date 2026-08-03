package main

import (
	"log"
	"os"

	"github.com/Zyko0/go-sdl3/bin/binsdl"
	"github.com/Zyko0/go-sdl3/sdl"

	"github.com/tridentsx/wipeout-go/internal/game"
	"github.com/tridentsx/wipeout-go/internal/psx"
)

// wipeoutDiscPath is where this development machine's copy of the real
// WipEout 2097 disc image lives -- same convention as
// internal/psx/prm_realdata_test.go's constant of the same name (a
// different package, so not literally shared). Track rendering is skipped
// wherever this path doesn't exist rather than failing startup, since it's
// inherently machine-specific real game data, not a bundled asset.
const wipeoutDiscPath = "/Users/tridentsx/Downloads/WipeOut.2097.PAL-PSX/WIPEOUT2-disc/WIPEOUT2"

const (
	worldScale = 2.0 // ship-demo world units -> pixels
	originX    = 640.0
	originY    = 360.0
	shipHalfPx = 4.0
)

// cameraTargetShip picks which ship the demo camera follows (index into the
// ships slice below) -- the port's future "local player" concept, once
// there is one.
const cameraTargetShip = 0

func main() {
	defer binsdl.Load().Unload()
	defer sdl.Quit()

	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		log.Fatal(err)
	}

	window, renderer, err := sdl.CreateWindowAndRenderer("WipeOut", 1280, 720, 0)
	if err != nil {
		log.Fatal(err)
	}
	defer renderer.Destroy()
	defer window.Destroy()

	track := loadTrackScenery("TRACK01")
	surface := loadTrackSurface(renderer, "TRACK01")
	defer surface.destroy()

	ships := []*game.Ship{
		{Position: game.Vector3{X: 0, Y: 0, Z: 0}, Velocity: game.Vector3{X: 320, Z: 40}},
		{Position: game.Vector3{X: -60, Y: 0, Z: 20}, Velocity: game.Vector3{X: 260, Z: -30}},
		{Position: game.Vector3{X: 60, Y: 0, Z: -20}, Velocity: game.Vector3{X: 200, Z: 80}},
	}

	sdl.RunLoop(func() error {
		var event sdl.Event
		for sdl.PollEvent(&event) {
			if event.Type == sdl.EVENT_QUIT {
				return sdl.EndLoop
			}
		}

		for _, s := range ships {
			game.UpdatePhysics(s)
		}
		// See internal/game/camera.go: NewChaseCamera is an explicitly
		// PLACEHOLDER, not-reverse-engineered camera (open item, TODO.md
		// "Camera system"). Only applied to the ship demo below -- the
		// track's own rendering still uses its independent bounding-box fit
		// (loadTrackScenery/draw), since the ship-demo and real track data
		// are on unrelated coordinate scales that haven't been unified yet.
		camera := game.NewChaseCamera(ships[cameraTargetShip])

		renderer.SetDrawColor(10, 10, 20, 255)
		renderer.Clear()

		renderer.SetDrawColor(90, 90, 110, 255)
		track.draw(renderer)

		surface.draw(renderer) // sets its own color per-face (real tile colors, see trackSurface.draw)

		renderer.SetDrawColor(255, 255, 0, 255)
		surface.drawSections(renderer)

		renderer.SetDrawColor(0, 220, 255, 255)
		for _, s := range ships {
			cx, cz := camera.ProjectTopDown(s.Position)
			px := originX + cx*worldScale
			py := originY - cz*worldScale // camera forward (+Z) is "up" on screen
			renderer.RenderFillRect(&sdl.FRect{
				X: px - shipHalfPx,
				Y: py - shipHalfPx,
				W: shipHalfPx * 2,
				H: shipHalfPx * 2,
			})
		}

		renderer.Present()
		return nil
	})
}

// trackScenery is a loaded track's scenery objects (SCENE.PRM), pre-scaled
// and pre-offset to fit the demo window in a top-down (X/Z) projection --
// the real object-space transform (rotation matrix, per-object Position)
// isn't applied here, this is asset-loading validation, not the real
// renderer.
type trackScenery struct {
	objects []psx.Object
	scale   float32
	offsetX float32
	offsetZ float32
}

func loadTrackScenery(trackDir string) trackScenery {
	path := wipeoutDiscPath + "/" + trackDir + "/SCENE.PRM"
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("track scenery not loaded (%v) -- continuing without it", err)
		return trackScenery{}
	}

	objects, err := psx.DecodePRM(data)
	if err != nil && len(objects) == 0 {
		log.Printf("SCENE.PRM parse failed (%v) -- continuing without it", err)
		return trackScenery{}
	}

	minX, maxX := int32(0), int32(0)
	minZ, maxZ := int32(0), int32(0)
	for i, obj := range objects {
		x, z := obj.Header.Position.X, obj.Header.Position.Z
		if i == 0 || x < minX {
			minX = x
		}
		if i == 0 || x > maxX {
			maxX = x
		}
		if i == 0 || z < minZ {
			minZ = z
		}
		if i == 0 || z > maxZ {
			maxZ = z
		}
	}

	const margin = 40.0
	spanX := float32(maxX-minX) + 1
	spanZ := float32(maxZ-minZ) + 1
	scaleX := (1280 - 2*margin) / spanX
	scaleZ := (720 - 2*margin) / spanZ
	scale := scaleX
	if scaleZ < scale {
		scale = scaleZ
	}

	log.Printf("loaded %d scenery objects from %s (world span %.0f x %.0f, display scale %.4f)",
		len(objects), path, spanX, spanZ, scale)

	return trackScenery{
		objects: objects,
		scale:   scale,
		offsetX: -float32(minX),
		offsetZ: -float32(minZ),
	}
}

func (t trackScenery) draw(renderer *sdl.Renderer) {
	const margin = 40.0
	for _, obj := range t.objects {
		ox := (float32(obj.Header.Position.X)+t.offsetX)*t.scale + margin
		oz := (float32(obj.Header.Position.Z)+t.offsetZ)*t.scale + margin
		for _, poly := range obj.Polygons {
			for i := range poly.Indices {
				a := obj.Vertices[poly.Indices[i]]
				b := obj.Vertices[poly.Indices[(i+1)%len(poly.Indices)]]
				renderer.RenderLine(
					ox+float32(a.X)*t.scale, oz+float32(a.Z)*t.scale,
					ox+float32(b.X)*t.scale, oz+float32(b.Z)*t.scale,
				)
			}
		}
	}
}

// trackSurface is the track's actual driving mesh (TRACK.TRV vertices +
// TRACK.TRF faces) -- distinct from trackScenery's decorative objects.
// Vertices are already in absolute track space (no per-object origin to
// apply), so this uses its own independent bounding-box fit, same as
// trackScenery does for its own coordinate range.
type trackSurface struct {
	vertices []psx.TrackVertex
	faces    []psx.TrackFace
	sections []psx.TrackSection
	// tileTextures is each texture tile's real .TIM image, uploaded as an
	// SDL texture and indexed by TrackFace.Tile -- real UV-mapped texturing,
	// not the flat per-face average-color stand-in this replaced.
	tileTextures []*sdl.Texture
	// tileColors is the same per-tile average RGB as before, kept only as
	// the fallback color for tiles whose texture failed to load.
	tileColors [][3]uint8
	scale      float32
	offsetX    float32
	offsetZ    float32
}

func loadTrackSurface(renderer *sdl.Renderer, trackDir string) trackSurface {
	base := wipeoutDiscPath + "/" + trackDir
	trvData, err := os.ReadFile(base + "/TRACK.TRV")
	if err != nil {
		log.Printf("track surface not loaded (%v) -- continuing without it", err)
		return trackSurface{}
	}
	trfData, err := os.ReadFile(base + "/TRACK.TRF")
	if err != nil {
		log.Printf("track surface not loaded (%v) -- continuing without it", err)
		return trackSurface{}
	}

	vertices, err := psx.DecodeTRV(trvData)
	if err != nil {
		log.Printf("TRACK.TRV parse failed (%v) -- continuing without it", err)
		return trackSurface{}
	}
	faces, err := psx.DecodeTRF(trfData)
	if err != nil {
		log.Printf("TRACK.TRF parse failed (%v) -- continuing without it", err)
		return trackSurface{}
	}

	// TRACK.TRS (section graph) is optional -- the surface mesh itself
	// (TRV/TRF) is still useful without it, so a missing/bad .TRS logs and
	// continues rather than dropping the whole surface.
	var sections []psx.TrackSection
	if trsData, err := os.ReadFile(base + "/TRACK.TRS"); err != nil {
		log.Printf("track sections not loaded (%v)", err)
	} else if sections, err = psx.DecodeTRS(trsData); err != nil {
		log.Printf("TRACK.TRS parse failed (%v)", err)
		sections = nil
	}

	tileTextures, tileColors := loadTileTextures(renderer, base+"/TRACK.CMP")

	minX, maxX := int32(0), int32(0)
	minZ, maxZ := int32(0), int32(0)
	for i, v := range vertices {
		if i == 0 || v.X < minX {
			minX = v.X
		}
		if i == 0 || v.X > maxX {
			maxX = v.X
		}
		if i == 0 || v.Z < minZ {
			minZ = v.Z
		}
		if i == 0 || v.Z > maxZ {
			maxZ = v.Z
		}
	}

	const margin = 40.0
	spanX := float32(maxX-minX) + 1
	spanZ := float32(maxZ-minZ) + 1
	scaleX := (1280 - 2*margin) / spanX
	scaleZ := (720 - 2*margin) / spanZ
	scale := scaleX
	if scaleZ < scale {
		scale = scaleZ
	}

	log.Printf("loaded track surface: %d vertices, %d faces, %d sections (world span %.0f x %.0f, display scale %.4f)",
		len(vertices), len(faces), len(sections), spanX, spanZ, scale)

	return trackSurface{
		vertices:     vertices,
		faces:        faces,
		sections:     sections,
		tileTextures: tileTextures,
		tileColors:   tileColors,
		scale:        scale,
		offsetX:      -float32(minX),
		offsetZ:      -float32(minZ),
	}
}

// loadTileTextures decodes a .CMP texture bundle (e.g. TRACK.CMP) into one
// real SDL texture per member .TIM, plus that same image's average RGB as a
// fallback color for members that fail to decode or upload. Returns (nil,
// nil) if the bundle can't be loaded -- callers fall back to a flat draw
// color, same as when track data itself is missing.
func loadTileTextures(renderer *sdl.Renderer, cmpPath string) ([]*sdl.Texture, [][3]uint8) {
	data, err := os.ReadFile(cmpPath)
	if err != nil {
		log.Printf("tile textures not loaded (%v)", err)
		return nil, nil
	}
	members, err := psx.DecodeCMP(data)
	if err != nil {
		log.Printf("%s parse failed (%v)", cmpPath, err)
		return nil, nil
	}

	textures := make([]*sdl.Texture, len(members))
	colors := make([][3]uint8, len(members))
	loaded := 0
	for i, m := range members {
		img, err := psx.DecodeTIM(m)
		if err != nil {
			continue // not every CMP member is necessarily a TIM
		}

		var sumR, sumG, sumB, n uint64
		for p := 0; p+3 < len(img.Pixels); p += 4 {
			if img.Pixels[p+3] == 0 {
				continue // color-key transparent (pure black source pixel) -- not a real color
			}
			sumR += uint64(img.Pixels[p])
			sumG += uint64(img.Pixels[p+1])
			sumB += uint64(img.Pixels[p+2])
			n++
		}
		if n > 0 {
			colors[i] = [3]uint8{uint8(sumR / n), uint8(sumG / n), uint8(sumB / n)}
		}

		if img.Width == 0 || img.Height == 0 {
			continue
		}
		tex, err := renderer.CreateTexture(sdl.PIXELFORMAT_RGBA8888, sdl.TEXTUREACCESS_STATIC, img.Width, img.Height)
		if err != nil {
			log.Printf("tile %d: texture create failed (%v)", i, err)
			continue
		}
		if err := tex.Update(nil, img.Pixels, int32(img.Width*4)); err != nil {
			log.Printf("tile %d: texture upload failed (%v)", i, err)
			tex.Destroy()
			continue
		}
		// Color-key transparency (see tim.go) relies on alpha, so blend it.
		if err := tex.SetBlendMode(sdl.BLENDMODE_BLEND); err != nil {
			log.Printf("tile %d: blend mode failed (%v)", i, err)
		}
		textures[i] = tex
		loaded++
	}

	log.Printf("loaded %d/%d tile textures from %s", loaded, len(members), cmpPath)
	return textures, colors
}

func (t trackSurface) destroy() {
	for _, texture := range t.tileTextures {
		if texture != nil {
			texture.Destroy()
		}
	}
}

func (t trackSurface) draw(renderer *sdl.Renderer) {
	const margin = 40.0
	const fallbackR, fallbackG, fallbackB = 255, 90, 200 // no/blank tile data

	// Standard full-tile UV mapping: .TRF carries no per-vertex UVs (see
	// psx.TrackFace), just a tile index, so each face's corners map to a
	// tile's four corners in a fixed winding -- the only reasonable
	// inference for PS1-style tile-based track texturing.
	quadUV := [4]sdl.FPoint{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}}

	for _, f := range t.faces {
		var tex *sdl.Texture
		col := sdl.FColor{R: fallbackR / 255.0, G: fallbackG / 255.0, B: fallbackB / 255.0, A: 1}
		if int(f.Tile) < len(t.tileTextures) {
			tex = t.tileTextures[f.Tile]
		}
		if tex == nil && int(f.Tile) < len(t.tileColors) {
			if c := t.tileColors[f.Tile]; c != ([3]uint8{}) {
				col = sdl.FColor{R: float32(c[0]) / 255.0, G: float32(c[1]) / 255.0, B: float32(c[2]) / 255.0, A: 1}
			}
		}
		if tex != nil {
			// White so the texture's own colors show through unmodulated.
			col = sdl.FColor{R: 1, G: 1, B: 1, A: 1}
		}

		n := 4
		if f.Indices[2] == f.Indices[3] {
			n = 3 // triangle: last index repeats, per wipeout.js's TrackFace layout
		}
		verts := make([]sdl.Vertex, n)
		for i := 0; i < n; i++ {
			v := t.vertices[f.Indices[i]]
			verts[i] = sdl.Vertex{
				Position: sdl.FPoint{
					X: (float32(v.X)+t.offsetX)*t.scale + margin,
					Y: (float32(v.Z)+t.offsetZ)*t.scale + margin,
				},
				Color:    col,
				TexCoord: quadUV[i],
			}
		}
		indices := []int32{0, 1, 2}
		if n == 4 {
			indices = []int32{0, 1, 2, 0, 2, 3} // fan-triangulate the quad
		}
		renderer.RenderGeometry(tex, verts, indices)
	}
}

// drawSections renders the section graph's spine -- a line from each
// section to its Next neighbor (Previous is redundant, every link is
// walked twice over the whole graph; NextJunction fork points aren't drawn
// separately here) -- using the same scale/offset as draw() so it lines up
// with the surface mesh it indexes into.
func (t trackSurface) drawSections(renderer *sdl.Renderer) {
	const margin = 40.0
	for _, s := range t.sections {
		if s.Next < 0 || int(s.Next) >= len(t.sections) {
			continue
		}
		next := t.sections[s.Next]
		ax := (float32(s.X)+t.offsetX)*t.scale + margin
		az := (float32(s.Z)+t.offsetZ)*t.scale + margin
		bx := (float32(next.X)+t.offsetX)*t.scale + margin
		bz := (float32(next.Z)+t.offsetZ)*t.scale + margin
		renderer.RenderLine(ax, az, bx, bz)
	}
}
