package game

import (
	"testing"

	"github.com/tridentsx/wipeout-go/internal/assets"
)

// Every menu circuit must load and place a craft, or picking it from the menu fails.
func TestAllTracksLoadAndSpawn(t *testing.T) {
	loader := assets.Loader{Root: "/home/epkcfsm/src/vibeout-go/assets/WIPEOUT2"}
	for i, entry := range TrackMenuEntries {
		dir, ok := TrackDirectories[entry.TrackID]
		if !ok {
			t.Errorf("menu %d (%s): no directory", i, entry.Name)
			continue
		}
		track, err := loader.LoadTrack(dir)
		if err != nil {
			t.Errorf("menu %d (%s, %s): %v", i, entry.Name, dir, err)
			continue
		}
		line, ok := TrackStartLineSection[entry.TrackID]
		if !ok {
			t.Errorf("menu %d (%s): no start line section", i, entry.Name)
			continue
		}
		var ship Ship
		if err := PlaceShipOnStartingGrid(&ship, track, line, PlayerGridSlot(15)); err != nil {
			t.Errorf("menu %d (%s): spawn failed: %v", i, entry.Name, err)
			continue
		}
		t.Logf("%-14s %-8s %3d sections, spawn at %3d (%.0f,%.0f,%.0f)",
			entry.Name, dir, len(track.Sections), ship.SectionID,
			ship.Position.X, ship.Position.Y, ship.Position.Z)
	}
}
