// Package sfx owns short, event-driven game sounds. It deliberately knows
// nothing about file formats; the assets layer supplies decoded clips.
package sfx

type Event uint16

const (
	Engine Event = iota
	Airbrake
	Collision
	WeaponFire
	Pickup
	MenuMove
	MenuConfirm
)

type Clip struct {
	Samples    []int16
	SampleRate int
	Loop       bool
}

// Player is the boundary implemented by an SDL (or other) audio backend.
type Player interface {
	Play(Event, Clip) error
	Stop(Event)
	SetGain(Event, float32)
}

// Bank maps semantic game events to decoded clips.
type Bank map[Event]Clip
