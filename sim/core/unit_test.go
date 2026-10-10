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

// Unit::SpellHealingBonusTaken takes GetMaxPositiveAuraModifier and GetMaxNegativeAuraModifier of
// SPELL_AURA_MOD_HEALING_PCT, one of each, and multiplies them. Here: Improved Devotion Aura 6, Guardian
// Spirit 40, Spirit Bond 10, Mind Trauma -20, Mortal Strike -50.
func TestHealingTakenPctsDontStack(t *testing.T) {
	unit := &Unit{Label: "healed"}
	unit.PseudoStats = stats.NewPseudoStats()
	steps := []struct {
		pct  int32
		want float64
	}{{6, 1.06}, {40, 1.4}, {10, 1.4}, {-20, 1.12}, {-50, 0.7}}
	for _, step := range steps {
		unit.AddHealingTakenPct(step.pct)
		if got := unit.PseudoStats.HealingTakenMultiplier; got != step.want {
			t.Errorf("after adding %d%%: healing taken x%v, want x%v", step.pct, got, step.want)
		}
	}
	// in another order than they went on
	for _, step := range []struct {
		pct  int32
		want float64
	}{{-50, 1.12}, {40, 0.88}, {6, 0.88}, {-20, 1.1}, {10, 1}} {
		unit.RemoveHealingTakenPct(step.pct)
		if got := unit.PseudoStats.HealingTakenMultiplier; got != step.want {
			t.Errorf("after removing %d%%: healing taken x%v, want x%v", step.pct, got, step.want)
		}
	}

	// the server's float(100) x 1.12f is a heal of 112, which 1.4 x 0.8 as float64s would make 111
	unit.AddHealingTakenPct(40)
	unit.AddHealingTakenPct(-20)
	if got := wholeHeal(100 * unit.PseudoStats.HealingTakenMultiplier); got != 112 {
		t.Errorf("a heal of 100 under 40%% and -20%% is %v, want 112", got)
	}
	unit.RemoveHealingTakenPct(40)
	unit.RemoveHealingTakenPct(-20)

	// Strength of Wrynn's healing done stands in as a plain multiplier, which the auras leave alone
	unit.PseudoStats.HealingTakenMultiplier *= 1.3
	unit.AddHealingTakenPct(6)
	unit.AddHealingTakenPct(35)
	if got := unit.PseudoStats.HealingTakenMultiplier; got < 1.3*1.35-1e-12 || got > 1.3*1.35+1e-12 {
		t.Errorf("x1.3 with 6%% and 35%%: healing taken x%v, want x%v", got, 1.3*1.35)
	}
}
