package psx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// wipeoutDiscPath is the extracted WipEout 2097 disc tree kept under the
// repository's git-ignored assets/ directory. Package tests run with the
// package directory as the working directory, so this is relative to
// internal/psx/ -- two levels below the repo root. Tests that need real disc
// data skip when it is absent; they validate the parsers against real game
// data rather than synthetic fixtures.
var wipeoutDiscPath = filepath.Join("..", "..", "assets", "WIPEOUT2")

func beU16(data []byte, offset int) uint16 {
	return uint16(data[offset])<<8 | uint16(data[offset+1])
}

var validPolygonTypes = map[uint16]bool{
	1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true, 8: true,
	10: true, 11: true, 12: true, 13: true, 14: true, 15: true, 16: true,
	17: true, 18: true, 19: true, 20: true, 21: true, 22: true, 23: true,
}

func isEditorPRM(path string) bool {
	path = strings.ToUpper(filepath.ToSlash(path))
	return strings.HasSuffix(path, "/COMMON/SKY.PRM") ||
		strings.HasSuffix(path, "/COMMON/TRACK.PRM") ||
		strings.HasSuffix(path, "/TRACK08/TRAK2.PRM")
}

// TestRealPRMFilesParseCleanly decodes every .PRM file on the real disc and
// checks each polygon's binary-confirmed byte length lands exactly on the
// next valid polygon or object boundary.
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
		if isEditorPRM(path) {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		offset := 0
		for offset+objectHeaderSize <= len(data) {
			vertexCount := int(beU16(data, offset+16))
			normalCount := int(beU16(data, offset+24))
			polygonCount := int(beU16(data, offset+32))
			offset += objectHeaderSize + (vertexCount+normalCount)*8
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

	for polyType, s := range stats {
		if s.bad != 0 {
			t.Errorf("polygon type 0x%02x: %d ok, %d bad", polyType, s.ok, s.bad)
		}
	}
}

func TestDecodeEveryRealPRMFile(t *testing.T) {
	if _, err := os.Stat(wipeoutDiscPath); err != nil {
		t.Skip("disc image not present:", err)
	}
	runtimeCount, editorCount := 0, 0
	err := filepath.Walk(wipeoutDiscPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".PRM" {
			return err
		}
		if isEditorPRM(path) {
			editorCount++
			return nil
		}
		runtimeCount++
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := DecodePRM(data); err != nil {
			t.Errorf("%s: %v", path, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtimeCount != 48 || editorCount != 3 {
		t.Fatalf("found %d runtime and %d editor PRM files, want 48 and 3", runtimeCount, editorCount)
	}
}
