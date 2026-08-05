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

// TrackFace flag bits. The names originally came from wipeout.js's
// Wipeout.TrackFace.FLAGS; the meanings below are confirmed against SLES_003.27
// by scanning every read of the flags byte -- 84 sites -- and recording which
// mask each one tests. The split by subsystem is what makes them convincing:
// Flip is touched by nothing but the three face-drawing functions, Boost by
// nothing but ship physics and the AI, and the two weapon-pad bits only by the
// classifier that also registers pad triggers.
//
// Nothing is lost at load: decodeTrackFaces stores the whole byte, so these
// constants only name bits that were always present in TrackFace.Flags.
const (
	TrackFaceWall   = 0
	TrackFaceTrack  = 1 // section-run marker; every face scan is `while ((flags & 1) == 0)`
	TrackFaceWeapon = 2 // weapon pad; coloured (0xff,0x23,0x75) pink at load
	TrackFaceFlip   = 4 // flip texture horizontally; read only by the face drawers

	TrackFaceWeapon2 = 8 // second weapon-pad variant; coloured (0x54,0xee,0x75) green

	// TrackFaceUnused16 is tested at zero of the 84 flag-read sites in
	// SLES_003.27. It was previously called TrackFaceUnknown and exported to
	// track.json as "special", which reads as a meaning it does not have. Kept
	// under a name that says so, rather than removed, because the bit does occur
	// in .TRF data and dropping the constant would just hide it again.
	TrackFaceUnused16 = 16

	// TrackFaceUnknown is retained as a deprecated alias so existing callers keep
	// compiling. Prefer TrackFaceUnused16.
	TrackFaceUnknown = TrackFaceUnused16

	TrackFaceBoost = 32 // speed pad; coloured (0x23,0x23,0xff) blue

	// TrackFaceAlternateRoute marks a run of sections forming a branch of a track
	// fork. sub_8004c108 counts consecutive sections carrying it, called from
	// maybe_FindSectionByDifficulty (0x8004c1dc), and it is coloured (0x80,0,0)
	// dark red at load.
	//
	// This was briefly named TrackFaceStartGrid, on the strength of that
	// "difficulty"/section-run pairing. The track geometry disproves it: on
	// TRACK01 the flagged run is sections 290..319, and walking Previous back from
	// the start/finish line goes 5,4,3,2,1,0,288,287,286,285,284,283 -- section 0's
	// previous is 288, so the flagged run is not on the racing line at all. It sits
	// on the far side of the fork that leaves at sections 283/284 (nextJunction
	// 320) and is reachable from the junction at 257/258 (nextJunction 289).
	// Craft placed on it start on a bypass rather than on the grid.
	TrackFaceAlternateRoute = 64

	// TrackFaceStartGrid is retained as a deprecated alias for the bit, since
	// callers were written against that name. Prefer TrackFaceAlternateRoute.
	TrackFaceStartGrid = TrackFaceAlternateRoute

	// TrackFaceCheckpoint marks checkpoint faces. The load-time classifier records
	// each one's section index into 6-byte records, and maybe_FindShipCurrentSection
	// walks that same array for exactly six iterations -- matching the six section
	// slots per CPOINT*.CHK file this project already decodes. Coloured white.
	TrackFaceCheckpoint = 128
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
