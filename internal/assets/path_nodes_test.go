package assets

import (
	"testing"

	"github.com/tridentsx/wipeout-go/internal/psx"
)

// Path nodes are authored per circuit. Talon's Reach and Valparaiso have none, which is
// why the mechanism looked dead when only Talon's Reach was examined -- including in a
// live emulator session that read the flag three times across a full race start.
func TestPathNodesArePerTrack(t *testing.T) {
	loader := Loader{Root: "/home/epkcfsm/src/wipeout-go/assets/WIPEOUT2"}
	loader.Root = "/home/epkcfsm/src/vibeout-go/assets/WIPEOUT2"
	want := map[string]int{
		"TRACK01": 0, "TRACK02": 1, "TRACK06": 6, "TRACK07": 5,
		"TRACK08": 1, "TRACK13": 0, "TRACK17": 2, "TRACK20": 3,
	}
	for dir, expected := range want {
		track, err := loader.LoadTrack(dir)
		if err != nil {
			t.Skipf("%s unavailable: %v", dir, err)
		}
		got := 0
		for _, s := range track.Sections {
			if s.Flags&psx.SectionFlagPathStart != 0 {
				got++
			}
		}
		if got != expected {
			t.Errorf("%s has %d path nodes, want %d", dir, got, expected)
		}
	}
	// The two circuits with none are the reason this took a live session to settle.
	for _, dir := range []string{"TRACK01", "TRACK13"} {
		track, err := loader.LoadTrack(dir)
		if err != nil {
			continue
		}
		for i, s := range track.Sections {
			if s.Flags&psx.SectionFlagPathStart != 0 {
				t.Errorf("%s section %d unexpectedly carries a path node", dir, i)
			}
		}
	}
}
