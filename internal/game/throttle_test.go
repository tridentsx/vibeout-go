package game

import "testing"

func TestUpdateThrottleAnalogRampsUpTowardTarget(t *testing.T) {
	s := &Ship{MaxSpeed: 100, Speed: 0}
	UpdateThrottle(s, true, 200, false) // full throttle -> target == MaxSpeed
	if s.Speed != 19 {
		t.Errorf("expected Speed to ramp up by the 19-unit step, got %v", s.Speed)
	}
}

func TestUpdateThrottleAnalogClampsToMaxSpeed(t *testing.T) {
	s := &Ship{MaxSpeed: 100, Speed: 95}
	UpdateThrottle(s, true, 200, false)
	if s.Speed != 100 {
		t.Errorf("expected Speed clamped to MaxSpeed 100, got %v", s.Speed)
	}
}

func TestUpdateThrottleAnalogZeroThrottleDecelerates(t *testing.T) {
	s := &Ship{MaxSpeed: 100, Speed: 50}
	UpdateThrottle(s, true, 0, false)
	if s.Speed != 31 {
		t.Errorf("expected Speed to ramp down by 19, got %v", s.Speed)
	}
}

func TestUpdateThrottleDigitalAccelerate(t *testing.T) {
	s := &Ship{MaxSpeed: 100, Speed: 0}
	UpdateThrottle(s, false, 0, true)
	if s.Speed != 19 {
		t.Errorf("expected digital accelerate to ramp up by 19, got %v", s.Speed)
	}
}

func TestUpdateThrottleDigitalDecelerate(t *testing.T) {
	s := &Ship{MaxSpeed: 100, Speed: 50}
	UpdateThrottle(s, false, 0, false)
	if s.Speed != 31 {
		t.Errorf("expected digital decelerate to ramp down by 19, got %v", s.Speed)
	}
}
