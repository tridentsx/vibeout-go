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
	// Slot 0 is an even slot, so retail uses the section's first face carrying
	// flag bit 0x01 without advancing: the midpoint of its vertices 0 and 2,
	// with no normal offset because that face has no normal here.
	ship := &Ship{}
	if err := PlaceShipOnStartingGrid(ship, track, 0, 0); err != nil {
		t.Fatal(err)
	}
	if ship.Position != (Vector3{X: 100, Y: 1000, Z: 200}) {
		t.Fatalf("even-slot position = %+v", ship.Position)
	}
	if ship.SectionID != 0 || ship.Yaw != 3072 {
		t.Fatalf("section/yaw = %d/%d, want 0/3072", ship.SectionID, ship.Yaw)
	}

	// Slot 1 is odd, so it advances one face -- the other lane -- and that face
	// does carry a normal, lifting the craft by normal * 75/1024.
	odd := &Ship{}
	if err := PlaceShipOnStartingGrid(odd, track, 0, 1); err != nil {
		t.Fatal(err)
	}
	if odd.Position != (Vector3{X: 200, Y: 700, Z: 200}) {
		t.Fatalf("odd-slot position = %+v", odd.Position)
	}
	if odd.Position.X == ship.Position.X {
		t.Error("both slots landed in the same lane; the stagger is not applied")
	}
}

// Retail advances one face only when the slot's side flag is set, and that flag
// starts at 0 for slot 0 and toggles per slot. So even slots take the first face
// carrying bit 0x01 and odd slots take the one after it. This was inverted, which
// left the craft a lane away from its pad.
func TestStartingGridLaneParity(t *testing.T) {
	faces := []assets.TrackFace{
		{Flags: psx.TrackFaceTrack},
		{},
	}
	section := assets.TrackSection{FirstFace: 0, NumFaces: 2}
	for _, tc := range []struct {
		slot, want int
	}{{0, 0}, {1, 1}, {2, 0}, {3, 1}, {14, 0}} {
		index, err := startingGridFace(faces, section, tc.slot)
		if err != nil || index != tc.want {
			t.Errorf("slot %d: index/error = %d/%v, want %d/nil", tc.slot, index, err, tc.want)
		}
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
