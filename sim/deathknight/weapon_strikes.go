package deathknight

import (
	"github.com/wowsims/wotlk/sim/core"
)

// normalizedStrikeBase is a strike's normalized weapon damage plus its fixed bonus, before the weapon
// percent, in Spell::EffectWeaponDmg's whole steps.
func normalizedStrikeBase(sim *core.Simulation, spell *core.Spell, isMH bool, fixed float64) float64 {
	if isMH {
		return spell.StrikeFixed(fixed, core.MainHand) + spell.MHNormalizedStrikeDamage(sim, spell.MeleeAttackPower())
	}
	return spell.StrikeFixed(fixed, core.OffHand) + spell.OHNormalizedStrikeDamage(sim, spell.MeleeAttackPower())
}
