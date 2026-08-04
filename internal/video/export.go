// Package video owns cutscene export and, eventually, runtime playback.
package video

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tridentsx/wipeout-go/internal/psx"
)

const (
	CutsceneFrameRate = "225/16" // 14.0625 fps: four frames per three stripped XA sectors
	CutsceneAudioRate = 18900
)

// ExportMKV writes a lossless, single-file conversion suitable for Topaz
// Video, Video2X, FFmpeg, and other modern tools. Video is encoded as FFV1 and
// audio remains uncompressed signed 16-bit stereo PCM in a Matroska container.
// FFmpeg must be installed and available on PATH.
func ExportMKV(ctx context.Context, movie *psx.AV, outputPath string) error {
	if movie == nil || len(movie.Frames) == 0 {
		return fmt.Errorf("video: movie has no frames")
	}
	if filepath.Ext(outputPath) != ".mkv" {
		return fmt.Errorf("video: lossless export path must end in .mkv")
	}
	if _, err := os.Stat(outputPath); err == nil {
		return fmt.Errorf("video: output already exists: %s", outputPath)
	} else if !os.IsNotExist(err) {
		return err
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("video: FFmpeg is required for MKV export: %w", err)
	}

	pcm, err := movie.DecodeXA4BitStereo()
	if err != nil {
		return err
	}
	audio, err := os.CreateTemp("", "wipeout-av-*.pcm")
	if err != nil {
		return err
	}
	audioPath := audio.Name()
	defer os.Remove(audioPath)
	w := bufio.NewWriter(audio)
	for _, sample := range pcm {
		if err := binary.Write(w, binary.LittleEndian, sample); err != nil {
			audio.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		audio.Close()
		return err
	}
	if err := audio.Close(); err != nil {
		return err
	}

	first := movie.Frames[0].Header
	temporaryOutput, err := os.CreateTemp(filepath.Dir(outputPath), ".wipeout-export-*.mkv")
	if err != nil {
		return err
	}
	temporaryPath := temporaryOutput.Name()
	if err := temporaryOutput.Close(); err != nil {
		return err
	}
	_ = os.Remove(temporaryPath)
	defer os.Remove(temporaryPath)

	cmd := exec.CommandContext(ctx, ffmpeg,
		"-v", "error", "-f", "rawvideo", "-pixel_format", "rgba", "-video_size", fmt.Sprintf("%dx%d", first.Width, first.Height), "-framerate", CutsceneFrameRate, "-i", "-",
		"-f", "s16le", "-ar", fmt.Sprint(CutsceneAudioRate), "-ac", "2", "-i", audioPath,
		"-map", "0:v:0", "-map", "1:a:0", "-c:v", "ffv1", "-level", "3", "-g", "1", "-c:a", "pcm_s16le", "-shortest", "-y", temporaryPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("video: start FFmpeg: %w", err)
	}
	writeErr := writeFrames(stdin, movie)
	closeErr := stdin.Close()
	waitErr := cmd.Wait()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if waitErr != nil {
		return fmt.Errorf("video: FFmpeg export failed: %w", waitErr)
	}
	if err := os.Rename(temporaryPath, outputPath); err != nil {
		return fmt.Errorf("video: finalize export: %w", err)
	}
	return nil
}

func writeFrames(dst io.Writer, movie *psx.AV) error {
	width, height := movie.Frames[0].Header.Width, movie.Frames[0].Header.Height
	for i := range movie.Frames {
		frame := &movie.Frames[i]
		if frame.Header.Width != width || frame.Header.Height != height {
			return fmt.Errorf("video: frame %d changes dimensions", i)
		}
		img, err := frame.DecodeRGBA()
		if err != nil {
			return err
		}
		if _, err = dst.Write(img.Pix); err != nil {
			return fmt.Errorf("video: write frame %d: %w", i, err)
		}
	}
	return nil
}
