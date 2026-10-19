package deathknight

import (
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

var PlagueStrikeActionID = core.ActionID{SpellID: 49921}

var plagueStrikeEffect = core.SpellEffect{Effect: 0, Min: 378, Max: 378, WeaponPct: 0.5}

func (dk *Deathknight) newPlagueStrikeSpell(isMH bool) *core.Spell {
	conf := core.SpellConfig{
		ActionID:    PlagueStrikeActionID.WithTag(core.TernaryInt32(isMH, 1, 2)),
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    dk.threatOfThassarianProcMask(isMH),
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage,

		RuneCost: core.RuneCostOptions{
			UnholyRuneCost: 1,
			RunicPowerGain: 10 + 2.5*float64(dk.Talents.Dirge),
			Refundable:     true,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
		},

		BonusCritRating:    (dk.annihilationCritBonus() + dk.scourgebornePlateCritBonus() + dk.viciousStrikesCritChanceBonus()) * core.CritRatingPerCritChance,
		TotalPctMultiplier: core.TernaryFloat64(isMH, 1, dk.nervesOfColdSteelBonus()),
		DamageMultiplier: core.TernaryFloat64(isMH, 1, dk.nervesOfColdSteelBonus()) *
			(1.0 + 0.1*float64(dk.Talents.Outbreak)) *
			core.TernaryFloat64(dk.HasMajorGlyph(proto.DeathknightMajorGlyph_GlyphOfPlagueStrike), 1.2, 1.0),
		CritMultiplier:   dk.bonusCritMultiplier(dk.Talents.ViciousStrikes),
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := core.ApplyPct(normalizedStrikeBase(sim, spell, isMH, spell.Direct.Roll(sim)), spell.Direct.WeaponPct)
			baseDamage *= dk.RoRTSBonus(target)
			// the class scripts are DoneTotalMod, which scales the flat creature-type bonus too
			baseDamage += spell.WeaponBonusForDonePct(target, dk.RoRTSBonus(target))

			result := spell.CalcDamage(sim, target, baseDamage, dk.threatOfThassarianOutcomeApplier(spell))

			if isMH {
				spell.SpendRefundableCost(sim, result)
				// Blood Plague is the strike's launch effect, so it's up before the off hand's strike
				if result.Landed() {
					dk.BloodPlagueExtended[target.Index] = 0
					dk.BloodPlagueSpell.Cast(sim, target)
				}
				dk.threatOfThassarianProc(sim, result, dk.PlagueStrikeOhHit)
			}

			spell.DealDamage(sim, result)
		},
	}
	if !isMH { // only MH has cost & gcd
		conf.RuneCost = core.RuneCostOptions{}
		conf.Cast = core.CastConfig{}
		// Threat of Thassarian's off-hand strike, always rank 1's 66216: the script looks up the rank from 66216,
		// but spell_ranks starts the off-hand chain at 59133, so there's no higher rank to find
		conf.Direct = core.SpellEffect{Effect: 0, FromSpellID: 66216, Min: 62, Max: 62, WeaponPct: 0.5}
		conf.ThreatSpellID = 66216
	} else {
		conf.Flags |= core.SpellFlagAPL
		conf.Direct = plagueStrikeEffect
	}

	return dk.RegisterSpell(conf)
}

func (dk *Deathknight) registerPlagueStrikeSpell() {
	dk.PlagueStrikeMhHit = dk.newPlagueStrikeSpell(true)
	dk.PlagueStrikeOhHit = dk.newPlagueStrikeSpell(false)
	dk.PlagueStrike = dk.PlagueStrikeMhHit
}
func (dk *Deathknight) registerDrwPlagueStrikeSpell() {
	dk.RuneWeapon.PlagueStrike = dk.RuneWeapon.RegisterSpell(core.SpellConfig{
		ActionID:    PlagueStrikeActionID.WithTag(1),
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage,

		BonusCritRating:  (dk.annihilationCritBonus() + dk.scourgebornePlateCritBonus() + dk.viciousStrikesCritChanceBonus()) * core.CritRatingPerCritChance,
		DamageMultiplier: 1.0 + 0.1*float64(dk.Talents.Outbreak),
		CritMultiplier:   dk.drwBonusCritMultiplier(dk.Talents.ViciousStrikes),
		ThreatMultiplier: 1,

		Direct: plagueStrikeEffect,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// Rage of Rivendare and Tundra Stalker scale the flat bonus too
			baseDamage := (core.ApplyPct(spell.Direct.Roll(sim)+dk.DrwWeaponDamage(sim, true), spell.Direct.WeaponPct) +
				dk.drwVersusAttackPowerBonus(spell, true)) * dk.RoRTSBonus(target)

			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)

			if result.Landed() {
				dk.RuneWeapon.BloodPlagueSpell.Cast(sim, target)
			}
		},
	})
}
