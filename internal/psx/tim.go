// Package psx parses WipEout 2097's original PlayStation asset formats,
// including TIM textures, CMP bundles, PRM models, and track geometry.
package psx

import (
	"encoding/binary"
	"fmt"
)

// TIM image types, per the PS1 GPU's own packed-pixel formats.
const (
	ImagePaletted4BPP   = 0x08
	ImagePaletted8BPP   = 0x09
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

// A TIM starts with an eight-byte {magic,type} header. Paletted images then
// have a CLUT block, while true-color images proceed directly to the image
// block. All fields are little-endian, matching the PS1 CPU.
const (
	timMagic            = 0x10
	timCommonHeaderSize = 8
	timBlockHeaderSize  = 12 // byte length, VRAM X/Y, width, height
)

// imagePixelHeader mirrors Wipeout.ImagePixelHeader: 8 bytes.
type imagePixelHeader struct {
	SkipX  uint16
	SkipY  uint16
	Width  uint16
	Height uint16
}

const imagePixelHeaderSize = 8

// DecodeTIM decodes a single standard .TIM file into an RGBA image.
func DecodeTIM(data []byte) (*Image, error) {
	if len(data) < timCommonHeaderSize {
		return nil, fmt.Errorf("psx: TIM file too short (%d bytes)", len(data))
	}

	if magic := binary.LittleEndian.Uint32(data[0:4]); magic != timMagic {
		return nil, fmt.Errorf("psx: invalid TIM magic 0x%x", magic)
	}
	typ := binary.LittleEndian.Uint32(data[4:8])
	offset := timCommonHeaderSize

	var palette []uint16
	if typ == ImagePaletted4BPP || typ == ImagePaletted8BPP {
		if offset+timBlockHeaderSize > len(data) {
			return nil, fmt.Errorf("psx: TIM CLUT header runs past end of file")
		}
		blockLength := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
		colors := int(binary.LittleEndian.Uint16(data[offset+8 : offset+10]))
		palettes := int(binary.LittleEndian.Uint16(data[offset+10 : offset+12]))
		if blockLength < timBlockHeaderSize || blockLength > len(data)-offset {
			return nil, fmt.Errorf("psx: TIM CLUT block length %d is invalid", blockLength)
		}
		n := colors * palettes
		if n == 0 || timBlockHeaderSize+n*2 > blockLength {
			return nil, fmt.Errorf("psx: TIM palette dimensions %dx%d exceed its block", colors, palettes)
		}
		palette = make([]uint16, n)
		for i := 0; i < n; i++ {
			pos := offset + timBlockHeaderSize + i*2
			palette[i] = binary.LittleEndian.Uint16(data[pos : pos+2])
		}
		offset += blockLength
	}

	pixelsPerShort := 1
	switch typ {
	case ImagePaletted8BPP:
		pixelsPerShort = 2
	case ImagePaletted4BPP:
		pixelsPerShort = 4
	case ImageTrueColor16BPP:
	default:
		return nil, fmt.Errorf("psx: unknown TIM image type 0x%x", typ)
	}

	if offset+timBlockHeaderSize > len(data) {
		return nil, fmt.Errorf("psx: TIM pixel header runs past end of file")
	}
	blockLength := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
	if blockLength < timBlockHeaderSize || blockLength > len(data)-offset {
		return nil, fmt.Errorf("psx: TIM pixel block length %d is invalid", blockLength)
	}
	dim := imagePixelHeader{
		SkipX:  binary.LittleEndian.Uint16(data[offset+4 : offset+6]),
		SkipY:  binary.LittleEndian.Uint16(data[offset+6 : offset+8]),
		Width:  binary.LittleEndian.Uint16(data[offset+8 : offset+10]),
		Height: binary.LittleEndian.Uint16(data[offset+10 : offset+12]),
	}
	offset += timBlockHeaderSize

	width := int(dim.Width) * pixelsPerShort
	height := int(dim.Height)
	entries := int(dim.Width) * int(dim.Height)

	if entries == 0 || entries*2 > blockLength-timBlockHeaderSize {
		return nil, fmt.Errorf(
			"psx: TIM pixel data (%dx%d, %d entries) exceeds its block",
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

	switch typ {
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
	}

	return &Image{Width: width, Height: height, Pixels: pixels}, nil
}
