package psx

import (
	"encoding/binary"
	"fmt"
)

const (
	menuLineRecordSize = 16
	// LoadMenuLineData copies exactly 0x10740 bytes from MENU.DAT into the
	// runtime line table. The remaining 351 records in the retained file are
	// not copied by the PAL retail executable.
	retailMenuLineBytes = 0x10740
)

// MenuLine is one 16-byte MENU.DAT line-art record. Start/End are native
// little-endian screen coordinates. The final eight bytes are two four-byte
// color/control values; their individual alpha/control semantics have not yet
// been independently confirmed, so they remain losslessly represented.
type MenuLine struct {
	StartX, StartY int16
	EndX, EndY     int16
	ColorA         [4]byte
	ColorB         [4]byte
}

// MenuLineData separates the records copied by the retail executable from
// additional records retained at the end of the development-produced file.
type MenuLineData struct {
	Retail   []MenuLine
	Trailing []MenuLine
}

// DecodeMenuDAT parses COMMON/MENU.DAT. LoadMenuLineData performs no byte
// swapping, confirming that this format is stored in PS1-native little endian.
func DecodeMenuDAT(data []byte) (*MenuLineData, error) {
	if len(data) < retailMenuLineBytes {
		return nil, fmt.Errorf("psx: MENU.DAT is %d bytes, retail table needs %d", len(data), retailMenuLineBytes)
	}
	if len(data)%menuLineRecordSize != 0 {
		return nil, fmt.Errorf("psx: MENU.DAT size %d is not a multiple of %d", len(data), menuLineRecordSize)
	}
	lines := make([]MenuLine, len(data)/menuLineRecordSize)
	for i := range lines {
		offset := i * menuLineRecordSize
		lines[i] = MenuLine{
			StartX: int16(binary.LittleEndian.Uint16(data[offset : offset+2])),
			StartY: int16(binary.LittleEndian.Uint16(data[offset+2 : offset+4])),
			EndX:   int16(binary.LittleEndian.Uint16(data[offset+4 : offset+6])),
			EndY:   int16(binary.LittleEndian.Uint16(data[offset+6 : offset+8])),
		}
		copy(lines[i].ColorA[:], data[offset+8:offset+12])
		copy(lines[i].ColorB[:], data[offset+12:offset+16])
	}
	retailCount := retailMenuLineBytes / menuLineRecordSize
	return &MenuLineData{Retail: lines[:retailCount], Trailing: lines[retailCount:]}, nil
}
