package assets

import "github.com/tridentsx/wipeout-go/internal/psx"

// Runtime geometry aliases make the asset boundary explicit to consumers
// while preserving the binary-confirmed layouts supplied by psx decoders.
type TrackVertex = psx.TrackVertex
type TrackFace = psx.TrackFace
type TrackSection = psx.TrackSection
type Image = psx.Image
type Object = psx.Object
type Polygon = psx.Polygon
type Color = psx.Color
type ObjectHeader = psx.ObjectHeader

const TrackFaceTrack = psx.TrackFaceTrack
const TrackFaceBoost = psx.TrackFaceBoost

const TrackSectionJunctionEnd = psx.TrackSectionJunctionEnd
const TrackSectionJunctionStart = psx.TrackSectionJunctionStart
const TrackSectionJunction = psx.TrackSectionJunction
const TrackSectionJump = psx.TrackSectionJump

const PolygonSpriteTopAnchor = psx.PolygonSpriteTopAnchor
const PolygonSpriteBottomAnchor = psx.PolygonSpriteBottomAnchor
