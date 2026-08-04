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
