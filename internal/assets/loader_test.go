package assets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTrackCorpus(t *testing.T) {
	root := filepath.Join("..", "..", "assets", "WIPEOUT2")
	if _, err := os.Stat(root); err != nil {
		t.Skip(err)
	}
	track, err := (Loader{Root: root}).LoadTrack("TRACK01")
	if err != nil {
		t.Fatal(err)
	}
	if len(track.Vertices) == 0 || len(track.Faces) == 0 || len(track.Sections) == 0 || len(track.Visibility) != len(track.Sections) || len(track.Tiles) == 0 {
		t.Fatalf("incomplete track: %d vertices, %d faces, %d sections, %d visibility, %d tiles", len(track.Vertices), len(track.Faces), len(track.Sections), len(track.Visibility), len(track.Tiles))
	}
}
