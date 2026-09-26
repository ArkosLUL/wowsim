package deathknight

import (
	"github.com/wowsims/wotlk/sim/core"
)

var HeartStrikeActionID = core.ActionID{SpellID: 55262}

func (dk *Deathknight) newHeartStrikeSpell(isMainTarget bool, isDrw bool) *core.Spell {
	diseaseMulti := dk.dkDiseaseMultiplier(0.1)

	critMultiplier := dk.bonusCritMultiplier(dk.Talents.MightOfMograine)

	conf := core.SpellConfig{
		ActionID:    HeartStrikeActionID.WithTag(core.TernaryInt32(isMainTarget, 1, 2)),
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
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
		// the second target takes the effects' 0.5 chain multiplier
		DamageMultiplier: core.TernaryFloat64(isMainTarget, 1, 0.5) *
			dk.thassariansPlateDamageBonus() *
			dk.scourgelordsBattlegearDamageBonus(ScourgelordBonusSpellHS) *
			dk.bloodyStrikesBonus(BloodyStrikesHS),
		CritMultiplier:   critMultiplier,
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 736, Max: 736, WeaponPct: 0.5},
		Mods:   []core.SpellMod{{Op: core.SpellModEffect1, Flat: dk.sigilOfTheDarkRiderBonus()}},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			var baseDamage float64
			if isDrw {
				baseDamage = (spell.Direct.Roll(sim) + dk.DrwWeaponDamage(sim, spell)) * spell.Direct.WeaponPct
			} else {
				baseDamage = normalizedStrikeBase(sim, spell, true, spell.Direct.Roll(sim)) * spell.Direct.WeaponPct
			}

			activeDiseases := core.TernaryFloat64(isDrw, dk.drwCountActiveDiseases(target), dk.dkCountActiveDiseases(target))
			baseDamage *= 1 + activeDiseases*diseaseMulti

			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)

			if isMainTarget {
				if isDrw {
					if dk.Env.GetNumTargets() > 1 {
						dk.RuneWeapon.HeartStrikeOffHit.Cast(sim, dk.Env.NextTargetUnit(target))
					}
				} else {
					spell.SpendRefundableCost(sim, result)

					if dk.Env.GetNumTargets() > 1 {
						dk.HeartStrikeOffHit.Cast(sim, dk.Env.NextTargetUnit(target))
					}
				}
			}
		},
	}
	if !isMainTarget || isDrw { // off target doesnt need GCD
		conf.RuneCost = core.RuneCostOptions{}
		conf.Cast = core.CastConfig{}
	}

	if isMainTarget {
		conf.Flags |= core.SpellFlagAPL
	}

	if isDrw {
		return dk.RuneWeapon.RegisterSpell(conf)
	} else {
		return dk.RegisterSpell(conf)
	}
}

func (dk *Deathknight) registerHeartStrikeSpell() {
	if !dk.Talents.HeartStrike {
		return
	}

	dk.HeartStrike = dk.newHeartStrikeSpell(true, false)
	dk.HeartStrikeOffHit = dk.newHeartStrikeSpell(false, false)
}

func (dk *Deathknight) registerDrwHeartStrikeSpell() {
	if !dk.Talents.HeartStrike {
		return
	}

	dk.RuneWeapon.HeartStrike = dk.newHeartStrikeSpell(true, true)
	dk.RuneWeapon.HeartStrikeOffHit = dk.newHeartStrikeSpell(false, true)
}
