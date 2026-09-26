package warrior

import (
	"math"
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

func (warrior *Warrior) registerConcussionBlowSpell() {
	if !warrior.Talents.ConcussionBlow {
		return
	}

	warrior.ConcussionBlow = warrior.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 12809},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		// spell_warr_concussion_blow sets the hit damage directly from attack power, skipping
		// SpellDamageBonusDone and its caster-side percent modifiers.
		Flags: core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage | core.SpellFlagAPL | core.SpellFlagIgnoreAttackerModifiers,

		RageCost: core.RageCostOptions{
			Cost:   15 - float64(warrior.Talents.FocusedRage),
			Refund: 0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    warrior.NewTimer(),
				Duration: time.Second * 30,
			},
		},

		DamageMultiplier: 1,
		CritMultiplier:   warrior.critMultiplier(mh),
		ThreatMultiplier: 2,

		// spell_warr_concussion_blow deals the dummy effect's 38 as a percent of attack power
		Direct: core.SpellEffect{Effect: 2, Min: 38, Max: 38},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := math.Floor(spell.Direct.Roll(sim) * spell.MeleeAttackPower() / 100)
			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)

			if !result.Landed() {
				spell.IssueRefund(sim)
			}
		},
	})
}
