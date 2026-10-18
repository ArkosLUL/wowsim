package shaman

import (
	"slices"

	"github.com/wowsims/wotlk/sim/core"
)

// chainHealJumps is Spell::SearchChainTargets for a chain heal: each jump goes to the unit with the largest
// health deficit the chain hasn't healed yet, and the chain stops when no one else is hurt. The sim has no
// positions, so everyone counts as in jump range and sight.
func chainHealJumps(sim *core.Simulation, target *core.Unit, jumps []*core.Unit, maxJumps int) []*core.Unit {
	for len(jumps) < maxJumps {
		var next *core.Unit
		var maxDeficit uint32
		for _, unit := range sim.Raid.AllUnits {
			// a pet that's away isn't in the world to jump to
			if unit == target || !unit.IsEnabled() || !unit.HasHealthBar() || slices.Contains(jumps, unit) {
				continue
			}
			cur, most := unit.CurrentHealth(), unit.MaxHealth()
			// SpellInfo::CheckTarget keeps the dead out of the search
			if cur >= most || cur <= 0 {
				continue
			}
			// whole health on the server, and a tie keeps the unit found first
			if deficit := uint32(most) - uint32(cur); deficit > maxDeficit {
				next, maxDeficit = unit, deficit
			}
		}
		if next == nil {
			break
		}
		jumps = append(jumps, next)
	}
	return jumps
}
