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
		return spell.Direct.Roll(sim) + 95 + core.PowerBonus(spell.Direct.AP*1.5835, spell.MeleeAttackPower())
	}
	return spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.AP, impurityAP)
}

func (dk *Deathknight) registerBloodBoilSpell() {
	// no refund: with no unit target (Spell::InitExplicitTargets drops it), Spell::TakePower counts it a hit
	dk.BloodBoil = dk.RegisterSpell(core.SpellConfig{
		ActionID:    BloodBoilActionID,
		Flags:       core.SpellFlagAPL,
		SpellSchool: core.SpellSchoolShadow,
		ProcMask:    core.ProcMaskSpellDamage,

		RuneCost: core.RuneCostOptions{
			BloodRuneCost:  1,
			RunicPowerGain: 10,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		DamageMultiplier: dk.bloodyStrikesBonus(BloodyStrikesBB),
		CritMultiplier:   dk.magicCritMultiplier(dk.Talents.MightOfMograine),
		ThreatMultiplier: 1.0,

		Direct: bloodBoilEffect,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			for _, aoeTarget := range sim.Encounter.TargetUnits {
				baseDamage := bloodBoilBase(sim, spell, dk.DiseasesAreActive(aoeTarget), dk.getImpurityBonus(spell)) * dk.RoRTSBonus(aoeTarget)
				spell.AoECapUnits = len(sim.Encounter.Targets)

				spell.CalcAndDealDamage(sim, aoeTarget, baseDamage, spell.OutcomeMagicHitAndCrit)
			}
		},
	})
}
