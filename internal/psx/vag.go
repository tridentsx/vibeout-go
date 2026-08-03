package psx

import (
	"encoding/binary"
	"fmt"
	"strings"
)

const vagHeaderSize = 48

// VAG is a Sony PlayStation VAGp sound: a big-endian metadata header followed
// by 16-byte SPU ADPCM blocks. The retail game uploads Data from header offset
// 0x30 directly to SPU RAM; DecodePCM reproduces the SPU's mono decoding for
// modern audio backends.
type VAG struct {
	Version    uint32
	SampleRate uint32
	Name       string
	Data       []byte
	// Trailing preserves bytes beyond the header-declared SPU upload length.
	// The retail loader ignores them; three.vag retains 448 such bytes.
	Trailing []byte
}

// DecodeVAG parses a complete VAGp resource.
func DecodeVAG(data []byte) (*VAG, error) {
	if len(data) < vagHeaderSize {
		return nil, fmt.Errorf("psx: VAG file too short (%d bytes)", len(data))
	}
	if string(data[:4]) != "VAGp" {
		return nil, fmt.Errorf("psx: invalid VAG magic %q", data[:4])
	}
	dataSize := int(binary.BigEndian.Uint32(data[12:16]))
	if dataSize < 0 || dataSize%16 != 0 || vagHeaderSize+dataSize > len(data) {
		return nil, fmt.Errorf("psx: VAG data size %d does not match file size %d", dataSize, len(data))
	}
	rate := binary.BigEndian.Uint32(data[16:20])
	if rate == 0 {
		return nil, fmt.Errorf("psx: VAG sample rate is zero")
	}
	nameBytes := data[32:48]
	if end := strings.IndexByte(string(nameBytes), 0); end >= 0 {
		nameBytes = nameBytes[:end]
	}
	return &VAG{
		Version:    binary.BigEndian.Uint32(data[4:8]),
		SampleRate: rate,
		Name:       string(nameBytes),
		Data:       append([]byte(nil), data[vagHeaderSize:vagHeaderSize+dataSize]...),
		Trailing:   append([]byte(nil), data[vagHeaderSize+dataSize:]...),
	}, nil
}

var vagPredictorCoefficients = [5][2]int32{
	{0, 0}, {60, 0}, {115, -52}, {98, -55}, {122, -60},
}

// DecodePCM expands the VAG's SPU ADPCM blocks into signed 16-bit mono PCM.
// Loop flags are retained in the source VAG but do not duplicate looped audio;
// callers can loop playback using LoopStart and LoopEnd.
func (v *VAG) DecodePCM() ([]int16, error) {
	pcm := make([]int16, 0, len(v.Data)/16*28)
	var previous1, previous2 int32
	for offset := 0; offset < len(v.Data); offset += 16 {
		block := v.Data[offset : offset+16]
		predictor := int(block[0] >> 4)
		shift := uint(block[0] & 0x0f)
		if predictor >= len(vagPredictorCoefficients) || shift > 12 {
			return nil, fmt.Errorf("psx: invalid VAG ADPCM header 0x%02x at block %d", block[0], offset/16)
		}
		coeff := vagPredictorCoefficients[predictor]
		for _, packed := range block[2:] {
			for _, nibble := range []byte{packed & 0x0f, packed >> 4} {
				signed := int32(nibble)
				if signed >= 8 {
					signed -= 16
				}
				sample := (signed << 12) >> shift
				sample += (previous1*coeff[0] + previous2*coeff[1] + 32) >> 6
				if sample > 32767 {
					sample = 32767
				} else if sample < -32768 {
					sample = -32768
				}
				pcm = append(pcm, int16(sample))
				previous2, previous1 = previous1, sample
			}
		}
	}
	return pcm, nil
}

// LoopRange returns the decoded PCM sample range marked by SPU ADPCM loop
// flags. ok is false when the sound has no complete loop marker pair.
func (v *VAG) LoopRange() (start, end int, ok bool) {
	start = -1
	for offset := 0; offset < len(v.Data); offset += 16 {
		flags := v.Data[offset+1]
		if flags&0x04 != 0 {
			start = offset / 16 * 28
		}
		if flags&0x02 != 0 && start >= 0 {
			return start, (offset/16 + 1) * 28, true
		}
	}
	return 0, 0, false
}
