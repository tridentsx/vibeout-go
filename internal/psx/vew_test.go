package psx

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeVEW(t *testing.T) {
	sections := []TrackSection{{ViewCounts: [3][5]uint16{{2, 0, 1, 0, 1}}}}
	data := make([]byte, 8)
	for i, value := range []uint16{3, 0x120, 7, 9} {
		binary.BigEndian.PutUint16(data[i*2:i*2+2], value)
	}
	visibility, err := DecodeVEW(data, sections)
	if err != nil {
		t.Fatal(err)
	}
	if len(visibility) != 1 || len(visibility[0].Lists[0][0]) != 2 || visibility[0].Lists[0][2][0] != 7 {
		t.Fatalf("visibility = %+v", visibility)
	}
}

func TestDecodeVEWCorpus(t *testing.T) {
	if _, err := os.Stat(wipeoutDiscPath); err != nil {
		t.Skipf("real disc data unavailable: %v", err)
	}
	paths, err := filepath.Glob(filepath.Join(wipeoutDiscPath, "TRACK*", "*.VEW"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 14 {
		t.Fatalf("found %d VEW files, want 14", len(paths))
	}
	for _, path := range paths {
		base := path[:len(path)-len(filepath.Ext(path))]
		trsPath := filepath.Join(filepath.Dir(path), "TRACK.TRS")
		// Alternate geometry variants carry their own matching .TRS basename
		// when one exists; otherwise the runtime track section table is used.
		if _, err := os.Stat(base + ".TRS"); err == nil {
			trsPath = base + ".TRS"
		}
		trsData, err := os.ReadFile(trsPath)
		if err != nil {
			t.Fatal(err)
		}
		sections, err := DecodeTRS(trsData)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeVEW(data, sections); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
}
