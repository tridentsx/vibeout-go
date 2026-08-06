package game

import "github.com/tridentsx/wipeout-go/internal/psx"

// TrackPathfinder adapts a loaded track to MovingObjectPathfinder, so the path system
// reads its waypoints from the circuit's own data exactly as retail does: the waypoints
// are sections carrying psx.SectionFlagPathStart, authored per track, not a table in the
// port.
type TrackPathfinder struct {
	Sections []TrackSectionView
}

// TrackSectionView is the little a path needs from a section.
type TrackSectionView struct {
	X, Y, Z int32
	Next    int32
	Flags   uint16
}

func (t TrackPathfinder) SectionCount() int { return len(t.Sections) }

func (t TrackPathfinder) SectionCenter(i int) Vector3 {
	if i < 0 || i >= len(t.Sections) {
		return Vector3{}
	}
	s := t.Sections[i]
	return Vector3{X: float32(s.X), Y: float32(s.Y), Z: float32(s.Z)}
}

func (t TrackPathfinder) SectionNext(i int) int {
	if i < 0 || i >= len(t.Sections) {
		return -1
	}
	return int(t.Sections[i].Next)
}

func (t TrackPathfinder) IsPathNode(i int) bool {
	if i < 0 || i >= len(t.Sections) {
		return false
	}
	return t.Sections[i].Flags&psx.SectionFlagPathStart != 0
}

// PathNodes lists every waypoint section on the track, which is per-circuit data: six on
// Vostok Island, five on Spilskinanke, none at all on Talon's Reach or Valparaiso.
func (t TrackPathfinder) PathNodes() []int {
	var out []int
	for i := range t.Sections {
		if t.IsPathNode(i) {
			out = append(out, i)
		}
	}
	return out
}

// The rescue craft's lights, from maybe_AnimateRescueCraftGlow (0x80048d08). One phase
// accumulator drives three sine samples, and the craft's eleven primitives split into
// three colour groups -- the last of which is the red rear lights that blink as it leaves.
const (
	// CraftGlowPhaseStep is the 0x8c added to the phase each tick.
	CraftGlowPhaseStep = 0x8c
	// CraftGlowSampleOffset is the 75 added for the second sample, putting it out of step
	// with the first.
	CraftGlowSampleOffset = 75
	// CraftGlowHalfRateOffset is the 32 added to a half-rate phase for the third sample.
	CraftGlowHalfRateOffset = 32
	// CraftGlowBase is the 0x80 added after the >> 5, so a sample sweeps 0 to 0xff.
	CraftGlowBase = 0x80
	// CraftGlowShift is the 5 the sine is shifted by.
	CraftGlowShift = 5
	// CraftGlowDim is the 0x28 written to the channels a group does not modulate.
	CraftGlowDim = 0x28
	// CraftPrimitiveCount is how many primitives the animator walks.
	CraftPrimitiveCount = 11
	// CraftGreenGroupEnd and CraftRedGroupStart bound the three groups: below the first is
	// green, from the second is red, and between them is blue.
	CraftGreenGroupEnd = 2
	CraftRedGroupStart = 6
)

// CraftGlow holds the animator's phase.
type CraftGlow struct {
	Phase Angle
}

// Tick advances the phase, as the read-modify-write at 0x80048d08 does.
func (c *CraftGlow) Tick() { c.Phase = (c.Phase + CraftGlowPhaseStep).Wrapped() }

// sample is `clamp((GetSin(phase) >> 5) + 0x80, 0xff)`.
func (c *CraftGlow) sample(phase Angle) uint8 {
	v := int(phase.Wrapped().Sin()*4096) >> CraftGlowShift
	v += CraftGlowBase
	if v > 0xff {
		return 0xff
	}
	if v < 0 {
		return 0
	}
	return uint8(v)
}

// Colors returns the RGB for each of the craft's eleven primitives.
func (c *CraftGlow) Colors() [CraftPrimitiveCount]StartLightRGB {
	first := c.sample(c.Phase)
	second := c.sample(c.Phase + CraftGlowSampleOffset)
	third := c.sample(c.Phase/2 + CraftGlowHalfRateOffset)

	var out [CraftPrimitiveCount]StartLightRGB
	for i := range out {
		switch {
		case i < CraftGreenGroupEnd:
			out[i] = StartLightRGB{R: CraftGlowDim, G: second, B: CraftGlowDim}
		case i >= CraftRedGroupStart:
			// The rear lights. Observed in play as red and blinking, which is what a
			// sample sweeping the full range produces.
			out[i] = StartLightRGB{R: first, G: CraftGlowDim, B: CraftGlowDim}
		default:
			out[i] = StartLightRGB{R: third / 2, G: third / 2, B: third}
		}
	}
	return out
}
