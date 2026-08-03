package psx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeTEXTextureList(t *testing.T) {
	tex, err := DecodeTEX([]byte("c:\\wipeout\\textures\\one.tim\r\nc:\\wipeout\\textures\\two.tim\r\n\r\n\x1a"))
	if err != nil {
		t.Fatal(err)
	}
	if tex.Kind != TEXTextureList || len(tex.Paths) != 2 || tex.Paths[1] != `c:\wipeout\textures\two.tim` {
		t.Fatalf("decoded TEX = %+v", tex)
	}
}

func TestDecodeTEXFaceAttributes(t *testing.T) {
	tex, err := DecodeTEX([]byte{7, TrackFaceTrack, 3, TrackFaceBoost})
	if err != nil {
		t.Fatal(err)
	}
	if tex.Kind != TEXFaceAttributes || len(tex.FaceValues) != 2 {
		t.Fatalf("decoded TEX = %+v", tex)
	}
	if got := tex.FaceValues[1]; got.Tile != 3 || got.Flags != TrackFaceBoost {
		t.Fatalf("second face value = %+v", got)
	}
}

func TestDecodeTEXCorpus(t *testing.T) {
	if _, err := os.Stat(wipeoutDiscPath); err != nil {
		t.Skipf("real disc data unavailable: %v", err)
	}

	var paths []string
	err := filepath.Walk(wipeoutDiscPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".TEX" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 34 {
		t.Fatalf("found %d TEX files, want 34", len(paths))
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeTEX(data); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
}
