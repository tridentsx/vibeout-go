// Package trackpack defines the whole-track ".trackpack" format and utilities
// for reading it, shared by the encoder (cmd/encode-track) and the runtime
// renderer. The format bundles a track's sky + scenery + driving surface as
// modern, upres-ready assets while keeping the surface's tile indices and
// per-face gameplay flags/triggers as first-class data. See
// docs/track-format.md for the full specification.
package trackpack

// Pack is a decoded track.json plus the directory it was loaded from (so tile
// and layer files can be resolved). All geometry is in glTF space
// ((x,-y,-z), Y-up, right-handed).
type Pack struct {
	FormatVersion int      `json:"formatVersion"`
	Name          string   `json:"name"`
	Axes          string   `json:"axes"`
	Surface       Surface  `json:"surface"`
	Layers        Layers   `json:"layers"`
	Textures      Textures `json:"textures"`

	dir string // base directory, set by Load; not serialized
}

// Surface is the driving surface's resolution-independent logic. It is never
// baked to a mesh: a consumer builds triangles from Faces (see Face.Triangles)
// and applies the tile textures by index.
type Surface struct {
	TileCount   int          `json:"tileCount"`
	TileDir     string       `json:"tileDir"`
	Vertices    [][3]int32   `json:"vertices"`
	Faces       []Face       `json:"faces"`
	Sections    []Section    `json:"sections"`
	Checkpoints []Checkpoint `json:"checkpoints"`
}

// Face is one quad of the driving surface (a triangle repeats V[3]=V[2]).
type Face struct {
	V      [4]uint16  `json:"v"`      // indices into Surface.Vertices
	Normal [3]float64 `json:"normal"` // unit, glTF space
	Tile   int        `json:"tile"`   // index into the surface tiles
	Color  [3]uint8   `json:"color"`  // 0..255, PS1 128==1.0
	Flip   bool       `json:"flip"`   // horizontal texture flip
	Flags  FaceFlags  `json:"flags"`
}

// FaceFlags preserves the raw flag byte plus decoded triggers.
type FaceFlags struct {
	Raw        uint8 `json:"raw"`
	Track      bool  `json:"track"`      // bit 0 (0x01): section-run marker
	WeaponPad  bool  `json:"weaponPad"`  // bit 1 (0x02): weapon pad
	Flip       bool  `json:"flip"`       // bit 2 (0x04): horizontal texture flip
	WeaponPad2 bool  `json:"weaponPad2"` // bit 3 (0x08): weapon pad, second variant
	Unused16   bool  `json:"unused16"`   // bit 4 (0x10): never set, never read -- see psx.TrackFaceUnused16
	Boost      bool  `json:"boost"`      // bit 5 (0x20): speed pad
	StartGrid  bool  `json:"startGrid"`  // bit 6 (0x40): starting-grid run
	Checkpoint bool  `json:"checkpoint"` // bit 7 (0x80): checkpoint face
}

// Section is a node in the track's section graph.
type Section struct {
	Prev         int32        `json:"prev"`
	Next         int32        `json:"next"`
	NextJunction int32        `json:"nextJunction"`
	Center       [3]int32     `json:"center"`
	FirstFace    uint32       `json:"firstFace"`
	NumFaces     uint16       `json:"numFaces"`
	Flags        SectionFlags `json:"flags"`
}

// SectionFlags preserves the raw section flag word plus decoded bits.
type SectionFlags struct {
	Raw           uint16 `json:"raw"`
	Jump          bool   `json:"jump"`
	Junction      bool   `json:"junction"`
	JunctionStart bool   `json:"junctionStart"`
	JunctionEnd   bool   `json:"junctionEnd"`
}

// Checkpoint holds one CPOINT*.CHK file's per-record section indices.
type Checkpoint struct {
	File     string  `json:"file"`
	Sections []int16 `json:"sections"`
}

// Layers references the baked scenery and sky glTF meshes.
type Layers struct {
	Scenery *LayerRef `json:"scenery,omitempty"`
	Sky     *LayerRef `json:"sky,omitempty"`
}

// LayerRef points at a file within the pack.
type LayerRef struct {
	File string `json:"file"`
}

// Textures manifests the surface tile files and the embedded layer textures.
type Textures struct {
	Surface []SurfaceTexture `json:"surface"`
	Scenery *EmbeddedTexture `json:"scenery,omitempty"`
	Sky     *EmbeddedTexture `json:"sky,omitempty"`
}

// SurfaceTexture describes one surface tile PNG, keyed by logical tile index.
type SurfaceTexture struct {
	Tile   int    `json:"tile"`
	File   string `json:"file"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// EmbeddedTexture records the original source of textures baked into a layer.
type EmbeddedTexture struct {
	Source     string `json:"source"`
	EmbeddedIn string `json:"embeddedIn"`
}
