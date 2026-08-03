package main

import (
	"log"

	"github.com/Zyko0/go-sdl3/bin/binsdl"
	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/game"
	"github.com/tridentsx/wipeout-go/internal/physics"
	gameRender "github.com/tridentsx/wipeout-go/internal/render"
)

const wipeoutDiscPath = "/Users/tridentsx/Downloads/WipeOut.2097.PAL-PSX/WIPEOUT2-disc/WIPEOUT2"

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

	trackAsset, err := (assets.Loader{Root: wipeoutDiscPath}).LoadTrack("TRACK01")
	if err != nil {
		log.Printf("track not loaded (%v) -- continuing without it", err)
	}
	if trackAsset != nil {
		for _, warning := range trackAsset.Warnings {
			log.Printf("optional track asset: %v", warning)
		}
	}
	trackRenderer, err := gameRender.NewTrackRenderer(renderer, trackAsset, 1280, 720)
	if err != nil {
		log.Printf("track renderer unavailable (%v)", err)
	}
	defer trackRenderer.Destroy()

	ships := []*game.Ship{
		{Position: game.Vector3{}, Velocity: game.Vector3{X: 320, Z: 40}},
		{Position: game.Vector3{X: -60, Z: 20}, Velocity: game.Vector3{X: 260, Z: -30}},
		{Position: game.Vector3{X: 60, Z: -20}, Velocity: game.Vector3{X: 200, Z: 80}},
	}

	sdl.RunLoop(func() error {
		var event sdl.Event
		for sdl.PollEvent(&event) {
			if event.Type == sdl.EVENT_QUIT {
				return sdl.EndLoop
			}
		}
		for _, ship := range ships {
			physics.UpdatePhysics(ship)
		}
		camera := gameRender.NewChaseCamera(ships[0])
		renderer.SetDrawColor(10, 10, 20, 255)
		renderer.Clear()
		renderer.SetDrawColor(90, 90, 110, 255)
		trackRenderer.DrawScenery(renderer)
		trackRenderer.Draw(renderer)
		renderer.SetDrawColor(255, 255, 0, 255)
		trackRenderer.DrawSections(renderer)
		renderer.SetDrawColor(0, 220, 255, 255)
		gameRender.DrawShipsTopDown(renderer, camera, ships, 640, 360, 2)
		renderer.Present()
		return nil
	})
}
