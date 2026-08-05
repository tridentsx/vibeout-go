package render

import (
	"math"
	"testing"

	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/game"
)

func TestNewChaseCameraUsesConfirmedBinaryAnchor(t *testing.T) {
	ship := &game.Ship{Position: game.Vector3{X: 0, Y: 0, Z: 0}}
	cam := NewChaseCamera(ship)

	if cam.Position.Z != -1024 {
		t.Errorf("camera Z = %v, want -1024", cam.Position.Z)
	}
	if cam.Position.Y != -200 {
		t.Errorf("camera Y = %v, want -200", cam.Position.Y)
	}
}

func TestRaceCameraCockpitUsesLocalUpOffsetAndRoll(t *testing.T) {
	ship := &game.Ship{Position: game.Vector3{X: 10, Y: 20, Z: 30}}
	camera := NewRaceCamera(ship, nil)
	camera.ToggleView(ship)
	camera.Update(ship, nil)

	if camera.Position != (game.Vector3{X: 10, Y: -108, Z: 30}) {
		t.Errorf("cockpit position = %+v, want (10,-108,30)", camera.Position)
	}
	ship.Roll = 123
	camera.Update(ship, nil)
	if camera.Roll != -ship.Roll || ship.Flags&game.ShipFlagCockpitCamera == 0 {
		t.Errorf("cockpit camera roll/flag mismatch: camera=%d ship=%d flags=%#x", camera.Roll, ship.Roll, ship.Flags)
	}
}

func TestRaceStartCameraHandsOffAtRetailTimer(t *testing.T) {
	sections := make([]assets.TrackSection, 12)
	for i := range sections {
		previous := i - 1
		if previous < 0 {
			previous = 0
		}
		sections[i] = assets.TrackSection{Previous: int32(previous), X: int32(i * 200), Z: int32(i * 800)}
	}
	ship := &game.Ship{SectionID: 8, Position: game.Vector3{X: 1600, Y: 0, Z: 6400}}
	camera := NewRaceCamera(ship, sections)
	camera.BeginRaceStart()
	if !camera.RaceStartActive || camera.RaceStartTimer != 0xa6 {
		t.Fatalf("start state = active:%v timer:%#x", camera.RaceStartActive, camera.RaceStartTimer)
	}
	for i := 0; i < 66; i++ {
		camera.Update(ship, sections)
		camera.AdvanceRaceStart()
	}
	if camera.RaceStartTimer != 0x64 || !camera.RaceStartActive {
		t.Fatalf("before handoff = active:%v timer:%#x", camera.RaceStartActive, camera.RaceStartTimer)
	}
	camera.Update(ship, sections)
	if camera.RaceStartActive {
		t.Fatal("race-start camera did not hand off at timer 0x64")
	}
}

// offCenterTrack builds a straight track and returns a ship parked off the
// centerline, so the chase camera has a non-zero error to correct.
func offCenterTrack() ([]assets.TrackSection, *game.Ship) {
	sections := make([]assets.TrackSection, 12)
	for i := range sections {
		previous := i - 1
		if previous < 0 {
			previous = 0
		}
		next := i + 1
		if next >= len(sections) {
			next = i
		}
		sections[i] = assets.TrackSection{
			Previous: int32(previous), Next: int32(next), X: 0, Z: int32(i * 800),
		}
	}
	ship := &game.Ship{SectionID: 5, Position: game.Vector3{X: 900, Y: 0, Z: 4000}}
	return sections, ship
}

// The retail clearance accumulator is separate from the spring and decays more
// slowly (/16 against /8), read from 0x80020850-0x8002088c. An earlier revision
// folded the term into SpringVelocity using wipeout-rewrite's formula because
// 0x80020608 was unreadable; these pin the retail behaviour so that cannot
// silently regress.
func TestChaseCameraAccumulatesVerticalClearance(t *testing.T) {
	sections, ship := offCenterTrack()
	camera := NewRaceCamera(ship, sections)

	if camera.ClearanceBias <= 0 {
		t.Fatalf("clearance did not accumulate off-centerline: %v", camera.ClearanceBias)
	}
	first := camera.ClearanceBias
	for i := 0; i < 20; i++ {
		camera.Update(ship, sections)
	}
	if camera.ClearanceBias <= first {
		t.Errorf("clearance %v did not grow past the first frame's %v", camera.ClearanceBias, first)
	}
}

func TestChaseCameraClearanceDecaysBySixteenth(t *testing.T) {
	// The decay is `clearance -= clearance >> 4` (0x80020870-0x80020874). It
	// cannot be isolated by centering the ship: the anchor sits 200 units off
	// the section line by construction, so a small increment always runs too.
	// Starting from a large bias makes the decay dominate that increment by
	// three orders of magnitude, which pins the coefficient rather than the sum.
	sections := make([]assets.TrackSection, 4)
	for i := range sections {
		previous := i - 1
		if previous < 0 {
			previous = 0
		}
		sections[i] = assets.TrackSection{Previous: int32(previous), X: 0, Z: int32(i * 800)}
	}
	ship := &game.Ship{SectionID: 1, Position: game.Vector3{X: 0, Y: 0, Z: 800}}
	camera := NewRaceCamera(ship, sections)
	camera.SpringVelocity = game.Vector3{}

	const start = 160000
	camera.ClearanceBias = start
	camera.Update(ship, sections)

	want := float32(start - start/16)
	if diff := camera.ClearanceBias - want; diff > 20 || diff < -20 {
		t.Errorf("clearance after one decay = %v, want about %v (15/16 of %v)",
			camera.ClearanceBias, want, float32(start))
	}
	// And confirm it is /16 rather than the spring's /8, which would land at 140000.
	if camera.ClearanceBias < float32(start)*0.9 {
		t.Errorf("clearance %v decayed faster than 1/16 -- looks like the spring's 1/8",
			camera.ClearanceBias)
	}
}

func TestChaseCameraClearanceLiftsTheCamera(t *testing.T) {
	// Y points down in WipEout's world, so subtracting the accumulator raises
	// the view. Compare against the same camera with the accumulator zeroed.
	sections, ship := offCenterTrack()
	withBias := NewRaceCamera(ship, sections)
	withBias.ClearanceBias = 4000
	withBias.Update(ship, sections)

	noBias := NewRaceCamera(ship, sections)
	noBias.ClearanceBias = 0
	noBias.SpringVelocity = withBias.SpringVelocity
	noBias.Update(ship, sections)

	if withBias.Position.Y >= noBias.Position.Y {
		t.Errorf("clearance did not lift the camera: withBias Y=%v, noBias Y=%v",
			withBias.Position.Y, noBias.Position.Y)
	}
}

func TestProjectTopDownShipAheadIsPositiveZ(t *testing.T) {
	cam := Camera{Position: game.Vector3{X: 0, Y: 0, Z: -40}, Yaw: 0}
	_, z := cam.ProjectTopDown(game.Vector3{X: 0, Y: 0, Z: 0})
	if z <= 0 {
		t.Errorf("expected a point ahead of the camera to project to positive Z, got %v", z)
	}
}

func TestSectionCameraAimsAtNextSection(t *testing.T) {
	track := &assets.Track{Sections: make([]assets.TrackSection, 30)}
	for i := range track.Sections {
		track.Sections[i] = assets.TrackSection{X: int32(i * 300), Y: int32(100 + i*10), Z: int32(i * 1000), Previous: int32(i - 1), Next: int32(i + 1)}
	}
	camera := SectionCamera(track, 10)
	target := camera.WorldToCamera(sectionPoint(track.Sections[18]))
	if math.Abs(float64(target.X)) > 20 || target.Y <= 0 || target.Z <= 0 {
		t.Fatalf("next section in camera space = %+v camera=%+v, want forward view", target, camera)
	}
}
