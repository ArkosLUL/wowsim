package warrior

import (
	"github.com/wowsims/wotlk/sim/core"
)

// Spell::EffectWeaponDmg scales a physical strike's fixed bonus by its hand's TOTAL_PCT, whose base is
// 0.5 in the off hand. OHNormalizedWeaponDamage already halves the weapon.
const offHandFixedPct = 0.5

// normalizedStrike is a physical strike's normalized weapon hit plus its fixed bonus e, times e's weapon
// percent: for a strike whose server effects list the fixed bonus before the percent.
func normalizedStrike(sim *core.Simulation, spell *core.Spell, e *core.SpellEffect, isMH bool) float64 {
	if isMH {
		return (e.Roll(sim) + spell.Unit.MHNormalizedWeaponDamage(sim, spell.MeleeAttackPower()) + spell.BonusWeaponDamage()) * e.WeaponPct
	}
	return (offHandFixedPct*e.Roll(sim) + spell.Unit.OHNormalizedWeaponDamage(sim, spell.MeleeAttackPower()) + spell.BonusWeaponDamage()) * e.WeaponPct
}
