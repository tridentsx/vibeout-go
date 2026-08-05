package trackpack

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

const sampleJSON = `{
  "formatVersion": 1, "name": "T", "axes": "gltf",
  "surface": {
    "tileCount": 1, "tileDir": "surface/tiles",
    "vertices": [[0,0,0],[1,0,0],[1,-1,0],[0,-1,0]],
    "faces": [ {"v":[0,1,2,3],"normal":[0,1,0],"tile":0,"color":[128,128,128],"flip":false,
      "flags":{"raw":33,"track":true,"weaponPad":false,"flip":false,"weaponPad2":false,"special":false,"boost":true}} ],
    "sections": [ {"prev":-1,"next":0,"nextJunction":-1,"center":[0,0,0],"firstFace":0,"numFaces":1,
      "flags":{"raw":1,"jump":true,"junction":false,"junctionStart":false,"junctionEnd":false}} ],
    "checkpoints": [ {"file":"CPOINT0.CHK","sections":[0,-1,-1,-1,-1,-1]} ]
  },
  "layers": { "sky": {"file":"sky.glb"} },
  "textures": { "surface": [ {"tile":0,"file":"surface/tiles/000.png","width":128,"height":128} ] }
}`

func TestDecodeSurfaceAndFlags(t *testing.T) {
	p, err := Decode(bytes.NewReader([]byte(sampleJSON)))
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "T" || p.Surface.TileCount != 1 || len(p.Surface.Faces) != 1 {
		t.Fatalf("bad pack: %+v", p)
	}
	f := p.Surface.Faces[0]
	if !f.Flags.Track || !f.Flags.Boost || f.Flags.Raw != 33 {
		t.Fatalf("face flags = %+v, want track+boost raw=33", f.Flags)
	}
	if !p.Surface.Sections[0].Flags.Jump {
		t.Fatalf("section flags = %+v, want jump", p.Surface.Sections[0].Flags)
	}
	if len(p.Surface.Checkpoints) != 1 || p.Surface.Checkpoints[0].Sections[0] != 0 {
		t.Fatalf("checkpoints = %+v", p.Surface.Checkpoints)
	}
}

func TestDecodeRejectsUnsupportedVersion(t *testing.T) {
	if _, err := Decode(bytes.NewReader([]byte(`{"formatVersion":2}`))); err == nil {
		t.Fatal("expected unsupported-version error")
	}
}

func TestFaceCornerUVAndTriangles(t *testing.T) {
	f := Face{V: [4]uint16{0, 1, 2, 3}}
	uv := f.CornerUV()
	if uv[0] != ([2]float32{1, 0}) || uv[2] != ([2]float32{0, 1}) {
		t.Fatalf("unflipped uv = %v", uv)
	}
	f.Flip = true
	if f.CornerUV()[0] != ([2]float32{0, 0}) {
		t.Fatalf("flipped uv[0] = %v, want (0,0)", f.CornerUV()[0])
	}

	tris := (&Face{V: [4]uint16{10, 11, 12, 13}}).Triangles()
	if tris[0][0].Index != 10 || tris[0][1].Index != 11 || tris[0][2].Index != 12 {
		t.Fatalf("tri0 indices = %v %v %v, want 10,11,12", tris[0][0].Index, tris[0][1].Index, tris[0][2].Index)
	}
	if tris[1][0].Index != 13 || tris[1][1].Index != 10 || tris[1][2].Index != 12 {
		t.Fatalf("tri1 indices = %v %v %v, want 13,10,12", tris[1][0].Index, tris[1][1].Index, tris[1][2].Index)
	}
}

func TestLoadResolvesFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "track.json"), []byte(sampleJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	tileDir := filepath.Join(dir, "surface", "tiles")
	if err := os.MkdirAll(tileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(tileDir, "000.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := os.WriteFile(filepath.Join(dir, "sky.glb"), []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Dir() != dir {
		t.Fatalf("Dir() = %q", p.Dir())
	}
	path, ok := p.TilePath(0)
	if !ok || filepath.Base(path) != "000.png" {
		t.Fatalf("TilePath(0) = %q, %v", path, ok)
	}
	if _, err := p.LoadTile(0); err != nil {
		t.Fatalf("LoadTile(0): %v", err)
	}
	if sky, ok := p.SkyPath(); !ok || filepath.Base(sky) != "sky.glb" {
		t.Fatalf("SkyPath() = %q, %v", sky, ok)
	}
	if _, ok := p.SceneryPath(); ok {
		t.Fatal("SceneryPath() should be absent for this pack")
	}
}
