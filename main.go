package main

import (
	"log"

	"github.com/Zyko0/go-sdl3/bin/binsdl"
	"github.com/Zyko0/go-sdl3/sdl"

	"github.com/tridentsx/wipeout-go/internal/game"
)

// Smoke-test scaffold: a handful of ships with initial velocity, stepped
// through game.UpdatePhysics each frame and drawn top-down (X/Z plane,
// ignoring altitude) as flat-colored squares. Visual/manual validation only
// that the ported physics behaves sanely (ships glide and decelerate) --
// not the real renderer, which per TODO.md will be SDL3's GPU API using the
// original's primitive set.
const (
	worldScale  = 2.0 // world units -> pixels
	originX     = 640.0
	originY     = 360.0
	shipHalfPx  = 4.0
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
