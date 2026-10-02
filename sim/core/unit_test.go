package core

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core/stats"
)

func TestBlockValue(t *testing.T) {
	for _, c := range []struct {
		name       string
		unitType   UnitType
		stat, mult float64
		want       float64
	}{
		{"player", PlayerUnit, 357, 1, 347},
		{"player, multiplier after the 10", PlayerUnit, 120, 1.15, 126},
		{"player under 10", PlayerUnit, 6, 2, 0},
		{"enemy", EnemyUnit, 41, 1, 41},
	} {
		unit := &Unit{Type: c.unitType}
		unit.stats[stats.BlockValue] = c.stat
		unit.PseudoStats.BlockValueMultiplier = c.mult
		if got := unit.BlockValue(); got != c.want {
			t.Errorf("%s: block value %v, want %v", c.name, got, c.want)
		}
	}
}
