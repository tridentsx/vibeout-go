// Package physics contains deterministic simulation systems. Runtime state is
// owned by package game; physics only mutates that state.
package physics

import "github.com/tridentsx/wipeout-go/internal/game"

type Ship = game.Ship
type Vector3 = game.Vector3
type Angle = game.Angle

const ShipFlagRetired = game.ShipFlagRetired
const AngleFullTurn = game.AngleFullTurn
