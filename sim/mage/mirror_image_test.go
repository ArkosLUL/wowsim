package mage

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// A Mirror Image (31216) as Guardian::InitStatsForLevel and spell_mage_pet_scaling build it: the
// fallback create stats, a creature's flat 5% crit, and from the owner at the summon 30% of his
// stamina and intellect, 33% of his spell power and 35% of his armor.
func TestMirrorImageStats(t *testing.T) {
	sim, mage := newProcSim(t, procMage("", "p5_bis_arcane", `{"type": "TypeAPL", "priorityList": []}`, PlayerOptionsArcane))
	image := mage.mirrorImage
	image.Enable(sim, image)

	owner := mage.GetStats()
	if owner[stats.SpellPower] < 1000 || owner[stats.Intellect] < 500 {
		t.Fatalf("owner spell power %v, intellect %v: the gear set isn't on", owner[stats.SpellPower], owner[stats.Intellect])
	}
	stamina := core.PetScalingPct(owner[stats.Stamina], 30)
	intellect := core.PetScalingPct(owner[stats.Intellect], 30)
	got := image.GetStats()
	for _, tc := range []struct {
		name       string
		have, want float64
	}{
		{"strength", got[stats.Strength], 22},
		{"agility", got[stats.Agility], 22},
		{"stamina", got[stats.Stamina], 25 + stamina},
		{"intellect", got[stats.Intellect], 28 + intellect},
		{"spirit", got[stats.Spirit], 27},
		{"health", image.MaxHealth(), 4032 + 10*stamina},
		{"mana", image.MaxMana(), 8814 + 15*intellect},
		{"armor", got[stats.Armor], 4000 + core.PetScalingPct(owner[stats.Armor], 35)},
		{"attack power", got[stats.AttackPower], 2*22 - 20},
		{"spell power", got[stats.SpellPower], core.PetScalingPct(owner[stats.SpellPower], 33)},
		{"spell crit rating", got[stats.SpellCrit], 5 * core.CritRatingPerCritChance},
	} {
		if !core.WithinToleranceFloat64(tc.want, tc.have, 1e-6) {
			t.Errorf("%s %v, want %v", tc.name, tc.have, tc.want)
		}
	}
	if image.BaseMana != 2428 {
		t.Errorf("create mana %v, want 2428", image.BaseMana)
	}
	// 59638 and 59637 cost a flat 90 and 120
	if bolt, blast := image.Frostbolt.DefaultCast.Cost, image.Fireblast.DefaultCast.Cost; bolt != 90 || blast != 120 {
		t.Errorf("Frostbolt costs %v and Fire Blast %v, want 90 and 120", bolt, blast)
	}
}
