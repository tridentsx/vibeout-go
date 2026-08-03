package game

import "testing"

func TestUpdateSteeringDigitalRampsUpWhenLeftHeld(t *testing.T) {
	s := &Ship{TurnAccel: 5, TurnRate: 100, SteeringRate: 0}
	UpdateSteeringDigital(s, true, false)
	if s.SteeringRate != 5 {
		t.Errorf("expected SteeringRate to ramp up (left, positive) by TurnAccel, got %v", s.SteeringRate)
	}
}

func TestUpdateSteeringDigitalRampsDownWhenRightHeld(t *testing.T) {
	s := &Ship{TurnAccel: 5, TurnRate: 100, SteeringRate: 0}
	UpdateSteeringDigital(s, false, true)
	if s.SteeringRate != -5 {
		t.Errorf("expected SteeringRate to ramp down (right, negative) by TurnAccel, got %v", s.SteeringRate)
	}
}

func TestUpdateSteeringDigitalLeftDoublesRateCrossingFromRight(t *testing.T) {
	s := &Ship{TurnAccel: 5, TurnRate: 100, SteeringRate: -3}
	UpdateSteeringDigital(s, true, false)
	if s.SteeringRate != 7 { // -3 + 5*2
		t.Errorf("expected doubled rate when crossing from right, got %v", s.SteeringRate)
	}
}

func TestUpdateSteeringDigitalRightDoublesRateCrossingFromLeft(t *testing.T) {
	s := &Ship{TurnAccel: 5, TurnRate: 100, SteeringRate: 3}
	UpdateSteeringDigital(s, false, true)
	if s.SteeringRate != -7 { // 3 - 5*2
		t.Errorf("expected doubled rate when crossing from left, got %v", s.SteeringRate)
	}
}

func TestUpdateSteeringDigitalLeftTakesPriorityWhenBothHeld(t *testing.T) {
	s := &Ship{TurnAccel: 5, TurnRate: 100, SteeringRate: 0}
	UpdateSteeringDigital(s, true, true)
	if s.SteeringRate != 5 {
		t.Errorf("expected left to take priority when both held, got %v", s.SteeringRate)
	}
}

func TestUpdateSteeringDigitalDecaysWhenNeitherHeld(t *testing.T) {
	s := &Ship{TurnAccel: 5, TurnRate: 100, SteeringRate: 12}
	UpdateSteeringDigital(s, false, false)
	if s.SteeringRate != 7 {
		t.Errorf("expected SteeringRate to decay by TurnAccel, got %v", s.SteeringRate)
	}
}

func TestUpdateSteeringDigitalDecaysTowardZeroFromNegative(t *testing.T) {
	s := &Ship{TurnAccel: 5, TurnRate: 100, SteeringRate: -12}
	UpdateSteeringDigital(s, false, false)
	if s.SteeringRate != -7 {
		t.Errorf("expected SteeringRate to decay toward zero from negative, got %v", s.SteeringRate)
	}
}

func TestUpdateSteeringDigitalStaysAtZeroWhenNeitherHeld(t *testing.T) {
	s := &Ship{TurnAccel: 5, TurnRate: 100, SteeringRate: 0}
	UpdateSteeringDigital(s, false, false)
	if s.SteeringRate != 0 {
		t.Errorf("expected SteeringRate to remain 0, got %v", s.SteeringRate)
	}
}

func TestUpdateSteeringDigitalClampsToTurnRate(t *testing.T) {
	s := &Ship{TurnAccel: 5, TurnRate: 10, SteeringRate: 9}
	UpdateSteeringDigital(s, true, false)
	if s.SteeringRate != 10 {
		t.Errorf("expected SteeringRate clamped to TurnRate 10, got %v", s.SteeringRate)
	}
}

func TestUpdateSteeringDigitalClampsToNegativeTurnRate(t *testing.T) {
	s := &Ship{TurnAccel: 5, TurnRate: 10, SteeringRate: -9}
	UpdateSteeringDigital(s, false, true)
	if s.SteeringRate != -10 {
		t.Errorf("expected SteeringRate clamped to -TurnRate -10, got %v", s.SteeringRate)
	}
}

// UpdateSteeringTwist tests use the live-confirmed default calibration
// (session 9): TwistMargin=6, TwistDivisor=255, center=128.

func TestUpdateSteeringTwistLeftIsPositive(t *testing.T) {
	// Matches the session 9 live test: twist byte 13 (well left of center)
	// produced a strongly positive SteeringRate in the real game.
	s := &Ship{TurnAccel: 3144, TurnRate: 3384, TwistMargin: 6, TwistDivisor: 255}
	UpdateSteeringTwist(s, 13)
	if s.SteeringRate <= 0 {
		t.Errorf("expected positive (left) SteeringRate for a low twist byte, got %v", s.SteeringRate)
	}
}

func TestUpdateSteeringTwistRightIsNegative(t *testing.T) {
	// Matches the session 9 live test: twist byte 243 (well right of
	// center) produced a strongly negative SteeringRate in the real game.
	s := &Ship{TurnAccel: 3144, TurnRate: 3384, TwistMargin: 6, TwistDivisor: 255}
	UpdateSteeringTwist(s, 243)
	if s.SteeringRate >= 0 {
		t.Errorf("expected negative (right) SteeringRate for a high twist byte, got %v", s.SteeringRate)
	}
}

func TestUpdateSteeringTwistCenterStaysAtZero(t *testing.T) {
	s := &Ship{TurnAccel: 100, TurnRate: 3384, TwistMargin: 6, TwistDivisor: 255}
	UpdateSteeringTwist(s, 128)
	if s.SteeringRate != 0 {
		t.Errorf("expected SteeringRate to stay 0 at centered twist, got %v", s.SteeringRate)
	}
}

func TestUpdateSteeringTwistWithinMarginStaysAtZero(t *testing.T) {
	// 128-3=125, within TwistMargin=6 of center -- deadzone should hold at 0.
	s := &Ship{TurnAccel: 100, TurnRate: 3384, TwistMargin: 6, TwistDivisor: 255}
	UpdateSteeringTwist(s, 125)
	if s.SteeringRate != 0 {
		t.Errorf("expected SteeringRate to stay 0 within the deadzone margin, got %v", s.SteeringRate)
	}
}

func TestUpdateSteeringTwistClampsToTurnRate(t *testing.T) {
	s := &Ship{TurnAccel: 10000, TurnRate: 50, TwistMargin: 6, TwistDivisor: 255}
	UpdateSteeringTwist(s, 0) // maximum left deflection
	if s.SteeringRate != 50 {
		t.Errorf("expected SteeringRate clamped to TurnRate 50, got %v", s.SteeringRate)
	}
}

func TestIntegrateYawFromSteeringPositive(t *testing.T) {
	s := &Ship{Yaw: 0, SteeringRate: 640}
	IntegrateYawFromSteering(s)
	if s.Yaw != 10 { // 640/64
		t.Errorf("expected Yaw to advance by SteeringRate/64, got %v", s.Yaw)
	}
}

func TestIntegrateYawFromSteeringNegativeWraps(t *testing.T) {
	// Angle stores its raw uint16 value and only normalizes to the 0-4095
	// range on demand via Wrapped() (see angle.go) -- since 65536 is an
	// exact multiple of AngleFullTurn (4096), a raw uint16 underflow is
	// still numerically equivalent to the wrapped angle for Sin/Cos
	// purposes, it just isn't in canonical [0,4095) form until Wrapped()
	// is called.
	s := &Ship{Yaw: 5, SteeringRate: -640}
	IntegrateYawFromSteering(s)
	if s.Yaw.Wrapped() != AngleFullTurn-5 {
		t.Errorf("expected Yaw to wrap around correctly, got %v (wrapped %v)", s.Yaw, s.Yaw.Wrapped())
	}
}

func TestUpdatePitchInputRampsUpWhenDownHeld(t *testing.T) {
	s := &Ship{}
	UpdatePitchInput(s, true, false)
	if s.PitchRate != 36 {
		t.Errorf("PitchRate = %v, want 36", s.PitchRate)
	}
}

func TestUpdatePitchInputRampsDownWhenUpHeld(t *testing.T) {
	s := &Ship{}
	UpdatePitchInput(s, false, true)
	if s.PitchRate != -36 {
		t.Errorf("PitchRate = %v, want -36", s.PitchRate)
	}
}

func TestUpdatePitchInputDownTakesPriorityWhenBothHeld(t *testing.T) {
	s := &Ship{}
	UpdatePitchInput(s, true, true)
	if s.PitchRate != 36 {
		t.Errorf("PitchRate = %v, want 36 (down priority)", s.PitchRate)
	}
}

func TestUpdatePitchInputUnchangedWhenNeitherHeld(t *testing.T) {
	s := &Ship{PitchRate: 100}
	UpdatePitchInput(s, false, false)
	if s.PitchRate != 100 {
		t.Errorf("PitchRate = %v, want unchanged at 100 (decay happens in IntegratePitchAndRoll, not here)", s.PitchRate)
	}
}

func TestIntegratePitchAndRollDecaysPitchRate(t *testing.T) {
	s := &Ship{PitchRate: 100}
	IntegratePitchAndRoll(s)
	// (100-60) - (100-60)/4 = 40 - 10 = 30
	if s.PitchRate != 30 {
		t.Errorf("PitchRate = %v, want 30", s.PitchRate)
	}
}

func TestIntegratePitchAndRollFeedsPitch(t *testing.T) {
	s := &Ship{PitchRate: 100, Pitch: 0}
	IntegratePitchAndRoll(s)
	// Pitch += PitchRate/16 using the *decayed* PitchRate (30): 30/16 = 1 (int32 truncation)
	if s.Pitch != 1 {
		t.Errorf("Pitch = %v, want 1", s.Pitch)
	}
}

func TestIntegratePitchAndRollRollRateFedBySteeringRate(t *testing.T) {
	s := &Ship{SteeringRate: 640}
	IntegratePitchAndRoll(s)
	// rollRate = 640/32 = 20, then -= 20/2 = 10 -> rollRate = 10
	if s.RollRate != 10 {
		t.Errorf("RollRate = %v, want 10", s.RollRate)
	}
}

func TestIntegratePitchAndRollRollRateZeroWithoutSteering(t *testing.T) {
	s := &Ship{SteeringRate: 0}
	IntegratePitchAndRoll(s)
	if s.RollRate != 0 {
		t.Errorf("RollRate = %v, want 0 with no steering input", s.RollRate)
	}
}

func TestIntegratePitchAndRollIntegratesRoll(t *testing.T) {
	s := &Ship{SteeringRate: 640, Roll: 0}
	IntegratePitchAndRoll(s)
	// rollRate=10 (see above), rollSum = 0+10 = 10, roll = 10 - 10/8 = 10-1 = 9
	if s.Roll != 9 {
		t.Errorf("Roll = %v, want 9", s.Roll)
	}
}
