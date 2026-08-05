package game

import (
	"testing"

	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/psx"
)

func TestPlaceShipOnStartingGrid(t *testing.T) {
	track := &assets.Track{
		Vertices: []assets.TrackVertex{
			{X: 0, Y: 1000, Z: 0}, {X: 100, Y: 1000, Z: 0},
			{X: 200, Y: 1000, Z: 400}, {X: 300, Y: 1000, Z: 400},
		},
		Faces: []assets.TrackFace{
			{Indices: [4]uint16{0, 1, 2, 3}, Flags: psx.TrackFaceTrack},
			{Indices: [4]uint16{1, 1, 3, 3}, NormalY: -4096},
		},
		Sections: []assets.TrackSection{
			{Next: 1, FirstFace: 0, NumFaces: 2},
			{Next: 0, X: 1000},
		},
	}
	ship := &Ship{}
	if err := PlaceShipOnStartingGrid(ship, track, 0, 0); err != nil {
		t.Fatal(err)
	}
	if ship.Position != (Vector3{X: 200, Y: 700, Z: 200}) {
		t.Fatalf("position = %+v", ship.Position)
	}
	if ship.SectionID != 0 || ship.Yaw != 3072 {
		t.Fatalf("section/yaw = %d/%d, want 0/3072", ship.SectionID, ship.Yaw)
	}
}

func TestStartingGridOddSlotUsesDrivingFace(t *testing.T) {
	faces := []assets.TrackFace{
		{Flags: psx.TrackFaceTrack},
		{},
	}
	section := assets.TrackSection{FirstFace: 0, NumFaces: 2}
	index, err := startingGridFace(faces, section, 1)
	if err != nil || index != 0 {
		t.Fatalf("index/error = %d/%v, want 0/nil", index, err)
	}
}

// A single race starts the player in the last grid slot; other modes resolve the
// slot from qualifying or standings instead.
func TestPlayerGridSlotIsTheLastOne(t *testing.T) {
	if got := PlayerGridSlot(15); got != 14 {
		t.Errorf("PlayerGridSlot(15) = %d, want 14", got)
	}
	if got := PlayerGridSlot(1); got != 0 {
		t.Errorf("PlayerGridSlot(1) = %d, want 0", got)
	}
	if got := PlayerGridSlot(0); got != 0 {
		t.Errorf("PlayerGridSlot(0) = %d, want 0 for a track with no grid run", got)
	}
}
