package psx

import (
	"encoding/binary"
	"fmt"
)

// TrackVisibility is one section's fifteen ordered visibility-index lists.
// InitViewList arranges them as three lanes of five groups and assigns their
// pointers to section fields from 0x24 through 0x5c.
type TrackVisibility struct {
	Lists [3][5][]uint16
}

// DecodeVEW decodes a big-endian TRACK.VEW index stream and partitions it
// using the five list lengths stored in each corresponding TRS section.
func DecodeVEW(data []byte, sections []TrackSection) ([]TrackVisibility, error) {
	if len(data)%2 != 0 {
		return nil, fmt.Errorf("psx: VEW file length %d is not even", len(data))
	}
	values := make([]uint16, len(data)/2)
	for i := range values {
		values[i] = binary.BigEndian.Uint16(data[i*2 : i*2+2])
	}

	result := make([]TrackVisibility, len(sections))
	cursor := 0
	for sectionIndex, section := range sections {
		for lane := range section.ViewCounts {
			for group, count16 := range section.ViewCounts[lane] {
				count := int(count16)
				end := cursor + count
				if end < cursor || end > len(values) {
					return nil, fmt.Errorf("psx: VEW ends in section %d lane %d group %d", sectionIndex, lane, group)
				}
				result[sectionIndex].Lists[lane][group] = values[cursor:end]
				cursor = end
			}
		}
	}
	if cursor != len(values) {
		return nil, fmt.Errorf("psx: VEW has %d unreferenced indices", len(values)-cursor)
	}
	return result, nil
}
