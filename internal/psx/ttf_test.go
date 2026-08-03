package psx

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeTTF(t *testing.T) {
	data := make([]byte, 2*ttfRecordSize)
	for i := 0; i < TTFValueCount*2; i++ {
		binary.BigEndian.PutUint16(data[i*2:i*2+2], uint16(i))
	}
	records, err := DecodeTTF(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Values[20] != 20 || records[1].Values[0] != 21 {
		t.Fatalf("decoded records = %+v", records)
	}
}

func TestDecodeTTFRejectsPartialRecord(t *testing.T) {
	if _, err := DecodeTTF(make([]byte, ttfRecordSize-1)); err == nil {
		t.Fatal("accepted partial TTF record")
	}
}

func TestDecodeTTFCorpus(t *testing.T) {
	if _, err := os.Stat(wipeoutDiscPath); err != nil {
		t.Skipf("real disc data unavailable: %v", err)
	}
	var paths []string
	err := filepath.Walk(wipeoutDiscPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.EqualFold(filepath.Ext(path), ".TTF") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 10 {
		t.Fatalf("found %d TTF files, want 10", len(paths))
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeTTF(data); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
}
