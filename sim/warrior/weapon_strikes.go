package warrior

import (
	"github.com/wowsims/wotlk/sim/core"
)

// normalizedStrike is a physical strike's normalized weapon hit plus its fixed bonus e, times e's weapon
// percent, in Spell::EffectWeaponDmg's whole steps: for a strike whose server effects list the fixed bonus
// before the percent.
func normalizedStrike(sim *core.Simulation, spell *core.Spell, e *core.SpellEffect, isMH bool) float64 {
	if isMH {
		return core.ApplyPct(spell.StrikeFixed(e.Roll(sim), core.MainHand)+spell.MHNormalizedStrikeDamage(sim, spell.MeleeAttackPower()), e.WeaponPct)
	}
	return core.ApplyPct(spell.StrikeFixed(e.Roll(sim), core.OffHand)+spell.OHNormalizedStrikeDamage(sim, spell.MeleeAttackPower()), e.WeaponPct)
}
