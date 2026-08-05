package assets

import (
	"os"
	"path/filepath"
	"testing"
)

func discRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join("..", "..", "assets", "WIPEOUT2")
	if _, err := os.Stat(root); err != nil {
		t.Skip(err)
	}
	return root
}

func TestLoadModelUntexturedCraft(t *testing.T) {
	loader := Loader{Root: discRoot(t)}
	model, err := loader.LoadModel("COMMON", "VECTO.PRM")
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Objects) == 0 {
		t.Fatal("no objects")
	}
	if model.Pages != nil {
		t.Fatalf("untextured craft should have no pages, got %d", len(model.Pages))
	}
	if len(model.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", model.Warnings)
	}
}

func TestLoadModelTexturedCraftResolvesPages(t *testing.T) {
	loader := Loader{Root: discRoot(t)}
	model, err := loader.LoadModel("COMMON", "TERRY.PRM")
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", model.Warnings)
	}
	if len(model.Pages) == 0 {
		t.Fatal("textured craft resolved no pages")
	}
	decoded := 0
	for _, p := range model.Pages {
		if p != nil {
			decoded++
		}
	}
	if decoded == 0 {
		t.Fatal("no CMP member decoded as a TIM page")
	}
	// Every texture index used by the model must be within the page array.
	for _, o := range model.Objects {
		for j := range o.Polygons {
			if tex := o.Polygons[j].Texture; tex != nil && int(*tex) >= len(model.Pages) {
				t.Fatalf("texture index %d out of range (%d pages)", *tex, len(model.Pages))
			}
		}
	}
}
