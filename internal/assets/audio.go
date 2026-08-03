package assets

import (
	"github.com/tridentsx/wipeout-go/internal/audio/music"
	"github.com/tridentsx/wipeout-go/internal/audio/sfx"
)

func (l Loader) LoadSFX(wadName, sampleName string, loop bool) (sfx.Clip, error) {
	vag, err := l.LoadVAG(wadName, sampleName)
	if err != nil {
		return sfx.Clip{}, err
	}
	pcm, err := vag.DecodePCM()
	if err != nil {
		return sfx.Clip{}, err
	}
	return sfx.Clip{Samples: pcm, SampleRate: int(vag.SampleRate), Loop: loop}, nil
}

// LoadMovieAudio exposes an AV soundtrack through the music-domain type.
// XA sectors in retail movies are stereo at 37,800 Hz.
func (l Loader) LoadMovieAudio(name string) (music.Track, error) {
	av, err := l.LoadAV(name)
	if err != nil {
		return music.Track{}, err
	}
	pcm, err := av.DecodeXA4BitStereo()
	if err != nil {
		return music.Track{}, err
	}
	return music.Track{Name: name, Samples: pcm, SampleRate: 37800, Channels: 2}, nil
}
