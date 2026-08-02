package game

import "testing"

func TestUpdatePhysicsDecaysAndIntegrates(t *testing.T) {
	s := &Ship{Velocity: Vector3{X: 320, Y: 0, Z: 0}}

	UpdatePhysics(s)

	wantVelX := float32(320 * 15.0 / 16.0)
	if s.Velocity.X != wantVelX {
		t.Errorf("Velocity.X = %v, want %v", s.Velocity.X, wantVelX)
	}
	wantPosX := wantVelX / 32.0
	if s.Position.X != wantPosX {
		t.Errorf("Position.X = %v, want %v", s.Position.X, wantPosX)
	}
}

func TestUpdatePhysicsDecaysTowardZero(t *testing.T) {
	s := &Ship{Velocity: Vector3{X: 1000}}
	for i := 0; i < 500; i++ {
		UpdatePhysics(s)
	}
	if s.Velocity.X > 1 {
		t.Errorf("Velocity.X after 500 frames = %v, expected near zero", s.Velocity.X)
	}
}
