package game

import (
	"fmt"
	"math"

	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/psx"
)

// PlaceShipOnStartingGrid ports the position/orientation portion of
// InitializeRaceShipsAndStartingGrid (SLES_003.27 0x80022bbc). startSection
// is selected by a small executable table from the active track id; its
// confirmed value for TRACK01 is zero. gridSlot is the already-resolved race
// order slot, not necessarily the ship's array index.
func PlaceShipOnStartingGrid(ship *Ship, track *assets.Track, startSection, gridSlot int) error {
	if ship == nil || track == nil {
		return fmt.Errorf("game: starting grid requires a ship and track")
	}
	if startSection < 0 || startSection >= len(track.Sections) {
		return fmt.Errorf("game: start section %d out of range", startSection)
	}
	if gridSlot < 0 || gridSlot >= 15 {
		return fmt.Errorf("game: grid slot %d out of range", gridSlot)
	}

	sectionIndex := startSection
	for i := 0; i < gridSlot*2; i++ {
		next := int(track.Sections[sectionIndex].Next)
		if next < 0 || next >= len(track.Sections) {
			return fmt.Errorf("game: section %d has invalid next link %d", sectionIndex, next)
		}
		sectionIndex = next
	}
	section := track.Sections[sectionIndex]
	faceIndex, err := startingGridFace(track.Faces, section, gridSlot)
	if err != nil {
		return fmt.Errorf("game: section %d: %w", sectionIndex, err)
	}
	face := track.Faces[faceIndex]
	firstIndex, secondIndex := int(face.Indices[0]), int(face.Indices[2])
	if firstIndex >= len(track.Vertices) || secondIndex >= len(track.Vertices) {
		return fmt.Errorf("game: grid face %d has invalid vertices %d and %d", faceIndex, firstIndex, secondIndex)
	}
	// InitializeRaceShipsAndStartingGrid loads halfwords at runtime face
	// offsets +0 and +4: decoded .TRF indices 0 and 2.
	first, second := track.Vertices[firstIndex], track.Vertices[secondIndex]
	ship.Position = Vector3{
		X: float32(midpoint(first.X, second.X) + normalGridOffset(face.NormalX)),
		Y: float32(midpoint(first.Y, second.Y) + normalGridOffset(face.NormalY)),
		Z: float32(midpoint(first.Z, second.Z) + normalGridOffset(face.NormalZ)),
	}
	ship.SectionID = int16(sectionIndex)

	nextIndex := int(section.Next)
	if nextIndex < 0 || nextIndex >= len(track.Sections) {
		return fmt.Errorf("game: section %d has invalid next link %d", sectionIndex, nextIndex)
	}
	next := track.Sections[nextIndex]
	ship.Yaw = angleFromGridDirection(next.X-section.X, next.Z-section.Z)
	ship.Pitch, ship.Roll = 0, 0
	return nil
}

func startingGridFace(faces []assets.TrackFace, section assets.TrackSection, gridSlot int) (int, error) {
	begin, end := int(section.FirstFace), int(section.FirstFace)+int(section.NumFaces)
	if begin < 0 || begin >= len(faces) || end > len(faces) {
		return 0, fmt.Errorf("face range [%d:%d) out of range", begin, end)
	}
	for index := begin; index < end; index++ {
		if faces[index].Flags&psx.TrackFaceTrack == 0 {
			continue
		}
		// The executable stores 0x10,0x20,0x40 as little-endian halfwords
		// and indexes the resulting byte array by slot. Consequently even
		// slots use the following face while odd slots use this track face.
		if gridSlot&1 == 0 {
			index++
		}
		if index >= end {
			return 0, fmt.Errorf("alternating grid face falls outside section")
		}
		return index, nil
	}
	return 0, fmt.Errorf("no driving-surface face")
}

func midpoint(a, b int32) int32 { return int32((int64(a) + int64(b)) >> 1) }

func normalGridOffset(normal int16) int32 {
	return int32((int64(normal) * 75) >> 10)
}

func angleFromGridDirection(dx, dz int32) Angle {
	radians := -math.Atan2(float64(dx), float64(dz))
	units := int64(math.Round(radians * float64(AngleFullTurn) / (2 * math.Pi)))
	units %= int64(AngleFullTurn)
	if units < 0 {
		units += int64(AngleFullTurn)
	}
	return Angle(units)
}
