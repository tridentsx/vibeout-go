package render

import (
	"fmt"

	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/psx"
)

// This file adds a 2D screen-space layer on top of the existing 3D pipeline: the
// boot splashes, the PRESS START title, the overlay placeholder screens and menu
// text. It reuses Frame.Submit, so 2D and 3D share one depth-tested pass.
//
// Retail worked in a 320x240 (PAL 320x256) framebuffer and every coordinate in the
// executable is in that space -- "PRESS START" is drawn at (0xa0, 0xe4) = (160,
// 228). Rather than rescale those constants at each call site, this layer takes
// retail coordinates and maps them to the window.

const (
	// RetailWidth and RetailHeight are the framebuffer retail drew into. The
	// executable's draw calls use these coordinates directly.
	RetailWidth  = 320
	RetailHeight = 240

)

// UI draws screen-space quads in retail coordinates. It owns the font texture and
// scales to whatever the window is.
type UI struct {
	device *Device
	font   *sdl.GPUTexture
	fontW  int
	fontH  int
	// white is a 1x1 opaque texture for untextured fills, so solid quads go through
	// the same textured pipeline rather than needing a second one.
	white  *sdl.GPUTexture
	width  float32
	height float32
}

// NewUI loads the font atlas and builds the helper texture. The font is
// TEXTURES/WOFONT.TIM, an 80x24 grid of 6x6 cells (13 per row) as
// maybe_BuildTextCharacterQuadPrimitive indexes it.
func NewUI(device *Device, loader assets.Loader, width, height int32) (*UI, error) {
	img, err := loader.LoadTIM("TEXTURES", "WOFONT.TIM")
	if err != nil {
		return nil, fmt.Errorf("render: loading the menu font: %w", err)
	}
	font, err := device.NewTexture(img.Width, img.Height, img.Pixels)
	if err != nil {
		return nil, fmt.Errorf("render: uploading the menu font: %w", err)
	}
	white, err := device.NewTexture(1, 1, []byte{0xff, 0xff, 0xff, 0xff})
	if err != nil {
		return nil, fmt.Errorf("render: creating the fill texture: %w", err)
	}
	return &UI{
		device: device, font: font,
		fontW: img.Width, fontH: img.Height,
		white: white,
		width: float32(width), height: float32(height),
	}, nil
}

// Destroy releases the textures the UI owns.
func (u *UI) Destroy() {
	if u == nil {
		return
	}
	u.device.ReleaseTexture(u.font)
	u.device.ReleaseTexture(u.white)
}

// ndc converts a retail pixel coordinate to clip space. Retail's Y grows
// downwards, clip space upwards.
func (u *UI) ndc(x, y float32) (float32, float32) {
	return x/RetailWidth*2 - 1, 1 - y/RetailHeight*2
}

// quad submits an axis-aligned rectangle as two triangles. Source coordinates are
// in texture pixels; passing a zero-size source stretches the whole texture.
func (u *UI) quad(f *Frame, tex *sdl.GPUTexture, dstX, dstY, dstW, dstH float32,
	srcX, srcY, srcW, srcH, texW, texH float32, col sdl.FColor) {
	if tex == nil || f == nil {
		return
	}
	u0, v0 := srcX/texW, srcY/texH
	u1, v1 := (srcX+srcW)/texW, (srcY+srcH)/texH

	x0, y0 := u.ndc(dstX, dstY)
	x1, y1 := u.ndc(dstX+dstW, dstY+dstH)

	// The vertex shader takes inPosition.z as *camera-space Z*, not a depth value,
	// and emits vec4(x*z, y*z, depth*z, z) so the GPU's perspective divide recovers
	// the screen position. Passing 0 makes that vec4(0,0,0,0) and nothing renders at
	// all -- which is exactly what happened first: a black screen.
	//
	// Sitting exactly on the near plane gives depth = (z-near)/(far-near) = 0, the
	// nearest value, so the UI passes the LESS test against the 1.0 depth clear and
	// draws over everything the 3D pass emitted.
	const z = depthNear
	tl := Vertex{X: x0, Y: y0, Z: z, U: u0, V: v0, R: col.R, G: col.G, B: col.B, A: col.A}
	tr := Vertex{X: x1, Y: y0, Z: z, U: u1, V: v0, R: col.R, G: col.G, B: col.B, A: col.A}
	bl := Vertex{X: x0, Y: y1, Z: z, U: u0, V: v1, R: col.R, G: col.G, B: col.B, A: col.A}
	br := Vertex{X: x1, Y: y1, Z: z, U: u1, V: v1, R: col.R, G: col.G, B: col.B, A: col.A}

	f.Submit([3]Vertex{tl, tr, bl}, tex)
	f.Submit([3]Vertex{tr, br, bl}, tex)
}

// White is a convenience colour for untinted drawing.
var White = sdl.FColor{R: 1, G: 1, B: 1, A: 1}

// Fill covers a rectangle in a solid colour, in retail coordinates.
func (u *UI) Fill(f *Frame, x, y, w, h int, col sdl.FColor) {
	u.quad(f, u.white, float32(x), float32(y), float32(w), float32(h), 0, 0, 1, 1, 1, 1, col)
}

// FillScreen covers the whole framebuffer, for the black placeholder screens.
func (u *UI) FillScreen(f *Frame, col sdl.FColor) {
	u.Fill(f, 0, 0, RetailWidth, RetailHeight, col)
}

// DrawText draws a string with its top-left at (x, y) in retail coordinates,
// using retail's glyph metrics.
func (u *UI) DrawText(f *Frame, s string, x, y int, col sdl.FColor) int {
	glyphs, pen := psx.LayoutText(s, x, y)
	for _, g := range glyphs {
		u.quad(f, u.font,
			float32(g.X), float32(g.Y), psx.FontCellSize, psx.FontCellSize,
			float32(g.U), float32(g.V), psx.FontCellSize, psx.FontCellSize,
			float32(u.fontW), float32(u.fontH), col)
	}
	return pen
}

// DrawTextCentered centres a string on x, which is how retail positions several
// banners -- "PRESS START" is drawn at x = 0xa0 = 160, the screen centre.
func (u *UI) DrawTextCentered(f *Frame, s string, centerX, y int, col sdl.FColor) {
	u.DrawText(f, s, centerX-psx.TextWidth(s)/2, y, col)
}

// DrawImage draws a full texture at a position and size in retail coordinates.
func (u *UI) DrawImage(f *Frame, tex *sdl.GPUTexture, x, y, w, h int, col sdl.FColor) {
	u.quad(f, tex, float32(x), float32(y), float32(w), float32(h), 0, 0, 1, 1, 1, 1, col)
}

// DrawFullscreenImage stretches a texture over the whole framebuffer.
func (u *UI) DrawFullscreenImage(f *Frame, tex *sdl.GPUTexture) {
	u.DrawImage(f, tex, 0, 0, RetailWidth, RetailHeight, White)
}

// DrawSplash fills the framebuffer from a boot TIM, taking only the leftmost
// retail-sized region when the image is wider.
//
// WARNING.TIM is 640x256 where COPY2097.TIM, REDBPAL.TIM and STARTPAL.TIM are all
// 320x256, the PAL framebuffer size. Stretching the wide one across the screen would
// squeeze two half-images together, so it is cropped instead. What the right-hand
// half of that image is for has not been established -- a second language or region
// variant is the obvious guess, but it is only a guess.
func (u *UI) DrawSplash(f *Frame, tex *sdl.GPUTexture, texW, texH int) {
	if tex == nil || texW <= 0 || texH <= 0 {
		return
	}
	srcW := float32(texW)
	if texW > RetailWidth {
		srcW = RetailWidth
	}
	srcH := float32(texH)
	if texH > RetailHeight {
		srcH = RetailHeight
	}
	u.quad(f, tex, 0, 0, RetailWidth, RetailHeight,
		0, 0, srcW, srcH, float32(texW), float32(texH), White)
}
