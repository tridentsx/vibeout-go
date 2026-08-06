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
	// An even slot advances to the second face carrying flag 0x01 -- see the pad evidence
	// in startingGridFace -- and here that face carries a normal, lifting the craft.
	ship := &Ship{}
	if err := PlaceShipOnStartingGrid(ship, track, 0, 0); err != nil {
		t.Fatal(err)
	}
	if ship.Position != (Vector3{X: 200, Y: 700, Z: 200}) {
		t.Fatalf("even-slot position = %+v", ship.Position)
	}
	if ship.SectionID != 0 || ship.Yaw != 3072 {
		t.Fatalf("section/yaw = %d/%d, want 0/3072", ship.SectionID, ship.Yaw)
	}

	// Slot 1 is odd, so it stays on the first flagged face, which has no normal here.
	odd := &Ship{}
	if err := PlaceShipOnStartingGrid(odd, track, 0, 1); err != nil {
		t.Fatal(err)
	}
	if odd.Position != (Vector3{X: 100, Y: 1000, Z: 200}) {
		t.Fatalf("odd-slot position = %+v", odd.Position)
	}
	if odd.Position.X == ship.Position.X {
		t.Error("both slots landed in the same lane; the stagger is not applied")
	}
}

// The two faces carrying bit 0x01 sit at the section centre and 1750 units to its
// right. Retail starts the player on the right, so the player's slot -- 14, an even one
// on the current reading of the permutation -- must advance to the second flagged face.
// See the note in grid.go: the parity has been inverted twice and the real uncertainty is
// the slot, not the parity.
func TestStartingGridLaneParity(t *testing.T) {
	faces := []assets.TrackFace{
		{Flags: psx.TrackFaceTrack},
		{},
	}
	section := assets.TrackSection{FirstFace: 0, NumFaces: 2}
	for _, tc := range []struct {
		slot, want int
	}{{0, 1}, {1, 0}, {2, 1}, {3, 0}, {13, 0}, {14, 1}} {
		index, err := startingGridFace(faces, section, tc.slot)
		if err != nil || index != tc.want {
			t.Errorf("slot %d: index/error = %d/%v, want %d/nil", tc.slot, index, err, tc.want)
		}
	}
}

// A single race starts the player in the last grid slot; other modes resolve the
// slot from qualifying or standings instead.
func TestPlayerGridSlotIsTwoShortOfTheField(t *testing.T) {
	if got := PlayerGridSlot(12); got != 10 {
		t.Errorf("PlayerGridSlot(12) = %d, want 10", got)
	}
	if got := PlayerGridSlot(15); got != 13 {
		t.Errorf("PlayerGridSlot(15) = %d, want 13", got)
	}
	if got := PlayerGridSlot(1); got != 0 {
		t.Errorf("PlayerGridSlot(1) = %d, want 0", got)
	}
	if got := PlayerGridSlot(0); got != 0 {
		t.Errorf("PlayerGridSlot(0) = %d, want 0 for a track with no grid run", got)
	}
}

// The starting pads are authored into the track as tile 1, which makes them an oracle for
// the whole placement: a craft that is not on one is in the wrong place. On Talon's Reach
// the six rearmost slots each land on a section with a pad in exactly one lane, so they pin
// the lane parity precisely. This is what showed the parity had to be inverted relative to
// a literal reading of the disassembly, and what showed the player's slot is 14 and not 13.
func TestRearmostSlotsLandOnPads(t *testing.T) {
	loader := assets.Loader{Root: "/home/epkcfsm/src/vibeout-go/assets/WIPEOUT2"}
	track, err := loader.LoadTrack("TRACK01")
	if err != nil {
		t.Skip(err)
	}
	const padTile = 1
	for slot := 9; slot <= 14; slot++ {
		section, ok := FindStartGridSection(track, TrackStartLineSection[1], slot)
		if !ok {
			t.Fatalf("slot %d: no grid section", slot)
		}
		s := track.Sections[section]
		face, err := startingGridFace(track.Faces, s, slot)
		if err != nil {
			t.Fatalf("slot %d: %v", slot, err)
		}
		if got := track.Faces[face].Tile; got != padTile {
			t.Errorf("slot %d (section %d, face %d): tile %d, want the pad tile %d",
				slot, section, face, got, padTile)
		}
	}
	// The player's own slot must land on a pad too, and on the right one. Retail puts the
	// craft on the fourth pad counting back from the front of the grid's zig-zag, which on
	// Talon's Reach is section 271, with the three pads behind it empty.
	//
	// +X is the right-hand side, which the grid map confirms: the pads alternate left, right
	// going back from the line and the lane names line up with observation once the player is
	// in the correct section.
	slots := GridSlotCount(0)
	if slots != 12 {
		t.Fatalf("an ordinary race has %d slots, want 12", slots)
	}
	section, ok := FindStartGridSection(track, TrackStartLineSection[1], PlayerGridSlot(slots))
	if !ok {
		t.Fatal("no section for the player's slot")
	}
	if section != 273 {
		t.Errorf("the player starts in section %d, want 273 -- the right-hand pad with left, "+
			"right, left on the pads behind it", section)
	}
	s := track.Sections[section]
	face, err := startingGridFace(track.Faces, s, PlayerGridSlot(slots))
	if err != nil {
		t.Fatal(err)
	}
	if got := face - int(s.FirstFace); got != 2 {
		t.Errorf("the player is on face offset %d, want 2 (the right-hand lane)", got)
	}
	// And that lane really is the right-hand one: +X is right for a craft heading along +Z
	// with -Y up, which is the convention the rest of the port uses.
	f := track.Faces[face]
	v0, v2 := track.Vertices[f.Indices[0]], track.Vertices[f.Indices[2]]
	if dx := (int(v0.X)+int(v2.X))/2 - int(s.X); dx <= 0 {
		t.Errorf("the player's pad is %d from the section centre, so it is not the right lane", dx)
	}
	if got := track.Faces[face].Tile; got != padTile {
		t.Errorf("the player's face is tile %d, want the pad tile %d", got, padTile)
	}
}
