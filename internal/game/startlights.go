package game

// The start light gantry, ported from maybe_UpdateStartLightGantry
// (SLES_003.27 0x80049ef4). Retail draws three instances of COMMON/LIGHT.PRM -- one
// object, `light1`, 63 polygons of which 29 are type 4 -- and tints the first eight
// type-4 primitives of each per countdown phase. Nothing in VRAM changes; the colour
// is written straight into the objects' own primitives.

// StartLightCount is how many primitives each gantry tints.
const StartLightCount = 8

// GantryCount is how many gantries retail instantiates from the one model.
const GantryCount = 3

// Countdown thresholds, read off the comparisons at 0x8004a040-0x8004a070. The
// countdown is ship+0xe0, which starts at 0xa6 and is decremented once per tick.
const (
	// CountdownStart is the value the timer is loaded with.
	CountdownStart = 0xa6
	// CountdownCameraHandover is where the intro camera arc gives way to the chase
	// or cockpit camera, and where the craft is released.
	CountdownCameraHandover = 0x64
	// CountdownRedFrom and CountdownRedTo bound the red phase: the test is
	// `(unsigned)(cd - 42) < 42`, so 42 through 83 inclusive.
	CountdownRedFrom = 42
	CountdownRedTo   = 83
	// CountdownYellowFrom and CountdownYellowTo bound yellow: `(unsigned)(cd - 1) < 41`,
	// so 1 through 41 inclusive.
	CountdownYellowFrom = 1
	CountdownYellowTo   = 41
	// SweepDelayCount is the post-green counter's threshold. Retail increments it once
	// per tinted primitive while green, so with three gantries of eight it is reached
	// about 17 ticks -- two thirds of a second -- after green.
	SweepDelayCount = 400
)

// StartLightPhase is which stage of the countdown the gantry is showing.
type StartLightPhase int

const (
	// StartLightNeutral is before the sequence begins: retail leaves the primitives
	// at whatever maybe_ResetStartLightColors last wrote, a flat 0x80 grey.
	StartLightNeutral StartLightPhase = iota
	StartLightRed
	StartLightYellow
	StartLightGreen
	// StartLightSweep is the running-light effect that follows green, one lamp bright
	// with a gradient behind it, cycling across the eight.
	StartLightSweep
)

// StartLightRGB is one primitive's colour.
type StartLightRGB struct{ R, G, B uint8 }

// NeutralStartLightRGB is what maybe_ResetStartLightColors (0x80049e5c) writes.
var NeutralStartLightRGB = StartLightRGB{R: 0x80, G: 0x80, B: 0x80}

// StartLightState drives the gantry. Counter mirrors retail's post-green counter at
// 0x80094aec and Frame its sweep cursor.
type StartLightState struct {
	// Countdown mirrors ship+0xe0.
	Countdown int
	// Counter is the post-green primitive counter.
	Counter int
	// Frame advances once per tick and positions the sweep.
	Frame int
}

// NewStartLightState begins a countdown.
func NewStartLightState() *StartLightState {
	return &StartLightState{Countdown: CountdownStart}
}

// Phase reports what the gantry is showing.
func (s *StartLightState) Phase() StartLightPhase {
	if s.Counter >= SweepDelayCount {
		return StartLightSweep
	}
	switch {
	case s.Countdown >= CountdownRedFrom && s.Countdown <= CountdownRedTo:
		return StartLightRed
	case s.Countdown >= CountdownYellowFrom && s.Countdown <= CountdownYellowTo:
		return StartLightYellow
	case s.Countdown == 0:
		return StartLightGreen
	}
	return StartLightNeutral
}

// Colors returns the colour for each of the eight primitives of one gantry.
func (s *StartLightState) Colors() [StartLightCount]StartLightRGB {
	var out [StartLightCount]StartLightRGB
	switch s.Phase() {
	case StartLightRed:
		for i := range out {
			out[i] = StartLightRGB{R: 0xff}
		}
	case StartLightYellow:
		// Retail writes 0xff red with 0x80 green, an amber rather than a pure yellow.
		for i := range out {
			out[i] = StartLightRGB{R: 0xff, G: 0x80}
		}
	case StartLightGreen:
		for i := range out {
			out[i] = StartLightRGB{G: 0xff}
		}
	case StartLightSweep:
		// One lamp is fully lit and the others carry an index-scaled gradient, so the
		// bright one appears to run along the gantry: `if (frame % 8 == i) g = 0xff
		// else g = i << 4`.
		lit := s.Frame % StartLightCount
		if lit < 0 {
			lit += StartLightCount
		}
		for i := range out {
			if i == lit {
				out[i] = StartLightRGB{G: 0xff}
				continue
			}
			out[i] = StartLightRGB{G: uint8(i << 4)}
		}
	default:
		for i := range out {
			out[i] = NeutralStartLightRGB
		}
	}
	return out
}

// Tick advances the countdown by one, mirroring maybe_UpdateRaceStartCountdown, and
// advances the sweep. It reports the sound effect to play this tick, or -1.
func (s *StartLightState) Tick() int {
	s.Frame++
	if s.Countdown > 0 {
		s.Countdown--
	}
	if s.Phase() == StartLightGreen && s.Counter < SweepDelayCount {
		// Retail increments once per tinted primitive across all three gantries.
		s.Counter += StartLightCount * GantryCount
		if s.Counter > SweepDelayCount {
			s.Counter = SweepDelayCount
		}
	}
	return CountdownSoundEffect(s.Countdown)
}

// Countdown sound effects, from maybe_UpdateRaceStartCountdown (0x800251d0). The
// three tones sit either side of the colour thresholds -- 0x53 is the tick red starts
// and 0x29 the tick yellow ends -- and the fourth fires on green.
const (
	SfxCountdownTone3 = 0x1a
	SfxCountdownTone2 = 0x1b
	SfxCountdownTone1 = 0x1c
	SfxCountdownGo    = 0x1d
)

// CountdownSoundEffect returns the effect id to play at a countdown value, or -1.
func CountdownSoundEffect(countdown int) int {
	switch countdown {
	case 0x7d:
		return SfxCountdownTone3
	case 0x53:
		return SfxCountdownTone2
	case 0x29:
		return SfxCountdownTone1
	case 0:
		return SfxCountdownGo
	}
	return -1
}

// NowPlayingBannerTicks is how long the track banner shows. Retail's duration has not
// been recovered; two seconds reads like the brief flash it is.
const NowPlayingBannerTicks = 2 * TicksPerSecond
