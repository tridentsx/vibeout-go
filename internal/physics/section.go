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
	best, distance, err := NearestTrackSection(ship.Position, track.Sections, current)
	if err != nil {
		return 0, err
	}
	ship.PreviousSectionID = int16(current)
	ship.SectionID = int16(best)
	return distance, nil
}

// NearestTrackSection is UpdateShipTrackSection's graph search, factored out
// to take a bare world position instead of a *Ship so non-ship trackers (the
// chase camera; see render.RaceCamera) can follow the same junction-aware
// path the ship's own physics does. A junction-blind search that only ever
// follows .Next/.Previous silently tracks the *other* branch of a fork
// (WipEout's parallel-section forks, e.g. a boost-pad shortcut) whenever the
// tracked entity actually took the branch through .NextJunction, which is
// exactly what an earlier camera-only reimplementation of this search did:
// the chase camera visibly drifted ahead of the ship across a fork and ended
// up in front of it by the time the branches rejoined.
func NearestTrackSection(position Vector3, sections []assets.TrackSection, current int) (int, float32, error) {
	if !validSectionIndex(current, sections) {
		return 0, 0, fmt.Errorf("physics: section %d out of range", current)
	}

	candidate := current
	for range 3 {
		candidate = int(sections[candidate].Previous)
		if !validSectionIndex(candidate, sections) {
			return 0, 0, fmt.Errorf("physics: invalid Previous link in section search")
		}
	}
	initialFlags := sections[candidate].Flags
	best := candidate
	bestDistanceSquared := sectionDistanceSquared(position, sections[candidate])
	junction := -1

	for range 6 {
		candidate = int(sections[candidate].Next)
		if !validSectionIndex(candidate, sections) {
			return 0, 0, fmt.Errorf("physics: invalid Next link in section search")
		}
		if link := int(sections[candidate].NextJunction); link != -1 {
			if !validSectionIndex(link, sections) {
				return 0, 0, fmt.Errorf("physics: section %d has invalid junction link %d", candidate, link)
			}
			junction = link
		}
		best, bestDistanceSquared = nearerSection(position, sections, candidate, best, bestDistanceSquared)
	}

	if junction != -1 {
		candidate = junction
		if sections[junction].Flags&assets.TrackSectionJunctionStart == 0 && initialFlags == assets.TrackSectionJunction {
			candidate = int(sections[candidate].Next)
			if !validSectionIndex(candidate, sections) {
				return 0, 0, fmt.Errorf("physics: invalid junction entry Next link")
			}
		}
		for range 6 {
			best, bestDistanceSquared = nearerSection(position, sections, candidate, best, bestDistanceSquared)
			if sections[candidate].Flags&assets.TrackSectionJunctionEnd != 0 {
				candidate = int(sections[candidate].Previous)
			} else {
				candidate = int(sections[candidate].Next)
			}
			if !validSectionIndex(candidate, sections) {
				return 0, 0, fmt.Errorf("physics: invalid link in junction section search")
			}
		}
	}

	return best, float32(math.Sqrt(float64(bestDistanceSquared))), nil
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
