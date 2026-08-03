package psx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeCHK(t *testing.T) {
	data := make([]byte, CheckpointCount*checkpointRecordSize)
	data[0], data[1] = 0x34, 0x12
	copy(data[2:6], []byte{1, 2, 3, 4})
	data[6], data[7] = 0xff, 0xff

	checkpoints, err := DecodeCHK(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(checkpoints) != CheckpointCount {
		t.Fatalf("got %d checkpoints, want %d", len(checkpoints), CheckpointCount)
	}
	if checkpoints[0].Section != 0x1234 || checkpoints[0].Parameters != [4]byte{1, 2, 3, 4} {
		t.Fatalf("first checkpoint = %+v", checkpoints[0])
	}
	if checkpoints[1].Section != -1 {
		t.Fatalf("sentinel section = %d, want -1", checkpoints[1].Section)
	}
}

func TestDecodeCHKRejectsWrongSize(t *testing.T) {
	for _, size := range []int{0, CheckpointCount*checkpointRecordSize - 1, CheckpointCount*checkpointRecordSize + 1} {
		if _, err := DecodeCHK(make([]byte, size)); err == nil {
			t.Fatalf("accepted %d-byte CHK", size)
		}
	}
}

func TestDecodeCHKCorpus(t *testing.T) {
	if _, err := os.Stat(wipeoutDiscPath); err != nil {
		t.Skipf("real disc data unavailable: %v", err)
	}

	paths, err := filepath.Glob(filepath.Join(wipeoutDiscPath, "TRACK*", "CPOINT*.CHK"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 32 {
		t.Fatalf("found %d CHK files, want 32", len(paths))
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeCHK(data); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
}
