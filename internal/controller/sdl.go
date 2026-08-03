package controller

import (
	"github.com/Zyko0/go-sdl3/sdl"
)

// SDLGamepad owns one SDL3 gamepad and presents it as a PS1-style pad.
// SDL's mapping database handles DualShock/DualSense/Xbox/Nintendo layouts.
type SDLGamepad struct {
	pad      *sdl.Gamepad
	previous Buttons
	deadzone int16
}

func OpenFirstSDLGamepad() (*SDLGamepad, error) {
	ids, err := sdl.GetGamepads()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return &SDLGamepad{deadzone: 6000}, nil
	}
	pad, err := ids[0].OpenGamepad()
	if err != nil {
		return nil, err
	}
	return &SDLGamepad{pad: pad, deadzone: 6000}, nil
}

func (g *SDLGamepad) Connected() bool { return g != nil && g.pad != nil && g.pad.Connected() }
func (g *SDLGamepad) Name() string {
	if !g.Connected() {
		return "NO CONTROLLER"
	}
	return g.pad.Name()
}
func (g *SDLGamepad) Close() {
	if g != nil && g.pad != nil {
		g.pad.Close()
		g.pad = nil
	}
}

// Poll returns a resolved frame. Call it after SDL_PollEvent has pumped the
// event queue. Keyboard support can feed the same Mapping through Buttons.
func (g *SDLGamepad) Poll(mapping Mapping) State {
	if !g.Connected() {
		return State{}
	}
	now := readButtons(g.pad)
	x := normalizedAxis(g.pad.Axis(sdl.GAMEPAD_AXIS_LEFTX), g.deadzone)
	state := mapping.Resolve(now, g.previous, x)
	g.previous = now
	return state
}

func readButtons(pad *sdl.Gamepad) Buttons {
	var b Buttons
	pairs := [...]struct {
		ps  Button
		sdl sdl.GamepadButton
	}{
		{DPadUp, sdl.GAMEPAD_BUTTON_DPAD_UP}, {DPadDown, sdl.GAMEPAD_BUTTON_DPAD_DOWN},
		{DPadLeft, sdl.GAMEPAD_BUTTON_DPAD_LEFT}, {DPadRight, sdl.GAMEPAD_BUTTON_DPAD_RIGHT},
		{Cross, sdl.GAMEPAD_BUTTON_SOUTH}, {Circle, sdl.GAMEPAD_BUTTON_EAST},
		{Square, sdl.GAMEPAD_BUTTON_WEST}, {Triangle, sdl.GAMEPAD_BUTTON_NORTH},
		{L1, sdl.GAMEPAD_BUTTON_LEFT_SHOULDER}, {R1, sdl.GAMEPAD_BUTTON_RIGHT_SHOULDER},
		{Select, sdl.GAMEPAD_BUTTON_BACK}, {Start, sdl.GAMEPAD_BUTTON_START},
	}
	for _, pair := range pairs {
		b.Set(pair.ps, pad.Button(pair.sdl))
	}
	// SDL exposes modern triggers as axes; treating a half press as the PS1's
	// digital L2/R2 preserves the original game's input model.
	b.Set(L2, pad.Axis(sdl.GAMEPAD_AXIS_LEFT_TRIGGER) > 16384)
	b.Set(R2, pad.Axis(sdl.GAMEPAD_AXIS_RIGHT_TRIGGER) > 16384)
	return b
}

func normalizedAxis(v, deadzone int16) float32 {
	if v >= -deadzone && v <= deadzone {
		return 0
	}
	if v < 0 {
		return float32(int32(v)+int32(deadzone)) / float32(32768-int32(deadzone))
	}
	return float32(int32(v)-int32(deadzone)) / float32(32767-int32(deadzone))
}
