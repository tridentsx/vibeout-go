package psx

import (
	"encoding/binary"
	"fmt"
	"strings"
)

const wadEntrySize = 25

// WADEntry is one member of a WipEout resource archive. The on-disc table
// stores a 16-byte NUL-terminated name, two sizes, and a flags byte, followed
// by all member payloads concatenated in table order.
type WADEntry struct {
	Name             string
	StoredSize       uint32
	UncompressedSize uint32
	Flags            uint8
	Data             []byte
}

// DecodeWAD decodes a complete game WAD archive. All eleven archives in the
// PAL disc corpus use flags=0 and equal stored/uncompressed sizes; non-zero
// flags are retained but rejected until their transform is observed in a real
// file rather than guessed.
func DecodeWAD(data []byte) ([]WADEntry, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("psx: WAD file too short (%d bytes)", len(data))
	}
	count := int(binary.LittleEndian.Uint16(data[:2]))
	headerSize := 2 + count*wadEntrySize
	if headerSize > len(data) {
		return nil, fmt.Errorf("psx: WAD table for %d entries exceeds file", count)
	}

	entries := make([]WADEntry, count)
	payloadOffset := headerSize
	for i := range entries {
		offset := 2 + i*wadEntrySize
		nameBytes := data[offset : offset+16]
		nameEnd := 0
		for nameEnd < len(nameBytes) && nameBytes[nameEnd] != 0 {
			nameEnd++
		}
		if nameEnd == 0 {
			return nil, fmt.Errorf("psx: WAD entry %d has an empty name", i)
		}

		storedSize := binary.LittleEndian.Uint32(data[offset+16 : offset+20])
		uncompressedSize := binary.LittleEndian.Uint32(data[offset+20 : offset+24])
		flags := data[offset+24]
		if flags != 0 || storedSize != uncompressedSize {
			return nil, fmt.Errorf("psx: WAD entry %q uses unsupported encoding (flags=%d, stored=%d, unpacked=%d)",
				string(nameBytes[:nameEnd]), flags, storedSize, uncompressedSize)
		}
		end := payloadOffset + int(storedSize)
		if end < payloadOffset || end > len(data) {
			return nil, fmt.Errorf("psx: WAD entry %q payload exceeds file", string(nameBytes[:nameEnd]))
		}
		entries[i] = WADEntry{
			Name:             strings.ToLower(string(nameBytes[:nameEnd])),
			StoredSize:       storedSize,
			UncompressedSize: uncompressedSize,
			Flags:            flags,
			Data:             data[payloadOffset:end],
		}
		payloadOffset = end
	}
	if payloadOffset != len(data) {
		return nil, fmt.Errorf("psx: WAD has %d trailing bytes", len(data)-payloadOffset)
	}
	return entries, nil
}
