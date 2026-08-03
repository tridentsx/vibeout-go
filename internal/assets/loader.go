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
	Name     string
	Scenery  []psx.Object
	Vertices []psx.TrackVertex
	Faces    []psx.TrackFace
	Sections []psx.TrackSection
	Tiles    []*psx.Image
	Warnings []error
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
	if data, readErr := l.read(name, "SCENE.PRM"); readErr != nil {
		track.Warnings = append(track.Warnings, readErr)
	} else if track.Scenery, err = psx.DecodePRM(data); err != nil {
		track.Warnings = append(track.Warnings, fmt.Errorf("assets: %s SCENE.PRM: %w", name, err))
	}
	if data, readErr := l.read(name, "TRACK.CMP"); readErr != nil {
		track.Warnings = append(track.Warnings, readErr)
	} else if members, decodeErr := psx.DecodeCMP(data); decodeErr != nil {
		track.Warnings = append(track.Warnings, fmt.Errorf("assets: %s TRACK.CMP: %w", name, decodeErr))
	} else {
		track.Tiles = make([]*psx.Image, len(members))
		for i, member := range members {
			if image, imageErr := psx.DecodeTIM(member); imageErr == nil {
				track.Tiles[i] = image
			}
		}
	}
	return track, nil
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
