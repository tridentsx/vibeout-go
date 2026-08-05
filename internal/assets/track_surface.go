package assets

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/tridentsx/wipeout-go/internal/psx"
)

// TrackCheckpoint is one CPOINT*.CHK file's per-record section indices.
type TrackCheckpoint struct {
	File     string
	Sections []int16
}

// TrackSurface is the driving surface's resolution-independent logic plus its
// composed tile textures. Faces keep their tile index and gameplay flags, so
// the surface is never baked away; a consumer builds the mesh and applies tiles
// by index at load. Vertices/faces are raw PSX values (world space); callers
// convert to their target coordinate space.
type TrackSurface struct {
	Vertices    []psx.TrackVertex
	Faces       []psx.TrackFace // TRACK.TEX overrides applied when present (WO2097)
	Sections    []psx.TrackSection
	Checkpoints []TrackCheckpoint
	Tiles       []*psx.Image // composed 128x128 near tiles, indexed by logical tile index
}

// LoadTrackSurface assembles a track's driving surface: TRV vertices, TRF faces
// (with TRACK.TEX per-face tile/flag overrides applied when present), TRS
// sections, CPOINT*.CHK checkpoints, and the composed LIBRARY tiles keyed by the
// logical tile index that TrackFace.Tile references.
func (l Loader) LoadTrackSurface(name string) (*TrackSurface, error) {
	surface := &TrackSurface{}

	trv, err := l.read(name, "TRACK.TRV")
	if err != nil {
		return nil, err
	}
	if surface.Vertices, err = psx.DecodeTRV(trv); err != nil {
		return nil, fmt.Errorf("assets: %s TRACK.TRV: %w", name, err)
	}

	trf, err := l.read(name, "TRACK.TRF")
	if err != nil {
		return nil, err
	}
	if surface.Faces, err = psx.DecodeTRF(trf); err != nil {
		return nil, fmt.Errorf("assets: %s TRACK.TRF: %w", name, err)
	}

	// WipEout 2097 stores authoritative per-face tile/flags in TRACK.TEX; the
	// retail loader copies the first face_count records over the TRF values.
	if texData, texErr := l.read(name, "TRACK.TEX"); texErr == nil {
		if tex, decodeErr := psx.DecodeTEX(texData); decodeErr == nil && tex.Kind == psx.TEXFaceAttributes {
			for i := range surface.Faces {
				if i < len(tex.FaceValues) {
					surface.Faces[i].Tile = tex.FaceValues[i].Tile
					surface.Faces[i].Flags = tex.FaceValues[i].Flags
				}
			}
		}
	}

	trs, err := l.read(name, "TRACK.TRS")
	if err != nil {
		return nil, err
	}
	if surface.Sections, err = psx.DecodeTRS(trs); err != nil {
		return nil, fmt.Errorf("assets: %s TRACK.TRS: %w", name, err)
	}

	if surface.Tiles, err = l.LoadTrackTiles(name); err != nil {
		return nil, err
	}

	if surface.Checkpoints, err = l.loadCheckpoints(name); err != nil {
		return nil, err
	}
	return surface, nil
}

// loadCheckpoints decodes every CPOINT*.CHK file in a track directory.
func (l Loader) loadCheckpoints(name string) ([]TrackCheckpoint, error) {
	paths, err := filepath.Glob(filepath.Join(l.Root, name, "CPOINT*.CHK"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var checkpoints []TrackCheckpoint
	for _, path := range paths {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		records, decodeErr := psx.DecodeCHK(data)
		if decodeErr != nil {
			continue
		}
		sections := make([]int16, len(records))
		for i, record := range records {
			sections[i] = record.Section
		}
		checkpoints = append(checkpoints, TrackCheckpoint{File: filepath.Base(path), Sections: sections})
	}
	return checkpoints, nil
}
