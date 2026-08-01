// Package psx parses WipEout 2097's original PS1 asset formats (.TIM
// textures, .CMP compressed bundles, .PRM 3D models). Ported field-for-field
// from phoboslab's wipeout.js (https://github.com/phoboslab/wipeout), the
// only known working reference implementation of these formats -- struct
// layouts and byte offsets here are not guesses, they're a direct port of
// that project's Struct.create() declarations.
package psx

import (
	"encoding/binary"
	"fmt"
)

// TIM image types, per the PS1 GPU's own packed-pixel formats.
const (
	ImagePaletted4BPP  = 0x08
	ImagePaletted8BPP  = 0x09
	ImageTrueColor16BPP = 0x02
)

// Image is a decoded TIM texture, always expanded to 8-bit-per-channel RGBA
// regardless of the source format's bit depth.
type Image struct {
	Width  int
	Height int
	// Pixels is width*height RGBA bytes, row-major, matching image/color.RGBA
	// byte order (R,G,B,A per pixel) so it can be handed straight to
	// image.NewRGBA-style consumers.
	Pixels []byte
}

// imageFileHeader mirrors wipeout.js's Wipeout.ImageFileHeader: 20 bytes,
// little-endian (the PS1 CPU's own byte order -- .TIM files on disk are
// little-endian despite the PS1 GPU's internal registers being documented
// big-endian-first in most references).
type imageFileHeader struct {
	Magic         uint32
	Type          uint32
	HeaderLength  uint32
	PaletteX      uint16
	PaletteY      uint16
	PaletteColors uint16
	Palettes      uint16
}

const imageFileHeaderSize = 20

// imagePixelHeader mirrors Wipeout.ImagePixelHeader: 8 bytes.
type imagePixelHeader struct {
	SkipX  uint16
	SkipY  uint16
	Width  uint16
	Height uint16
}

const imagePixelHeaderSize = 8

// DecodeTIM decodes a single .TIM file's bytes into an RGBA image. Ported
// from Wipeout.prototype.readImage.
func DecodeTIM(data []byte) (*Image, error) {
	if len(data) < imageFileHeaderSize {
		return nil, fmt.Errorf("psx: TIM file too short (%d bytes)", len(data))
	}

	file := imageFileHeader{
		Magic:         binary.LittleEndian.Uint32(data[0:4]),
		Type:          binary.LittleEndian.Uint32(data[4:8]),
		HeaderLength:  binary.LittleEndian.Uint32(data[8:12]),
		PaletteX:      binary.LittleEndian.Uint16(data[12:14]),
		PaletteY:      binary.LittleEndian.Uint16(data[14:16]),
		PaletteColors: binary.LittleEndian.Uint16(data[16:18]),
		Palettes:      binary.LittleEndian.Uint16(data[18:20]),
	}
	offset := imageFileHeaderSize

	var palette []uint16
	if file.Type == ImagePaletted4BPP || file.Type == ImagePaletted8BPP {
		n := int(file.PaletteColors)
		if offset+n*2 > len(data) {
			return nil, fmt.Errorf("psx: TIM palette runs past end of file")
		}
		palette = make([]uint16, n)
		for i := 0; i < n; i++ {
			palette[i] = binary.LittleEndian.Uint16(data[offset+i*2 : offset+i*2+2])
		}
		offset += n * 2
	}
	offset += 4 // skip the pixel data block's own byte-length field

	pixelsPerShort := 1
	switch file.Type {
	case ImagePaletted8BPP:
		pixelsPerShort = 2
	case ImagePaletted4BPP:
		pixelsPerShort = 4
	}

	if offset+imagePixelHeaderSize > len(data) {
		return nil, fmt.Errorf("psx: TIM pixel header runs past end of file")
	}
	dim := imagePixelHeader{
		SkipX:  binary.LittleEndian.Uint16(data[offset : offset+2]),
		SkipY:  binary.LittleEndian.Uint16(data[offset+2 : offset+4]),
		Width:  binary.LittleEndian.Uint16(data[offset+4 : offset+6]),
		Height: binary.LittleEndian.Uint16(data[offset+6 : offset+8]),
	}
	offset += imagePixelHeaderSize

	width := int(dim.Width) * pixelsPerShort
	height := int(dim.Height)
	entries := int(dim.Width) * int(dim.Height)

	// Not every .TIM-extension file on the disc is a single conventional
	// image -- e.g. LEGALPAL.TIM's pixel header reads as
	// skipX=skipY=width=height=0x8000, which isn't a real 32768x32768
	// texture; something in the real game reads it differently, not
	// through this generic path. Whatever it turns out to be, this parser
	// should report that cleanly instead of computing a bogus multi-GB
	// pixel count and panicking on an out-of-bounds read.
	if entries < 0 || offset+entries*2 > len(data) {
		return nil, fmt.Errorf(
			"psx: TIM pixel data (%dx%d, %d entries) runs past end of file -- not a conventional single image",
			dim.Width, dim.Height, entries,
		)
	}

	pixels := make([]byte, width*height*4)

	// Every PS1 texture format packs 15-bit BGR555 colors (5 bits per
	// channel); pure black (0x0000) is the hardware's own color-key
	// transparency value, not a real color -- matching wipeout.js's putPixel.
	putPixel := func(dst []byte, offset int, color uint16) {
		dst[offset+0] = byte((color & 0x1f) << 3)
		dst[offset+1] = byte(((color >> 5) & 0x1f) << 3)
		dst[offset+2] = byte(((color >> 10) & 0x1f) << 3)
		if color == 0 {
			dst[offset+3] = 0
		} else {
			dst[offset+3] = 0xff
		}
	}

	switch file.Type {
	case ImageTrueColor16BPP:
		for i := 0; i < entries; i++ {
			c := binary.LittleEndian.Uint16(data[offset+i*2 : offset+i*2+2])
			putPixel(pixels, i*4, c)
		}
	case ImagePaletted8BPP:
		for i := 0; i < entries; i++ {
			p := binary.LittleEndian.Uint16(data[offset+i*2 : offset+i*2+2])
			putPixel(pixels, i*8+0, palette[p&0xff])
			putPixel(pixels, i*8+4, palette[(p>>8)&0xff])
		}
	case ImagePaletted4BPP:
		for i := 0; i < entries; i++ {
			p := binary.LittleEndian.Uint16(data[offset+i*2 : offset+i*2+2])
			putPixel(pixels, i*16+0, palette[p&0xf])
			putPixel(pixels, i*16+4, palette[(p>>4)&0xf])
			putPixel(pixels, i*16+8, palette[(p>>8)&0xf])
			putPixel(pixels, i*16+12, palette[(p>>12)&0xf])
		}
	default:
		return nil, fmt.Errorf("psx: unknown TIM image type 0x%x", file.Type)
	}

	return &Image{Width: width, Height: height, Pixels: pixels}, nil
}
