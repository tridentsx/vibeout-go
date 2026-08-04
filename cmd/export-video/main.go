package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/tridentsx/wipeout-go/internal/psx"
	"github.com/tridentsx/wipeout-go/internal/video"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s INPUT.AV [OUTPUT.mkv]\n\nExports lossless FFV1 video and PCM audio in Matroska. Requires FFmpeg on PATH.\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 1 || flag.NArg() > 2 {
		flag.Usage()
		os.Exit(2)
	}
	input := flag.Arg(0)
	output := strings.TrimSuffix(input, filepath.Ext(input)) + ".mkv"
	if flag.NArg() == 2 {
		output = flag.Arg(1)
	}
	data, err := os.ReadFile(input)
	if err != nil {
		fatal(err)
	}
	movie, err := psx.DecodeAV(data)
	if err != nil {
		fatal(err)
	}
	fmt.Fprintf(os.Stderr, "Exporting %d frames and %d audio sectors to %s (lossless FFV1 + PCM)...\n", len(movie.Frames), len(movie.AudioSectors), output)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := video.ExportMKV(ctx, movie, output); err != nil {
		fatal(err)
	}
	fmt.Fprintln(os.Stderr, "Export complete.")
}

func fatal(err error) { fmt.Fprintln(os.Stderr, "export-video:", err); os.Exit(1) }
