package glb

import (
	"path/filepath"
	"testing"

	"github.com/qmuntal/gltf"
	"github.com/tridentsx/wipeout-go/internal/model"
	"github.com/tridentsx/wipeout-go/internal/psx"
)

func tex(v uint16) *uint16 { return &v }

// buildMixedMesh returns a mesh with one textured quad (page 0) and one
// untextured tri, plus a synthetic 2x2 texture page.
func buildMixedMesh() (*model.Mesh, []*psx.Image) {
	col := psx.Color{R: 128, G: 128, B: 128}
	obj := psx.Object{
		Header:   psx.ObjectHeader{Name: "obj", Position: psx.Vector3{X: 1, Y: 2, Z: 3}},
		Vertices: []psx.Vertex{{X: 0, Y: 0, Z: 0}, {X: 4, Y: 0, Z: 0}, {X: 4, Y: 4, Z: 0}, {X: 0, Y: 4, Z: 0}},
		Polygons: []psx.Polygon{
			{
				Type: psx.PolygonTexturedQuadVertexColor, Indices: []uint16{0, 1, 2, 3}, Texture: tex(0),
				UV:     []psx.UV{{U: 0, V: 0}, {U: 2, V: 0}, {U: 2, V: 2}, {U: 0, V: 2}},
				Colors: []psx.Color{col, col, col, col},
			},
			{Type: psx.PolygonFlatTrisFaceColor, Indices: []uint16{0, 1, 2}, Color: &col},
		},
	}
	page := &psx.Image{Width: 2, Height: 2, Pixels: make([]byte, 2*2*4)}
	for i := range page.Pixels {
		page.Pixels[i] = 0xff // opaque white
	}
	pages := []*psx.Image{page}
	return model.FromPRM([]psx.Object{obj}, model.PageSizes(pages)), pages
}

func TestBuildDocumentStructure(t *testing.T) {
	mesh, pages := buildMixedMesh()
	doc, err := BuildDocument("test", mesh, pages, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Images) != 1 {
		t.Fatalf("images = %d, want 1", len(doc.Images))
	}
	if len(doc.Samplers) != 1 {
		t.Fatalf("samplers = %d, want 1", len(doc.Samplers))
	}
	if len(doc.Materials) != 2 {
		t.Fatalf("materials = %d, want 2 (untextured + page0)", len(doc.Materials))
	}
	if len(doc.Meshes) != 1 || len(doc.Nodes) != 1 {
		t.Fatalf("meshes=%d nodes=%d, want 1/1", len(doc.Meshes), len(doc.Nodes))
	}
	if len(doc.Scenes) == 0 || len(doc.Scenes[*doc.Scene].Nodes) != 1 {
		t.Fatalf("scene nodes not wired")
	}
	foundUnlit := false
	for _, e := range doc.ExtensionsUsed {
		if e == "KHR_materials_unlit" {
			foundUnlit = true
		}
	}
	if !foundUnlit {
		t.Fatal("KHR_materials_unlit not declared in extensionsUsed")
	}
	// Exactly one primitive should carry TEXCOORD_0 (the textured quad).
	textured := 0
	for _, p := range doc.Meshes[0].Primitives {
		if _, ok := p.Attributes[gltf.TEXCOORD_0]; ok {
			textured++
		}
	}
	if textured != 1 {
		t.Fatalf("primitives with TEXCOORD_0 = %d, want 1", textured)
	}
}

func TestBuildDocumentRoundTrip(t *testing.T) {
	mesh, pages := buildMixedMesh()
	doc, err := BuildDocument("test", mesh, pages, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "test.glb")
	if err := Save(path, doc); err != nil {
		t.Fatal(err)
	}
	reopened, err := gltf.Open(path)
	if err != nil {
		t.Fatalf("re-open .glb: %v", err)
	}
	if len(reopened.Images) != 1 || len(reopened.Materials) != 2 || len(reopened.Meshes) != 1 {
		t.Fatalf("re-opened images=%d materials=%d meshes=%d, want 1/2/1",
			len(reopened.Images), len(reopened.Materials), len(reopened.Meshes))
	}
	if len(reopened.Meshes[0].Primitives) != 2 {
		t.Fatalf("re-opened primitives = %d, want 2", len(reopened.Meshes[0].Primitives))
	}
}
