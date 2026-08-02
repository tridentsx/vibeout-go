package game

import "testing"

func TestDistance(t *testing.T) {
	a := Vector3{X: 0, Y: 0, Z: 0}
	b := Vector3{X: 3, Y: 4, Z: 0}
	if got := Distance(a, b); got != 5 {
		t.Errorf("Distance() = %v, want 5", got)
	}
}

func TestTooClose(t *testing.T) {
	a := &Ship{Position: Vector3{X: 0, Y: 0, Z: 0}}
	b := &Ship{Position: Vector3{X: 3, Y: 4, Z: 0}}

	if !TooClose(a, b, 10) {
		t.Error("expected ships 5 apart to be within radius 10")
	}
	if TooClose(a, b, 4) {
		t.Error("expected ships 5 apart to NOT be within radius 4")
	}
}
