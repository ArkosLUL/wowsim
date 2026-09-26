package rogue

import (
	"math"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (rogue *Rogue) registerEviscerate() {
	rogue.Eviscerate = rogue.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 48668},
		SpellSchool:  core.SpellSchoolPhysical,
		ProcMask:     core.ProcMaskMeleeMHSpecial,
		Flags:        core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage | rogue.finisherFlags() | SpellFlagColdBlooded | core.SpellFlagAPL,
		MetricSplits: 6,

		EnergyCost: core.EnergyCostOptions{
			Cost:          35,
			Refund:        0.4 * float64(rogue.Talents.QuickRecovery),
			RefundMetrics: rogue.QuickRecoveryMetrics,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: time.Second,
			},
			IgnoreHaste: true,
			ModifyCast: func(sim *core.Simulation, spell *core.Spell, cast *core.Cast) {
				spell.SetMetricsSplit(spell.Unit.ComboPoints())
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return rogue.ComboPoints() > 0
		},

		BonusCritRating: core.TernaryFloat64(
			rogue.HasMajorGlyph(proto.RogueMajorGlyph_GlyphOfEviscerate), 10*core.CritRatingPerCritChance, 0.0),
		DamageMultiplier: 1 +
			[]float64{0.0, 0.07, 0.14, 0.2}[rogue.Talents.ImprovedEviscerate] +
			0.02*float64(rogue.Talents.FindWeakness) +
			0.03*float64(rogue.Talents.Aggression),
		CritMultiplier:   rogue.MeleeCritMultiplier(false),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 127, Max: 381, SP: 1},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			rogue.BreakStealth(sim)
			comboPoints := float64(rogue.ComboPoints())
			roll := spell.Direct.Min + (spell.Direct.Max-spell.Direct.Min)*sim.RandomFloat("Eviscerate")

			// the effect's 370 per combo point, then Spell::EffectSchoolDMG's 7% AP per point, in whole points
			baseDamage := roll + 370*comboPoints +
				math.Floor(0.07*comboPoints*spell.MeleeAttackPower()) +
				spell.BonusWeaponDamage()

			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)

			if result.Landed() {
				rogue.ApplyFinisher(sim, spell)
				rogue.ApplyCutToTheChase(sim)
			} else {
				spell.IssueRefund(sim)
			}

			spell.DealDamage(sim, result)
		},
	})
}
