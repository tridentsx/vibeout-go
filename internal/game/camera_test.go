package game

import "testing"

func TestNewChaseCameraFollowsBehindShip(t *testing.T) {
	ship := &Ship{Position: Vector3{X: 0, Y: 0, Z: 0}, Yaw: 0} // facing +Z
	cam := NewChaseCamera(ship)

	if cam.Position.Z >= 0 {
		t.Errorf("expected camera behind ship (negative Z) when ship faces +Z, got Z=%v", cam.Position.Z)
	}
	if cam.Position.Y <= 0 {
		t.Errorf("expected camera above ship (positive Y), got Y=%v", cam.Position.Y)
	}
}

func TestProjectTopDownShipAheadIsPositiveZ(t *testing.T) {
	cam := Camera{Position: Vector3{X: 0, Y: 0, Z: -40}, Yaw: 0}
	_, z := cam.ProjectTopDown(Vector3{X: 0, Y: 0, Z: 0})
	if z <= 0 {
		t.Errorf("expected a point ahead of the camera to project to positive Z, got %v", z)
	}
}
