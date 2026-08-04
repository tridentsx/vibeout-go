package physics

import (
	"testing"

	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/game"
)

func TestUpdateShipTrackSectionSearchesLocalWindow(t *testing.T) {
	sections := makeRingSections(10)
	ship := &Ship{
		Position:  Vector3{X: 700},
		SectionID: 5,
		Flags:     game.ShipFlagFarFromTrackSection,
	}
	distance, err := UpdateShipTrackSection(ship, &assets.Track{Sections: sections})
	if err != nil {
		t.Fatal(err)
	}
	if distance != 0 || ship.SectionID != 7 || ship.PreviousSectionID != 5 {
		t.Fatalf("section update = distance %v current %d previous %d", distance, ship.SectionID, ship.PreviousSectionID)
	}
	if ship.Flags&game.ShipFlagFarFromTrackSection == 0 {
		t.Fatal("nearest-section search unexpectedly changed physics branch flags")
	}
}

func TestUpdateShipTrackSectionWeightsYByQuarterWithoutChangingFlags(t *testing.T) {
	sections := makeRingSections(7)
	ship := &Ship{Position: Vector3{X: 10000, Y: 4000}, SectionID: 0}
	distance, err := UpdateShipTrackSection(ship, &assets.Track{Sections: sections})
	if err != nil {
		t.Fatal(err)
	}
	if distance < 9000 || ship.Flags&game.ShipFlagFarFromTrackSection != 0 {
		t.Fatalf("far update = distance %v flags %#x", distance, ship.Flags)
	}
}

func makeRingSections(count int) []assets.TrackSection {
	sections := make([]assets.TrackSection, count)
	for i := range sections {
		sections[i] = assets.TrackSection{
			NextJunction: -1,
			Previous:     int32((i - 1 + count) % count),
			Next:         int32((i + 1) % count),
			X:            int32(i * 100),
		}
	}
	return sections
}
