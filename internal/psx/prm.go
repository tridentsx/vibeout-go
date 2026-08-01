package psx

import (
	"encoding/binary"
	"fmt"
)

// Polygon types, per wipeout.js's Wipeout.POLYGON_TYPE. UNKNOWN_00 and the
// sprite-anchor distinction are exactly as unresolved in the original
// project as they are here -- see wipeout/README.md's "Known Problems".
const (
	PolygonUnknown00               = 0x00
	PolygonFlatTrisFaceColor       = 0x01
	PolygonTexturedTrisFaceColor   = 0x02
	PolygonFlatQuadFaceColor       = 0x03
	PolygonTexturedQuadFaceColor   = 0x04
	PolygonFlatTrisVertexColor     = 0x05
	PolygonTexturedTrisVertexColor = 0x06
	PolygonFlatQuadVertexColor     = 0x07
	PolygonTexturedQuadVertexColor = 0x08
	PolygonSpriteTopAnchor         = 0x0A
	PolygonSpriteBottomAnchor      = 0x0B
)

// Vector3 is a PRM file's 32-bit-per-axis vector (object origin/position).
type Vector3 struct {
	X, Y, Z int32
}

// Vertex is a PRM file's model-space vertex, 16-bit per axis.
type Vertex struct {
	X, Y, Z int16
}

// UV is an 8-bit texture coordinate pair -- PS1 textures are at most 256x256,
// so a single byte per axis is exact, not a lossy quantization.
type UV struct {
	U, V uint8
}

// Color is a face or vertex color read straight from the file's raw R,G,B
// bytes (a 4th, unused pad byte follows each on disk). The PS1's GTE treats
// 0x80 (128), not 0xff (255), as "1.0x" when multiplying colors by these
// during lighting, so values above 128 are a legitimate over-bright boost --
// preserve that convention when converting to a float for shading rather
// than normalizing by 255.
type Color struct {
	R, G, B uint8
}

// ObjectHeader is a PRM object's 144-byte header. Ported from
// Wipeout.ObjectHeader; the `skip()` gaps in wipeout.js are fields this
// project (and the reference implementation before it) never needed to
// decode, not bytes that don't exist.
type ObjectHeader struct {
	Name         string
	VertexCount  uint16
	PolygonCount uint16
	Index1       uint16
	Origin       Vector3
	Position     Vector3
}

const objectHeaderSize = 144

// Polygon is one PRM face. Which fields are meaningful depends on Type:
// sprites (0x0A/0x0B) use Index/Width/Height/Texture/Color and leave
// Indices/UV/Colors empty; tris (3 indices) and quads (4 indices) use
// Indices/UV/Colors and leave the sprite fields zero. Exactly one of Color
// or Colors is set for a tri/quad (face-color vs. vertex-color variants);
// Texture is nil for the untextured variants.
type Polygon struct {
	Type    uint16
	Indices []uint16
	Texture *uint16
	UV      []UV
	Color   *Color
	Colors  []Color

	SpriteIndex  uint16
	SpriteWidth  uint16
	SpriteHeight uint16
}

// Object is one parsed PRM 3D object (a ship, a track section's decoration,
// etc. -- a single PRM file is a flat sequence of these).
type Object struct {
	Header   ObjectHeader
	Vertices []Vertex
	Polygons []Polygon
}

// DecodePRM parses every Object in a .PRM file's bytes. Ported from
// Wipeout.prototype.readObjects/readObject.
//
// Polygon type 0x00 is an open question this format has never had a
// confirmed answer for -- phoboslab's own wipeout.js README lists it as
// "possibly padding?", and empirically (checked against every real .PRM
// file on the WipEout 2097 disc: tens of thousands of polygons) its
// declared 18-byte size is only wrong in a handful of isolated spots, with
// every other polygon type (0x01-0x08, 0x0B) parsing correctly 100% of the
// time. Rather than discard an entire file over one ambiguous polygon deep
// inside it, DecodePRM returns every Object successfully parsed *before*
// the failure alongside a non-nil error, so a caller can use what's real
// instead of getting nothing.
func DecodePRM(data []byte) ([]Object, error) {
	var objects []Object
	offset := 0
	for offset < len(data) {
		obj, size, err := readObject(data, offset)
		if err != nil {
			return objects, fmt.Errorf("psx: PRM object at offset %d: %w", offset, err)
		}
		objects = append(objects, obj)
		offset += size
	}
	return objects, nil
}

func readObject(data []byte, offset int) (Object, int, error) {
	start := offset
	if offset+objectHeaderSize > len(data) {
		return Object{}, 0, fmt.Errorf("header runs past end of file")
	}

	name := ""
	for i := 0; i < 15; i++ {
		b := data[offset+i]
		if b == 0 {
			break
		}
		name += string(rune(b))
	}

	header := ObjectHeader{
		Name:         name,
		VertexCount:  binary.BigEndian.Uint16(data[offset+16 : offset+18]),
		PolygonCount: binary.BigEndian.Uint16(data[offset+32 : offset+34]),
		Index1:       binary.BigEndian.Uint16(data[offset+56 : offset+58]),
		Origin: Vector3{
			X: int32(binary.BigEndian.Uint32(data[offset+84 : offset+88])),
			Y: int32(binary.BigEndian.Uint32(data[offset+88 : offset+92])),
			Z: int32(binary.BigEndian.Uint32(data[offset+92 : offset+96])),
		},
		Position: Vector3{
			X: int32(binary.BigEndian.Uint32(data[offset+116 : offset+120])),
			Y: int32(binary.BigEndian.Uint32(data[offset+120 : offset+124])),
			Z: int32(binary.BigEndian.Uint32(data[offset+124 : offset+128])),
		},
	}
	offset += objectHeaderSize

	vertices := make([]Vertex, header.VertexCount)
	for i := range vertices {
		vOff := offset + i*8
		if vOff+8 > len(data) {
			return Object{}, 0, fmt.Errorf("vertex %d runs past end of file", i)
		}
		vertices[i] = Vertex{
			X: int16(binary.BigEndian.Uint16(data[vOff : vOff+2])),
			Y: int16(binary.BigEndian.Uint16(data[vOff+2 : vOff+4])),
			Z: int16(binary.BigEndian.Uint16(data[vOff+4 : vOff+6])),
			// vOff+6:vOff+8 is padding, matching Wipeout.Vertex.
		}
	}
	offset += int(header.VertexCount) * 8

	polygons := make([]Polygon, header.PolygonCount)
	for i := range polygons {
		poly, size, err := readPolygon(data, offset)
		if err != nil {
			return Object{}, 0, fmt.Errorf("polygon %d: %w", i, err)
		}
		polygons[i] = poly
		offset += size
	}

	return Object{Header: header, Vertices: vertices, Polygons: polygons}, offset - start, nil
}

// readIndices reads n big-endian uint16 vertex indices starting at offset.
func readIndices(data []byte, offset, n int) []uint16 {
	indices := make([]uint16, n)
	for i := 0; i < n; i++ {
		indices[i] = binary.BigEndian.Uint16(data[offset+i*2 : offset+i*2+2])
	}
	return indices
}

// readUVs reads n UV pairs (1 byte per axis, no endian concern) starting at offset.
func readUVs(data []byte, offset, n int) []UV {
	uvs := make([]UV, n)
	for i := 0; i < n; i++ {
		uvs[i] = UV{U: data[offset+i*2], V: data[offset+i*2+1]}
	}
	return uvs
}

// readColor reads a single R,G,B color (the 4th on-disk byte is padding).
func readColor(data []byte, offset int) Color {
	return Color{R: data[offset], G: data[offset+1], B: data[offset+2]}
}

// readColors reads n R,G,B colors, each stored as a padded 4-byte field.
func readColors(data []byte, offset, n int) []Color {
	colors := make([]Color, n)
	for i := 0; i < n; i++ {
		colors[i] = readColor(data, offset+i*4)
	}
	return colors
}

// readPolygon reads one polygon starting at offset, dispatching on its own
// 4-byte header's type field the same way wipeout.js peeks PolygonHeader
// before picking which full struct to decode. Returns the polygon and its
// total byte length (header included).
func readPolygon(data []byte, offset int) (Polygon, int, error) {
	if offset+4 > len(data) {
		return Polygon{}, 0, fmt.Errorf("polygon header runs past end of file")
	}
	polyType := binary.BigEndian.Uint16(data[offset : offset+2])
	body := offset + 4 // past PolygonHeader{type, subtype}

	texture := func(v uint16) *uint16 { return &v }

	switch polyType {
	case PolygonUnknown00:
		// header(4) + uint16[7] = 18 bytes total; contents unused.
		return Polygon{Type: polyType}, 18, nil

	case PolygonFlatTrisFaceColor:
		indices := readIndices(data, body, 3)
		color := readColor(data, body+6+2)
		return Polygon{Type: polyType, Indices: indices, Color: &color}, 16, nil

	case PolygonTexturedTrisFaceColor:
		indices := readIndices(data, body, 3)
		tex := binary.BigEndian.Uint16(data[body+6 : body+8])
		uv := readUVs(data, body+6+2+4, 3)
		color := readColor(data, body+6+2+4+6+2)
		return Polygon{Type: polyType, Indices: indices, Texture: texture(tex), UV: uv, Color: &color}, 28, nil

	case PolygonFlatQuadFaceColor:
		indices := readIndices(data, body, 4)
		color := readColor(data, body+8)
		return Polygon{Type: polyType, Indices: indices, Color: &color}, 16, nil

	case PolygonTexturedQuadFaceColor:
		indices := readIndices(data, body, 4)
		tex := binary.BigEndian.Uint16(data[body+8 : body+10])
		uv := readUVs(data, body+8+2+4, 4)
		color := readColor(data, body+8+2+4+8+2)
		return Polygon{Type: polyType, Indices: indices, Texture: texture(tex), UV: uv, Color: &color}, 32, nil

	case PolygonFlatTrisVertexColor:
		indices := readIndices(data, body, 3)
		colors := readColors(data, body+6+2, 3)
		return Polygon{Type: polyType, Indices: indices, Colors: colors}, 24, nil

	case PolygonTexturedTrisVertexColor:
		indices := readIndices(data, body, 3)
		tex := binary.BigEndian.Uint16(data[body+6 : body+8])
		uv := readUVs(data, body+6+2+4, 3)
		colors := readColors(data, body+6+2+4+6+2, 3)
		return Polygon{Type: polyType, Indices: indices, Texture: texture(tex), UV: uv, Colors: colors}, 36, nil

	case PolygonFlatQuadVertexColor:
		indices := readIndices(data, body, 4)
		colors := readColors(data, body+8, 4)
		return Polygon{Type: polyType, Indices: indices, Colors: colors}, 28, nil

	case PolygonTexturedQuadVertexColor:
		indices := readIndices(data, body, 4)
		tex := binary.BigEndian.Uint16(data[body+8 : body+10])
		uv := readUVs(data, body+8+2+4, 4)
		colors := readColors(data, body+8+2+4+8+2, 4)
		return Polygon{Type: polyType, Indices: indices, Texture: texture(tex), UV: uv, Colors: colors}, 44, nil

	case PolygonSpriteTopAnchor, PolygonSpriteBottomAnchor:
		index := binary.BigEndian.Uint16(data[body : body+2])
		width := binary.BigEndian.Uint16(data[body+2 : body+4])
		height := binary.BigEndian.Uint16(data[body+4 : body+6])
		tex := binary.BigEndian.Uint16(data[body+6 : body+8])
		color := readColor(data, body+8)
		return Polygon{
			Type: polyType, Texture: texture(tex), Color: &color,
			SpriteIndex: index, SpriteWidth: width, SpriteHeight: height,
		}, 16, nil

	default:
		return Polygon{}, 0, fmt.Errorf("unknown polygon type 0x%x", polyType)
	}
}
