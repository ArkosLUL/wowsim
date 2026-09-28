package rogue

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

func (rogue *Rogue) registerAmbushSpell() {
	rogue.Ambush = rogue.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48691},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage | SpellFlagBuilder | SpellFlagColdBlooded | core.SpellFlagAPL,

		EnergyCost: core.EnergyCostOptions{
			Cost:   rogue.costModifier(60 - 4*float64(rogue.Talents.SlaughterFromTheShadows)),
			Refund: 0,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: time.Second,
			},
			IgnoreHaste: true,
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return !rogue.PseudoStats.InFrontOfTarget && rogue.HasDagger(core.MainHand) && rogue.IsStealthed()
		},

		BonusCritRating: []float64{0, 2, 4, 6}[rogue.Talents.TurnTheTables]*core.CritRatingPerCritChance +
			25*core.CritRatingPerCritChance*float64(rogue.Talents.ImprovedAmbush),
		// Find Weakness and Opportunity are each their own SPELLMOD_DAMAGE (Player::ApplySpellMod
		// multiplies them, it doesn't add them).
		DamageMultiplier: spellModDamage(
			0.02*float64(rogue.Talents.FindWeakness),
			0.1*float64(rogue.Talents.Opportunity),
		),
		CritMultiplier:   rogue.MeleeCritMultiplier(true),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 330, Max: 330, WeaponPct: 2.75},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			rogue.BreakStealth(sim)
			baseDamage := normalizedStrike(sim, spell, &spell.Direct, true)

			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)

			if result.Landed() {
				rogue.AddComboPoints(sim, 2, spell.ComboPointMetrics())
			} else {
				spell.IssueRefund(sim)
			}
		},
	})
}
