package psx

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeWAD(t *testing.T) {
	data := make([]byte, 2+2*wadEntrySize+5)
	binary.LittleEndian.PutUint16(data[:2], 2)
	writeEntry := func(index int, name string, size uint32) {
		offset := 2 + index*wadEntrySize
		copy(data[offset:offset+16], name)
		binary.LittleEndian.PutUint32(data[offset+16:offset+20], size)
		binary.LittleEndian.PutUint32(data[offset+20:offset+24], size)
	}
	writeEntry(0, "ONE.TIM", 3)
	writeEntry(1, "TWO.PRM", 2)
	copy(data[2+2*wadEntrySize:], []byte{1, 2, 3, 4, 5})

	entries, err := DecodeWAD(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name != "one.tim" || entries[1].Name != "two.prm" {
		t.Fatalf("entries = %+v", entries)
	}
	if got := entries[1].Data; len(got) != 2 || got[0] != 4 || got[1] != 5 {
		t.Fatalf("second payload = %v", got)
	}
}

func TestDecodeWADRejectsTruncatedPayload(t *testing.T) {
	data := make([]byte, 2+wadEntrySize)
	binary.LittleEndian.PutUint16(data[:2], 1)
	copy(data[2:18], "bad.bin")
	binary.LittleEndian.PutUint32(data[18:22], 1)
	binary.LittleEndian.PutUint32(data[22:26], 1)
	if _, err := DecodeWAD(data); err == nil {
		t.Fatal("accepted truncated WAD payload")
	}
}

func TestDecodeWADCorpus(t *testing.T) {
	if _, err := os.Stat(wipeoutDiscPath); err != nil {
		t.Skipf("real disc data unavailable: %v", err)
	}

	var paths []string
	err := filepath.Walk(wipeoutDiscPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.EqualFold(filepath.Ext(path), ".WAD") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 11 {
		t.Fatalf("found %d WAD files, want 11", len(paths))
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		entries, err := DecodeWAD(data)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if len(entries) == 0 {
			t.Errorf("%s: empty archive", path)
		}
	}
}
