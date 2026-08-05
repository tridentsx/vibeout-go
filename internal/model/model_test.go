package model

import (
	"testing"

	"github.com/tridentsx/wipeout-go/internal/psx"
)

func texIndex(v uint16) *uint16 { return &v }

func TestTexturedQuadExpandsToTwoTrianglesWithNormalizedUV(t *testing.T) {
	obj := psx.Object{
		Header:   psx.ObjectHeader{Name: "t", Position: psx.Vector3{X: 10, Y: 20, Z: 30}},
		Vertices: []psx.Vertex{{X: 0, Y: 0, Z: 0}, {X: 2, Y: 0, Z: 0}, {X: 2, Y: 2, Z: 0}, {X: 0, Y: 2, Z: 0}},
		Polygons: []psx.Polygon{{
			Type:    psx.PolygonTexturedQuadVertexColor,
			Indices: []uint16{0, 1, 2, 3},
			Texture: texIndex(0),
			UV:      []psx.UV{{U: 0, V: 0}, {U: 64, V: 0}, {U: 64, V: 64}, {U: 0, V: 64}},
			Colors:  []psx.Color{{R: 128, G: 128, B: 128}, {R: 128, G: 128, B: 128}, {R: 128, G: 128, B: 128}, {R: 128, G: 128, B: 128}},
		}},
	}
	mesh := FromPRM([]psx.Object{obj}, []Page{{W: 64, H: 64}})
	if len(mesh.Objects) != 1 {
		t.Fatalf("objects = %d", len(mesh.Objects))
	}
	o := mesh.Objects[0]
	if o.Translation != [3]float32{10, -20, -30} {
		t.Fatalf("translation = %v, want {10,-20,-30}", o.Translation)
	}
	if len(o.Primitives) != 1 {
		t.Fatalf("primitives = %d", len(o.Primitives))
	}
	p := o.Primitives[0]
	if !p.Textured() || p.Page != 0 {
		t.Fatalf("page = %d textured=%v", p.Page, p.Textured())
	}
	if len(p.Positions) != 4 || len(p.Indices) != 6 || len(p.UVs) != 4 {
		t.Fatalf("positions=%d indices=%d uvs=%d, want 4/6/4", len(p.Positions), len(p.Indices), len(p.UVs))
	}
	if p.Positions[2] != [3]float32{2, -2, 0} {
		t.Fatalf("position[2] = %v, want {2,-2,0}", p.Positions[2])
	}
	if p.UVs[1] != [2]float32{1, 0} || p.UVs[2] != [2]float32{1, 1} {
		t.Fatalf("uv = %v %v, want {1,0} {1,1}", p.UVs[1], p.UVs[2])
	}
	if p.Colors[0] != [4]uint8{255, 255, 255, 255} {
		t.Fatalf("color = %v, want {255,255,255,255} (128=1.0)", p.Colors[0])
	}
	// Reference winding: tri (2,1,0); quad (2,1,0)+(2,3,1).
	want := []uint32{2, 1, 0, 2, 3, 1}
	for i, w := range want {
		if p.Indices[i] != w {
			t.Fatalf("indices = %v, want %v (reference winding)", p.Indices, want)
		}
	}
}

func TestUntexturedFaceColorTriangle(t *testing.T) {
	col := psx.Color{R: 128, G: 64, B: 0}
	obj := psx.Object{
		Vertices: []psx.Vertex{{X: 0, Y: 0, Z: 0}, {X: 1, Y: 0, Z: 0}, {X: 0, Y: 1, Z: 0}},
		Polygons: []psx.Polygon{{Type: psx.PolygonFlatTrisFaceColor, Indices: []uint16{0, 1, 2}, Color: &col}},
	}
	mesh := FromPRM([]psx.Object{obj}, nil)
	p := mesh.Objects[0].Primitives[0]
	if p.Textured() || p.Page != Untextured {
		t.Fatalf("expected untextured, page=%d", p.Page)
	}
	if len(p.Positions) != 3 || len(p.Indices) != 3 || len(p.UVs) != 0 {
		t.Fatalf("positions=%d indices=%d uvs=%d, want 3/3/0", len(p.Positions), len(p.Indices), len(p.UVs))
	}
	if p.Colors[0] != [4]uint8{255, 128, 0, 255} {
		t.Fatalf("color = %v, want {255,128,0,255}", p.Colors[0])
	}
}

func TestCornerDeduplication(t *testing.T) {
	// Two identical textured triangles: the second must reuse the first's
	// three corners (same vertex+UV+color), so only 3 positions exist.
	poly := psx.Polygon{
		Type:    psx.PolygonTexturedTrisVertexColor,
		Indices: []uint16{0, 1, 2},
		Texture: texIndex(0),
		UV:      []psx.UV{{U: 0, V: 0}, {U: 10, V: 0}, {U: 0, V: 10}},
		Colors:  []psx.Color{{R: 128, G: 128, B: 128}, {R: 128, G: 128, B: 128}, {R: 128, G: 128, B: 128}},
	}
	obj := psx.Object{
		Vertices: []psx.Vertex{{X: 0, Y: 0, Z: 0}, {X: 10, Y: 0, Z: 0}, {X: 0, Y: 10, Z: 0}},
		Polygons: []psx.Polygon{poly, poly},
	}
	mesh := FromPRM([]psx.Object{obj}, []Page{{W: 16, H: 16}})
	p := mesh.Objects[0].Primitives[0]
	if len(p.Positions) != 3 {
		t.Fatalf("positions = %d, want 3 (deduped)", len(p.Positions))
	}
	if len(p.Indices) != 6 {
		t.Fatalf("indices = %d, want 6", len(p.Indices))
	}
}

func TestSpriteBakesTexturedQuad(t *testing.T) {
	spriteColor := psx.Color{R: 128, G: 128, B: 128}
	obj := psx.Object{
		Vertices: []psx.Vertex{{X: 0, Y: 0, Z: 0}},
		Polygons: []psx.Polygon{{
			Type:         psx.PolygonSpriteBottomAnchor,
			Texture:      texIndex(0),
			SpriteIndex:  0,
			SpriteWidth:  4,
			SpriteHeight: 8,
			Color:        &spriteColor,
		}},
	}
	mesh := FromPRM([]psx.Object{obj}, []Page{{W: 32, H: 32}})
	p := mesh.Objects[0].Primitives[0]
	if !p.Textured() || len(p.Positions) != 4 || len(p.Indices) != 6 || len(p.UVs) != 4 {
		t.Fatalf("sprite quad positions=%d indices=%d uvs=%d textured=%v", len(p.Positions), len(p.Indices), len(p.UVs), p.Textured())
	}
	// Bottom anchor at origin extends up: y in [0,8], x in [-2,2].
	if p.Positions[0] != [3]float32{-2, 0, 0} || p.Positions[2] != [3]float32{2, 8, 0} {
		t.Fatalf("sprite corners = %v .. %v", p.Positions[0], p.Positions[2])
	}
}

func TestMultipleMaterialsSplitIntoPrimitives(t *testing.T) {
	col := psx.Color{R: 128, G: 128, B: 128}
	obj := psx.Object{
		Vertices: []psx.Vertex{{X: 0, Y: 0, Z: 0}, {X: 1, Y: 0, Z: 0}, {X: 0, Y: 1, Z: 0}},
		Polygons: []psx.Polygon{
			{Type: psx.PolygonFlatTrisFaceColor, Indices: []uint16{0, 1, 2}, Color: &col},
			{Type: psx.PolygonTexturedTrisFaceColor, Indices: []uint16{0, 1, 2}, Texture: texIndex(1), UV: []psx.UV{{}, {}, {}}, Color: &col},
		},
	}
	mesh := FromPRM([]psx.Object{obj}, []Page{{W: 8, H: 8}, {W: 8, H: 8}})
	if len(mesh.Objects[0].Primitives) != 2 {
		t.Fatalf("primitives = %d, want 2 (untextured + page 1)", len(mesh.Objects[0].Primitives))
	}
}

func TestObjectMeshRemapsAndPrunesPages(t *testing.T) {
	col := psx.Color{R: 128, G: 128, B: 128}
	obj := psx.Object{
		Vertices: []psx.Vertex{{X: 0, Y: 0, Z: 0}, {X: 1, Y: 0, Z: 0}, {X: 0, Y: 1, Z: 0}},
		Polygons: []psx.Polygon{
			{Type: psx.PolygonTexturedTrisFaceColor, Indices: []uint16{0, 1, 2}, Texture: texIndex(5), UV: []psx.UV{{}, {}, {}}, Color: &col},
			{Type: psx.PolygonFlatTrisFaceColor, Indices: []uint16{0, 1, 2}, Color: &col},
		},
	}
	pages := make([]Page, 6)
	for i := range pages {
		pages[i] = Page{W: 8, H: 8}
	}
	mesh := FromPRM([]psx.Object{obj}, pages)

	sub, used := ObjectMesh(mesh.Objects[0])
	if len(used) != 1 || used[0] != 5 {
		t.Fatalf("used pages = %v, want [5]", used)
	}
	if sub.NumPages != 1 {
		t.Fatalf("NumPages = %d, want 1", sub.NumPages)
	}
	textured := false
	for _, p := range sub.Objects[0].Primitives {
		if p.Textured() {
			textured = true
			if p.Page != 0 {
				t.Fatalf("remapped page = %d, want 0", p.Page)
			}
		}
	}
	if !textured {
		t.Fatal("expected a textured primitive")
	}
}

func TestIsCollection(t *testing.T) {
	col := psx.Color{R: 128, G: 128, B: 128}
	tri := func(pos int16) psx.Object {
		return psx.Object{
			Header:   psx.ObjectHeader{Position: psx.Vector3{X: int32(pos)}},
			Vertices: []psx.Vertex{{X: -100}, {X: 100}, {Y: 100}},
			Polygons: []psx.Polygon{{Type: psx.PolygonFlatTrisFaceColor, Indices: []uint16{0, 1, 2}, Color: &col}},
		}
	}
	small := func(pos int32) psx.Object {
		return psx.Object{
			Header:   psx.ObjectHeader{Position: psx.Vector3{X: pos}},
			Vertices: []psx.Vertex{{X: -10}, {X: 10}, {Y: 10}},
			Polygons: []psx.Polygon{{Type: psx.PolygonFlatTrisFaceColor, Indices: []uint16{0, 1, 2}, Color: &col}},
		}
	}

	// Two large objects a few units apart (spread << radius) -> collection.
	if !FromPRM([]psx.Object{tri(0), tri(5)}, nil).IsCollection() {
		t.Error("stacked objects should be detected as a collection")
	}
	// Two small objects far apart (spread >> radius) -> scene.
	if FromPRM([]psx.Object{small(0), small(100000)}, nil).IsCollection() {
		t.Error("widely-spread objects should be a scene, not a collection")
	}
	// A single object is never a collection.
	if FromPRM([]psx.Object{tri(0)}, nil).IsCollection() {
		t.Error("single object must not be a collection")
	}
}
