package psx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRealTRVTRFFilesParseCleanly(t *testing.T) {
	if _, err := os.Stat(wipeoutDiscPath); err != nil {
		t.Skip("disc image not present:", err)
	}

	trvPath := filepath.Join(wipeoutDiscPath, "TRACK01", "TRACK.TRV")
	trvData, err := os.ReadFile(trvPath)
	if err != nil {
		t.Fatal(err)
	}
	vertices, err := DecodeTRV(trvData)
	if err != nil {
		t.Fatal(err)
	}
	if len(vertices) == 0 {
		t.Fatal("decoded zero vertices")
	}

	trfPath := filepath.Join(wipeoutDiscPath, "TRACK01", "TRACK.TRF")
	trfData, err := os.ReadFile(trfPath)
	if err != nil {
		t.Fatal(err)
	}
	faces, err := DecodeTRF(trfData)
	if err != nil {
		t.Fatal(err)
	}
	if len(faces) == 0 {
		t.Fatal("decoded zero faces")
	}

	// Every face's vertex indices must point inside the real vertex table --
	// the strongest cross-check that trackVertexSize/trackFaceSize (and the
	// byte layout ported from wipeout.js) are actually correct, not just
	// "divides the file size evenly".
	for i, f := range faces {
		for _, idx := range f.Indices {
			if int(idx) >= len(vertices) {
				t.Fatalf("face %d: vertex index %d out of range (only %d vertices)", i, idx, len(vertices))
			}
		}
	}

	t.Logf("TRACK01: %d vertices, %d faces", len(vertices), len(faces))

	trsPath := filepath.Join(wipeoutDiscPath, "TRACK01", "TRACK.TRS")
	trsData, err := os.ReadFile(trsPath)
	if err != nil {
		t.Fatal(err)
	}
	sections, err := DecodeTRS(trsData)
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) == 0 {
		t.Fatal("decoded zero sections")
	}

	// Every section's FirstFace/NumFaces range, and every non-sentinel
	// Previous/Next/NextJunction link, must point inside the real face and
	// section tables -- the same cross-check strategy as the TRV/TRF
	// validation above, now for the section graph.
	for i, s := range sections {
		if int(s.FirstFace)+int(s.NumFaces) > len(faces) {
			t.Fatalf("section %d: face range [%d:%d) exceeds %d faces", i, s.FirstFace, s.FirstFace+uint32(s.NumFaces), len(faces))
		}
		for name, link := range map[string]int32{"Previous": s.Previous, "Next": s.Next, "NextJunction": s.NextJunction} {
			if link != -1 && (link < 0 || int(link) >= len(sections)) {
				t.Fatalf("section %d: %s link %d out of range (only %d sections)", i, name, link, len(sections))
			}
		}
	}

	t.Logf("TRACK01: %d sections", len(sections))
}
