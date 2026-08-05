package assets

import (
	"testing"

	"github.com/tridentsx/wipeout-go/internal/psx"
)

func solidTile(r, g, b byte) *psx.Image {
	px := make([]byte, trackSubTileSize*trackSubTileSize*4)
	for i := 0; i < len(px); i += 4 {
		px[i], px[i+1], px[i+2], px[i+3] = r, g, b, 255
	}
	return &psx.Image{Width: trackSubTileSize, Height: trackSubTileSize, Pixels: px}
}

func pixelAt(img *psx.Image, x, y int) [4]byte {
	i := (y*img.Width + x) * 4
	return [4]byte{img.Pixels[i], img.Pixels[i+1], img.Pixels[i+2], img.Pixels[i+3]}
}

func TestComposeNearTilePlacesSubTilesInGrid(t *testing.T) {
	red := solidTile(255, 0, 0)
	green := solidTile(0, 255, 0)
	subtiles := []*psx.Image{red, green}

	var rec psx.TTFRecord
	rec.Values[0] = 0 // grid cell (tx0,ty0) -> red
	rec.Values[1] = 1 // grid cell (tx1,ty0) -> green
	// remaining near indices default to 0 -> red

	tile := composeNearTile(rec, subtiles)
	if tile.Width != trackNearTileSize || tile.Height != trackNearTileSize {
		t.Fatalf("tile size = %dx%d, want %d²", tile.Width, tile.Height, trackNearTileSize)
	}
	if got := pixelAt(tile, 0, 0); got != [4]byte{255, 0, 0, 255} {
		t.Fatalf("cell(0,0) = %v, want red", got)
	}
	if got := pixelAt(tile, trackSubTileSize, 0); got != [4]byte{0, 255, 0, 255} {
		t.Fatalf("cell(1,0) = %v, want green", got)
	}
	if got := pixelAt(tile, 0, trackSubTileSize); got != [4]byte{255, 0, 0, 255} {
		t.Fatalf("cell(0,1) = %v, want red (default index 0)", got)
	}
}

func TestLoadTrackTilesComposes128Tiles(t *testing.T) {
	tiles, err := Loader{Root: discRoot(t)}.LoadTrackTiles("TRACK01")
	if err != nil {
		t.Fatal(err)
	}
	if len(tiles) == 0 {
		t.Fatal("no track tiles composed")
	}
	for i, tile := range tiles {
		if tile == nil || tile.Width != trackNearTileSize || tile.Height != trackNearTileSize {
			t.Fatalf("tile %d = %+v, want %d² image", i, tile, trackNearTileSize)
		}
		if len(tile.Pixels) != trackNearTileSize*trackNearTileSize*4 {
			t.Fatalf("tile %d has %d pixel bytes", i, len(tile.Pixels))
		}
	}
}
