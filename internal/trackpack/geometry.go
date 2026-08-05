package trackpack

// trackCornerUV are the driving surface's fixed per-corner UVs (normalized,
// v=0 at the top of the tile), matching the retail loader. Index 0..3 maps to
// the four face corners; the second row is the horizontally flipped variant.
var trackCornerUV = [2][4][2]float32{
	{{1, 0}, {0, 0}, {0, 1}, {1, 1}},
	{{0, 0}, {1, 0}, {1, 1}, {0, 1}},
}

// CornerUV returns the four corner UVs for this face, honoring Flip. Track
// faces do not store UVs; they are derived from this fixed rule, so they are
// resolution-independent.
func (f *Face) CornerUV() [4][2]float32 {
	if f.Flip {
		return trackCornerUV[1]
	}
	return trackCornerUV[0]
}

// SurfaceVertex is one triangulated corner: an index into Surface.Vertices plus
// its tile UV.
type SurfaceVertex struct {
	Index uint16
	UV    [2]float32
}

// Triangles returns the two triangles of a track face, matching the retail
// winding (v0,v1,v2 and v3,v0,v2) with their corner UVs, ready to feed a mesh
// builder. Vertices are already in glTF space, so the winding is preserved.
func (f *Face) Triangles() [2][3]SurfaceVertex {
	uv := f.CornerUV()
	return [2][3]SurfaceVertex{
		{{f.V[0], uv[0]}, {f.V[1], uv[1]}, {f.V[2], uv[2]}},
		{{f.V[3], uv[3]}, {f.V[0], uv[0]}, {f.V[2], uv[2]}},
	}
}
