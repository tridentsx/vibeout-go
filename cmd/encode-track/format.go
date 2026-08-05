package main

import (
	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/psx"
	"github.com/tridentsx/wipeout-go/internal/trackpack"
)

// buildSurface converts the decoded surface into the pack's logical form,
// mapping positions to glTF space (x,-y,-z) and decoding flag bytes.
func buildSurface(s *assets.TrackSurface) trackpack.Surface {
	out := trackpack.Surface{
		TileCount: len(s.Tiles),
		TileDir:   "surface/tiles",
		Vertices:  make([][3]int32, len(s.Vertices)),
		Faces:     make([]trackpack.Face, len(s.Faces)),
		Sections:  make([]trackpack.Section, len(s.Sections)),
	}
	for i, v := range s.Vertices {
		out.Vertices[i] = [3]int32{v.X, -v.Y, -v.Z}
	}
	for i := range s.Faces {
		out.Faces[i] = convertFace(&s.Faces[i])
	}
	for i := range s.Sections {
		out.Sections[i] = convertSection(&s.Sections[i])
	}
	for _, c := range s.Checkpoints {
		out.Checkpoints = append(out.Checkpoints, trackpack.Checkpoint{File: c.File, Sections: c.Sections})
	}
	return out
}

func convertFace(f *psx.TrackFace) trackpack.Face {
	return trackpack.Face{
		V:      f.Indices,
		Normal: [3]float64{float64(f.NormalX) / 4096, -float64(f.NormalY) / 4096, -float64(f.NormalZ) / 4096},
		Tile:   int(f.Tile),
		Color:  [3]uint8{uint8(f.Color >> 24), uint8(f.Color >> 16), uint8(f.Color >> 8)},
		Flip:   f.Flags&psx.TrackFaceFlip != 0,
		Flags: trackpack.FaceFlags{
			Raw:        f.Flags,
			Track:      f.Flags&psx.TrackFaceTrack != 0,
			WeaponPad:  f.Flags&psx.TrackFaceWeapon != 0,
			Flip:       f.Flags&psx.TrackFaceFlip != 0,
			WeaponPad2: f.Flags&psx.TrackFaceWeapon2 != 0,
			Unused16:   f.Flags&psx.TrackFaceUnused16 != 0,
			Boost:      f.Flags&psx.TrackFaceBoost != 0,
			StartGrid:  f.Flags&psx.TrackFaceStartGrid != 0,
			Checkpoint: f.Flags&psx.TrackFaceCheckpoint != 0,
		},
	}
}

func convertSection(s *psx.TrackSection) trackpack.Section {
	return trackpack.Section{
		Prev:         s.Previous,
		Next:         s.Next,
		NextJunction: s.NextJunction,
		Center:       [3]int32{s.X, -s.Y, -s.Z},
		FirstFace:    s.FirstFace,
		NumFaces:     s.NumFaces,
		Flags: trackpack.SectionFlags{
			Raw:           s.Flags,
			Jump:          s.Flags&psx.TrackSectionJump != 0,
			Junction:      s.Flags&psx.TrackSectionJunction != 0,
			JunctionStart: s.Flags&psx.TrackSectionJunctionStart != 0,
			JunctionEnd:   s.Flags&psx.TrackSectionJunctionEnd != 0,
		},
	}
}
