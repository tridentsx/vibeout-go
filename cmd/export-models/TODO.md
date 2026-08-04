Go handles:

PRM parser → CMP decompressor → TIM decoder → mesh conversion → GLB writer

The JS viewer already documents the PRM header, vertex and polygon layouts, including the different triangle, quad, textured, coloured and sprite polygon records. It also establishes the coordinate conversion and triangle winding currently used to render the models.

Recommended Go library

Use:

go get github.com/qmuntal/gltf

qmuntal/gltf supports binary .glb output, embedded images, accessors, materials and extensions. Its modeler package writes positions, UVs, colours, indices and PNG images into the GLB buffer.



I would use explicit little-endian reader functions rather than rely heavily on binary.Read and padded structs:

type Reader struct {
	data []byte
	pos  int
}

func (r *Reader) Uint8() uint8 {
	v := r.data[r.pos]
	r.pos++
	return v
}

func (r *Reader) Uint16() uint16 {
	v := binary.LittleEndian.Uint16(r.data[r.pos:])
	r.pos += 2
	return v
}

func (r *Reader) Int16() int16 {
	return int16(r.Uint16())
}

func (r *Reader) Uint32() uint32 {
	v := binary.LittleEndian.Uint32(r.data[r.pos:])
	r.pos += 4
	return v
}

func (r *Reader) Int32() int32 {
	return int32(r.Uint32())
}

func (r *Reader) Skip(n int) {
	r.pos += n
}

That makes the undocumented padding in the object header much easier to reproduce and debug.

The parser would read:

type Vertex struct {
	X int16
	Y int16
	Z int16
}

type UV struct {
	U uint8
	V uint8
}

type Object struct {
	Name     string
	Position [3]int32
	Vertices []Vertex
	Polygons []Polygon
}

Each polygon should be represented as one normalized Go type after parsing:

type Polygon struct {
	Type    uint16
	Indices []uint16
	Texture *uint16
	UV      []UV
	Colors  []uint32
	Sprite  *Sprite
}
Vertex duplication is essential

PRM stores UV and colour values at the polygon corner level, while glTF stores these as vertex attributes. Therefore, the same PRM vertex must sometimes become several GLB vertices.

Use a key like:

type CornerKey struct {
	SourceVertex uint16
	U            uint8
	V            uint8
	Color        uint32
}

For each material primitive:

vertexMap := make(map[CornerKey]uint32)

func addCorner(
	key CornerKey,
	source []Vertex,
	positions *[][3]float32,
	uvs *[][2]float32,
	colors *[][4]uint8,
) uint32 {
	if index, ok := vertexMap[key]; ok {
		return index
	}

	v := source[key.SourceVertex]

	index := uint32(len(*positions))

	// Matches the conversion used by the existing viewer.
	*positions = append(*positions, [3]float32{
		float32(v.X),
		-float32(v.Y),
		-float32(v.Z),
	})

	*uvs = append(*uvs, [2]float32{
		float32(key.U),
		float32(key.V),
	})

	*colors = append(*colors, decodeColor(key.Color))
	vertexMap[key] = index

	return index
}

UV normalization should happen using the dimensions of the relevant decoded TIM image:

u := float32(rawU) / float32(textureWidth)
v := 1.0 - float32(rawV)/float32(textureHeight)

That matches the viewer’s current behaviour.

GLB primitive creation

The core GLB-writing portion can be quite small:

func makePrimitive(
	doc *gltf.Document,
	positions [][3]float32,
	uvs [][2]float32,
	colors [][4]uint8,
	indices []uint32,
	material int,
) *gltf.Primitive {
	attributes := gltf.PrimitiveAttributes{
		gltf.POSITION:   modeler.WritePosition(doc, positions),
		gltf.TEXCOORD_0: modeler.WriteTextureCoord(doc, uvs),
		gltf.COLOR_0:    modeler.WriteColor(doc, colors),
	}

	return &gltf.Primitive{
		Mode:       gltf.PrimitiveTriangles,
		Indices:    gltf.Index(modeler.WriteIndices(doc, indices)),
		Material:   gltf.Index(material),
		Attributes: attributes,
	}
}

The Go library provides these specific accessor-writing helpers and SaveBinary for GLB output.

Embedded PNG textures and unlit materials

Decode each TIM into image.RGBA, encode it with Go’s image/png, and embed it:

func addTexture(
	doc *gltf.Document,
	name string,
	img image.Image,
) (int, error) {
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, img); err != nil {
		return 0, err
	}

	imageIndex, err := modeler.WriteImage(
		doc,
		name,
		"image/png",
		&pngData,
	)
	if err != nil {
		return 0, err
	}

	samplerIndex := len(doc.Samplers)
	doc.Samplers = append(doc.Samplers, &gltf.Sampler{
		MagFilter: gltf.MagNearest,
		MinFilter: gltf.MinNearest,
		WrapS:     gltf.WrapClampToEdge,
		WrapT:     gltf.WrapClampToEdge,
	})

	textureIndex := len(doc.Textures)
	doc.Textures = append(doc.Textures, &gltf.Texture{
		Name:    name,
		Source:  gltf.Index(imageIndex),
		Sampler: gltf.Index(samplerIndex),
	})

	return textureIndex, nil
}

The original viewer uses nearest-neighbour filtering, alpha cutoff and unlit MeshBasicMaterial, so KHR_materials_unlit is the closest GLB equivalent. The Go library has built-in support for this extension.

material := &gltf.Material{
	Name: "ship-texture-0",
	Extensions: gltf.Extensions{
		unlit.ExtensionName: unlit.Unlit{},
	},
	PBRMetallicRoughness: &gltf.PBRMetallicRoughness{
		BaseColorTexture: &gltf.TextureInfo{Index: textureIndex},
		MetallicFactor:   gltf.Float(0),
		RoughnessFactor:  gltf.Float(1),
	},
	AlphaMode:   gltf.AlphaMask,
	AlphaCutoff: gltf.Float(0.5),
	DoubleSided: true,
}

doc.ExtensionsUsed = append(doc.ExtensionsUsed, unlit.ExtensionName)

I would initially enable DoubleSided because the repository notes that some Wipeout 2097/XL PRM faces appear backwards. Later, an option such as --fix-winding could address affected polygons more selectively.

Implementation order

The safest sequence would be:

Parse PRM and print object names/counts.
Export untextured geometry to GLB.
Implement CMP decompression.
Decode TIM files to standalone PNGs.
Add texture-based GLB primitives.
Add vertex colours.
Add sprites such as engine glows.
Export one GLB per ship.

This is a standalone plan there is a lot of code already for parsing these formats, please study existing code and redo this plan including any refactoring and repo structure changes needed

