package psx

import (
	"encoding/binary"
	"fmt"
)

// Polygon types accepted by WipEout 2097's IntelPrim/LoadPrm dispatch table.
// Types 3 and 9 route to the loader's bad-primitive error. Types above 11
// are 2097 additions whose renderer semantics still need naming.
const (
	PolygonFlatTrisFaceColor       uint16 = 1
	PolygonTexturedTrisFaceColor   uint16 = 2
	PolygonFlatQuadFaceColor       uint16 = 3
	PolygonTexturedQuadFaceColor   uint16 = 4
	PolygonFlatTrisVertexColor     uint16 = 5
	PolygonTexturedTrisVertexColor uint16 = 6
	PolygonFlatQuadVertexColor     uint16 = 7
	PolygonTexturedQuadVertexColor uint16 = 8
	PolygonSpriteTopAnchor         uint16 = 10
	PolygonSpriteBottomAnchor      uint16 = 11
	PolygonType12                  uint16 = 12
	PolygonType13                  uint16 = 13
	PolygonType14                  uint16 = 14
	PolygonType15                  uint16 = 15
	PolygonType16                  uint16 = 16
	PolygonType17                  uint16 = 17
	PolygonType18                  uint16 = 18
	PolygonType19                  uint16 = 19
	PolygonType20                  uint16 = 20
	PolygonType21                  uint16 = 21
	PolygonType22                  uint16 = 22
	PolygonType23                  uint16 = 23
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
	NormalCount  uint16
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

	// Raw is the complete on-disc record. It preserves every field while
	// semantic names for the later 2097-only primitive types are recovered.
	Raw []byte
}

// Object is one parsed PRM 3D object (a ship, a track section's decoration,
// etc. -- a single PRM file is a flat sequence of these).
type Object struct {
	Header   ObjectHeader
	Vertices []Vertex
	Normals  []Vertex
	Polygons []Polygon
}

// DecodePRM parses every Object in a retail-runtime .PRM file.
//
// Record sizes and valid type IDs come directly from LoadPrm's switch at
// SLES_003.27 0x80026220. Three files retained in the extracted development
// tree (COMMON/SKY.PRM, COMMON/TRACK.PRM, and TRACK08/TRAK2.PRM) use an
// expanded editor/interchange representation. They are neither named by the
// executable nor present in a WAD directory and are deliberately not guessed
// at here; the track10 tool recorded in TRACK.INF converted source scenes into
// the specialized retail TRV/TRF/TRS/VEW and SCENE.PRM/SKY.PRM assets.
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
	nameLimit := 15
	for i := 0; i < nameLimit; i++ {
		b := data[offset+i]
		if b == 0 {
			break
		}
		name += string(rune(b))
	}

	header := ObjectHeader{
		Name:         name,
		VertexCount:  binary.BigEndian.Uint16(data[offset+16 : offset+18]),
		NormalCount:  binary.BigEndian.Uint16(data[offset+24 : offset+26]),
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
	normals := make([]Vertex, header.NormalCount)
	for i := range normals {
		vOff := offset + i*8
		if vOff+8 > len(data) {
			return Object{}, 0, fmt.Errorf("normal %d runs past end of file", i)
		}
		normals[i] = Vertex{
			X: int16(binary.BigEndian.Uint16(data[vOff : vOff+2])),
			Y: int16(binary.BigEndian.Uint16(data[vOff+2 : vOff+4])),
			Z: int16(binary.BigEndian.Uint16(data[vOff+4 : vOff+6])),
		}
	}
	offset += int(header.NormalCount) * 8

	polygons := make([]Polygon, header.PolygonCount)
	for i := range polygons {
		poly, size, err := readPolygon(data, offset)
		if err != nil {
			return Object{}, 0, fmt.Errorf("polygon %d: %w", i, err)
		}
		polygons[i] = poly
		offset += size
	}

	return Object{Header: header, Vertices: vertices, Normals: normals, Polygons: polygons}, offset - start, nil
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
	size, ok := prmPolygonSizes[polyType]
	if !ok {
		return Polygon{}, 0, fmt.Errorf("unknown polygon type 0x%x", polyType)
	}
	if offset+size > len(data) {
		return Polygon{}, 0, fmt.Errorf("polygon type 0x%x runs past end of file", polyType)
	}

	poly := Polygon{Type: polyType, Raw: append([]byte(nil), data[offset:offset+size]...)}
	// LoadPrm's fixup cases prove that types 1/3/4/6 use three vertex
	// indices and types 2/5 use four, all beginning immediately after the
	// four-byte {type,subtype} header.
	switch polyType {
	case PolygonFlatTrisFaceColor, PolygonTexturedTrisFaceColor,
		PolygonFlatTrisVertexColor, PolygonTexturedTrisVertexColor:
		poly.Indices = readIndices(data, offset+4, 3)
	case PolygonTexturedQuadFaceColor, PolygonFlatQuadVertexColor, PolygonTexturedQuadVertexColor:
		poly.Indices = readIndices(data, offset+4, 4)
	case PolygonFlatQuadFaceColor:
		poly.Indices = readIndices(data, offset+4, 4)
	}
	texture := func(value uint16) *uint16 { return &value }
	switch polyType {
	case PolygonTexturedTrisFaceColor, PolygonTexturedTrisVertexColor:
		poly.Texture = texture(binary.BigEndian.Uint16(data[offset+10 : offset+12]))
	case PolygonTexturedQuadFaceColor, PolygonTexturedQuadVertexColor, PolygonType13:
		poly.Texture = texture(binary.BigEndian.Uint16(data[offset+12 : offset+14]))
	case PolygonType15:
		poly.Texture = texture(binary.BigEndian.Uint16(data[offset+14 : offset+16]))
	case PolygonType17:
		poly.Texture = texture(binary.BigEndian.Uint16(data[offset+16 : offset+18]))
	case PolygonType19:
		poly.Texture = texture(binary.BigEndian.Uint16(data[offset+20 : offset+22]))
	}

	// Semantics for the original primitive family are independently known;
	// retain the convenient decoded fields in addition to Raw.
	body := offset + 4
	switch polyType {
	case PolygonFlatTrisFaceColor:
		color := readColor(data, body+8)
		poly.Color = &color
	case PolygonTexturedTrisFaceColor:
		poly.UV = readUVs(data, body+12, 3)
		color := readColor(data, body+20)
		poly.Color = &color
	case PolygonFlatQuadFaceColor:
		color := readColor(data, body+8)
		poly.Color = &color
	case PolygonTexturedQuadFaceColor:
		poly.UV = readUVs(data, body+14, 4)
		color := readColor(data, body+24)
		poly.Color = &color
	case PolygonFlatTrisVertexColor:
		poly.Colors = readColors(data, body+8, 3)
	case PolygonTexturedTrisVertexColor:
		poly.UV = readUVs(data, body+12, 3)
		poly.Colors = readColors(data, body+20, 3)
	case PolygonFlatQuadVertexColor:
		poly.Colors = readColors(data, body+8, 4)
	case PolygonTexturedQuadVertexColor:
		poly.UV = readUVs(data, body+14, 4)
		poly.Colors = readColors(data, body+24, 4)
	case PolygonSpriteTopAnchor, PolygonSpriteBottomAnchor:
		poly.SpriteIndex = binary.BigEndian.Uint16(data[body : body+2])
		poly.SpriteWidth = binary.BigEndian.Uint16(data[body+2 : body+4])
		poly.SpriteHeight = binary.BigEndian.Uint16(data[body+4 : body+6])
		tex := binary.BigEndian.Uint16(data[body+6 : body+8])
		poly.Texture = texture(tex)
		color := readColor(data, body+8)
		poly.Color = &color
	}
	return poly, size, nil
}

var prmPolygonSizes = map[uint16]int{
	1: 16, 2: 28, 3: 16, 4: 32, 5: 24, 6: 36, 7: 28, 8: 44,
	10: 16, 11: 16, 12: 16, 13: 28, 14: 20, 15: 32,
	16: 28, 17: 40, 18: 36, 19: 52, 20: 56, 21: 16,
	22: 28, 23: 40,
}
