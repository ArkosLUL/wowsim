package deathknight

import (
	"github.com/wowsims/wotlk/sim/core"
)

var BloodStrikeActionID = core.ActionID{SpellID: 49930}

func (dk *Deathknight) bloodStrikeEffect() (core.SpellEffect, []core.SpellMod) {
	return core.SpellEffect{Effect: 0, Min: 764, Max: 764, WeaponPct: 0.4},
		[]core.SpellMod{{Op: core.SpellModEffect1, Flat: dk.sigilOfTheDarkRiderBonus()}}
}

func (dk *Deathknight) newBloodStrikeSpell(isMH bool) *core.Spell {
	diseaseMulti := dk.dkDiseaseMultiplier(0.125)
	deathConvertChance := float64(dk.Talents.BloodOfTheNorth+dk.Talents.Reaping) / 3

	conf := core.SpellConfig{
		ActionID:    BloodStrikeActionID.WithTag(core.TernaryInt32(isMH, 1, 2)),
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    dk.threatOfThassarianProcMask(isMH),
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage,

		RuneCost: core.RuneCostOptions{
			BloodRuneCost:  1,
			RunicPowerGain: 10,
			Refundable:     true,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
		},

		BonusCritRating: (dk.subversionCritBonus() + dk.annihilationCritBonus()) * core.CritRatingPerCritChance,
		DamageMultiplier: core.TernaryFloat64(isMH, 1, dk.nervesOfColdSteelBonus()) *
			dk.bloodOfTheNorthCoeff() *
			dk.thassariansPlateDamageBonus() *
			dk.bloodyStrikesBonus(BloodyStrikesBS),
		CritMultiplier:   dk.bonusCritMultiplier(dk.Talents.MightOfMograine + dk.Talents.GuileOfGorefiend),
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := normalizedStrikeBase(sim, spell, isMH, spell.Direct.Roll(sim)) * spell.Direct.WeaponPct
			baseDamage *= dk.RoRTSBonus(target) *
				(1.0 + dk.dkCountActiveDiseases(target)*diseaseMulti)

			result := spell.CalcDamage(sim, target, baseDamage, dk.threatOfThassarianOutcomeApplier(spell))

			if isMH {
				spell.SpendRefundableCostAndConvertBloodRune(sim, result, deathConvertChance)
				dk.threatOfThassarianProc(sim, result, dk.BloodStrikeOhHit)

				if result.Landed() {
					if dk.DesolationAura != nil {
						dk.DesolationAura.Activate(sim)
					}
				}
			}

			spell.DealDamage(sim, result)
		},
	}

	conf.Direct, conf.Mods = dk.bloodStrikeEffect()
	if !isMH { // offhand doesn't need GCD
		conf.RuneCost = core.RuneCostOptions{}
		conf.Cast = core.CastConfig{}
		// Threat of Thassarian's off-hand strike
		conf.Direct = core.SpellEffect{Effect: 0, FromSpellID: 66979, Min: 382, Max: 382, WeaponPct: 0.4}
	} else {
		conf.Flags |= core.SpellFlagAPL
	}

	return dk.RegisterSpell(conf)
}

func (dk *Deathknight) registerBloodStrikeSpell() {
	dk.BloodStrikeMhHit = dk.newBloodStrikeSpell(true)
	dk.BloodStrikeOhHit = dk.newBloodStrikeSpell(false)
	dk.BloodStrike = dk.BloodStrikeMhHit
}

func (dk *Deathknight) registerDrwBloodStrikeSpell() {
	diseaseMulti := dk.dkDiseaseMultiplier(0.125)
	direct, mods := dk.bloodStrikeEffect()

	dk.RuneWeapon.BloodStrike = dk.RuneWeapon.RegisterSpell(core.SpellConfig{
		ActionID:    BloodStrikeActionID.WithTag(1),
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage,

		BonusCritRating: (dk.subversionCritBonus() + dk.annihilationCritBonus()) * core.CritRatingPerCritChance,
		DamageMultiplier: dk.bloodOfTheNorthCoeff() *
			dk.thassariansPlateDamageBonus() *
			dk.bloodyStrikesBonus(BloodyStrikesBS),
		CritMultiplier:   dk.bonusCritMultiplier(dk.Talents.MightOfMograine + dk.Talents.GuileOfGorefiend),
		ThreatMultiplier: 1,

		Direct: direct,
		Mods:   mods,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := (spell.Direct.Roll(sim) + dk.DrwWeaponDamage(sim, spell)) * spell.Direct.WeaponPct

			baseDamage *= dk.RoRTSBonus(target) *
				(1.0 + dk.drwCountActiveDiseases(target)*diseaseMulti)

			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)
		},
	})
}
