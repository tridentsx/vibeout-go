package psx

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeVAG(block []byte) []byte {
	data := make([]byte, vagHeaderSize+len(block))
	copy(data, "VAGp")
	binary.BigEndian.PutUint32(data[4:8], 2)
	binary.BigEndian.PutUint32(data[12:16], uint32(len(block)))
	binary.BigEndian.PutUint32(data[16:20], 44100)
	copy(data[32:48], "test.aif")
	copy(data[vagHeaderSize:], block)
	return data
}

func TestDecodeVAG(t *testing.T) {
	block := make([]byte, 16)
	block[0] = 0x0c
	block[2] = 0x1f // low nibble -1, high nibble +1
	vag, err := DecodeVAG(makeVAG(block))
	if err != nil {
		t.Fatal(err)
	}
	if vag.SampleRate != 44100 || vag.Name != "test.aif" {
		t.Fatalf("unexpected header: %+v", vag)
	}
	pcm, err := vag.DecodePCM()
	if err != nil {
		t.Fatal(err)
	}
	if len(pcm) != 28 || pcm[0] != -1 || pcm[1] != 1 {
		t.Fatalf("unexpected PCM prefix/length: %v, %d", pcm[:2], len(pcm))
	}
}

func TestDecodeVAGCorpus(t *testing.T) {
	root := filepath.Join(wipeoutDiscPath, "SOUND", "SAMPLES.WAD")
	data, err := os.ReadFile(root)
	if err != nil {
		t.Skip("disc image not present:", err)
	}
	entries, err := DecodeWAD(data)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	trailingCount := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name, ".vag") {
			continue
		}
		count++
		vag, err := DecodeVAG(entry.Data)
		if err != nil {
			t.Errorf("%s: %v", entry.Name, err)
			continue
		}
		pcm, err := vag.DecodePCM()
		if err != nil {
			t.Errorf("%s PCM: %v", entry.Name, err)
		}
		if len(pcm) != len(vag.Data)/16*28 {
			t.Errorf("%s: decoded %d samples", entry.Name, len(pcm))
		}
		if len(vag.Trailing) != 0 {
			trailingCount++
			if entry.Name != "three.vag" || len(vag.Trailing) != 448 {
				t.Errorf("%s: unexpected %d trailing bytes", entry.Name, len(vag.Trailing))
			}
		}
	}
	if count != 39 {
		t.Fatalf("decoded %d VAG entries, want 39", count)
	}
	if trailingCount != 1 {
		t.Fatalf("found %d VAGs with ignored trailing data, want 1", trailingCount)
	}
}
