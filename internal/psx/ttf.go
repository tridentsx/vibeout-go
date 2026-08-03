package psx

import (
	"encoding/binary"
	"fmt"
)

// TTFValueCount is the number of big-endian uint16 values in one game TTF
// record. This is WipEout's own track-data format, unrelated to TrueType.
const TTFValueCount = 21

const ttfRecordSize = TTFValueCount * 2

// TTFRecord preserves one complete record. LoadTtfFile (SLES_003.27
// 0x8002031c) byte-swaps all 21 values in place and exposes the record count;
// individual field meanings have not yet been proven by its consumers.
type TTFRecord struct {
	Values [TTFValueCount]uint16
}

// DecodeTTF decodes WipEout's fixed-size, big-endian TTF track-data records.
func DecodeTTF(data []byte) ([]TTFRecord, error) {
	if len(data)%ttfRecordSize != 0 {
		return nil, fmt.Errorf("psx: TTF file length %d is not a multiple of %d", len(data), ttfRecordSize)
	}
	records := make([]TTFRecord, len(data)/ttfRecordSize)
	for i := range records {
		offset := i * ttfRecordSize
		for j := range records[i].Values {
			word := offset + j*2
			records[i].Values[j] = binary.BigEndian.Uint16(data[word : word+2])
		}
	}
	return records, nil
}
