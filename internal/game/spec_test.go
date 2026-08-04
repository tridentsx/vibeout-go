package game

import "testing"

func TestRaceShipSpecUsesExecutableTables(t *testing.T) {
	spec, err := RaceShipSpec(2, 7)
	if err != nil {
		t.Fatal(err)
	}
	if spec.InertiaFactor != 165 || spec.MaxSpeed != 1800 || spec.DragCoefficient != 155 || spec.TurnAccel != 186 || spec.TurnRate != 2616 {
		t.Fatalf("difficulty 2 slot 7 spec = %+v", spec)
	}
}

func TestApplyRaceShipSpec(t *testing.T) {
	ship := &Ship{}
	spec, _ := RaceShipSpec(0, 0)
	ApplyRaceShipSpec(ship, spec)
	if ship.InertiaFactor != 155 || ship.MaxSpeed != 800 || ship.DragCoefficient != 135 || ship.TurnAccel != 228 || ship.TurnRate != 3072 {
		t.Fatalf("applied spec = %+v", ship)
	}
}
