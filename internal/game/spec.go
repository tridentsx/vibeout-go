package game

import "fmt"

const ShipSpecCount = 16

// ShipSpec contains the five race-setup values confirmed to feed the ship
// physics and steering paths. Difficulty is the raceConfig+8 value 0..3;
// slot is the original sixteen-entry ship-array index.
type ShipSpec struct {
	InertiaFactor   float32
	MaxSpeed        float32
	DragCoefficient float32
	GroundedSpring  float32
	TurnAccel       float32
	TurnRate        float32
}

var shipInertia = [4][ShipSpecCount]int16{
	{155, 155, 155, 160, 160, 160, 165, 165, 165, 130, 130, 130, 110, 110, 110, 150},
	{155, 155, 155, 160, 160, 160, 165, 165, 165, 130, 130, 130, 110, 110, 110, 150},
	{155, 155, 155, 160, 160, 160, 165, 165, 165, 130, 130, 130, 110, 110, 110, 150},
	{155, 155, 155, 160, 160, 160, 165, 165, 165, 130, 130, 130, 110, 110, 110, 150},
}

var shipMaxSpeed = [4][ShipSpecCount]int16{
	{800, 800, 800, 900, 900, 900, 950, 950, 950, 700, 700, 700, 750, 750, 750, 800},
	{1050, 1050, 1050, 1220, 1220, 1220, 1320, 1320, 1320, 820, 820, 820, 900, 900, 900, 1100},
	{1450, 1300, 1450, 1550, 1620, 1520, 1750, 1800, 1700, 1130, 1160, 1050, 1400, 1500, 1200, 1400},
	{1775, 1775, 1775, 2000, 2000, 2000, 2050, 2050, 2050, 1350, 1350, 1350, 1600, 1600, 1600, 1800},
}

var shipDrag = [4][ShipSpecCount]int16{
	{135, 135, 135, 140, 140, 140, 145, 145, 145, 130, 130, 130, 145, 145, 145, 134},
	{135, 135, 135, 140, 140, 140, 145, 145, 145, 130, 130, 130, 145, 145, 145, 134},
	{145, 145, 145, 150, 150, 150, 155, 155, 155, 140, 140, 140, 155, 155, 155, 130},
	{145, 145, 145, 150, 150, 150, 155, 155, 155, 140, 140, 140, 155, 155, 155, 130},
}

// shipGroundedSpring is loaded into ship+0xa6 by
// InitializeRaceShipsAndStartingGrid. IntegrateShipPhysicsAndTrackContact
// adds one quarter of the combined air-brake ramps to it and uses the result
// as the grounded velocity-redirect spring divisor.
var shipGroundedSpring = [4][ShipSpecCount]int16{
	{10, 10, 10, 12, 12, 12, 14, 14, 14, 8, 8, 8, 4, 4, 4, 12},
	{10, 10, 10, 12, 12, 12, 14, 14, 14, 8, 8, 8, 4, 4, 4, 12},
	{8, 8, 8, 10, 10, 10, 10, 10, 10, 6, 6, 6, 2, 2, 2, 8},
	{6, 6, 6, 8, 8, 8, 8, 8, 8, 6, 6, 6, 2, 2, 2, 8},
}

var shipTurnAccel = [4][ShipSpecCount]int16{
	{190, 190, 190, 175, 175, 175, 155, 155, 155, 200, 200, 200, 230, 230, 230, 190},
	{190, 190, 190, 175, 175, 175, 155, 155, 155, 200, 200, 200, 230, 230, 230, 190},
	{190, 190, 190, 175, 175, 175, 155, 155, 155, 200, 200, 200, 230, 230, 230, 190},
	{190, 190, 190, 175, 175, 175, 155, 155, 155, 200, 200, 200, 230, 230, 230, 190},
}

var shipTurnRate = [4][ShipSpecCount]int16{
	{2560, 2560, 2560, 2420, 2420, 2420, 2180, 2180, 2180, 2620, 2620, 2620, 2700, 2700, 2700, 2560},
	{2560, 2560, 2560, 2420, 2420, 2420, 2180, 2180, 2180, 2620, 2620, 2620, 2700, 2700, 2700, 2560},
	{2560, 2560, 2560, 2420, 2420, 2420, 2180, 2180, 2180, 2620, 2620, 2620, 2700, 2700, 2700, 2560},
	{2560, 2560, 2560, 2420, 2420, 2420, 2180, 2180, 2180, 2620, 2620, 2620, 2700, 2700, 2700, 2560},
}

func RaceShipSpec(difficulty, slot int) (ShipSpec, error) {
	if difficulty < 0 || difficulty >= len(shipInertia) {
		return ShipSpec{}, fmt.Errorf("game: difficulty %d out of range", difficulty)
	}
	if slot < 0 || slot >= ShipSpecCount {
		return ShipSpec{}, fmt.Errorf("game: ship spec slot %d out of range", slot)
	}
	return ShipSpec{
		InertiaFactor:   float32(shipInertia[difficulty][slot]),
		MaxSpeed:        float32(shipMaxSpeed[difficulty][slot]),
		DragCoefficient: float32(shipDrag[difficulty][slot]),
		GroundedSpring:  float32(shipGroundedSpring[difficulty][slot]),
		TurnAccel:       float32(int32(shipTurnAccel[difficulty][slot]) * 60 / 50),
		TurnRate:        float32(int32(shipTurnRate[difficulty][slot]) * 60 / 50),
	}, nil
}

func ApplyRaceShipSpec(ship *Ship, spec ShipSpec) {
	ship.InertiaFactor = spec.InertiaFactor
	ship.MaxSpeed = spec.MaxSpeed
	ship.DragCoefficient = spec.DragCoefficient
	ship.GroundedSpring = spec.GroundedSpring
	ship.TurnAccel = spec.TurnAccel
	ship.TurnRate = spec.TurnRate
}
