package render

import (
	"testing"

	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/game"
)

// Every model the menu references must load and contain the object it names, or a
// selection row silently shows nothing.
func TestMenuModelsResolve(t *testing.T) {
	loader := assets.Loader{Root: "/home/epkcfsm/src/vibeout-go/assets/WIPEOUT2"}
	type want struct{ file, object string }
	var wants []want
	for _, m := range game.ClassModels {
		wants = append(wants, want{m.File, m.Object})
	}
	for _, team := range game.TeamEntries {
		wants = append(wants, want{"TERRY.PRM", team.Object})
	}
	for _, team := range game.AnimalTeamEntries {
		wants = append(wants, want{"WIERD.PRM", team.Object})
	}
	for _, obj := range game.TrackPreviewObjects {
		if obj != "" {
			wants = append(wants, want{"JUNE.PRM", obj})
		}
	}
	cache := map[string]*assets.Model{}
	for _, w := range wants {
		model := cache[w.file]
		if model == nil {
			m, err := loader.LoadModel("COMMON", w.file)
			if err != nil {
				t.Skipf("%s unavailable: %v", w.file, err)
			}
			cache[w.file] = m
			model = m
		}
		obj := FindObject(model.Objects, w.object)
		if obj == nil {
			names := []string{}
			for _, o := range model.Objects {
				names = append(names, o.Header.Name)
			}
			t.Errorf("%s has no object %q; it has %v", w.file, w.object, names)
			continue
		}
		if len(obj.Polygons) == 0 || len(obj.Vertices) == 0 {
			t.Errorf("%s/%s is empty", w.file, w.object)
		}
	}
	t.Logf("resolved %d menu models across %d files", len(wants), len(cache))
}
