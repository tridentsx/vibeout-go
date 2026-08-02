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

		renderer.SetDrawColor(10, 10, 20, 255)
		renderer.Clear()

		renderer.SetDrawColor(90, 90, 110, 255)
		track.draw(renderer)

		renderer.SetDrawColor(0, 220, 255, 255)
		for _, s := range ships {
			px := originX + s.Position.X*worldScale
			py := originY + s.Position.Z*worldScale
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
		ox := (float32(obj.Header.Position.X) + t.offsetX) * t.scale + margin
		oz := (float32(obj.Header.Position.Z) + t.offsetZ) * t.scale + margin
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
