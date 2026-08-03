package psx

import (
	"encoding/binary"
	"fmt"
)

const (
	avSectorSize       = 2048
	avVideoHeaderSize  = 32
	avVideoSectorMagic = 0x80010160
)

// AVVideoHeader is the 32-byte header at the start of each video sector in
// the game's .AV streams. It is the conventional PlayStation STR/MDEC sector
// header stored in little endian.
type AVVideoHeader struct {
	ChunkIndex  uint16
	ChunkCount  uint16
	FrameNumber uint32
	DemuxedSize uint32
	Width       uint16
	Height      uint16
	MDECWords   uint16
	MDECMagic   uint16
	QuantScale  uint16
	Version     uint16
}

// AVFrame is one reassembled MDEC bitstream. Data contains exactly
// DemuxedSize bytes; sector padding is discarded.
type AVFrame struct {
	Header AVVideoHeader
	Data   []byte
}

// AV contains reassembled video frames and the interleaved cooked XA ADPCM
// sectors. The source alternates seven video sectors with one audio sector.
type AV struct {
	Frames       []AVFrame
	AudioSectors [][]byte
	// PaddingSectors counts all-zero unused video slots after the last frame.
	PaddingSectors int
}

// DecodeAV demultiplexes a WipEout .AV file. Files are concatenations of
// 2048-byte cooked sectors: video sectors begin with 0x80010160 and carry
// 2016 payload bytes, while every eighth sector is headerless XA ADPCM.
func DecodeAV(data []byte) (*AV, error) {
	if len(data) == 0 || len(data)%avSectorSize != 0 {
		return nil, fmt.Errorf("psx: AV size %d is not a non-zero multiple of %d", len(data), avSectorSize)
	}
	result := &AV{}
	var frame *AVFrame
	for sectorIndex := 0; sectorIndex < len(data)/avSectorSize; sectorIndex++ {
		sector := data[sectorIndex*avSectorSize : (sectorIndex+1)*avSectorSize]
		if binary.LittleEndian.Uint32(sector[:4]) != avVideoSectorMagic {
			if sectorIndex%8 != 7 {
				allZero := true
				for _, value := range sector {
					if value != 0 {
						allZero = false
						break
					}
				}
				if allZero && frame == nil {
					result.PaddingSectors++
					continue
				}
				return nil, fmt.Errorf("psx: AV sector %d lacks video magic outside the audio slot", sectorIndex)
			}
			result.AudioSectors = append(result.AudioSectors, append([]byte(nil), sector...))
			continue
		}
		header := AVVideoHeader{
			ChunkIndex:  binary.LittleEndian.Uint16(sector[4:6]),
			ChunkCount:  binary.LittleEndian.Uint16(sector[6:8]),
			FrameNumber: binary.LittleEndian.Uint32(sector[8:12]),
			DemuxedSize: binary.LittleEndian.Uint32(sector[12:16]),
			Width:       binary.LittleEndian.Uint16(sector[16:18]),
			Height:      binary.LittleEndian.Uint16(sector[18:20]),
			MDECWords:   binary.LittleEndian.Uint16(sector[20:22]),
			MDECMagic:   binary.LittleEndian.Uint16(sector[22:24]),
			QuantScale:  binary.LittleEndian.Uint16(sector[24:26]),
			Version:     binary.LittleEndian.Uint16(sector[26:28]),
		}
		if header.ChunkCount == 0 || header.ChunkIndex >= header.ChunkCount {
			return nil, fmt.Errorf("psx: AV sector %d has invalid chunk %d/%d", sectorIndex, header.ChunkIndex, header.ChunkCount)
		}
		if header.Width == 0 || header.Height == 0 || header.MDECMagic != 0x3800 {
			return nil, fmt.Errorf("psx: AV sector %d has invalid MDEC header", sectorIndex)
		}
		if header.ChunkIndex == 0 {
			if frame != nil {
				return nil, fmt.Errorf("psx: AV frame %d ended before all chunks arrived", frame.Header.FrameNumber)
			}
			result.Frames = append(result.Frames, AVFrame{Header: header})
			frame = &result.Frames[len(result.Frames)-1]
		} else if frame == nil || header.FrameNumber != frame.Header.FrameNumber ||
			header.ChunkIndex != uint16(len(frame.Data)/2016) {
			return nil, fmt.Errorf("psx: AV sector %d is an out-of-order frame chunk", sectorIndex)
		}
		if header.ChunkCount != frame.Header.ChunkCount || header.DemuxedSize != frame.Header.DemuxedSize ||
			header.Width != frame.Header.Width || header.Height != frame.Header.Height {
			return nil, fmt.Errorf("psx: AV frame %d chunk headers disagree", header.FrameNumber)
		}
		frame.Data = append(frame.Data, sector[avVideoHeaderSize:]...)
		if header.ChunkIndex+1 == header.ChunkCount {
			if uint64(frame.Header.DemuxedSize) > uint64(len(frame.Data)) {
				return nil, fmt.Errorf("psx: AV frame %d declares %d bytes but chunks hold %d", header.FrameNumber, frame.Header.DemuxedSize, len(frame.Data))
			}
			frame.Data = frame.Data[:frame.Header.DemuxedSize]
			frame = nil
		}
	}
	if frame != nil {
		return nil, fmt.Errorf("psx: AV final frame %d is incomplete", frame.Header.FrameNumber)
	}
	return result, nil
}

// DecodeXA4BitStereo decodes the headerless 2048-byte XA sectors used by AV.
// Each sector contains sixteen 128-byte XA sound groups. The returned samples
// are interleaved signed 16-bit left/right PCM at the stream's playback rate.
func (av *AV) DecodeXA4BitStereo() ([]int16, error) {
	pcm := make([]int16, 0, len(av.AudioSectors)*16*112*2)
	var previous1, previous2 [2]int32
	for sectorIndex, sector := range av.AudioSectors {
		if len(sector) != avSectorSize {
			return nil, fmt.Errorf("psx: AV audio sector %d has %d bytes", sectorIndex, len(sector))
		}
		for groupOffset := 0; groupOffset < len(sector); groupOffset += 128 {
			group := sector[groupOffset : groupOffset+128]
			for pair := 0; pair < 4; pair++ {
				var decoded [2][28]int16
				for channel := 0; channel < 2; channel++ {
					unit := pair*2 + channel
					parameterIndex := unit
					if unit >= 4 {
						parameterIndex += 4
					}
					parameter := group[parameterIndex]
					filter, shift := int(parameter>>4), uint(parameter&0x0f)
					if filter >= len(vagPredictorCoefficients) || shift > 12 {
						return nil, fmt.Errorf("psx: invalid XA parameter 0x%02x in audio sector %d", parameter, sectorIndex)
					}
					coeff := vagPredictorCoefficients[filter]
					for sampleIndex := 0; sampleIndex < 28; sampleIndex++ {
						packed := group[16+sampleIndex*4+unit/2]
						nibble := packed & 0x0f
						if unit&1 != 0 {
							nibble = packed >> 4
						}
						signed := int32(nibble)
						if signed >= 8 {
							signed -= 16
						}
						value := (signed << 12) >> shift
						value += (previous1[channel]*coeff[0] + previous2[channel]*coeff[1] + 32) >> 6
						if value > 32767 {
							value = 32767
						} else if value < -32768 {
							value = -32768
						}
						decoded[channel][sampleIndex] = int16(value)
						previous2[channel], previous1[channel] = previous1[channel], value
					}
				}
				for i := 0; i < 28; i++ {
					pcm = append(pcm, decoded[0][i], decoded[1][i])
				}
			}
		}
	}
	return pcm, nil
}
