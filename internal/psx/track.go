package psx

import (
	"encoding/binary"
	"fmt"
)

// TrackVertex is one .TRV record -- the real driving-surface mesh's vertex
// positions, distinct from a .PRM object's model-space Vertex (these are
// already in track/world space, no per-object origin to add). Ported from
// wipeout.js's Wipeout.TrackVertex; big-endian like every other real
// WipEout 2097 asset struct field (verified against TRACK01/TRACK.TRV: the
// first record's big-endian reading gives plausible track-scale
// coordinates, the little-endian one doesn't).
type TrackVertex struct {
	X, Y, Z int32
}

const trackVertexSize = 16 // X,Y,Z,padding, 4 bytes each

// DecodeTRV parses every TrackVertex in a .TRV file's bytes.
func DecodeTRV(data []byte) ([]TrackVertex, error) {
	if len(data)%trackVertexSize != 0 {
		return nil, fmt.Errorf("psx: TRV file length %d not a multiple of %d", len(data), trackVertexSize)
	}
	vertices := make([]TrackVertex, len(data)/trackVertexSize)
	for i := range vertices {
		off := i * trackVertexSize
		vertices[i] = TrackVertex{
			X: int32(binary.BigEndian.Uint32(data[off : off+4])),
			Y: int32(binary.BigEndian.Uint32(data[off+4 : off+8])),
			Z: int32(binary.BigEndian.Uint32(data[off+8 : off+12])),
			// off+12:off+16 is padding, matching Wipeout.TrackVertex.
		}
	}
	return vertices, nil
}

// TrackFace flag bits, ported from wipeout.js's Wipeout.TrackFace.FLAGS.
const (
	TrackFaceWall    = 0
	TrackFaceTrack   = 1
	TrackFaceWeapon  = 2
	TrackFaceFlip    = 4
	TrackFaceWeapon2 = 8
	TrackFaceUnknown = 16
	TrackFaceBoost   = 32
)

// TrackFace is one .TRF record: a quad into TrackVertex (Indices[3] repeats
// Indices[2] for a triangle, matching the original's fixed-4-index layout),
// its face normal, a texture tile index, gameplay flags (see TrackFace*
// constants), and a fallback flat color.
type TrackFace struct {
	Indices                   [4]uint16
	NormalX, NormalY, NormalZ int16
	Tile                      uint8
	Flags                     uint8
	Color                     uint32
}

const trackFaceSize = 20

// DecodeTRF parses every TrackFace in a .TRF file's bytes.
func DecodeTRF(data []byte) ([]TrackFace, error) {
	if len(data)%trackFaceSize != 0 {
		return nil, fmt.Errorf("psx: TRF file length %d not a multiple of %d", len(data), trackFaceSize)
	}
	faces := make([]TrackFace, len(data)/trackFaceSize)
	for i := range faces {
		off := i * trackFaceSize
		faces[i] = TrackFace{
			Indices: [4]uint16{
				binary.BigEndian.Uint16(data[off : off+2]),
				binary.BigEndian.Uint16(data[off+2 : off+4]),
				binary.BigEndian.Uint16(data[off+4 : off+6]),
				binary.BigEndian.Uint16(data[off+6 : off+8]),
			},
			NormalX: int16(binary.BigEndian.Uint16(data[off+8 : off+10])),
			NormalY: int16(binary.BigEndian.Uint16(data[off+10 : off+12])),
			NormalZ: int16(binary.BigEndian.Uint16(data[off+12 : off+14])),
			Tile:    data[off+14],
			Flags:   data[off+15],
			Color:   binary.BigEndian.Uint32(data[off+16 : off+20]),
		}
	}
	return faces, nil
}

// TrackSection flag bits, ported from wipeout.js's Wipeout.TrackSection.FLAGS.
const (
	TrackSectionJump          = 1
	TrackSectionJunctionEnd   = 8
	TrackSectionJunctionStart = 16
	TrackSectionJunction      = 32
)

// TrackSection is one .TRS record: a node in the track's section graph
// (Previous/Next link neighboring sections; NextJunction branches off for
// track forks) plus its center position and which FirstFace/NumFaces range
// of the .TRF face list belongs to it. This is the same "track section"
// concept independently confirmed throughout bn-psx's ship-physics RE work
// (e.g. maybe_AssignTrackSectionIndices writing [ship+0x98], maybe_RunShipAutopilot's
// waypoint-following) -- real disc data validating the binary analysis, not
// just a new asset format.
type TrackSection struct {
	NextJunction, Previous, Next int32
	X, Y, Z                      int32
	// ViewCounts are fifteen list lengths at offsets 0x60..0x7a, arranged
	// as three lanes of five lists. InitViewList uses them to partition
	// TRACK.VEW and attach fifteen visibility-list pointers per section.
	ViewCounts [3][5]uint16
	FirstFace  uint32
	NumFaces   uint16
	// CollisionFlags is the full word at runtime TrackSection+0x94. Wall
	// collision dispatch tests bits 0x180000; the low half is also exposed
	// as Flags for the older section-type constants below.
	CollisionFlags uint32
	Flags          uint16
}

const trackSectionSize = 156

// DecodeTRS parses every TrackSection in a .TRS file's bytes.
func DecodeTRS(data []byte) ([]TrackSection, error) {
	if len(data)%trackSectionSize != 0 {
		return nil, fmt.Errorf("psx: TRS file length %d not a multiple of %d", len(data), trackSectionSize)
	}
	sections := make([]TrackSection, len(data)/trackSectionSize)
	for i := range sections {
		off := i * trackSectionSize
		sections[i] = TrackSection{
			NextJunction: int32(binary.BigEndian.Uint32(data[off : off+4])),
			Previous:     int32(binary.BigEndian.Uint32(data[off+4 : off+8])),
			Next:         int32(binary.BigEndian.Uint32(data[off+8 : off+12])),
			X:            int32(binary.BigEndian.Uint32(data[off+12 : off+16])),
			Y:            int32(binary.BigEndian.Uint32(data[off+16 : off+20])),
			Z:            int32(binary.BigEndian.Uint32(data[off+20 : off+24])),
			FirstFace:    binary.BigEndian.Uint32(data[off+140 : off+144]),
			NumFaces:     binary.BigEndian.Uint16(data[off+144 : off+146]),
			// off+146:off+150 (4 bytes) skipped.
			CollisionFlags: binary.BigEndian.Uint32(data[off+148 : off+152]),
			Flags:          binary.BigEndian.Uint16(data[off+150 : off+152]),
			// off+152:off+156 (4 bytes) skipped.
		}
		for lane := range sections[i].ViewCounts {
			for group := range sections[i].ViewCounts[lane] {
				countOffset := off + 96 + group*6 + lane*2
				sections[i].ViewCounts[lane][group] = binary.BigEndian.Uint16(data[countOffset : countOffset+2])
			}
		}
	}
	return sections, nil
}
