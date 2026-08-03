// Package controller exposes WipEout 2097's controls as game actions rather
// than SDL button numbers.  The original controls screen only remaps the four
// race controls; steering, pause, view and menu controls remain fixed.
package controller

import (
	"errors"
	"fmt"
)

// Button names the physical controls on a PlayStation-style pad.  Face button
// names are positions, so the mapping stays correct on non-Sony SDL gamepads.
type Button uint8

const (
	ButtonNone Button = iota
	DPadUp
	DPadDown
	DPadLeft
	DPadRight
	Cross
	Circle
	Square
	Triangle
	L1
	R1
	L2
	R2
	Select
	Start
)

var buttonNames = [...]string{"—", "UP", "DOWN", "LEFT", "RIGHT", "CROSS", "CIRCLE", "SQUARE", "TRIANGLE", "L1", "R1", "L2", "R2", "SELECT", "START"}

func (b Button) String() string {
	if int(b) >= len(buttonNames) {
		return fmt.Sprintf("BUTTON(%d)", b)
	}
	return buttonNames[b]
}

// Action is a semantic control consumed by the game or its menus.
type Action uint8

const (
	SteerLeft Action = iota
	SteerRight
	Accelerate
	FireWeapon
	LeftAirbrake
	RightAirbrake
	ChangeView
	Pause
	MenuUp
	MenuDown
	MenuLeft
	MenuRight
	MenuConfirm
	MenuBack
	actionCount
)

type ActionInfo struct {
	Action     Action
	Label      string
	Remappable bool
}

var actionInfo = [...]ActionInfo{
	{SteerLeft, "STEER LEFT", false},
	{SteerRight, "STEER RIGHT", false},
	{Accelerate, "ACCELERATE", true},
	{FireWeapon, "FIRE WEAPON", true},
	{LeftAirbrake, "LEFT AIRBRAKE", true},
	{RightAirbrake, "RIGHT AIRBRAKE", true},
	{ChangeView, "CHANGE VIEW", false},
	{Pause, "PAUSE", false},
	{MenuUp, "MENU UP", false},
	{MenuDown, "MENU DOWN", false},
	{MenuLeft, "MENU LEFT", false},
	{MenuRight, "MENU RIGHT", false},
	{MenuConfirm, "SELECT", false},
	{MenuBack, "BACK", false},
}

func Info(a Action) (ActionInfo, bool) {
	if a >= actionCount {
		return ActionInfo{}, false
	}
	return actionInfo[a], true
}

// RemappableActions is ordered exactly as rows should appear on the controls
// screen.
func RemappableActions() []Action {
	return []Action{Accelerate, FireWeapon, LeftAirbrake, RightAirbrake}
}

// Mapping holds the complete virtual PS1 pad layout.  Fixed controls cannot
// be changed through Assign.
type Mapping struct{ buttons [actionCount]Button }

func DefaultMapping() Mapping {
	var m Mapping
	m.buttons[SteerLeft], m.buttons[SteerRight] = DPadLeft, DPadRight
	m.buttons[Accelerate], m.buttons[FireWeapon] = Cross, Square
	m.buttons[LeftAirbrake], m.buttons[RightAirbrake] = L2, R2
	m.buttons[ChangeView], m.buttons[Pause] = Triangle, Start
	m.buttons[MenuUp], m.buttons[MenuDown] = DPadUp, DPadDown
	m.buttons[MenuLeft], m.buttons[MenuRight] = DPadLeft, DPadRight
	m.buttons[MenuConfirm], m.buttons[MenuBack] = Cross, Circle
	return m
}

func (m Mapping) Button(a Action) Button {
	if a >= actionCount {
		return ButtonNone
	}
	return m.buttons[a]
}

var (
	ErrFixedAction   = errors.New("control is fixed")
	ErrInvalidAction = errors.New("invalid action")
	ErrInvalidButton = errors.New("button cannot be assigned")
)

// Assign gives action a button.  If another remappable action owns it, the
// two buttons are swapped.  This prevents unreachable controls and mirrors a
// console controls screen better than silently creating duplicates.
func (m *Mapping) Assign(action Action, button Button) error {
	info, ok := Info(action)
	if !ok {
		return ErrInvalidAction
	}
	if !info.Remappable {
		return ErrFixedAction
	}
	if !remapButton(button) {
		return ErrInvalidButton
	}
	old := m.buttons[action]
	for _, other := range RemappableActions() {
		if other != action && m.buttons[other] == button {
			m.buttons[other] = old
			break
		}
	}
	m.buttons[action] = button
	return nil
}

func remapButton(b Button) bool {
	// Triangle is reserved for the fixed change-view action; Start and the
	// D-pad are likewise deliberately absent. Race actions must not make a
	// fixed action fire as a side effect.
	return b == Cross || b == Circle || b == Square ||
		b == L1 || b == R1 || b == L2 || b == R2
}

// State is one frame of virtual-pad input. Axis values use [-1, 1].
type State struct {
	Down    [actionCount]bool
	Pressed [actionCount]bool
	Steer   float32
}

func (s State) IsDown(a Action) bool     { return a < actionCount && s.Down[a] }
func (s State) WasPressed(a Action) bool { return a < actionCount && s.Pressed[a] }

// Resolve converts physical input into actions and edge-triggered presses.
func (m Mapping) Resolve(now, previous Buttons, analogX float32) State {
	var state State
	for a := Action(0); a < actionCount; a++ {
		b := m.buttons[a]
		state.Down[a] = now.Has(b)
		state.Pressed[a] = state.Down[a] && !previous.Has(b)
	}
	state.Steer = clamp(analogX)
	if state.Down[SteerLeft] {
		state.Steer = -1
	}
	if state.Down[SteerRight] {
		state.Steer = 1
	}
	return state
}

type Buttons uint32

func (b Buttons) Has(button Button) bool { return button > ButtonNone && b&(1<<button) != 0 }
func (b *Buttons) Set(button Button, down bool) {
	if button == ButtonNone {
		return
	}
	if down {
		*b |= 1 << button
	} else {
		*b &^= 1 << button
	}
}

func clamp(v float32) float32 {
	if v < -1 {
		return -1
	}
	if v > 1 {
		return 1
	}
	return v
}
