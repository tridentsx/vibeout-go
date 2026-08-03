package psx

import (
	"encoding/binary"
	"fmt"
)

// CheckpointCount is the number of records stored in every CPOINT*.CHK file.
// The race initializer clears six destination records before loading the file,
// and all 32 files in the PAL disc corpus are exactly 6*6 bytes.
const CheckpointCount = 6

const checkpointRecordSize = 6

// Checkpoint is one record from a CPOINT*.CHK file.
//
// Section is a little-endian signed track-section index; -1 marks an unused
// record. The executable copies the remaining four bytes verbatim. Their
// individual gameplay meanings have not yet been established from consumers,
// so they remain explicitly neutral rather than receiving guessed names.
type Checkpoint struct {
	Section    int16
	Parameters [4]byte
}

// DecodeCHK decodes the complete fixed-size checkpoint file used by the race
// initializer (InitCheckPoints at SLES_003.27 address 0x8003c680).
func DecodeCHK(data []byte) ([]Checkpoint, error) {
	const fileSize = CheckpointCount * checkpointRecordSize
	if len(data) != fileSize {
		return nil, fmt.Errorf("psx: CHK must be %d bytes (got %d)", fileSize, len(data))
	}

	checkpoints := make([]Checkpoint, CheckpointCount)
	for i := range checkpoints {
		offset := i * checkpointRecordSize
		checkpoints[i].Section = int16(binary.LittleEndian.Uint16(data[offset : offset+2]))
		copy(checkpoints[i].Parameters[:], data[offset+2:offset+checkpointRecordSize])
	}
	return checkpoints, nil
}
