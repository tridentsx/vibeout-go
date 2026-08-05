package trackpack

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
)

// Load reads a .trackpack directory's track.json into a Pack and records the
// directory so TilePath/LoadTile/SceneryPath/SkyPath can resolve files.
func Load(dir string) (*Pack, error) {
	f, err := os.Open(filepath.Join(dir, "track.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	p, err := Decode(f)
	if err != nil {
		return nil, err
	}
	p.dir = dir
	return p, nil
}

// Decode parses a track.json document. It does not resolve external files;
// use Load when file access is needed.
func Decode(r io.Reader) (*Pack, error) {
	var p Pack
	if err := json.NewDecoder(r).Decode(&p); err != nil {
		return nil, fmt.Errorf("trackpack: decode track.json: %w", err)
	}
	if p.FormatVersion != 1 {
		return nil, fmt.Errorf("trackpack: unsupported formatVersion %d (want 1)", p.FormatVersion)
	}
	return &p, nil
}

// Dir returns the pack's base directory (empty if the Pack was produced by
// Decode rather than Load).
func (p *Pack) Dir() string { return p.dir }

// TilePath returns the absolute path to the surface tile texture for the given
// logical tile index, resolving against the pack directory.
func (p *Pack) TilePath(index int) (string, bool) {
	for _, t := range p.Textures.Surface {
		if t.Tile == index {
			return filepath.Join(p.dir, filepath.FromSlash(t.File)), true
		}
	}
	return "", false
}

// LoadTile decodes the surface tile PNG for the given logical tile index.
func (p *Pack) LoadTile(index int) (image.Image, error) {
	path, ok := p.TilePath(index)
	if !ok {
		return nil, fmt.Errorf("trackpack: no surface tile %d", index)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

// SceneryPath returns the absolute path to the baked scenery mesh, if present.
func (p *Pack) SceneryPath() (string, bool) { return p.layerPath(p.Layers.Scenery) }

// SkyPath returns the absolute path to the baked sky mesh, if present.
func (p *Pack) SkyPath() (string, bool) { return p.layerPath(p.Layers.Sky) }

func (p *Pack) layerPath(ref *LayerRef) (string, bool) {
	if ref == nil || ref.File == "" {
		return "", false
	}
	return filepath.Join(p.dir, filepath.FromSlash(ref.File)), true
}
