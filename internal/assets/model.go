package assets

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tridentsx/wipeout-go/internal/psx"
)

// Model is a decoded PRM together with its resolved texture pages, taken from
// the same-basename CMP (confirmed against the disc: FOO.PRM's textures live in
// FOO.CMP, indexed directly by each polygon's Texture field). Pages is nil for
// untextured models; an individual entry is nil when a CMP member is not a
// decodable TIM. Warnings records a missing or unreadable texture bundle
// without failing the geometry load.
type Model struct {
	Name     string
	Objects  []psx.Object
	Pages    []*psx.Image
	Warnings []error
}

// LoadModel loads a PRM relative to the disc root and, when the model uses
// textured or sprite polygons, decodes its paired <base>.CMP texture pages.
func (l Loader) LoadModel(parts ...string) (*Model, error) {
	data, err := l.read(parts...)
	if err != nil {
		return nil, err
	}
	objects, err := psx.DecodePRM(data)
	if err != nil {
		return nil, fmt.Errorf("assets: decode %s: %w", filepath.Join(parts...), err)
	}
	model := &Model{Name: modelBaseName(parts...), Objects: objects}
	if !objectsUseTextures(objects) {
		return model, nil
	}

	cmpParts := replaceExtension(parts, ".CMP")
	cmpData, cmpErr := l.read(cmpParts...)
	if cmpErr != nil {
		model.Warnings = append(model.Warnings,
			fmt.Errorf("assets: textured model %s has no readable texture bundle: %w", filepath.Join(parts...), cmpErr))
		return model, nil
	}
	members, decErr := psx.DecodeCMP(cmpData)
	if decErr != nil {
		model.Warnings = append(model.Warnings, fmt.Errorf("assets: %s: %w", filepath.Join(cmpParts...), decErr))
		return model, nil
	}
	model.Pages = make([]*psx.Image, len(members))
	for i, member := range members {
		if img, imgErr := psx.DecodeTIM(member); imgErr == nil {
			model.Pages[i] = img
		}
	}
	return model, nil
}

func objectsUseTextures(objects []psx.Object) bool {
	for i := range objects {
		for j := range objects[i].Polygons {
			if objects[i].Polygons[j].Texture != nil {
				return true
			}
		}
	}
	return false
}

// replaceExtension returns a copy of parts with the final element's extension
// replaced, without mutating the caller's slice.
func replaceExtension(parts []string, ext string) []string {
	out := append([]string(nil), parts...)
	last := len(out) - 1
	out[last] = strings.TrimSuffix(out[last], filepath.Ext(out[last])) + ext
	return out
}

func modelBaseName(parts ...string) string {
	base := filepath.Base(filepath.Join(parts...))
	return strings.TrimSuffix(base, filepath.Ext(base))
}
