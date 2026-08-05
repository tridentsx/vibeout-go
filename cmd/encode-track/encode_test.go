package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qmuntal/gltf"
	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/trackpack"
)

// TestEncodeTrackProducesPack encodes a real track and reads it back through
// the shared renderer-facing API (trackpack.Load), exercising tile loading,
// layer resolution, and surface triangulation.
func TestEncodeTrackProducesPack(t *testing.T) {
	disc := filepath.Join("..", "..", "assets", "WIPEOUT2")
	if _, err := os.Stat(disc); err != nil {
		t.Skip(err)
	}
	out := t.TempDir()
	if err := encodeTrack(assets.Loader{Root: disc}, "TRACK01", out); err != nil {
		t.Fatal(err)
	}

	p, err := trackpack.Load(filepath.Join(out, "TRACK01.trackpack"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "TRACK01" || p.FormatVersion != 1 {
		t.Fatalf("bad header: %+v", p)
	}
	if len(p.Surface.Vertices) == 0 || len(p.Surface.Faces) == 0 || len(p.Surface.Sections) == 0 || p.Surface.TileCount == 0 {
		t.Fatalf("incomplete surface: verts=%d faces=%d sections=%d tiles=%d",
			len(p.Surface.Vertices), len(p.Surface.Faces), len(p.Surface.Sections), p.Surface.TileCount)
	}

	// Faces: tile in range, flags preserved, triangulation references valid verts.
	driveable := false
	for _, f := range p.Surface.Faces {
		if f.Tile >= p.Surface.TileCount {
			t.Fatalf("face tile %d >= tileCount %d", f.Tile, p.Surface.TileCount)
		}
		if f.Flags.Track {
			driveable = true
		}
		for _, tri := range f.Triangles() {
			for _, corner := range tri {
				if int(corner.Index) >= len(p.Surface.Vertices) {
					t.Fatalf("triangulated vertex %d out of range (%d)", corner.Index, len(p.Surface.Vertices))
				}
			}
		}
	}
	if !driveable {
		t.Error("no drivable faces decoded (flags lost?)")
	}

	// Every surface tile decodes through the API.
	for i := 0; i < p.Surface.TileCount; i++ {
		if _, err := p.LoadTile(i); err != nil {
			t.Fatalf("LoadTile(%d): %v", i, err)
		}
	}

	// Baked layers resolve and re-open as glTF.
	for _, layer := range []func() (string, bool){p.SceneryPath, p.SkyPath} {
		path, ok := layer()
		if !ok {
			t.Fatal("missing baked layer path")
		}
		if _, err := gltf.Open(path); err != nil {
			t.Errorf("re-open %s: %v", path, err)
		}
	}
}
