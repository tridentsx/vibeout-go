package game

import "testing"

// The phase boundaries are the unsigned range tests at 0x8004a040 and 0x8004a05c:
// red is (cd-42) < 42 and yellow is (cd-1) < 41.
func TestStartLightPhaseBoundaries(t *testing.T) {
	for _, tc := range []struct {
		countdown int
		want      StartLightPhase
	}{
		{CountdownStart, StartLightNeutral}, // 166, before the lights engage
		{100, StartLightNeutral},            // the camera handover
		{84, StartLightNeutral},             // one tick before red
		{83, StartLightRed},
		{42, StartLightRed},
		{41, StartLightYellow},
		{1, StartLightYellow},
		{0, StartLightGreen},
	} {
		s := &StartLightState{Countdown: tc.countdown}
		if got := s.Phase(); got != tc.want {
			t.Errorf("countdown %d gave phase %d, want %d", tc.countdown, got, tc.want)
		}
	}
}

// Red runs 42 ticks and yellow 41, which at 25 Hz is about 1.7 seconds each.
func TestStartLightPhaseDurations(t *testing.T) {
	red, yellow := 0, 0
	for cd := CountdownStart; cd >= 0; cd-- {
		switch (&StartLightState{Countdown: cd}).Phase() {
		case StartLightRed:
			red++
		case StartLightYellow:
			yellow++
		}
	}
	if red != 42 {
		t.Errorf("red lasts %d ticks, want 42", red)
	}
	if yellow != 41 {
		t.Errorf("yellow lasts %d ticks, want 41", yellow)
	}
}

// Each phase paints all eight lamps one colour; retail's yellow is an amber, red with
// half green rather than full.
func TestStartLightColors(t *testing.T) {
	red := (&StartLightState{Countdown: 60}).Colors()
	for i, c := range red {
		if c != (StartLightRGB{R: 0xff}) {
			t.Errorf("red lamp %d is %+v", i, c)
		}
	}
	yellow := (&StartLightState{Countdown: 20}).Colors()
	if yellow[0] != (StartLightRGB{R: 0xff, G: 0x80}) {
		t.Errorf("yellow is %+v, want R 0xff G 0x80", yellow[0])
	}
	green := (&StartLightState{Countdown: 0}).Colors()
	if green[0] != (StartLightRGB{G: 0xff}) {
		t.Errorf("green is %+v", green[0])
	}
	neutral := (&StartLightState{Countdown: 150}).Colors()
	if neutral[0] != NeutralStartLightRGB {
		t.Errorf("neutral is %+v, want the 0x80 grey", neutral[0])
	}
}

// The sweep lights exactly one lamp fully and gives the rest an index-scaled
// gradient, and the lit lamp advances one position per tick.
func TestStartLightSweep(t *testing.T) {
	s := &StartLightState{Countdown: 0, Counter: SweepDelayCount}
	if s.Phase() != StartLightSweep {
		t.Fatal("counter at the threshold must select the sweep")
	}
	for frame := 0; frame < StartLightCount*2; frame++ {
		s.Frame = frame
		colors := s.Colors()
		bright := -1
		for i, c := range colors {
			if c.G == 0xff {
				if bright >= 0 {
					t.Fatalf("frame %d has two fully lit lamps", frame)
				}
				bright = i
			} else if c.G != uint8(i<<4) {
				t.Errorf("frame %d lamp %d gradient is %d, want %d", frame, i, c.G, i<<4)
			}
			if c.R != 0 || c.B != 0 {
				t.Errorf("frame %d lamp %d is not pure green: %+v", frame, i, c)
			}
		}
		if bright != frame%StartLightCount {
			t.Errorf("frame %d lit lamp %d, want %d", frame, bright, frame%StartLightCount)
		}
	}
}

// The sweep begins shortly after green, not immediately: retail's counter needs 400
// increments and it gains 24 per tick across three gantries of eight lamps.
func TestSweepFollowsGreen(t *testing.T) {
	s := NewStartLightState()
	greenTick, sweepTick := -1, -1
	for tick := 0; tick < 400; tick++ {
		s.Tick()
		if s.Phase() == StartLightGreen && greenTick < 0 {
			greenTick = tick
		}
		if s.Phase() == StartLightSweep && sweepTick < 0 {
			sweepTick = tick
		}
	}
	if greenTick < 0 || sweepTick < 0 {
		t.Fatalf("green at %d, sweep at %d; both must occur", greenTick, sweepTick)
	}
	if sweepTick <= greenTick {
		t.Errorf("sweep began at %d, not after green at %d", sweepTick, greenTick)
	}
	if delay := sweepTick - greenTick; delay > TicksPerSecond {
		t.Errorf("sweep began %d ticks after green, expected under a second", delay)
	}
}

// The three tones straddle the colour changes and the fourth fires on green.
func TestCountdownSoundEffects(t *testing.T) {
	if CountdownSoundEffect(0x53) != SfxCountdownTone2 {
		t.Error("no tone at the tick red begins")
	}
	if CountdownSoundEffect(0x29) != SfxCountdownTone1 {
		t.Error("no tone at the tick yellow ends")
	}
	if CountdownSoundEffect(0) != SfxCountdownGo {
		t.Error("no tone on green")
	}
	if CountdownSoundEffect(100) != -1 {
		t.Error("unexpected tone during the camera sweep")
	}
}
