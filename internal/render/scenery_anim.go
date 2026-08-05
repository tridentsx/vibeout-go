package render

import (
	"fmt"
	"strings"

	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/game"
)

// Animated track scenery: spinning fans, cycling billboards and smoke plumes.
//
// Retail drives these from maybe_UpdatePerTrackAnimatedObjects (0x8004216c),
// which dispatches on the internal track ID at raceSetup[5] and calls one
// animator per object kind. The objects themselves are bound by name out of the
// track's resources by maybe_LoadIndexedResource, in the chain at
// 0x8003f7dc-0x8004016c inside maybe_RaceMain.
//
// Two mechanisms cover everything:
//
//   - Fans rotate. maybe_RotateFanObjects (0x8004903c) writes one shared angle
//     into every fan's node and advances it by 100 per tick.
//   - Billboards and smoke swap texture frames. maybe_AnimateBillboardTextureFrames
//     (0x800484a8) and maybe_AnimateSmokeTextureFrames (0x800481b4) pick a frame
//     descriptor out of the global texture atlas and stamp its UVs, texture page
//     and CLUT into the object's own GPU primitives. Nothing in VRAM changes.
//
// This file owns the animation *state* only: what angle the fans are at and
// which texture frame each object set is showing. Applying that to geometry is
// the draw path's job, which keeps the timing logic testable without SDL.

// MenuIndexToTrackID maps menu/display order to the sparse internal track ID
// the scenery dispatch keys off. Mirrors maybe_MenuIndexToTrackId (0x80094d50),
// eight bytes read base+index by main, the track-select menu and challenge setup.
//
//	0 TALON'S REACH -> 1     4 SAGARMATHA    ->  2
//	1 GARE D'EUROPA -> 8     5 VALPARAISO    -> 17
//	2 VOSTOK ISLAND -> 13    6 ODESSA KEYS   ->  6
//	3 SPILSKINANKE  -> 20    7 PHENITIA PARK ->  7
var MenuIndexToTrackID = [8]uint8{1, 8, 13, 20, 2, 17, 6, 7}

// TrackSceneryBindings lists the animated object name prefixes per internal
// track ID.
//
// The retail loader compares with `strncmp(objectName, wanted, n)` where n is
// simply the length of the wanted name (maybe_LoadIndexedResource, 0x80048ed0),
// so matching is by prefix and there is no cap on how many objects bind -- it
// walks the whole resource list and reports the total through its out-parameter.
// The per-name integers in the dispatch chain are that strncmp length, not a
// count: "fan" passes 3, "redb" 4, "camera"/"smokes"/"smokef" 6, each equal to
// its own strlen. An earlier revision here mistook them for object counts and
// capped the bindings, which silently dropped two of TRACK01's eight "smokef*"
// plumes.
//
// Only ID 1 (Talon's Reach) is transcribed, because that is the block this
// project has read instruction by instruction. The other seven dispatch cases
// exist and their names are known, but they have not been transcribed, so they
// are absent rather than guessed. An absent entry animates nothing, which fails
// visibly rather than wrongly.
var TrackSceneryBindings = map[uint8][]string{
	1: {"camera", "fan", "smokes", "smokef", "redb"},
}

// TrackBillboardTextureSet names the CMP holding a track's billboard frames.
// maybe_RaceMain loads one of these immediately after binding "redb", and the
// file differs per track: the Greek-lettered names are per-track variants of the
// same two-frame advertisement pair.
var TrackBillboardTextureSet = map[uint8]string{
	1: "ALPHARED.CMP",
	6: "ZETARED.CMP",
	7: "ETARED.CMP",
	8: "ALPHARED.CMP",
}

// SmokeTextureSet is the shared smoke flipbook, 25 frames of 32x96 -- matching
// the animators' frame count of 0x19.
const SmokeTextureSet = "SMOKE.CMP"

const (
	// fanAngleStep is the per-tick advance from 0x80049094 (`addiu $v0, $v0, 100`).
	// With game.AngleFullTurn == 4096 this is a revolution every ~41 ticks, so
	// about 1.6s at the retail 25Hz tick.
	fanAngleStep = game.Angle(100)

	// smokeFrameCount is arg5 of maybe_AnimateSmokeTextureFrames, 0x19 at both
	// call sites (0x800421ec, 0x80042244).
	smokeFrameCount = 25

	// The two smoke sets differ only in their per-tick frame advance -- the last
	// argument is 1 for "smokes" and 2 for "smokef", which is what the names mean.
	smokeSlowStep = 1
	smokeFastStep = 2

	// billboardFrameCount and billboardGate are arg5 and arg3 of
	// maybe_AnimateBillboardTextureFrames(objects, count, 0x14, 0, 2): two frames,
	// advanced when the tick counter is a multiple of 20.
	//
	// NOTE: at a 25Hz tick that is a 1.6s cycle, but live observation of retail
	// recorded the billboard content changing roughly every 30 seconds. One of the
	// two is wrong: either the counter fed to the gate is not per-tick, or the
	// observation caught a different object. Flagged rather than silently tuned.
	billboardFrameCount = 2
	billboardGate       = 20
)

// AnimatedScenery holds the per-tick animation state for one track's animated
// objects. Index slices point into the track's Scenery slice.
type AnimatedScenery struct {
	// Fans share a single angle in retail, so this is one value, not one per fan.
	Fans     []int
	FanAngle game.Angle

	SmokeSlow      []int
	SmokeSlowFrame int
	SmokeFast      []int
	SmokeFastFrame int

	Billboards     []int
	BillboardTick  int
	BillboardFrame int

	// Frame textures, uploaded by LoadFrameTextures. Retail indexes the global
	// texture atlas from a per-set base; here each set is its own slice.
	SmokeTextures     []*sdl.GPUTexture
	BillboardTextures []*sdl.GPUTexture

	// Cameras are the trackside camera props. They aim at the player and bob on a
	// sine (maybe_AimAndBobCameraObjects, 0x8004a4b0); their state is per object,
	// so it is not folded into this struct's scalars.
	Cameras []int
}

// BindAnimatedScenery resolves the named object sets for a track's internal ID.
// Objects absent from the track's scenery are simply not bound.
func BindAnimatedScenery(trackID uint8, scenery []assets.Object) *AnimatedScenery {
	a := &AnimatedScenery{}
	for _, prefix := range TrackSceneryBindings[trackID] {
		found := findSceneryByPrefix(scenery, prefix)
		switch prefix {
		case "fan":
			a.Fans = found
		case "smokes":
			a.SmokeSlow = found
		case "smokef":
			a.SmokeFast = found
		case "redb":
			a.Billboards = found
		case "camera":
			a.Cameras = found
		}
	}
	return a
}

// findSceneryByPrefix collects every object index whose PRM name starts with
// prefix, mirroring the retail loader's `strncmp(objectName, wanted, strlen(wanted))`
// over the whole resource list.
//
// Prefix rather than equality matters because the authored names are numbered:
// TRACK01's SCENE.PRM contains no object called "smokes" at all -- it has
// "smokes1", "smokes3" and "smokes4" -- while "camera" and "redb" appear as six
// and four exact duplicates. An equality match found zero slow smoke plumes,
// which is how this was caught. It does not over-reach on this data either:
// "fan" matches the two "fan" objects without pulling in "hfan"/"hfan2".
func findSceneryByPrefix(scenery []assets.Object, prefix string) []int {
	var found []int
	lower := strings.ToLower(prefix)
	for i := range scenery {
		objectName := strings.TrimRight(scenery[i].Header.Name, "\x00")
		if strings.HasPrefix(strings.ToLower(objectName), lower) {
			found = append(found, i)
		}
	}
	return found
}

// Tick advances one game tick, matching the order and rates of the retail
// animators. Safe to call with no bound objects.
func (a *AnimatedScenery) Tick() {
	// One shared angle for every fan, advanced once per tick regardless of count.
	a.FanAngle += fanAngleStep

	a.SmokeSlowFrame = (a.SmokeSlowFrame + smokeSlowStep) % smokeFrameCount
	a.SmokeFastFrame = (a.SmokeFastFrame + smokeFastStep) % smokeFrameCount

	a.BillboardTick++
	if a.BillboardTick%billboardGate == 0 {
		a.BillboardFrame = (a.BillboardFrame + 1) % billboardFrameCount
	}
}

// FanRoll is the rotation to apply to every fan object's node. Retail passes the
// shared angle as the fourth argument of maybe_BuildRotationMatrixFromEulerAnglesAlt
// with the other two zero; that slot is roll, so fans spin about the forward axis.
func (a *AnimatedScenery) FanRoll() game.Angle { return a.FanAngle.Wrapped() }

// Overrides maps the current animation state onto per-object draw overrides,
// keyed by index into the track's Scenery slice.
//
// Frame textures come from the sets loaded by LoadFrameTextures, not from the
// track's SCENE.CMP: retail takes them from TEXTURES/SMOKE.CMP and
// TEXTURES/<set>RED.CMP, whose member counts match the animators' frame counts
// exactly. An earlier revision indexed the scenery tile array instead, which
// drew whatever happened to sit at that index -- on TRACK01 the billboards came
// out as the track's yellow-and-black warning stripes.
func (a *AnimatedScenery) Overrides() map[int]SceneryOverride {
	if !a.Animated() {
		return nil
	}
	overrides := make(map[int]SceneryOverride, len(a.Fans)+len(a.SmokeSlow)+len(a.SmokeFast)+len(a.Billboards))
	roll := a.FanRoll()
	for _, i := range a.Fans {
		overrides[i] = SceneryOverride{Roll: roll}
	}
	// Smoke and billboards both need PanelUVs. Their authored UVs address tiny
	// placeholder textures -- 8x8 for smoke, 4x4 for billboards -- because retail
	// overwrites the UVs *and* the texture page per frame, so whatever the artist
	// left in the PRM was never meant to be sampled. Reusing those UVs against a
	// real 32x96 frame samples a small, mostly-opaque corner and draws the plume
	// as a solid block, hiding the frame's own colour-key transparency.
	for _, i := range a.SmokeSlow {
		overrides[i] = SceneryOverride{
			Texture:  frameAt(a.SmokeTextures, a.SmokeSlowFrame),
			PanelUVs: true,
		}
	}
	for _, i := range a.SmokeFast {
		overrides[i] = SceneryOverride{
			Texture:  frameAt(a.SmokeTextures, a.SmokeFastFrame),
			PanelUVs: true,
		}
	}
	for _, i := range a.Billboards {
		// Billboards are multi-panel: one frame spans all four quads.
		overrides[i] = SceneryOverride{
			Texture:  frameAt(a.BillboardTextures, a.BillboardFrame),
			PanelUVs: true,
		}
	}
	return overrides
}

// frameAt selects a frame texture, or nil when the set is absent or the index is
// out of range. nil leaves the polygon's authored texture in place, which shows
// the object statically rather than showing the wrong image.
func frameAt(set []*sdl.GPUTexture, frame int) *sdl.GPUTexture {
	if frame < 0 || frame >= len(set) {
		return nil
	}
	return set[frame]
}

// Animated reports whether anything was bound, so callers can skip the work.
func (a *AnimatedScenery) Animated() bool {
	return len(a.Fans)+len(a.SmokeSlow)+len(a.SmokeFast)+len(a.Billboards)+len(a.Cameras) > 0
}

// LoadFrameTextures uploads the animation frame sets for this track: the shared
// smoke flipbook and the track's billboard pair. Missing files are not fatal --
// the affected objects simply keep their authored textures.
func (a *AnimatedScenery) LoadFrameTextures(device *Device, loader assets.Loader, trackID uint8) error {
	upload := func(name string) ([]*sdl.GPUTexture, error) {
		images, err := loader.LoadTextureSet(name)
		if err != nil {
			return nil, err
		}
		textures := make([]*sdl.GPUTexture, 0, len(images))
		for i, img := range images {
			tex, err := device.NewTexture(img.Width, img.Height, img.Pixels)
			if err != nil {
				return nil, fmt.Errorf("%s frame %d: %w", name, i, err)
			}
			textures = append(textures, tex)
		}
		return textures, nil
	}

	var problems []error
	if len(a.SmokeSlow)+len(a.SmokeFast) > 0 {
		if textures, err := upload(SmokeTextureSet); err != nil {
			problems = append(problems, err)
		} else {
			a.SmokeTextures = textures
		}
	}
	if len(a.Billboards) > 0 {
		if set, ok := TrackBillboardTextureSet[trackID]; ok {
			if textures, err := upload(set); err != nil {
				problems = append(problems, err)
			} else {
				a.BillboardTextures = textures
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("render: animation frame textures: %v", problems)
	}
	return nil
}
