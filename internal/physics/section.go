package physics

import (
	"fmt"
	"math"

	"github.com/tridentsx/wipeout-go/internal/assets"
)

// UpdateShipTrackSection ports the nearest-section graph search at
// 0x80025674. It searches from three Previous links behind the current
// section through six Next links, then optionally examines six nodes on the
// encountered junction route. Y error is divided by four before squaring.
func UpdateShipTrackSection(ship *Ship, track *assets.Track) (float32, error) {
	if ship == nil || track == nil {
		return 0, fmt.Errorf("physics: section update requires a ship and track")
	}
	current := int(ship.SectionID)
	if !validSectionIndex(current, track.Sections) {
		return 0, fmt.Errorf("physics: section %d out of range", current)
	}
	previous := current

	candidate := current
	for range 3 {
		candidate = int(track.Sections[candidate].Previous)
		if !validSectionIndex(candidate, track.Sections) {
			return 0, fmt.Errorf("physics: invalid Previous link in section search")
		}
	}
	initialFlags := track.Sections[candidate].Flags
	best := candidate
	bestDistanceSquared := sectionDistanceSquared(ship.Position, track.Sections[candidate])
	junction := -1

	for range 6 {
		candidate = int(track.Sections[candidate].Next)
		if !validSectionIndex(candidate, track.Sections) {
			return 0, fmt.Errorf("physics: invalid Next link in section search")
		}
		if link := int(track.Sections[candidate].NextJunction); link != -1 {
			if !validSectionIndex(link, track.Sections) {
				return 0, fmt.Errorf("physics: section %d has invalid junction link %d", candidate, link)
			}
			junction = link
		}
		best, bestDistanceSquared = nearerSection(ship.Position, track.Sections, candidate, best, bestDistanceSquared)
	}

	if junction != -1 {
		candidate = junction
		if track.Sections[junction].Flags&assets.TrackSectionJunctionStart == 0 && initialFlags == assets.TrackSectionJunction {
			candidate = int(track.Sections[candidate].Next)
			if !validSectionIndex(candidate, track.Sections) {
				return 0, fmt.Errorf("physics: invalid junction entry Next link")
			}
		}
		for range 6 {
			best, bestDistanceSquared = nearerSection(ship.Position, track.Sections, candidate, best, bestDistanceSquared)
			if track.Sections[candidate].Flags&assets.TrackSectionJunctionEnd != 0 {
				candidate = int(track.Sections[candidate].Previous)
			} else {
				candidate = int(track.Sections[candidate].Next)
			}
			if !validSectionIndex(candidate, track.Sections) {
				return 0, fmt.Errorf("physics: invalid link in junction section search")
			}
		}
	}

	distance := float32(math.Sqrt(float64(bestDistanceSquared)))
	ship.PreviousSectionID = int16(previous)
	ship.SectionID = int16(best)
	return distance, nil
}

func nearerSection(position Vector3, sections []assets.TrackSection, candidate, best int, bestDistanceSquared float32) (int, float32) {
	distanceSquared := sectionDistanceSquared(position, sections[candidate])
	if distanceSquared < bestDistanceSquared {
		return candidate, distanceSquared
	}
	return best, bestDistanceSquared
}

func sectionDistanceSquared(position Vector3, section assets.TrackSection) float32 {
	dx := position.X - float32(section.X)
	dy := (position.Y - float32(section.Y)) / 4
	dz := position.Z - float32(section.Z)
	return dx*dx + dy*dy + dz*dz
}

func validSectionIndex(index int, sections []assets.TrackSection) bool {
	return index >= 0 && index < len(sections)
}
