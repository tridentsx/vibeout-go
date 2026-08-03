// Package music owns long-running soundtrack playback independently from
// short sound effects. Tracks can therefore be streamed and faded without
// coupling those policies to gameplay events.
package music

import (
	"path/filepath"
	"sort"
)

type Track struct {
	Name       string
	Path       string
	Samples    []int16
	SampleRate int
	Channels   int
}

// Library discovers the extracted CD-audio soundtrack independently from
// game data and sound-effect WADs.
type Library struct{ Root string }

func (l Library) Tracks() ([]Track, error) {
	paths, err := filepath.Glob(filepath.Join(l.Root, "*.flac"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	tracks := make([]Track, len(paths))
	for i, path := range paths {
		tracks[i] = Track{Name: filepath.Base(path), Path: path, Channels: 2}
	}
	return tracks, nil
}

type Player interface {
	Play(Track) error
	Pause()
	Resume()
	Stop()
	SetVolume(float32)
}
