package assets

import (
	"fmt"

	"github.com/tridentsx/wipeout-go/internal/psx"
)

// Track-surface tiles are composed from a 4x4 grid of 32x32 sub-tiles, giving
// the 128x128 "near" (highest-detail) tile the driving surface samples.
const (
	trackTileGrid     = 4  // near tiles are a 4x4 grid of sub-tiles
	trackSubTileSize  = 32 // each LIBRARY sub-tile is 32x32
	trackNearTileSize = trackTileGrid * trackSubTileSize
)

// LoadTrackTiles composes a track's high-detail driving-surface tiles from
// LIBRARY.TTF (the tile layout table) and LIBRARY.CMP (32x32 sub-tiles). The
// returned slice is indexed by the logical tile index that TrackFace.Tile and
// TRACK.TEX reference: each entry is the 128x128 "near" tile, a 4x4 grid of the
// sub-tiles named by that TTF record's first 16 (near) values.
//
// This is the resolution-independent identity for the track surface: an upres'd
// tile keyed by the same index drops straight back in, and the per-face
// gameplay flags/triggers stay in TRACK.TRF/TRACK.TEX untouched.
func (l Loader) LoadTrackTiles(name string) ([]*psx.Image, error) {
	ttfData, err := l.read(name, "LIBRARY.TTF")
	if err != nil {
		return nil, err
	}
	records, err := psx.DecodeTTF(ttfData)
	if err != nil {
		return nil, fmt.Errorf("assets: %s LIBRARY.TTF: %w", name, err)
	}
	cmpData, err := l.read(name, "LIBRARY.CMP")
	if err != nil {
		return nil, err
	}
	members, err := psx.DecodeCMP(cmpData)
	if err != nil {
		return nil, fmt.Errorf("assets: %s LIBRARY.CMP: %w", name, err)
	}
	subtiles := make([]*psx.Image, len(members))
	for i, member := range members {
		if img, decodeErr := psx.DecodeTIM(member); decodeErr == nil {
			subtiles[i] = img
		}
	}
	tiles := make([]*psx.Image, len(records))
	for i := range records {
		tiles[i] = composeNearTile(records[i], subtiles)
	}
	return tiles, nil
}

// composeNearTile builds one 128x128 near tile from a TTF record's near[16]
// sub-tile indices (row-major 4x4), matching the retail loader's composition.
func composeNearTile(record psx.TTFRecord, subtiles []*psx.Image) *psx.Image {
	size := trackNearTileSize
	tile := &psx.Image{Width: size, Height: size, Pixels: make([]byte, size*size*4)}
	for ty := 0; ty < trackTileGrid; ty++ {
		for tx := 0; tx < trackTileGrid; tx++ {
			index := int(record.Values[ty*trackTileGrid+tx])
			if index < 0 || index >= len(subtiles) || subtiles[index] == nil {
				continue
			}
			blitTile(tile, subtiles[index], tx*trackSubTileSize, ty*trackSubTileSize)
		}
	}
	return tile
}

// blitTile copies src into dst at (dstX,dstY), clamped to a single sub-tile
// cell so an unexpected sub-tile size cannot bleed into neighbors.
func blitTile(dst, src *psx.Image, dstX, dstY int) {
	w, h := src.Width, src.Height
	if w > trackSubTileSize {
		w = trackSubTileSize
	}
	if h > trackSubTileSize {
		h = trackSubTileSize
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			di := ((dstY+y)*dst.Width + (dstX + x)) * 4
			si := (y*src.Width + x) * 4
			if di+4 > len(dst.Pixels) || si+4 > len(src.Pixels) {
				continue
			}
			copy(dst.Pixels[di:di+4], src.Pixels[si:si+4])
		}
	}
}

// LoadTextureSet decodes a CMP texture archive from the shared TEXTURES
// directory into its member images, in file order.
//
// Retail's animated scenery takes its frames from these files rather than from
// the track's own SCENE.CMP: maybe_RaceMain loads TEXTURES/SMOKE.CMP right after
// binding the "smokes"/"smokef" objects and TEXTURES/<set>RED.CMP right after
// binding "redb", then stores each load's base index into the global texture
// atlas for the animators to index from. The member counts corroborate the
// constants in the animator calls -- SMOKE.CMP holds exactly 25 frames against
// arg5 = 0x19, and each <set>RED.CMP holds exactly 2 against arg5 = 2.
func (l Loader) LoadTextureSet(name string) ([]*psx.Image, error) {
	data, err := l.read("TEXTURES", name)
	if err != nil {
		return nil, err
	}
	members, err := psx.DecodeCMP(data)
	if err != nil {
		return nil, fmt.Errorf("assets: %s: %w", name, err)
	}
	images := make([]*psx.Image, 0, len(members))
	for i, member := range members {
		image, err := psx.DecodeTIM(member)
		if err != nil {
			return nil, fmt.Errorf("assets: %s member %d: %w", name, i, err)
		}
		images = append(images, image)
	}
	return images, nil
}
