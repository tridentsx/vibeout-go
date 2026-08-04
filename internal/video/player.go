package video

import "context"

// Player is the future runtime cutscene-playback boundary. Implementations
// will synchronize decoded MDEC frames and XA audio without coupling playback
// to the SDL renderer, asset loader, or the offline exporter.
type Player interface {
	Play(context.Context, string) error
	Stop()
}

// TODO(video-playback): implement an SDL-backed streaming player. It should
// decode frames incrementally (not retain an entire movie as RGBA), queue XA
// PCM through SDL audio, use audio as the synchronization clock, support skip
// input and clean cancellation, and fall back to the lossless/modern exported
// MKV replacement when one is configured.
