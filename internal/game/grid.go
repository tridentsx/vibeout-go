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

// GridSlotCount is how many craft take the grid. InitializeRaceShipsAndStartingGrid builds
// its permutation with a stride that depends on a flag, and the field size follows from it
// (0x80022ef4 and 0x80022f9c):
//
//	t2 = (gp[162] == 0) ? 4 : 5
//	for pass = 0..2: perm[pass*t2 .. +4] = {2,5,8,11,14} - pass
//	count = (t2 << 1) | t2        // t2*3, so 12 or 15
//
// gp[162] is set from the speed class being Phantom and from the circuit's section count,
// so an ordinary race runs **twelve** craft and only Phantom runs fifteen. That matters for
// placement because the player takes the last slot: 11 in a normal race, 14 in Phantom.
//
// Confirmed against the authored pads. Talon's Reach has six pads in a zig-zag on sections
// 265, 267, 269, 271, 273 and 275. Twelve slots put the rearmost craft on section 271, which
// is the fourth pad counting from the back of the grid, leaving the three behind it empty --
// exactly what retail shows. Fifteen slots would fill all six and put the player on the
// rearmost, which it does not.
func GridSlotCount(speedClass SpeedClass) int {
	if speedClass >= 3 {
		return 15
	}
	return 12
}

// PlayerGridSlot is the slot the player's craft starts in, given how many slots
// the track's grid holds.
//
// In a single race the player starts in the last slot. The permutation compaction at
// 0x80022fcc, read instruction by instruction, is:
//
//	for (s2 = 0; s2 < count; s2++) {
//	    v = perm[s2];
//	    if (v == pilot1 || v == pilot2) continue;   // both, unconditionally
//	    gridPosition[v] = a0++;
//	}
//	if (mode == 2) gridPosition[pilot1] = a0++;     // single player: one placed back
//
// The loop skips both human pilot entries but a single race places only one of them back,
// so the counter falls two short of the field size and the player takes `slots - 2`. With
// twelve craft that is slot 10.
//
// The grid map settles this. Talon's Reach has a pad on each of sections 275, 273, 271, 269,
// 267 and 265, alternating left, right, left, right, left, right, and the stride-two walk
// gives them slots 9 to 14. Retail puts the player on a right-hand pad with left, right, left
// on the pads behind, which is section 273 and no other: 273 is right, and 271, 269 and 267
// behind it are left, right, left. Section 273 is slot 10.
//
// Getting here took two wrong turns worth recording. Slot 14 (`slots - 1` with a fifteen-craft
// field) is a right-hand pad too, but it is the rearmost, with nothing behind it. Slot 11
// (`slots - 1` with twelve) is a left-hand pad. Only slot 10 matches both the lane and what is
// behind it. The other modes do not: championship and challenge assign
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
	if slots < 3 {
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
		// The side flag and the face scan both read as follows at instruction level:
		//
		//	side[s2] = a0; a0 ^= 1        ; 0x80023984, byte array at sp+120, a0 starts 0
		//	scan: v0 = a3[15] & 1
		//	      if (v0 == 0) { a3 += 20; goto scan }   ; the delay slot runs every pass,
		//	                                            ; so a3 exits one PAST the match
		//	      a3 -= 20                              ; delay slot, always -> the match
		//	      if (side[gridPosition] != 0) a3 += 20
		//
		// Read literally that means an even slot takes the flagged face. The authored pads
		// say otherwise, and they are the better evidence because they are data rather than
		// an interpretation: tile 1 marks the starting pads, and on TRACK01 the six rearmost
		// slots land on sections 275, 273, 271, 269, 267 and 265, each with a pad in exactly
		// one lane. Advancing on an *even* slot puts all six on their pad; the literal
		// reading puts none of them on one. Six for six against zero for six is not a
		// coincidence, and slot 14 then sits on section 265's pad 855 units right of centre,
		// which is the side retail starts on.
		//
		// So the parity here is inverted relative to the disassembly, and the likely reason
		// is not the parity at all but the *order of faces within a section*: if retail's
		// runtime face array runs right-to-left where the decoded .TRF runs left-to-right,
		// "the first flagged face" means opposite lanes in the two. That would explain the
		// discrepancy without either reading being wrong. Confirming it needs the runtime
		// face order, which is an emulator question.
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
