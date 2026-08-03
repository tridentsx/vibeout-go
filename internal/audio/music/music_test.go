package music

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLibraryTracksAreSorted(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"track10.flac", "track02.flac", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	tracks, err := (Library{Root: root}).Tracks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 2 || tracks[0].Name != "track02.flac" || tracks[1].Name != "track10.flac" {
		t.Fatalf("unexpected tracks: %#v", tracks)
	}
}
