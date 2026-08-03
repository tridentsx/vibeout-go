package controller

import (
	"errors"
	"testing"
)

func TestDefaultsAndFixedControls(t *testing.T) {
	m := DefaultMapping()
	if m.Button(Accelerate) != Cross || m.Button(FireWeapon) != Square || m.Button(Pause) != Start {
		t.Fatal("unexpected default PS1 layout")
	}
	if err := m.Assign(Pause, Circle); !errors.Is(err, ErrFixedAction) {
		t.Fatalf("Assign fixed action = %v", err)
	}
	if err := m.Assign(Accelerate, Triangle); !errors.Is(err, ErrInvalidButton) {
		t.Fatalf("Assign reserved change-view button = %v", err)
	}
}

func TestAssignSwapsConflictingRemappableButtons(t *testing.T) {
	m := DefaultMapping()
	if err := m.Assign(Accelerate, Square); err != nil {
		t.Fatal(err)
	}
	if m.Button(Accelerate) != Square || m.Button(FireWeapon) != Cross {
		t.Fatalf("mapping did not swap: accelerate=%s fire=%s", m.Button(Accelerate), m.Button(FireWeapon))
	}
}

func TestResolveEdgesAndDigitalSteering(t *testing.T) {
	m := DefaultMapping()
	var previous, now Buttons
	now.Set(Cross, true)
	now.Set(DPadLeft, true)
	s := m.Resolve(now, previous, .25)
	if !s.IsDown(Accelerate) || !s.WasPressed(Accelerate) || s.Steer != -1 {
		t.Fatalf("state = %+v", s)
	}
	previous = now
	s = m.Resolve(now, previous, .25)
	if s.WasPressed(Accelerate) {
		t.Fatal("held control reported a second press")
	}
}

func TestEditorFlow(t *testing.T) {
	e := NewEditor(DefaultMapping())
	e.Move(1)
	if e.Selected() != FireWeapon {
		t.Fatal("cursor did not move")
	}
	e.BeginAssign()
	if e.Capture(DPadUp) || !e.Waiting {
		t.Fatal("invalid capture should be ignored")
	}
	if !e.Capture(Circle) || e.Waiting || e.Mapping.Button(FireWeapon) != Circle {
		t.Fatal("valid capture failed")
	}
}

func TestNormalizeAxisDeadzone(t *testing.T) {
	if normalizedAxis(5000, 6000) != 0 || normalizedAxis(-5000, 6000) != 0 {
		t.Fatal("deadzone failed")
	}
	if got := normalizedAxis(32767, 6000); got != 1 {
		t.Fatalf("positive max = %v", got)
	}
	if got := normalizedAxis(-32768, 6000); got != -1 {
		t.Fatalf("negative max = %v", got)
	}
}
