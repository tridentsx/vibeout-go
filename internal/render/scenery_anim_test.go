package render

import (
	"testing"

	"github.com/Zyko0/go-sdl3/sdl"

	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/game"
	"github.com/tridentsx/wipeout-go/internal/psx"
)

func sceneryNamed(names ...string) []assets.Object {
	objects := make([]assets.Object, len(names))
	for i, n := range names {
		objects[i] = psx.Object{Header: psx.ObjectHeader{Name: n}}
	}
	return objects
}

// The mapping is read base+index from maybe_MenuIndexToTrackId (0x80094d50) at
// 19 sites. Talon's Reach being ID 1 is what makes the ID-1 block -- fans, smoke
// and billboards -- the one that runs on the first track.
func TestMenuIndexToTrackIDMatchesRetailTable(t *testing.T) {
	want := [8]uint8{1, 8, 13, 20, 2, 17, 6, 7}
	if MenuIndexToTrackID != want {
		t.Errorf("track ID table = %v, want %v", MenuIndexToTrackID, want)
	}
	if MenuIndexToTrackID[0] != 1 {
		t.Error("Talon's Reach (menu 0) must map to internal ID 1")
	}
}

func TestBindAnimatedSceneryResolvesNamesAndCounts(t *testing.T) {
	scenery := sceneryNamed("fan", "wall", "fan", "smokes", "fan", "fan", "redb", "camera")
	a := BindAnimatedScenery(1, scenery)

	if len(a.Fans) != 4 {
		t.Errorf("bound %d fans, want all 4 -- retail applies no count cap", len(a.Fans))
	}
	if len(a.SmokeSlow) != 1 || len(a.Billboards) != 1 || len(a.Cameras) != 1 {
		t.Errorf("smokes=%d billboards=%d cameras=%d, want 1 each",
			len(a.SmokeSlow), len(a.Billboards), len(a.Cameras))
	}
	if len(a.SmokeFast) != 0 {
		t.Errorf("smokef=%d, want 0 -- none present in this scenery", len(a.SmokeFast))
	}
	if !a.Animated() {
		t.Error("Animated() false despite bound objects")
	}
}

// The authored names are numbered: TRACK01 has no "smokes" object, only
// "smokes1"/"smokes3"/"smokes4". An equality match found zero slow smoke, which
// is the bug this pins.
func TestBindMatchesNumberedNameVariants(t *testing.T) {
	scenery := sceneryNamed("smokes4", "smokes3", "smokes1", "smokef7", "hfan", "fan")
	a := BindAnimatedScenery(1, scenery)
	if len(a.SmokeSlow) != 3 {
		t.Errorf("smokes matched %d objects, want 3 numbered variants", len(a.SmokeSlow))
	}
	if len(a.SmokeFast) != 1 {
		t.Errorf("smokef matched %d, want 1", len(a.SmokeFast))
	}
	if len(a.Fans) != 1 {
		t.Errorf("fan matched %d, want 1 -- must not pull in \"hfan\"", len(a.Fans))
	}
}

func TestBindAnimatedSceneryUnknownTrackBindsNothing(t *testing.T) {
	// Only ID 1 is transcribed; an unlisted ID must animate nothing rather than
	// fall back to another track's bindings.
	a := BindAnimatedScenery(13, sceneryNamed("fan", "smokes", "redb"))
	if a.Animated() {
		t.Error("unlisted track ID bound objects; should bind none")
	}
}

// One shared angle for every fan, advanced 100 per tick at 0x80049094.
func TestFanAngleAdvancesOneHundredPerTick(t *testing.T) {
	a := BindAnimatedScenery(1, sceneryNamed("fan", "fan"))
	if a.FanAngle != 0 {
		t.Fatalf("initial fan angle = %d, want 0", a.FanAngle)
	}
	a.Tick()
	if a.FanAngle != 100 {
		t.Errorf("after one tick = %d, want 100", a.FanAngle)
	}
	for i := 0; i < 9; i++ {
		a.Tick()
	}
	if a.FanAngle != 1000 {
		t.Errorf("after ten ticks = %d, want 1000", a.FanAngle)
	}
}

func TestFanCompletesRevolutionInFortyOneTicks(t *testing.T) {
	// 4096 angle units / 100 per tick = 41 ticks, ~1.6s at the retail 25Hz tick.
	a := &AnimatedScenery{}
	for i := 0; i < 41; i++ {
		a.Tick()
	}
	if a.FanAngle < game.AngleFullTurn {
		t.Errorf("41 ticks reached %d, expected at least one full turn (%d)",
			a.FanAngle, game.AngleFullTurn)
	}
	if roll := a.FanRoll(); roll >= game.AngleFullTurn {
		t.Errorf("FanRoll() = %d, must be wrapped below %d", roll, game.AngleFullTurn)
	}
}

// "smokes" advances one frame per tick and "smokef" two -- the last argument of
// maybe_AnimateSmokeTextureFrames at its two call sites. Both cycle 25 frames.
func TestSmokeFastAdvancesTwiceAsQuicklyAsSlow(t *testing.T) {
	a := &AnimatedScenery{}
	for i := 0; i < 5; i++ {
		a.Tick()
	}
	if a.SmokeSlowFrame != 5 {
		t.Errorf("slow smoke frame = %d, want 5", a.SmokeSlowFrame)
	}
	if a.SmokeFastFrame != 10 {
		t.Errorf("fast smoke frame = %d, want 10", a.SmokeFastFrame)
	}
}

func TestSmokeFramesWrapAtTwentyFive(t *testing.T) {
	a := &AnimatedScenery{}
	for i := 0; i < 60; i++ {
		a.Tick()
		if a.SmokeSlowFrame < 0 || a.SmokeSlowFrame >= smokeFrameCount {
			t.Fatalf("slow frame %d out of range after %d ticks", a.SmokeSlowFrame, i+1)
		}
		if a.SmokeFastFrame < 0 || a.SmokeFastFrame >= smokeFrameCount {
			t.Fatalf("fast frame %d out of range after %d ticks", a.SmokeFastFrame, i+1)
		}
	}
	// 25 ticks of +1 returns the slow set to its starting frame.
	b := &AnimatedScenery{}
	for i := 0; i < smokeFrameCount; i++ {
		b.Tick()
	}
	if b.SmokeSlowFrame != 0 {
		t.Errorf("slow frame after a full 25-tick cycle = %d, want 0", b.SmokeSlowFrame)
	}
}

// Billboards hold each frame for 20 ticks and alternate between two.
func TestBillboardHoldsEachFrameForTwentyTicks(t *testing.T) {
	a := &AnimatedScenery{}
	for i := 0; i < 19; i++ {
		a.Tick()
	}
	if a.BillboardFrame != 0 {
		t.Errorf("frame changed early, at tick 19: %d", a.BillboardFrame)
	}
	a.Tick() // tick 20
	if a.BillboardFrame != 1 {
		t.Errorf("frame at tick 20 = %d, want 1", a.BillboardFrame)
	}
	for i := 0; i < 20; i++ {
		a.Tick()
	}
	if a.BillboardFrame != 0 {
		t.Errorf("frame at tick 40 = %d, want 0 (two-frame cycle)", a.BillboardFrame)
	}
}

func TestTickIsSafeWithNothingBound(t *testing.T) {
	a := BindAnimatedScenery(1, nil)
	if a.Animated() {
		t.Error("Animated() true with empty scenery")
	}
	a.Tick() // must not panic
}

func TestOverridesGiveFansRollAndFramesTextures(t *testing.T) {
	scenery := sceneryNamed("fan", "smokes", "redb", "smokef")
	a := BindAnimatedScenery(1, scenery)
	for i := 0; i < 3; i++ {
		a.Tick()
	}
	overrides := a.Overrides()

	fan, ok := overrides[0]
	if !ok {
		t.Fatal("no override for the fan object")
	}
	if fan.Roll != a.FanRoll() {
		t.Errorf("fan roll = %d, want %d", fan.Roll, a.FanRoll())
	}
	if fan.Texture != nil {
		t.Error("fan must not get a texture override")
	}

	smoke, ok := overrides[1]
	if !ok {
		t.Fatal("no override for the smokes object")
	}
	if smoke.Roll != 0 {
		t.Errorf("smoke should not roll, got %d", smoke.Roll)
	}
	// No frame textures loaded in this test, so the override must leave the
	// authored texture alone rather than selecting nothing and drawing blank.
	if smoke.Texture != nil {
		t.Error("smoke got a texture with no frame set loaded")
	}
}

func TestFrameAtRefusesOutOfRangeRatherThanClamping(t *testing.T) {
	// Selecting a neighbouring frame would draw a plausible-looking wrong image
	// and hide the bug; nil keeps the authored texture visible instead.
	if frameAt(nil, 0) != nil {
		t.Error("empty frame set should yield nil")
	}
	set := make([]*sdl.GPUTexture, 3)
	if frameAt(set, 5) != nil {
		t.Error("out-of-range frame should yield nil, not a clamped neighbour")
	}
	if frameAt(set, -1) != nil {
		t.Error("negative frame should yield nil")
	}
}

func TestOverridesNilWhenNothingAnimated(t *testing.T) {
	a := BindAnimatedScenery(13, sceneryNamed("fan"))
	if a.Overrides() != nil {
		t.Error("expected nil overrides when nothing is bound")
	}
}

// A billboard's four quads between them show one frame, so the UV mapping must
// span the object's extent rather than repeat per polygon. Reusing the authored
// UVs drew four copies in a 2x2 grid; the authored UVs address a 4x4 placeholder
// texture anyway, while the real frames are 128x128.
func TestPanelUVsSpanTheObjectExtent(t *testing.T) {
	e := objectExtent{
		uAxis: game.Vector3{X: 1}, vAxis: game.Vector3{Y: 1},
		minU: -100, maxU: 100, minV: -50, maxV: 50, valid: true,
	}
	corners := []struct {
		x, y, z float32
		u, v    float32
		corner  string
	}{
		{-100, -50, 0, 0, 0, "top-left"},
		{100, -50, 0, 1, 0, "top-right"},
		{-100, 50, 0, 0, 1, "bottom-left"},
		{100, 50, 0, 1, 1, "bottom-right"},
		{0, 0, 0, 0.5, 0.5, "centre"},
	}
	for _, c := range corners {
		uv := e.uvFor(c.x, c.y, c.z)
		if uv.X != c.u || uv.Y != c.v {
			t.Errorf("%s (%.0f,%.0f) -> (%.2f,%.2f), want (%.2f,%.2f)",
				c.corner, c.x, c.y, uv.X, uv.Y, c.u, c.v)
		}
	}
}

func TestObjectWithNoGeometryHasNoExtent(t *testing.T) {
	if e := measureObjectExtent(assets.Object{}); e.valid {
		t.Error("an object with no vertices reported a valid extent")
	}
}

// Both animated object kinds carry placeholder textures whose UVs must not be
// used: TRACK01's smoke quads reference an 8x8 texture and its billboards a 4x4,
// while the real frames are 32x96 and 128x128. Sampling with the authored UVs
// drew smoke as a solid block, because it picked a small opaque corner instead of
// the frame's colour-keyed plume.
func TestFrameAnimatedObjectsIgnoreAuthoredUVs(t *testing.T) {
	a := BindAnimatedScenery(1, sceneryNamed("smokes1", "smokef1", "redb", "fan"))
	overrides := a.Overrides()
	for label, indices := range map[string][]int{
		"smokes": a.SmokeSlow, "smokef": a.SmokeFast, "redb": a.Billboards,
	} {
		for _, i := range indices {
			if !overrides[i].PanelUVs {
				t.Errorf("%s object %d must span its geometry, not reuse placeholder UVs", label, i)
			}
		}
	}
	// Fans keep their authored UVs -- they are not frame-animated, only rotated.
	for _, i := range a.Fans {
		if overrides[i].PanelUVs {
			t.Errorf("fan object %d should keep its authored UVs", i)
		}
	}
}
