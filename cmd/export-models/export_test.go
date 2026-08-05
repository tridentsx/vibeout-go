package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qmuntal/gltf"
	"github.com/tridentsx/wipeout-go/internal/glb"
)

func discRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join("..", "..", "assets", "WIPEOUT2")
	if _, err := os.Stat(root); err != nil {
		t.Skip(err)
	}
	return root
}

// countTriangles sums indexed triangles across a document's primitives.
func countTriangles(doc *gltf.Document) int {
	n := 0
	for _, m := range doc.Meshes {
		for _, p := range m.Primitives {
			if p.Indices != nil {
				acc := doc.Accessors[*p.Indices]
				n += int(acc.Count) / 3
			}
		}
	}
	return n
}

// TestExportAllRuntimeModels exports every runtime PRM and validates that each
// resulting .glb re-opens with geometry and materials. It also confirms that a
// textured craft embeds images while an untextured one does not.
func TestExportAllRuntimeModels(t *testing.T) {
	disc := discRoot(t)
	out := t.TempDir()

	if err := exportAll(disc, out, glb.DefaultOptions(), layoutMerge); err != nil {
		t.Fatal(err)
	}

	var glbs []string
	err := filepath.WalkDir(out, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".glb") {
			glbs = append(glbs, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(glbs) != 48 {
		t.Fatalf("exported %d .glb files, want 48", len(glbs))
	}

	for _, path := range glbs {
		doc, err := gltf.Open(path)
		if err != nil {
			t.Errorf("%s: re-open failed: %v", filepath.Base(path), err)
			continue
		}
		if len(doc.Meshes) == 0 {
			t.Errorf("%s: no meshes", filepath.Base(path))
		}
		if len(doc.Materials) == 0 {
			t.Errorf("%s: no materials", filepath.Base(path))
		}
		if tris := countTriangles(doc); tris == 0 {
			t.Errorf("%s: no triangles", filepath.Base(path))
		}
	}
}

func TestExportTexturedAndUntexturedCrafts(t *testing.T) {
	disc := discRoot(t)
	out := t.TempDir()

	if _, _, err := exportOne(filepath.Join(disc, "COMMON", "TERRY.PRM"), out, glb.DefaultOptions(), layoutMerge); err != nil {
		t.Fatal(err)
	}
	if _, _, err := exportOne(filepath.Join(disc, "COMMON", "VECTO.PRM"), out, glb.DefaultOptions(), layoutMerge); err != nil {
		t.Fatal(err)
	}

	terry, err := gltf.Open(filepath.Join(out, "TERRY.glb"))
	if err != nil {
		t.Fatal(err)
	}
	if len(terry.Images) == 0 {
		t.Error("TERRY (textured Qirex craft) embedded no images")
	}
	if len(terry.Textures) == 0 || len(terry.Samplers) == 0 {
		t.Errorf("TERRY: textures=%d samplers=%d, want >0", len(terry.Textures), len(terry.Samplers))
	}

	vecto, err := gltf.Open(filepath.Join(out, "VECTO.glb"))
	if err != nil {
		t.Fatal(err)
	}
	if len(vecto.Images) != 0 {
		t.Errorf("VECTO (untextured) embedded %d images, want 0", len(vecto.Images))
	}
	if countTriangles(vecto) == 0 {
		t.Error("VECTO: no triangles")
	}
}

func TestExportSplitWritesOneFilePerObject(t *testing.T) {
	disc := discRoot(t)
	out := t.TempDir()

	// JUNE is a multi-object collection whose preview objects overlap at the
	// origin; --split must write one single-object file per object.
	name, files, err := exportOne(filepath.Join(disc, "COMMON", "JUNE.PRM"), out, glb.DefaultOptions(), layoutSplit)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(out, name)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("split dir: %v", err)
	}
	glbs := 0
	for _, e := range entries {
		if strings.EqualFold(filepath.Ext(e.Name()), ".glb") {
			glbs++
			doc, oerr := gltf.Open(filepath.Join(dir, e.Name()))
			if oerr != nil {
				t.Errorf("%s: %v", e.Name(), oerr)
				continue
			}
			if len(doc.Meshes) != 1 {
				t.Errorf("%s: %d meshes, want 1 (one object per split file)", e.Name(), len(doc.Meshes))
			}
		}
	}
	if glbs < 2 || glbs != files {
		t.Fatalf("split produced %d .glb on disk, exportOne reported %d (want >1 and equal)", glbs, files)
	}
}

func TestExportAutoLayoutSplitsCollectionsNotScenes(t *testing.T) {
	disc := discRoot(t)
	out := t.TempDir()
	if err := exportAll(disc, out, glb.DefaultOptions(), layoutAuto); err != nil {
		t.Fatal(err)
	}
	isDir := func(rel string) bool {
		fi, err := os.Stat(filepath.Join(out, filepath.FromSlash(rel)))
		return err == nil && fi.IsDir()
	}
	isFile := func(rel string) bool {
		fi, err := os.Stat(filepath.Join(out, filepath.FromSlash(rel)))
		return err == nil && !fi.IsDir()
	}
	// Collections (stacked previews / per-team sets) auto-split into a
	// per-object directory.
	for _, c := range []string{"COMMON/JUNE", "COMMON/HARRY", "COMMON/TERRY", "COMMON/TELCO"} {
		if !isDir(c) {
			t.Errorf("expected %s to auto-split into a directory", c)
		}
	}
	// Scenes and single-object models stay a single merged file -- notably
	// COMMON/TRAIN, whose cars sit at distinct positions.
	for _, s := range []string{"COMMON/TRAIN.glb", "COMMON/VECTO.glb", "TRACK01/SCENE.glb", "TRACK01/SKY.glb"} {
		if !isFile(s) {
			t.Errorf("expected %s to be a single merged file", s)
		}
	}
}
