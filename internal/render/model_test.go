package render

import (
	"testing"

	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/game"
)

func TestFindObject(t *testing.T) {
	objects := []assets.Object{{Header: assets.ObjectHeader{Name: "quirex1"}}, {Header: assets.ObjectHeader{Name: "fiesar1"}}}
	if object := FindObject(objects, "fiesar1"); object == nil || object.Header.Name != "fiesar1" {
		t.Fatalf("FindObject = %+v", object)
	}
}

func TestShipUpVectorAtIdentity(t *testing.T) {
	up := cross(game.Vector3{Z: 1}, game.Vector3{X: 1})
	if up != (game.Vector3{Y: 1}) {
		t.Fatalf("up = %+v", up)
	}
}

func TestPRMColorUsesPSXOneAt128(t *testing.T) {
	color := polygonColor(assets.Polygon{Color: &assets.Color{R: 128, G: 64, B: 255}})
	if color.R != 1 || color.G != 0.5 || color.B != 1 || color.A != 1 {
		t.Fatalf("color = %+v", color)
	}
}
