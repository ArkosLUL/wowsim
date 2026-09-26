package mage

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

func (mage *Mage) registerFireBlastSpell() {
	mage.FireBlast = mage.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 42873},
		SpellSchool: core.SpellSchoolFire,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       SpellFlagMage | HotStreakSpells | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.21,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    mage.NewTimer(),
				Duration: time.Second*8 - time.Second*time.Duration(mage.Talents.ImprovedFireBlast),
			},
		},

		BonusCritRating: 0 +
			float64(mage.Talents.CriticalMass+mage.Talents.Incineration)*2*core.CritRatingPerCritChance,
		DamageMultiplierAdditive: 1 +
			.02*float64(mage.Talents.SpellImpact) +
			.02*float64(mage.Talents.FirePower),
		CritMultiplier:   mage.SpellCritMultiplier(1, mage.bonusCritDamage),
		ThreatMultiplier: 1 - 0.1*float64(mage.Talents.BurningSoul),

		Direct: core.SpellEffect{Effect: 0, Min: 925, Max: 1095, SP: 0.4286},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) + spell.Direct.SP*spell.SpellPower()
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
		},
	})
}
