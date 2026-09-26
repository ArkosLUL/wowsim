package deathknight

import (
	"github.com/wowsims/wotlk/sim/core"
)

var IcyTouchActionID = core.ActionID{SpellID: 49909}

func (dk *Deathknight) icyTouchEffect() (core.SpellEffect, []core.SpellMod) {
	return core.SpellEffect{Effect: 0, Min: 227, Max: 245, AP: 0.1},
		[]core.SpellMod{{Op: core.SpellModEffect1, Flat: dk.sigilOfTheFrozenConscienceBonus()}}
}

func (dk *Deathknight) registerIcyTouchSpell() {
	direct, mods := dk.icyTouchEffect()

	dk.IcyTouch = dk.RegisterSpell(core.SpellConfig{
		ActionID:    IcyTouchActionID,
		Flags:       core.SpellFlagAPL,
		SpellSchool: core.SpellSchoolFrost,
		ProcMask:    core.ProcMaskSpellDamage,

		RuneCost: core.RuneCostOptions{
			FrostRuneCost:  1,
			RunicPowerGain: 10 + 2.5*float64(dk.Talents.ChillOfTheGrave),
			Refundable:     true,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		BonusCritRating:  dk.rimeCritBonus() * core.CritRatingPerCritChance,
		DamageMultiplier: 1 + 0.05*float64(dk.Talents.ImprovedIcyTouch),
		CritMultiplier:   dk.DefaultMeleeCritMultiplier(),
		ThreatMultiplier: 1.0,

		Direct: direct,
		Mods:   mods,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := (spell.Direct.Roll(sim) + spell.Direct.AP*dk.getImpurityBonus(spell)) *
				dk.glacielRotBonus(target) *
				dk.RoRTSBonus(target) *
				dk.mercilessCombatBonus(sim)

			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			spell.SpendRefundableCost(sim, result)

			if result.Landed() {
				dk.FrostFeverExtended[target.Index] = 0
				dk.FrostFeverSpell.Cast(sim, target)
			}

			spell.DealDamage(sim, result)
		},
	})
}
func (dk *Deathknight) registerDrwIcyTouchSpell() {
	direct, mods := dk.icyTouchEffect()

	dk.RuneWeapon.IcyTouch = dk.RuneWeapon.RegisterSpell(core.SpellConfig{
		ActionID:    IcyTouchActionID,
		SpellSchool: core.SpellSchoolFrost,
		ProcMask:    core.ProcMaskSpellDamage,
		//Flags:       core.SpellFlagIgnoreAttackerModifiers,

		BonusCritRating:  dk.rimeCritBonus() * core.CritRatingPerCritChance,
		DamageMultiplier: 1 + 0.05*float64(dk.Talents.ImprovedIcyTouch),
		CritMultiplier:   dk.DefaultMeleeCritMultiplier(),
		ThreatMultiplier: 1,

		Direct: direct,
		Mods:   mods,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := spell.Direct.Roll(sim) + spell.Direct.AP*dk.RuneWeapon.getImpurityBonus(spell)

			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			if result.Landed() {
				dk.RuneWeapon.FrostFeverSpell.Cast(sim, target)
			}
			spell.DealDamage(sim, result)
		},
	})
}
