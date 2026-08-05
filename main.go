package main

import (
	"bufio"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/Zyko0/go-sdl3/bin/binsdl"
	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/controller"
	"github.com/tridentsx/wipeout-go/internal/game"
	"github.com/tridentsx/wipeout-go/internal/physics"
	gameRender "github.com/tridentsx/wipeout-go/internal/render"
)

// wipeoutDiscPath is the extracted WipEout 2097 disc tree, kept under the
// repository's git-ignored assets/ directory. Relative to the repo root, so
// run the binary from there (e.g. `go run .`).
const wipeoutDiscPath = "assets/WIPEOUT2"

type keyboardState struct {
	accelerate, left, right, leftBrake, rightBrake bool
}

func main() {
	defer binsdl.Load().Unload()
	defer sdl.Quit()
	if err := sdl.Init(sdl.INIT_VIDEO | sdl.INIT_GAMEPAD); err != nil {
		log.Fatal(err)
	}
	window, err := sdl.CreateWindow("WipeOut", 1280, 720, 0)
	if err != nil {
		log.Fatal(err)
	}
	defer window.Destroy()
	device, err := gameRender.NewDevice(window, 1280, 720)
	if err != nil {
		log.Fatal(err)
	}
	defer device.Destroy()
	frameServer, err := startFrameServer("127.0.0.1:8097")
	if err != nil {
		log.Fatal(err)
	}
	defer frameServer.Close()
	log.Printf("frame capture: http://127.0.0.1:8097/frame.png")

	loader := assets.Loader{Root: wipeoutDiscPath}
	track, err := loader.LoadTrack("TRACK01")
	if err != nil {
		log.Fatal(err)
	}
	for _, warning := range track.Warnings {
		log.Printf("optional track asset: %v", warning)
	}
	trackRenderer, err := gameRender.NewTrackRenderer(device, track, 1280, 720)
	if err != nil {
		log.Fatal(err)
	}
	defer trackRenderer.Destroy()

	// TERRY.PRM holds the five real per-team craft hulls (confirmed against a
	// real PRM viewer): quirex1/fiesar1/auricom2/ag1/piranha2, matching the
	// five team names in the executable's own strings (QIREX/FEISAR/AURICOM/
	// AG SYSTEMS/PIRANHA). VECTO.PRM's standalone "vect" and HARRY.PRM's
	// bundled "vect"/"ven"/"rap"/"phant" are menu class-select icons, not
	// this craft. This preview defaults to the FEISAR team ("fiesar1").
	craftObjects, err := loader.LoadPRM("COMMON", "TERRY.PRM")
	if err != nil || len(craftObjects) == 0 {
		log.Fatalf("load player craft: objects=%d err=%v", len(craftObjects), err)
	}
	craft := gameRender.FindObject(craftObjects, "fiesar1")
	if craft == nil {
		log.Fatal("load player craft: TERRY.PRM has no \"fiesar1\" object")
	}

	ship := &game.Ship{ControlSource: game.ControlLocalPlayer, Flags: 0x248}
	spec, err := game.RaceShipSpec(0, 0)
	if err != nil {
		log.Fatal(err)
	}
	game.ApplyRaceShipSpec(ship, spec)
	if err := game.PlaceShipOnStartingGrid(ship, track, 0, 0); err != nil {
		log.Fatal(err)
	}
	physics.UpdateShipOrientationVectors(ship)
	if err := physics.UpdateShipTrackFaceSide(ship, track); err != nil {
		log.Fatal(err)
	}
	initialSideFlag := ship.Flags & game.ShipFlagTrackFaceSide
	for _, sideFlag := range []uint32{0, game.ShipFlagTrackFaceSide} {
		ship.Flags = ship.Flags&^game.ShipFlagTrackFaceSide | sideFlag
		if sample, sampleErr := physics.SampleShipTrackContact(ship, track); sampleErr == nil {
			log.Printf("spawn contact side=%#x face=%d distance=%.1f forwardDistance=%.1f normal=(%.4f,%.4f,%.4f) sectionY=%.1f",
				sideFlag, sample.FaceIndex, sample.CenterDistance, sample.ForwardDistance, sample.Normal.X, sample.Normal.Y, sample.Normal.Z, sample.SectionY)
		}
	}
	ship.Flags = ship.Flags&^game.ShipFlagTrackFaceSide | initialSideFlag
	camera := gameRender.NewRaceCamera(ship, track.Sections)
	// Race setup installs the retail presentation callback before the first
	// frame. Keep this enabled in the validation preview so the captured
	// frames exercise the authentic starting-grid camera arc and handoff.
	camera.BeginRaceStart()
	traceFile, err := os.Create("camera_trace.log")
	if err != nil {
		log.Fatal(err)
	}
	defer traceFile.Close()
	trace := bufio.NewWriter(traceFile)
	defer trace.Flush()
	_, _ = fmt.Fprintln(trace, "frame,start_active,start_timer,ship_section,ship_x,ship_y,ship_z,camera_section,camera_x,camera_y,camera_z,camera_yaw,camera_pitch,camera_roll")
	log.Printf("spawn section=%d position=(%.1f,%.1f,%.1f) yaw=%d pitch=%d roll=%d forward=(%.4f,%.4f,%.4f)",
		ship.SectionID, ship.Position.X, ship.Position.Y, ship.Position.Z, ship.Yaw, ship.Pitch, ship.Roll,
		ship.Forward.X, ship.Forward.Y, ship.Forward.Z)

	gamepad, err := controller.OpenFirstSDLGamepad()
	if err != nil {
		log.Printf("gamepad unavailable: %v", err)
	}
	if gamepad != nil {
		defer gamepad.Close()
	}
	mapping := controller.DefaultMapping()
	keys := keyboardState{}
	last := time.Now()
	accumulator := time.Duration(0)
	physicsTicks := uint64(0)
	// The PAL game presents 50 Hz fields, but advances its race/game state at
	// 25 Hz. The renderer continues to run as often as SDL calls the loop;
	// this fixed step controls physics, camera callbacks, and their timers.
	const tick = time.Second / 25
	log.Printf("race preview: W accelerate, arrows steer, A/D airbrakes, V camera")

	sdl.RunLoop(func() error {
		var event sdl.Event
		for sdl.PollEvent(&event) {
			switch event.Type {
			case sdl.EVENT_QUIT:
				return sdl.EndLoop
			case sdl.EVENT_KEY_DOWN, sdl.EVENT_KEY_UP:
				key := event.KeyboardEvent()
				down := event.Type == sdl.EVENT_KEY_DOWN
				switch key.Scancode {
				case sdl.SCANCODE_W, sdl.SCANCODE_UP:
					keys.accelerate = down
				case sdl.SCANCODE_LEFT:
					keys.left = down
				case sdl.SCANCODE_RIGHT:
					keys.right = down
				case sdl.SCANCODE_A:
					keys.leftBrake = down
				case sdl.SCANCODE_D:
					keys.rightBrake = down
				case sdl.SCANCODE_V:
					if down && !key.Repeat {
						camera.ToggleView(ship)
					}
				}
			}
		}

		pad := controller.State{}
		if gamepad != nil {
			pad = gamepad.Poll(mapping)
		}
		now := time.Now()
		accumulator += now.Sub(last)
		last = now
		if accumulator > 5*tick {
			accumulator = 5 * tick
		}
		for accumulator >= tick {
			accelerate := keys.accelerate || pad.IsDown(controller.Accelerate)
			left := keys.left || pad.IsDown(controller.SteerLeft)
			right := keys.right || pad.IsDown(controller.SteerRight)
			leftBrake := keys.leftBrake || pad.IsDown(controller.LeftAirbrake)
			rightBrake := keys.rightBrake || pad.IsDown(controller.RightAirbrake)
			physics.UpdateThrottle(ship, false, 0, accelerate)
			physics.UpdateSteeringDigital(ship, left, right)
			physics.UpdateAirBrakes(ship, leftBrake, rightBrake)
			physics.IntegrateYawFromSteering(ship)
			responses, err := physics.StepShipTrackPhysics(ship, track)
			if err != nil {
				log.Printf("physics stopped: %v", err)
				return sdl.EndLoop
			}
			physics.IntegratePitchAndRoll(ship)
			camera.Update(ship, track.Sections)
			camera.AdvanceRaceStart()
			physicsTicks++
			// Keep a frame-synchronous scalar trace for comparison with the
			// DuckStation debugger. A breakpoint hit and a row in this file
			// represent the same simulation step.
			_, _ = fmt.Fprintf(trace, "%d,%t,%d,%d,%.6f,%.6f,%.6f,%d,%.6f,%.6f,%.6f,%d,%d,%d\n",
				physicsTicks, camera.RaceStartActive, camera.RaceStartTimer, ship.SectionID,
				ship.Position.X, ship.Position.Y, ship.Position.Z, camera.Section,
				camera.Position.X, camera.Position.Y, camera.Position.Z,
				camera.Yaw, camera.Pitch, camera.Roll)
			_ = trace.Flush()
			if physicsTicks <= 20 || physicsTicks%25 == 0 {
				cameraRight, cameraDown, cameraForward := camera.Camera.Basis()
				shipInCamera := camera.Camera.WorldToCamera(ship.Position)
				section := track.Sections[ship.SectionID]
				sectionInCamera := camera.Camera.WorldToCamera(game.Vector3{X: float32(section.X), Y: float32(section.Y), Z: float32(section.Z)})
				log.Printf("tick=%d raceStart=%t timer=%d section=%d previous=%d flags=%#x contacts=%d pos=(%.1f,%.1f,%.1f) vel=(%.1f,%.1f,%.1f) speed=%.1f yaw=%d pitch=%d roll=%d steer=%.1f pitchRate=%.1f rollRate=%.1f cameraSection=%d camera=(%.1f,%.1f,%.1f)",
					physicsTicks, camera.RaceStartActive, camera.RaceStartTimer, ship.SectionID, ship.PreviousSectionID, ship.Flags, responses,
					ship.Position.X, ship.Position.Y, ship.Position.Z, ship.Velocity.X, ship.Velocity.Y, ship.Velocity.Z,
					ship.Speed, ship.Yaw, ship.Pitch, ship.Roll, ship.SteeringRate, ship.PitchRate, ship.RollRate,
					camera.Section, camera.Position.X, camera.Position.Y, camera.Position.Z)
				log.Printf("camera-basis tick=%d right=(%.4f,%.4f,%.4f) down=(%.4f,%.4f,%.4f) forward=(%.4f,%.4f,%.4f) dots=(rd=%.5f rf=%.5f df=%.5f) shipCamera=(%.1f,%.1f,%.1f) sectionCamera=(%.1f,%.1f,%.1f) sectionScreen=(%.1f,%.1f)",
					physicsTicks,
					cameraRight.X, cameraRight.Y, cameraRight.Z,
					cameraDown.X, cameraDown.Y, cameraDown.Z,
					cameraForward.X, cameraForward.Y, cameraForward.Z,
					vectorDot(cameraRight, cameraDown), vectorDot(cameraRight, cameraForward), vectorDot(cameraDown, cameraForward),
					shipInCamera.X, shipInCamera.Y, shipInCamera.Z,
					sectionInCamera.X, sectionInCamera.Y, sectionInCamera.Z,
					perspectiveScreenCoordinate(sectionInCamera.X, sectionInCamera.Z, 1280, 1280),
					perspectiveScreenCoordinate(sectionInCamera.Y, sectionInCamera.Z, 720, 1280))
			}
			if !shipStateFinite(ship, camera.Camera) {
				log.Printf("physics stopped: non-finite state at tick %d: ship=%+v camera=%+v", physicsTicks, *ship, camera.Camera)
				return sdl.EndLoop
			}
			accumulator -= tick
		}

		frame, err := device.BeginFrame()
		if err != nil {
			log.Printf("render: begin frame: %v", err)
			return nil
		}
		trackRenderer.DrawSkyPerspective(frame, camera.Camera, 1280, 720)
		trackRenderer.DrawPerspective(frame, camera.Camera, 1280, 720)
		trackRenderer.DrawSceneryPerspective(frame, camera.Camera, 1280, 720)
		if camera.View == gameRender.CameraExternal {
			gameRender.DrawShipPerspective(frame, camera.Camera, ship, craft, 1280, 720)
		}
		if err := device.Present(frame); err != nil {
			log.Printf("render: present: %v", err)
			return nil
		}
		select {
		case request := <-frameServer.requests:
			captured, captureErr := device.CapturePNG()
			request.result <- frameCaptureResult{png: captured, err: captureErr}
		default:
		}
		return nil
	})
}

func vectorDot(a, b game.Vector3) float32 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }

func perspectiveScreenCoordinate(axis, depth, viewport, width float32) float32 {
	if depth == 0 {
		return 0
	}
	return viewport/2 + axis*(1000*width/320)/depth
}

func shipStateFinite(ship *game.Ship, camera gameRender.Camera) bool {
	values := [...]float32{
		ship.Position.X, ship.Position.Y, ship.Position.Z,
		ship.Velocity.X, ship.Velocity.Y, ship.Velocity.Z,
		ship.Speed, ship.SteeringRate, ship.PitchRate, ship.RollRate,
		camera.Position.X, camera.Position.Y, camera.Position.Z,
	}
	for _, value := range values {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
	}
	return true
}
