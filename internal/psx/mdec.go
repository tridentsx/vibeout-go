package psx

import (
	"fmt"
	"image"
	"math"
)

type mdecVLC struct {
	code  uint16
	bits  uint8
	run   uint8
	level int16
}

var mdecZigzag = [64]uint8{
	0, 1, 8, 16, 9, 2, 3, 10, 17, 24, 32, 25, 18, 11, 4, 5,
	12, 19, 26, 33, 40, 48, 41, 34, 27, 20, 13, 6, 7, 14, 21, 28,
	35, 42, 49, 56, 57, 50, 43, 36, 29, 22, 15, 23, 30, 37, 44, 51,
	58, 59, 52, 45, 38, 31, 39, 46, 53, 60, 61, 54, 47, 55, 62, 63,
}

var mdecQuant = [64]int{
	8, 16, 19, 22, 26, 27, 29, 34, 16, 16, 22, 24, 27, 29, 34, 37,
	19, 22, 26, 27, 29, 34, 34, 38, 22, 22, 26, 27, 29, 34, 37, 40,
	22, 26, 27, 29, 32, 35, 40, 48, 26, 27, 29, 32, 35, 40, 48, 58,
	26, 27, 29, 34, 38, 46, 56, 69, 27, 29, 35, 38, 46, 56, 69, 83,
}

var mdecVLCCodes = [113][2]uint16{
	{0x3, 2}, {0x4, 4}, {0x5, 5}, {0x6, 7}, {0x26, 8}, {0x21, 8}, {0xa, 10}, {0x1d, 12},
	{0x18, 12}, {0x13, 12}, {0x10, 12}, {0x1a, 13}, {0x19, 13}, {0x18, 13}, {0x17, 13}, {0x1f, 14},
	{0x1e, 14}, {0x1d, 14}, {0x1c, 14}, {0x1b, 14}, {0x1a, 14}, {0x19, 14}, {0x18, 14}, {0x17, 14},
	{0x16, 14}, {0x15, 14}, {0x14, 14}, {0x13, 14}, {0x12, 14}, {0x11, 14}, {0x10, 14}, {0x18, 15},
	{0x17, 15}, {0x16, 15}, {0x15, 15}, {0x14, 15}, {0x13, 15}, {0x12, 15}, {0x11, 15}, {0x10, 15},
	{0x3, 3}, {0x6, 6}, {0x25, 8}, {0xc, 10}, {0x1b, 12}, {0x16, 13}, {0x15, 13}, {0x1f, 15},
	{0x1e, 15}, {0x1d, 15}, {0x1c, 15}, {0x1b, 15}, {0x1a, 15}, {0x19, 15}, {0x13, 16}, {0x12, 16},
	{0x11, 16}, {0x10, 16}, {0x5, 4}, {0x4, 7}, {0xb, 10}, {0x14, 12}, {0x14, 13}, {0x7, 5},
	{0x24, 8}, {0x1c, 12}, {0x13, 13}, {0x6, 5}, {0xf, 10}, {0x12, 12}, {0x7, 6}, {0x9, 10},
	{0x12, 13}, {0x5, 6}, {0x1e, 12}, {0x14, 16}, {0x4, 6}, {0x15, 12}, {0x7, 7}, {0x11, 12},
	{0x5, 7}, {0x11, 13}, {0x27, 8}, {0x10, 13}, {0x23, 8}, {0x1a, 16}, {0x22, 8}, {0x19, 16},
	{0x20, 8}, {0x18, 16}, {0xe, 10}, {0x17, 16}, {0xd, 10}, {0x16, 16}, {0x8, 10}, {0x15, 16},
	{0x1f, 12}, {0x1a, 12}, {0x19, 12}, {0x17, 12}, {0x16, 12}, {0x1f, 13}, {0x1e, 13}, {0x1d, 13},
	{0x1c, 13}, {0x1b, 13}, {0x1f, 16}, {0x1e, 16}, {0x1d, 16}, {0x1c, 16}, {0x1b, 16},
	{0x1, 6}, {0x2, 2},
}

var mdecLevels = [111]int16{
	1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
	21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40,
	1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18,
	1, 2, 3, 4, 5, 1, 2, 3, 4, 1, 2, 3, 1, 2, 3, 1, 2, 3, 1, 2, 1, 2,
	1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
}

var mdecRuns = [111]uint8{
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	2, 2, 2, 2, 2, 3, 3, 3, 3, 4, 4, 4, 5, 5, 5, 6, 6, 6, 7, 7, 8, 8,
	9, 9, 10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15, 16, 16,
	17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31,
}

type mdecBits struct {
	data []byte
	bit  int
}

func (r *mdecBits) read(n int) (uint32, error) {
	if n < 0 || r.bit+n > len(r.data)*8 {
		return 0, fmt.Errorf("psx: truncated MDEC bitstream")
	}
	var value uint32
	for i := 0; i < n; i++ {
		wordBit := r.bit & 15
		byteOffset := (r.bit >> 4) * 2
		var b byte
		if wordBit < 8 {
			b = r.data[byteOffset+1]
		} else {
			b = r.data[byteOffset]
		}
		value = value<<1 | uint32((b>>uint(7-(wordBit&7)))&1)
		r.bit++
	}
	return value, nil
}

func (r *mdecBits) signed(n int) (int, error) {
	v, err := r.read(n)
	if err != nil {
		return 0, err
	}
	if v&(1<<uint(n-1)) != 0 {
		return int(v) - 1<<uint(n), nil
	}
	return int(v), nil
}

func decodeMDECVLC(r *mdecBits) (index int, escape, eob bool, err error) {
	var code uint32
	for bits := 1; bits <= 16; bits++ {
		b, readErr := r.read(1)
		if readErr != nil {
			return 0, false, false, readErr
		}
		code = code<<1 | b
		for i, entry := range mdecVLCCodes {
			if int(entry[1]) == bits && uint32(entry[0]) == code {
				return i, i == 111, i == 112, nil
			}
		}
	}
	return 0, false, false, fmt.Errorf("psx: invalid MDEC VLC at bit %d", r.bit)
}

func decodeMDECBlock(r *mdecBits, qscale int) ([64]float64, error) {
	var block [64]float64
	dc, err := r.signed(10)
	if err != nil {
		return block, err
	}
	block[0] = float64(2*dc + 1024)
	position := 0
	for {
		index, escape, eob, err := decodeMDECVLC(r)
		if err != nil {
			return block, err
		}
		if eob {
			return block, nil
		}
		var run, level int
		if escape {
			rawRun, err := r.read(6)
			if err != nil {
				return block, err
			}
			level, err = r.signed(10)
			if err != nil {
				return block, err
			}
			run = int(rawRun) + 1
		} else {
			sign, err := r.read(1)
			if err != nil {
				return block, err
			}
			run = int(mdecRuns[index]) + 1
			level = int(mdecLevels[index])
			if sign != 0 {
				level = -level
			}
		}
		position += run
		if position > 63 {
			return block, fmt.Errorf("psx: MDEC coefficient run exceeds block")
		}
		matrixIndex := int(mdecZigzag[position])
		magnitude := level
		if magnitude < 0 {
			magnitude = -magnitude
		}
		value := (magnitude * qscale * mdecQuant[matrixIndex]) >> 3
		if escape {
			value = (value - 1) | 1
		}
		if level < 0 {
			value = -value
		}
		block[matrixIndex] = float64(value)
	}
}

func mdecIDCT(coeff [64]float64) [64]uint8 {
	var out [64]uint8
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			var sum float64
			for v := 0; v < 8; v++ {
				for u := 0; u < 8; u++ {
					cu, cv := 1.0, 1.0
					if u == 0 {
						cu = 1 / math.Sqrt2
					}
					if v == 0 {
						cv = 1 / math.Sqrt2
					}
					sum += cu * cv * coeff[v*8+u] * math.Cos(float64((2*x+1)*u)*math.Pi/16) * math.Cos(float64((2*y+1)*v)*math.Pi/16)
				}
			}
			value := int(math.Round(sum / 4))
			if value < 0 {
				value = 0
			}
			if value > 255 {
				value = 255
			}
			out[y*8+x] = uint8(value)
		}
	}
	return out
}

// DecodeRGBA decodes a reassembled version-1 PlayStation MDEC frame.
func (frame *AVFrame) DecodeRGBA() (*image.RGBA, error) {
	if frame.Header.Version != 1 {
		return nil, fmt.Errorf("psx: unsupported MDEC version %d", frame.Header.Version)
	}
	if len(frame.Data) < 8 {
		return nil, fmt.Errorf("psx: truncated MDEC frame")
	}
	r := &mdecBits{data: frame.Data, bit: 64}
	width, height := int(frame.Header.Width), int(frame.Header.Height)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	mbWidth, mbHeight := (width+15)/16, (height+15)/16
	for mbX := 0; mbX < mbWidth; mbX++ {
		for mbY := 0; mbY < mbHeight; mbY++ {
			var pixels [6][64]uint8
			order := [6]int{5, 4, 0, 1, 2, 3}
			for _, blockIndex := range order {
				coeff, err := decodeMDECBlock(r, int(frame.Header.QuantScale))
				if err != nil {
					return nil, fmt.Errorf("psx: frame %d macroblock %d,%d: %w", frame.Header.FrameNumber, mbX, mbY, err)
				}
				pixels[blockIndex] = mdecIDCT(coeff)
			}
			for y := 0; y < 16; y++ {
				for x := 0; x < 16; x++ {
					px, py := mbX*16+x, mbY*16+y
					if px >= width || py >= height {
						continue
					}
					yBlock := (y/8)*2 + x/8
					yy := int(pixels[yBlock][(y&7)*8+(x&7)])
					cb := int(pixels[4][(y/2)*8+x/2]) - 128
					cr := int(pixels[5][(y/2)*8+x/2]) - 128
					rr := yy + (91881 * cr >> 16)
					gg := yy - ((22554*cb + 46802*cr) >> 16)
					bb := yy + (116130 * cb >> 16)
					if rr < 0 {
						rr = 0
					}
					if rr > 255 {
						rr = 255
					}
					if gg < 0 {
						gg = 0
					}
					if gg > 255 {
						gg = 255
					}
					if bb < 0 {
						bb = 0
					}
					if bb > 255 {
						bb = 255
					}
					o := img.PixOffset(px, py)
					img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = byte(rr), byte(gg), byte(bb), 255
				}
			}
		}
	}
	return img, nil
}
