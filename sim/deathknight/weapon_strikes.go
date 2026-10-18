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
	return offHandStrikeFixed(spell, fixed) + spell.OHNormalizedStrikeDamage(sim, spell.MeleeAttackPower())
}

// offHandStrikeFixed is an off-hand strike's fixed bonus after the hand's TOTAL_PCT. The DBC already halves
// Threat of Thassarian's, so Spell::EffectWeaponDmg doubles a physical one back before the off hand's half.
func offHandStrikeFixed(spell *core.Spell, fixed float64) float64 {
	if spell.SpellSchool.Matches(core.SpellSchoolPhysical) {
		fixed *= 2
	}
	return spell.StrikeFixed(fixed, core.OffHand)
}
