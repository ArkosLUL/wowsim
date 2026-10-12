package mage

import (
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (mage *Mage) registerArcaneExplosionSpell() {
	mage.ArcaneExplosion = mage.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 42921},
		SpellSchool: core.SpellSchoolArcane,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       SpellFlagMage | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.22,
			Multiplier: 1 - core.TernaryFloat64(mage.HasMajorGlyph(proto.MageMajorGlyph_GlyphOfArcaneExplosion), .1, 0),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		BonusHitRating:           float64(mage.Talents.ArcaneFocus) * core.SpellHitRatingPerHitChance,
		BonusCritRating:          float64(mage.Talents.WorldInFlames) * 2 * core.CritRatingPerCritChance,
		DamageMultiplier:         1,
		DamageMultiplierAdditive: spellModDamage(.02 * float64(mage.Talents.SpellImpact)),
		CritMultiplier:           mage.SpellCritMultiplier(1, mage.bonusCritDamage),
		ThreatMultiplier:         1,

		Direct: core.SpellEffect{Effect: 0, Min: 538, Max: 582, SP: 0.214},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			dmgFromSP := core.PowerBonus(spell.Direct.SP, spell.SpellPower())
			for _, aoeTarget := range sim.Encounter.TargetUnits {
				baseDamage := spell.Direct.Roll(sim) + dmgFromSP
				spell.AoECapUnits = len(sim.Encounter.Targets)
				spell.CalcAndDealDamage(sim, aoeTarget, baseDamage, spell.OutcomeMagicHitAndCrit)
			}
			// spell_proc 36032 takes the Arcane Blast stacks off at the cast, once every target has its damage
			mage.ArcaneBlastAura.Deactivate(sim)
		},
	})
}
