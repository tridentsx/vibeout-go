package main

import (
	"log"

	"github.com/Zyko0/go-sdl3/bin/binsdl"
	"github.com/Zyko0/go-sdl3/sdl"
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

	sdl.RunLoop(func() error {
		var event sdl.Event
		for sdl.PollEvent(&event) {
			if event.Type == sdl.EVENT_QUIT {
				return sdl.EndLoop
			}
		}
		renderer.SetDrawColor(10, 10, 20, 255)
		renderer.Clear()
		renderer.Present()
		return nil
	})
}
