package druid

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

const IdolAvenger int32 = 31025
const IdolSteadfastRenewal int32 = 40712

func (druid *Druid) registerWrathSpell() {
	bonusFlatDamage := core.TernaryFloat64(druid.Ranged().ID == IdolAvenger, 25, 0) +
		core.TernaryFloat64(druid.Ranged().ID == IdolSteadfastRenewal, 70, 0)
	impInsectSwarm := 1 + 0.01*float64(druid.Talents.ImprovedInsectSwarm)

	druid.Wrath = druid.RegisterSpell(Humanoid|Moonkin, core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 48461},
		SpellSchool:  core.SpellSchoolNature,
		ProcMask:     core.ProcMaskSpellDamage,
		Flags:        SpellFlagNaturesGrace | SpellFlagOmenTrigger | core.SpellFlagAPL,
		MissileSpeed: 20,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.11,
			Multiplier: 1 - 0.03*float64(druid.Talents.Moonglow),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Second*2 - time.Millisecond*100*time.Duration(druid.Talents.StarlightWrath),
			},
		},

		BonusCritRating: 0 +
			2*float64(druid.Talents.NaturesMajesty)*core.CritRatingPerCritChance +
			core.TernaryFloat64(druid.HasSetBonus(ItemSetDreamwalkerGarb, 4), 5*core.CritRatingPerCritChance, 0),
		DamageMultiplier: spellModDamage(
			[]float64{0.0, 0.03, 0.06, 0.1}[druid.Talents.Moonfury],
			core.TernaryFloat64(druid.HasSetBonus(ItemSetMalfurionsRegalia, 4), 0.04, 0),
		),
		CritMultiplier:   druid.BalanceCritMultiplier(),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 557, Max: 627, SP: 0.571},
		Mods: []core.SpellMod{
			{Op: core.SpellModBonusMultiplier, Flat: 2 * druid.Talents.WrathOfCenarius},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			roll := spell.Direct.Roll(sim)
			// Improved Insect Swarm scales the roll alone, before spell power (Spell::EffectSchoolDMG)
			if druid.InsectSwarm != nil && druid.InsectSwarm.Dot(target).IsActive() {
				roll *= impInsectSwarm
			}
			baseDamage := bonusFlatDamage + roll + spell.Direct.SP*spell.SpellPower()
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				spell.DealDamage(sim, result)
			})
		},
	})
}
