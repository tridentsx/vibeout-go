// Package assets assembles decoded PlayStation files into resources consumed
// by game systems. Raw binary parsing remains isolated in internal/psx.
package assets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tridentsx/wipeout-go/internal/psx"
)

// Loader reads assets from an extracted WipEout 2097 disc tree.
type Loader struct{ Root string }

// Track is the renderer/physics-facing collection of one retail track's
// runtime assets. Warnings records missing optional scenery, sections, or
// textures; the driving surface itself is required.
type Track struct {
	Name         string
	Scenery      []psx.Object
	Sky          []psx.Object
	Vertices     []psx.TrackVertex
	Faces        []psx.TrackFace
	Sections     []psx.TrackSection
	Visibility   []psx.TrackVisibility
	Tiles        []*psx.Image
	SceneryTiles []*psx.Image
	SkyTiles     []*psx.Image
	Warnings     []error
}

func (l Loader) read(parts ...string) ([]byte, error) {
	path := filepath.Join(append([]string{l.Root}, parts...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("assets: read %s: %w", path, err)
	}
	return data, nil
}

// LoadTrack loads the specialized runtime products used by the PS1 game.
func (l Loader) LoadTrack(name string) (*Track, error) {
	track := &Track{Name: name}
	trv, err := l.read(name, "TRACK.TRV")
	if err != nil {
		return nil, err
	}
	track.Vertices, err = psx.DecodeTRV(trv)
	if err != nil {
		return nil, fmt.Errorf("assets: %s TRACK.TRV: %w", name, err)
	}
	trf, err := l.read(name, "TRACK.TRF")
	if err != nil {
		return nil, err
	}
	track.Faces, err = psx.DecodeTRF(trf)
	if err != nil {
		return nil, fmt.Errorf("assets: %s TRACK.TRF: %w", name, err)
	}

	if data, readErr := l.read(name, "TRACK.TRS"); readErr != nil {
		track.Warnings = append(track.Warnings, readErr)
	} else if track.Sections, err = psx.DecodeTRS(data); err != nil {
		track.Warnings = append(track.Warnings, fmt.Errorf("assets: %s TRACK.TRS: %w", name, err))
	}
	if len(track.Sections) != 0 {
		if data, readErr := l.read(name, "TRACK.VEW"); readErr != nil {
			track.Warnings = append(track.Warnings, readErr)
		} else if track.Visibility, err = psx.DecodeVEW(data, track.Sections); err != nil {
			track.Warnings = append(track.Warnings, fmt.Errorf("assets: %s TRACK.VEW: %w", name, err))
		}
	}
	if data, readErr := l.read(name, "SCENE.PRM"); readErr != nil {
		track.Warnings = append(track.Warnings, readErr)
	} else if track.Scenery, err = psx.DecodePRM(data); err != nil {
		track.Warnings = append(track.Warnings, fmt.Errorf("assets: %s SCENE.PRM: %w", name, err))
	}
	// The track floor's real texture source is LIBRARY.CMP (a bank of small
	// 32x32 tiles) plus LIBRARY.TTF, which for each TrackFace.Tile value
	// lists 16 tile indices (a 4x4 "near" grid) composing one 128x128
	// material -- confirmed against wipeout.js's own createTrack/.TTF
	// handling (Wipeout.TrackTextureIndex: near[16]/med[4]/far[1], and
	// `textures: LIBRARY.CMP, textureIndex: LIBRARY.TTF` in loadTrack).
	// TRACK.CMP is not part of this path at all -- it was a wrong-file bug,
	// not an intentional lower-detail source; wipeout.js itself always uses
	// the near tier regardless of distance, which a modern GPU can afford
	// unconditionally too, so med/far are intentionally not composed here.
	if libraryCMP, readErr := l.read(name, "LIBRARY.CMP"); readErr != nil {
		track.Warnings = append(track.Warnings, readErr)
	} else if libraryTTF, readErr := l.read(name, "LIBRARY.TTF"); readErr != nil {
		track.Warnings = append(track.Warnings, readErr)
	} else if members, decodeErr := psx.DecodeCMP(libraryCMP); decodeErr != nil {
		track.Warnings = append(track.Warnings, fmt.Errorf("assets: %s LIBRARY.CMP: %w", name, decodeErr))
	} else if records, decodeErr := psx.DecodeTTF(libraryTTF); decodeErr != nil {
		track.Warnings = append(track.Warnings, fmt.Errorf("assets: %s LIBRARY.TTF: %w", name, decodeErr))
	} else {
		tiles := make([]*psx.Image, len(members))
		for i, member := range members {
			if image, imageErr := psx.DecodeTIM(member); imageErr == nil {
				tiles[i] = image
			}
		}
		track.Tiles = make([]*psx.Image, len(records))
		for i, record := range records {
			track.Tiles[i] = composeNearTexture(tiles, record)
		}
	}
	if data, readErr := l.read(name, "SCENE.CMP"); readErr != nil {
		track.Warnings = append(track.Warnings, readErr)
	} else if members, decodeErr := psx.DecodeCMP(data); decodeErr != nil {
		track.Warnings = append(track.Warnings, fmt.Errorf("assets: %s SCENE.CMP: %w", name, decodeErr))
	} else {
		track.SceneryTiles = make([]*psx.Image, len(members))
		for i, member := range members {
			if image, imageErr := psx.DecodeTIM(member); imageErr == nil {
				track.SceneryTiles[i] = image
			}
		}
	}
	// SKY.PRM/SKY.CMP are the horizon backdrop, loaded independently of
	// SCENE.PRM/SCENE.CMP -- confirmed against wipeout.js's loadTrack, which
	// loads them as their own createScene({scale:48}) pass. Not drawing this
	// at all (as this project didn't until now) left a real gap between the
	// nearby track/scenery geometry and the far horizon: confirmed with a
	// magenta clear-color diagnostic that the gap showed the clear color
	// through, i.e. no geometry was submitted there, not just dark shading.
	if data, readErr := l.read(name, "SKY.PRM"); readErr != nil {
		track.Warnings = append(track.Warnings, readErr)
	} else if objects, decodeErr := psx.DecodePRM(data); decodeErr != nil {
		track.Warnings = append(track.Warnings, fmt.Errorf("assets: %s SKY.PRM: %w", name, decodeErr))
	} else {
		track.Sky = objects
	}
	if data, readErr := l.read(name, "SKY.CMP"); readErr != nil {
		track.Warnings = append(track.Warnings, readErr)
	} else if members, decodeErr := psx.DecodeCMP(data); decodeErr != nil {
		track.Warnings = append(track.Warnings, fmt.Errorf("assets: %s SKY.CMP: %w", name, decodeErr))
	} else {
		track.SkyTiles = make([]*psx.Image, len(members))
		for i, member := range members {
			if image, imageErr := psx.DecodeTIM(member); imageErr == nil {
				track.SkyTiles[i] = image
			}
		}
	}
	return track, nil
}

// composeNearTexture builds one 128x128 material image from a TTF record's
// 16 "near" tile indices (the record's first 16 values), arranged as a 4x4
// grid of 32x32 tiles: near[y*4+x] at pixel (x*32, y*32). Matches
// wipeout.js's createTrack composedImage loop exactly. Missing/out-of-range
// source tiles leave their 32x32 cell transparent black rather than failing
// the whole material, consistent with this codebase's partial-decode style.
func composeNearTexture(tiles []*psx.Image, record psx.TTFRecord) *psx.Image {
	const tileSize = 32
	const gridSize = 4
	const composedSize = tileSize * gridSize
	composed := &psx.Image{Width: composedSize, Height: composedSize, Pixels: make([]byte, composedSize*composedSize*4)}
	for y := 0; y < gridSize; y++ {
		for x := 0; x < gridSize; x++ {
			index := int(record.Values[y*gridSize+x])
			if index < 0 || index >= len(tiles) || tiles[index] == nil {
				continue
			}
			source := tiles[index]
			for sy := 0; sy < source.Height && sy < tileSize; sy++ {
				dstRow := (y*tileSize + sy) * composedSize * 4
				srcRow := sy * source.Width * 4
				dstOff := dstRow + x*tileSize*4
				width := source.Width
				if width > tileSize {
					width = tileSize
				}
				copy(composed.Pixels[dstOff:dstOff+width*4], source.Pixels[srcRow:srcRow+width*4])
			}
		}
	}
	return composed
}

func (l Loader) LoadVAG(wadName, sampleName string) (*psx.VAG, error) {
	data, err := l.read(wadName)
	if err != nil {
		return nil, err
	}
	wad, err := psx.DecodeWAD(data)
	if err != nil {
		return nil, err
	}
	for _, entry := range wad {
		if strings.EqualFold(entry.Name, sampleName) {
			return psx.DecodeVAG(entry.Data)
		}
	}
	return nil, fmt.Errorf("assets: %s not found in %s", sampleName, wadName)
}

func (l Loader) LoadAV(name string) (*psx.AV, error) {
	data, err := l.read(name)
	if err != nil {
		return nil, err
	}
	return psx.DecodeAV(data)
}

// LoadPRM loads a runtime object bundle relative to the disc root.
func (l Loader) LoadPRM(parts ...string) ([]psx.Object, error) {
	data, err := l.read(parts...)
	if err != nil {
		return nil, err
	}
	objects, err := psx.DecodePRM(data)
	if err != nil {
		return nil, fmt.Errorf("assets: decode %s: %w", filepath.Join(parts...), err)
	}
	return objects, nil
}

// LoadTIM decodes a standalone .TIM image from the disc tree. Used for the boot
// splashes and the menu font, which retail loads with LoadTIMTexture.
func (l Loader) LoadTIM(parts ...string) (*psx.Image, error) {
	data, err := l.read(parts...)
	if err != nil {
		return nil, err
	}
	img, err := psx.DecodeTIM(data)
	if err != nil {
		return nil, fmt.Errorf("assets: decoding TIM %v: %w", parts, err)
	}
	return img, nil
}
