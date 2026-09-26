package deathknight

import (
	"github.com/wowsims/wotlk/sim/core"
)

var BloodBoilActionID = core.ActionID{SpellID: 49941}

var bloodBoilEffect = core.SpellEffect{Effect: 0, Min: 180, Max: 220, AP: 0.06}

// bloodBoilBase is SpellDamageBonusDone's Blood Boil case: a target with the caster's Blood Plague or
// Frost Fever takes 95 more, and the AP coefficient is multiplied by 1.5835 in place of Impurity's bonus.
func bloodBoilBase(sim *core.Simulation, spell *core.Spell, diseased bool, impurityAP float64) float64 {
	if diseased {
		return spell.Direct.Roll(sim) + 95 + spell.Direct.AP*1.5835*spell.MeleeAttackPower()
	}
	return spell.Direct.Roll(sim) + spell.Direct.AP*impurityAP
}

func (dk *Deathknight) registerBloodBoilSpell() {
	// TODO: Handle blood boil correctly -
	//  There is no refund and you only get RP on at least one of the effects hitting.
	dk.BloodBoil = dk.RegisterSpell(core.SpellConfig{
		ActionID:    BloodBoilActionID,
		Flags:       core.SpellFlagAPL,
		SpellSchool: core.SpellSchoolShadow,
		ProcMask:    core.ProcMaskSpellDamage,

		RuneCost: core.RuneCostOptions{
			BloodRuneCost:  1,
			RunicPowerGain: 10,
			Refundable:     true,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		DamageMultiplier: dk.bloodyStrikesBonus(BloodyStrikesBB),
		CritMultiplier:   dk.bonusCritMultiplier(dk.Talents.MightOfMograine),
		ThreatMultiplier: 1.0,

		Direct: bloodBoilEffect,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			for _, aoeTarget := range sim.Encounter.TargetUnits {
				baseDamage := bloodBoilBase(sim, spell, dk.DiseasesAreActive(aoeTarget), dk.getImpurityBonus(spell)) * dk.RoRTSBonus(aoeTarget)
				baseDamage *= sim.Encounter.AOECapMultiplier()

				result := spell.CalcAndDealDamage(sim, aoeTarget, baseDamage, spell.OutcomeMagicHitAndCrit)

				if aoeTarget == target {
					spell.SpendRefundableCost(sim, result)
				}
			}
		},
	})
}

func (dk *Deathknight) registerDrwBloodBoilSpell() {
	dk.RuneWeapon.BloodBoil = dk.RuneWeapon.RegisterSpell(core.SpellConfig{
		ActionID:    BloodBoilActionID,
		SpellSchool: core.SpellSchoolShadow,
		ProcMask:    core.ProcMaskSpellDamage,

		DamageMultiplier: dk.bloodyStrikesBonus(BloodyStrikesBB),
		CritMultiplier:   dk.bonusCritMultiplier(dk.Talents.MightOfMograine),
		ThreatMultiplier: 1,

		Direct: bloodBoilEffect,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			for _, aoeTarget := range sim.Encounter.TargetUnits {
				baseDamage := bloodBoilBase(sim, spell, dk.DrwDiseasesAreActive(aoeTarget), dk.RuneWeapon.getImpurityBonus(spell))
				baseDamage *= sim.Encounter.AOECapMultiplier()

				spell.CalcAndDealDamage(sim, aoeTarget, baseDamage, spell.OutcomeMagicHitAndCrit)
			}
		},
	})
}
