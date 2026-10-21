package druid

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

func (druid *Druid) registerFerociousBiteSpell() {
	druid.FerociousBite = druid.RegisterSpell(Cat, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48577},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage | core.SpellFlagAPL,

		EnergyCost: core.EnergyCostOptions{
			Cost:          35,
			Refund:        0.4 * float64(druid.Talents.PrimalPrecision),
			RefundMetrics: druid.PrimalPrecisionRecoveryMetrics,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: time.Second,
			},
			IgnoreHaste: true,
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return druid.ComboPoints() > 0
		},

		BonusCritRating: 0 +
			core.TernaryFloat64(druid.HasSetBonus(ItemSetMalfurionsBattlegear, 4), 5*core.CritRatingPerCritChance, 0.0) +
			core.TernaryFloat64(druid.AssumeBleedActive, rendAndTearBiteCrit(druid.Talents.RendAndTear)*core.CritRatingPerCritChance, 0),
		DamageMultiplier: (1 + 0.03*float64(druid.Talents.FeralAggression)) *
			core.TernaryFloat64(druid.HasSetBonus(ItemSetThunderheartHarness, 4), 1.15, 1.0),
		CritMultiplier:   druid.MeleeCritMultiplier(Cat),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 120, Max: 260},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			comboPoints := float64(druid.ComboPoints())
			attackPower := spell.MeleeAttackPower()
			excessEnergy := min(druid.CurrentEnergy(), 30)

			// 290 a combo point from the effect, the energy and AP shares from Spell::EffectSchoolDMG
			baseDamage := sim.RollWhole(spell.Direct.Min, spell.Direct.Max, "Ferocious Bite") +
				290*comboPoints +
				core.PowerBonus(9.4+attackPower/410, excessEnergy) +
				core.ApplyPct(comboPoints*attackPower, 0.07)

			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)

			if result.Landed() {
				druid.SpendEnergy(sim, excessEnergy, spell.Cost.(*core.EnergyCost).ResourceMetrics)
				druid.SpendComboPoints(sim, spell.ComboPointMetrics())
			} else {
				spell.IssueRefund(sim)
			}
		},
	})
}

func (druid *Druid) CurrentFerociousBiteCost() float64 {
	return druid.FerociousBite.ApplyCostModifiers(druid.FerociousBite.DefaultCast.Cost)
}
