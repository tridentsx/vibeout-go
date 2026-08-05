package assets

import "testing"

func TestLoadTrackSurface(t *testing.T) {
	surface, err := Loader{Root: discRoot(t)}.LoadTrackSurface("TRACK01")
	if err != nil {
		t.Fatal(err)
	}
	if len(surface.Vertices) == 0 || len(surface.Faces) == 0 || len(surface.Sections) == 0 || len(surface.Tiles) == 0 {
		t.Fatalf("incomplete surface: verts=%d faces=%d sections=%d tiles=%d",
			len(surface.Vertices), len(surface.Faces), len(surface.Sections), len(surface.Tiles))
	}
	for i, f := range surface.Faces {
		if int(f.Tile) >= len(surface.Tiles) {
			t.Fatalf("face %d references tile %d but only %d tiles composed", i, f.Tile, len(surface.Tiles))
		}
		for _, idx := range f.Indices {
			if int(idx) >= len(surface.Vertices) {
				t.Fatalf("face %d vertex index %d out of range (%d)", i, idx, len(surface.Vertices))
			}
		}
	}
	if len(surface.Checkpoints) == 0 {
		t.Error("no checkpoints loaded")
	}
}
