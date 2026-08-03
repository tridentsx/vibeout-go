package psx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeMenuDATCorpus(t *testing.T) {
	path := filepath.Join(wipeoutDiscPath, "COMMON", "MENU.DAT")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("disc image not present:", err)
	}
	menu, err := DecodeMenuDAT(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(menu.Retail) != 4212 || len(menu.Trailing) != 351 {
		t.Fatalf("got %d retail and %d trailing lines", len(menu.Retail), len(menu.Trailing))
	}
	first := menu.Retail[0]
	if first.StartX != 21 || first.StartY != 82 || first.EndX != 111 || first.EndY != 179 {
		t.Fatalf("unexpected first line: %+v", first)
	}
}

func TestDecodeMenuDATRejectsTruncatedData(t *testing.T) {
	if _, err := DecodeMenuDAT(make([]byte, retailMenuLineBytes-16)); err == nil {
		t.Fatal("expected truncated MENU.DAT error")
	}
}
