package psx

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeTIMTrueColor16BPP(t *testing.T) {
	// Standard type-2 TIM: common header followed immediately by an image
	// block. Four bytes after the declared block exercise tolerated disc-file
	// padding without making it part of the image.
	data := make([]byte, 8+12+4+4)
	binary.LittleEndian.PutUint32(data[0:4], timMagic)
	binary.LittleEndian.PutUint32(data[4:8], ImageTrueColor16BPP)
	binary.LittleEndian.PutUint32(data[8:12], 16)
	binary.LittleEndian.PutUint16(data[16:18], 2)
	binary.LittleEndian.PutUint16(data[18:20], 1)
	binary.LittleEndian.PutUint16(data[20:22], 0x001f) // red
	binary.LittleEndian.PutUint16(data[22:24], 0x7c00) // blue

	image, err := DecodeTIM(data)
	if err != nil {
		t.Fatal(err)
	}
	if image.Width != 2 || image.Height != 1 {
		t.Fatalf("dimensions = %dx%d, want 2x1", image.Width, image.Height)
	}
	want := []byte{248, 0, 0, 255, 0, 0, 248, 255}
	for i := range want {
		if image.Pixels[i] != want[i] {
			t.Fatalf("pixels[%d] = %d, want %d; all pixels: %v", i, image.Pixels[i], want[i], image.Pixels)
		}
	}
}

func TestDecodeTIMPaletted4BPP(t *testing.T) {
	data := make([]byte, 8+12+32+12+2)
	binary.LittleEndian.PutUint32(data[0:4], timMagic)
	binary.LittleEndian.PutUint32(data[4:8], ImagePaletted4BPP)
	binary.LittleEndian.PutUint32(data[8:12], 44)
	binary.LittleEndian.PutUint16(data[16:18], 16)
	binary.LittleEndian.PutUint16(data[18:20], 1)
	binary.LittleEndian.PutUint16(data[22:24], 0x03e0) // palette index 1: green
	imageOffset := 52
	binary.LittleEndian.PutUint32(data[imageOffset:imageOffset+4], 14)
	binary.LittleEndian.PutUint16(data[imageOffset+8:imageOffset+10], 1)
	binary.LittleEndian.PutUint16(data[imageOffset+10:imageOffset+12], 1)
	binary.LittleEndian.PutUint16(data[imageOffset+12:imageOffset+14], 0x1111)

	image, err := DecodeTIM(data)
	if err != nil {
		t.Fatal(err)
	}
	if image.Width != 4 || image.Height != 1 {
		t.Fatalf("dimensions = %dx%d, want 4x1", image.Width, image.Height)
	}
	for i := 0; i < 4; i++ {
		pixel := image.Pixels[i*4 : i*4+4]
		if pixel[0] != 0 || pixel[1] != 248 || pixel[2] != 0 || pixel[3] != 255 {
			t.Fatalf("pixel %d = %v, want opaque green", i, pixel)
		}
	}
}

func TestDecodeTIMRejectsInvalidMagic(t *testing.T) {
	_, err := DecodeTIM(make([]byte, 20))
	if err == nil {
		t.Fatal("DecodeTIM accepted invalid magic")
	}
}

func TestDecodeEveryRealTIMFile(t *testing.T) {
	if _, err := os.Stat(wipeoutDiscPath); err != nil {
		t.Skip("disc image not present:", err)
	}
	count := 0
	err := filepath.Walk(wipeoutDiscPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.EqualFold(filepath.Ext(path), ".TIM") {
			return err
		}
		count++
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		image, err := DecodeTIM(data)
		if err != nil {
			t.Errorf("%s: %v", path, err)
		} else if len(image.Pixels) != image.Width*image.Height*4 {
			t.Errorf("%s: invalid RGBA length %d", path, len(image.Pixels))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 53 {
		t.Fatalf("decoded %d TIM files, want 53", count)
	}
}
