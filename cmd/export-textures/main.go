// Command export-textures extracts every original WipEout 2097 texture to PNG,
// keyed by a stable identity (source file + member index), as the source of
// truth for upscaling ("upres").
//
//	go run ./cmd/export-textures                 # -> export-textures/ + manifest.json
//
// It deliberately does NOT bake tiled surfaces (the track road) into meshes.
// The game's tile-index and per-face gameplay-flag/trigger logic stays in code;
// only the texture pixels are replaced by hi-res versions keyed the same way.
// For the road that means TRACK*/LIBRARY.CMP tiles come out one PNG per tile
// index (TRACK01/LIBRARY/000.png ...), exactly the index the .TTF and TRF faces
// reference, so an upres'd tile drops straight back into the tile system.
package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tridentsx/wipeout-go/internal/psx"
)

// entry is one extracted texture's manifest record. Source+Index is the stable
// identity an upres pass and the runtime override loader key on.
type entry struct {
	Source string `json:"source"` // disc-relative source, e.g. "TRACK01/LIBRARY.CMP"
	Index  int    `json:"index"`  // member index within a CMP, or -1 for a standalone TIM
	Width  int    `json:"width"`
	Height int    `json:"height"`
	SHA1   string `json:"sha1"` // of the decoded RGBA pixels, for dedup
	File   string `json:"file"` // output PNG, relative to the output dir
}

func main() {
	disc := flag.String("disc", "assets/WIPEOUT2", "extracted WipEout 2097 disc root")
	out := flag.String("out", "export-textures", "output directory for PNGs + manifest.json")
	flag.Parse()

	total, unique, err := exportTextures(*disc, *out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "export-textures:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "extracted %d texture(s) (%d unique by pixels) to %s\n", total, unique, *out)
}

// exportTextures walks the disc, extracts every CMP member and standalone TIM
// to PNG, and writes a manifest. It returns the total and unique-by-pixels
// texture counts.
func exportTextures(disc, outDir string) (int, int, error) {
	var entries []entry
	err := filepath.WalkDir(disc, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := filepath.ToSlash(mustRel(disc, path))
		switch strings.ToUpper(filepath.Ext(path)) {
		case ".CMP":
			es, e := extractCMP(path, rel, outDir)
			if e != nil {
				return fmt.Errorf("%s: %w", rel, e)
			}
			entries = append(entries, es...)
		case ".TIM":
			es, e := extractTIM(path, rel, outDir)
			if e != nil {
				return fmt.Errorf("%s: %w", rel, e)
			}
			entries = append(entries, es...)
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Source != entries[j].Source {
			return entries[i].Source < entries[j].Source
		}
		return entries[i].Index < entries[j].Index
	})

	unique := map[string]struct{}{}
	for _, e := range entries {
		unique[e.SHA1] = struct{}{}
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return 0, 0, err
	}
	manifest, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return 0, 0, err
	}
	if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), manifest, 0o644); err != nil {
		return 0, 0, err
	}
	return len(entries), len(unique), nil
}

// extractCMP writes each decodable TIM member of a CMP bundle. Non-image
// members (e.g. font data) are skipped.
func extractCMP(path, rel, outDir string) ([]entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	members, err := psx.DecodeCMP(data)
	if err != nil {
		return nil, err
	}
	dir := strings.TrimSuffix(rel, filepath.Ext(rel)) // e.g. TRACK01/LIBRARY
	var out []entry
	for i, member := range members {
		img, e := psx.DecodeTIM(member)
		if e != nil {
			continue // not an image member
		}
		file := filepath.ToSlash(filepath.Join(dir, fmt.Sprintf("%03d.png", i)))
		if err := writePNG(filepath.Join(outDir, filepath.FromSlash(file)), img); err != nil {
			return out, err
		}
		out = append(out, entry{Source: rel, Index: i, Width: img.Width, Height: img.Height, SHA1: pixelHash(img), File: file})
	}
	return out, nil
}

// extractTIM writes a standalone .TIM file.
func extractTIM(path, rel, outDir string) ([]entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	img, err := psx.DecodeTIM(data)
	if err != nil {
		return nil, err
	}
	file := filepath.ToSlash(strings.TrimSuffix(rel, filepath.Ext(rel)) + ".png")
	if err := writePNG(filepath.Join(outDir, filepath.FromSlash(file)), img); err != nil {
		return nil, err
	}
	return []entry{{Source: rel, Index: -1, Width: img.Width, Height: img.Height, SHA1: pixelHash(img), File: file}}, nil
}

func writePNG(path string, img *psx.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	nrgba := &image.NRGBA{Pix: img.Pixels, Stride: img.Width * 4, Rect: image.Rect(0, 0, img.Width, img.Height)}
	return png.Encode(f, nrgba)
}

func pixelHash(img *psx.Image) string {
	sum := sha1.Sum(img.Pixels)
	return hex.EncodeToString(sum[:])
}

func mustRel(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}
