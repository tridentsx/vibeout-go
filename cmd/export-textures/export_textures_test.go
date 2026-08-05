package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExportTexturesFromDisc(t *testing.T) {
	disc := filepath.Join("..", "..", "assets", "WIPEOUT2")
	if _, err := os.Stat(disc); err != nil {
		t.Skip(err)
	}
	out := t.TempDir()

	total, unique, err := exportTextures(disc, out)
	if err != nil {
		t.Fatal(err)
	}
	if total == 0 || unique == 0 || unique > total {
		t.Fatalf("total=%d unique=%d (want 0 < unique <= total)", total, unique)
	}

	data, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []entry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("manifest parse: %v", err)
	}
	if len(entries) != total {
		t.Fatalf("manifest has %d entries, extracted %d", len(entries), total)
	}

	// The road's tile library must come out keyed by tile index.
	if _, err := os.Stat(filepath.Join(out, "TRACK01", "LIBRARY", "000.png")); err != nil {
		t.Errorf("expected road tile TRACK01/LIBRARY/000.png: %v", err)
	}

	// Every manifest PNG must exist and carry a sane identity.
	for _, e := range entries {
		if e.Width <= 0 || e.Height <= 0 || e.SHA1 == "" || e.Source == "" {
			t.Fatalf("bad manifest entry: %+v", e)
		}
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(e.File))); err != nil {
			t.Fatalf("missing PNG for %+v: %v", e, err)
		}
	}
}
