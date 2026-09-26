package mage

import (
	"github.com/wowsims/wotlk/sim/core"
)

func (mage *Mage) registerIceLanceSpell() {
	mage.IceLance = mage.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 42914},
		SpellSchool:  core.SpellSchoolFrost,
		ProcMask:     core.ProcMaskSpellDamage,
		Flags:        SpellFlagMage | core.SpellFlagAPL,
		MissileSpeed: 38,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.06,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		DamageMultiplierAdditive: 1 + .01*float64(mage.Talents.ChilledToTheBone),
		CritMultiplier:           mage.SpellCritMultiplier(1, mage.bonusCritDamage+float64(mage.Talents.IceShards)/3),
		ThreatMultiplier:         1 - (0.1/3)*float64(mage.Talents.FrostChanneling),

		Direct: core.SpellEffect{Effect: 0, Min: 223, Max: 257, SP: 0.1429},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) + spell.Direct.SP*spell.SpellPower()
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				spell.DealDamage(sim, result)
			})
		},
	})
}
