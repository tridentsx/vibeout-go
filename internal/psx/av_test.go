package psx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeAVCorpus(t *testing.T) {
	wants := map[string]struct{ frames, audio, padding int }{
		"MAKE.AV":  {1714, 1287, 0},
		"XTRO1.AV": {540, 405, 0},
		"XTRO2.AV": {500, 376, 11},
		"XTRO3.AV": {650, 488, 7},
		"XTRO4.AV": {800, 601, 10},
	}
	root := filepath.Dir(wipeoutDiscPath)
	for name, want := range wants {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Skip("disc image not present:", err)
		}
		av, err := DecodeAV(data)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(av.Frames) != want.frames || len(av.AudioSectors) != want.audio || av.PaddingSectors != want.padding {
			t.Errorf("%s: got %d frames/%d audio/%d padding, want %d/%d/%d", name, len(av.Frames), len(av.AudioSectors), av.PaddingSectors, want.frames, want.audio, want.padding)
		}
		pcm, err := av.DecodeXA4BitStereo()
		if err != nil {
			t.Errorf("%s audio: %v", name, err)
		} else if len(pcm) != want.audio*16*112*2 {
			t.Errorf("%s: decoded %d audio samples", name, len(pcm))
		}
	}
}

func TestDecodeAVRejectsNonSectorData(t *testing.T) {
	if _, err := DecodeAV(make([]byte, avSectorSize-1)); err == nil {
		t.Fatal("expected sector alignment error")
	}
}

func TestDecodeAVVideoFrameSamples(t *testing.T) {
	root := filepath.Dir(wipeoutDiscPath)
	for _, name := range []string{"MAKE.AV", "XTRO1.AV", "XTRO2.AV", "XTRO3.AV", "XTRO4.AV"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Skip("disc image not present:", err)
		}
		av, err := DecodeAV(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, frameIndex := range []int{0, len(av.Frames) / 2, len(av.Frames) - 1} {
			frame := &av.Frames[frameIndex]
			img, err := frame.DecodeRGBA()
			if err != nil {
				t.Errorf("%s frame %d: %v", name, frameIndex, err)
				continue
			}
			if img.Bounds().Dx() != int(frame.Header.Width) || img.Bounds().Dy() != int(frame.Header.Height) {
				t.Errorf("%s frame %d: decoded dimensions %v", name, frameIndex, img.Bounds())
			}
		}
	}
}
