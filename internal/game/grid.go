package game

import (
	"fmt"
	"math"

	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/psx"
)

// TrackStartLineSection is the per-track start/finish line, read from the table
// at SLES_003.27 0x8008f47c and indexed by the *internal* track ID (the sparse
// value from the menu-order mapping at 0x80094d50), not by menu order:
//
//	Talon's Reach   ID  1 -> section   5
//	Sagarmatha      ID  2 -> section  87
//	Odessa Keys     ID  6 -> section  32
//	Phenitia Park   ID  7 -> section 371
//	Gare d'Europa   ID  8 -> section   6
//	Vostok Island   ID 13 -> section  16
//	Valparaiso      ID 17 -> section  12
//	Spilskinanke    ID 20 -> section  22
//
// This is the *line*, not the grid: on TRACK01 section 5 is one of the two
// sections carrying checkpoint faces (the other is 139), while the starting grid
// is the run of sections whose faces carry TrackFaceStartGrid -- 290 to 319.
// Placing a craft at the table value puts it on the line, ahead of the grid.
var TrackStartLineSection = map[uint8]int{
	1: 5, 2: 87, 6: 32, 7: 371, 8: 6, 13: 16, 17: 12, 20: 22,
}

// FindStartGridSection returns the section a lone craft should start in: the
// grid sits on the racing line immediately before the start/finish line, so this
// walks Previous back from that line.
//
// The anchor is TrackStartLineSection, which is what retail uses: the grid loop
// first scans sections for the one whose id matches the per-track table, then
// walks back from it. TrackFaceAlternateRoute is deliberately not used -- that bit
// was briefly taken for a start-grid marker, but on TRACK01 its run (sections
// 290..319) lies on the far branch of a fork and never on the racing line, so
// craft placed there started on a bypass.
func FindStartGridSection(track *assets.Track, lineSection, slot int) (int, bool) {
	if track == nil || lineSection < 0 || lineSection >= len(track.Sections) {
		return 0, false
	}
	// InitializeRaceShipsAndStartingGrid (0x80022bbc) walks Previous *once* to reach
	// slot 0, then twice per subsequent slot:
	//
	//	a3_1 = *(section + 4)              // Previous -> slot 0
	//	do { slots[i] = a3_1
	//	     a3_1 = *(*(a3_1 + 4) + 4)     // prev(prev(...)) -> next slot
	//	     side[i] = flag; flag ^= 1     // alternating side
	//	} while (i < 0xf)                  // 15 slots
	//
	// so slot k sits 2k+1 sections back, not 2k+2. It also alternates which side of
	// the road each slot uses -- the staggered F1-style grid -- with lateral offsets
	// from a {16, 32, 64} table copied to the stack at entry. Only the along-track
	// position is reproduced here; the stagger is not yet applied.
	steps := slot*gridSlotSectionStride + 1
	si := lineSection
	for i := 0; i < steps; i++ {
		p := int(track.Sections[si].Previous)
		if p < 0 || p >= len(track.Sections) {
			return si, true
		}
		si = p
	}
	return si, true
}

// gridSlotSectionStride is how many sections apart consecutive grid slots sit,
// taken from InitializeRaceShipsAndStartingGrid's section walk.
const gridSlotSectionStride = 2

// PlayerGridSlot is the slot the player's craft starts in, given how many slots
// the track's grid holds.
//
// In a single race the player starts at `slots - 2`, one short of the back of the grid.
// That falls out of how InitializeRaceShipsAndStartingGrid compacts its permutation
// (0x80022fcc, read instruction by instruction):
//
//	for (s2 = 0; s2 < count; s2++) {
//	    v = perm[s2];
//	    if (v == pilot1 || v == pilot2) continue;   // both, unconditionally
//	    gridPosition[v] = a0++;
//	}
//	if (mode == 2) gridPosition[pilot1] = a0++;     // single player: one placed back
//
// The loop skips *both* human pilot entries with no test on the race mode, but a single
// race places only one of them afterwards. So with a fifteen-entry permutation the loop
// assigns 0..12, the counter reaches 13, and the player takes 13. Position 14 goes to
// nobody. Reading the loop as skipping one entry gave 14, and 14 is even, which put the
// craft in the left-hand lane -- the wrong side, as seen in play. The other modes do not: championship and challenge assign
// grid order from qualifying or from standings carried between races, which is
// what maybe_ShuffleRaceOrder (called from main) and
// maybe_AdvanceShuffledRaceOrderStep (called from maybe_ResetRaceCountdown)
// resolve before the grid is laid out -- hence PlaceShipOnStartingGrid taking an
// "already-resolved race order slot, not necessarily the ship's array index".
//
// Kept as a function so those modes have somewhere to hook in rather than
// needing the caller changed. It will need the race mode as a parameter once the
// port has one (the retail enum lives at maybe_RaceModeSelection: 0 single race,
// 1 time trial, 2/3 two-player, 4 challenge).
func PlayerGridSlot(slots int) int {
	if slots <= 0 {
		return 0
	}
	if slots == 1 {
		return 0
	}
	return slots - 2
}

// PlaceShipOnStartingGrid ports the position/orientation portion of
// InitializeRaceShipsAndStartingGrid (SLES_003.27 0x80022bbc). lineSection is the
// start/finish line from TrackStartLineSection, keyed by internal track ID; the
// grid is walked backwards from it. gridSlot is the already-resolved race order
// slot, not necessarily the ship's array index, and it selects both the section
// and which side of the road the craft sits on -- the staggered grid.
func PlaceShipOnStartingGrid(ship *Ship, track *assets.Track, lineSection, gridSlot int) error {
	if ship == nil || track == nil {
		return fmt.Errorf("game: starting grid requires a ship and track")
	}
	if lineSection < 0 || lineSection >= len(track.Sections) {
		return fmt.Errorf("game: line section %d out of range", lineSection)
	}
	if gridSlot < 0 || gridSlot >= 15 {
		return fmt.Errorf("game: grid slot %d out of range", gridSlot)
	}

	// Retail walks *backwards* from the start/finish line, not forwards:
	// InitializeRaceShipsAndStartingGrid (0x80022bbc) takes Previous once to reach
	// slot 0, then Previous twice per subsequent slot, so slot k sits 2k+1 sections
	// behind the line. Walking forwards instead put the craft past the line, and on
	// TRACK01 it also crossed the fork at sections 283/284 onto a bypass branch.
	sectionIndex := lineSection
	for i := 0; i < gridSlot*gridSlotSectionStride+1; i++ {
		previous := int(track.Sections[sectionIndex].Previous)
		if previous < 0 || previous >= len(track.Sections) {
			return fmt.Errorf("game: section %d has invalid previous link %d", sectionIndex, previous)
		}
		sectionIndex = previous
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
		// InitializeRaceShipsAndStartingGrid (0x80022bbc) scans forward from the
		// section's first face for one carrying flag bit 0x01, then advances a
		// single face only when the slot's side flag is set:
		//
		//	do { f = face->0x0f & 1; face += 0x14; } while (f == 0);
		//	face -= 0x14;
		//	if (sideFlag[slot] != 0) face += 0x14;
		//
		// The side flag and the face scan, both read at instruction level:
		//
		//	side[s2] = a0; a0 ^= 1        ; 0x80023984, byte array at sp+120, a0 starts 0
		//	...
		//	scan: v0 = a3[15] & 1
		//	      if (v0 == 0) { a3 += 20; goto scan }   ; the delay slot runs every pass,
		//	                                            ; so a3 exits one PAST the match
		//	      a3 -= 20                              ; delay slot, always -> the match
		//	      if (side[gridPosition] != 0) a3 += 20 ; only an odd slot advances
		//
		// So an even slot takes the flagged face and an odd slot the one after it, which is
		// what this originally did. On TRACK01 section 265 the two flagged faces have
		// centres 841 units left and 855 right of the section centre, and +X is to the
		// right, so an odd slot is the right-hand lane. With the player at slot 13 that is
		// where it belongs.
		//
		// This was briefly inverted to force the right-hand lane while the slot was still
		// believed to be 14. Inverting it was the wrong repair: the parity is faithful and
		// the slot was wrong. Note also that the face *centre* is what matters here, not
		// vertex 0 -- the position is midpoint(v0, v2), and reading vertex 0 offsets made
		// the flagged faces look like "centre and right" rather than "left and right".
		if gridSlot&1 != 0 {
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

// AngleFromDirection is the yaw retail derives with `-ratan2(dx, dz)`, used for the
// starting grid and for aiming trackside objects along the track.
func AngleFromDirection(dx, dz int32) Angle { return angleFromGridDirection(dx, dz) }

func angleFromGridDirection(dx, dz int32) Angle {
	radians := -math.Atan2(float64(dx), float64(dz))
	units := int64(math.Round(radians * float64(AngleFullTurn) / (2 * math.Pi)))
	units %= int64(AngleFullTurn)
	if units < 0 {
		units += int64(AngleFullTurn)
	}
	return Angle(units)
}
