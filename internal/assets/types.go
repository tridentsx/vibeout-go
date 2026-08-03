package assets

import "github.com/tridentsx/wipeout-go/internal/psx"

// Runtime geometry aliases make the asset boundary explicit to consumers
// while preserving the binary-confirmed layouts supplied by psx decoders.
type TrackVertex = psx.TrackVertex
type TrackFace = psx.TrackFace
type TrackSection = psx.TrackSection
type Image = psx.Image

const TrackFaceTrack = psx.TrackFaceTrack
const TrackFaceBoost = psx.TrackFaceBoost
