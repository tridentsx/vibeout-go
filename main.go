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
	menu := game.NewMenuCursor()
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
	// PALTITLE.TIM is the PAL title art. STARTPAL.TIM, used here before, is the
	// loading screen -- a different image; the NTSC title is WIPTITLE.TIM.
	var titleTex *sdl.GPUTexture
	var titleW, titleH int
	if img, imgErr := loader.LoadTIM("TEXTURES", "PALTITLE.TIM"); imgErr == nil {
		if tex, texErr := device.NewTexture(img.Width, img.Height, img.Pixels); texErr == nil {
			titleTex = tex
			titleW, titleH = img.Width, img.Height
		}
	} else {
		log.Printf("title screen unavailable: %v", imgErr)
	}

	// STARTPAL.TIM is the loading screen retail shows while an overlay loads.
	var loadTex *sdl.GPUTexture
	var loadW, loadH int
	if img, imgErr := loader.LoadTIM("TEXTURES", "STARTPAL.TIM"); imgErr == nil {
		if tex, texErr := device.NewTexture(img.Width, img.Height, img.Pixels); texErr == nil {
			loadTex, loadW, loadH = tex, img.Width, img.Height
		}
	}
	// The PAL menu texture pack. Member 5 is the 320x256 background and member 0 the
	// 288x184 panel; MENUTIMS.CMP is the NTSC equivalent.
	var menuBackTex, menuPanelTex *sdl.GPUTexture
	var menuBackW, menuBackH, menuPanelW, menuPanelH int
	if imgs, cmpErr := loader.LoadTextureSet("MENUTIMP.CMP"); cmpErr == nil {
		if len(imgs) > 5 && imgs[5] != nil {
			if tex, e := device.NewTexture(imgs[5].Width, imgs[5].Height, imgs[5].Pixels); e == nil {
				menuBackTex, menuBackW, menuBackH = tex, imgs[5].Width, imgs[5].Height
			}
		}
		if len(imgs) > 0 && imgs[0] != nil {
			if tex, e := device.NewTexture(imgs[0].Width, imgs[0].Height, imgs[0].Pixels); e == nil {
				menuPanelTex, menuPanelW, menuPanelH = tex, imgs[0].Width, imgs[0].Height
			}
		}
	} else {
		log.Printf("menu artwork unavailable: %v", cmpErr)
	}

	// The menu's 3D models. Each entry is a whole PRM whose objects are looked up by
	// name; see bn-psx/docs/wipeout2097_menu_system.md for what lives where.
	menuModels := map[string]*assets.Model{}
	for _, file := range []string{"VECTO.PRM", "VENOM.PRM", "RAPIE.PRM", "PHANT.PRM",
		"TERRY.PRM", "WIERD.PRM", "JUNE.PRM", "HARRY.PRM", "JULIE.PRM"} {
		model, modelErr := loader.LoadModel("COMMON", file)
		if modelErr != nil {
			log.Printf("menu model %s unavailable: %v", file, modelErr)
			continue
		}
		menuModels[file] = model
	}
	menuModelTextures := map[string][]*sdl.GPUTexture{}
	for file, model := range menuModels {
		menuModelTextures[file] = device.NewTextures(model.Pages)
	}
	menuSpin := game.Angle(0)

	// Start is edge-triggered: the state machine debounces, but a held key must not
	// read as a fresh press on the next screen.
	startPressed := false
	// Menu input, also edge-triggered so a held key steps one row.
	menuMove := 0
	menuActivate := false
	menuBack := false
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
					// Up doubles as accelerate in a race and menu-up in the front end;
					// the two states never read it at the same time.
					keys.accelerate = down
					if down && !key.Repeat {
						menuMove--
					}
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
						// One press must not both leave the title and activate the first
						// menu row, so only count it as an activation when the front end
						// is already open.
						if states.State() == game.StateFrontEnd {
							menuActivate = true
						} else {
							startPressed = true
						}
					}
				case sdl.SCANCODE_ESCAPE, sdl.SCANCODE_BACKSPACE:
					if down && !key.Repeat {
						// In a race this stands in for finishing it. Without it a race was
						// terminal -- RaceFinished was never called from anywhere -- so
						// once the title timed out into a demo there was no way back to
						// the menu at all.
						if states.State() == game.StateRace {
							states.RaceFinished()
						} else {
							menuBack = true
						}
					}
				case sdl.SCANCODE_DOWN:
					if down && !key.Repeat {
						menuMove++
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
					if menuMove != 0 {
						menu.Move(menuMove, &states.Context)
					}
					if menuBack && !menu.Back() {
						// Backing out of the main screen leaves the front end, which is
						// what retail's return of 1 means.
						states.FrontEndResult(game.RaceSetupBackToTitle)
					}
					if menuActivate {
						if menu.Activate(&states.Context) == game.ActionStartRace {
							states.Context.Normalise(gameRender.MenuIndexToTrackID[:])
							log.Printf("start: track %s (id %d), class %d",
								game.TrackMenuEntries[states.Context.MenuTrackIndex].Name,
								states.Context.TrackID, states.Context.SpeedClass)
							states.FrontEndResult(int(states.Context.TrackID))
						}
					}
				}
				if states.State() == game.StateFrontEnd {
					menuSpin = (menuSpin + gameRender.MenuModelSpinRate).Wrapped()
				}
				menuMove, menuActivate, menuBack = 0, false, false
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
		ui.BeginFrame()
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
			ui.DrawSplash(frame, loadTex, loadW, loadH)
			if overlay, ok := states.CurrentOverlay(); ok {
				ui.DrawTextCentered(frame, overlay.BaseName(), gameRender.RetailWidth/2, 0xe4, gameRender.White)
			}
		case game.StateFrontEnd:
			ui.FillScreen(frame, sdl.FColor{A: 1})
			ui.DrawSplash(frame, menuBackTex, menuBackW, menuBackH)
			if menuPanelTex != nil {
				// Centre the 288x184 panel in the frame.
				ui.DrawImage(frame, menuPanelTex,
					(gameRender.RetailWidth-menuPanelW)/2, (gameRender.RetailHeight-menuPanelH)/2,
					menuPanelW, menuPanelH, gameRender.White)
			}
			drawMenu(ui, frame, menu, &states.Context, menuModels, menuModelTextures, menuSpin)
		case game.StateTitleAttract:
			ui.FillScreen(frame, sdl.FColor{A: 1})
			ui.DrawSplash(frame, titleTex, titleW, titleH)
			// Retail draws this at (0xa0, 0xe4) and blinks it 18 ticks in every 25.
			if game.PressStartVisible(states.Tick()) {
				ui.DrawTextCentered(frame, "PRESS START", 0xa0, 0xe4, gameRender.White)
			}
			ui.DrawTextCentered(frame, "ENTER", gameRender.RetailWidth/2, 236,
				sdl.FColor{R: 0.4, G: 0.4, B: 0.45, A: 1})
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

// The main screen's layout, measured off a retail screenshot. Three columns, each a
// heading over a model over the current selection's name, then a highlighted START
// bar and a bottom row carrying the select prompt and OPTIONS.
const (
	menuPanelLeft   = 16
	menuPanelRight  = gameRender.RetailWidth - 16
	menuColumnCount = 3
	menuHeadingY    = 52
	menuModelY      = 104
	menuNameY       = 148
	// Model fill sizes are the port's own: DrawMenuModel derives its scale from each
	// object's extent to fill this many pixels, since retail's own per-screen camera
	// distance has not been reversed. Craft read smaller than the track shapes and
	// race type icons at the same extent, because their bulk is concentrated along one
	// axis, so they get a 15% larger target.
	menuColumnFill      = 66
	menuColumnFillCraft = 76
	menuSubFill         = 120
	menuSubFillCraft    = 138
	menuStartY      = 164
	menuFooterY     = 182
)

// mainMenuColumn describes one of the three columns.
type mainMenuColumn struct {
	heading []string
	value   string
	screen  game.MenuScreenID
}

// mainMenuColumns builds the three columns for the current context. Retail groups
// class and track under one heading, "CLASS AND TRACK", and shows the track there.
func mainMenuColumns(ctx *game.GameContext) []mainMenuColumn {
	raceType := "ARCADE"
	if items := game.Screens[game.ScreenRaceType].Items; ctx.RaceTypeIndex < uint8(len(items)) {
		raceType = items[ctx.RaceTypeIndex].Label
	}
	team := "?"
	if teams := ctx.Teams(); int(ctx.TeamIndex) < len(teams) {
		team = teams[ctx.TeamIndex].Name
	}
	track := "?"
	if int(ctx.MenuTrackIndex) < len(game.TrackMenuEntries) {
		track = game.TrackMenuEntries[ctx.MenuTrackIndex].Name
	}
	return []mainMenuColumn{
		{heading: []string{"RACE", "TYPE"}, value: raceType, screen: game.ScreenRaceType},
		{heading: []string{"TEAM"}, value: team, screen: game.ScreenTeam},
		{heading: []string{"CLASS", "AND", "TRACK"}, value: track, screen: game.ScreenTrack},
	}
}

// columnCentre is the horizontal centre of column i.
func columnCentre(i int) int {
	width := (menuPanelRight - menuPanelLeft) / menuColumnCount
	return menuPanelLeft + width*i + width/2
}

// drawMainScreen renders the three-column layout.
func drawMainScreen(ui *gameRender.UI, frame *gameRender.Frame, menu *game.MenuCursor,
	ctx *game.GameContext, models map[string]*assets.Model,
	textures map[string][]*sdl.GPUTexture, spin game.Angle) {
	columns := mainMenuColumns(ctx)
	selected := menu.Selection()

	// Models first, then the text band so labels sit in front.
	for i := range columns {
		file, object := mainColumnModel(ctx, i)
		fill := float32(menuColumnFill)
		if i == 1 {
			// The team column shows a craft.
			fill = menuColumnFillCraft
		}
		drawNamedModel(ui, frame, models, textures, file, object, columnCentre(i), menuModelY, fill, spin)
	}
	ui.BeginTextBand()

	dim := sdl.FColor{R: 0.55, G: 0.55, B: 0.62, A: 1}
	for i, column := range columns {
		colour := dim
		if i == selected {
			colour = gameRender.White
		}
		x := columnCentre(i)
		for line, text := range column.heading {
			ui.DrawTextCentered(frame, text, x, menuHeadingY+line*9, colour)
		}
		ui.DrawTextCentered(frame, column.value, x, menuNameY, colour)
	}

	// START occupies its own row and is selectable after the three columns.
	startColour := dim
	if selected == len(columns) {
		startColour = gameRender.White
		ui.Fill(frame, menuPanelLeft, menuStartY-2, menuPanelRight-menuPanelLeft, 11,
			sdl.FColor{R: 0.35, G: 0.05, B: 0.15, A: 1})
		ui.BeginTextBand()
	}
	ui.DrawTextCentered(frame, "START", gameRender.RetailWidth/2, menuStartY, startColour)

	optionsColour := dim
	if selected == len(columns)+1 {
		optionsColour = gameRender.White
	}
	ui.DrawText(frame, "SELECT", menuPanelLeft+4, menuFooterY, dim)
	ui.DrawTextCentered(frame, "OPTIONS", gameRender.RetailWidth/2, menuFooterY, optionsColour)
}

// mainColumnModel is the model shown in each main-screen column.
func mainColumnModel(ctx *game.GameContext, column int) (file, object string) {
	switch column {
	case 0:
		if int(ctx.RaceTypeIndex) >= len(game.RaceTypeModels) {
			return "", ""
		}
		m := game.RaceTypeModels[ctx.RaceTypeIndex]
		return m.File, m.Object
	case 1:
		teams := ctx.Teams()
		if int(ctx.TeamIndex) >= len(teams) {
			return "", ""
		}
		file = "TERRY.PRM"
		if ctx.AnimalTeams {
			file = "WIERD.PRM"
		}
		return file, teams[ctx.TeamIndex].Object
	case 2:
		if int(ctx.MenuTrackIndex) >= len(game.TrackPreviewObjects) {
			return "", ""
		}
		return "JUNE.PRM", game.TrackPreviewObjects[ctx.MenuTrackIndex]
	}
	return "", ""
}

// drawNamedModel looks an object up by name and draws it, doing nothing if either the
// file or the object is missing.
func drawNamedModel(ui *gameRender.UI, frame *gameRender.Frame, models map[string]*assets.Model,
	textures map[string][]*sdl.GPUTexture, file, object string, x, y int, fill float32, spin game.Angle) {
	if file == "" || object == "" {
		return
	}
	model := models[file]
	if model == nil {
		return
	}
	target := gameRender.FindObject(model.Objects, object)
	if target == nil {
		return
	}
	gameRender.DrawMenuModel(frame, ui, target, textures[file], model.Pages, x, y, fill, spin)
}

// drawMenu renders the current front end screen.
func drawMenu(ui *gameRender.UI, frame *gameRender.Frame, menu *game.MenuCursor, ctx *game.GameContext,
	models map[string]*assets.Model, textures map[string][]*sdl.GPUTexture, spin game.Angle) {
	screen := menu.Screen()
	if screen == game.ScreenMain {
		drawMainScreen(ui, frame, menu, ctx, models, textures, spin)
		return
	}

	// Sub-screens: one model for the highlighted row, with the rows listed down the
	// left.
	file, object := subScreenModel(menu, ctx)
	subFill := float32(menuSubFill)
	switch screen {
	case game.ScreenTeam, game.ScreenClass:
		subFill = menuSubFillCraft
	}
	drawNamedModel(ui, frame, models, textures, file, object, 210, 118, subFill, spin)
	ui.BeginTextBand()

	const (
		titleY   = 28
		firstY   = 52
		rowPitch = 13
		rowX     = 24
	)
	dim := sdl.FColor{R: 0.55, G: 0.55, B: 0.62, A: 1}
	ui.DrawText(frame, game.Screens[screen].Title, rowX, titleY, gameRender.White)
	selected := menu.Selection()

	switch screen {
	case game.ScreenTrack:
		count := ctx.SelectableTrackCount()
		for i := 0; i < count && i < len(game.TrackMenuEntries); i++ {
			colour := dim
			if i == selected {
				colour = gameRender.White
			}
			ui.DrawText(frame, game.TrackMenuEntries[i].Name, rowX, firstY+i*rowPitch, colour)
		}
		if selected < len(game.TrackMenuEntries) {
			entry := game.TrackMenuEntries[selected]
			ui.DrawText(frame, entry.Rating, rowX, 196, dim)
			ui.DrawTextCentered(frame, entry.Description, gameRender.RetailWidth/2, 210, dim)
		}
	case game.ScreenTeam:
		for i, team := range ctx.Teams() {
			colour := dim
			if i == selected {
				colour = gameRender.White
			}
			ui.DrawText(frame, team.Name, rowX, firstY+i*rowPitch, colour)
		}
	default:
		items := game.Screens[screen].Items
		if len(items) == 0 {
			ui.DrawText(frame, "NOT IMPLEMENTED", rowX, firstY, dim)
			break
		}
		for i, item := range items {
			colour := dim
			if i == selected {
				colour = gameRender.White
			}
			ui.DrawText(frame, item.Label, rowX, firstY+i*rowPitch, colour)
		}
	}
	ui.DrawText(frame, "UP DOWN  ENTER  ESC", rowX, 236,
		sdl.FColor{R: 0.4, G: 0.4, B: 0.45, A: 1})
}

// subScreenModel is the model representing the highlighted row of a sub-screen.
func subScreenModel(menu *game.MenuCursor, ctx *game.GameContext) (file, object string) {
	i := menu.Selection()
	switch menu.Screen() {
	case game.ScreenRaceType:
		if i < len(game.RaceTypeModels) {
			return game.RaceTypeModels[i].File, game.RaceTypeModels[i].Object
		}
	case game.ScreenClass:
		if i < len(game.ClassModels) {
			return game.ClassModels[i].File, game.ClassModels[i].Object
		}
	case game.ScreenTeam:
		teams := ctx.Teams()
		if i < len(teams) {
			file = "TERRY.PRM"
			if ctx.AnimalTeams {
				file = "WIERD.PRM"
			}
			return file, teams[i].Object
		}
	case game.ScreenTrack:
		if i < len(game.TrackPreviewObjects) {
			return "JUNE.PRM", game.TrackPreviewObjects[i]
		}
	}
	return "", ""
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
