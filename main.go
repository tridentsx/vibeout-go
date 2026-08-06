package main

import (
	"bufio"
	"fmt"
	"log"
	"math"
	"os"
	"path"
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

	// Animated scenery: fans spin, billboards and smoke cycle texture frames.
	// TRACK01 is menu index 0, which maps to internal track ID 1 -- the only ID
	// whose object bindings have been read out of the retail dispatch chain, and
	// the one that has fans and smoke. See internal/render/scenery_anim.go.
	sceneryAnim := gameRender.BindAnimatedScenery(gameRender.MenuIndexToTrackID[0], track.Scenery)
	log.Printf("animated scenery: %d fan(s), %d slow smoke, %d fast smoke, %d billboard(s), %d camera(s)",
		len(sceneryAnim.Fans), len(sceneryAnim.SmokeSlow), len(sceneryAnim.SmokeFast),
		len(sceneryAnim.Billboards), len(sceneryAnim.Cameras))
	// Frame textures come from TEXTURES/SMOKE.CMP and TEXTURES/<set>RED.CMP, not
	// from the track's SCENE.CMP.
	if err := sceneryAnim.LoadFrameTextures(device, loader, gameRender.MenuIndexToTrackID[0]); err != nil {
		log.Printf("optional animation textures: %v", err)
	}
	log.Printf("animation frames: %d smoke, %d billboard",
		len(sceneryAnim.SmokeTextures), len(sceneryAnim.BillboardTextures))

	// TERRY.PRM holds the five real per-team craft hulls (confirmed against a
	// real PRM viewer): quirex1/fiesar1/auricom2/ag1/piranha2, matching the
	// five team names in the executable's own strings (QIREX/FEISAR/AURICOM/
	// AG SYSTEMS/PIRANHA). VECTO.PRM's standalone "vect" and HARRY.PRM's
	// bundled "vect"/"ven"/"rap"/"phant" are menu class-select icons, not
	// this craft. This preview defaults to the FEISAR team ("fiesar1").
	craftModel, err := loader.LoadModel("COMMON", "TERRY.PRM")
	if err != nil || len(craftModel.Objects) == 0 {
		log.Fatalf("load player craft: objects=%d err=%v", len(craftModel.Objects), err)
	}
	for _, warning := range craftModel.Warnings {
		log.Printf("optional craft asset: %v", warning)
	}
	craft := gameRender.FindObject(craftModel.Objects, "fiesar1")
	if craft == nil {
		log.Fatal("load player craft: TERRY.PRM has no \"fiesar1\" object")
	}
	craftTextures := device.NewTextures(craftModel.Pages)
	defer func() {
		for _, texture := range craftTextures {
			if texture != nil {
				device.ReleaseTexture(texture)
			}
		}
	}()

	ship := &game.Ship{ControlSource: game.ControlLocalPlayer, Flags: 0x248}
	spec, err := game.RaceShipSpec(0, 0)
	if err != nil {
		log.Fatal(err)
	}
	game.ApplyRaceShipSpec(ship, spec)
	// The craft starts on the grid, not on the line: the grid is the run of
	// sections flagged TrackFaceStartGrid (290-319 on TRACK01), while the
	// per-track table gives the start/finish line (section 5 there).
	// The grid is walked backwards from the start/finish line. A lone craft in a
	// single race starts in the last slot; other modes resolve the slot from
	// qualifying or standings. The slot also picks which side of the road the craft
	// sits on, so it must be passed through rather than flattened to zero.
	trackID := gameRender.MenuIndexToTrackID[0]
	lineSection := game.TrackStartLineSection[trackID]
	const gridSlots = 15
	gridSlot := game.PlayerGridSlot(gridSlots)
	log.Printf("start line at section %d; craft in grid slot %d of %d", lineSection, gridSlot, gridSlots)
	if err := game.PlaceShipOnStartingGrid(ship, track, lineSection, gridSlot); err != nil {
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
	// The screen-space layer and the top-level state machine. The state machine is
	// ported from main (0x8001a464); see internal/game/state.go.
	ui, err := gameRender.NewUI(device, loader, 1280, 720)
	if err != nil {
		log.Fatal(err)
	}
	defer ui.Destroy()
	states := game.NewStateMachine()
	// Boot splash textures, uploaded once. A missing file is not fatal: the state
	// machine still holds for the retail duration, just on black.
	type splashImage struct {
		tex  *sdl.GPUTexture
		w, h int
	}
	splashTextures := make([]splashImage, len(game.BootSplashes))
	for i, splash := range game.BootSplashes {
		dir, file := path.Split(splash.Texture)
		img, imgErr := loader.LoadTIM(path.Clean(dir), file)
		if imgErr != nil {
			// Expected for DOLBYPAL.TIM: the executable asks for it but no PAL disc
			// carries it, so retail's load fails too and the previous image stays on
			// screen for that slot's duration. Leaving the entry nil reproduces that,
			// because the draw path falls back to the last texture it showed.
			log.Printf("boot splash %s not on this disc; holding the previous screen", splash.Texture)
			continue
		}
		tex, texErr := device.NewTexture(img.Width, img.Height, img.Pixels)
		if texErr != nil {
			log.Printf("boot splash %s upload failed: %v", splash.Texture, texErr)
			continue
		}
		splashTextures[i] = splashImage{tex: tex, w: img.Width, h: img.Height}
	}
	// The loading/title screen, drawn behind the overlay placeholders and on the
	// title itself, which is what retail has on screen at both points.
	var titleTex *sdl.GPUTexture
	var titleW, titleH int
	if img, imgErr := loader.LoadTIM("TEXTURES", "STARTPAL.TIM"); imgErr == nil {
		if tex, texErr := device.NewTexture(img.Width, img.Height, img.Pixels); texErr == nil {
			titleTex = tex
			titleW, titleH = img.Width, img.Height
		}
	} else {
		log.Printf("title screen unavailable: %v", imgErr)
	}

	// Start is edge-triggered: the state machine debounces, but a held key must not
	// read as a fresh press on the next screen.
	startPressed := false
	// lastSplash holds the most recent splash actually drawn, so a slot whose image
	// is absent from the disc keeps the previous screen rather than flashing black.
	var lastSplashTex *sdl.GPUTexture
	var lastSplashW, lastSplashH int

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
				case sdl.SCANCODE_RETURN, sdl.SCANCODE_SPACE:
					if down && !key.Repeat {
						startPressed = true
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
			// Advance the top-level state machine first: it decides whether this tick
			// belongs to a race at all.
			start := startPressed || pad.IsDown(controller.Accelerate)
			states.Advance(start)
			startPressed = false
			if states.State() != game.StateRace {
				// Nothing to simulate outside a race. The front end has no
				// implementation yet, so treat it as an immediate back-out, which
				// returns the machine to the title exactly as retail's return of 1 does.
				if states.State() == game.StateFrontEnd {
					states.FrontEndResult(game.RaceSetupBackToTitle)
				}
				accumulator -= tick
				continue
			}
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
			sceneryAnim.Tick()
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
		switch states.State() {
		case game.StateBootSplash:
			ui.FillScreen(frame, sdl.FColor{A: 1})
			// A missing image holds whatever was last shown, which is what retail does:
			// LoadTIMTexture fails and nothing clears the framebuffer.
			if i := states.SplashIndex(); i >= 0 && splashTextures[i].tex != nil {
				lastSplashTex = splashTextures[i].tex
				lastSplashW, lastSplashH = splashTextures[i].w, splashTextures[i].h
			}
			ui.DrawSplash(frame, lastSplashTex, lastSplashW, lastSplashH)
		case game.StateBootOverlay, game.StatePostRaceOverlay:
			// Overlays are separate PS-EXEs running FMV, which a port cannot execute.
			// Retail has the loading screen up while they load, so draw that and name
			// the overlay over it rather than blanking to black.
			ui.FillScreen(frame, sdl.FColor{A: 1})
			ui.DrawSplash(frame, titleTex, titleW, titleH)
			if overlay, ok := states.CurrentOverlay(); ok {
				ui.DrawTextCentered(frame, overlay.BaseName(), gameRender.RetailWidth/2, 0xe4, gameRender.White)
			}
		case game.StateTitleAttract:
			ui.FillScreen(frame, sdl.FColor{A: 1})
			ui.DrawSplash(frame, titleTex, titleW, titleH)
			// Retail draws this at (0xa0, 0xe4) and blinks it 18 ticks in every 25.
			if game.PressStartVisible(states.Tick()) {
				ui.DrawTextCentered(frame, "PRESS START", 0xa0, 0xe4, gameRender.White)
			}
		default:
			trackRenderer.DrawSkyPerspective(frame, camera.Camera, 1280, 720)
			trackRenderer.DrawPerspective(frame, camera.Camera, 1280, 720)
			trackRenderer.DrawSceneryPerspectiveAnimated(frame, camera.Camera, 1280, 720, sceneryAnim)
			if camera.View == gameRender.CameraExternal {
				gameRender.DrawShipPerspective(frame, camera.Camera, ship, craft, craftTextures, craftModel.Pages, 1280, 720)
			}
			if states.IsDemo() {
				ui.DrawTextCentered(frame, "DEMO MODE", gameRender.RetailWidth/2, 20, gameRender.White)
			}
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
