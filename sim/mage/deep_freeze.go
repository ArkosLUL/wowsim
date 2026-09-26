package mage

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

func (mage *Mage) registerDeepFreezeSpell() {
	if !mage.Talents.DeepFreeze {
		return
	}

	mage.DeepFreeze = mage.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 44572},
		SpellSchool: core.SpellSchoolFrost,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       SpellFlagMage | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.09,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    mage.NewTimer(),
				Duration: time.Second * 30,
			},
		},

		DamageMultiplier: 1,
		CritMultiplier:   mage.SpellCritMultiplier(1, mage.bonusCritDamage+float64(mage.Talents.IceShards)/3),
		ThreatMultiplier: 1 - (0.1/3)*float64(mage.Talents.FrostChanneling),

		// the damage is 71757's, which Deep Freeze Immunity State (71761) casts on a stun immune target
		Direct: core.SpellEffect{Effect: 0, FromSpellID: 71757, Min: 2369, Max: 2641, SP: 2.143},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) + spell.Direct.SP*spell.SpellPower()
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
		},
	})
}
