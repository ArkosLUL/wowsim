package deathknight

import (
	"github.com/wowsims/wotlk/sim/core"
)

// Spell::EffectWeaponDmg scales a physical strike's fixed bonus by its hand's TOTAL_PCT, whose base is
// 0.5 in the off hand. OHNormalizedWeaponDamage already halves the weapon, and Nerves of Cold Steel
// rides in DamageMultiplier, so only the fixed bonus is left to halve.
const offHandFixedPct = 0.5

// normalizedStrikeBase is a physical strike's normalized weapon damage plus its fixed bonus, before
// the weapon percent.
func normalizedStrikeBase(sim *core.Simulation, spell *core.Spell, isMH bool, fixed float64) float64 {
	if isMH {
		return fixed + spell.Unit.MHNormalizedWeaponDamage(sim, spell.MeleeAttackPower()) + spell.BonusWeaponDamage()
	}
	return offHandFixedPct*fixed + spell.Unit.OHNormalizedWeaponDamage(sim, spell.MeleeAttackPower()) + spell.BonusWeaponDamage()
}
