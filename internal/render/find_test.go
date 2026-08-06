package render

import (
	"testing"

	"github.com/tridentsx/wipeout-go/internal/assets"
)

// Every object the port asks for by name must actually resolve, or it silently draws
// nothing. The maintenance craft was invisible and this is the first thing to rule out.
func TestNamedObjectsResolve(t *testing.T) {
	loader := assets.Loader{Root: "/home/epkcfsm/src/vibeout-go/assets/WIPEOUT2"}
	for _, tc := range []struct{ file, object string }{
		{"RESCU.PRM", "grid2"},
		{"LIGHT.PRM", "light1"},
		{"TERRY.PRM", "fiesar1"},
	} {
		model, err := loader.LoadModel("COMMON", tc.file)
		if err != nil {
			t.Skipf("%s unavailable: %v", tc.file, err)
		}
		if obj := FindObject(model.Objects, tc.object); obj == nil {
			names := []string{}
			for _, o := range model.Objects {
				names = append(names, o.Header.Name)
			}
			t.Errorf("%s has no %q; it has %q", tc.file, tc.object, names)
			continue
		}
		t.Logf("%-12s %-10s resolved, %d texture page(s)", tc.file, tc.object, len(model.Pages))
	}
}
