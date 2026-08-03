package psx

import (
	"bytes"
	"fmt"
	"strings"
)

// TEXKind distinguishes the two unrelated file layouts sharing the .TEX
// extension on the disc.
type TEXKind uint8

const (
	TEXTextureList TEXKind = iota
	TEXFaceAttributes
)

// FaceTexture assigns the two bytes written by InitTrackTextures to offsets
// 14 and 15 of a 20-byte runtime track-face record. These offsets match
// TrackFace.Tile and TrackFace.Flags exactly.
type FaceTexture struct {
	Tile  uint8
	Flags uint8
}

// TEX contains either a CRLF-separated development texture-source list
// (SKY/SCENE/ICONS/LIBRARY and similarly named files) or TRACK.TEX's packed
// per-face tile/flag pairs.
type TEX struct {
	Kind       TEXKind
	Paths      []string
	FaceValues []FaceTexture
}

// DecodeTEX decodes both layouts found under the .TEX extension.
func DecodeTEX(data []byte) (*TEX, error) {
	if looksLikeTEXPathList(data) {
		trimmed := bytes.TrimRight(data, "\x00\x1a\r\n")
		if len(trimmed) == 0 {
			return nil, fmt.Errorf("psx: TEX texture list is empty")
		}
		lines := strings.Split(strings.ReplaceAll(string(trimmed), "\r\n", "\n"), "\n")
		paths := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				paths = append(paths, line)
			}
		}
		if len(paths) == 0 {
			return nil, fmt.Errorf("psx: TEX texture list has no paths")
		}
		return &TEX{Kind: TEXTextureList, Paths: paths}, nil
	}

	if len(data) == 0 || len(data)%2 != 0 {
		return nil, fmt.Errorf("psx: binary TEX length %d is not a non-zero multiple of 2", len(data))
	}
	values := make([]FaceTexture, len(data)/2)
	for i := range values {
		values[i] = FaceTexture{Tile: data[i*2], Flags: data[i*2+1]}
	}
	return &TEX{Kind: TEXFaceAttributes, FaceValues: values}, nil
}

func looksLikeTEXPathList(data []byte) bool {
	if len(data) < 3 || !bytes.Contains(data, []byte{'\\'}) {
		return false
	}
	for _, b := range data {
		if b != 0 && b != 0x1a && b != '\r' && b != '\n' && (b < 0x20 || b > 0x7e) {
			return false
		}
	}
	return true
}
