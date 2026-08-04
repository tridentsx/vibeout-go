package video

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tridentsx/wipeout-go/internal/psx"
)

func TestExportMKVRejectsEmptyMovie(t *testing.T) {
	err := ExportMKV(context.Background(), &psx.AV{}, filepath.Join(t.TempDir(), "empty.mkv"))
	if err == nil || !strings.Contains(err.Error(), "no frames") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExportMKVRequiresMatroskaExtension(t *testing.T) {
	movie := &psx.AV{Frames: []psx.AVFrame{{}}}
	err := ExportMKV(context.Background(), movie, filepath.Join(t.TempDir(), "movie.mp4"))
	if err == nil || !strings.Contains(err.Error(), ".mkv") {
		t.Fatalf("unexpected error: %v", err)
	}
}
