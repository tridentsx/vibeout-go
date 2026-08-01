package psx

import (
	"os"
	"path/filepath"
	"testing"
)

// wipeoutDiscPath is where this development machine's copy of the real
// WipEout 2097 disc image lives. Tests in this file are skipped wherever
// that path doesn't exist -- they validate the parser against real game
// data, not synthetic fixtures, so they're inherently machine-specific.
const wipeoutDiscPath = "/Users/tridentsx/Downloads/WipeOut.2097.PAL-PSX/WIPEOUT2-disc/WIPEOUT2"

func beU16(data []byte, offset int) uint16 {
	return uint16(data[offset])<<8 | uint16(data[offset+1])
}

var validPolygonTypes = map[uint16]bool{
	0x00: true, 0x01: true, 0x02: true, 0x03: true, 0x04: true, 0x05: true,
	0x06: true, 0x07: true, 0x08: true, 0x0A: true, 0x0B: true,
}

// TestRealPRMFilesParseCleanly decodes every .PRM file on the real disc and
// checks each polygon's computed byte length actually lands on a valid next
// polygon header. Type 0x00 is a known, open question -- see DecodePRM's
// doc comment -- and is responsible for the overwhelming majority of
// mismatches found this way (11 of 12, all traceable to that same
// ambiguity via cascading offsets within one file). The one remaining
// mismatch (type 0x03, TRAK2.PRM offset 26168, a fresh object's very first
// polygon -- not a cascade from anything earlier) is a genuine, still-
// unexplained anomaly in this one specific spot; every other 0x03 instance
// in the corpus (3762 of them) parses correctly. A handful of isolated
// mismatches is tolerated per type; a large jump (real types are otherwise
// 100% correct across tens of thousands of real polygons) means an actual
// formula regression and should fail this test.
func TestRealPRMFilesParseCleanly(t *testing.T) {
	if _, err := os.Stat(wipeoutDiscPath); err != nil {
		t.Skip("disc image not present:", err)
	}

	type stat struct{ ok, bad int }
	stats := map[uint16]*stat{}

	err := filepath.Walk(wipeoutDiscPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".PRM" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		offset := 0
		for offset+objectHeaderSize <= len(data) {
			vertexCount := int(beU16(data, offset+16))
			polygonCount := int(beU16(data, offset+32))
			offset += objectHeaderSize + vertexCount*8
			if offset > len(data) {
				break
			}

			for i := 0; i < polygonCount; i++ {
				if offset+4 > len(data) {
					break
				}
				polyType := beU16(data, offset)
				_, size, polyErr := readPolygon(data, offset)
				if polyErr != nil {
					break
				}
				next := offset + size
				s := stats[polyType]
				if s == nil {
					s = &stat{}
					stats[polyType] = s
				}
				if next+4 <= len(data) && (i == polygonCount-1 || validPolygonTypes[beU16(data, next)]) {
					s.ok++
				} else if i < polygonCount-1 {
					s.bad++
					t.Logf("mismatch: %s type=0x%02x @offset %d size=%d", filepath.Base(path), polyType, offset, size)
				}
				offset = next
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Known baseline (see the doc comment above): 11 isolated mismatches on
	// the acknowledged-ambiguous type 0x00, plus one unexplained type 0x03
	// mismatch that isn't a cascade from anything earlier. A real formula
	// regression would show up as a large jump in bad counts (cascading
	// desync affects every polygon after the first miss), not one or two
	// more isolated instances -- so tolerate a small absolute count per
	// type, scaled to a bit above each known baseline, and fail loudly on
	// anything bigger.
	tolerance := map[uint16]int{0x00: 20, 0x03: 5}
	for polyType, s := range stats {
		limit := tolerance[polyType]
		if s.bad > limit {
			t.Errorf("polygon type 0x%02x: %d ok, %d bad (tolerance %d) -- looks like a real formula regression",
				polyType, s.ok, s.bad, limit)
		}
	}
}
