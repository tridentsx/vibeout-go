// Package model converts parsed WipEout 2097 PRM objects into a neutral,
// renderer-agnostic triangle mesh suitable for glTF export. It depends on
// neither SDL nor the glTF library, so the offline exporter (and, later, the
// runtime renderer) can share one PRM->mesh conversion.
//
// The conversion is confirmed against the retail disc (see
// cmd/export-models/TODO.md): quads triangulate as a fan, per-corner UV/color
// are expanded to per-vertex attributes, positions use the axis map
// (x, -y, -z) (determinant +1, so winding is preserved) with the object's
// Position supplied separately as a node translation, colors follow the PS1
// GTE's 128 = 1.0 convention, and UVs are pixel coordinates normalized by the
// referenced texture page's dimensions with no V flip.
package model

import (
	"math"

	"github.com/tridentsx/wipeout-go/internal/psx"
)

// Untextured is the Primitive.Page value for geometry that carries no texture.
const Untextured = -1

// Page is the pixel size of a decoded texture page, used to normalize the
// PRM's pixel-space UVs into glTF's 0..1 range.
type Page struct{ W, H int }

// Primitive is a set of triangles that share one material: a single texture
// page, or none (Page == Untextured). The attribute slices are parallel and
// indexed by Indices; UVs is populated only when the primitive is textured.
type Primitive struct {
	Page      int
	Positions [][3]float32
	UVs       [][2]float32
	Colors    [][4]uint8
	Indices   []uint32
}

// Textured reports whether the primitive references a texture page.
func (p *Primitive) Textured() bool { return p.Page != Untextured }

// Object is one PRM object: primitives grouped by material plus the glTF-space
// node translation (the PRM object's world Position, axis-converted).
type Object struct {
	Name        string
	Translation [3]float32
	Primitives  []*Primitive
}

// Mesh is a whole PRM file's converted geometry.
type Mesh struct {
	Objects  []Object
	NumPages int
}

// FromPRM converts every object in a decoded PRM into neutral geometry. pages
// gives the pixel size of each texture page (from the paired CMP) so UVs can be
// normalized; pass nil for untextured models. Out-of-range page indices are
// tolerated (treated as 1x1) so a model whose CMP is missing still exports.
func FromPRM(objects []psx.Object, pages []Page) *Mesh {
	mesh := &Mesh{NumPages: len(pages)}
	for i := range objects {
		mesh.Objects = append(mesh.Objects, convertObject(&objects[i], pages))
	}
	return mesh
}

// ObjectMesh returns a single-object Mesh for o with its primitive texture
// page indices remapped to a compact 0..n range, plus the original page
// indices (in the new order) so the caller can select the matching images.
// It lets a multi-object PRM be exported as one file per object without each
// file embedding textures it does not use.
func ObjectMesh(o Object) (*Mesh, []int) {
	remap := map[int]int{}
	var used []int
	out := Object{Name: o.Name, Translation: o.Translation}
	for _, p := range o.Primitives {
		cp := &Primitive{
			Page:      p.Page,
			Positions: p.Positions,
			UVs:       p.UVs,
			Colors:    p.Colors,
			Indices:   p.Indices,
		}
		if p.Page != Untextured {
			ni, ok := remap[p.Page]
			if !ok {
				ni = len(used)
				remap[p.Page] = ni
				used = append(used, p.Page)
			}
			cp.Page = ni
		}
		out.Primitives = append(out.Primitives, cp)
	}
	return &Mesh{Objects: []Object{out}, NumPages: len(used)}, used
}

// IsCollection reports whether the mesh's objects are stacked at ~one place
// (each authored near a shared origin) rather than spread into a coherent
// scene. Collections -- the COMMON menu/ship/track previews and per-team
// collision sets -- overlap when placed by their node translations, so they
// are better exported one file per object; scenes (a track's SCENE.PRM, or the
// multi-car TRAIN model whose cars sit at distinct positions) must stay merged.
//
// The test compares the spread of object centers (node translations) against
// the largest object's radius. It is validated against every retail
// multi-object PRM with a ~25x margin: collections score spread/radius <= 0.37,
// scenes >= 9.5.
func (m *Mesh) IsCollection() bool {
	if len(m.Objects) < 2 {
		return false
	}
	lo, hi := m.Objects[0].Translation, m.Objects[0].Translation
	var maxRadius float32
	for _, o := range m.Objects {
		for a := 0; a < 3; a++ {
			if o.Translation[a] < lo[a] {
				lo[a] = o.Translation[a]
			}
			if o.Translation[a] > hi[a] {
				hi[a] = o.Translation[a]
			}
		}
		for _, p := range o.Primitives {
			for _, pos := range p.Positions {
				for a := 0; a < 3; a++ {
					if v := abs32(pos[a]); v > maxRadius {
						maxRadius = v
					}
				}
			}
		}
	}
	dx, dy, dz := hi[0]-lo[0], hi[1]-lo[1], hi[2]-lo[2]
	spread := float32(math.Sqrt(float64(dx*dx + dy*dy + dz*dz)))
	return spread < maxRadius
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// cornerKey deduplicates expanded corners: the same source vertex with the same
// UV and color becomes one glTF vertex, while differing UV/color split it.
type cornerKey struct {
	vertex     uint16
	u, v       uint8
	r, g, b, a uint8
}

// builder accumulates one material's primitive with corner de-duplication.
type builder struct {
	prim  *Primitive
	dedup map[cornerKey]uint32
}

func convertObject(obj *psx.Object, pages []Page) Object {
	out := Object{
		Name:        obj.Header.Name,
		Translation: axis(obj.Header.Position.X, obj.Header.Position.Y, obj.Header.Position.Z),
	}
	builders := map[int]*builder{}
	order := []int{}
	get := func(page int) *builder {
		b := builders[page]
		if b == nil {
			b = &builder{prim: &Primitive{Page: page}, dedup: map[cornerKey]uint32{}}
			builders[page] = b
			order = append(order, page)
		}
		return b
	}

	for i := range obj.Polygons {
		poly := &obj.Polygons[i]
		switch poly.Type {
		case psx.PolygonSpriteTopAnchor, psx.PolygonSpriteBottomAnchor:
			addSprite(get, obj, poly)
			continue
		}
		if len(poly.Indices) < 3 {
			continue
		}
		page := Untextured
		if poly.Texture != nil {
			page = int(*poly.Texture)
		}
		b := get(page)
		// Fan-triangulate: (0, k, k+1) for k in 1..n-2 covers tris and quads.
		local := make([]uint32, len(poly.Indices))
		valid := true
		for c := range poly.Indices {
			idx, ok := addCorner(b, obj, poly, c, pages, page)
			if !ok {
				valid = false
				break
			}
			local[c] = idx
		}
		if !valid {
			continue
		}
		// Match the reference renderers (wipeout.js, wipeout-rewrite/object.c)
		// exactly: winding reversed from file order (so front faces point
		// outward after the (x,-y,-z) map) and the quad split along the 1-2
		// diagonal. Only tris/quads occur in the retail corpus; a reversed fan
		// covers any other count defensively.
		switch len(local) {
		case 3:
			b.prim.Indices = append(b.prim.Indices, local[2], local[1], local[0])
		case 4:
			b.prim.Indices = append(b.prim.Indices,
				local[2], local[1], local[0],
				local[2], local[3], local[1])
		default:
			for k := 1; k+1 < len(local); k++ {
				b.prim.Indices = append(b.prim.Indices, local[k+1], local[k], local[0])
			}
		}
	}

	for _, page := range order {
		out.Primitives = append(out.Primitives, builders[page].prim)
	}
	return out
}

// addCorner appends (or reuses) the expanded vertex for corner c of poly.
func addCorner(b *builder, obj *psx.Object, poly *psx.Polygon, c int, pages []Page, page int) (uint32, bool) {
	vi := poly.Indices[c]
	if int(vi) >= len(obj.Vertices) {
		return 0, false
	}
	color := cornerColor(poly, c)
	var uv psx.UV
	if page != Untextured && c < len(poly.UV) {
		uv = poly.UV[c]
	}
	key := cornerKey{vertex: vi, u: uv.U, v: uv.V, r: color[0], g: color[1], b: color[2], a: color[3]}
	if idx, ok := b.dedup[key]; ok {
		return idx, true
	}
	v := obj.Vertices[vi]
	idx := uint32(len(b.prim.Positions))
	b.prim.Positions = append(b.prim.Positions, axis(int32(v.X), int32(v.Y), int32(v.Z)))
	b.prim.Colors = append(b.prim.Colors, color)
	if page != Untextured {
		b.prim.UVs = append(b.prim.UVs, normUV(uv, pageDims(pages, page)))
	}
	b.dedup[key] = idx
	return idx, true
}

// addSprite bakes a sprite polygon into a page-textured quad (two triangles).
func addSprite(get func(int) *builder, obj *psx.Object, poly *psx.Polygon) {
	if poly.Texture == nil || int(poly.SpriteIndex) >= len(obj.Vertices) {
		return
	}
	page := int(*poly.Texture)
	anchor := obj.Vertices[poly.SpriteIndex]
	a := axis(int32(anchor.X), int32(anchor.Y), int32(anchor.Z))
	ax, ay, az := a[0], a[1], a[2]
	hw := float32(poly.SpriteWidth) / 2
	h := float32(poly.SpriteHeight)
	// Vertical quad in the X-Y plane, centered horizontally on the anchor.
	// Bottom-anchor extends up (+Y); top-anchor extends down.
	yBottom, yTop := ay, ay+h
	if poly.Type == psx.PolygonSpriteTopAnchor {
		yBottom, yTop = ay-h, ay
	}
	var color [4]uint8
	if poly.Color != nil {
		color = scaleColor(*poly.Color)
	} else {
		color = [4]uint8{255, 255, 255, 255}
	}
	b := get(page)
	base := uint32(len(b.prim.Positions))
	// Corners: BL, BR, TR, TL with UV spanning the whole page (top-left origin).
	corners := [4]struct {
		pos [3]float32
		uv  [2]float32
	}{
		{[3]float32{ax - hw, yBottom, az}, [2]float32{0, 1}},
		{[3]float32{ax + hw, yBottom, az}, [2]float32{1, 1}},
		{[3]float32{ax + hw, yTop, az}, [2]float32{1, 0}},
		{[3]float32{ax - hw, yTop, az}, [2]float32{0, 0}},
	}
	for _, cr := range corners {
		b.prim.Positions = append(b.prim.Positions, cr.pos)
		b.prim.UVs = append(b.prim.UVs, cr.uv)
		b.prim.Colors = append(b.prim.Colors, color)
	}
	b.prim.Indices = append(b.prim.Indices, base, base+1, base+2, base, base+2, base+3)
}

// cornerColor returns the RGBA (128=1.0) color for corner c: the per-vertex
// color when present, else the face color, else opaque white.
func cornerColor(poly *psx.Polygon, c int) [4]uint8 {
	if c < len(poly.Colors) {
		return scaleColor(poly.Colors[c])
	}
	if poly.Color != nil {
		return scaleColor(*poly.Color)
	}
	return [4]uint8{255, 255, 255, 255}
}

// axis maps a PRM model-space vector to glTF space: (x, -y, -z). WipEout's Y
// axis points down and its space is left-handed relative to glTF; this map has
// determinant +1 so triangle winding is preserved.
func axis(x, y, z int32) [3]float32 {
	return [3]float32{float32(x), -float32(y), -float32(z)}
}

// scaleColor converts a PRM color (128 = 1.0) to an opaque glTF vertex color
// byte, clamping over-bright (>128) values to 1.0.
func scaleColor(c psx.Color) [4]uint8 {
	return [4]uint8{scale128(c.R), scale128(c.G), scale128(c.B), 255}
}

func scale128(v uint8) uint8 {
	x := (int(v)*255 + 64) / 128 // round(v*255/128)
	if x > 255 {
		x = 255
	}
	return uint8(x)
}

func normUV(uv psx.UV, page Page) [2]float32 {
	w, h := page.W, page.H
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}
	return [2]float32{float32(uv.U) / float32(w), float32(uv.V) / float32(h)}
}

func pageDims(pages []Page, index int) Page {
	if index >= 0 && index < len(pages) {
		return pages[index]
	}
	return Page{W: 1, H: 1}
}

// PageSizes extracts the pixel dimensions of decoded texture pages for UV
// normalization. A nil image (a CMP member that is not a decodable TIM) yields
// a 1x1 placeholder so conversion stays total.
func PageSizes(images []*psx.Image) []Page {
	pages := make([]Page, len(images))
	for i, img := range images {
		if img != nil {
			pages[i] = Page{W: img.Width, H: img.Height}
		} else {
			pages[i] = Page{W: 1, H: 1}
		}
	}
	return pages
}
