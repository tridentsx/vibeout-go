// Package glb writes a neutral mesh (internal/model) plus its decoded texture
// pages to a binary glTF (.glb) file. It is the only package that imports the
// qmuntal/gltf library.
//
// Materials are unlit (KHR_materials_unlit), matching the PS1's unlit,
// color-modulated, nearest-filtered look: one material per texture page
// (embedded PNG, alpha-mask cutoff for the color-keyed transparency, nearest
// filtering, clamped wrapping) plus one untextured material driven by the
// per-vertex COLOR_0 attribute. Each PRM object becomes one glTF mesh and node,
// the node translated by the object's world Position.
package glb

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"

	"github.com/qmuntal/gltf"
	"github.com/qmuntal/gltf/ext/unlit"
	"github.com/qmuntal/gltf/modeler"
	"github.com/tridentsx/wipeout-go/internal/model"
	"github.com/tridentsx/wipeout-go/internal/psx"
)

// Options controls material generation.
type Options struct {
	// DoubleSided disables back-face culling; on by default because some
	// WipEout 2097 faces are authored back-facing.
	DoubleSided bool
	// AlphaCutoff is the alpha-mask threshold for color-keyed transparency.
	AlphaCutoff float32
}

// DefaultOptions returns the highest-fidelity defaults.
func DefaultOptions() Options { return Options{DoubleSided: true, AlphaCutoff: 0.5} }

// BuildDocument converts a neutral mesh and its texture pages into a glTF
// document with embedded PNG textures and unlit materials. pages is indexed by
// Primitive.Page; a nil or invalid page falls back to the untextured material.
func BuildDocument(name string, mesh *model.Mesh, pages []*psx.Image, opts Options) (*gltf.Document, error) {
	doc := gltf.NewDocument()
	doc.Asset.Generator = "wipeout-go export-models"
	useExtension(doc, unlit.ExtensionName)

	sceneIndex := 0
	if doc.Scene != nil {
		sceneIndex = *doc.Scene
	}
	if sceneIndex >= len(doc.Scenes) {
		doc.Scenes = append(doc.Scenes, &gltf.Scene{})
		sceneIndex = len(doc.Scenes) - 1
		doc.Scene = gltf.Index(sceneIndex)
	}

	untextured := addUntexturedMaterial(doc, name, opts)

	var sampler *int
	materialForPage := make([]int, len(pages))
	for i, page := range pages {
		if page == nil || page.Width <= 0 || page.Height <= 0 {
			materialForPage[i] = untextured
			continue
		}
		if sampler == nil {
			sampler = gltf.Index(addSampler(doc))
		}
		mat, err := addTexturedMaterial(doc, fmt.Sprintf("%s_page%02d", name, i), page, sampler, opts)
		if err != nil {
			return nil, err
		}
		materialForPage[i] = mat
	}

	for _, obj := range mesh.Objects {
		gmesh := &gltf.Mesh{Name: obj.Name}
		for _, prim := range obj.Primitives {
			if len(prim.Indices) == 0 || len(prim.Positions) == 0 {
				continue
			}
			material := untextured
			if prim.Textured() && prim.Page >= 0 && prim.Page < len(materialForPage) {
				material = materialForPage[prim.Page]
			}
			attrs := gltf.PrimitiveAttributes{
				gltf.POSITION: modeler.WritePosition(doc, prim.Positions),
				gltf.COLOR_0:  modeler.WriteColor(doc, prim.Colors),
			}
			if prim.Textured() && len(prim.UVs) == len(prim.Positions) {
				attrs[gltf.TEXCOORD_0] = modeler.WriteTextureCoord(doc, prim.UVs)
			}
			gmesh.Primitives = append(gmesh.Primitives, &gltf.Primitive{
				Mode:       gltf.PrimitiveTriangles,
				Attributes: attrs,
				Indices:    gltf.Index(modeler.WriteIndices(doc, prim.Indices)),
				Material:   gltf.Index(material),
			})
		}
		if len(gmesh.Primitives) == 0 {
			continue
		}
		meshIndex := len(doc.Meshes)
		doc.Meshes = append(doc.Meshes, gmesh)
		node := &gltf.Node{Name: obj.Name, Mesh: gltf.Index(meshIndex)}
		if obj.Translation != [3]float32{} {
			node.Translation = [3]float64{float64(obj.Translation[0]), float64(obj.Translation[1]), float64(obj.Translation[2])}
		}
		nodeIndex := len(doc.Nodes)
		doc.Nodes = append(doc.Nodes, node)
		doc.Scenes[sceneIndex].Nodes = append(doc.Scenes[sceneIndex].Nodes, nodeIndex)
	}
	return doc, nil
}

// Save writes the document as a binary .glb file.
func Save(path string, doc *gltf.Document) error { return gltf.SaveBinary(doc, path) }

// Write encodes the document as binary glTF to w.
func Write(w io.Writer, doc *gltf.Document) error {
	enc := gltf.NewEncoder(w)
	enc.AsBinary = true
	return enc.Encode(doc)
}

func useExtension(doc *gltf.Document, name string) {
	for _, e := range doc.ExtensionsUsed {
		if e == name {
			return
		}
	}
	doc.ExtensionsUsed = append(doc.ExtensionsUsed, name)
}

func addSampler(doc *gltf.Document) int {
	doc.Samplers = append(doc.Samplers, &gltf.Sampler{
		MagFilter: gltf.MagNearest,
		MinFilter: gltf.MinNearest,
		WrapS:     gltf.WrapClampToEdge,
		WrapT:     gltf.WrapClampToEdge,
	})
	return len(doc.Samplers) - 1
}

func addUntexturedMaterial(doc *gltf.Document, name string, opts Options) int {
	doc.Materials = append(doc.Materials, &gltf.Material{
		Name:       name + "_untextured",
		Extensions: gltf.Extensions{unlit.ExtensionName: unlit.Unlit{}},
		PBRMetallicRoughness: &gltf.PBRMetallicRoughness{
			BaseColorFactor: &[4]float64{1, 1, 1, 1},
			MetallicFactor:  gltf.Float(0),
			RoughnessFactor: gltf.Float(1),
		},
		AlphaMode:   gltf.AlphaOpaque,
		DoubleSided: opts.DoubleSided,
	})
	return len(doc.Materials) - 1
}

func addTexturedMaterial(doc *gltf.Document, name string, page *psx.Image, sampler *int, opts Options) (int, error) {
	imageIndex, err := addImage(doc, name, page)
	if err != nil {
		return 0, err
	}
	texIndex := len(doc.Textures)
	doc.Textures = append(doc.Textures, &gltf.Texture{Name: name, Source: gltf.Index(imageIndex), Sampler: sampler})
	doc.Materials = append(doc.Materials, &gltf.Material{
		Name:       name,
		Extensions: gltf.Extensions{unlit.ExtensionName: unlit.Unlit{}},
		PBRMetallicRoughness: &gltf.PBRMetallicRoughness{
			BaseColorTexture: &gltf.TextureInfo{Index: texIndex},
			MetallicFactor:   gltf.Float(0),
			RoughnessFactor:  gltf.Float(1),
		},
		AlphaMode:   gltf.AlphaMask,
		AlphaCutoff: gltf.Float(float64(opts.AlphaCutoff)),
		DoubleSided: opts.DoubleSided,
	})
	return len(doc.Materials) - 1, nil
}

func addImage(doc *gltf.Document, name string, page *psx.Image) (int, error) {
	img := &image.NRGBA{
		Pix:    page.Pixels,
		Stride: page.Width * 4,
		Rect:   image.Rect(0, 0, page.Width, page.Height),
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return 0, fmt.Errorf("glb: encode %s texture: %w", name, err)
	}
	return modeler.WriteImage(doc, name, "image/png", &buf)
}
