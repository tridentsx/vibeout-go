package psx

import (
	"encoding/binary"
	"fmt"
)

// windowSize is the sliding-window size for the LZ-style compression used by
// .CMP files, matching wipeout.js's `wnd` buffer (0x2000 bytes, masked with
// 0x1fff everywhere it's indexed).
const windowSize = 0x2000

// DecodeCMP unpacks a .CMP bundle into its individual member files. Ported
// from Wipeout.prototype.unpackImages: a small custom LZ77 variant (a
// bitfield selects literal-byte vs. back-reference per bit, not per byte),
// preceded by a header giving each unpacked member file's length so the
// single decompressed stream can be split back into separate files.
func DecodeCMP(data []byte) ([][]byte, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("psx: CMP file too short")
	}

	numFiles := int(binary.LittleEndian.Uint32(data[0:4]))
	packedDataOffset := (numFiles + 1) * 4
	if packedDataOffset > len(data) {
		return nil, fmt.Errorf("psx: CMP file header runs past end of file")
	}

	unpackedLength := 0
	fileLengths := make([]int, numFiles)
	for i := 0; i < numFiles; i++ {
		n := int(binary.LittleEndian.Uint32(data[(i+1)*4 : (i+1)*4+4]))
		fileLengths[i] = n
		unpackedLength += n
	}

	src := data[packedDataOffset:]
	dst := make([]byte, unpackedLength)
	var wnd [windowSize]byte

	srcPos := 0
	dstPos := 0
	wndPos := 1
	var curByte byte
	bitMask := byte(0x80)

	readBitfield := func(size int) int {
		value := 0
		for size > 0 {
			if bitMask == 0x80 {
				if srcPos >= len(src) {
					return value
				}
				curByte = src[srcPos]
				srcPos++
			}
			if curByte&bitMask != 0 {
				value |= size
			}
			size >>= 1
			bitMask >>= 1
			if bitMask == 0 {
				bitMask = 0x80
			}
		}
		return value
	}

	for {
		if srcPos > len(src) || dstPos > unpackedLength {
			break
		}
		if bitMask == 0x80 {
			if srcPos >= len(src) {
				break
			}
			curByte = src[srcPos]
			srcPos++
		}
		curBit := curByte & bitMask

		bitMask >>= 1
		if bitMask == 0 {
			bitMask = 0x80
		}

		if curBit != 0 {
			if dstPos >= unpackedLength {
				break
			}
			b := readBitfield(0x80)
			wnd[wndPos&0x1fff] = byte(b)
			dst[dstPos] = byte(b)
			wndPos++
			dstPos++
		} else {
			position := readBitfield(0x1000)
			if position == 0 {
				break
			}
			length := readBitfield(0x08) + 2
			for i := 0; i <= length; i++ {
				if dstPos >= unpackedLength {
					break
				}
				b := wnd[(i+position)&0x1fff]
				wnd[wndPos&0x1fff] = b
				dst[dstPos] = b
				wndPos++
				dstPos++
			}
		}
	}

	files := make([][]byte, numFiles)
	fileOffset := 0
	for i := 0; i < numFiles; i++ {
		files[i] = dst[fileOffset : fileOffset+fileLengths[i]]
		fileOffset += fileLengths[i]
	}
	return files, nil
}
